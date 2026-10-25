package meta

import (
	"path/filepath"
	"testing"

	"github.com/ApolloF/syncer/internal/accounts"
	"github.com/ApolloF/syncer/internal/paths"
	"github.com/ApolloF/syncer/internal/store"
	"github.com/ApolloF/syncer/internal/syncthing"
)

// A split from another PC may name a place this PC syncs the game at: only
// the game's own shared folder counts, not an account's folder (it may be in
// the vault), a mod folder or a launcher's data.
func TestSyncedAtCountsOnlyTheGamesSharedFolder(t *testing.T) {
	base := t.TempDir()
	defer paths.SetRootForTest(paths.Roaming, base)()
	at := func(name string) string { return filepath.Join(base, name) }
	vaulted := accounts.FolderID("stardew", "aaaaaa")
	if _, err := store.UpdateSettings(func(s *store.Settings) {
		s.Mods["skyrim-mods"] = store.ModFolder{Kind: "mods"}
		s.Launchers["seaglass"] = "Seaglass"
	}); err != nil {
		t.Fatal(err)
	}
	cacheFolders(map[string]syncthing.Folder{
		"stardew":     {ID: "stardew", Path: at("Stardew")},
		vaulted:       {ID: vaulted, Path: at("Vault")},
		"skyrim-mods": {ID: "skyrim-mods", Path: at("Mods")},
		"seaglass":    {ID: "seaglass", Path: at("Seaglass")},
	})
	for _, c := range []struct {
		game, path string
		want       bool
	}{
		{"stardew", at("Stardew"), true},
		{"stardew", at("Vault"), false},
		{"other", at("Stardew"), false},
		{"skyrim-mods", at("Mods"), false},
		{"seaglass", at("Seaglass"), false},
	} {
		if got := syncedAt(c.game, c.path); got != c.want {
			t.Errorf("syncedAt(%s, %s) = %v, want %v", c.game, c.path, got, c.want)
		}
	}
}
