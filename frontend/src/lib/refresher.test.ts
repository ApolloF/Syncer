import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createRefresher } from './refresher'

describe('createRefresher', () => {
  beforeEach(() => { vi.useFakeTimers() })
  afterEach(() => { vi.useRealTimers() })

  it('refreshes on the timer and after changes while shown', () => {
    const calls: boolean[] = []
    const r = createRefresher(c => calls.push(c), 1000, 100)
    r.start()
    vi.advanceTimersByTime(1000)
    expect(calls).toEqual([false])
    r.changed()
    r.changed() // a burst of changes is one refresh
    vi.advanceTimersByTime(100)
    expect(calls).toEqual([false, true])
  })

  it('does nothing while hidden and catches up once shown', () => {
    const calls: boolean[] = []
    const r = createRefresher(c => calls.push(c), 1000, 100)
    r.start()
    r.changed()
    r.shown(false) // the pending refresh is dropped too
    r.changed()
    vi.advanceTimersByTime(10_000)
    expect(calls).toEqual([])
    r.shown(true)
    expect(calls).toEqual([true])
    vi.advanceTimersByTime(1000)
    expect(calls).toEqual([true, false])
  })

  it('starting hidden waits for the window', () => {
    const calls: boolean[] = []
    const r = createRefresher(c => calls.push(c), 1000, 100)
    r.shown(false)
    r.start()
    vi.advanceTimersByTime(5000)
    expect(calls).toEqual([])
    r.shown(true)
    r.shown(true) // no second catch-up
    expect(calls).toEqual([true])
  })
})
