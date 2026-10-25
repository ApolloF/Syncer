package discover

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ApolloF/syncer/internal/paths"
)

// KnownLocation reports whether path, a folder another PC syncs under label,
// is one of the save locations the game database lists for that game on this
// PC. It never looks at the disk: the folder may not exist here yet. It only
// answers from the cached game database (false without one).
func KnownLocation(label, path string) bool {
	classCache.Lock()
	idx := indexFor(CachedManifest())
	classCache.Unlock()
	return knownLocation(idx, label, path, currentUserName(), knownBroad())
}

// knownBroad are the folders that hold many games' (or programs') files,
// which are never one game's save folder: tooBroad, plus the folders Steam
// emulators keep every game's saves in (Scan counts those too), .NET's
// isolated storage, and the public profile and its Documents.
func knownBroad() map[string]bool {
	m := tooBroad()
	add := func(root, rel string) {
		if p, ok := paths.Resolve(root, rel); ok {
			m[strings.ToLower(filepath.Clean(p))] = true
		}
	}
	for _, r := range emulatorRoots {
		add(r.root, r.rel)
		add(r.root, filepath.ToSlash(filepath.Dir(filepath.FromSlash(r.rel))))
	}
	add(paths.Local, "IsolatedStorage")
	add(paths.Public, "Documents")
	add(paths.Public, "")
	return m
}

// emulatorFolder reports whether segs is one emulator's folder in a folder
// several share (…\Steam\CODEX): it holds every game that emulator runs.
func emulatorFolder(segs []string) bool {
	for _, r := range emulatorRoots {
		if r.group != "" {
			continue
		}
		if base, ok := paths.Resolve(r.root, r.rel); ok {
			bs := strings.Split(strings.ToLower(filepath.Clean(base)), `\`)
			if len(segs) == len(bs)+1 && strings.Join(segs[:len(bs)], `\`) == strings.Join(bs, `\`) {
				return true
			}
		}
	}
	return false
}

func knownLocation(idx *manifestIndex, label, path, userName string, broad map[string]bool) bool {
	segs, ok := splitPath(path)
	if !ok {
		return false
	}
	label = strings.TrimSpace(label)
	name := strings.TrimSpace(emuSuffix.ReplaceAllString(label, ""))
	n := normalize(name)
	if len(n) < 3 {
		return false
	}
	if neverSaves(segs) {
		return false
	}
	games := idx.byNorm[n]
	if name != label {
		return emulatorFolderOf(games, segs)
	}
	for _, e := range games {
		for _, loc := range saveLocations(e, userName) {
			if !loc.holds(segs) {
				continue
			}
			at := segs[:len(loc.segs)]
			if !broad[strings.Join(at, `\`)] && !emulatorFolder(at) {
				return true
			}
		}
	}
	return false
}

// location is a save folder as path segments: the root's segments are
// matched as written, the rest may be wildcards ("*", "?", "[…]"), each
// within one segment. A location widened from what the game database says
// (the folder around its paths) is that folder only: what lies below it may
// be another program's.
type location struct {
	segs  []segment
	exact bool
}

type segment struct {
	text string // lower-cased
	glob bool
}

// holds reports whether segs (lower-cased) is the location or lies below it
// (is it, for an exact one). A folder that holds the location is never one
// of the game's: it holds other things too.
func (l location) holds(segs []string) bool {
	if len(segs) < len(l.segs) || l.exact && len(segs) != len(l.segs) {
		return false
	}
	for i, s := range l.segs {
		if !s.glob {
			if s.text != segs[i] {
				return false
			}
			continue
		}
		if ok, err := filepath.Match(s.text, segs[i]); err != nil || !ok {
			return false
		}
	}
	return true
}

func key(segs []segment) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.text)
		b.WriteByte('\\')
	}
	return b.String()
}

// segments splits an expanded manifest path into its root's segments (roots
// of them) and the rest.
func segments(base, tail string) (l []segment, roots int) {
	for _, s := range strings.Split(strings.ToLower(filepath.Clean(base)), `\`) {
		if s != "" {
			l = append(l, segment{text: s})
		}
	}
	roots = len(l)
	for _, s := range strings.FieldsFunc(strings.ToLower(tail), func(r rune) bool { return r == '/' || r == '\\' }) {
		if s != "." {
			l = append(l, segment{text: s, glob: strings.ContainsAny(s, "*?[")})
		}
	}
	return l, roots
}

// saveLocations are the folders Scan could find e's saves in on this PC:
// each manifest path, and (exact) the folder of one that names files (a
// wildcard or an extension in its last segment, or a file without an
// extension that is on this PC), the folder a wildcard in the middle of a
// path sits in (Scan syncs it when the wildcard matches several folders),
// the folder above per-account folders, and the parent of several sibling
// folders, which Scan syncs as one.
func saveLocations(e Entry, userName string) []location {
	var out []location
	seen := map[string]int{}
	add := func(segs []segment, exact bool) {
		if len(segs) == 0 {
			return
		}
		k := key(segs)
		if i, ok := seen[k]; ok {
			out[i].exact = out[i].exact && exact
			return
		}
		seen[k] = len(out)
		out = append(out, location{segs: segs, exact: exact})
	}
	for _, raw := range e.Paths {
		base, tail, ok := expandParts(raw, userName)
		if !ok {
			continue
		}
		if strings.Contains(raw, "<storeUserId>") && !strings.HasPrefix(raw, ubisoftPlaceholder) {
			// Cut above the account's folder, as Scan syncs it; one
			// account's folder is the database's path for any account.
			l, _ := segments(base, tail)
			add(l, true)
			if b, t, ok := expandParts(strings.ReplaceAll(raw, "<storeUserId>", "*"), userName); ok {
				l, _ := segments(b, t)
				add(l, false)
			}
			continue
		}
		l, roots := segments(base, tail)
		add(l, false)
		if len(l) == roots {
			continue
		}
		last := l[len(l)-1]
		abs := filepath.Clean(filepath.FromSlash(base + tail))
		if last.glob || filepath.Ext(last.text) != "" || !strings.ContainsAny(abs, "*?[") && isFile(abs) {
			add(l[:len(l)-1], true)
		}
		for i := roots; i < len(l)-1; i++ {
			if l[i].glob {
				add(l[:i], true)
				break
			}
		}
	}
	parents := map[string]int{}
	for _, l := range out {
		parents[key(l.segs[:len(l.segs)-1])]++
	}
	for _, l := range out[:len(out):len(out)] {
		if p := l.segs[:len(l.segs)-1]; parents[key(p)] > 1 {
			add(p, true)
		}
	}
	return out
}

// neverSaves reports whether segs is at or below a folder that holds
// programs Windows runs or a shell runs at startup, whatever the game
// database says: the Start menu and the rest of Windows' own folders in
// AppData, PowerShell profiles, and per-user program folders.
func neverSaves(segs []string) bool {
	under := func(root string, rel ...string) bool {
		base := paths.Root(root)
		if base == "" {
			return false
		}
		bs := append(strings.Split(strings.ToLower(filepath.Clean(base)), `\`), rel...)
		return len(segs) >= len(bs) && strings.Join(segs[:len(bs)], `\`) == strings.Join(bs, `\`)
	}
	if under(paths.Roaming, "microsoft", "windows") || under(paths.ProgramData, "microsoft", "windows", "start menu") ||
		under(paths.Local, "programs") || under(paths.Local, "microsoft", "windowsapps") {
		return true
	}
	for _, docs := range [][]string{{paths.Documents}, {paths.Home, "documents"}} {
		root, rel := docs[0], docs[1:]
		if !under(root, rel...) {
			continue
		}
		bs := len(strings.Split(strings.ToLower(filepath.Clean(paths.Root(root))), `\`)) + len(rel)
		for _, s := range segs[bs:] {
			if strings.HasSuffix(s, "powershell") {
				return true
			}
		}
	}
	return false
}

// emulatorFolderOf reports whether segs is a Steam or Ubisoft emulator's
// folder for one of games (…\Steam\RUNE\<app id>), by its place alone.
func emulatorFolderOf(games []Entry, segs []string) bool {
	for _, r := range emulatorRoots {
		base, ok := paths.Resolve(r.root, r.rel)
		if !ok {
			continue
		}
		bs := strings.Split(strings.ToLower(filepath.Clean(base)), `\`)
		depth := len(bs) + 1
		if r.group == "" {
			depth++ // …\Steam\<emulator>\<app id>
		}
		if len(segs) != depth || strings.Join(segs[:len(bs)], `\`) != strings.Join(bs, `\`) {
			continue
		}
		id, err := strconv.Atoi(segs[depth-1])
		if err != nil || id <= 0 || strconv.Itoa(id) != segs[depth-1] {
			continue
		}
		for _, e := range games {
			if !r.ubisoft && e.SteamID == id {
				return true
			}
			for _, u := range e.UbisoftIDs {
				if r.ubisoft && u == id {
					return true
				}
			}
		}
	}
	return false
}

// splitPath lower-cases an absolute path and splits it into segments. Paths
// that could name another folder than they spell are refused: ".." (it
// would climb out of a location), a stream or a short name.
func splitPath(p string) ([]string, bool) {
	p = filepath.FromSlash(p)
	vol := filepath.VolumeName(p)
	if !filepath.IsAbs(p) || len(vol) != 2 {
		return nil, false // not a drive path (UNC, \\?\, relative)
	}
	var out []string
	for _, s := range strings.Split(strings.ToLower(p[len(vol):]), `\`) {
		switch {
		case s == "" || s == ".":
		case s == ".." || strings.Contains(s, ":") || paths.LooksShort(s) || strings.TrimRight(s, ". ") != s:
			return nil, false
		default:
			out = append(out, s)
		}
	}
	return append([]string{strings.ToLower(vol)}, out...), true
}
