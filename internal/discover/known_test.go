package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ApolloF/syncer/internal/paths"
)

func TestKnownLocation(t *testing.T) {
	home := `C:\Fake[1]\me` // a bracket in the profile path is no wildcard
	roaming := filepath.Join(home, `AppData\Roaming`)
	local := filepath.Join(home, `AppData\Local`)
	docs := filepath.Join(home, "Documents")
	public := `C:\Users\Public`
	for root, dir := range map[string]string{paths.Home: home, paths.Roaming: roaming, paths.Local: local,
		paths.Documents: docs, paths.Public: public, paths.SavedGames: filepath.Join(home, "Saved Games"),
		paths.LocalLow: filepath.Join(home, `AppData\LocalLow`), paths.Ubisoft: ""} {
		t.Cleanup(paths.SetRootForTest(root, dir))
	}
	t.Cleanup(SetManifestForTest([]Entry{
		{Name: "Stardew Valley", Paths: []string{"<winAppData>/StardewValley/Saves"}},
		{Name: "Slots Game", Paths: []string{"<winDocuments>/Slots/slot1", "<winDocuments>/Slots/slot2"}},
		{Name: "Glob Game", Paths: []string{"<winLocalAppData>/Glob/*.sav"}},
		{Name: "Account Game", Paths: []string{"<winAppData>/Acct/<storeUserId>/saves"}},
		{Name: "Ini Game", Paths: []string{"<winDocuments>/My Games/settings.ini", "<winAppData>/game.cfg"}},
		{Name: "Wild Game", Paths: []string{"<winAppData>/Wild/*/data"}},
		{Name: "User Game", Paths: []string{"<home>/Games/<osUserName>/save"}},
		{Name: "Bad Game", Paths: []string{"<winAppData>/../Local/Bad", "<winAppData>/*"}},
		{Name: "Emu Game", SteamID: 1086940, Paths: []string{"<winAppData>/Emu"}},
		{Name: "Age of Fear", Paths: []string{"<winAppData>/Age-of-Fear/**/*.sav"}},
		// As in the real game database.
		{Name: "Blood and Bacon", Paths: []string{"<winLocalAppData>/IsolatedStorage/*.*/*.*/*.*/*.*/Files/savegame"}},
		{Name: "The Spectrum Retreat", SteamID: 763250, Paths: []string{"<winDocuments>/My Games/TheSpectrumRetreat/Saves/SpectrumData",
			"<winPublic>/Documents/Steam/<storeUserId>/763250"}},
		{Name: "Shell Game", Paths: []string{"<winAppData>/Microsoft/Windows/Start Menu/Programs/Startup",
			"<winAppData>/Microsoft/Windows/Themes", "<winDocuments>/WindowsPowerShell", "<winDocuments>/Shell/PowerShell/x",
			"<winLocalAppData>/Programs/Shell", "<winLocalAppData>/Microsoft/WindowsApps", "<home>/Documents/PowerShell"}},
	}))

	in := filepath.Join
	cases := []struct {
		name, label, path string
		want              bool
	}{
		{"the game's save folder", "Stardew Valley", in(roaming, `StardewValley\Saves`), true},
		{"case and punctuation", "stardew valley™", in(roaming, `STARDEWVALLEY\saves`), true},
		{"slashes and a trailing separator", "Stardew Valley", filepath.ToSlash(in(roaming, `StardewValley\Saves`)) + "/", true},
		{"inside the save folder", "Stardew Valley", in(roaming, `StardewValley\Saves\Farm_123`), true},
		{"another folder under the game's name", "Stardew Valley", in(docs, "WindowsPowerShell"), false},
		{"the folder around it", "Stardew Valley", in(roaming, "StardewValley"), false},
		{"a whole root", "Stardew Valley", roaming, false},
		{"climbs out with ..", "Stardew Valley", in(roaming, `StardewValley\Saves`) + `\..\..\..\Local`, false},
		{"climbs out of the location", "Stardew Valley", in(roaming, `StardewValley\Saves`) + `\..\..\Other`, false},
		{"a short name", "Stardew Valley", in(roaming, `STARDE~1\Saves`), false},
		{"a trailing dot", "Stardew Valley", in(roaming, `StardewValley\Saves.`), false},
		{"a stream", "Stardew Valley", in(roaming, `StardewValley\Saves:x`), false},
		{"relative", "Stardew Valley", `AppData\Roaming\StardewValley\Saves`, false},
		{"network path", "Stardew Valley", `\\server\share\StardewValley\Saves`, false},
		{"another game's folder", "Glob Game", in(roaming, `StardewValley\Saves`), false},
		{"unknown game", "Not A Game", in(roaming, `StardewValley\Saves`), false},
		{"label too short", "St", in(roaming, `StardewValley\Saves`), false},
		{"another profile", "Stardew Valley", `C:\Fake1\me\AppData\Roaming\StardewValley\Saves`, false},

		{"sibling slots as one folder", "Slots Game", in(docs, "Slots"), true},
		{"one slot", "Slots Game", in(docs, `Slots\slot1`), true},
		{"another folder next to the slots", "Slots Game", in(docs, `Slots\slot3`), false},
		{"next to it", "Slots Game", in(docs, "SlotsX"), false},

		{"folder of save files", "Glob Game", in(local, "Glob"), true},
		{"a folder in the folder of save files", "Glob Game", in(local, `Glob\Backup`), false},

		{"per-account folders' parent", "Account Game", in(roaming, "Acct"), true},
		{"one account's folder", "Account Game", in(roaming, `Acct\76561198000000000\saves`), true},

		{"a settings file in a shared folder", "Ini Game", in(docs, "My Games"), false},
		{"another game in that folder", "Ini Game", in(docs, `My Games\Other`), false},
		{"a settings file in a root", "Ini Game", roaming, false},

		{"wildcard segment", "Wild Game", in(roaming, `Wild\abc\data`), true},
		{"the folder a wildcard sits in", "Wild Game", in(roaming, "Wild"), true},
		{"a folder in it the database doesn't name", "Wild Game", in(roaming, `Wild\abc`), false},
		{"next to it", "Wild Game", in(roaming, "WildX"), false},
		{"the folder around a ** wildcard", "Age of Fear", in(roaming, "Age-of-Fear"), true},
		{"a folder in it", "Age of Fear", in(roaming, `Age-of-Fear\Campaign\1`), false},
		{"the folder around that", "Age of Fear", roaming, false},

		{"user name", "User Game", in(home, `Games`, currentUserName(), "save"), currentUserName() != ""},
		{"untrusted database paths", "Bad Game", in(local, "Bad"), false},
		{"untrusted wildcard in a root", "Bad Game", in(roaming, "Anything"), false},

		{"emulator folder", "Emu Game (RUNE saves)", in(public, `Documents\Steam\RUNE\1086940`), true},
		{"emulator folder of another app", "Emu Game (RUNE saves)", in(public, `Documents\Steam\RUNE\1086941`), false},
		{"emulator id spelled another way", "Emu Game (RUNE saves)", in(public, `Documents\Steam\RUNE\01086940`), false},
		{"emulator folder without the emulator label", "Emu Game", in(public, `Documents\Steam\RUNE\1086940`), false},
		{"emulator label on the game's folder", "Emu Game (RUNE saves)", in(roaming, "Emu"), false},
		{"the game's own folder", "Emu Game", in(roaming, "Emu"), true},

		{"every .NET app's isolated storage", "Blood and Bacon", in(local, "IsolatedStorage"), false},
		{"another app's isolated storage", "Blood and Bacon", in(local, `IsolatedStorage\a.b\c.d`), false},
		{"the game's own isolated storage", "Blood and Bacon", in(local, `IsolatedStorage\a.b\c.d\e.f\g.h\Files\savegame`), true},
		{"every emulator's saves", "The Spectrum Retreat", in(public, `Documents\Steam`), false},
		{"one emulator's saves", "The Spectrum Retreat", in(public, `Documents\Steam\CODEX`), false},
		{"the public Documents", "The Spectrum Retreat", in(public, "Documents"), false},
		{"the game's folder in it", "The Spectrum Retreat", in(public, `Documents\Steam\CODEX\763250`), true},
		{"the game's own saves", "The Spectrum Retreat", in(docs, `My Games\TheSpectrumRetreat\Saves\SpectrumData`), true},

		{"the Startup folder", "Shell Game", in(roaming, `Microsoft\Windows\Start Menu\Programs\Startup`), false},
		{"Windows' own folders", "Shell Game", in(roaming, `Microsoft\Windows\Themes`), false},
		{"a PowerShell profile", "Shell Game", in(docs, "WindowsPowerShell"), false},
		{"a PowerShell folder deeper down", "Shell Game", in(docs, `Shell\PowerShell\x`), false},
		{"per-user programs", "Shell Game", in(local, `Programs\Shell`), false},
		{"app aliases", "Shell Game", in(local, `Microsoft\WindowsApps`), false},
		{"PowerShell in the profile's Documents", "Shell Game", in(home, `Documents\PowerShell`), false},
	}
	for _, c := range cases {
		if got := KnownLocation(c.label, c.path); got != c.want {
			t.Errorf("%s: KnownLocation(%q, %q) = %v, want %v", c.name, c.label, c.path, got, c.want)
		}
	}

	SetManifestForTest(nil)
	if KnownLocation("Stardew Valley", in(roaming, `StardewValley\Saves`)) {
		t.Error("without a game database nothing is a known location")
	}
}

// A manifest path without an extension may name a file: when it is one on
// this PC, the folder it's in is the game's save folder.
func TestKnownLocationOfAFileWithoutExtension(t *testing.T) {
	roaming := filepath.Join(t.TempDir(), "roaming")
	t.Cleanup(paths.SetRootForTest(paths.Roaming, roaming))
	t.Cleanup(paths.SetRootForTest(paths.Ubisoft, ""))
	t.Cleanup(SetManifestForTest([]Entry{{Name: "Plain Game", Paths: []string{"<winAppData>/Plain/savefile", "<winAppData>/Other/data/saves"}}}))
	writeStoreFile(t, filepath.Join(roaming, "Plain", "savefile"), "x")
	if err := os.MkdirAll(filepath.Join(roaming, "Other", "data", "saves"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !KnownLocation("Plain Game", filepath.Join(roaming, "Plain")) {
		t.Error("the folder of a save file without an extension isn't known")
	}
	if KnownLocation("Plain Game", filepath.Join(roaming, "Other", "data")) {
		t.Error("the folder around a save folder counts as known")
	}
}
