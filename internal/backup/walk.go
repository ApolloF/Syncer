package backup

import (
	"io/fs"
	"os"
	"path/filepath"
)

// readDir lists a folder; a variable so tests can make one fail or return
// entries a disk wouldn't.
var readDir = os.ReadDir

// walkTree is filepath.WalkDir over readDir. A folder that can't be listed is
// reported to fn, the root included.
func walkTree(root string, fn fs.WalkDirFunc) error {
	fi, err := os.Lstat(root)
	if err != nil {
		return fn(root, nil, err)
	}
	err = walkEntry(root, fs.FileInfoToDirEntry(fi), fn)
	if err == filepath.SkipDir || err == filepath.SkipAll {
		return nil
	}
	return err
}

func walkEntry(path string, d fs.DirEntry, fn fs.WalkDirFunc) error {
	if err := fn(path, d, nil); err != nil || !d.IsDir() {
		if err == filepath.SkipDir && d.IsDir() {
			err = nil
		}
		return err
	}
	ents, err := readDir(path)
	if err != nil {
		if err = fn(path, d, err); err == filepath.SkipDir {
			err = nil
		}
		return err
	}
	for _, e := range ents {
		if err := walkEntry(filepath.Join(path, e.Name()), e, fn); err != nil {
			if err == filepath.SkipDir {
				break
			}
			return err
		}
	}
	return nil
}
