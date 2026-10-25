package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ApolloF/syncer/internal/backup"
	"github.com/ApolloF/syncer/internal/features"
	"github.com/ApolloF/syncer/internal/paths"
)

func TestSwapExeWaitsForBackupsAndAccountOperations(t *testing.T) {
	if features.Personal {
		t.Skip("the personal build never swaps the exe")
	}
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	dir := t.TempDir()
	exe, next := filepath.Join(dir, "Syncer.exe"), filepath.Join(dir, "Syncer.exe.new")
	write := func(p, s string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(exe, "old")
	write(next, "new")
	untouched := func(when string) {
		t.Helper()
		if b, _ := os.ReadFile(exe); string(b) != "old" {
			t.Fatalf("%s: exe = %q, was replaced", when, b)
		}
	}

	unlock, err := backup.Lock()
	if err != nil {
		t.Fatal(err)
	}
	if err := swapExe(exe, next); !errors.Is(err, errBusy) {
		t.Fatalf("during a backup: %v, want errBusy", err)
	}
	untouched("during a backup")
	unlock()

	journal := filepath.Join(paths.AppDir(), "accounts-op.json")
	write(journal, `{"kind":"switch"}`)
	if err := swapExe(exe, next); !errors.Is(err, errBusy) {
		t.Fatalf("during an account operation: %v, want errBusy", err)
	}
	untouched("during an account operation")
	if err := os.Remove(journal); err != nil {
		t.Fatal(err)
	}

	if err := swapExe(exe, next); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "new" {
		t.Fatalf("exe = %q after the swap", b)
	}
	if unlock, err := backup.Lock(); err != nil {
		t.Fatalf("the backup lock was kept: %v", err)
	} else {
		unlock()
	}
}
