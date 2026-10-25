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

	"github.com/ApolloF/syncer/internal/paths"
	"github.com/ApolloF/syncer/internal/store"
	"github.com/ApolloF/syncer/internal/syncthing"
)

func trustFake(t *testing.T) (*fakeSyncthing, *syncthing.Client) {
	t.Helper()
	offer := map[string]any{"offeredBy": map[string]any{}}
	for _, d := range []string{"FRIEND", "FRIEND2", "IGNORED"} {
		offer["offeredBy"].(map[string]any)[d] = map[string]any{}
	}
	fake := &fakeSyncthing{
		devices: map[string]syncthing.Device{
			"ME": {DeviceID: "ME"}, "LAPTOP": {DeviceID: "LAPTOP"}, "FRIEND": {DeviceID: "FRIEND"},
			// A friend's PC LAPTOP knows through a notes folder, not a Syncer PC.
			"FRIEND2": {DeviceID: "FRIEND2", IntroducedBy: "LAPTOP"}, "IGNORED": {DeviceID: "IGNORED"},
			// Syncer shared the settings with these: one from an older Syncer,
			// one a trusted PC introduced through the settings folder.
			"UPGRADED": {DeviceID: "UPGRADED"}, "THIRD": {DeviceID: "THIRD", IntroducedBy: "LAPTOP"},
		},
		folders: map[string]syncthing.Folder{
			FolderID: {ID: FolderID, Path: Dir(), Devices: devs("ME", "LAPTOP", "UPGRADED", "THIRD")},
			"game": {ID: "game", Label: "Game", Path: filepath.Join(t.TempDir(), "Game"), Devices: devs("ME", "LAPTOP")},
		},
		remote:  map[string]string{FolderID + "/UPGRADED": syncthing.RemoteValid, FolderID + "/THIRD": syncthing.RemoteValid},
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
	for _, id := range []string{"LAPTOP", "THIRD", "UPGRADED"} {
		if !st.SyncerPCs[id] {
			t.Errorf("%s should be trusted: %v", id, st.SyncerPCs)
		}
	}
	for _, id := range []string{"FRIEND", "FRIEND2", "IGNORED"} {
		if st.SyncerPCs[id] {
			t.Errorf("%s was trusted without being allowed", id)
		}
	}
	waiting, ignored := JoinRequests()
	if !slices.Equal(waiting, []string{"FRIEND", "FRIEND2"}) || !slices.Equal(ignored, []string{"IGNORED"}) {
		t.Errorf("waiting %v, ignored %v", waiting, ignored)
	}
	for _, f := range []string{FolderID, "game"} {
		got := sharedWith(fake, f)
		if slices.Contains(got, "FRIEND") || slices.Contains(got, "FRIEND2") || slices.Contains(got, "IGNORED") {
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
	IgnorePC("FRIEND2")
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
	IgnorePC("FRIEND2")
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

func TestIgnoredEntriesOfUnlinkedDevicesAreDropped(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	MarkSyncerPC("LAPTOP")
	IgnorePC("IGNORED")
	IgnorePC("GONE")
	_, c := trustFake(t)
	if _, err := Reconcile(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if _, ignored := JoinRequests(); !slices.Equal(ignored, []string{"IGNORED"}) {
		t.Errorf("ignored %v", ignored)
	}
}

func TestIntroducersAreTurnedOff(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	MarkSyncerPC("LAPTOP")
	fake, c := trustFake(t)
	fake.mu.Lock()
	fake.devices["LAPTOP"] = syncthing.Device{DeviceID: "LAPTOP", Introducer: true}
	// Asks to join, and an earlier Syncer linked it as an introducer.
	fake.devices["FRIEND"] = syncthing.Device{DeviceID: "FRIEND", Introducer: true}
	// Introduced by LAPTOP, and given the flag with it.
	fake.devices["THIRD"] = syncthing.Device{DeviceID: "THIRD", IntroducedBy: "LAPTOP", Introducer: true}
	// Not Syncer's: the user made it an introducer for a notes folder.
	fake.devices["NAS"] = syncthing.Device{DeviceID: "NAS", Introducer: true}
	fake.folders["notes"] = syncthing.Folder{ID: "notes", Label: "Notes", Path: filepath.Join(t.TempDir(), "Notes"), Devices: devs("ME", "NAS")}
	fake.mu.Unlock()
	ctx := context.Background()
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	for _, id := range []string{"LAPTOP", "FRIEND", "THIRD"} {
		if fake.devices[id].Introducer {
			t.Errorf("%s is still an introducer", id)
		}
	}
	if !fake.devices["NAS"].Introducer {
		t.Error("the introducer of a folder that isn't Syncer's was turned off")
	}
	if len(fake.devices) != 8 {
		t.Errorf("devices changed: %v", slices.Sorted(maps.Keys(fake.devices)))
	}
	// Turned on again (by an older Syncer's habits, say): off again.
	d := fake.devices["LAPTOP"]
	d.Introducer = true
	fake.devices["LAPTOP"] = d
	fake.mu.Unlock()
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.devices["LAPTOP"].Introducer {
		t.Error("a Syncer PC introduces PCs again")
	}
}

func TestAnAllowedPCIntroducesNoOne(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	MarkSyncerPC("LAPTOP")
	fake, c := trustFake(t)
	ctx := context.Background()
	if _, err := Reconcile(ctx, c); err != nil { // the upgrade is done
		t.Fatal(err)
	}
	// It got the flag from an earlier Syncer while it waited.
	fake.mu.Lock()
	fake.devices["FRIEND"] = syncthing.Device{DeviceID: "FRIEND", Introducer: true}
	fake.mu.Unlock()
	if err := AllowPC(ctx, c, syncthing.Device{DeviceID: "FRIEND"}); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.devices["FRIEND"].Introducer {
		t.Error("the allowed PC can still add devices here")
	}
}

func TestTrustRulesApplyEvenWhileTheIntroducerCantBeTurnedOff(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	MarkSyncerPC("LAPTOP")
	fake, c := trustFake(t)
	fake.mu.Lock()
	fake.devices["LAPTOP"] = syncthing.Device{DeviceID: "LAPTOP", Introducer: true}
	fake.failDevicePatch = true
	fake.mu.Unlock()
	ctx := context.Background()
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}
	if !store.LoadState().TrustMigrated {
		t.Error("the old trust rules still apply")
	}
	fake.mu.Lock()
	fake.failDevicePatch = false
	fake.mu.Unlock()
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.devices["LAPTOP"].Introducer {
		t.Error("turning the introducer off wasn't tried again")
	}
}

func TestOldTrustIsTakenOverOnlyOnce(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	MarkSyncerPC("LAPTOP")
	fake, c := trustFake(t)
	ctx := context.Background()
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}
	// A device that syncs the shared settings now (added to them in
	// Syncthing by hand, say) isn't trusted for that any more.
	fake.mu.Lock()
	fake.devices["LATER"] = syncthing.Device{DeviceID: "LATER"}
	f := fake.folders[FolderID]
	f.Devices = append(f.Devices, syncthing.FolderDevice{DeviceID: "LATER"})
	fake.folders[FolderID] = f
	fake.remote[FolderID+"/LATER"] = syncthing.RemoteValid
	fake.mu.Unlock()
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}
	if store.LoadState().SyncerPCs["LATER"] {
		t.Error("a device was trusted for syncing the shared settings")
	}
	if got := sharedWith(fake, FolderID); slices.Contains(got, "LATER") {
		t.Errorf("the shared settings still reach a device that isn't trusted: %v", got)
	}
}

func TestOldTrustByFileSurvivesWhileThePCIsOff(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	fake, c := trustFake(t)
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	// An earlier Syncer took UPGRADED for a Syncer PC by its file; it is off.
	if err := store.WriteJSON(filepath.Join(Dir(), "UPGRADED.json"), DeviceFile{Device: "UPGRADED"}); err != nil {
		t.Fatal(err)
	}
	// FRIEND wrote a file too, but the settings were never shared with it.
	if err := store.WriteJSON(filepath.Join(Dir(), "FRIEND.json"), DeviceFile{Device: "FRIEND"}); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	delete(fake.remote, FolderID+"/UPGRADED")
	fake.mu.Unlock()
	if _, err := Reconcile(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if st := store.LoadState(); !st.SyncerPCs["UPGRADED"] || st.SyncerPCs["FRIEND"] {
		t.Errorf("trusted %v", st.SyncerPCs)
	}
}

func TestAFileOfAnUnknownDeviceGrantsNothing(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	MarkSyncerPC("LAPTOP")
	IgnorePC("IGNORED")
	fake, c := trustFake(t)
	ctx := context.Background()
	if _, err := Reconcile(ctx, c); err != nil { // the one-time takeover
		t.Fatal(err)
	}
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteJSON(filepath.Join(Dir(), "FRIEND.json"), DeviceFile{Device: "FRIEND",
		Folders: []SharedFolder{{ID: "planted", Label: "Planted", Root: paths.Documents, Rel: "Planted"}},
		PCs:     []PCRef{{ID: pcC, Name: "Stranger"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}
	st := store.LoadState()
	if st.SyncerPCs["FRIEND"] || len(st.Introduced) > 0 {
		t.Errorf("trusted %v, introduced %v", st.SyncerPCs, st.Introduced)
	}
	if fs := readOthers("ME"); len(fs) > 0 {
		t.Errorf("an unknown device's folders are read: %+v", fs)
	}
	for _, w := range st.Waiting {
		t.Errorf("an unknown device's folder waits for an OK: %+v", w)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if _, ok := fake.folders["planted"]; ok {
		t.Error("a folder from an unknown device's file was added")
	}
}

func TestAPCALinkedPCIsLinkedToOnlyAsks(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	fake, c := linkedPair(t, pcA, pcB)
	ctx := context.Background()
	if err := store.WriteJSON(filepath.Join(Dir(), pcB+".json"), DeviceFile{Device: pcB, Name: "SYNCER-B",
		PCs: []PCRef{{ID: pcA, Name: "SYNCER-A"}, {ID: pcC, Name: "SYNCER-C"}, {ID: "not an id", Name: "x"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}
	waiting, _ := JoinRequests()
	if !slices.Equal(waiting, []string{pcC}) {
		t.Fatalf("waiting %v", waiting)
	}
	if in := Introductions()[pcC]; in.Name != "SYNCER-C" || in.Via != pcB {
		t.Errorf("introduced as %+v", in)
	}
	if hasDevice(fake, pcC) || store.LoadState().SyncerPCs[pcC] {
		t.Error("an introduced PC was linked without the user's OK")
	}
	if got := readFile(t, pcA).PCs; len(got) != 1 || got[0].ID != pcB || got[0].Name != "Other PC" {
		t.Errorf("A publishes the PCs it is linked to as %+v", got)
	}

	if err := AllowPC(ctx, c, syncthing.Device{DeviceID: pcC, Name: "SYNCER-C"}); err != nil {
		t.Fatal(err)
	}
	if !hasDevice(fake, pcC) || !store.LoadState().SyncerPCs[pcC] {
		t.Error("allowing it didn't link it")
	}
	if waiting, _ := JoinRequests(); len(waiting) != 0 {
		t.Errorf("still waiting: %v", waiting)
	}
	if got := sharedWith(fake, "game"); !slices.Contains(got, pcC) {
		t.Errorf("the allowed PC doesn't get the saves: %v", got)
	}
}

func TestOnlyAPCsOwnFileIntroducesPCs(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	fake, c := linkedPair(t, pcA, pcB)
	if err := store.WriteJSON(filepath.Join(Dir(), pcB+".json"), DeviceFile{Device: pcB, PCs: []PCRef{{ID: pcC}}}); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.writer = map[string]string{pcB + ".json": pcC}
	fake.mu.Unlock()
	if _, err := Reconcile(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if waiting, _ := JoinRequests(); len(waiting) != 0 {
		t.Errorf("a file another PC wrote introduced %v", waiting)
	}
}

func TestAForgedFileOfATrustedPCIsNotRead(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	fake, c := linkedPair(t, pcA, pcB)
	ctx := context.Background()
	game := SharedFolder{ID: "game2", Label: "Game 2", Root: paths.Roaming, Rel: "Game2"}
	if err := store.WriteJSON(filepath.Join(Dir(), pcB+".json"), DeviceFile{Device: pcB, Folders: []SharedFolder{game}}); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.writer = map[string]string{pcB + ".json": pcC} // relayed from another PC
	fake.mu.Unlock()
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}
	if fs := readOthers(pcA); len(fs) > 0 {
		t.Errorf("a file another PC wrote is read: %+v", fs)
	}

	// Syncthing can't tell yet who wrote it (it just arrived): not read
	// this round, but not given up on either.
	fake.mu.Lock()
	fake.writer = map[string]string{pcB + ".json": ""}
	fake.mu.Unlock()
	if err := store.WriteJSON(filepath.Join(Dir(), pcB+".json"), DeviceFile{Device: pcB, Name: "PC B", Folders: []SharedFolder{game}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}
	if fs := readOthers(pcA); len(fs) > 0 {
		t.Errorf("a file whose writer isn't known is read: %+v", fs)
	}
	fake.mu.Lock()
	fake.writer = nil
	fake.mu.Unlock()
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}
	if fs := readOthers(pcA); len(fs) != 1 {
		t.Errorf("B's own file isn't read: %+v", fs)
	}
}

func TestAnIgnoredIntroductionSurvivesARoundWithoutAnswer(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	fake, c := linkedPair(t, pcA, pcB)
	ctx := context.Background()
	if err := store.WriteJSON(filepath.Join(Dir(), pcB+".json"), DeviceFile{Device: pcB, PCs: []PCRef{{ID: pcC, Name: "SYNCER-C"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}
	IgnorePC(pcC)
	// Who wrote B's file can't be told this round (it changed just now).
	fake.mu.Lock()
	fake.writer = map[string]string{pcB + ".json": ""}
	fake.mu.Unlock()
	if err := store.WriteJSON(filepath.Join(Dir(), pcB+".json"), DeviceFile{Device: pcB, Name: "B", PCs: []PCRef{{ID: pcC, Name: "SYNCER-C"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, ignored := JoinRequests(); !slices.Contains(ignored, pcC) {
		t.Errorf("the PC turned down was forgotten: %v", ignored)
	}
	if _, ok := Introductions()[pcC]; !ok {
		t.Error("the introduction was dropped")
	}
}

func TestAPCThatStaysAnIntroducerGetsNoFolders(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	MarkSyncerPC("LAPTOP")
	MarkSyncerPC("FRIEND")
	fake, c := trustFake(t)
	fake.mu.Lock()
	fake.devices["FRIEND"] = syncthing.Device{DeviceID: "FRIEND", Introducer: true}
	fake.failDevicePatch = true
	fake.mu.Unlock()
	if _, err := Reconcile(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{FolderID, "game"} {
		if got := sharedWith(fake, f); slices.Contains(got, "FRIEND") {
			t.Errorf("%s is shared with a PC that can still add devices here: %v", f, got)
		}
	}
}

func TestAnUnsureRoundKeepsAnUnlinkedPCTurnedDown(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	fake, c := linkedPair(t, pcA, pcB)
	ctx := context.Background()
	fake.mu.Lock()
	fake.connected = nil
	fake.devices[pcC] = syncthing.Device{DeviceID: pcC}
	fake.mu.Unlock()
	MarkSyncerPC(pcC)
	if err := store.WriteJSON(filepath.Join(Dir(), pcC+".json"), DeviceFile{Device: pcC, Features: []string{PCsFeature}, PCs: []PCRef{{ID: pcA}, {ID: pcB}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}
	// C's file can't be told for a round, the one the unlink runs in.
	fake.mu.Lock()
	fake.writer = map[string]string{pcC + ".json": ""}
	fake.mu.Unlock()
	if err := store.WriteJSON(filepath.Join(Dir(), pcC+".json"), DeviceFile{Device: pcC, Name: "C", Features: []string{PCsFeature}, PCs: []PCRef{{ID: pcA}, {ID: pcB}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Unlink(ctx, c, pcB); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.writer = nil
	fake.mu.Unlock()
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}
	if waiting, _ := JoinRequests(); slices.Contains(waiting, pcB) {
		t.Errorf("the PC just unlinked is offered again: %v", waiting)
	}
}
