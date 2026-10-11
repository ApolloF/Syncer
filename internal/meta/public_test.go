//go:build !personal

package meta

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ApolloF/syncer/internal/mods"
	"github.com/ApolloF/syncer/internal/paths"
	"github.com/ApolloF/syncer/internal/store"
	"github.com/ApolloF/syncer/internal/syncthing"
)

// A PC running the public build never takes over a personal PC's mod options
// or adds its mod and launcher folders, and doesn't publish mod options.

func TestPublicBuildIgnoresPeerModSettings(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	peer := DeviceFile{Device: "PEER", Name: "Laptop", Mods: &ModSettings{FindMods: true, ShareVortexMods: true, ModsMaxGB: 50, Changed: t0}}
	if err := store.WriteJSON(filepath.Join(Dir(), "PEER.json"), peer); err != nil {
		t.Fatal(err)
	}
	called := false
	ModSettingsAdopted = func(context.Context, *syncthing.Client, store.Settings, store.Settings) { called = true }
	defer func() { ModSettingsAdopted = nil }()

	before := ModSettingsOf(store.LoadSettings())
	adoptModSettings(context.Background(), nil, "ME")
	if s := store.LoadSettings(); ModSettingsOf(s) != before || called {
		t.Errorf("mod settings were taken over: %+v", ModSettingsOf(s))
	}
	if d := ModSettingsDiffer("ME"); len(d) != 0 {
		t.Errorf("differ = %v", d)
	}
}

func TestPublicBuildDoesNotAdoptModOrLauncherFolders(t *testing.T) {
	s := store.Settings{FindMods: true, AutoAddMods: true}
	mod := SharedFolder{ID: "sky-mods", Label: "Skyrim (Vortex mods)", Root: "vortex:skyrimse", Rel: "staging", Kind: mods.KindStaging, ModGame: "skyrimse"}
	if _, reason := Adoptable(mod, s, func(string) bool { return true }, nil); reason != SkipModsOff {
		t.Errorf("mod folder reason = %q, want %q", reason, SkipModsOff)
	}
	launcher := SharedFolder{ID: "sea", Label: "Seaglass", Root: paths.Roaming, Rel: "Seaglass/profile", Launcher: "Seaglass"}
	if _, reason := Adoptable(launcher, s, func(string) bool { return true }, nil); reason != SkipLaunchersOff {
		t.Errorf("launcher folder reason = %q, want %q", reason, SkipLaunchersOff)
	}
}

func TestPublicBuildAddsNoModFolderPaused(t *testing.T) {
	HoldNewMod = func(string) bool { t.Error("asked to hold a mod folder"); return true }
	defer func() { HoldNewMod = nil }()
	if spec := ModSpec("sky-mods", "Skyrim", `D:\mods`, mods.KindStaging, false); spec.Paused {
		t.Error("mod folder paused")
	}
}
