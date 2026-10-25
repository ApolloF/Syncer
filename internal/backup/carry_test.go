package backup

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ApolloF/syncer/internal/paths"
)

func TestCopyInfoGivesTheNewIDItsOwnInfoAndFileList(t *testing.T) {
	target := t.TempDir()
	f := Folder{ID: "old", Label: "Old Game", Path: filepath.Join(paths.Root(paths.Documents), "Old Game")}
	hash, err := writeFiles(target, f.ID, []FileEntry{{Path: "a.sav", Size: 1, MTime: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	newest := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	writeInfo(target, f, newest, "", hash, 30)

	if err := CopyInfo(target, "old", "new--pc"); err != nil {
		t.Fatal(err)
	}
	ins := ReadInfos(target, "new--pc")
	if len(ins) != 1 || !ins[0].Mine() || ins[0].Label != "Old Game" || !ins[0].Newest.Equal(newest) {
		t.Fatalf("copied info: %+v", ins)
	}
	files, err := ReadFiles(target, ins[0])
	if err != nil || len(files) != 1 || files[0].Path != "a.sav" {
		t.Fatalf("copied file list: %v, %v", files, err)
	}
	if len(ReadInfos(target, "old")) != 1 {
		t.Error("the source info was changed")
	}
}

func TestCopyInfoWithoutInfoDoesNothing(t *testing.T) {
	target := t.TempDir()
	if err := CopyInfo(target, "missing", "x"); err != nil {
		t.Fatal(err)
	}
	if len(ReadInfos(target, "x")) != 0 {
		t.Error("info made up")
	}
	if CopyInfo(target, "../a", "x") == nil {
		t.Error("invalid id accepted")
	}
}
