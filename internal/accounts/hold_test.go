package accounts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/ApolloF/syncer/internal/store"
)

// opFolder is OpFolders as the game hold and the pause use it.
func opFolder(t *testing.T, id string) bool {
	ids, err := OpFolders()
	if err != nil {
		t.Fatal(err)
	}
	return slices.Contains(ids, id)
}

// gameStarts and gameExits do what the game hold does (holdSyncWith in
// internal/app) against the fake Syncthing: it pauses the folders that
// aren't paused yet (and remembers those a pause the user set holds),
// except an operation's, and later resumes the ones it still holds. Each
// folder is paused or resumed under the state lock.
func (w *world) gameStarts() {
	var toPause []string
	if err := store.UpdateState(func(s *store.State) {
		for _, id := range w.folderIDs() {
			paused := w.st.folders[id].Paused
			if (paused && !slices.Contains(s.PausedFolders, id)) || opFolder(w.t, id) {
				continue
			}
			if !slices.Contains(s.GamePaused, id) {
				s.GamePaused = append(s.GamePaused, id)
			}
			if !paused {
				toPause = append(toPause, id)
			}
		}
		s.GameHeld = time.Now()
	}); err != nil {
		w.t.Fatal(err)
	}
	for _, id := range toPause {
		if err := store.UpdateState(func(s *store.State) {
			if slices.Contains(s.GamePaused, id) && !opFolder(w.t, id) {
				_ = w.st.PatchFolder(context.Background(), id, map[string]any{"paused": true})
			}
		}); err != nil {
			w.t.Fatal(err)
		}
	}
}

func (w *world) gameExits() {
	for _, id := range store.LoadState().GamePaused {
		if err := store.UpdateState(func(s *store.State) {
			if !slices.Contains(s.GamePaused, id) || opFolder(w.t, id) {
				return
			}
			if _, ok := w.st.folders[id]; ok {
				_ = w.st.PatchFolder(context.Background(), id, map[string]any{"paused": false})
			}
			s.GamePaused = slices.DeleteFunc(s.GamePaused, func(x string) bool { return x == id })
		}); err != nil {
			w.t.Fatal(err)
		}
	}
}

func (w *world) folderIDs() []string {
	var ids []string
	for id := range w.st.folders {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// movingPaused checks, at a journal write, that every folder whose files
// the operation moves is paused and no longer the game hold's.
func (w *world) movingPaused(j *Journal) error {
	held := store.LoadState().GamePaused
	check := func(id string, step int) error {
		f, ok := w.st.folders[id]
		if !ok {
			return nil
		}
		if !f.Paused {
			return fmt.Errorf("%s isn't paused while its saves move (%s, step %d)", id, j.Kind, step)
		}
		if slices.Contains(held, id) {
			return fmt.Errorf("%s is still the game hold's while its saves move (%s, step %d)", id, j.Kind, step)
		}
		return nil
	}
	var errs []error
	switch {
	case j.Kind == KindSplit && j.Step >= splitSnapshot && j.Step <= splitFolders:
		errs = append(errs, check(j.Record.Game, j.Step))
	case j.Kind == KindMerge && j.Step >= mergeSnapshot && j.Step <= mergeFolders:
		for _, a := range j.Record.Accounts {
			errs = append(errs, check(FolderID(j.Record.Game, a), j.Step))
		}
	}
	for _, m := range j.Moves {
		if m.Step >= swapHold && m.Step <= swapConfig {
			errs = append(errs, check(m.InID, m.Step), check(m.OutID, m.Step))
		}
	}
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *world) wantHold(ids ...string) {
	w.t.Helper()
	if got := store.LoadState().GamePaused; !slices.Equal(got, ids) {
		w.t.Errorf("game hold = %v, want %v", got, ids)
	}
}

func (w *world) wantPaused(id string, paused bool) {
	w.t.Helper()
	if f, ok := w.st.folders[id]; !ok {
		w.t.Errorf("%s is gone", id)
	} else if f.Paused != paused {
		w.t.Errorf("%s paused = %v, want %v", id, f.Paused, paused)
	}
}

// A game started before the split: the split takes its folder over from the
// hold, and the account folders it makes aren't the hold's either.
func TestSplitTakesOverGameHeldFolder(t *testing.T) {
	w := newWorld(t)
	alice, bob := w.setup()
	w.gameStarts()
	w.wantHold(game)
	if err := Start(context.Background(), w.env, w.splitRecord(alice, bob)); err != nil {
		t.Fatal(err)
	}
	w.wantHold()
	w.wantPaused(FolderID(game, alice), false)
	w.wantPaused(FolderID(game, bob), false)
}

// A switch resumes the folders the game hold paused when it's done.
func TestSwitchTakesOverGameHeldFolders(t *testing.T) {
	w := newWorld(t)
	alice, bob := w.setup()
	if err := Start(context.Background(), w.env, w.splitRecord(alice, bob)); err != nil {
		t.Fatal(err)
	}
	w.gameStarts()
	w.wantHold(FolderID(game, alice), FolderID(game, bob))
	if err := Switch(context.Background(), w.env, bob); err != nil {
		t.Fatal(err)
	}
	w.wantHold()
	w.wantPaused(FolderID(game, alice), false)
	w.wantPaused(FolderID(game, bob), false)
	if got := readAt(t, filepath.Join(w.live, "slot1.sav")); got != "bob slot 1" {
		t.Errorf("live slot1 = %q", got)
	}
}

// A merge removes the game-held account folders and isn't held itself.
func TestMergeTakesOverGameHeldFolders(t *testing.T) {
	w := newWorld(t)
	alice, bob := w.setup()
	if err := Start(context.Background(), w.env, w.splitRecord(alice, bob)); err != nil {
		t.Fatal(err)
	}
	w.gameStarts()
	r, _ := Load().Split(game)
	r.Kind, r.Winner, r.Assign, r.Gen = KindMerge, bob, nil, r.Gen+1
	if err := Start(context.Background(), w.env, r); err != nil {
		t.Fatal(err)
	}
	w.wantHold()
	w.wantPaused(game, false)
	for _, a := range []string{alice, bob} {
		if _, ok := w.st.folders[FolderID(game, a)]; ok {
			t.Errorf("%s's folder is still there", a)
		}
	}
}

// A game that starts or exits while saves move, or while the operation
// waits to be finished after a crash, never resumes the folders it moves.
func TestGameHoldLeavesSwapAlone(t *testing.T) {
	type crash struct{}
	w := newWorld(t)
	alice, bob := w.setup()
	if err := Start(context.Background(), w.env, w.splitRecord(alice, bob)); err != nil {
		t.Fatal(err)
	}
	w.gameStarts()
	crashHook = func(j *Journal) {
		if err := w.movingPaused(j); err != nil {
			t.Fatal(err)
		}
		if j.Kind != "switch" || len(j.Moves) == 0 {
			return
		}
		switch j.Moves[0].Step {
		case swapIn:
			w.gameExits()
			if err := w.movingPaused(j); err != nil {
				t.Fatalf("after the game exited: %v", err)
			}
		case swapOut:
			w.gameStarts()
			if err := w.movingPaused(j); err != nil {
				t.Fatalf("after the game started: %v", err)
			}
			w.gameExits()
			panic(crash{})
		}
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(crash); !ok {
					panic(r)
				}
			}
		}()
		_ = Switch(context.Background(), w.env, bob)
		t.Fatal("didn't reach the crash")
	}()
	crashHook = nil
	w.held = false
	if _, ok := PendingOp(); !ok {
		t.Fatal("no journal")
	}
	w.wantPaused(FolderID(game, alice), true)
	w.wantPaused(FolderID(game, bob), true)
	if _, err := Apply(context.Background(), w.env); err != nil {
		t.Fatal(err)
	}
	w.wantHold()
	w.wantPaused(FolderID(game, alice), false)
	w.wantPaused(FolderID(game, bob), false)
}

// A journal that can't be read (not just one that isn't there) leaves it
// open which folders are the operation's.
func TestOpFoldersWhenJournalUnreadable(t *testing.T) {
	newWorld(t)
	if ids, err := OpFolders(); err != nil || len(ids) != 0 {
		t.Fatalf("no journal: %v, %v", ids, err)
	}
	if err := os.MkdirAll(journalFile(), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := OpFolders(); err == nil {
		t.Error("an unreadable journal should be reported")
	}
}

// An operation that gives up and can't resume a folder (it may have taken
// it from the game hold) leaves it to the pause's background pass.
func TestFailedResumeIsLeftToThePause(t *testing.T) {
	w := newWorld(t)
	w.setup()
	f := w.st.folders[game]
	f.Paused = true
	w.st.folders[game] = f
	w.st.failOn = "unpause"
	j := Journal{Kind: KindSplit, WasPaused: map[string]bool{game: false, "gone": false}}
	if err := j.resume(context.Background(), w.st, game, "gone"); err == nil {
		t.Fatal("want the failure reported")
	}
	if got := store.LoadState().PausedFolders; !slices.Equal(got, []string{game}) {
		t.Errorf("left to the pause: %v, want [%s]", got, game)
	}
}
