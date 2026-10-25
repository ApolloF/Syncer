import type { conflict } from '../../wailsjs/go/models'

/** A folder's conflict count as a pill: one conflict is two versions of a save. */
export const conflictBadge = (n: number) => (n === 1 ? '2 versions' : `${n} conflicts`)

/** The other PCs that made conflict copies, once each and in the order first seen. */
export function conflictPCs(cs: conflict.Conflict[]): { device: string; name: string }[] {
  const m = new Map<string, string>()
  for (const c of cs) if (!m.has(c.device)) m.set(c.device, c.deviceName || 'another PC')
  return [...m.entries()].map(([device, name]) => ({ device, name }))
}

/** The caption over the file that is in place now. */
export const currentLabel = (c: conflict.Conflict) =>
  `Current${c.missing ? ' (deleted)' : ''}${c.currentName && !c.missing ? ` · from ${c.currentName}` : ''}`

/** The caption over the conflict copy; without a known PC it is "another PC". */
export const otherLabel = (c: conflict.Conflict) => `Other version · from ${c.deviceName || 'another PC'}`
