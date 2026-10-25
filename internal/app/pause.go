package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/ApolloF/syncer/internal/accounts"
	"github.com/ApolloF/syncer/internal/features"
	"github.com/ApolloF/syncer/internal/logx"
	"github.com/ApolloF/syncer/internal/mods"
	"github.com/ApolloF/syncer/internal/store"
	"github.com/ApolloF/syncer/internal/syncthing"
	"github.com/ApolloF/syncer/internal/tasks"
)

// A pause stops syncing (every Syncthing folder is paused) and automatic
// backups on this PC for a while. It always ends on its own: a one-off
// scheduled task runs "Syncer.exe --resume" at the end, and the window and
// the background task resume too as soon as they notice the time has passed.

const maxPause = 7 * 24 * time.Hour

// Pause pauses syncing and automatic backups until the given time (unix s).
// A manual "Back up now" still works.
func (a *App) Pause(until int64) error {
	t := time.Unix(until, 0)
	switch {
	case !t.After(time.Now()):
		return errors.New("pick a time in the future")
	case t.After(time.Now().Add(maxPause)):
		return errors.New("a pause can last a week at most")
	}
	if _, err := store.UpdateSettings(func(s *store.Settings) { s.PausedUntil = t }); err != nil {
		return err
	}
	scheduleResume(t)
	if c, err := a.client(); err == nil {
		ctx, cancel := a.callCtx()
		defer cancel()
		if err := syncPause(ctx, c); err != nil {
			// Not in effect (the folders couldn't be recorded or paused): don't
			// leave the window saying "Paused until" for a pause that isn't.
			logx.Printf("pause: %v", err)
			if rerr := a.Resume(); rerr != nil {
				logx.Printf("pause: undoing it: %v", rerr)
			}
			return errors.New("Couldn't pause right now. Try again in a moment.")
		}
	}
	logx.Printf("paused syncing and backups until %s", t.Format("Mon 15:04"))
	a.refreshTray()
	runtime.EventsEmit(a.ctx, "changed")
	return nil
}

// Resume ends a pause now.
func (a *App) Resume() error {
	if _, err := store.UpdateSettings(func(s *store.Settings) { s.PausedUntil = time.Time{} }); err != nil {
		return err
	}
	a.afterResume()
	return nil
}

// afterResume unpauses the folders and catches up on what the pause held back.
func (a *App) afterResume() {
	if c, err := a.client(); err == nil {
		ctx, cancel := a.callCtx()
		if err := syncPause(ctx, c); err != nil {
			logx.Printf("resume: %v", err)
		}
		cancel()
	}
	_ = tasks.Delete(tasks.ResumeTask)
	logx.Printf("syncing and backups resumed")
	a.refreshTray()
	runtime.EventsEmit(a.ctx, "changed")
	go a.runAutoAdd()
}

// pauseLoop ends an expired pause while the window or tray is open, and
// pauses folders that appeared (or a Syncthing that started) mid-pause.
func (a *App) pauseLoop(ctx context.Context) {
	for ctx.Err() == nil {
		s := store.LoadSettings()
		held := len(store.LoadState().PausedFolders) > 0
		switch {
		case s.Paused():
			if c, err := a.client(); err == nil {
				cctx, cancel := a.callCtx()
				_ = syncPause(cctx, c)
				cancel()
			}
		case !s.PausedUntil.IsZero(): // the pause just ran out
			_, _ = store.UpdateSettings(func(s *store.Settings) { s.PausedUntil = time.Time{} })
			a.afterResume()
			runtime.EventsEmit(a.ctx, "toast", "Syncing and backups resumed")
		case held: // Syncthing wasn't reachable when it ran out: try again
			if c, err := a.client(); err == nil {
				cctx, cancel := a.callCtx()
				_ = syncPause(cctx, c)
				cancel()
			}
		}
		sleep(ctx, 30*time.Second)
	}
}

// syncPause makes Syncthing match the pause setting: while paused, every
// running folder is paused and remembered; afterwards only those are resumed,
// so folders paused by hand in Syncthing stay paused.
func syncPause(ctx context.Context, c *syncthing.Client) error {
	paused := store.LoadSettings().Paused()
	held := store.LoadState().PausedFolders
	if !paused && len(held) == 0 {
		return nil
	}
	fs, err := c.Folders(ctx)
	if err != nil {
		return err
	}
	var errs []error
	if paused {
		var todo []string
		for _, f := range fs {
			if !f.Paused {
				todo = append(todo, f.ID)
			}
		}
		// Remembered before pausing, so a crash in between can't leave a
		// folder paused that nothing will ever resume.
		if len(todo) > 0 {
			err := store.UpdateState(func(st *store.State) {
				for _, id := range todo {
					if !slices.Contains(st.PausedFolders, id) {
						st.PausedFolders = append(st.PausedFolders, id)
					}
				}
			})
			if err != nil {
				return fmt.Errorf("not pausing folders that couldn't be remembered: %w", err)
			}
		}
		var failed []string
		for _, id := range todo {
			if err := c.PatchFolder(ctx, id, map[string]any{"paused": true}); err != nil {
				errs = append(errs, err)
				failed = append(failed, id)
			}
		}
		if gone := notPaused(ctx, c, failed); len(gone) > 0 {
			store.UpdateState(func(st *store.State) {
				st.PausedFolders = slices.DeleteFunc(st.PausedFolders, func(id string) bool {
					return slices.Contains(gone, id)
				})
			})
		}
		return errors.Join(errs...)
	}
	exists := map[string]bool{}
	for _, f := range fs {
		exists[f.ID] = true
	}
	// A game is running and holds these too: they resume when it exits.
	game := map[string]bool{}
	if st := store.LoadState(); time.Since(st.GameHeld) < staleHold {
		for _, id := range st.GamePaused {
			game[id] = true
		}
	}
	s := store.LoadSettings()
	var modHeld, dropped []string
	for _, id := range held {
		if !exists[id] || game[id] {
			dropped = append(dropped, id) // removed meanwhile, or the game's hold takes it over
			continue
		}
		// A mod folder waits while its mod manager is open (see modsTick),
		// and a PC receiving deployed mods keeps its folder paused.
		if isMod(s, id) && (!features.Mods || !holdable(s, id) || mods.ManagerRunning(processPaths())) {
			if holdable(s, id) {
				modHeld = append(modHeld, id)
			}
			dropped = append(dropped, id)
			continue
		}
		// Saves moving there (an account operation) resume when that's done.
		if err := resumeLocked(ctx, c, id, func(st *store.State) *[]string { return &st.PausedFolders }); err != nil {
			errs = append(errs, err)
		}
	}
	store.UpdateState(func(st *store.State) {
		st.PausedFolders = slices.DeleteFunc(st.PausedFolders, func(id string) bool { return slices.Contains(dropped, id) })
		for _, id := range modHeld {
			if !slices.Contains(st.ModHeld, id) {
				st.ModHeld = append(st.ModHeld, id)
			}
		}
	})
	return errors.Join(errs...)
}

// lockedCall bounds a Syncthing call made while holding the state lock,
// which every other change to the state waits for.
const lockedCall = 5 * time.Second

// opFolder reports whether an unfinished account operation works on a
// folder. When that can't be told, every folder counts as one of them.
func opFolder() func(id string) bool {
	ids, err := accounts.OpFolders()
	if err != nil {
		logx.Printf("%v", err)
		return func(string) bool { return true }
	}
	return func(id string) bool { return slices.Contains(ids, id) }
}

// resumeLocked resumes folder id if it's still in the list that list picks
// and no account operation works on it, and then drops it from that list.
// Both happen under the state lock: an account operation reads the lists
// under it too, so it never finds a folder paused that is being resumed. A
// folder that isn't resumed stays listed.
func resumeLocked(ctx context.Context, c *syncthing.Client, id string, list func(*store.State) *[]string) error {
	var perr error
	if err := store.UpdateState(func(st *store.State) {
		l := list(st)
		if !slices.Contains(*l, id) || opFolder()(id) {
			return
		}
		cctx, cancel := context.WithTimeout(ctx, lockedCall)
		defer cancel()
		if perr = c.PatchFolder(cctx, id, map[string]any{"paused": false}); perr == nil {
			*l = slices.DeleteFunc(*l, func(x string) bool { return x == id })
		}
	}); err != nil {
		return err
	}
	return perr
}

// runResume is what the one-off \Syncer-Resume task runs.
func runResume() {
	defer logx.RecoverExit("resume")
	s := store.LoadSettings()
	if s.Paused() {
		return // the pause was extended; its own task will come
	}
	if !s.PausedUntil.IsZero() {
		_, _ = store.UpdateSettings(func(s *store.Settings) { s.PausedUntil = time.Time{} })
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if c, err := syncthing.New(); err == nil {
		if err := syncPause(ctx, c); err != nil {
			logx.Printf("resume: %v", err)
		}
	}
	_ = tasks.Delete(tasks.ResumeTask)
	logx.Printf("pause over: syncing and backups resumed")
	// Catch up: apply other PCs' changes and run the backup the pause held back.
	if tasks.Exists(tasks.BackupTask) {
		if err := tasks.Run(tasks.BackupTask); err != nil {
			logx.Printf("resume: %v", err)
		}
	}
}

// notPaused returns those of ids that Syncthing really doesn't have paused.
// A pause that came back with an error may still have gone through, so the
// folders are looked at again, and the ones that are paused stay remembered.
// If Syncthing can't be asked, none is dropped.
func notPaused(ctx context.Context, c *syncthing.Client, ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	fs, err := c.AllFolders(ctx)
	if err != nil {
		return nil
	}
	paused := map[string]bool{}
	for _, f := range fs {
		paused[f.ID] = f.Paused
	}
	return slices.DeleteFunc(slices.Clone(ids), func(id string) bool { return paused[id] })
}

// runUninstall is what the uninstaller runs. Nothing of Syncer will be left
// to resume the folders it paused, so it does that now, and removes the
// tasks and the start-with-Windows entry. Syncthing and all data stay.
func runUninstall() {
	defer logx.RecoverExit("uninstall")
	_ = tasks.Delete(tasks.BackupTask)
	_ = tasks.Delete(tasks.ResumeTask)
	removeAutostart()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if c, err := syncthing.New(); err == nil {
		if err := releaseFolders(ctx, c); err != nil {
			logx.Printf("uninstall: %v", err)
		}
	}
}

// releaseFolders resumes the folders Syncer paused for a pause or a running
// game and forgets them. Folders paused by hand stay paused. So do the mod
// folders held on purpose (ModHeld), which is why that list isn't released
// here: they must not sync while a mod manager may change them. A mod folder
// that receives deployed mods stays paused too, as in syncPause.
func releaseFolders(ctx context.Context, c *syncthing.Client) error {
	s, err := store.UpdateSettings(func(s *store.Settings) { s.PausedUntil = time.Time{} })
	if err != nil {
		return err
	}
	st := store.LoadState()
	held := slices.Concat(st.PausedFolders, st.GamePaused)
	if len(held) == 0 {
		return nil
	}
	fs, err := c.Folders(ctx)
	if err != nil {
		return err
	}
	exists := map[string]bool{}
	for _, f := range fs {
		exists[f.ID] = true
	}
	var errs []error
	failed := map[string]bool{}
	busy := opFolder()
	for _, id := range held {
		if !exists[id] || failed[id] || isMod(s, id) && !holdable(s, id) {
			continue
		}
		if busy(id) {
			failed[id] = true // saves are moving there: kept until that's done
			continue
		}
		if err := c.PatchFolder(ctx, id, map[string]any{"paused": false}); err != nil {
			errs = append(errs, err)
			failed[id] = true
		}
	}
	keep := func(ids []string) []string {
		return slices.DeleteFunc(slices.Clone(ids), func(id string) bool { return !failed[id] })
	}
	store.UpdateState(func(s *store.State) {
		s.PausedFolders, s.GamePaused = keep(s.PausedFolders), keep(s.GamePaused)
	})
	return errors.Join(errs...)
}

// scheduleResume registers the task that ends the pause at t.
func scheduleResume(t time.Time) {
	exe, ok := taskExe()
	if !ok {
		return
	}
	err := tasks.Register(tasks.Spec{
		Name:        tasks.ResumeTask,
		Description: "Syncer: ends a pause of game save syncing and backups.",
		Exe:         exe,
		Args:        "--resume",
		At:          t,
		TimeLimit:   10 * time.Minute,
	})
	if err != nil {
		logx.Printf("schedule resume: %v", err)
	}
}

// taskExe is the exe scheduled tasks should run; false for `wails dev`
// builds, which must never be registered.
func taskExe() (string, bool) {
	exe, err := os.Executable()
	if err != nil {
		return "", false
	}
	exe, _ = filepath.EvalSymlinks(exe)
	return exe, !strings.HasSuffix(strings.ToLower(exe), "-dev.exe")
}
