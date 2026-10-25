package meta

import (
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ApolloF/syncer/internal/discover"
	"github.com/ApolloF/syncer/internal/paths"
	"github.com/ApolloF/syncer/internal/store"
	"github.com/ApolloF/syncer/internal/syncthing"
)

// Another PC publishes folders under game names. Only a game's own save
// folder (as the game database lists it) within the auto-add limit is added
// on its own; a folder labelled after a game but somewhere else, or a big
// one, waits for the user's OK.
func TestPeerFoldersOutsideKnownSavesWait(t *testing.T) {
	base := longTempDir(t)
	roaming := filepath.Join(base, `AppData\Roaming`)
	for root, dir := range map[string]string{paths.Home: base, paths.Roaming: roaming,
		paths.Local: filepath.Join(base, `AppData\Local`), paths.LocalLow: filepath.Join(base, `AppData\LocalLow`),
		paths.Documents: filepath.Join(base, "Documents"), paths.SavedGames: filepath.Join(base, "Saved Games"), paths.Ubisoft: ""} {
		t.Cleanup(paths.SetRootForTest(root, dir))
	}
	t.Cleanup(discover.SetManifestForTest([]discover.Entry{
		{Name: "Stardew Valley", Paths: []string{"<winAppData>/StardewValley/Saves"}},
		{Name: "Terraria", Paths: []string{"<winDocuments>/My Games/Terraria/Players"}},
	}))
	oldClassify := classify
	classify = func(string, string) discover.Class { return discover.Class{} } // no Steam on the test PC
	t.Cleanup(func() { classify = oldClassify })
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteJSON(filepath.Join(Dir(), "DESKTOP.json"), DeviceFile{Device: "DESKTOP", Name: "SYNCER-B",
		Folders: []SharedFolder{
			{ID: "stardew-valley", Label: "Stardew Valley", Root: paths.Roaming, Rel: "StardewValley/Saves"},
			{ID: "powershell", Label: "Stardew Valley", Root: paths.Documents, Rel: "WindowsPowerShell"},
			{ID: "terraria", Label: "Terraria", Root: paths.Documents, Rel: "My Games/Terraria/Players", SizeGB: 2},
		}}); err != nil {
		t.Fatal(err)
	}
	fake := &fakeSyncthing{
		devices: map[string]syncthing.Device{"ME": {DeviceID: "ME"}, "DESKTOP": {DeviceID: "DESKTOP"}},
		folders: map[string]syncthing.Folder{FolderID: {ID: FolderID, Path: Dir(), Devices: devs("ME", "DESKTOP")}},
		remote:  map[string]string{FolderID + "/DESKTOP": syncthing.RemoteValid},
	}
	srv := httptest.NewServer(http.HandlerFunc(fake.serve))
	defer srv.Close()
	c, err := syncthing.NewAt(strings.TrimPrefix(srv.URL, "http://"), "key")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	rep, err := Reconcile(ctx, c)
	if err != nil {
		t.Fatal(err)
	}

	fake.mu.Lock()
	ids := slices.Sorted(maps.Keys(fake.folders))
	stardew := fake.folders["stardew-valley"]
	fake.mu.Unlock()
	if want := []string{"stardew-valley", FolderID}; !slices.Equal(ids, want) {
		t.Errorf("folders = %v, want %v (added %v)", ids, want, rep.Added)
	}
	if want := filepath.Join(roaming, `StardewValley\Saves`); !strings.EqualFold(stardew.Path, want) {
		t.Errorf("Stardew Valley added at %q, want %q", stardew.Path, want)
	}

	wantWaiting := []store.WaitingFolder{
		{ID: "powershell", Label: "Stardew Valley", From: "SYNCER-B"},
		{ID: "terraria", Label: "Terraria", From: "SYNCER-B"},
	}
	if got := store.LoadState().Waiting; !slices.Equal(got, wantWaiting) {
		t.Errorf("waiting = %+v, want %+v", got, wantWaiting)
	}
	av, err := Available(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	reasons := map[string]string{}
	for _, a := range av {
		reasons[a.ID] = a.Reason
	}
	if reasons["powershell"] != SkipAsk || reasons["terraria"] != SkipAsk || len(reasons) != 2 {
		t.Errorf("offered = %v, want powershell and terraria waiting for an OK", reasons)
	}
	for _, a := range av {
		if want := a.ID == "powershell"; a.UnknownPlace != want {
			t.Errorf("%s: unknown place = %v, want %v", a.ID, a.UnknownPlace, want)
		}
	}
	DoneWaiting("powershell") // synced by the user
	if got := store.LoadState().Waiting; !slices.Equal(got, wantWaiting[1:]) {
		t.Errorf("waiting after syncing one = %+v, want %+v", got, wantWaiting[1:])
	}

	// Raising the limit lets the big one in; nothing waits for the folder
	// once it syncs here.
	if _, err := store.UpdateSettings(func(s *store.Settings) { s.AutoAddMaxGB = 5 }); err != nil {
		t.Fatal(err)
	}
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}
	if got := store.LoadState().Waiting; !slices.Equal(got, wantWaiting[:1]) {
		t.Errorf("waiting after raising the limit = %+v, want %+v", got, wantWaiting[:1])
	}
}

// longTempDir is t.TempDir() with its long names: the temp folder can be
// under a short-named profile (RUNNER~1 on CI), and known save
// locations refuse short names.
func longTempDir(t *testing.T) string {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
