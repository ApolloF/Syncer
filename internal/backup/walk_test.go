package backup

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeEntry struct {
	name string
	mode fs.FileMode
}

func (e fakeEntry) Name() string               { return e.name }
func (e fakeEntry) IsDir() bool                { return e.mode.IsDir() }
func (e fakeEntry) Type() fs.FileMode          { return e.mode.Type() }
func (e fakeEntry) Info() (fs.FileInfo, error) { return nil, errors.New("not available") }

// walkFolder returns a folder for the walk tests and a func that runs a backup
// of it.
func walkFolder(t *testing.T) (Folder, string, func() (resOK bool, errs []string)) {
	t.Helper()
	src, target := t.TempDir(), t.TempDir()
	id := "walk-" + strings.ReplaceAll(t.Name(), "/", "-") + time.Now().Format("150405.000000")
	t.Cleanup(func() { os.Remove(indexPath(id)) })
	f := Folder{ID: id, Label: "Walk", Path: src}
	return f, target, func() (bool, []string) {
		t.Helper()
		res, err := Run(context.Background(), []Folder{f}, Options{Target: target, KeepDays: 30})
		if err != nil {
			t.Fatal(err)
		}
		return res.OK, res.Errors
	}
}

func failReadDir(t *testing.T, bad string, mutate func([]os.DirEntry) []os.DirEntry) {
	t.Helper()
	old := readDir
	t.Cleanup(func() { readDir = old })
	readDir = func(name string) ([]os.DirEntry, error) {
		if mutate == nil && filepath.Clean(name) == filepath.Clean(bad) {
			return nil, errors.New("access denied")
		}
		es, err := old(name)
		if mutate != nil && filepath.Clean(name) == filepath.Clean(bad) {
			es = mutate(es)
		}
		return es, err
	}
}

func TestRunBacksUpNegatedFolder(t *testing.T) {
	f, target, run := walkFolder(t)
	write(t, filepath.Join(f.Path, ".stignore"), "!Saves/**\n*\n")
	write(t, filepath.Join(f.Path, "Saves", "slot1.sav"), "s")
	write(t, filepath.Join(f.Path, "Saves", "deep", "slot2.sav"), "d")
	write(t, filepath.Join(f.Path, "Logs", "a.log"), "l")
	write(t, filepath.Join(f.Path, "root.txt"), "r")

	if ok, errs := run(); !ok {
		t.Fatalf("errors: %v", errs)
	}
	if got := read(t, filepath.Join(target, f.ID, "Saves", "slot1.sav")); got != "s" {
		t.Errorf("slot1.sav = %q", got)
	}
	if got := read(t, filepath.Join(target, f.ID, "Saves", "deep", "slot2.sav")); got != "d" {
		t.Errorf("slot2.sav = %q", got)
	}
	for _, p := range []string{"Logs/a.log", "root.txt"} {
		if _, err := os.Stat(filepath.Join(target, f.ID, filepath.FromSlash(p))); err == nil {
			t.Errorf("%s was backed up", p)
		}
	}
}

func TestRunUnlistableRootFails(t *testing.T) {
	f, target, run := walkFolder(t)
	write(t, filepath.Join(f.Path, "slot1.sav"), "s")
	failReadDir(t, f.Path, nil)

	ok, errs := run()
	if ok || len(errs) == 0 {
		t.Fatalf("OK=%v errors=%v, want a failed run", ok, errs)
	}
	if _, err := os.Stat(filesPath(target, f.ID, hostKey)); err == nil {
		t.Error("file list written")
	}
	if _, err := os.Stat(infoPath(target, f.ID, hostKey)); err == nil {
		t.Error("info file written")
	}
}

func TestRunUnreadableSubfolderRetiresNothing(t *testing.T) {
	f, target, run := walkFolder(t)
	write(t, filepath.Join(f.Path, "a.sav"), "a")
	write(t, filepath.Join(f.Path, "locked", "b.sav"), "b")
	write(t, filepath.Join(f.Path, "gone.sav"), "g")
	if ok, errs := run(); !ok {
		t.Fatalf("first run: %v", errs)
	}

	if err := os.Remove(filepath.Join(f.Path, "gone.sav")); err != nil {
		t.Fatal(err)
	}
	failReadDir(t, filepath.Join(f.Path, "locked"), nil)
	ok, errs := run()
	if ok || len(errs) == 0 {
		t.Fatalf("OK=%v errors=%v, want a failed run", ok, errs)
	}
	for _, p := range []string{"gone.sav", filepath.Join("locked", "b.sav")} {
		if got := read(t, filepath.Join(target, f.ID, p)); got == "" {
			t.Errorf("%s retired", p)
		}
	}
	if _, err := os.Stat(filepath.Join(target, VersionsDir, f.ID)); err == nil {
		t.Error("a restore point was made")
	}
}

func TestRunOnlineOnlyFileFails(t *testing.T) {
	f, target, run := walkFolder(t)
	write(t, filepath.Join(f.Path, "a.sav"), "a")
	write(t, filepath.Join(f.Path, "cloud.sav"), "c")
	if ok, errs := run(); !ok {
		t.Fatalf("first run: %v", errs)
	}

	failReadDir(t, f.Path, func(es []os.DirEntry) []os.DirEntry {
		out := []os.DirEntry{fakeEntry{"cloud.sav", fs.ModeIrregular}}
		for _, e := range es {
			if e.Name() != "cloud.sav" {
				out = append(out, e)
			}
		}
		return out
	})
	ok, errs := run()
	if ok || len(errs) != 1 || !strings.Contains(errs[0], "available offline") || !strings.Contains(errs[0], "cloud.sav") {
		t.Fatalf("OK=%v errors=%v, want the offline message for cloud.sav", ok, errs)
	}
	if got := read(t, filepath.Join(target, f.ID, "cloud.sav")); got != "c" {
		t.Errorf("backup copy = %q, want it kept", got)
	}
	if _, err := os.Stat(filepath.Join(target, VersionsDir, f.ID)); err == nil {
		t.Error("a restore point was made")
	}
}
