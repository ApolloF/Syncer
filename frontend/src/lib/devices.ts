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

/** A PC asking to join: a linked device asking itself, or (via) a PC one of the user's PCs is linked to. */
export interface JoinAsk {
  name: string
  via?: string
}

/** The heading of a request to join. */
export function requestTitle(r: JoinAsk): string {
  return `${r.name} wants to join as one of your PCs`
}

/** What the user should know before allowing a request to join. */
export function requestNote(r: JoinAsk): string {
  return r.via
    ? `Your PC ${r.via} is linked to ${r.name}. Allow it here only if it is your own PC running Syncer: it gets all your game saves.`
    : 'It is already connected to this PC and says it runs Syncer. Allow it only if it is your own PC running Syncer: it gets all your game saves.'
}

/** The first sentence of the Allow dialog. */
export function allowNote(r: JoinAsk): string {
  return r.via
    ? `Your PC ${r.via} is linked to it. If you allow it here too, it gets all your game saves.`
    : 'A PC that is already connected to this one says it runs Syncer. If you allow it, it gets all your game saves.'
}

/** Who named the PC in the Allow dialog: the name is shown after this. */
export function namedBy(r: JoinAsk): string {
  return r.via ? `${r.via} calls it` : 'The name it gives itself is'
}

/** Where the Allow dialog sends the user to compare the ID. */
export function checkWhere(r: JoinAsk): string {
  return r.via
    ? 'Open Devices on the new PC itself and check that its ID is exactly this one:'
    : 'Open Devices on your other PC and check that its ID is exactly this one:'
}

/** The line under a request the user ignored. */
export function ignoredNote(r: JoinAsk): string {
  return r.via ? `Linked to ${r.via}. Syncer leaves it alone here.` : 'Asked to join as one of your PCs. Syncer leaves it alone.'
}

/** The Unlink dialog's text for a PC, which is online or off. */
export function unlinkNote(name: string, online: boolean): string {
  const pc = name.trim() || 'That PC'
  const stays = 'Nothing is deleted on either side, and folders you sync with it outside Syncer keep syncing.'
  return online
    ? `Saves stop syncing with that PC. If Syncer is running on ${pc}, it drops the link too. ${stays}`
    : `Saves stop syncing with that PC. ${stays} ${pc} isn't connected right now. It will show this PC as offline until you unlink it there too.`
}

/** The message after unlinking a PC; told: it got word and drops the link too. */
export function unlinkedMessage(name: string, online: boolean, told: boolean): string {
  if (told || !online) return 'Unlinked'
  const pc = name.trim() || 'That PC'
  return `Unlinked. ${pc} didn't answer in time, so it will show this PC as offline until you unlink it there too.`
}
