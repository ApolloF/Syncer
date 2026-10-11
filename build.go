package main

import (
	"errors"

	"github.com/ApolloF/syncer/internal/features"
	"github.com/ApolloF/syncer/internal/store"
)

// errNotInBuild is what a feature this build doesn't have answers with (see
// internal/features). Its methods stay bound in every build so the generated
// frontend bindings are the same; the UI hides the feature, so a user only
// meets this text if something else calls it.
var errNotInBuild = errors.New("This feature isn't part of this version of Syncer.")

// FeatureFlags tells the UI which optional parts this build has.
type FeatureFlags struct {
	Mods         bool `json:"mods"`
	GoogleSignIn bool `json:"googleSignIn"`
	Launchers    bool `json:"launchers"`
	Personal     bool `json:"personal"`
}

func buildFeatures() FeatureFlags {
	return FeatureFlags{Mods: features.Mods, GoogleSignIn: features.GoogleSignIn,
		Launchers: features.Launchers, Personal: features.Personal}
}

// hiddenFolder reports whether a synced folder belongs to a feature this
// build doesn't have (a mod folder, a launcher's data). Such a folder stays
// in Syncthing as it is, but isn't listed or counted here.
func hiddenFolder(s store.Settings, id string) bool {
	return !features.Mods && isMod(s, id) || !features.Launchers && isLauncherData(s, id)
}
