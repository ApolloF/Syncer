//go:build personal

package main

import (
	"context"
	"errors"
	"testing"

	"github.com/ApolloF/syncer/internal/mods"
	"github.com/ApolloF/syncer/internal/paths"
	"github.com/ApolloF/syncer/internal/store"
	"github.com/ApolloF/syncer/internal/update"
)

func TestPersonalBuildNeverInstallsTheReleaseOverItself(t *testing.T) {
	a := &App{}
	for _, auto := range []bool{false, true} {
		if err := a.installUpdate(context.Background(), auto); !errors.Is(err, update.ErrPersonalBuild) {
			t.Errorf("installUpdate(auto=%v) = %v, want ErrPersonalBuild", auto, err)
		}
	}
	if f := buildFeatures(); !f.Mods || !f.GoogleSignIn || !f.Launchers || !f.Personal {
		t.Errorf("features = %+v", f)
	}
}

func TestPersonalBuildModsLoopLeavesFreezingAlone(t *testing.T) {
	t.Cleanup(paths.SetRootForTest(paths.Roaming, t.TempDir()))
	t.Cleanup(paths.SetRootForTest(paths.Local, t.TempDir()))
	if _, err := store.UpdateSettings(func(s *store.Settings) {
		s.Mods["sky-mods"] = store.ModFolder{Kind: mods.KindStaging}
	}); err != nil {
		t.Fatal(err)
	}
	c, patched, mu := pauseServer(t, "sky-mods")
	if freezeMods(context.Background(), c) {
		t.Error("freezeMods paused something")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(patched) != 0 {
		t.Errorf("froze in the personal build: %v", patched)
	}
}
