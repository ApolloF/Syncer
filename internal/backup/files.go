package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ApolloF/syncer/internal/fsx"
	"github.com/ApolloF/syncer/internal/logx"
	"github.com/ApolloF/syncer/internal/paths"
)

// Next to its info file, a PC lists the files its last backup of a folder
// left in the backup (<target>\.syncer\<id>\files\<pc>.json), and the info
// file names the list's hash. Google Drive for desktop uploads and downloads
// files one by one, so the backup another PC sees may be half there: a PC
// only takes saves from the backup when every file on the list is there as
// listed. The list sits in a folder of its own, where older Syncer versions
// don't take it for an info file.
const filesDir = "files"

// maxFiles caps a file list; it comes from another PC, so it isn't trusted.
const maxFiles = 8 << 20

// FileEntry is one file of a backup: its path in the folder, size and
// modification time (unix milliseconds).
type FileEntry struct {
	Path  string `json:"p"`
	Size  int64  `json:"s"`
	MTime int64  `json:"m"`
}

func (e FileEntry) time() time.Time { return time.UnixMilli(e.MTime) }

type fileList struct {
	ID    string      `json:"id"`
	Files []FileEntry `json:"files"`
}

func filesPath(target, id, key string) string {
	return filepath.Join(target, InfoDir, id, filesDir, key+".json")
}

// writeFiles writes this PC's list of f's files in the backup and returns
// its hash.
func writeFiles(target, id string, files []FileEntry) (string, error) {
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	b, err := json.Marshal(fileList{ID: id, Files: files})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	hash := hex.EncodeToString(sum[:])
	p := filesPath(target, id, hostKey)
	if old, err := os.ReadFile(p); err == nil {
		if s := sha256.Sum256(old); hex.EncodeToString(s[:]) == hash {
			return hash, nil // unchanged: nothing for Drive to upload
		}
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	tmp := p + tmpSuffix
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return hash, nil
}

// ReadFiles returns the list of files another PC's backup of in.ID left in
// the backup, as its info file describes it. It fails when the list isn't
// (yet) the one the info file names, or holds a path that could point
// outside the folder.
func ReadFiles(target string, in Info) ([]FileEntry, error) {
	if in.Files == "" || in.key == "" || !paths.ValidID(in.ID) {
		return nil, errors.New("that PC's backup has no file list (an older Syncer)")
	}
	p := filesPath(target, in.ID, in.key)
	fi, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() || fi.Size() > maxFiles {
		return nil, errors.New("not a file list")
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	if s := sha256.Sum256(b); hex.EncodeToString(s[:]) != in.Files {
		return nil, errors.New("the file list hasn't fully arrived yet")
	}
	var l fileList
	if err := json.Unmarshal(b, &l); err != nil {
		return nil, err
	}
	if l.ID != in.ID {
		return nil, errors.New("file list of another folder")
	}
	for _, e := range l.Files {
		if !safeRel(e.Path) || e.Size < 0 {
			return nil, fmt.Errorf("unsafe path in file list: %q", e.Path)
		}
	}
	return l.Files, nil
}

// safeRel reports whether rel names a file inside a folder.
func safeRel(rel string) bool {
	if rel == "" || rel[0] == '/' || rel[0] == '\\' || filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" || strings.ContainsAny(rel, ":\x00") {
		return false
	}
	for _, seg := range strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == '\\' }) {
		if seg == ".." || seg == "." {
			return false
		}
	}
	return true
}

// MirrorMatches reports whether the backup of id holds every listed file as
// listed: Drive has brought all of that PC's backup to this PC.
func MirrorMatches(target, id string, files []FileEntry) bool {
	if len(files) == 0 {
		return false
	}
	for _, e := range files {
		fi, err := os.Stat(filepath.Join(target, id, filepath.FromSlash(e.Path)))
		if err != nil || fi.IsDir() || fi.Size() != e.Size || !sameTime(fi.ModTime(), e.time()) {
			return false
		}
	}
	return true
}

// Live is what a folder held at one moment: the size and modification time
// (unix nanoseconds) of every file a backup of it looks at, by lowercased
// slash path.
type Live map[string]indexEntry

// ReadLive reads what f's folder holds now. A folder that isn't there yet
// holds nothing.
func ReadLive(f Folder) (Live, error) {
	m := f.Matcher()
	l := Live{}
	err := filepath.WalkDir(f.Path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(f.Path, p)
		if rel == "." {
			return nil
		}
		if m.Ignored(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		l[strings.ToLower(filepath.ToSlash(rel))] = indexEntry{Size: info.Size(), MTime: info.ModTime().UnixNano()}
		return nil
	})
	if err != nil {
		if _, serr := os.Lstat(f.Path); errors.Is(serr, fs.ErrNotExist) {
			return Live{}, nil
		}
		return nil, err
	}
	return l, nil
}

// MatchesIndex reports whether l holds exactly the files the last backup of
// id from this PC saw: nothing changed here since then.
func (l Live) MatchesIndex(id string) bool {
	idx := loadIndex(id)
	if len(idx) == 0 || len(l) != len(idx) {
		return false
	}
	for k, e := range l {
		if i, ok := idx[k]; !ok || i.Size != e.Size || i.MTime != e.MTime {
			return false
		}
	}
	return true
}

// Same returns an error when f's folder no longer holds what l recorded.
func (l Live) Same(f Folder) error {
	if l == nil {
		return errors.New("the saves here couldn't be read")
	}
	now, err := ReadLive(f)
	if err != nil {
		return err
	}
	for k, e := range now {
		if o, ok := l[k]; !ok || o.Size != e.Size || o.MTime != e.MTime {
			return fmt.Errorf("%s changed here meanwhile", k)
		}
	}
	if len(now) != len(l) {
		return errors.New("a save here was removed meanwhile")
	}
	return nil
}

// LocalMatchesIndex reports whether f's files are exactly the ones its last
// backup from this PC saw: nothing changed here since then.
func LocalMatchesIndex(f Folder) bool {
	l, err := ReadLive(f)
	return err == nil && l.MatchesIndex(f.ID)
}

// TakeFiles copies the listed files from the backup of f into f's folder,
// the saves another PC backed up. Files that already hold the same are left
// alone, and files not on the list stay. All of it happens under the backup
// lock, so no backup or restore runs in between. Every file is first copied
// next to the folder's saves (into Syncthing's .stversions, which nothing
// syncs or backs up), and every file it replaces into a new pinned restore
// point; then precondition (nil = none) says whether it may still go ahead,
// and only then are they swapped in. If one can't be (the game has it open),
// the ones swapped already are put back. It returns how many files changed.
func TakeFiles(ctx context.Context, target string, f Folder, files []FileEntry, precondition func() error) (int, error) {
	if !paths.ValidID(f.ID) {
		return 0, fmt.Errorf("unsupported folder id %q", f.ID)
	}
	unlock, err := waitLock(ctx)
	if err != nil {
		return 0, err
	}
	defer unlock()
	m := f.Matcher()
	stage := filepath.Join(f.Path, ".stversions", ".syncer-take-"+time.Now().Format(stampFmt))
	keepStage := false // an original couldn't be put back: it's still in there
	defer func() {
		if !keepStage {
			_ = os.RemoveAll(stage)
		}
	}()

	type swap struct{ rel, staged, to, old, kept string }
	var swaps []swap
	for i, e := range files {
		rel := filepath.FromSlash(e.Path)
		if !safeRel(e.Path) || m.Ignored(rel) || strings.EqualFold(filepath.Base(rel), SteamMarker) {
			continue
		}
		from := filepath.Join(target, f.ID, rel)
		fi, err := os.Stat(from)
		if err != nil || fi.Size() != e.Size || !sameTime(fi.ModTime(), e.time()) {
			return 0, fmt.Errorf("%s changed in the backup meanwhile", e.Path)
		}
		to := filepath.Join(f.Path, rel)
		if same, err := fsx.SameContent(to, from); err == nil && same {
			continue
		}
		staged := filepath.Join(stage, "new", fmt.Sprint(i))
		if err := os.MkdirAll(filepath.Dir(staged), 0o755); err != nil {
			return 0, err
		}
		if err := copyFile(from, staged); err != nil {
			return 0, fmt.Errorf("%s: %w", e.Path, err)
		}
		_ = os.Chtimes(staged, fi.ModTime(), fi.ModTime())
		swaps = append(swaps, swap{rel: e.Path, staged: staged, to: to, old: filepath.Join(stage, "old", fmt.Sprint(i))})
	}
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if len(swaps) == 0 {
		return 0, nil
	}

	// The restore point is made once something goes into it, and dropped
	// again when the take doesn't happen after all.
	var point, stamp string
	keepPoint := false
	org := newOrigins(f.ID, f.Solo)
	defer func() {
		switch {
		case point == "":
		case keepPoint || keepStage:
			org.save(target, stamp)
		default:
			dropPoint(target, f.ID, stamp)
		}
	}()
	keepIn := func(rel string) string {
		if point == "" {
			var t time.Time
			point, t = newAsidePoint(target, f.ID)
			stamp = t.Format(stampFmt)
		}
		org.add(org.lookup(filepath.FromSlash(rel)))
		return filepath.Join(point, filepath.FromSlash(rel))
	}
	// Copied, not moved, and flushed: the point is under the backup folder,
	// maybe on another drive, and an original must be safe there before it
	// leaves its place.
	for i := range swaps {
		s := &swaps[i]
		fi, err := os.Stat(s.to)
		if err != nil {
			continue // new here: nothing to keep
		}
		if fi.IsDir() {
			return 0, fmt.Errorf("%s is a folder here", s.rel)
		}
		s.kept = keepIn(s.rel)
		if err := os.MkdirAll(filepath.Dir(s.kept), 0o755); err != nil {
			return 0, err
		}
		if err := copyFile(s.to, s.kept); err != nil {
			return 0, fmt.Errorf("could not keep %s first: %w", s.rel, err)
		}
		_ = os.Chtimes(s.kept, fi.ModTime(), fi.ModTime())
	}
	if precondition != nil {
		if err := precondition(); err != nil {
			return 0, err
		}
	}

	_ = os.MkdirAll(filepath.Join(stage, "old"), 0o755)
	var done []swap
	undo := func() {
		for i := len(done) - 1; i >= 0; i-- {
			s := done[i]
			if _, err := os.Stat(s.old); err == nil {
				// Rename replaces the new file with the original.
				if os.Rename(s.old, s.to) != nil {
					keepStage = true
				}
			} else {
				_ = os.Remove(s.to)
			}
		}
		if keepStage {
			logx.Printf("taking saves for %s failed and some originals couldn't be put back; they're in %s and in the restore point %s", f.Label, filepath.Join(stage, "old"), stamp)
		}
	}
	for _, s := range swaps {
		if _, err := os.Stat(s.to); err == nil {
			if err := os.Rename(s.to, s.old); err != nil {
				undo()
				return 0, fmt.Errorf("%s: %w (is the game running?)", s.rel, err)
			}
		} else if err := os.MkdirAll(filepath.Dir(s.to), 0o755); err != nil {
			undo()
			return 0, err
		}
		if err := os.Rename(s.staged, s.to); err != nil {
			if _, serr := os.Stat(s.old); serr == nil && os.Rename(s.old, s.to) != nil {
				keepStage = true
			}
			undo()
			return 0, fmt.Errorf("%s: %w", s.rel, err)
		}
		done = append(done, s)
	}
	keepPoint = true
	for _, s := range done {
		if _, err := os.Stat(s.old); err != nil {
			continue
		}
		if s.kept != "" {
			if same, err := fsx.SameContent(s.old, s.kept); err == nil && same {
				continue
			}
		}
		// Written after it was kept (or there only since): what was replaced
		// goes into the point as well, ahead of the earlier copy.
		if err := moveTo(s.old, keepIn(s.rel)); err != nil {
			keepStage = true
			logx.Printf("taking saves for %s: could not keep the replaced %s, it's in %s: %v", f.Label, s.rel, filepath.Join(stage, "old"), err)
		}
	}
	return len(done), nil
}

// indexFiles records what this run left in the backup, for the file list.
func indexFiles(idx map[string]indexEntry, names map[string]string) []FileEntry {
	out := make([]FileEntry, 0, len(idx))
	for k, e := range idx {
		rel := names[k]
		if rel == "" {
			continue
		}
		out = append(out, FileEntry{Path: filepath.ToSlash(rel), Size: e.Size, MTime: time.Unix(0, e.MTime).UnixMilli()})
	}
	return out
}
