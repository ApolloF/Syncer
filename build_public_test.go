//go:build !personal

package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ApolloF/syncer/internal/paths"
	"github.com/ApolloF/syncer/internal/store"
)

// The public build keeps the features' methods bound but inert.

func TestPublicBuildRefusesFeatureMethods(t *testing.T) {
	a := &App{}
	for name, err := range map[string]error{
		"AddModFolder":             a.AddModFolder("k"),
		"MakeModSource":            a.MakeModSource("id"),
		"HandOverMods":             a.HandOverMods("id"),
		"ApplyModUpdate":           a.ApplyModUpdate("id", ApplyConfirm{}),
		"RollbackMods":             a.RollbackMods("id", "s"),
		"ReleaseModHold":           a.ReleaseModHold("id"),
		"PushVortexList":           a.PushVortexList("g"),
		"UseModSettingsEverywhere": a.UseModSettingsEverywhere(),
		"SignOutGoogle":            a.SignOutGoogle(),
	} {
		if !errors.Is(err, errNotInBuild) {
			t.Errorf("%s: err = %v, want errNotInBuild", name, err)
		}
	}
	if _, err := a.ModUpdatePreview("id"); !errors.Is(err, errNotInBuild) {
		t.Errorf("ModUpdatePreview: %v", err)
	}
	if _, err := a.RunModAudit("id"); !errors.Is(err, errNotInBuild) {
		t.Errorf("RunModAudit: %v", err)
	}
	if _, err := a.SignInGoogle(); !errors.Is(err, errNotInBuild) {
		t.Errorf("SignInGoogle: %v", err)
	}
	if _, err := a.RepairFolder("id"); !errors.Is(err, errNotInBuild) {
		t.Errorf("RepairFolder: %v", err)
	}
	if _, err := a.launcherData(context.Background(), "seaglass", `C:\x`); !errors.Is(err, errNotInBuild) {
		t.Errorf("launcherData: %v", err)
	}
	if a.ScanMods(false) != nil || a.VortexShares() != nil || a.ModSnapshots("id") != nil || a.ModAudit("id") != nil || a.ModSettingsDiffer() != nil {
		t.Error("a view of mods wasn't empty")
	}
}

func TestPublicBuildDoesNotRunMods(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	s := store.Settings{FindMods: true, AutoAddMods: true, Mods: map[string]store.ModFolder{"sky-mods": {Kind: "mods"}}}
	// A nil client would panic if either went on to talk to Syncthing.
	if modsTick(context.Background(), nil) {
		t.Error("modsTick did something")
	}
	if got := autoAddMods(context.Background(), nil, s); got != nil {
		t.Errorf("autoAddMods added %v", got)
	}
	if got := modFounds(s); got != nil {
		t.Errorf("modFounds listed %v", got)
	}
}

func TestPublicBuildKeepsModSettingsWhenSaving(t *testing.T) {
	stored := store.Settings{FindMods: true, AutoAddMods: true, SyncDeployedMods: true, ShareVortexMods: true, ModsMaxGB: 20,
		ModsChanged: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	s := stored
	applySettingsInput(&s, store.Settings{Theme: "dark", IntervalHours: 3}, stored)
	if s.Theme != "dark" || s.IntervalHours != 3 {
		t.Errorf("the settings the build has weren't saved: %+v", s)
	}
	if s.FindMods != stored.FindMods || s.AutoAddMods != stored.AutoAddMods || s.SyncDeployedMods != stored.SyncDeployedMods ||
		s.ShareVortexMods != stored.ShareVortexMods || s.ModsMaxGB != stored.ModsMaxGB || !s.ModsChanged.Equal(stored.ModsChanged) {
		t.Errorf("mod settings changed: %+v", s)
	}
}

func TestPublicBuildBackupWaitsForAFolderAfterGoogleSignIn(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	defer paths.SetRootForTest(paths.Local, t.TempDir())()
	s := store.Settings{BackupBackend: "google"}
	if _, ok := backupTarget(s); ok {
		t.Error("backups have a target although the sign-in is gone")
	}
	if v := googleView(s); !v.Retired || v.Available {
		t.Errorf("googleView = %+v", v)
	}
	if err := googleSync(context.Background(), 0); err != nil {
		t.Errorf("googleSync: %v", err)
	}
	if _, err := store.UpdateSettings(func(s *store.Settings) { s.BackupBackend = "google" }); err != nil {
		t.Fatal(err)
	}
	if _, err := runBackup(context.Background(), nil, nil); !errors.Is(err, errGoogleRetired) {
		t.Errorf("runBackup err = %v, want errGoogleRetired", err)
	}
	if store.LoadState().LastBackup != nil {
		t.Error("a failed backup was recorded: it is just not set up")
	}
	if got := store.LoadSettings().BackupBackend; got != "google" {
		t.Errorf("backend = %q: the stored choice must stay for a build that has the sign-in", got)
	}
	// Choosing a folder takes it from there.
	if v := googleView(store.Settings{BackupBackend: ""}); v.Retired {
		t.Error("retired although a folder is chosen")
	}
}

func TestPublicBuildDoesNotServeTheLauncherAPI(t *testing.T) {
	done := make(chan struct{})
	go func() {
		serveAPI(context.Background(), &App{}) // would listen until the context ends
		runAPI()                                // would serve for a minute
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the launcher API runs in the public build")
	}
	if repairLaunchers(context.Background(), nil) {
		t.Error("repairLaunchers did something")
	}
}

func TestPublicBuildHidesFolderOfFeaturesItLacks(t *testing.T) {
	s := store.Settings{Mods: map[string]store.ModFolder{"sky-mods": {}}, Launchers: map[string]string{"sea": "Seaglass"}}
	for id, want := range map[string]bool{"sky-mods": true, "sea": true, "skyrim": false} {
		if got := hiddenFolder(s, id); got != want {
			t.Errorf("hiddenFolder(%q) = %v, want %v", id, got, want)
		}
	}
	if f := buildFeatures(); f.Mods || f.GoogleSignIn || f.Launchers || f.Personal {
		t.Errorf("features = %+v", f)
	}
}
