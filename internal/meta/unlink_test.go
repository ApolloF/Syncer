package meta

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ApolloF/syncer/internal/paths"
	"github.com/ApolloF/syncer/internal/store"
	"github.com/ApolloF/syncer/internal/syncthing"
)

// pcID is a device id in Syncthing's form, made of one letter.
func pcID(ch string) string {
	g := strings.Repeat(ch, 7)
	return strings.Join([]string{g, g, g, g, g, g, g, g}, "-")
}

var pcA, pcB, pcC = pcID("A"), pcID("B"), pcID("C")

// linkedPair is a Syncthing on PC me, linked with other: the shared
// settings, a game and a folder only those two sync.
func linkedPair(t *testing.T, me, other string) (*fakeSyncthing, *syncthing.Client) {
	t.Helper()
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	MarkSyncerPC(other)
	fake := &fakeSyncthing{
		me:      me,
		devices: map[string]syncthing.Device{me: {DeviceID: me}, other: {DeviceID: other, Name: "Other PC"}},
		folders: map[string]syncthing.Folder{
			FolderID: {ID: FolderID, Path: Dir(), Devices: devs(me, other)},
			"game": {ID: "game", Label: "Game", Path: filepath.Join(t.TempDir(), "Game"), Devices: devs(me, other),
				Versioning: syncthing.Versioning{Type: "staggered"}, MaxConflicts: -1},
		},
		remote:    map[string]string{FolderID + "/" + other: syncthing.RemoteValid, "game/" + other: syncthing.RemoteValid},
		connected: map[string]bool{other: true},
	}
	srv := httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(srv.Close)
	c, err := syncthing.NewAt(strings.TrimPrefix(srv.URL, "http://"), "key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Reconcile(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	return fake, c
}

func folderDevices(fake *fakeSyncthing) map[string][]string {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	m := map[string][]string{}
	for id, f := range fake.folders {
		for _, d := range f.Devices {
			m[id] = append(m[id], d.DeviceID)
		}
	}
	return m
}

func hasDevice(fake *fakeSyncthing, id string) bool {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	_, ok := fake.devices[id]
	return ok
}

func readFile(t *testing.T, device string) DeviceFile {
	t.Helper()
	var df DeviceFile
	b, err := os.ReadFile(filepath.Join(Dir(), device+".json"))
	if err != nil || json.Unmarshal(b, &df) != nil {
		t.Fatalf("%s.json: %v", device[:7], err)
	}
	return df
}

func TestUnlinkTellsTheOtherPCWhichDropsTheLinkToo(t *testing.T) {
	ctx := context.Background()
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	fakeA, cA := linkedPair(t, pcA, pcB)
	if err := store.WriteJSON(filepath.Join(Dir(), pcB+".json"), DeviceFile{Device: pcB, Name: "PC B"}); err != nil {
		t.Fatal(err)
	}
	// What B gets: A's file as it was when A asked Syncthing to send it.
	var notice []byte
	fakeA.onScan = func() { notice, _ = os.ReadFile(filepath.Join(Dir(), pcA+".json")) }

	told, err := Unlink(ctx, cA, pcB)
	if err != nil || !told {
		t.Fatalf("told %v, %v", told, err)
	}
	if hasDevice(fakeA, pcB) {
		t.Error("B is still linked on A")
	}
	for id, ds := range folderDevices(fakeA) {
		if slices.Contains(ds, pcB) {
			t.Errorf("%s is still shared with B", id)
		}
	}
	if _, err := os.Stat(filepath.Join(Dir(), pcB+".json")); !os.IsNotExist(err) {
		t.Errorf("B's file is still in the shared settings: %v", err)
	}
	if st := store.LoadState(); st.SyncerPCs[pcB] {
		t.Errorf("trusted %v", st.SyncerPCs)
	}
	// Still published for a while: a third PC may pass it on to B later.
	if df := readFile(t, pcA); len(df.Unlinked) != 1 || slices.ContainsFunc(df.PCs, func(p PCRef) bool { return p.ID == pcB }) {
		t.Errorf("A doesn't publish the unlink, or still B as linked: %+v", df)
	}
	var sent DeviceFile
	if err := json.Unmarshal(notice, &sent); err != nil || len(sent.Unlinked) != 1 || sent.Unlinked[0].ID != pcB {
		t.Fatalf("A didn't tell B: %s", notice)
	}

	// On B, A's file arrives.
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	fakeB, cB := linkedPair(t, pcB, pcA)
	// B has had A's file since they were linked.
	if err := store.WriteJSON(filepath.Join(Dir(), pcA+".json"), DeviceFile{Device: pcA, Updated: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := Reconcile(ctx, cB); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(Dir(), pcA+".json"), notice, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Reconcile(ctx, cB); err != nil {
		t.Fatal(err)
	}
	if hasDevice(fakeB, pcA) {
		t.Error("B didn't drop the link with A")
	}
	for id, ds := range folderDevices(fakeB) {
		if slices.Contains(ds, pcA) {
			t.Errorf("B still shares %s with A", id)
		}
	}
	if st := store.LoadState(); st.SyncerPCs[pcA] {
		t.Errorf("B still trusts A: %v", st.SyncerPCs)
	}

	// Linked anew, an old copy of that notice changes nothing.
	fakeB.mu.Lock()
	fakeB.devices[pcA] = syncthing.Device{DeviceID: pcA}
	fakeB.mu.Unlock()
	MarkSyncerPC(pcA)
	if err := os.WriteFile(filepath.Join(Dir(), pcA+".json"), notice, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Reconcile(ctx, cB); err != nil {
		t.Fatal(err)
	}
	if !hasDevice(fakeB, pcA) || !store.LoadState().SyncerPCs[pcA] {
		t.Error("an old unlink notice dropped the new link")
	}
}

func TestUnlinkNoticeCountsOnlyInItsOwnPCsFile(t *testing.T) {
	ctx := context.Background()
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	fake, c := linkedPair(t, pcB, pcA)
	notice := DeviceFile{Device: pcA, Name: "PC A", Unlinked: []UnlinkNote{{ID: pcB, At: time.Now().Unix()}}}
	if err := store.WriteJSON(filepath.Join(Dir(), pcA+".json"), notice); err != nil {
		t.Fatal(err)
	}
	// Another PC wrote A's file.
	fake.mu.Lock()
	fake.writer = map[string]string{pcA + ".json": pcC}
	fake.mu.Unlock()
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}
	if !hasDevice(fake, pcA) || !store.LoadState().SyncerPCs[pcA] {
		t.Error("a notice another PC wrote dropped the link")
	}

	// A device that isn't trusted here can't unlink anything either.
	fake.mu.Lock()
	fake.writer = nil
	fake.devices[pcC] = syncthing.Device{DeviceID: pcC}
	fake.mu.Unlock()
	if err := store.WriteJSON(filepath.Join(Dir(), pcC+".json"), DeviceFile{Device: pcC, Unlinked: []UnlinkNote{{ID: pcB, At: time.Now().Unix()}}}); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteJSON(filepath.Join(Dir(), pcA+".json"), DeviceFile{Device: pcA, Name: "PC A"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}
	if !hasDevice(fake, pcA) || !hasDevice(fake, pcC) {
		t.Error("an unknown device's file dropped a link")
	}
}

func TestUnlinkOfAnOfflinePCDoesntWait(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	fake, c := linkedPair(t, pcA, pcB)
	fake.mu.Lock()
	fake.connected = nil
	fake.mu.Unlock()
	if err := store.WriteJSON(filepath.Join(Dir(), pcB+".json"), DeviceFile{Device: pcB}); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	told, err := Unlink(context.Background(), c, pcB)
	if err != nil || told {
		t.Fatalf("told %v, %v", told, err)
	}
	if time.Since(start) > 5*time.Second || len(fake.scans) > 0 {
		t.Errorf("waited for a PC that is off (%v, scans %v)", time.Since(start), fake.scans)
	}
	if df := readFile(t, pcA); len(df.Unlinked) > 0 {
		t.Errorf("published an unlink no one can read: %+v", df.Unlinked)
	}
	if hasDevice(fake, pcB) || store.LoadState().SyncerPCs[pcB] {
		t.Error("B wasn't unlinked")
	}
	if _, err := os.Stat(filepath.Join(Dir(), pcB+".json")); !os.IsNotExist(err) {
		t.Errorf("B's file is still there: %v", err)
	}
}

func TestUnlinkGivesUpWaitingForTheOtherPC(t *testing.T) {
	defer func(d time.Duration) { unlinkWait = d }(unlinkWait)
	unlinkWait = 300 * time.Millisecond
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	fake, c := linkedPair(t, pcA, pcB)
	fake.mu.Lock()
	fake.completion = map[string]float64{FolderID + "/" + pcB: 50}
	fake.mu.Unlock()
	told, err := Unlink(context.Background(), c, pcB)
	if err != nil || told {
		t.Fatalf("told %v, %v", told, err)
	}
	if hasDevice(fake, pcB) {
		t.Error("B wasn't unlinked after the wait")
	}
}

func TestUnlinkKeepsTheFileAnotherPCStillNeeds(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	fake, c := linkedPair(t, pcA, pcB)
	fake.mu.Lock()
	fake.connected = nil
	fake.devices[pcC] = syncthing.Device{DeviceID: pcC}
	fake.mu.Unlock()
	MarkSyncerPC(pcC)
	for _, df := range []DeviceFile{{Device: pcB}, {Device: pcC, PCs: []PCRef{{ID: pcA}, {ID: pcB}}}} {
		if err := store.WriteJSON(filepath.Join(Dir(), df.Device+".json"), df); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Unlink(context.Background(), c, pcB); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(Dir(), pcB+".json")); err != nil {
		t.Errorf("B's file went, though C is still linked to B: %v", err)
	}
	// C still offers B, but B was turned down here on purpose.
	if waiting, _ := JoinRequests(); slices.Contains(waiting, pcB) {
		t.Errorf("the PC just unlinked is offered again: %v", waiting)
	}
}

func TestAnOldNoticeDoesntEndANewLink(t *testing.T) {
	ctx := context.Background()
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	fake, c := linkedPair(t, pcB, pcA)
	// A unlinked B while B's Syncer was closed; B unlinked A too, by hand,
	// and the user linked both again. A's old file is still here.
	ForgetSyncerPC(pcA)
	MarkSyncerPC(pcA)
	at := time.Now().Add(-time.Minute)
	if err := store.WriteJSON(filepath.Join(Dir(), pcA+".json"), DeviceFile{Device: pcA, Updated: at,
		Unlinked: []UnlinkNote{{ID: pcB, At: at.Unix()}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}
	if !hasDevice(fake, pcA) || !store.LoadState().SyncerPCs[pcA] {
		t.Error("a notice from before the new link ended it")
	}
}

func TestUnlinkKeepsTheFileWhileAnOlderSyncerMayNeedIt(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	fake, c := linkedPair(t, pcA, pcB)
	fake.mu.Lock()
	fake.connected = nil
	fake.devices[pcC] = syncthing.Device{DeviceID: pcC}
	fake.mu.Unlock()
	MarkSyncerPC(pcC)
	// C runs a Syncer that doesn't say which PCs it is linked to.
	for _, df := range []DeviceFile{{Device: pcB}, {Device: pcC, Version: "v1.0.0"}} {
		if err := store.WriteJSON(filepath.Join(Dir(), df.Device+".json"), df); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Unlink(context.Background(), c, pcB); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(Dir(), pcB+".json")); err != nil {
		t.Errorf("B's file went, though C may still be linked to B: %v", err)
	}
}

func TestAReconcileDuringAnUnlinkKeepsTheNotice(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	fake, c := linkedPair(t, pcA, pcB)
	// A folder whose size Reconcile asks while it puts A's file together.
	fake.mu.Lock()
	fake.folders["sized"] = syncthing.Folder{ID: "sized", Label: "Sized", Path: filepath.Join(paths.Root(paths.Roaming), "Sized"),
		Devices: devs(pcA, pcB), Versioning: syncthing.Versioning{Type: "staggered"}, MaxConflicts: -1}
	done := make(chan error, 1)
	once := false
	fake.onStatus = func() {
		if once {
			return
		}
		once = true
		go func() { done <- publishUnlink(pcA, pcB) }()
		time.Sleep(200 * time.Millisecond)
	}
	fake.mu.Unlock()
	if _, err := Reconcile(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if df := readFile(t, pcA); len(df.Unlinked) != 1 {
		t.Errorf("the reconcile wrote A's file back without the notice: %+v", df.Unlinked)
	}
}

// notesShared adds a folder this PC and other sync outside Syncer.
func notesShared(t *testing.T, fake *fakeSyncthing, me, other string) {
	t.Helper()
	store.UpdateState(func(st *store.State) { st.OtherFolders = map[string]bool{"notes": true} })
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.folders["notes"] = syncthing.Folder{ID: "notes", Label: "Notes", Path: filepath.Join(t.TempDir(), "Notes"), Devices: devs(me, other)}
	fake.remote["notes/"+other] = syncthing.RemoteValid
}

func TestARemoteUnlinkLeavesFoldersOutsideSyncerAlone(t *testing.T) {
	ctx := context.Background()
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	fake, c := linkedPair(t, pcB, pcA)
	notesShared(t, fake, pcB, pcA)
	if err := store.WriteJSON(filepath.Join(Dir(), pcA+".json"), DeviceFile{Device: pcA, Updated: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteJSON(filepath.Join(Dir(), pcA+".json"), DeviceFile{Device: pcA, Updated: time.Now(),
		Unlinked: []UnlinkNote{{ID: pcB, At: time.Now().Unix()}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Reconcile(ctx, c); err != nil {
		t.Fatal(err)
	}
	got := folderDevices(fake)
	if slices.Contains(got["game"], pcA) || slices.Contains(got[FolderID], pcA) {
		t.Errorf("A still gets Syncer's folders: %v", got)
	}
	if !slices.Contains(got["notes"], pcA) || !hasDevice(fake, pcA) {
		t.Errorf("the notes folder lost A: %v", got)
	}
	if store.LoadState().SyncerPCs[pcA] {
		t.Error("A is still trusted")
	}
}

func TestUnlinkLeavesFoldersOutsideSyncerAlone(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	fake, c := linkedPair(t, pcA, pcB)
	notesShared(t, fake, pcA, pcB)
	fake.mu.Lock()
	fake.connected = nil
	fake.mu.Unlock()
	if _, err := Unlink(context.Background(), c, pcB); err != nil {
		t.Fatal(err)
	}
	got := folderDevices(fake)
	if slices.Contains(got["game"], pcB) || !slices.Contains(got["notes"], pcB) || !hasDevice(fake, pcB) {
		t.Errorf("folders %v, B in Syncthing %v", got, hasDevice(fake, pcB))
	}
}

func TestANoticeInTheFirstFileSinceLinkingCounts(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	fake, c := linkedPair(t, pcB, pcA) // linked just now; A's file hasn't arrived yet
	at := time.Now().Add(5 * time.Second)
	if err := store.WriteJSON(filepath.Join(Dir(), pcA+".json"), DeviceFile{Device: pcA, Updated: at,
		Unlinked: []UnlinkNote{{ID: pcB, At: at.Unix()}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Reconcile(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if hasDevice(fake, pcA) {
		t.Error("an unlink after the link was passed over")
	}
}

func TestLinkingAnewForgetsTheUnlinksSeen(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	store.UpdateState(func(st *store.State) { st.UnlinkSeen = map[string]int64{pcA: 123} })
	MarkSyncerPC(pcA)
	if st := store.LoadState(); st.UnlinkSeen[pcA] != 0 {
		t.Errorf("unlink seen %v", st.UnlinkSeen)
	}
}

func TestAReplacedFileIsNotTakenForTheOneChecked(t *testing.T) {
	defer paths.SetRootForTest(paths.Roaming, t.TempDir())()
	fake, c := linkedPair(t, pcA, pcB)
	p := filepath.Join(Dir(), pcB+".json")
	write := func(label string) {
		t.Helper()
		if err := store.WriteJSON(p, DeviceFile{Device: pcB, Folders: []SharedFolder{{ID: "g", Label: label, Root: paths.Roaming, Rel: "G"}}}); err != nil {
			t.Fatal(err)
		}
	}
	write("Game A")
	if _, err := Reconcile(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(p)
	// Another PC's version of B's file, of the same size and time.
	fake.mu.Lock()
	fake.writer = map[string]string{pcB + ".json": pcC}
	fake.mu.Unlock()
	write("Game X")
	if err := os.Chtimes(p, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	if fs := readOthers(pcA); len(fs) > 0 {
		t.Errorf("a replaced file is read as the one checked: %+v", fs)
	}
}
