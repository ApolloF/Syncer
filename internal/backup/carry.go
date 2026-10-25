package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ApolloF/syncer/internal/paths"
	"github.com/ApolloF/syncer/internal/store"
)

// CopyInfo gives this PC's info file and file list of from to the folder id
// to, so a game that stops syncing and gets a new backup id still shows when
// it was last backed up. Nothing is done when this PC has no info for from.
func CopyInfo(target, from, to string) error {
	if target == "" || !paths.ValidID(from) || !paths.ValidID(to) {
		return errors.New("unsupported folder id")
	}
	in, err := readInfo(infoPath(target, from, hostKey))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	unlock, err := lock()
	if err != nil {
		return err
	}
	defer unlock()
	if b, err := os.ReadFile(filesPath(target, from, hostKey)); err == nil && in.Files != "" {
		var l fileList
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) == in.Files && json.Unmarshal(b, &l) == nil {
			in.Files, err = writeFiles(target, to, l.Files)
			if err != nil {
				return err
			}
		} else {
			in.Files = "" // not trustworthy: the next backup writes a new list
		}
	} else {
		in.Files = ""
	}
	in.ID = to
	in.Rejected = store.LoadState().RejectedIn(to)
	out := infoPath(target, to, hostKey)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return store.WriteJSON(out, in)
}
