/** A device ID in the groups of 7 Syncthing shows it in; anything else is returned as given. */
export function idGroups(id: string): string[] {
  const raw = id.replace(/[-\s]/g, '').toUpperCase()
  if (raw.length !== 56) return id ? [id] : []
  return raw.match(/.{7}/g) ?? []
}

/** The groups of an ID, each with whether a dash follows it: a line may only break after a dash. */
export function idSegments(id: string): { group: string; dash: boolean }[] {
  const groups = idGroups(id)
  return groups.map((group, i) => ({ group, dash: i < groups.length - 1 }))
}

/** The name a PC gave itself, or a plain stand-in when it sent none. */
export function selfName(name: string | undefined | null): string {
  return name?.trim() || 'A PC'
}
