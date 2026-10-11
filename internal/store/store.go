// Package store persists Syncer's local settings and run state as JSON.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"

	"github.com/ApolloF/syncer/internal/logx"
	"github.com/ApolloF/syncer/internal/paths"
)

// Settings are user choices local to this PC.
type Settings struct {
	Theme         string `json:"theme"`         // system | light | dark
	BackupEnabled bool   `json:"backupEnabled"` // scheduled Drive backup on/off
	BackupRoot    string `json:"backupRoot"`    // override for the Drive folder; "" = auto-detect
	DriveRoot     string `json:"driveRoot"`     // chosen "My Drive" when several accounts are signed in; "" = first found
	// BackupBackend is "google" when Syncer signs in to Google itself and
	// syncs the backup with Drive (no Google Drive for desktop needed).
	BackupBackend string          `json:"backupBackend,omitempty"`
	IntervalHours int             `json:"intervalHours"` // backup schedule
	KeepDays      int             `json:"keepDays"`      // how long old versions are kept
	NoBackup      map[string]bool `json:"noBackup"`      // folder ids excluded from backup
	Ignored       map[string]bool `json:"ignored"`       // folder ids never auto-added from other PCs
	// Treat games confirmed to be in Steam Cloud like any other: list them and
	// add them automatically. (JSON name kept from when it only showed them.)
	IncludeSteamCloud bool            `json:"showSteamCloud"`
	AutoAdd           bool            `json:"autoAdd"`      // sync newly detected games automatically
	AutoAddMaxGB      int             `json:"autoAddMaxGB"` // skip bigger folders when auto-adding; -1 = no limit
	Dismissed         map[string]bool `json:"dismissed"`    // portable paths the user stopped syncing; never auto-added again
	CloseToTray       bool            `json:"closeToTray"`  // closing the window keeps Syncer running in the tray
	StartAtLogin      bool            `json:"startAtLogin"` // start hidden in the tray when signing in to Windows
	Migrated          bool            `json:"migrated"`     // legacy script setup adopted

	PauseWhileGaming bool                   `json:"pauseWhileGaming"` // hold automatic backups while a game runs
	InstalledOnly    bool                   `json:"installedOnly"`    // adopt folders from other PCs only for installed games
	SyncDisabled     bool                   `json:"syncDisabled"`     // set by "Undo everything": leave Syncthing alone
	BackupOnly       map[string]LocalFolder `json:"backupOnly"`       // backed up here, not synced (by folder id); with NoBackup too: "off"

	PausedUntil   time.Time `json:"pausedUntil,omitzero"` // syncing and automatic backups are paused until then
	Notify        bool      `json:"notify"`               // Windows notifications about problems
	NoUpdateCheck bool      `json:"noUpdateCheck"`        // don't look for new Syncer releases
	NoAutoUpdate  bool      `json:"noAutoUpdate"`         // don't install new releases by themselves
	// NoCloudPull: don't take newer saves from another PC's backup by
	// yourself (see cloudpull.go); "Get it" still does.
	NoCloudPull bool `json:"noCloudPull"`
	// NoHoldWhilePlaying: keep syncing while a game runs (see session.go).
	NoHoldWhilePlaying bool `json:"noHoldWhilePlaying"`

	// Exclude lists, per save folder (keyed like Dismissed, so it survives a
	// game switching between synced and backup-only), file patterns that are
	// neither synced nor backed up on this PC.
	Exclude map[string][]string `json:"exclude,omitempty"`

	// Mods: FindMods lists mod managers' folders (Vortex) among the games
	// found on this PC, to be synced by hand. AutoAddMods (experimental)
	// syncs them without asking, up to ModsMaxGB (-1 = no limit).
	// SyncDeployedMods (experimental) also offers the mods deployed in a
	// game's own folder. ShareVortexMods (experimental) shares Vortex's
	// mod list (which mods are installed and enabled, with their details)
	// between PCs, so Vortex manages the same synced mods on each of them.
	FindMods         bool `json:"findMods"`
	AutoAddMods      bool `json:"autoAddMods"`
	SyncDeployedMods bool `json:"syncDeployedMods"`
	ShareVortexMods  bool `json:"shareVortexMods"`
	ModsMaxGB        int  `json:"modsMaxGB"`
	// ModsChanged is when the options above last changed, here or on
	// another PC: they are the same on every PC, and the newest change wins
	// (see meta.ModSettings).
	ModsChanged time.Time `json:"modsChanged,omitzero"`
	// Mods are the synced (or backup-only) folders that are mod folders, by
	// folder id: how they are described to other PCs.
	Mods map[string]ModFolder `json:"mods,omitempty"`
	// Accounts shows accounts (separate saves per person) on this PC. It
	// can only be turned off while no game is split per account.
	Accounts bool `json:"accounts,omitempty"`
	// Launchers are synced folders that hold a game launcher's own data
	// (Seaglass's playtime, achievements and settings), by folder id: the
	// launcher that asked for it. They aren't games.
	Launchers map[string]string `json:"launchers,omitempty"`
}

// ModFolder is a mod manager's folder that Syncer syncs.
type ModFolder struct {
	Kind     string `json:"kind"`    // mods.Kind*
	Manager  string `json:"manager"` // "vortex"
	Game     string `json:"game"`    // the manager's game id
	GameName string `json:"gameName"`
	Root     string `json:"root"` // portable mod root, e.g. "vortex:skyrimse"
	Rel      string `json:"rel"`
	// Role of this PC for deployed mods: "source" (its deployment is sent
	// to the other PCs) or "receiver".
	Role string `json:"role,omitempty"`
	// SizeGB is the size last published (Syncthing can't tell while paused).
	SizeGB int `json:"sizeGB,omitempty"`
}

// Paused reports whether syncing and automatic backups are paused right now.
func (s Settings) Paused() bool { return time.Now().Before(s.PausedUntil) }

// LocalFolder is a save folder known only to this PC.
type LocalFolder struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Path  string `json:"path"`
	// SyncID is the shared id it had while synced, so syncing it again
	// rejoins the same folder on the other PCs.
	SyncID string `json:"syncID,omitempty"`
	// CopiedFrom is the backup its history was copied from when it was added
	// from "Other backups", so that backup isn't offered again.
	CopiedFrom string `json:"copiedFrom,omitempty"`
}

// BackupRun is the outcome of the last backup.
type BackupRun struct {
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
	OK       bool      `json:"ok"`
	Folders  int       `json:"folders"`
	Copied   int       `json:"copied"`
	Versions int       `json:"versions"`
	Bytes    int64     `json:"bytes"`
	Errors   []string  `json:"errors"`
	// Held: files left as they are in the backup because another PC backed
	// up newer saves there (see backup.mirror), per game.
	Held   []string `json:"held,omitempty"`
	Target string   `json:"target"`
	// NotPruned says why old versions weren't thinned or expired (the
	// PC's clock couldn't be checked, or is off); empty when they were.
	NotPruned string   `json:"notPruned,omitempty"`
	Backed    []string `json:"-"` // ids of the folders backed up without errors
}

// State is machine-written status.
type State struct {
	LastBackup  *BackupRun `json:"lastBackup"`
	LastSuccess time.Time  `json:"lastSuccess,omitzero"` // last backup that finished without errors
	// Protected: when the existing files of a folder (id|path) were saved as
	// a restore point before it started syncing.
	Protected map[string]time.Time `json:"protected,omitempty"`
	// PausedFolders are the Syncthing folders a pause stopped, so resuming
	// leaves folders paused by hand alone.
	PausedFolders []string `json:"pausedFolders,omitempty"`
	// GamePaused are the Syncthing folders paused while a game runs, so they
	// resume when it exits (or when Syncer next starts, if it quit first).
	GamePaused []string `json:"gamePaused,omitempty"`
	// GameHeld is when the window last confirmed it holds them.
	GameHeld time.Time `json:"gameHeld,omitzero"`
	// GoogleSynced is when the backup was last synced with Google Drive
	// through Syncer's own sign-in; GoogleError why the last try failed.
	GoogleSynced time.Time `json:"googleSynced,omitzero"`
	GoogleError  string    `json:"googleError,omitempty"`
	Update       *Update   `json:"update,omitempty"` // newest release seen
	// Notified remembers which problems were already reported (key -> when).
	Notified map[string]time.Time `json:"notified,omitempty"`
	// NoticesDone are the one-time notices in Overview the user dismissed.
	NoticesDone map[string]bool `json:"noticesDone,omitempty"`
	// FolderBackups is when each folder (by id) was last backed up without errors.
	FolderBackups map[string]time.Time `json:"folderBackups,omitempty"`
	// BackgroundTask describes the background task as last registered, so it
	// is only registered again when something about it changed.
	BackgroundTask string `json:"backgroundTask,omitempty"`
	// ModHeld are mod folders paused while their mod manager is open, to be
	// resumed when it closes.
	ModHeld []string `json:"modHeld,omitempty"`
	// ModSync is the state of each deployed-mods folder (by id).
	ModSync map[string]ModSyncState `json:"modSync,omitempty"`
	// Decisions are the recent choices between two versions of a save,
	// newest last, so each can be changed later.
	Decisions []Decision `json:"decisions,omitempty"`
	// Rejoin are launcher data folders (by id) taken out of Syncthing to be
	// added back with a fresh index, until that has worked.
	Rejoin map[string]LocalFolder `json:"rejoin,omitempty"`
	// Repaired is when each launcher data folder (by id) was last repaired,
	// so a repair that didn't take isn't repeated over and over.
	Repaired map[string]time.Time `json:"repaired,omitempty"`
	// HistoryCopies are account folders (by id) still to get the backup
	// history of the game they were split from (its id), until that has
	// worked.
	HistoryCopies map[string]string `json:"historyCopies,omitempty"`
	// BackupFrom maps an account folder (by id) to the backup of the game it
	// was split from (or merged back out of), whose unchanged files its own
	// backup takes over instead of uploading them again. Dropped once that
	// backup is gone.
	BackupFrom map[string]string `json:"backupFrom,omitempty"`
	// SyncerPCs are the linked devices known to run Syncer (by device id).
	// Syncthing may be linked to other devices too, for other things.
	SyncerPCs map[string]bool `json:"syncerPCs,omitempty"`
	// JoinRequests are linked devices that offer Syncer's shared settings
	// and wait for the user to allow them as one of their PCs; IgnoredPCs
	// are those the user turned down.
	JoinRequests map[string]bool `json:"joinRequests,omitempty"`
	IgnoredPCs   map[string]bool `json:"ignoredPCs,omitempty"`
	// SyncerFolders and OtherFolders are the Syncthing folders (by id)
	// found to be Syncer's, or something else's (see meta.Reconcile).
	SyncerFolders map[string]bool `json:"syncerFolders,omitempty"`
	OtherFolders  map[string]bool `json:"otherFolders,omitempty"`
}

// Decision is a choice between two versions of a save file.
type Decision struct {
	Folder string    `json:"folder"`
	Rel    string    `json:"rel"` // the save file, relative to the folder
	At     time.Time `json:"at"`
	Kept   string    `json:"kept,omitempty"`  // the PC the version kept was from, if known
	Other  string    `json:"other,omitempty"` // … and the version put aside
	Put    string    `json:"put"`             // where the version put aside is
	// OtherMod is the modification time of the version put aside: that
	// exact version is never taken back over the choice (from another PC's
	// backup, or held in the backup over the version kept).
	OtherMod time.Time `json:"otherMod,omitzero"`
}

// Rejected is a version of a save file that was decided against.
type Rejected struct {
	Rel string `json:"rel"` // forward slashes
	Mod int64  `json:"mod"` // its modification time, unix seconds
}

// RejectedIn lists the versions of a folder's files decided against here.
func (st State) RejectedIn(id string) []Rejected {
	var out []Rejected
	for _, d := range st.Decisions {
		if d.Folder == id && !d.OtherMod.IsZero() {
			out = append(out, Rejected{Rel: filepath.ToSlash(d.Rel), Mod: d.OtherMod.Unix()})
		}
	}
	return out
}

// IsRejected reports whether the file rel, modified at mod, is one of
// rejected (the same file, within two seconds: file systems differ).
func IsRejected(rejected []Rejected, rel string, mod time.Time) bool {
	rel = strings.ToLower(filepath.ToSlash(rel))
	for _, r := range rejected {
		if strings.ToLower(r.Rel) == rel && abs64(mod.Unix()-r.Mod) <= 2 {
			return true
		}
	}
	return false
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// ModSyncState tracks updates of a deployed-mods folder.
type ModSyncState struct {
	Phase      string    `json:"phase"`                // idle | pending | applying | held
	AppliedGen int64     `json:"appliedGen"`           // source inventory generation applied here
	SourceGen  int64     `json:"sourceGen,omitempty"`  // source: generation published
	SourceHash string    `json:"sourceHash,omitempty"` // source: what the published inventory was built from
	Since      int64     `json:"since,omitempty"`      // source: when this PC became the source (unix ns)
	Held       string    `json:"held,omitempty"`       // why it is held
	HeldBy     string    `json:"heldBy,omitempty"`     // what held it: off, unreadable, handover, check, interrupted
	LastAudit  time.Time `json:"lastAudit,omitzero"`
	Started    time.Time `json:"started,omitzero"` // when the update being applied started
	PID        int       `json:"pid,omitempty"`    // the process applying it
	// Source is the PC a receiver takes updates from (its folder is shared
	// with that PC only); Handover is another PC that claims to send them
	// now, waiting for the user to agree.
	Source   string `json:"source,omitempty"`
	Handover string `json:"handover,omitempty"`
}

// Update is the newest Syncer release found on GitHub.
type Update struct {
	Checked time.Time `json:"checked"`
	Latest  string    `json:"latest"` // tag, e.g. "v0.7.0"
	URL     string    `json:"url"`    // release page
}

var mu sync.Mutex

func defaults() Settings {
	return Settings{Theme: "system", BackupEnabled: true, IntervalHours: 3, KeepDays: 30, AutoAdd: true, AutoAddMaxGB: 1,
		PauseWhileGaming: true, Notify: true, NoBackup: map[string]bool{}, Ignored: map[string]bool{}, Dismissed: map[string]bool{},
		BackupOnly: map[string]LocalFolder{}, Exclude: map[string][]string{}, ModsMaxGB: 20, Mods: map[string]ModFolder{}}
}

// LoadSettings reads settings, falling back to defaults.
func LoadSettings() Settings {
	s, _ := loadSettings(false)
	return s
}

// loadSettings is LoadSettings; locked says the caller holds the settings
// lock. A file that can't be read is an error, and a caller about to save
// must stop on it: defaults would replace the real settings.
func loadSettings(locked bool) (Settings, error) {
	s := defaults()
	err := loadFile("settings.json", &s, locked)
	if err != nil && !locked {
		lastGood(filepath.Join(paths.AppDir(), "settings.json"), &s)
	}
	if s.NoBackup == nil {
		s.NoBackup = map[string]bool{}
	}
	if s.Ignored == nil {
		s.Ignored = map[string]bool{}
	}
	if s.Dismissed == nil {
		s.Dismissed = map[string]bool{}
	}
	if s.BackupOnly == nil {
		s.BackupOnly = map[string]LocalFolder{}
	}
	if s.Exclude == nil {
		s.Exclude = map[string][]string{}
	}
	if s.Mods == nil {
		s.Mods = map[string]ModFolder{}
	}
	if s.Launchers == nil {
		s.Launchers = map[string]string{}
	}
	if s.ModsMaxGB == 0 || s.ModsMaxGB < -1 {
		s.ModsMaxGB = 20
	}
	if !s.FindMods {
		s.AutoAddMods, s.SyncDeployedMods, s.ShareVortexMods = false, false, false
	}
	if s.IntervalHours <= 0 {
		s.IntervalHours = 3
	}
	if s.KeepDays <= 0 {
		s.KeepDays = 30
	}
	if s.AutoAddMaxGB == 0 || s.AutoAddMaxGB < -1 {
		s.AutoAddMaxGB = 1
	}
	return s, err
}

// SaveSettings writes settings atomically.
func SaveSettings(s Settings) error { return save("settings.json", s) }

// UpdateSettings applies fn to the stored settings under a lock and saves.
func UpdateSettings(fn func(*Settings)) (Settings, error) {
	mu.Lock()
	defer mu.Unlock()
	defer fileLock("settings")()
	s, err := loadSettings(true)
	if err != nil {
		return s, err
	}
	fn(&s)
	return s, save("settings.json", s)
}

// LoadState reads run state.
func LoadState() State {
	st, _ := loadState(false)
	return st
}

// loadState is LoadState; locked says the caller holds the state lock. A file
// that can't be read is an error (see loadSettings).
func loadState(locked bool) (State, error) {
	var st State
	err := loadFile("state.json", &st, locked)
	if err != nil && !locked {
		lastGood(filepath.Join(paths.AppDir(), "state.json"), &st)
	}
	return st, err
}

// SaveState writes run state atomically.
func SaveState(st State) error { return save("state.json", st) }

var stateMu sync.Mutex

// UpdateState applies fn to the stored state under a lock and saves. It
// returns an error if the state couldn't be read or saved, so callers that
// must not go on without the change (recording before pausing a folder) can
// tell.
func UpdateState(fn func(*State)) error {
	stateMu.Lock()
	defer stateMu.Unlock()
	defer fileLock("state")()
	st, err := loadState(true)
	if err != nil {
		logx.Printf("not updating state.json: %v", err)
		return err
	}
	fn(&st)
	if err := SaveState(st); err != nil {
		logx.Printf("saving state.json: %v", err)
		return err
	}
	return nil
}

// FileLock takes the named cross-process lock (see fileLock).
func FileLock(name string) (unlock func()) { return fileLock(name) }

// fileLock also keeps the other Syncer process (the window and the background
// task run separately) from updating the same file at the same time, which
// would lose one of the two changes. The OS drops the lock if a process dies.
func fileLock(name string) (unlock func()) {
	p, err := windows.UTF16PtrFromString(filepath.Join(paths.AppDir(), name+".lock"))
	if err != nil {
		return func() {}
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return func() {} // better an unlocked update than none
	}
	ol := new(windows.Overlapped)
	if err := windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, ol); err != nil {
		windows.CloseHandle(h)
		return func() {}
	}
	return func() {
		_ = windows.UnlockFileEx(h, 0, 1, 0, ol)
		windows.CloseHandle(h)
	}
}

// loadFile fills v from the named file in the app dir; locked says the caller
// holds the file's lock (see fileLock). A missing file leaves v as it is (a
// first run). A file that can't be read for any other reason (the other Syncer
// process is replacing it, a virus scanner holds it) also leaves v alone, and
// no file is touched. Only a file that was read and is not valid JSON (an
// empty one after a power cut, say) is set aside, never overwritten, and
// replaced by the last good copy (name.bak). That recovery runs under the
// lock and reads the file again first: another process may have done it
// already, or be doing it now, which can show up as a missing file.
func loadFile(name string, v any, locked bool) error {
	p := filepath.Join(paths.AppDir(), name)
	if b, err := readRetry(p); err == nil && json.Valid(b) {
		decode(b, v)
		remember(p, b)
		return nil
	} else if errors.Is(err, os.ErrNotExist) {
		if _, berr := os.Stat(p + ".bak"); berr != nil {
			return nil // a first run
		}
	} else if err != nil {
		warnOnce(name, err)
		return err
	}
	if !locked {
		defer fileLock(strings.TrimSuffix(name, ".json"))()
	}
	b, err := readRetry(p)
	switch {
	case err == nil && json.Valid(b):
		decode(b, v) // recovered by the other process meanwhile
		remember(p, b)
		return nil
	case err == nil:
		if !setAside(p, errors.New("not valid JSON")) {
			// Never write over a file that is still there.
			return fmt.Errorf("%s is damaged and could not be set aside", name)
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		warnOnce(name, err)
		return err
	}
	bak, berr := os.ReadFile(p + ".bak")
	if berr == nil && json.Valid(bak) {
		decode(bak, v)
		remember(p, bak)
		logx.Printf("%s: restored from %s.bak", name, name)
		if werr := writeAtomic(p, bak); werr != nil {
			logx.Printf("%s: writing the restored copy: %v", name, werr)
		}
		return nil
	}
	logx.Printf("%s: no usable backup copy (%v); starting from defaults", name, berr)
	return nil
}

// good holds, per file path, what this process last read from it successfully.
var good struct {
	sync.Mutex
	m map[string][]byte
}

func remember(path string, b []byte) {
	good.Lock()
	defer good.Unlock()
	if good.m == nil {
		good.m = map[string][]byte{}
	}
	good.m[path] = b
}

// lastGood fills v from the last good read of the file, if there was one: a
// file that can't be read just now is better answered with what it held a
// moment ago than with defaults. Without one, v keeps its defaults.
func lastGood(path string, v any) {
	good.Lock()
	b := good.m[path]
	good.Unlock()
	if b != nil {
		decode(b, v)
	}
}

// readFile is os.ReadFile (tests make it fail).
var readFile = os.ReadFile

// readRetry is os.ReadFile, tried again for a moment when the file can't be
// opened (but does exist): the other Syncer process or a scanner has it, or
// is replacing it.
func readRetry(p string) ([]byte, error) {
	b, err := readFile(p)
	for wait := 10 * time.Millisecond; err != nil && !errors.Is(err, os.ErrNotExist) && wait <= 160*time.Millisecond; wait *= 2 {
		time.Sleep(wait)
		b, err = readFile(p)
	}
	return b, err
}

// decode fills v from valid JSON. A field of another type than expected (a
// newer Syncer changed it) is skipped; the rest is kept.
func decode(b []byte, v any) { _ = json.Unmarshal(b, v) }

var warned sync.Map // file name -> true once logged

func warnOnce(name string, err error) {
	if _, dup := warned.LoadOrStore(name, true); !dup {
		logx.Printf("%s can't be read right now (%v); using defaults for now", name, err)
	}
}

// setAside renames an unusable file to name.corrupt-<time>, so what is in it
// stays available and a later save can't overwrite it. It reports whether
// that worked.
func setAside(p string, cause error) bool {
	aside := p + ".corrupt-" + time.Now().Format("20060102-150405.000")
	if err := renameRetry(p, aside); err != nil {
		logx.Printf("%s is unusable (%v) and could not be set aside: %v", filepath.Base(p), cause, err)
		return false
	}
	logx.Printf("%s is unusable (%v); kept as %s", filepath.Base(p), cause, filepath.Base(aside))
	return true
}

// save writes the named file, first keeping the current one as name.bak if it
// is good: a settings file lost to a crash or power cut can then be restored
// by load.
func save(name string, v any) error {
	p := filepath.Join(paths.AppDir(), name)
	if old, err := os.ReadFile(p); err == nil && json.Valid(old) {
		if err := writeAtomic(p+".bak", old); err != nil {
			logx.Printf("%s: keeping a backup copy: %v", name, err)
		}
	}
	return WriteJSON(p, v)
}

// WriteJSON writes v to path via a uniquely named temp file + rename, so
// readers never see a half-written file and the window and the background
// task never clobber each other's temp file.
func WriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, b)
}

// writeAtomic is WriteJSON for bytes. The temp file is flushed to disk before
// the rename: otherwise a power cut can leave the renamed file empty. The
// folder isn't flushed too (Windows can't fsync a directory); NTFS journals
// the rename itself.
func writeAtomic(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = renameRetry(f.Name(), path)
	}
	if err != nil {
		_ = os.Remove(f.Name())
	}
	return err
}

var rename = os.Rename

// renameRetry is os.Rename, tried again a few times when it is refused with
// access denied or a sharing violation: on Windows that is what replacing a
// file another process has open (the other Syncer, a scanner) looks like, and
// it passes within moments.
func renameRetry(from, to string) error {
	var err error
	for i, wait := 0, 10*time.Millisecond; i < 6; i, wait = i+1, wait*2 {
		if err = rename(from, to); err == nil || !(errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SHARING_VIOLATION)) {
			return err
		}
		time.Sleep(wait)
	}
	return err
}
