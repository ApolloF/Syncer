/**
 * Refreshes the UI on a timer and shortly after backend changes, but only
 * while the window shows: hidden in the tray it does nothing (no timer, no
 * backend calls), and it catches up the moment the window shows again.
 * `refresh(true)` means views should refetch too, `refresh(false)` is the
 * timer's lighter pass.
 */
export function createRefresher(refresh: (changed: boolean) => void, every = 15000, settle = 400) {
  let shown = true
  let timer: ReturnType<typeof setInterval> | null = null
  let pending: ReturnType<typeof setTimeout> | null = null

  const start = () => { timer ??= setInterval(() => refresh(false), every) }
  const stop = () => {
    if (timer) clearInterval(timer)
    if (pending) clearTimeout(pending)
    timer = pending = null
  }

  return {
    /** Starts the timer (unless hidden). */
    start() { if (shown) start() },
    /** The backend reported a change: refresh soon, unless hidden. */
    changed() {
      if (!shown || pending) return
      pending = setTimeout(() => { pending = null; refresh(true) }, settle)
    },
    /** The window was shown or hidden. */
    shown(on: boolean) {
      if (on === shown) return
      shown = on
      if (!on) return stop()
      start()
      refresh(true)
    },
  }
}
