//go:build !personal

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ApolloF/syncer/internal/syncthing"

	"github.com/ApolloF/syncer/internal/mods"
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
	applySettingsInput(&s, store.Settings{Theme: "dark", IntervalHours: 3, ModsMaxGB: 50}, stored)
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
	// Choosing a folder, or Google Drive for desktop, takes it from there.
	for name, choose := range map[string]func() error{
		"folder":        func() error { return useBackupFolder(`D:\Backups`) },
		"Drive desktop": (&App{}).UseDriveApp,
	} {
		if _, err := store.UpdateSettings(func(s *store.Settings) { s.BackupBackend = "google" }); err != nil {
			t.Fatal(err)
		}
		if err := choose(); err != nil {
			t.Fatal(err)
		}
		if s := store.LoadSettings(); s.BackupBackend != "" || googleView(s).Retired {
			t.Errorf("%s: backend %q, still retired", name, s.BackupBackend)
		}
	}
}

func TestPublicBuildNotifiesOnceAboutRetiredGoogleBackups(t *testing.T) {
	s := store.Settings{BackupEnabled: true, BackupBackend: "google"}
	var keys []string
	for _, p := range problems(s, store.State{}, nil, nil, nil, nil, time.Now()) {
		keys = append(keys, p.key)
	}
	if !slices.Contains(keys, "once:google-retired") {
		t.Errorf("no notification about the Google sign-in: %v", keys)
	}
	s.BackupBackend = ""
	for _, p := range problems(s, store.State{}, nil, nil, nil, nil, time.Now()) {
		if p.key == "once:google-retired" {
			t.Error("notified although a folder is chosen")
		}
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

// pausingST is a Syncthing with folders that can be paused; it remembers
// every change of "paused" asked for.
type pausingST struct {
	mu      sync.Mutex
	paused  map[string]bool
	refuse  map[string]bool // folders whose change is refused
	changes []string        // "pause id" / "resume id"
}

func newPausingST(t *testing.T, running, paused []string) (*pausingST, *syncthing.Client) {
	t.Helper()
	st := &pausingST{paused: map[string]bool{}, refuse: map[string]bool{}}
	for _, id := range running {
		st.paused[id] = false
	}
	for _, id := range paused {
		st.paused[id] = true
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st.mu.Lock()
		defer st.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/config/folders":
			var out []map[string]any
			for id, p := range st.paused {
				out = append(out, map[string]any{"id": id, "label": id, "path": `D:\` + id, "paused": p})
			}
			_ = json.NewEncoder(w).Encode(out)
		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/rest/config/folders/"):
			id := strings.TrimPrefix(r.URL.Path, "/rest/config/folders/")
			var body struct{ Paused bool }
			_ = json.NewDecoder(r.Body).Decode(&body)
			if st.refuse[id] {
				http.Error(w, "no", http.StatusInternalServerError)
				return
			}
			st.paused[id] = body.Paused
			st.changes = append(st.changes, map[bool]string{true: "pause ", false: "resume "}[body.Paused]+id)
		default:
			_, _ = w.Write([]byte("{}"))
		}
	}))
	t.Cleanup(srv.Close)
	c, err := syncthing.NewAt(strings.TrimPrefix(srv.URL, "http://"), "key")
	if err != nil {
		t.Fatal(err)
	}
	return st, c
}

func freezeSetup(t *testing.T) {
	t.Helper()
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	t.Cleanup(paths.SetRootForTest(paths.Local, t.TempDir()))
	if _, err := store.UpdateSettings(func(s *store.Settings) {
		s.Mods["sky-mods"] = store.ModFolder{Kind: mods.KindStaging}
		s.Mods["sky-load"] = store.ModFolder{Kind: mods.KindProfiles}
		s.Mods["sky-deployed"] = store.ModFolder{Kind: mods.KindDeployed} // a receiver
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPublicBuildFreezesModFolders(t *testing.T) {
	freezeSetup(t)
	st, c := newPausingST(t, []string{"sky-mods", "sky-load", "skyrim"}, []string{"sky-deployed"})
	st.refuse["sky-load"] = true
	if !freezeMods(context.Background(), c) {
		t.Fatal("nothing frozen")
	}
	if got := st.changes; len(got) != 1 || got[0] != "pause sky-mods" {
		t.Errorf("changes = %v, want only sky-mods paused (not the game, not the receiver, and sky-load was refused)", got)
	}
	held := store.LoadState().ModHeld
	if !slices.Equal(held, []string{"sky-mods"}) {
		t.Errorf("ModHeld = %v: a frozen folder is recorded, a refused one is not", held)
	}
	st.changes = nil
	if freezeMods(context.Background(), c) || len(st.changes) != 0 {
		t.Errorf("froze again: %v", st.changes)
	}
}

func TestPublicBuildNeverResumesFrozenModFolders(t *testing.T) {
	freezeSetup(t)
	st, c := newPausingST(t, nil, []string{"sky-mods", "skyrim"})
	store.UpdateState(func(s *store.State) {
		s.ModHeld = []string{"sky-mods"}
		s.GamePaused = []string{"sky-mods", "skyrim"}
		s.GameHeld = time.Now()
	})
	ctx := context.Background()
	if modsTick(ctx, c) {
		t.Error("modsTick changed something")
	}
	if err := holdSyncWith(ctx, c, false); err != nil { // the game exited
		t.Fatal(err)
	}
	if got := st.changes; !slices.Equal(got, []string{"resume skyrim"}) {
		t.Errorf("after the game exited: %v, want only the game's folder resumed", got)
	}
	// A pause that ends hands it to ModHeld too.
	st.changes = nil
	store.UpdateState(func(s *store.State) { s.PausedFolders = []string{"sky-mods"} })
	if err := syncPause(ctx, c); err != nil {
		t.Fatal(err)
	}
	if len(st.changes) != 0 {
		t.Errorf("after the pause: %v", st.changes)
	}
	if held := store.LoadState().ModHeld; !slices.Equal(held, []string{"sky-mods"}) {
		t.Errorf("ModHeld = %v", held)
	}
}

func TestPublicBuildShowsTheFrozenModsNoticeOnce(t *testing.T) {
	freezeSetup(t)
	a := &App{}
	s, st := store.LoadSettings(), store.LoadState()
	if got := activeNotices(s, st); !slices.Equal(got, []string{noticeModsFrozen}) {
		t.Fatalf("notices = %v", got)
	}
	if err := a.DismissNotice(noticeModsFrozen); err != nil {
		t.Fatal(err)
	}
	if got := activeNotices(s, store.LoadState()); len(got) != 0 {
		t.Errorf("shown again: %v", got)
	}
	if err := a.DismissNotice("whatever"); err == nil {
		t.Error("an unknown notice was accepted")
	}
	// Without mod folders there is nothing to tell.
	if got := activeNotices(store.Settings{}, store.State{}); len(got) != 0 {
		t.Errorf("notices without mod folders: %v", got)
	}
}

func TestPublicBuildFoldersLeavesOutModAndLauncherFolders(t *testing.T) {
	freezeSetup(t)
	if _, err := store.UpdateSettings(func(s *store.Settings) {
		s.Launchers["sea"] = "Seaglass"
		for _, id := range []string{"sky-mods", "sea", "skyrim"} {
			s.BackupOnly[id] = store.LocalFolder{ID: id, Label: id, Path: `D:\` + id}
		}
	}); err != nil {
		t.Fatal(err)
	}
	got, err := (&App{}).Folders()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "skyrim" {
		t.Errorf("Folders = %+v, want only skyrim", got)
	}
}
