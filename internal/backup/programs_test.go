package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Programs and scripts in a save folder aren't backed up, also when its
// .stignore tries to bring them back; a mod folder's are.
func TestBackupSkipsProgramsOutsideModFolders(t *testing.T) {
	target := t.TempDir()
	stamp := time.Now().Format("150405.000000")
	folder := func(name string, f Folder) Folder {
		f.ID, f.Label, f.Path = "programs-"+name+"-"+stamp, name, t.TempDir()
		t.Cleanup(func() { os.Remove(indexPath(f.ID)) })
		write(t, filepath.Join(f.Path, "slot1.sav"), "save")
		write(t, filepath.Join(f.Path, "plugins", "Payload.DLL"), "code")
		write(t, filepath.Join(f.Path, "run.ps1"), "code")
		write(t, filepath.Join(f.Path, "ui.js"), "data")
		return f
	}
	save := folder("save", Folder{})
	write(t, filepath.Join(save.Path, ".stignore"), "!*.dll\n!run.ps1\n")
	mod := folder("mod", Folder{Mod: true})
	solo := folder("solo", Folder{Solo: true}) // backed up only, no .stignore

	res, err := Run(context.Background(), []Folder{save, mod, solo}, Options{Target: target})
	if err != nil || !res.OK {
		t.Fatalf("Run: %v %+v", err, res)
	}
	for _, c := range []struct {
		f        Folder
		programs bool
	}{{save, false}, {mod, true}, {solo, false}} {
		for rel, want := range map[string]bool{"slot1.sav": true, "ui.js": true,
			filepath.Join("plugins", "Payload.DLL"): c.programs, "run.ps1": c.programs} {
			_, err := os.Stat(filepath.Join(target, c.f.ID, rel))
			if got := err == nil; got != want {
				t.Errorf("%s: %s backed up = %v, want %v", c.f.Label, rel, got, want)
			}
		}
	}
}

// A backup made before programs were kept out may still hold one another PC
// put there: restoring it doesn't bring it back into a save folder, and a mod
// folder gets its programs back.
func TestRestoreSkipsProgramsOutsideModFolders(t *testing.T) {
	for _, mod := range []bool{false, true} {
		target := t.TempDir()
		f := Folder{ID: "restore-programs", Label: "Game", Path: t.TempDir(), Mod: mod}
		write(t, filepath.Join(target, f.ID, "slot1.sav"), "save")
		write(t, filepath.Join(target, f.ID, "evil.exe"), "code")
		write(t, filepath.Join(target, f.ID, "plugins", "x.dll"), "code")
		if _, err := Restore(target, f, time.Time{}); err != nil {
			t.Fatal(err)
		}
		for rel, want := range map[string]bool{"slot1.sav": true, "evil.exe": mod, filepath.Join("plugins", "x.dll"): mod} {
			_, err := os.Stat(filepath.Join(f.Path, rel))
			if got := err == nil; got != want {
				t.Errorf("mod %v: %s restored = %v, want %v", mod, rel, got, want)
			}
		}
	}
}
