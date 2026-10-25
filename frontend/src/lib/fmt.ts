export function bytes(n: number): string {
  if (!n) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), u.length - 1)
  const v = n / Math.pow(1024, i)
  return `${v >= 100 || i === 0 ? v.toFixed(0) : v.toFixed(1)} ${u[i]}`
}

export function ago(t: string | number | Date | undefined | null): string {
  if (!t) return 'never'
  const d = typeof t === 'number' ? new Date(t * 1000) : new Date(t)
  if (isNaN(d.getTime()) || d.getFullYear() < 2000) return 'never'
  const s = Math.round((Date.now() - d.getTime()) / 1000)
  if (s < 45) return 'just now'
  const m = Math.round(s / 60)
  if (m < 60) return `${m} min ago`
  const h = Math.round(m / 60)
  if (h < 24) return `${h} h ago`
  const days = Math.round(h / 24)
  if (days < 30) return `${days} day${days === 1 ? '' : 's'} ago`
  return d.toLocaleDateString()
}

export interface LastBackupLines {
  when: string // "Last backup": the last run that finished without errors
  failed: string // a second muted line, only when the latest run failed
}

export function lastBackupLines(
  lastSuccess: string | number | Date | undefined | null,
  attempt: { ok: boolean; finished?: string | number | Date | null; errors?: string[] | null } | undefined | null,
): LastBackupLines {
  const done = ago(lastSuccess)
  const when = done === 'never' ? 'No backup yet' : done
  if (!attempt || attempt.ok) return { when, failed: '' }
  const why = attempt.errors?.[0]?.trim()
  return { when, failed: `Last attempt failed ${ago(attempt.finished)}${why ? ` · ${why}` : ''}` }
}

export function when(unix: number): string {
  return new Date(unix * 1000).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}

/** "14:30", or "Fri 06:00" when it isn't today. */
export function pausedUntil(t: string | number | Date | undefined | null): string {
  if (!t) return ''
  const d = new Date(t)
  if (isNaN(d.getTime())) return ''
  const time = d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
  return d.toDateString() === new Date().toDateString() ? time : `${d.toLocaleDateString(undefined, { weekday: 'short' })} ${time}`
}

/** 06:00 tomorrow, local time. */
export function tomorrowMorning(): Date {
  const d = new Date()
  d.setDate(d.getDate() + 1)
  d.setHours(6, 0, 0, 0)
  return d
}

export function err(e: unknown): string {
  if (typeof e === 'string') return e
  if (e && typeof e === 'object' && 'message' in e) return String((e as Error).message)
  return String(e)
}

// choices returns a dropdown's options plus the current value when it isn't
// one of them (set by an older version or by hand), so the select isn't blank.
export function choices(options: number[], current: number | undefined): number[] {
  if (!current || current <= 0 || options.includes(current)) return options
  return [...options, current].sort((a, b) => a - b)
}
