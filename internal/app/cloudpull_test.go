package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ApolloF/syncer/internal/backup"
	"github.com/ApolloF/syncer/internal/paths"
)

// pullFixture is another PC's backup of a game holding a newer slot.sav, and
// this PC's folder with an older one.
func pullFixture(t *testing.T) (target string, f backup.Folder, files []backup.FileEntry) {
	t.Helper()
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	target = t.TempDir()
	f = backup.Folder{ID: "game", Label: "Game", Path: t.TempDir()}
	at := time.Now().Add(-time.Hour).Truncate(time.Second)
	src := filepath.Join(target, f.ID, "slot.sav")
	mustWrite(t, src, "newer")
	if err := os.Chtimes(src, at, at); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(f.Path, "slot.sav"), "older")
	return target, f, []backup.FileEntry{{Path: "slot.sav", Size: int64(len("newer")), MTime: at.UnixMilli()}}
}

func mustWrite(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func slot(t *testing.T, f backup.Folder) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.Path, "slot.sav"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// takeNewer checks again, under the backup lock, what gatherPull saw: a game
// started or a save written since stops it.
func TestTakeNewerRechecks(t *testing.T) {
	target, f, files := pullFixture(t)
	live, err := backup.ReadLive(f)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if _, err := takeNewer(ctx, nil, target, f, files, live, func() bool { return true }); err == nil {
		t.Error("took the save while a game runs")
	}
	if slot(t, f) != "older" {
		t.Fatal("the save here changed although the take was refused")
	}

	mustWrite(t, filepath.Join(f.Path, "slot.sav"), "played since")
	if _, err := takeNewer(ctx, nil, target, f, files, live, func() bool { return false }); err == nil {
		t.Error("took the save over one written since gatherPull")
	}
	if slot(t, f) != "played since" {
		t.Fatal("a save written since gatherPull was replaced")
	}
	if pts := backup.Points(target, f.ID); len(pts) != 0 {
		t.Errorf("a refused take left restore points: %v", pts)
	}

	live, _ = backup.ReadLive(f)
	if n, err := takeNewer(ctx, nil, target, f, files, live, func() bool { return false }); err != nil || n != 1 {
		t.Fatalf("take = %d, %v", n, err)
	}
	if slot(t, f) != "newer" {
		t.Error("the newer save wasn't taken")
	}
	pts := backup.Points(target, f.ID)
	if len(pts) != 1 {
		t.Fatalf("want the replaced save as a restore point, got %v", pts)
	}
	if _, err := backup.Restore(target, f, pts[0]); err != nil {
		t.Fatal(err)
	}
	if slot(t, f) != "played since" {
		t.Error("the restore point doesn't give back the replaced save")
	}
}
