package syncthing

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ApolloF/syncer/internal/paths"
)

const createNoWindow = 0x08000000

// ManualInstallURL is where to get Syncthing when winget can't install it.
const ManualInstallURL = "https://syncthing.net/downloads/"

// FindExe locates syncthing.exe: Syncer's own winget install first, then
// other installs Syncer can use as they are, then PATH.
func FindExe() string {
	for _, c := range exeCandidates(paths.Root(paths.Local), paths.Root(paths.Roaming), os.Getenv("ProgramFiles")) {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	if p, err := exec.LookPath("syncthing.exe"); err == nil {
		return p
	}
	return ""
}

// exeCandidates lists where an installed syncthing.exe is expected, best
// first:
//   - winget: the link in WinGet\Links, then the package folders (newest version first)
//   - "Syncthing Windows Setup" (Bill-Stewart): %LOCALAPPDATA%\Programs\Syncthing for
//     the current user, %ProgramFiles%\Syncthing for all users
//   - SyncTrayzor: the copy it runs, %APPDATA%\SyncTrayzor, then the one its
//     installer ships in %ProgramFiles%\SyncTrayzor
func exeCandidates(local, roaming, programFiles string) []string {
	cands := []string{filepath.Join(local, "Microsoft", "WinGet", "Links", "syncthing.exe")}
	m, _ := filepath.Glob(filepath.Join(local, "Microsoft", "WinGet", "Packages", "Syncthing.Syncthing_*", "syncthing-*", "syncthing.exe"))
	cands = append(cands, newestFirst(m)...)
	cands = append(cands, filepath.Join(local, "Programs", "Syncthing", "syncthing.exe"))
	if programFiles != "" {
		cands = append(cands, filepath.Join(programFiles, "Syncthing", "syncthing.exe"))
	}
	if roaming != "" {
		cands = append(cands, filepath.Join(roaming, "SyncTrayzor", "syncthing.exe"))
	}
	if programFiles != "" {
		cands = append(cands, filepath.Join(programFiles, "SyncTrayzor", "syncthing.exe"))
	}
	return cands
}

var versionPattern = regexp.MustCompile(`v(\d+(?:\.\d+)*)`)

// newestFirst sorts paths by the version number in them, highest first. The
// numbers are compared as numbers (1.10 is newer than 1.9, which a plain
// string sort gets wrong).
func newestFirst(ps []string) []string {
	out := slices.Clone(ps)
	slices.SortStableFunc(out, func(a, b string) int { return -compareVersions(pathVersion(a), pathVersion(b)) })
	return out
}

// pathVersion is the version in the last "syncthing-…" folder name of p, e.g.
// [1 27 9] for syncthing-windows-amd64-v1.27.9\syncthing.exe.
func pathVersion(p string) []int {
	m := versionPattern.FindStringSubmatch(filepath.Base(filepath.Dir(p)))
	if m == nil {
		return nil
	}
	var nums []int
	for _, part := range strings.Split(m[1], ".") {
		n, _ := strconv.Atoi(part)
		nums = append(nums, n)
	}
	return nums
}

// compareVersions returns -1, 0 or 1; a missing part counts as 0.
func compareVersions(a, b []int) int {
	for i := 0; i < max(len(a), len(b)); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return cmp.Compare(x, y)
		}
	}
	return 0
}

// Hidden returns a command that runs without flashing a console window.
func Hidden(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd
}

// HiddenContext is Hidden for a command that must be cancelable, e.g. a
// winget install/uninstall the user can abort.
func HiddenContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd
}

// Start launches Syncthing in the background and waits for its API.
func Start(ctx context.Context) error {
	exe := FindExe()
	if exe == "" {
		return errors.New("syncthing.exe not found")
	}
	cmd := Hidden(exe, "--no-browser", "--no-restart")
	cmd.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	return WaitReady(ctx, 30*time.Second)
}

// WaitReady polls until the API answers or timeout passes.
func WaitReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if c, err := New(); err == nil {
			if _, err := c.Status(ctx); err == nil {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return ErrNotRunning
}

// winget returns winget's own app alias under %LOCALAPPDATA%, so a program
// named winget earlier on PATH isn't run instead; PATH only as a fallback.
func winget() string {
	p := filepath.Join(paths.Root(paths.Local), "Microsoft", "WindowsApps", "winget.exe")
	if _, err := os.Lstat(p); err == nil {
		return p
	}
	return "winget"
}

// Install installs Syncthing via winget (per-user, silent). The error says
// what went wrong and where to get Syncthing by hand.
func Install(ctx context.Context) error {
	cmd := HiddenContext(ctx, winget(), "install", "--id", "Syncthing.Syncthing", "-e", "--source", "winget", "--silent",
		"--accept-package-agreements", "--accept-source-agreements", "--disable-interactivity")
	out, err := cmd.CombinedOutput()
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil && FindExe() == "" {
		return installError(err, string(out))
	}
	return nil
}

// installError is the message for a failed winget install: winget's own error
// (it is empty when winget could not even start), a hint, and the manual link.
func installError(err error, output string) error {
	cause := err.Error()
	if line := lastLine(output); line != "" {
		cause = line + " (" + cause + ")"
	}
	return fmt.Errorf("Syncer couldn't install Syncthing automatically: %s. Windows' App Installer (winget) may be missing or out of date. You can install Syncthing yourself from %s, then open Syncer again.", cause, ManualInstallURL)
}

// Uninstall removes Syncthing via winget (per-user, silent).
func Uninstall(ctx context.Context) error {
	cmd := HiddenContext(ctx, winget(), "uninstall", "--id", "Syncthing.Syncthing", "-e", "--silent",
		"--disable-interactivity", "--accept-source-agreements")
	out, err := cmd.CombinedOutput()
	if err != nil && FindExe() != "" {
		return errors.New("winget uninstall failed: " + lastLine(string(out)))
	}
	return nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
