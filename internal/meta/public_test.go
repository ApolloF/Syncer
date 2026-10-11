//go:build !personal

package meta

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// pubFakeST answers the few calls Reconcile and Available make, with the
// given folders, and remembers the changes asked for.
type pubFakeST struct {
	mu      sync.Mutex
	folders []map[string]any
	changes []string
}

func newPubFakeST(t *testing.T, folders []map[string]any) (*pubFakeST, *syncthing.Client) {
	t.Helper()
	f := &pubFakeST{folders: folders}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		call := r.Method + " " + r.URL.Path
		reply := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		switch {
		case call == "GET /rest/system/status":
			reply(map[string]any{"myID": "ME"})
		case call == "GET /rest/config/devices":
			reply([]map[string]any{{"deviceID": "ME"}, {"deviceID": "OTHER"}})
		case call == "GET /rest/config/folders":
			reply(f.folders)
		case r.Method != http.MethodGet:
			f.changes = append(f.changes, call)
		default:
			reply(map[string]any{})
		}
	}))
	t.Cleanup(srv.Close)
	c, err := syncthing.NewAt(strings.TrimPrefix(srv.URL, "http://"), "key")
	if err != nil {
		t.Fatal(err)
	}
	return f, c
}

func TestPublicBuildLeavesModFoldersAloneInReconcile(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	defer paths.SetRootForTest(paths.Local, t.TempDir())()
	MarkSyncerPC("OTHER")
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateSettings(func(s *store.Settings) {
		s.Mods["sky-mods"] = store.ModFolder{Kind: mods.KindStaging, Game: "skyrimse"}
	}); err != nil {
		t.Fatal(err)
	}
	me := []map[string]any{{"deviceID": "ME"}}
	f, c := newPubFakeST(t, []map[string]any{
		{"id": FolderID, "label": "Syncer", "path": Dir(), "type": "sendreceive", "devices": append(me, map[string]any{"deviceID": "OTHER"})},
		{"id": "sky-mods", "label": "Skyrim (Vortex mods)", "path": filepath.Join(t.TempDir(), "mods"), "type": "sendreceive", "devices": me},
	})
	if _, err := Reconcile(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	for _, ch := range f.changes {
		if strings.Contains(ch, "sky-mods") {
			t.Errorf("a mod folder was changed: %s", ch)
		}
	}
	b, err := os.ReadFile(filepath.Join(Dir(), "ME.json"))
	if err != nil {
		t.Fatal(err)
	}
	var df DeviceFile
	if err := json.Unmarshal(b, &df); err != nil || df.Device != "ME" || df.Mods != nil {
		t.Errorf("own file = %s (%v): mod settings must not be published", b, err)
	}
}

func TestPublicBuildDoesNotOfferModOrLauncherFolders(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	defer paths.SetRootForTest(paths.Local, t.TempDir())()
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	peer := DeviceFile{Device: "OTHER", Name: "Laptop", Folders: []SharedFolder{
		{ID: "sky-mods", Label: "Skyrim (Vortex mods)", Root: "vortex:skyrimse", Rel: "staging", Kind: mods.KindStaging, ModGame: "skyrimse"},
		{ID: "sea", Label: "Seaglass", Root: paths.Roaming, Rel: "Seaglass/profile", Launcher: "Seaglass"},
	}}
	if err := store.WriteJSON(filepath.Join(Dir(), "OTHER.json"), peer); err != nil {
		t.Fatal(err)
	}
	_, c := newPubFakeST(t, nil)
	av, err := Available(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if len(av) != 0 {
		t.Errorf("offered %+v", av)
	}
}
