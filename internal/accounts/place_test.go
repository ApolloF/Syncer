package accounts

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ApolloF/syncer/internal/discover"
	"github.com/ApolloF/syncer/internal/paths"
)

// A split from another PC creates the active account's folder at the place
// the record names, without asking: only a place this PC already keeps the
// game's saves at, or one of the game's save folders in the game database,
// is taken in.
func TestCombineRefusesSplitsAtUnknownPlaces(t *testing.T) {
	home := `C:\Fake\me`
	for root, dir := range map[string]string{paths.Home: home, paths.Roaming: filepath.Join(home, `AppData\Roaming`),
		paths.Local: filepath.Join(home, `AppData\Local`), paths.Documents: filepath.Join(home, "Documents"), paths.Ubisoft: ""} {
		t.Cleanup(paths.SetRootForTest(root, dir))
	}
	t.Cleanup(discover.SetManifestForTest([]discover.Entry{{Name: "Stardew Valley", Paths: []string{"<winAppData>/StardewValley/Saves"}}}))
	oldResolve, oldSynced := resolveLive, SyncedAt
	resolveLive = func(r Record) (string, error) { // the folders don't exist on the test PC
		if p, ok := paths.Resolve(r.Root, r.Rel); ok {
			return p, nil
		}
		return "", errNoLive
	}
	notes := filepath.Join(home, `Documents\Notes`)
	SyncedAt = func(game, p string) bool { return game == "notes" && p == notes }
	t.Cleanup(func() { resolveLive, SyncedAt = oldResolve, oldSynced })

	t0 := time.Now()
	rec := func(game, label, root, rel string, gen int) Record {
		return Record{Kind: KindSplit, Game: game, Label: label, Root: root, Rel: rel, Accounts: []string{"aaaaaa", "bbbbbb"}, Gen: gen, Created: t0}
	}
	var s State
	s.Combine([]Shared{{Records: []Record{
		rec("stardew-valley", "Stardew Valley", paths.Documents, "WindowsPowerShell", 1),
		rec("other-game", "Not In The Database", paths.Roaming, "Other", 1),
	}}})
	if len(s.Records) != 0 {
		t.Fatalf("took in splits at places that aren't the game's: %+v", s.Records)
	}

	s.Combine([]Shared{{Records: []Record{
		rec("stardew-valley", "Stardew Valley", paths.Roaming, "StardewValley/Saves", 1),
		rec("notes", "Notes", paths.Documents, "Notes", 1), // synced here already
	}}})
	if len(s.Records) != 2 {
		t.Fatalf("legit splits refused: %+v", s.Records)
	}

	// A newer record moving the saves elsewhere is refused; one that keeps
	// them where they are is taken, whatever the game database says.
	SyncedAt = nil
	s.Combine([]Shared{{Records: []Record{
		rec("stardew-valley", "Stardew Valley", paths.Documents, "WindowsPowerShell", 2),
		rec("notes", "Notes", paths.Documents, "Notes", 2),
	}}})
	for _, r := range s.Records {
		switch {
		case r.Game == "stardew-valley" && (r.Gen != 1 || r.Rel != "StardewValley/Saves"):
			t.Errorf("a newer record moved Stardew Valley's saves: %+v", r)
		case r.Game == "notes" && r.Gen != 2:
			t.Errorf("a newer record at the same place was refused: %+v", r)
		}
	}
}
