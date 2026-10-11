package store

import (
	"sync"

	"golang.org/x/sys/windows"
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

func TestUnreadableFileIsLeftAlone(t *testing.T) {
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	if err := os.WriteFile(settingsFile()+".bak", []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// A directory can't be read as a file, like one another process holds.
	if err := os.Mkdir(settingsFile(), 0o755); err != nil {
		t.Fatal(err)
	}
	if s := LoadSettings(); s.Theme == "dark" || s.KeepDays != 30 {
		t.Errorf("want defaults for this call, got %+v", s)
	}
	if fi, err := os.Stat(settingsFile()); err != nil || !fi.IsDir() {
		t.Error("the file was touched")
	}
	if len(corruptCopies(t)) != 0 {
		t.Error("an unreadable file must not be set aside")
	}
	if b, _ := os.ReadFile(settingsFile() + ".bak"); string(b) != `{"theme":"dark"}` {
		t.Errorf(".bak changed: %q", b)
	}
}

func TestTypeChangeIsNotCorruption(t *testing.T) {
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	if err := os.WriteFile(settingsFile(), []byte(`{"theme":"dark","keepDays":"many"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsFile()+".bak", []byte(`{"theme":"light"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if s := LoadSettings(); s.Theme != "dark" {
		t.Errorf("the valid JSON should be used as far as it fits, got %+v", s)
	}
	if len(corruptCopies(t)) != 0 {
		t.Error("valid JSON of another shape was set aside")
	}
}

func TestMissingFileWithBackupIsRecovered(t *testing.T) {
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	if err := os.WriteFile(settingsFile()+".bak", []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if s := LoadSettings(); s.Theme != "dark" {
		t.Errorf("got %+v", s)
	}
}

func TestConcurrentRecoveryAgrees(t *testing.T) {
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	if err := os.WriteFile(settingsFile()+".bak", []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsFile(), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	themes := make([]string, 8)
	for i := range themes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			themes[i] = LoadSettings().Theme
		}()
	}
	wg.Wait()
	for i, th := range themes {
		if th != "dark" {
			t.Errorf("reader %d saw %q, want the recovered settings", i, th)
		}
	}
	if n := len(corruptCopies(t)); n != 1 {
		t.Errorf("%d set-aside copies, want 1", n)
	}
}

func TestUpdateSettingsRecoversWithoutDeadlock(t *testing.T) {
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	if err := os.WriteFile(settingsFile()+".bak", []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsFile(), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan Settings)
	go func() {
		s, _ := UpdateSettings(func(s *Settings) { s.KeepDays = 9 })
		done <- s
	}()
	select {
	case s := <-done:
		if s.Theme != "dark" || s.KeepDays != 9 {
			t.Errorf("got %+v", s)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("UpdateSettings deadlocked on its own lock")
	}
}

func TestRenameRetriesWhenAnotherProcessHoldsTheFile(t *testing.T) {
	old := rename
	t.Cleanup(func() { rename = old })
	calls := 0
	rename = func(from, to string) error {
		calls++
		if calls < 3 {
			return &os.LinkError{Op: "rename", Old: from, New: to, Err: windows.ERROR_SHARING_VIOLATION}
		}
		return old(from, to)
	}
	p := filepath.Join(t.TempDir(), "x.json")
	if err := WriteJSON(p, map[string]int{"v": 1}); err != nil || calls != 3 {
		t.Fatalf("err=%v after %d renames", err, calls)
	}
}

// failReads makes reading the settings or state file fail, as when another
// process holds it, until the returned func is called.
func failReads(t *testing.T) (restore func()) {
	t.Helper()
	old := readFile
	readFile = func(p string) ([]byte, error) {
		if strings.HasSuffix(p, "settings.json") || strings.HasSuffix(p, "state.json") {
			return nil, &os.PathError{Op: "open", Path: p, Err: windows.ERROR_SHARING_VIOLATION}
		}
		return old(p)
	}
	return func() { readFile = old }
}

func TestUpdateSettingsDoesNotSaveWhenTheFileCantBeRead(t *testing.T) {
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	if err := SaveSettings(Settings{Theme: "dark", KeepDays: 7}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(settingsFile())
	restore := failReads(t)
	_, err := UpdateSettings(func(s *Settings) { s.KeepDays = 1 })
	restore()
	if err == nil {
		t.Fatal("want the read error")
	}
	if after, _ := os.ReadFile(settingsFile()); string(after) != string(before) {
		t.Errorf("settings.json changed: %q", after)
	}
	if _, err := os.Stat(settingsFile() + ".bak"); err == nil {
		t.Error("a .bak was written for a save that never happened")
	}
}

func TestUpdateStateDoesNotSaveWhenTheFileCantBeRead(t *testing.T) {
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	UpdateState(func(s *State) { s.PausedFolders = []string{"a"} })
	statePath := filepath.Join(paths.AppDir(), "state.json")
	before, _ := os.ReadFile(statePath)
	restore := failReads(t)
	err := UpdateState(func(s *State) { s.PausedFolders = nil })
	restore()
	if err == nil {
		t.Error("UpdateState reported no failure")
	}
	if after, _ := os.ReadFile(statePath); string(after) != string(before) {
		t.Errorf("state.json changed: %q", after)
	}
	if got := LoadState().PausedFolders; len(got) != 1 {
		t.Errorf("got %v", got)
	}
}

func TestLoadAnswersWithLastGoodValueWhenReadFails(t *testing.T) {
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	if err := SaveSettings(Settings{Theme: "dark", KeepDays: 7}); err != nil {
		t.Fatal(err)
	}
	if s := LoadSettings(); s.KeepDays != 7 { // a good read to remember
		t.Fatalf("got %+v", s)
	}
	defer failReads(t)()
	if s := LoadSettings(); s.Theme != "dark" || s.KeepDays != 7 {
		t.Errorf("want the last good settings, got %+v", s)
	}
}

func TestLoadFallsBackToDefaultsWhenNothingWasEverRead(t *testing.T) {
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	if err := SaveSettings(Settings{Theme: "dark", KeepDays: 7}); err != nil {
		t.Fatal(err)
	}
	defer failReads(t)()
	if s := LoadSettings(); s.Theme != "system" || s.KeepDays != 30 {
		t.Errorf("want defaults, got %+v", s)
	}
}
