package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ApolloF/syncer/internal/backup"
	"github.com/ApolloF/syncer/internal/paths"
	"github.com/ApolloF/syncer/internal/store"
)

func TestMergeFolders(t *testing.T) {
	synced := []backup.Folder{{ID: "a", Label: "A", Path: `C:\Saves\A`}}
	bo := map[string]store.LocalFolder{
		"a":     {ID: "a", Label: "A dup id", Path: `C:\Other`},
		"a2":    {ID: "a2", Label: "A dup path", Path: `c:\saves\a\`},
		"b--pc": {ID: "b--pc", Label: "B", Path: `C:\Saves\B`},
	}
	got := mergeFolders(synced, bo)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b--pc" {
		t.Fatalf("got %+v", got)
	}
}

func TestBackupOnlyID(t *testing.T) {
	id := backupOnlyID("Hollow Knight", nil)
	if !strings.HasPrefix(id, "hollow-knight--") || !paths.ValidID(id) {
		t.Fatalf("id %q", id)
	}
	if id2 := backupOnlyID("Hollow Knight", map[string]bool{id: true}); id2 == id || !paths.ValidID(id2) {
		t.Fatalf("collision not avoided: %q", id2)
	}
	if long := backupOnlyID(strings.Repeat("x", 200), nil); !paths.ValidID(long) {
		t.Fatalf("long label gives invalid id %q", long)
	}
}

func TestToBackupOnlyKeepsBackupChoice(t *testing.T) {
	s := store.Settings{NoBackup: map[string]bool{"off": true}, Ignored: map[string]bool{},
		Dismissed: map[string]bool{}, BackupOnly: map[string]store.LocalFolder{}}
	on := toBackupOnly(&s, "on", "On Game", `C:\Saves\On`)
	off := toBackupOnly(&s, "off", "Off Game", `C:\Saves\Off`)
	if s.NoBackup[on] || !s.NoBackup[off] || s.NoBackup["off"] {
		t.Errorf("backup choice not carried over: %v", s.NoBackup)
	}
	if lf := s.BackupOnly[off]; lf.SyncID != "off" || lf.Path != `C:\Saves\Off` || !s.Ignored["off"] {
		t.Errorf("backup-only record: %+v", lf)
	}
	if !s.Dismissed[dismissKey(`C:\Saves\Off`)] {
		t.Error("auto-add could sync it again")
	}
}

func TestCleanMarkers(t *testing.T) {
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(dir, ".stfolder"), 0o755))
	must(os.WriteFile(filepath.Join(dir, ".stfolder", "syncthing-folder-abc.txt"), nil, 0o644))
	must(os.MkdirAll(filepath.Join(dir, ".stversions"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "save.sav"), []byte("x"), 0o644))
	if v := cleanMarkers(dir); v != "" {
		t.Errorf("empty .stversions reported as kept: %q", v)
	}
	for _, n := range []string{".stfolder", ".stversions"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			t.Errorf("%s not removed", n)
		}
	}
	must(os.MkdirAll(filepath.Join(dir, ".stversions"), 0o755))
	must(os.WriteFile(filepath.Join(dir, ".stversions", "old.sav"), []byte("o"), 0o644))
	if v := cleanMarkers(dir); v == "" {
		t.Error("non-empty .stversions not reported")
	}
	for _, n := range []string{"save.sav", filepath.Join(".stversions", "old.sav")} {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Errorf("%s was deleted", n)
		}
	}
}

func TestNestedIn(t *testing.T) {
	got := nestedIn([]backup.Folder{
		{ID: "roaming-arrowhead", Path: `C:\R\Arrowhead`},
		{ID: "helldivers-2", Path: `C:\R\Arrowhead\Helldivers2\saves`},
		{ID: "other", Path: `C:\R\ArrowheadX`},
	})
	if len(got) != 1 || got["helldivers-2"] != "roaming-arrowhead" {
		t.Fatalf("got %v", got)
	}
}

func TestOverlapsSynced(t *testing.T) {
	synced := []backup.Folder{{ID: "outer", Label: "Arrowhead", Path: `C:\R\Arrowhead`}}
	for _, c := range []struct {
		path, skip string
		bad        bool
	}{
		{`C:\R\Arrowhead\Helldivers2`, "", true}, // inside
		{`C:\R`, "", true},                       // holds it
		{`C:\R\ArrowheadX`, "", false},           // next to it
		{`C:\R\Arrowhead`, "outer", false},       // itself
	} {
		err := overlapsSynced(c.path, c.skip, synced)
		var ce coveredError
		if (err != nil) != c.bad || (err != nil && !errors.As(err, &ce)) {
			t.Errorf("%s: got %v", c.path, err)
		}
	}
}

// A .stignore Syncer wrote goes with the other markers; one with the user's
// own lines stays.
func TestCleanMarkersStignore(t *testing.T) {
	dir := t.TempDir()
	ig := filepath.Join(dir, ".stignore")
	writeExclusions("x", dir)
	if b, err := os.ReadFile(ig); err != nil || !strings.Contains(string(b), "steam_autocloud.vdf") {
		t.Fatalf(".stignore without steam_autocloud.vdf: %q %v", b, err)
	}
	cleanMarkers(dir)
	if _, err := os.Stat(ig); err == nil {
		t.Error("Syncer's own .stignore left behind")
	}
	if err := os.WriteFile(ig, []byte("// mine\nTrainers/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeExclusions("x", dir)
	cleanMarkers(dir)
	if b, err := os.ReadFile(ig); err != nil || !strings.HasPrefix(string(b), "// mine\nTrainers/\n") {
		t.Errorf("user's .stignore changed: %q %v", b, err)
	}
}

func TestForgetBackupReportsOtherPC(t *testing.T) {
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	target := t.TempDir()
	if _, err := store.UpdateSettings(func(s *store.Settings) { s.BackupRoot = target }); err != nil {
		t.Fatal(err)
	}
	id := "shared-game"
	for p, c := range map[string]string{
		filepath.Join(target, id, "a.sav"):                     "a",
		filepath.Join(target, backup.InfoDir, id, "zz-b.json"): `{"id":"shared-game","label":"Shared","host":"SYNCER-B"}`,
	} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	other, err := forgetBackup(id, true)
	if err != nil || other != "SYNCER-B" {
		t.Fatalf("forgetBackup = %q, %v", other, err)
	}
	if _, err := os.Stat(filepath.Join(target, id, "a.sav")); err != nil {
		t.Error("backup deleted while another PC uses it")
	}
	if got := removeMessage(true, other); got != "SYNCER-B still backs it up here, so the backup stays" {
		t.Errorf("message %q", got)
	}
	if removeMessage(true, "") != "Backup deleted" || removeMessage(false, "") != "" {
		t.Error("removeMessage")
	}

	if err := os.Remove(filepath.Join(target, backup.InfoDir, id, "zz-b.json")); err != nil {
		t.Fatal(err)
	}
	if other, err := forgetBackup(id, true); err != nil || other != "" {
		t.Fatalf("forgetBackup = %q, %v", other, err)
	}
	if _, err := os.Stat(filepath.Join(target, id)); err == nil {
		t.Error("backup left although no other PC uses it")
	}
}

func TestKeepHistoryCarriesTheBackupTimeToTheNewID(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	defer paths.SetRootForTest(paths.Local, t.TempDir())()
	at := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	_ = store.UpdateState(func(st *store.State) { st.FolderBackups = map[string]time.Time{"old": at} })
	s := store.Settings{BackupRoot: t.TempDir()}
	if err := keepHistory(context.Background(), s, "old", "old--pc"); err != nil {
		t.Fatal(err)
	}
	if got := store.LoadState().FolderBackups["old--pc"]; !got.Equal(at) {
		t.Errorf("backup time of the new id = %v, want %v", got, at)
	}
	if err := keepHistory(context.Background(), s, "../bad", "x"); err == nil {
		t.Error("a failed copy was not reported")
	}
	if _, ok := store.LoadState().FolderBackups["x"]; ok {
		t.Error("backup time carried over after a failed copy")
	}
}

func TestCheckBackupRoot(t *testing.T) {
	for _, ok := range []string{"", `D:\GameSaveBackup`, `\\nas\share\saves`} {
		if err := checkBackupRoot(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{`GameSaveBackup`, `..\x`, `\Backups`, `D:Backups`} {
		if checkBackupRoot(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestHistoryErrorIsToldApart(t *testing.T) {
	err := fmt.Errorf("x: %w", &historyError{errors.New("disk full")})
	if !isHistoryError(err) || isHistoryError(errors.New("other")) {
		t.Error("history errors not told apart")
	}
}
