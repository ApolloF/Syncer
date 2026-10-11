package main

import (
	"context"
	"os"
	"path/filepath"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ApolloF/syncer/internal/mods"
	"github.com/ApolloF/syncer/internal/paths"
	"github.com/ApolloF/syncer/internal/store"
	"github.com/ApolloF/syncer/internal/syncthing"
)

// pauseServer is a Syncthing with the given folders; it records which ones
// were patched to paused (true) or resumed (false).
func pauseServer(t *testing.T, ids ...string) (*syncthing.Client, map[string]bool, *sync.Mutex) {
	t.Helper()
	var mu sync.Mutex
	patched := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/config/folders":
			var parts []string
			for _, id := range ids {
				parts = append(parts, `{"id":"`+id+`","label":"`+id+`","path":"C:/x","paused":false}`)
			}
			_, _ = w.Write([]byte("[" + strings.Join(parts, ",") + "]"))
		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/rest/config/folders/"):
			b := make([]byte, 64)
			n, _ := r.Body.Read(b)
			mu.Lock()
			patched[strings.TrimPrefix(r.URL.Path, "/rest/config/folders/")] = strings.Contains(string(b[:n]), "true")
			mu.Unlock()
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := syncthing.NewAt(strings.TrimPrefix(srv.URL, "http://"), "key")
	if err != nil {
		t.Fatal(err)
	}
	return c, patched, &mu
}

func TestReleaseFoldersResumesWhatSyncerPaused(t *testing.T) {
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	c, patched, mu := pauseServer(t, "a", "b", "c", "d")
	if _, err := store.UpdateSettings(func(s *store.Settings) { s.PausedUntil = time.Now().Add(time.Hour) }); err != nil {
		t.Fatal(err)
	}
	store.UpdateState(func(s *store.State) {
		s.PausedFolders = []string{"a", "gone"}
		s.GamePaused = []string{"b"}
		s.ModHeld = []string{"c"}
	})
	if _, err := store.UpdateSettings(func(s *store.Settings) {
		s.Mods = map[string]store.ModFolder{"d": {Kind: mods.KindDeployed, Role: "receiver"}}
	}); err != nil {
		t.Fatal(err)
	}
	store.UpdateState(func(s *store.State) { s.PausedFolders = append(s.PausedFolders, "d") })
	if err := releaseFolders(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(patched) != 2 || patched["a"] || patched["b"] {
		t.Errorf("patched %v, want a and b resumed and nothing else (c is a held mod folder, d receives deployed mods)", patched)
	}
	if st := store.LoadState(); len(st.PausedFolders)+len(st.GamePaused) != 0 || len(st.ModHeld) != 1 {
		t.Errorf("want the pause and game holds forgotten and the mod hold kept: %+v", st)
	}
	if !store.LoadSettings().PausedUntil.IsZero() {
		t.Error("the pause is still set")
	}
}

func TestSyncPauseRemembersFoldersBeforePausingThem(t *testing.T) {
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	if _, err := store.UpdateSettings(func(s *store.Settings) { s.PausedUntil = time.Now().Add(time.Hour) }); err != nil {
		t.Fatal(err)
	}
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`[{"id":"a","path":"C:/x","paused":false}]`))
		case http.MethodPatch:
			seen = store.LoadState().PausedFolders // what was on disk when Syncthing was asked to pause
			http.Error(w, "no", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	c, _ := syncthing.NewAt(strings.TrimPrefix(srv.URL, "http://"), "key")
	if err := syncPause(context.Background(), c); err == nil {
		t.Fatal("a failed pause should be reported")
	}
	if len(seen) != 1 || seen[0] != "a" {
		t.Errorf("recorded before pausing: %v, want [a]", seen)
	}
	if got := store.LoadState().PausedFolders; len(got) != 0 {
		t.Errorf("a folder that failed to pause is still recorded: %v", got)
	}
}

func TestGameHoldRemembersFoldersBeforePausingThem(t *testing.T) {
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`[{"id":"a","path":"C:/x","paused":false}]`))
		case http.MethodPatch:
			seen = store.LoadState().GamePaused
			http.Error(w, "no", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	c, _ := syncthing.NewAt(strings.TrimPrefix(srv.URL, "http://"), "key")
	if err := holdSyncWith(context.Background(), c, true); err == nil {
		t.Fatal("a failed pause should be reported")
	}
	if len(seen) != 1 || seen[0] != "a" {
		t.Errorf("recorded before pausing: %v, want [a]", seen)
	}
	if got := store.LoadState().GamePaused; len(got) != 0 {
		t.Errorf("a folder that failed to pause is still recorded: %v", got)
	}
}

// pausingServer pauses a folder and then answers the request with an error,
// as when Syncthing applied the change but the answer was lost.
func pausingServer(t *testing.T, id string) *syncthing.Client {
	t.Helper()
	paused := false
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`[{"id":"` + id + `","path":"C:/x","paused":` + strconv.FormatBool(paused) + `}]`))
		case http.MethodPatch:
			paused = true
			http.Error(w, "lost", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := syncthing.NewAt(strings.TrimPrefix(srv.URL, "http://"), "key")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPauseKeepsTrackingFolderThatPausedDespiteAnError(t *testing.T) {
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	if _, err := store.UpdateSettings(func(s *store.Settings) { s.PausedUntil = time.Now().Add(time.Hour) }); err != nil {
		t.Fatal(err)
	}
	_ = syncPause(context.Background(), pausingServer(t, "a"))
	if got := store.LoadState().PausedFolders; len(got) != 1 || got[0] != "a" {
		t.Errorf("a paused folder must stay remembered, got %v", got)
	}
}

func TestGameHoldKeepsTrackingFolderThatPausedDespiteAnError(t *testing.T) {
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	_ = holdSyncWith(context.Background(), pausingServer(t, "a"), true)
	if got := store.LoadState().GamePaused; len(got) != 1 || got[0] != "a" {
		t.Errorf("a paused folder must stay remembered, got %v", got)
	}
}

// unreadableState makes state.json unreadable (a folder stands in its place).
func unreadableState(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(paths.AppDir(), "state.json"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestNoFolderIsPausedWhenItCantBeRemembered(t *testing.T) {
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	if _, err := store.UpdateSettings(func(s *store.Settings) { s.PausedUntil = time.Now().Add(time.Hour) }); err != nil {
		t.Fatal(err)
	}
	unreadableState(t)
	c, patched, mu := pauseServer(t, "a")
	if err := syncPause(context.Background(), c); err == nil {
		t.Error("syncPause: want an error")
	}
	if err := holdSyncWith(context.Background(), c, true); err == nil {
		t.Error("holdSyncWith: want an error")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(patched) != 0 {
		t.Errorf("paused without a record: %v", patched)
	}
}
