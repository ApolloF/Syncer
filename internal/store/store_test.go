package store

import (
	"strings"

	"encoding/json"
	"github.com/ApolloF/syncer/internal/paths"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultsSurviveOlderFiles(t *testing.T) {
	s := defaults()
	if err := json.Unmarshal([]byte(`{"theme":"dark","keepDays":7}`), &s); err != nil {
		t.Fatal(err)
	}
	if !s.PauseWhileGaming || !s.Notify || s.NoUpdateCheck || s.Paused() || s.Theme != "dark" || s.KeepDays != 7 || s.BackupOnly == nil {
		t.Errorf("got %+v", s)
	}
}

func TestPaused(t *testing.T) {
	if !(Settings{PausedUntil: time.Now().Add(time.Hour)}).Paused() {
		t.Error("pause in the future should be active")
	}
	if (Settings{PausedUntil: time.Now().Add(-time.Second)}).Paused() {
		t.Error("expired pause should be over")
	}
}

func TestWriteJSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.json")
	for _, v := range []int{1, 2} {
		if err := WriteJSON(p, map[string]int{"v": v}); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]int
	if err := json.Unmarshal(b, &m); err != nil || m["v"] != 2 {
		t.Errorf("got %s, %v", b, err)
	}
	if es, _ := os.ReadDir(dir); len(es) != 1 {
		t.Errorf("temp files left behind: %d entries", len(es))
	}
}

func settingsFile() string { return filepath.Join(paths.AppDir(), "settings.json") }

func corruptCopies(t *testing.T) []string {
	t.Helper()
	m, _ := filepath.Glob(settingsFile() + ".corrupt-*")
	return m
}

func TestLoadMissingFileGivesDefaults(t *testing.T) {
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	if s := LoadSettings(); s.KeepDays != 30 || len(corruptCopies(t)) != 0 {
		t.Errorf("got %+v, corrupt copies %v", s, corruptCopies(t))
	}
}

func TestLoadSetsAsideUnusableFile(t *testing.T) {
	for name, content := range map[string]string{"empty": "", "garbage": "{\"theme\": \"da\x00\x00"} {
		t.Run(name, func(t *testing.T) {
			t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
			if err := os.WriteFile(settingsFile(), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			if s := LoadSettings(); s.KeepDays != 30 || s.Theme == "dark" {
				t.Errorf("no backup: want defaults, got %+v", s)
			}
			cs := corruptCopies(t)
			if len(cs) != 1 {
				t.Fatalf("want the bad file kept aside, got %v", cs)
			}
			if b, _ := os.ReadFile(cs[0]); string(b) != content {
				t.Errorf("the set-aside copy changed: %q", b)
			}
			if _, err := os.Stat(settingsFile()); err == nil {
				t.Error("the unusable file is still in place and would be overwritten")
			}
		})
	}
}

func TestLoadRecoversFromBackup(t *testing.T) {
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	if err := SaveSettings(Settings{Theme: "dark", KeepDays: 7}); err != nil {
		t.Fatal(err)
	}
	if err := SaveSettings(Settings{Theme: "light", KeepDays: 9}); err != nil { // keeps the dark one as .bak
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsFile(), nil, 0o644); err != nil { // a power cut leaves it empty
		t.Fatal(err)
	}
	s := LoadSettings()
	if s.Theme != "dark" || s.KeepDays != 7 {
		t.Errorf("want the last good copy, got %+v", s)
	}
	if len(corruptCopies(t)) != 1 {
		t.Errorf("the empty file should be kept aside: %v", corruptCopies(t))
	}
	if again := LoadSettings(); again.Theme != "dark" {
		t.Errorf("the restored copy was not put back: %+v", again)
	}
}

func TestSaveDoesNotKeepBadFileAsBackup(t *testing.T) {
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	if err := SaveSettings(Settings{Theme: "dark"}); err != nil {
		t.Fatal(err)
	}
	if err := SaveSettings(Settings{Theme: "light"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsFile(), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveSettings(Settings{Theme: "system"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(settingsFile() + ".bak")
	if !json.Valid(b) || !strings.Contains(string(b), "dark") {
		t.Errorf("the .bak must stay the last good file, got %q", b)
	}
}
