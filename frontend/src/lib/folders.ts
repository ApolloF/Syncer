import type { app, store } from '../../wailsjs/go/models'
import { bytes, ago } from './fmt'

/** A time from Go; the zero time means "never". */
export const isTime = (t: any) => !!t && new Date(t).getFullYear() > 2000

export const plural = (n: number, w: string) => `${n} ${w}${n === 1 ? '' : 's'}`

export function backupLine(f: app.FolderView): string {
  const parts = [isTime(f.backedUp) ? `Backed up ${ago(f.backedUp)}` : 'Not backed up yet']
  if (f.backupBytes) parts.push(bytes(f.backupBytes))
  if (f.points) parts.push(plural(f.points, 'restore point'))
  return parts.join(' · ')
}

/** A synced folder's state as a pill. */
export function stateOf(f: app.FolderView): { kind: string; text: string } {
  // Syncthing stopped the folder, or some files can't be synced: the row says why.
  if (f.problem) return { kind: 'err', text: f.errors ? `${f.errors} error${f.errors > 1 ? 's' : ''}` : 'Stopped' }
  if (!f.exists) return { kind: 'warn', text: 'Waiting' }
  switch (f.state) {
    case 'idle': return f.needBytes ? { kind: 'accent', text: `${bytes(f.needBytes)} to go` } : { kind: 'ok', text: 'Synced' }
    case 'scanning': case 'scan-waiting': return { kind: '', text: 'Scanning' }
    case 'syncing': case 'sync-preparing': case 'sync-waiting': return { kind: 'accent', text: 'Syncing' }
    case 'paused': return { kind: '', text: 'Paused' }
    case 'error': return { kind: 'err', text: 'Error' }
    default: return { kind: '', text: f.state || '…' }
  }
}

/** The Overview line about folders from other PCs that wait for the user's OK ('' when none do). */
export function waitingText(waiting: store.WaitingFolder[] | null | undefined): string {
  const n = waiting?.length ?? 0
  if (!n) return ''
  const from = new Set(waiting!.map(w => w.from))
  const pc = from.size === 1 && waiting![0].from ? waiting![0].from : 'your other PCs'
  return `${plural(n, 'folder')} from ${pc} wait${n === 1 ? 's' : ''} for your OK`
}

/** Why a folder from another PC waits for the user's OK before it syncs here. */
export function waitReason(a: app.AvailableView, autoAddMaxGB: number | undefined): string {
  if (!a.unknownPlace && autoAddMaxGB && autoAddMaxGB > 0 && a.sizeGB > autoAddMaxGB)
    return `Bigger than ${autoAddMaxGB} GB, the largest save Syncer adds automatically.`
  return 'Not a known save location on this PC.'
}
