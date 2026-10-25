package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ApolloF/syncer/internal/backup"
	"github.com/ApolloF/syncer/internal/paths"
	"github.com/ApolloF/syncer/internal/store"
)

func keeperSetup(t *testing.T) (backup.Folder, string, string) {
	t.Helper()
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	target := t.TempDir()
	if _, err := store.UpdateSettings(func(s *store.Settings) { s.BackupRoot = target }); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "save.dat")
	if err := os.WriteFile(src, []byte("losing"), 0o644); err != nil {
		t.Fatal(err)
	}
	return backup.Folder{ID: "game", Label: "Game", Path: dir}, target, src
}

func TestKeeperWaitsForTheBackupThenFallsBack(t *testing.T) {
	f, _, src := keeperSetup(t)
	unlock, err := backup.Lock()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	old := conflictKeepWait
	conflictKeepWait = 50 * time.Millisecond
	defer func() { conflictKeepWait = old }()

	keep, _ := keeper(f)
	put, err := keep(src, "save.dat", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(put, ".stversions") {
		t.Errorf("put = %q, want .stversions after the wait ran out", put)
	}
}

func TestKeeperUsesTheLockTimeout(t *testing.T) {
	f, target, src := keeperSetup(t)
	oldKeep := keepWithin
	defer func() { keepWithin = oldKeep }()
	var waited time.Duration
	keepWithin = func(wait time.Duration, tg, id, s, rel string, move bool) (string, error) {
		waited = wait
		if tg != target {
			t.Errorf("target = %q, want %q", tg, target)
		}
		return "in backup history", nil
	}
	keep, where := keeper(f)
	put, err := keep(src, "save.dat", true)
	if err != nil || put != "in backup history" || where != "backup history" {
		t.Errorf("put = %q, where = %q, err = %v", put, where, err)
	}
	if waited != time.Minute {
		t.Errorf("waited up to %s, want 1m", waited)
	}

	keepWithin = func(time.Duration, string, string, string, string, bool) (string, error) {
		return "", backup.ErrBusy
	}
	put, err = keep(src, "save.dat", true)
	if err != nil || !strings.Contains(put, ".stversions") {
		t.Errorf("busy: put = %q, err = %v, want .stversions", put, err)
	}
}

func TestKeeperWaitsOnlyOnceForManyFiles(t *testing.T) {
	f, _, src := keeperSetup(t)
	unlock, err := backup.Lock()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	old := conflictKeepWait
	conflictKeepWait = 300 * time.Millisecond
	defer func() { conflictKeepWait = old }()
	// Count the waits rather than time them: a slow runner takes longer
	// per file than the wait itself.
	waits := 0
	keepWithin = func(d time.Duration, t, id, abs, rel string, move bool) (string, error) {
		waits++
		return backup.KeepWithin(d, t, id, abs, rel, move)
	}
	defer func() { keepWithin = backup.KeepWithin }()
	src2 := filepath.Join(filepath.Dir(src), "other.dat")
	if err := os.WriteFile(src2, []byte("losing 2"), 0o644); err != nil {
		t.Fatal(err)
	}

	keep, _ := keeper(f)
	for _, p := range []string{src, src2} {
		put, err := keep(p, filepath.Base(p), true)
		if err != nil || !strings.Contains(put, ".stversions") {
			t.Fatalf("put = %q, err = %v, want .stversions", put, err)
		}
	}
	if waits != 1 {
		t.Errorf("waited %d times for two files, want once", waits)
	}
}
