package app

import (
	"testing"

	"github.com/ApolloF/syncer/internal/backup"
)

func TestLabelAccountBackups(t *testing.T) {
	list := []backup.Orphan{
		{ID: "stardew-valley.u-nb3naf", Label: "Stardew valley.u nb3naf"},
		{ID: "stardew-valley.u-aaaaaa", Label: "x"},
		{ID: "gone.u-nb3naf", Label: "x"},
		{ID: "celeste", Label: "Celeste"},
		{ID: "hades.u-zzzzzz", Label: "x"},
	}
	name := func(id string) string {
		if id == "nb3naf" {
			return "Alice"
		}
		return ""
	}
	game := func(g string) string { return map[string]string{"stardew-valley": "Stardew Valley", "gone": "Gone"}[g] }
	got := labelAccountBackups(list, map[string]bool{"hades": true}, name, game)
	want := []string{"Stardew Valley · Alice", "Stardew Valley · removed account", "Gone · Alice", "Celeste"}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Label != w {
			t.Errorf("entry %d: %q, want %q", i, got[i].Label, w)
		}
	}
	split := labelAccountBackups(list[:1], map[string]bool{"stardew-valley": true}, name, game)
	if len(split) != 0 {
		t.Errorf("history of a still split game shown: %+v", split)
	}
}
