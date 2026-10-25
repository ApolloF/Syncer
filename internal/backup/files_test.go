package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestFileList(t *testing.T) {
	p := newTwoPCs(t)
	write(t, p.local("slot1.sav"), "one")
	write(t, p.local(filepath.Join("Sub", "Slot2.sav")), "two")
	res, err := Run(context.Background(), []Folder{p.f}, Options{Target: p.target, Device: "DEV-A"})
	if err != nil || !res.OK {
		t.Fatalf("run: %v %v", err, res)
	}
	ins := ReadInfos(p.target, p.f.ID)
	if len(ins) != 1 || ins[0].Device != "DEV-A" || ins[0].Files == "" {
		t.Fatalf("info: %+v", ins)
	}
	files, err := ReadFiles(p.target, ins[0])
	if err != nil || len(files) != 2 || files[0].Path != "Sub/Slot2.sav" || files[1].Path != "slot1.sav" {
		t.Fatalf("files = %+v, %v", files, err)
	}
	if !MirrorMatches(p.target, p.f.ID, files) {
		t.Error("the backup doesn't match its own list")
	}
	if !LocalMatchesIndex(p.f) {
		t.Error("nothing changed here since the backup")
	}

	// Drive hasn't brought one file yet.
	write(t, p.backup("slot1.sav"), "on its way")
	if MirrorMatches(p.target, p.f.ID, files) {
		t.Error("a half-arrived backup matches")
	}
	// The list itself is newer than the info file.
	lp := filesPath(p.target, p.f.ID, ins[0].key)
	write(t, lp, `{"id":"`+p.f.ID+`","files":[]}`)
	if _, err := ReadFiles(p.target, ins[0]); err == nil {
		t.Error("a list with another hash was read")
	}

	write(t, p.local("slot1.sav"), "changed here")
	if LocalMatchesIndex(p.f) {
		t.Error("a changed save matches the index")
	}
}

func TestSafeRel(t *testing.T) {
	for rel, ok := range map[string]bool{"a.sav": true, "Sub/b.sav": true, "../x": false, "a/../../x": false,
		`C:\Windows\x`: false, "/abs": false, "": false, "a:b": false, "./a": false} {
		if safeRel(rel) != ok {
			t.Errorf("safeRel(%q) = %v", rel, !ok)
		}
	}
}

// takeFixture is another PC's backup of a game with three files, and this
// PC's folder with an older slot1.sav, the same same.sav and a file of its own.
func takeFixture(t *testing.T) (target string, f Folder, files []FileEntry, at time.Time) {
	t.Helper()
	target = t.TempDir()
	local := t.TempDir()
	f = Folder{ID: "game", Label: "Game", Path: local}
	at = time.Now().Add(-time.Hour).Truncate(time.Second)
	for _, n := range []string{"slot1.sav", "Sub/slot2.sav", "same.sav"} {
		src := filepath.Join(target, "game", filepath.FromSlash(n))
		write(t, src, "newer "+n)
		setTime(t, src, at)
		files = append(files, FileEntry{Path: n, Size: int64(len("newer " + n)), MTime: at.UnixMilli()})
	}
	write(t, filepath.Join(local, "slot1.sav"), "older")
	write(t, filepath.Join(local, "same.sav"), "newer same.sav")
	write(t, filepath.Join(local, "only-here.sav"), "stays")
	return target, f, files, at
}

func TestTakeFiles(t *testing.T) {
	target, f, files, at := takeFixture(t)
	local := f.Path

	// The game holds slot1 open: nothing changes.
	h, err := windows.CreateFile(windows.StringToUTF16Ptr(filepath.Join(local, "slot1.sav")), windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := TakeFiles(context.Background(), target, f, files, nil); err == nil {
		t.Error("took files while one was open")
	}
	windows.CloseHandle(h)
	if read(t, filepath.Join(local, "slot1.sav")) != "older" {
		t.Error("slot1 changed although taking failed")
	}
	if _, err := os.Stat(filepath.Join(local, "Sub", "slot2.sav")); err == nil {
		t.Error("slot2 left behind by a failed take")
	}
	if pts := Points(target, f.ID); len(pts) != 0 {
		t.Errorf("a failed take left restore points: %v", pts)
	}

	n, err := TakeFiles(context.Background(), target, f, files, nil)
	if err != nil || n != 2 {
		t.Fatalf("take = %d, %v", n, err)
	}
	if read(t, filepath.Join(local, "slot1.sav")) != "newer slot1.sav" || read(t, filepath.Join(local, "Sub", "slot2.sav")) != "newer Sub/slot2.sav" ||
		read(t, filepath.Join(local, "only-here.sav")) != "stays" {
		t.Error("wrong files after taking")
	}
	if fi, _ := os.Stat(filepath.Join(local, "slot1.sav")); !sameTime(fi.ModTime(), at) {
		t.Error("modification time not kept")
	}
	if es, _ := os.ReadDir(filepath.Join(local, ".stversions")); len(es) != 0 {
		t.Errorf("staging left behind: %v", es)
	}

	// The backup changed after the list was read.
	write(t, filepath.Join(target, "game", "slot1.sav"), "newest")
	if _, err := TakeFiles(context.Background(), target, f, files, nil); err == nil {
		t.Error("took a file that no longer matches the list")
	}
}

// The files a take replaces go into a pinned restore point of their own, and
// restoring to it brings them back.
func TestTakeFilesKeepsReplaced(t *testing.T) {
	target, f, files, _ := takeFixture(t)
	if n, err := TakeFiles(context.Background(), target, f, files, nil); err != nil || n != 2 {
		t.Fatalf("take = %d, %v", n, err)
	}
	pts := Points(target, f.ID)
	if len(pts) != 1 {
		t.Fatalf("want 1 restore point, got %v", pts)
	}
	stamp := pts[0].Format(stampFmt)
	root := filepath.Join(target, VersionsDir, f.ID)
	if !marks(root, PinnedDir)[stamp] || !marks(root, AsideDir)[stamp] {
		t.Error("the point of the replaced files isn't pinned and aside")
	}
	if got := read(t, filepath.Join(root, stamp, "slot1.sav")); got != "older" {
		t.Errorf("kept slot1 = %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, stamp, "same.sav")); err == nil {
		t.Error("a file the take left alone was kept")
	}
	if _, err := Restore(target, f, pts[0]); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(f.Path, "slot1.sav")); got != "older" {
		t.Errorf("restored slot1 = %q", got)
	}
}

// A save written here after the caller looked (gatherPull) stops the take
// before anything changes: the precondition runs under the lock, after the
// replaced files are kept and before the first swap, so no backup can run
// in between either.
func TestTakeFilesPrecondition(t *testing.T) {
	target, f, files, _ := takeFixture(t)
	live, err := ReadLive(f)
	if err != nil {
		t.Fatal(err)
	}
	slot1 := filepath.Join(f.Path, "slot1.sav")
	calls := 0
	_, err = TakeFiles(context.Background(), target, f, files, func() error {
		calls++
		if _, err := Run(context.Background(), []Folder{f}, Options{Target: target}); err != ErrBusy {
			t.Errorf("a backup ran between keeping and swapping: %v", err)
		}
		pts := Points(target, f.ID)
		if len(pts) != 1 || read(t, filepath.Join(target, VersionsDir, f.ID, pts[0].Format(stampFmt), "slot1.sav")) != "older" {
			t.Errorf("replaced files not kept before the swap: %v", pts)
		}
		write(t, slot1, "played meanwhile")
		return live.Same(f)
	})
	if err == nil || calls != 1 {
		t.Fatalf("take went ahead over a changed save (%d call(s))", calls)
	}
	if got := read(t, slot1); got != "played meanwhile" {
		t.Errorf("slot1 = %q", got)
	}
	if _, err := os.Stat(filepath.Join(f.Path, "Sub", "slot2.sav")); err == nil {
		t.Error("slot2 written by an aborted take")
	}
	if es, _ := os.ReadDir(filepath.Join(f.Path, ".stversions")); len(es) != 0 {
		t.Errorf("staging left behind: %v", es)
	}
	root := filepath.Join(target, VersionsDir, f.ID)
	if pts := Points(target, f.ID); len(pts) != 0 || len(marks(root, PinnedDir)) != 0 || len(marks(root, AsideDir)) != 0 {
		t.Errorf("an aborted take left a restore point: %v", pts)
	}
}

// A save written after the precondition passed is what the take replaces,
// so that version is the one its restore point gives back.
func TestTakeFilesKeepsLateWrite(t *testing.T) {
	target, f, files, _ := takeFixture(t)
	if _, err := TakeFiles(context.Background(), target, f, files, func() error {
		write(t, filepath.Join(f.Path, "slot1.sav"), "written just before the swap")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	pts := Points(target, f.ID)
	if len(pts) != 1 {
		t.Fatalf("want 1 restore point, got %v", pts)
	}
	if _, err := Restore(target, f, pts[0]); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(f.Path, "slot1.sav")); got != "written just before the swap" {
		t.Errorf("restored slot1 = %q", got)
	}
}

func TestLiveSame(t *testing.T) {
	_, f, _, _ := takeFixture(t)
	live, err := ReadLive(f)
	if err != nil || len(live) != 3 {
		t.Fatalf("live = %v, %v", live, err)
	}
	if err := live.Same(f); err != nil {
		t.Errorf("unchanged folder: %v", err)
	}
	write(t, filepath.Join(f.Path, "Sub", "new.sav"), "new")
	if live.Same(f) == nil {
		t.Error("a new file went unnoticed")
	}
	os.Remove(filepath.Join(f.Path, "Sub", "new.sav"))
	os.Remove(filepath.Join(f.Path, "only-here.sav"))
	if live.Same(f) == nil {
		t.Error("a removed file went unnoticed")
	}
	if l, err := ReadLive(Folder{ID: "x", Path: filepath.Join(f.Path, "not-there")}); err != nil || l == nil || len(l) != 0 {
		t.Errorf("missing folder = %v, %v", l, err)
	}
	if Live(nil).Same(f) == nil {
		t.Error("an unread folder counts as unchanged")
	}
}
