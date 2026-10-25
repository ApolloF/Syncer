package backup

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Matcher applies Syncthing-style ignore patterns (from .stignore) plus
// Syncthing's own bookkeeping files, so the backup mirrors what gets synced.
type Matcher struct {
	rules []rule
	neg   bool // some rule un-ignores
}

type rule struct {
	re  *regexp.Regexp
	neg bool
}

// steam_autocloud.vdf belongs to the PC it's on: Steam writes it for the
// account that uses that PC, and a copy of it on another PC tells Steam (and
// Syncer) the wrong owner.
var builtin = []string{".stfolder", ".stversions", ".stignore", "~syncthing~*", ".syncthing.*.tmp", "*.syncer-tmp", "desktop.ini", "Thumbs.db",
	SteamMarker}

// ProgramTypes are the programs and scripts that never belong in a save
// folder, and files Explorer acts on just by showing them (.library-ms,
// .searchConnector-ms): a folder another PC sends could carry one. Data
// types some games keep (.js, .reg) aren't among them. Mod folders are full
// of programs, so they keep theirs.
var ProgramTypes = []string{"*.exe", "*.dll", "*.scr", "*.com", "*.msi", "*.bat", "*.cmd", "*.ps1", "*.psm1",
	"*.vbs", "*.vbe", "*.wsf", "*.wsh", "*.lnk", "*.hta", "*.cpl", "*.pif", "*.jse", "*.url", "*.appref-ms", "*.scf",
	"*.wsc", "*.psd1",
	"*.library-ms", "*.searchConnector-ms", "*.jar", "*.msc", "*.msp", "*.application", "*.settingcontent-ms"}

// SteamMarker is the file Steam Cloud writes into the save folders it keeps.
const SteamMarker = "steam_autocloud.vdf"

// The lines Syncer writes into a synced folder's .stignore (a game's
// exclusions) sit between these markers; the user's own lines are kept.
const (
	IgnoreBegin = "// Syncer: begin (edit these in Syncer)"
	IgnoreEnd   = "// Syncer: end"
)

// LoadMatcher reads <dir>/.stignore if present, plus extra patterns (the
// game's exclusions). Syncer's own block in .stignore is skipped: extra is
// the current list, the block may be left over from when it was synced.
func LoadMatcher(dir string, extra ...string) *Matcher {
	return loadMatcher(dir, nil, extra)
}

// Matcher is the Matcher for backing up (or restoring) f: a save folder's
// programs are skipped ahead of its .stignore, so no line there brings them
// back.
func (f Folder) Matcher() *Matcher {
	var types []string
	if !f.Mod {
		types = ProgramTypes
	}
	return loadMatcher(f.Path, types, f.Exclude)
}

// loadMatcher is LoadMatcher with patterns that go first, with the built-in ones.
func loadMatcher(dir string, first, extra []string) *Matcher {
	pats := append(append([]string(nil), builtin...), first...)
	if f, err := os.Open(filepath.Join(dir, ".stignore")); err == nil {
		sc := bufio.NewScanner(f)
		managed := false
		for sc.Scan() {
			switch l := sc.Text(); strings.TrimSpace(l) {
			case IgnoreBegin:
				managed = true
			case IgnoreEnd:
				managed = false
			default:
				if !managed {
					pats = append(pats, l)
				}
			}
		}
		f.Close()
	}
	return NewMatcher(append(pats, extra...))
}

// NewMatcher compiles patterns the way Syncthing reads them: in order, the
// first one that matches decides, and a leading ! un-ignores. #include lines
// are skipped.
func NewMatcher(pats []string) *Matcher {
	m := &Matcher{}
	for _, p := range pats {
		p = strings.TrimSpace(p)
		if p == "" || strings.HasPrefix(p, "//") || strings.HasPrefix(p, "#") {
			continue
		}
		neg := strings.HasPrefix(p, "!")
		p = strings.TrimSpace(strings.TrimPrefix(p, "!"))
		for {
			switch {
			case strings.HasPrefix(p, "(?d)"), strings.HasPrefix(p, "(?i)"):
				p = p[4:]
				continue
			}
			break
		}
		p = strings.TrimSuffix(strings.ReplaceAll(p, `\`, "/"), "/")
		anchored := strings.HasPrefix(p, "/")
		p = strings.TrimPrefix(p, "/")
		if p == "" {
			continue
		}
		re := globRE(p)
		if anchored {
			re = "^" + re + "(/.*)?$"
		} else {
			re = "(^|/)" + re + "(/.*)?$"
		}
		if r, err := regexp.Compile("(?i)" + re); err == nil {
			m.rules = append(m.rules, rule{r, neg})
			m.neg = m.neg || neg
		}
	}
	return m
}

// Ignored reports whether rel (forward or back slashes) is excluded.
func (m *Matcher) Ignored(rel string) bool {
	rel = filepath.ToSlash(rel)
	for _, r := range m.rules {
		if r.re.MatchString(rel) {
			return !r.neg
		}
	}
	return false
}

// HasNegation reports whether a rule un-ignores something, so that an ignored
// folder may still hold files that are not.
func (m *Matcher) HasNegation() bool { return m.neg }

func globRE(p string) string {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch c {
		case '*':
			if i+1 < len(p) && p[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return b.String()
}
