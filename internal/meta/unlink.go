package meta

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/ApolloF/syncer/internal/logx"
	"github.com/ApolloF/syncer/internal/store"
	"github.com/ApolloF/syncer/internal/syncthing"
)

// Unlinking takes both PCs: this one tells the other through the shared
// settings that it unlinks it, and the other drops the link when it reads
// that. A refused connection looks just like a PC that is off, so without
// the notice the other PC would go on showing this one as offline.

// unlinkWait is how long Unlink waits for the other PC to get the notice.
var unlinkWait = 10 * time.Second

// unlinkFresh is how long a notice is published. It stays after the
// unlink is done: the other PC may get it later through a third PC. Linking
// that PC anew ends it at once (MarkSyncerPC), and the other PC acts on a
// notice only once, and never on one older than the link (dropUnlinkers).
const unlinkFresh = 15 * time.Minute

// PCsFeature: this PC's Syncer publishes the PCs it is linked to (PCs).
const PCsFeature = "pcs-v1"

// ownFile is held while this PC's own file in the shared settings is put
// together and written, so a reconcile running alongside an unlink can't
// write it back without the notice.
var ownFile sync.Mutex

// pruneUnlinking forgets the notices published for longer than unlinkFresh.
func pruneUnlinking() {
	st := store.LoadState()
	var old []string
	for id, at := range st.Unlinking {
		if time.Since(time.Unix(at, 0)) >= unlinkFresh {
			old = append(old, id)
		}
	}
	if len(old) == 0 {
		return
	}
	store.UpdateState(func(st *store.State) {
		for _, id := range old {
			delete(st.Unlinking, id)
		}
	})
}

// linkedPCs are the Syncer PCs this one is linked to (others), by the names
// they have here, for its own file.
func linkedPCs(ds []syncthing.Device, others []string) []PCRef {
	names := map[string]string{}
	for _, d := range ds {
		names[d.DeviceID] = d.Name
	}
	var out []PCRef
	for _, id := range others {
		out = append(out, PCRef{ID: id, Name: names[id]})
	}
	slices.SortFunc(out, func(a, b PCRef) int { return cmp.Compare(a.ID, b.ID) })
	return out
}

// unlinkingNow are the unlink notices this PC publishes: the unlinks still
// running here.
func unlinkingNow() []UnlinkNote {
	var out []UnlinkNote
	for id, at := range store.LoadState().Unlinking {
		if d := time.Since(time.Unix(at, 0)); d < unlinkFresh && d > -unlinkFresh {
			out = append(out, UnlinkNote{ID: id, At: at})
		}
	}
	slices.SortFunc(out, func(a, b UnlinkNote) int { return cmp.Compare(a.ID, b.ID) })
	return out
}

// Unlink unlinks PC id: when it is online and syncs the shared settings it
// is told first, and given up to unlinkWait to get that, so it drops the
// link too. Then the device goes from Syncthing and from every folder, and
// this PC forgets it. told reports whether the other PC got the notice.
func Unlink(ctx context.Context, c *syncthing.Client, id string) (told bool, err error) {
	st, err := c.Status(ctx)
	if err != nil {
		return false, err
	}
	me := st.MyID
	if id == me {
		return false, errors.New("that's this PC")
	}
	if reachable(ctx, c, id) {
		told = tell(ctx, c, me, id)
	}
	if err := forgetPC(ctx, c, me, id); err != nil {
		return told, err
	}
	if _, err := Reconcile(ctx, c); err != nil {
		return told, err
	}
	return told, nil
}

// reachable reports whether PC id is connected and syncs the shared
// settings, so a notice there reaches it.
func reachable(ctx context.Context, c *syncthing.Client, id string) bool {
	conns, err := c.Connections(ctx)
	if err != nil || !conns.Connections[id].Connected {
		return false
	}
	comp, err := c.FolderCompletion(ctx, FolderID, id)
	return err == nil && comp.Accepted()
}

// tell publishes that this PC (me) unlinks id and waits until id has this
// PC's file, or unlinkWait passes.
func tell(ctx context.Context, c *syncthing.Client, me, id string) bool {
	if err := publishUnlink(me, id); err != nil {
		logx.Printf("meta: can't tell %s about the unlink: %v", shortID(id), err)
		return false
	}
	// Scanned right away: Syncthing's watcher would wait a few seconds,
	// and the other PC's progress must be measured against this version.
	if err := c.Rescan(ctx, FolderID); err != nil {
		logx.Printf("meta: can't tell %s about the unlink: %v", shortID(id), err)
		return false
	}
	deadline := time.Now().Add(unlinkWait)
	for {
		comp, err := c.FolderCompletion(ctx, FolderID, id)
		if err == nil && comp.Completion >= 100 && comp.NeedBytes == 0 {
			return true
		}
		if err != nil && !linkedTo(ctx, c, id) {
			return true // it unlinked this PC at the same time, and that was acted on here
		}
		if time.Now().After(deadline) {
			logx.Printf("meta: %s didn't get the unlink in time", shortID(id))
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// publishUnlink adds the notice that this PC (me) unlinks id to this PC's
// file in the shared settings.
func publishUnlink(me, id string) error {
	ownFile.Lock()
	defer ownFile.Unlock()
	own := filepath.Join(Dir(), me+".json")
	b, err := os.ReadFile(own)
	if err != nil {
		return err
	}
	var df DeviceFile
	if err := json.Unmarshal(b, &df); err != nil || df.Device != me {
		return errors.New("this PC's file is unreadable")
	}
	at := time.Now().Unix()
	store.UpdateState(func(st *store.State) {
		if st.Unlinking == nil {
			st.Unlinking = map[string]int64{}
		}
		st.Unlinking[id] = at
	})
	df.Unlinked, df.Updated = unlinkingNow(), time.Now()
	return store.WriteJSON(own, df)
}

// linkedTo reports whether device id is still in Syncthing here (true when
// that can't be told).
func linkedTo(ctx context.Context, c *syncthing.Client, id string) bool {
	ds, err := c.AllDevices(ctx)
	return err != nil || slices.ContainsFunc(ds, func(d syncthing.Device) bool { return d.DeviceID == id })
}

// dropUnlinkers drops the link with each trusted PC that tells this one (me)
// it unlinked it. Only a PC's own file, last written by that PC (see
// readOtherFiles), counts, and only for the link with that PC. A notice is
// acted on once, and only when it is newer than the link: newer than the
// first file of that PC seen since it was linked (both by that PC's clock),
// or, in that first file, than when it was linked here (clocks of two PCs,
// close enough for a notice published while an unlink runs). So an old copy
// still here when the PCs are linked anew changes nothing.
// The other PC may read a notice hours later, when its Syncer next runs. It
// returns the devices left.
func dropUnlinkers(ctx context.Context, c *syncthing.Client, me string, ds []syncthing.Device) []syncthing.Device {
	linked := map[string]bool{}
	for _, d := range ds {
		linked[d.DeviceID] = true
	}
	st := store.LoadState()
	first := map[string]int64{}
	dropped := false
	for _, df := range readOtherFiles(me) {
		if !linked[df.Device] {
			continue
		}
		since, ok := st.LinkSince[df.Device]
		if !ok {
			since = st.LinkedAt[df.Device] // 0 for a PC linked by an earlier Syncer, which sent no notices
		}
		var at int64
		for _, u := range df.Unlinked {
			if u.ID == me && u.At > st.UnlinkSeen[df.Device] && u.At > since {
				at = max(at, u.At)
			}
		}
		if at == 0 {
			if !ok {
				first[df.Device] = df.Updated.Unix()
			}
			continue
		}
		if err := forgetPC(ctx, c, me, df.Device); err != nil {
			logx.Printf("meta: %s unlinked this PC, but dropping it failed: %v", shortID(df.Device), err)
			continue
		}
		store.UpdateState(func(st *store.State) {
			if st.UnlinkSeen == nil {
				st.UnlinkSeen = map[string]int64{}
			}
			st.UnlinkSeen[df.Device] = at
		})
		logx.Printf("meta: %s (%s) unlinked this PC: dropped it here too", cmp.Or(df.Name, "a PC"), shortID(df.Device))
		dropped = true
	}
	if len(first) > 0 {
		store.UpdateState(func(st *store.State) {
			for id, at := range first {
				if _, ok := st.LinkSince[id]; !ok && st.SyncerPCs[id] {
					if st.LinkSince == nil {
						st.LinkSince = map[string]int64{}
					}
					st.LinkSince[id] = at
				}
			}
		})
	}
	if !dropped {
		return ds
	}
	if left, err := c.AllDevices(ctx); err == nil {
		return left
	}
	return ds
}

// forgetPC takes PC id off Syncer's folders and forgets it as one of the
// user's PCs. Folders shared only with it stay here, synced with no one.
// Folders that aren't Syncer's (a notes folder the two PCs sync) are left
// alone, and the device stays in Syncthing for them; without any, it goes.
// It stays turned down as a PC another PC may still offer; linking it on
// purpose undoes that. Its file in the shared settings goes unless another
// trusted PC may still be linked to it.
func forgetPC(ctx context.Context, c *syncthing.Client, me, id string) error {
	fs, err := c.AllFolders(ctx)
	if err != nil {
		return err
	}
	own := store.LoadState().SyncerFolders
	inUse := false
	for _, f := range fs {
		keep := slices.DeleteFunc(slices.Clone(f.Devices), func(d syncthing.FolderDevice) bool { return d.DeviceID == id })
		if len(keep) == len(f.Devices) {
			continue
		}
		if f.ID != FolderID && !own[f.ID] {
			inUse = true
			continue
		}
		if keep == nil {
			keep = []syncthing.FolderDevice{}
		}
		if err := c.PatchFolder(ctx, f.ID, map[string]any{"devices": keep}); err != nil {
			return err
		}
	}
	ds, err := c.AllDevices(ctx)
	if err != nil {
		return err
	}
	if !inUse && slices.ContainsFunc(ds, func(d syncthing.Device) bool { return d.DeviceID == id }) {
		if err := c.RemoveDevice(ctx, id); err != nil {
			return err
		}
	}
	store.UpdateState(func(st *store.State) {
		delete(st.SyncerPCs, id)
		delete(st.JoinRequests, id)
		delete(st.Introduced, id)
		delete(st.LinkSince, id)
		delete(st.LinkedAt, id)
		if st.IgnoredPCs == nil {
			st.IgnoredPCs = map[string]bool{}
		}
		st.IgnoredPCs[id] = true
	})
	dropFile(me, id)
	return nil
}

// dropFile deletes PC id's file from the shared settings, unless another
// trusted PC may be linked to it: that PC still reads the file, and the
// delete would reach it. Only a PC whose Syncer publishes the PCs it is
// linked to (and whose file can be read) can tell it isn't.
func dropFile(me, id string) {
	files := map[string]DeviceFile{}
	for _, df := range readOtherFiles(me) {
		files[df.Device] = df
	}
	for pc := range knownPCs() {
		if pc == me || pc == id {
			continue
		}
		df, ok := files[pc]
		if !ok || !slices.Contains(df.Features, PCsFeature) || slices.ContainsFunc(df.PCs, func(p PCRef) bool { return p.ID == id }) {
			return
		}
	}
	if err := os.Remove(filepath.Join(Dir(), id+".json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		logx.Printf("meta: remove the file of %s: %v", shortID(id), err)
	}
}
