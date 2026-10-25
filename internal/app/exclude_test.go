package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ApolloF/syncer/internal/backup"
	"github.com/ApolloF/syncer/internal/mods"
	"github.com/ApolloF/syncer/internal/paths"
	"github.com/ApolloF/syncer/internal/store"
)

// Whatever another PC puts into a save folder, programs and scripts in it
// neither sync nor get backed up; a mod folder keeps syncing its own.
func TestSaveFoldersIgnorePrograms(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	if _, err := store.UpdateSettings(func(s *store.Settings) {
		s.Mods["skyrim-mods"] = store.ModFolder{Kind: mods.KindStaging}
	}); err != nil {
		t.Fatal(err)
	}
	save, mod := t.TempDir(), t.TempDir()
	writeExclusions("stardew-valley", save)
	writeExclusions("skyrim-mods", mod)

	lines := readIgnores(filepath.Join(save, ".stignore"))
	for _, p := range []string{"*.exe", "*.dll", "*.scr", "*.com", "*.msi", "*.bat", "*.cmd", "*.ps1", "*.psm1",
		"*.vbs", "*.vbe", "*.wsf", "*.wsh", "*.lnk", "*.hta", "*.cpl", "*.pif", "*.jse", "*.url", "*.appref-ms", "*.scf",
		"*.wsc", "*.psd1",
		"*.library-ms", "*.searchConnector-ms", "*.jar", "*.msc", "*.msp", "*.application", "*.settingcontent-ms"} {
		if !slices.Contains(lines, p) {
			t.Errorf("save folder's .stignore lacks %q: %q", p, lines)
		}
	}
	if slices.Contains(readIgnores(filepath.Join(mod, ".stignore")), "*.dll") {
		t.Error("a mod folder's programs aren't synced")
	}

	// The backup skips them too, but not in a mod folder (see backup.Folder.Mod).
	if _, err := store.UpdateSettings(func(s *store.Settings) {
		s.SyncDisabled = true // only the backup-only folders below
		s.BackupOnly["stardew-valley"] = store.LocalFolder{ID: "stardew-valley", Path: save}
		s.BackupOnly["skyrim-mods"] = store.LocalFolder{ID: "skyrim-mods", Path: mod}
	}); err != nil {
		t.Fatal(err)
	}
	fs, err := backupFolders()
	if err != nil {
		t.Fatal(err)
	}
	isModFolder := map[string]bool{}
	for _, f := range fs {
		isModFolder[f.ID] = f.Mod
	}
	if len(isModFolder) != 2 || isModFolder["stardew-valley"] || !isModFolder["skyrim-mods"] {
		t.Errorf("backed up as mod folders: %v, want only skyrim-mods", isModFolder)
	}
}

// Syncthing uses the first line that matches: Syncer's block goes first, so
// the user's lines (or a file they include, which may come from another PC)
// can't sync programs again.
func TestUserLinesDontBringProgramsBack(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	save := t.TempDir()
	ig := filepath.Join(save, ".stignore")
	old := "!*.dll\n!Saves/**\n#include more.stignore\n*\n" + backup.IgnoreBegin + "\nsteam_autocloud.vdf\n" + backup.IgnoreEnd + "\n"
	if err := os.WriteFile(ig, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	writeExclusions("stardew-valley", save)
	lines := readIgnores(ig)
	if len(lines) == 0 || lines[0] != backup.IgnoreBegin {
		t.Fatalf("Syncer's block isn't first: %q", lines)
	}
	if n := slices.Index(lines, backup.IgnoreBegin); slices.Index(lines[n+1:], backup.IgnoreBegin) >= 0 {
		t.Errorf("two blocks: %q", lines)
	}
	m := backup.NewMatcher(lines) // matches like Syncthing: first line wins
	for rel, want := range map[string]bool{`Saves\mod.dll`: true, "run.ps1": true, `Saves\Farm_1`: false} {
		if got := m.Ignored(rel); got != want {
			t.Errorf("%s ignored = %v, want %v (lines %q)", rel, got, want, lines)
		}
	}
}
