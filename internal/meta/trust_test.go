package meta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ApolloF/syncer/internal/paths"
	"github.com/ApolloF/syncer/internal/store"
	"github.com/ApolloF/syncer/internal/syncthing"
)

func trustFake(t *testing.T) (*fakeSyncthing, *syncthing.Client) {
	t.Helper()
	offer := map[string]any{"offeredBy": map[string]any{}}
	for _, d := range []string{"FRIEND", "INTRODUCED", "IGNORED"} {
		offer["offeredBy"].(map[string]any)[d] = map[string]any{}
	}
	fake := &fakeSyncthing{
		devices: map[string]syncthing.Device{
			"ME": {DeviceID: "ME"}, "LAPTOP": {DeviceID: "LAPTOP"}, "FRIEND": {DeviceID: "FRIEND"},
			"INTRODUCED": {DeviceID: "INTRODUCED", IntroducedBy: "LAPTOP"}, "IGNORED": {DeviceID: "IGNORED"},
			"UPGRADED": {DeviceID: "UPGRADED"},
		},
		folders: map[string]syncthing.Folder{
			"game": {ID: "game", Label: "Game", Path: filepath.Join(t.TempDir(), "Game"), Devices: devs("ME", "LAPTOP")},
		},
		remote:  map[string]string{FolderID + "/UPGRADED": syncthing.RemoteValid},
		pending: map[string]any{FolderID: offer},
	}
	srv := httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(srv.Close)
	c, err := syncthing.NewAt(strings.TrimPrefix(srv.URL, "http://"), "key")
	if err != nil {
		t.Fatal(err)
	}
	return fake, c
}

func sharedWith(fake *fakeSyncthing, folder string) []string {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	var out []string
	for _, d := range fake.folders[folder].Devices {
		out = append(out, d.DeviceID)
	}
	slices.Sort(out)
	return out
}

func TestPairedDeviceOfferingTheSharedSettingsOnlyAsks(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	MarkSyncerPC("LAPTOP")
	IgnorePC("IGNORED")
	fake, c := trustFake(t)

	if _, err := Reconcile(context.Background(), c); err != nil {
		t.Fatal(err)
	}

	st := store.LoadState()
	for _, id := range []string{"LAPTOP", "INTRODUCED", "UPGRADED"} {
		if !st.SyncerPCs[id] {
			t.Errorf("%s should be trusted: %v", id, st.SyncerPCs)
		}
	}
	for _, id := range []string{"FRIEND", "IGNORED"} {
		if st.SyncerPCs[id] {
			t.Errorf("%s was trusted without being allowed", id)
		}
	}
	waiting, ignored := JoinRequests()
	if !slices.Equal(waiting, []string{"FRIEND"}) || !slices.Equal(ignored, []string{"IGNORED"}) {
		t.Errorf("waiting %v, ignored %v", waiting, ignored)
	}
	for _, f := range []string{FolderID, "game"} {
		got := sharedWith(fake, f)
		if slices.Contains(got, "FRIEND") || slices.Contains(got, "IGNORED") {
			t.Errorf("%s is shared with a device nobody allowed: %v", f, got)
		}
	}
	if got := sharedWith(fake, FolderID); !slices.Contains(got, "LAPTOP") {
		t.Errorf("the shared settings aren't offered to the trusted PC: %v", got)
	}
}

func TestAllowedDeviceBecomesASyncerPC(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	MarkSyncerPC("LAPTOP")
	IgnorePC("IGNORED")
	fake, c := trustFake(t)
	ctx := context.Background()
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}

	MarkSyncerPC("FRIEND")
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}

	if waiting, _ := JoinRequests(); len(waiting) != 0 {
		t.Errorf("still waiting: %v", waiting)
	}
	if got := sharedWith(fake, FolderID); !slices.Contains(got, "FRIEND") {
		t.Errorf("the allowed device doesn't get the shared settings: %v", got)
	}
	if got := sharedWith(fake, "game"); !slices.Contains(got, "FRIEND") {
		t.Errorf("the allowed device doesn't get the saves: %v", got)
	}
}

func TestIgnoredDeviceStaysIgnored(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	MarkSyncerPC("LAPTOP")
	IgnorePC("IGNORED")
	_, c := trustFake(t)
	ctx := context.Background()
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}
	IgnorePC("FRIEND")
	for range 2 {
		if _, err := Reconcile(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	waiting, ignored := JoinRequests()
	if len(waiting) != 0 || !slices.Contains(ignored, "FRIEND") || store.LoadState().SyncerPCs["FRIEND"] {
		t.Errorf("waiting %v, ignored %v, trusted %v", waiting, ignored, store.LoadState().SyncerPCs)
	}

	// Linking it on purpose changes that.
	MarkSyncerPC("FRIEND")
	if _, ignored := JoinRequests(); slices.Contains(ignored, "FRIEND") {
		t.Error("a device linked on purpose is still ignored")
	}
}

func TestExistingInstallKeepsItsPCsWithoutAsking(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	// An earlier Syncer recorded no trust file entry for UPGRADED, but it
	// syncs the shared settings, which only Syncer offers its own PCs; LAPTOP
	// was recorded.
	MarkSyncerPC("LAPTOP")
	_, c := trustFake(t)
	if _, err := Reconcile(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if waiting, _ := JoinRequests(); slices.Contains(waiting, "UPGRADED") || slices.Contains(waiting, "LAPTOP") {
		t.Errorf("an existing PC is asked about again: %v", waiting)
	}
	if st := store.LoadState(); !st.SyncerPCs["UPGRADED"] || !st.SyncerPCs["LAPTOP"] {
		t.Errorf("trust lost on upgrade: %v", st.SyncerPCs)
	}
}

func TestForgettingAPCClearsItsRequest(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	IgnorePC("A")
	ForgetSyncerPC("A")
	if _, ignored := JoinRequests(); len(ignored) != 0 {
		t.Errorf("ignored %v", ignored)
	}
}
