package accounts

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestApplySweepsOldTrash(t *testing.T) {
	w := newWorld(t)
	at := time.Date(2026, 10, 25, 12, 0, 0, 0, time.Local)
	oldNow := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = oldNow })

	trash := filepath.Join(localRoot(), VaultName, ".trash")
	entry := func(age time.Duration) string {
		p := filepath.Join(trash, at.Add(-age).Format(trashStamp), "Stardew")
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "save"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		return filepath.Dir(p)
	}
	old, fresh := entry(8*24*time.Hour), entry(6*24*time.Hour)
	stranger := filepath.Join(trash, "not-a-stamp")
	if err := os.MkdirAll(stranger, 0o755); err != nil {
		t.Fatal(err)
	}

	// Something a link inside an old entry points at must survive.
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "keep"), []byte("k"), 0o644); err != nil {
		t.Fatal(err)
	}
	linked := entry(9 * 24 * time.Hour)
	// A junction needs no privilege, unlike a symlink.
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(linked, "link"), outside).CombinedOutput(); err != nil {
		t.Fatalf("junction: %v: %s", err, out)
	}

	if _, err := Apply(context.Background(), w.env); err != nil {
		t.Fatal(err)
	}
	for p, wantGone := range map[string]bool{old: true, linked: true, fresh: false, stranger: false} {
		if _, err := os.Lstat(p); (err != nil) != wantGone {
			t.Errorf("%s: gone=%v, want %v", filepath.Base(p), err != nil, wantGone)
		}
	}
	if _, err := os.Stat(filepath.Join(outside, "keep")); err != nil {
		t.Error("a link was followed out of the .trash")
	}
}
