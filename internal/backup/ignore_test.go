package backup

import "testing"

func TestMatcherFirstMatchWins(t *testing.T) {
	cases := []struct {
		name string
		pats []string
		rel  string
		want bool
	}{
		{"negation first keeps the file", []string{"!Saves/**", "*"}, "Saves/slot1.sav", false},
		{"the rest is ignored", []string{"!Saves/**", "*"}, "Other/x.txt", true},
		{"negation after a wider pattern loses", []string{"*", "!Saves/**"}, "Saves/slot1.sav", true},
		{"negation before a narrower pattern", []string{"!keep.log", "*.log"}, "a/keep.log", false},
		{"narrower pattern after the negation still applies", []string{"!keep.log", "*.log"}, "a/other.log", true},
		{"case is ignored", []string{"!saves/**", "*"}, "SAVES/a.sav", false},
		{"flags before a negation", []string{"!(?i)keep.log", "*.log"}, "keep.log", false},
		{"nothing matches", []string{"!keep.log"}, "x.txt", false},
	}
	for _, c := range cases {
		if got := NewMatcher(c.pats).Ignored(c.rel); got != c.want {
			t.Errorf("%s: Ignored(%q) with %v = %v, want %v", c.name, c.rel, c.pats, got, c.want)
		}
	}
}

func TestMatcherBuiltinsBeatUserNegation(t *testing.T) {
	m := NewMatcher(append(append([]string(nil), builtin...), "!desktop.ini", "!Saves/**", "*"))
	if !m.Ignored("Saves/desktop.ini") {
		t.Error("a negation in .stignore must not bring back Syncthing's own files")
	}
	if m.Ignored("Saves/slot1.sav") {
		t.Error("Saves/slot1.sav should be kept")
	}
}

func TestHasNegation(t *testing.T) {
	if NewMatcher([]string{"*.log"}).HasNegation() {
		t.Error("no negation expected")
	}
	if !NewMatcher([]string{"*.log", "!a.log"}).HasNegation() {
		t.Error("negation expected")
	}
}
