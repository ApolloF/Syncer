package syncthing

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

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
	for _, tt := range []struct {
		a, b []int
		want int
	}{
		{[]int{1, 10}, []int{1, 9}, 1},
		{[]int{1, 9}, []int{1, 10}, -1},
		{[]int{2}, []int{2, 0, 0}, 0},
		{nil, []int{1}, -1},
	} {
		if got := compareVersions(tt.a, tt.b); got != tt.want {
			t.Errorf("compareVersions(%v, %v) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestExeCandidatesCoverOtherInstallers(t *testing.T) {
	local, roaming, pf := `L`, `R`, `PF`
	got := exeCandidates(local, roaming, pf)
	for _, want := range []string{
		filepath.Join("L", "Microsoft", "WinGet", "Links", "syncthing.exe"),
		filepath.Join("L", "Programs", "Syncthing", "syncthing.exe"),
		filepath.Join("PF", "Syncthing", "syncthing.exe"),
		filepath.Join("R", "SyncTrayzor", "syncthing.exe"),
		filepath.Join("PF", "SyncTrayzor", "syncthing.exe"),
	} {
		if !slices.Contains(got, want) {
			t.Errorf("%s is not looked at: %v", want, got)
		}
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
	got := exeCandidates(local, "", "")
	if !strings.Contains(got[1], "v1.10.0") || !strings.Contains(got[2], "v1.9.0") {
		t.Errorf("got %v", got)
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
