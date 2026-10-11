//go:build personal

package main

import (
	"context"
	"errors"
	"testing"

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
