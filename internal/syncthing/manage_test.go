package syncthing

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const bs = "\\"

func TestNewestFirstComparesVersionsAsNumbers(t *testing.T) {
	ps := []string{
		`P\syncthing-windows-amd64-v1.9.0\syncthing.exe`,
		`P\syncthing-windows-amd64-v1.27.9\syncthing.exe`,
		`P\syncthing-windows-amd64-v1.10.1\syncthing.exe`,
		`P\syncthing-windows-amd64-v2.0.0\syncthing.exe`,
	}
	got := newestFirst(ps)
	want := []string{ps[3], ps[1], ps[2], ps[0]}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCompareVersions(t *testing.T) {
	v := func(pre string, nums ...int) version { return version{nums: nums, pre: pre} }
	for _, tt := range []struct {
		a, b version
		want int
	}{
		{v("", 1, 10), v("", 1, 9), 1},
		{v("", 1, 9), v("", 1, 10), -1},
		{v("", 2), v("", 2, 0, 0), 0},
		{version{}, v("", 1), -1},
		{v("rc.1", 2, 0, 0), v("", 2, 0, 0), -1},
		{v("", 2, 0, 0), v("rc.1", 2, 0, 0), 1},
		{v("rc.2", 2, 0, 0), v("rc.1", 2, 0, 0), 1},
		{v("rc.1", 2, 1, 0), v("", 2, 0, 5), 1},
	} {
		if got := compareVersions(tt.a, tt.b); got != tt.want {
			t.Errorf("compareVersions(%+v, %+v) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestPrereleaseSortsBelowItsRelease(t *testing.T) {
	ps := []string{
		"P"+bs+"syncthing-windows-amd64-v2.0.0-rc.1"+bs+"syncthing.exe",
		"P"+bs+"syncthing-windows-amd64-v2.0.0"+bs+"syncthing.exe",
	}
	if got := newestFirst(ps); got[0] != ps[1] {
		t.Errorf("got %v", got)
	}
}

func TestExeCandidatesOnlyUseDefaultConfigInstalls(t *testing.T) {
	got := exeCandidates("L")
	want := []string{
		filepath.Join("L", "Microsoft", "WinGet", "Links", "syncthing.exe"),
		filepath.Join("L", "Programs", "Syncthing", "syncthing.exe"),
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExeCandidatesPutNewestWingetPackageFirst(t *testing.T) {
	local := t.TempDir()
	for _, v := range []string{"v1.9.0", "v1.10.0"} {
		dir := filepath.Join(local, "Microsoft", "WinGet", "Packages", "Syncthing.Syncthing_x", "syncthing-windows-amd64-"+v)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "syncthing.exe"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := exeCandidates(local)
	if !strings.Contains(got[1], "v1.10.0") || !strings.Contains(got[2], "v1.9.0") {
		t.Errorf("got %v", got)
	}
}

func TestFromWinget(t *testing.T) {
	local := filepath.Join("C:"+bs, "Users", "me", "AppData", "Local")
	for exe, want := range map[string]bool{
		filepath.Join(local, "Microsoft", "WinGet", "Links", "syncthing.exe"):                 true,
		filepath.Join(local, "Microsoft", "WinGet", "Packages", "S_x", "a", "syncthing.exe"): true,
		filepath.Join(local, "Programs", "Syncthing", "syncthing.exe"):                        false,
		filepath.Join("C:"+bs, "tools", "syncthing.exe"):                                      false,
	} {
		if got := fromWinget(exe, local); got != want {
			t.Errorf("fromWinget(%s) = %v, want %v", exe, got, want)
		}
	}
}

func TestInstallErrorHasCauseAndManualLink(t *testing.T) {
	msg := installError(errors.New(`exec: "winget": executable file not found in %PATH%`), "").Error()
	if !strings.Contains(msg, "executable file not found") || !strings.Contains(msg, ManualInstallURL) {
		t.Errorf("got %q", msg)
	}
	msg = installError(errors.New("exit status 1"), "Downloading\nNo package found matching input criteria.\n").Error()
	if !strings.Contains(msg, "No package found matching input criteria.") {
		t.Errorf("winget's own message missing: %q", msg)
	}
}
