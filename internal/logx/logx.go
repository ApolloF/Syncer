// Package logx writes a small, self-rotating log to %APPDATA%\Syncer\syncer.log,
// and keeps panics from vanishing: the app has no console to print them to.
package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ApolloF/syncer/internal/paths"
)

const maxSize = 1 << 20 // rotate at 1 MiB

var mu sync.Mutex

// Path of the current log file. Tests write theirs to a temporary folder,
// never to this PC's real log.
func Path() string {
	if testing.Testing() {
		return filepath.Join(os.TempDir(), "syncer-test.log")
	}
	return filepath.Join(paths.AppDir(), "syncer.log")
}

// Printf appends a timestamped line to the log.
func Printf(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	p := Path()
	if fi, err := os.Stat(p); err == nil && fi.Size() > maxSize {
		_ = os.Rename(p, p+".1")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
}

// Tail returns the last n lines of the log.
func Tail(n int) []string {
	mu.Lock()
	defer mu.Unlock()
	b, err := os.ReadFile(Path())
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(b), "\r\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// CrashPath is the file the Go runtime appends a fatal crash's stack to.
func CrashPath() string {
	if testing.Testing() {
		return filepath.Join(os.TempDir(), "syncer-test-crash.log")
	}
	return filepath.Join(paths.AppDir(), "crash.log")
}

// RecordCrashes makes a crash the program can't recover from (a fatal error,
// or a panic nothing caught) leave its stack in CrashPath.
func RecordCrashes() {
	p := CrashPath()
	if fi, err := os.Stat(p); err == nil && fi.Size() > maxSize {
		_ = os.Rename(p, p+".1")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		Printf("crash log: %v", err)
		return
	}
	defer f.Close() // SetCrashOutput keeps its own handle
	if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
		Printf("crash log: %v", err)
	}
}

// Recover logs a panic with its stack instead of letting it end the program
// unseen. Use it as `defer logx.Recover("what")` at the top of a goroutine.
func Recover(where string) {
	if r := recover(); r != nil {
		Printf("panic in %s: %v\n%s", where, r, debug.Stack())
	}
}

// RecoverExit is Recover for a run that can't go on: it logs, then exits with
// an error code so a scheduled task doesn't report success.
func RecoverExit(where string) {
	if r := recover(); r != nil {
		Printf("panic in %s: %v\n%s", where, r, debug.Stack())
		os.Exit(1)
	}
}

// restartDelay and maxRestarts bound how a panicking loop is retried (tests
// shorten the delay).
var (
	restartDelay = 30 * time.Second
	maxRestarts  = 5
)

// Run calls fn, a loop that normally runs until the program quits. If it
// panics, the panic is logged and fn starts again after a pause; after too
// many panics the loop is given up on, so a bug can't spin forever.
func Run(name string, fn func()) {
	for i := 0; ; i++ {
		if !runOnce(name, fn) {
			return
		}
		if i >= maxRestarts {
			Printf("%s stopped after %d panics; restart Syncer to start it again", name, i+1)
			return
		}
		time.Sleep(restartDelay)
	}
}

// runOnce reports whether fn panicked.
func runOnce(name string, fn func()) (panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			Printf("panic in %s: %v\n%s", name, r, debug.Stack())
		}
	}()
	fn()
	return false
}
