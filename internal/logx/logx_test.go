package logx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ApolloF/syncer/internal/paths"
)

func TestRunRestartsAfterPanic(t *testing.T) {
	restartDelay = time.Millisecond
	t.Cleanup(func() { restartDelay = 30 * time.Second })
	calls := 0
	Run("test loop", func() {
		calls++
		if calls < 3 {
			panic("boom")
		}
	})
	if calls != 3 {
		t.Fatalf("loop ran %d times, want 3 (two panics, then a clean return)", calls)
	}
	b, _ := os.ReadFile(Path())
	if !strings.Contains(string(b), "panic in test loop: boom") {
		t.Error("the panic was not logged")
	}
}

func TestRunGivesUp(t *testing.T) {
	restartDelay = time.Millisecond
	t.Cleanup(func() { restartDelay = 30 * time.Second })
	calls := 0
	Run("broken loop", func() { calls++; panic("again") })
	if calls != maxRestarts+1 {
		t.Fatalf("loop ran %d times, want %d", calls, maxRestarts+1)
	}
}

func TestRecoverLogsPanic(t *testing.T) {
	func() {
		defer Recover("a goroutine")
		panic("oops")
	}()
	b, _ := os.ReadFile(Path())
	if !strings.Contains(string(b), "panic in a goroutine: oops") {
		t.Error("the panic was not logged")
	}
}

func TestTestsNeverWriteToTheRealLog(t *testing.T) {
	real := filepath.Join(paths.AppDir(), "syncer.log")
	before, beforeErr := os.Stat(real)
	if Path() == real || CrashPath() == filepath.Join(paths.AppDir(), "crash.log") {
		t.Fatalf("tests log to the real files: %s, %s", Path(), CrashPath())
	}
	Printf("logx test line")
	after, afterErr := os.Stat(real)
	if (beforeErr == nil) != (afterErr == nil) || (beforeErr == nil && (after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()))) {
		t.Error("the real syncer.log changed")
	}
}
