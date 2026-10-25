import { describe, expect, it } from 'vitest'
import { allowNote, checkWhere, idGroups, namedBy, idSegments, ignoredNote, requestNote, requestTitle, selfName, unlinkedMessage, unlinkNote } from './devices'

const id = 'MFZWI3D-BONSGYC-YLTMRWG-C43ENR5-QXGZDMM-FZWI3DP-BONSGYY-LTMRWAD'

describe('idGroups', () => {
  it('splits an ID into groups of 7', () => {
    expect(idGroups(id)).toHaveLength(8)
    expect(idGroups(id).join('-')).toBe(id)
  })
  it('accepts an ID without dashes or in lower case', () => {
    expect(idGroups(id.replaceAll('-', '').toLowerCase()).join('-')).toBe(id)
  })
  it('returns something that is not an ID as it is', () => {
    expect(idGroups('abc')).toEqual(['abc'])
    expect(idGroups('')).toEqual([])
  })
})

describe('idSegments', () => {
  it('puts a dash after every group but the last', () => {
    const segs = idSegments(id)
    expect(segs).toHaveLength(8)
    expect(segs.slice(0, 7).every((s) => s.dash)).toBe(true)
    expect(segs[7].dash).toBe(false)
    expect(segs.map((s) => s.group + (s.dash ? '-' : '')).join('')).toBe(id)
  })
  it('keeps something that is not an ID whole', () => {
    expect(idSegments('abc')).toEqual([{ group: 'abc', dash: false }])
    expect(idSegments('')).toEqual([])
  })
})

describe('selfName', () => {
  it('falls back when the PC sent no name', () => {
    expect(selfName('')).toBe('A PC')
    expect(selfName('  ')).toBe('A PC')
    expect(selfName(undefined)).toBe('A PC')
  })
  it('keeps the name it gave itself', () => {
    expect(selfName(' My Desktop ')).toBe('My Desktop')
  })
})

describe('join requests', () => {
  it('names the linked PC a PC came through', () => {
    expect(requestTitle({ name: 'SYNCER-C', via: 'SYNCER-A' })).toBe('SYNCER-C wants to join as one of your PCs')
    expect(requestNote({ name: 'SYNCER-C', via: 'SYNCER-A' })).toBe('Your PC SYNCER-A is linked to SYNCER-C. Allow it here only if it is your own PC running Syncer: it gets all your game saves.')
    expect(allowNote({ name: 'SYNCER-C', via: 'SYNCER-A' })).toBe('Your PC SYNCER-A is linked to it. If you allow it here too, it gets all your game saves, and it links to this PC by itself.')
    expect(namedBy({ name: 'SYNCER-C', via: 'SYNCER-A' })).toBe('SYNCER-A calls it')
    expect(checkWhere({ name: 'SYNCER-C', via: 'SYNCER-A' })).toMatch(/^Open Devices on the new PC itself/)
    expect(ignoredNote({ name: 'SYNCER-C', via: 'SYNCER-A' })).toBe('Linked to SYNCER-A. Syncer leaves it alone here.')
  })
  it('keeps the plain request of a linked device', () => {
    expect(requestTitle({ name: 'Desk', via: '' })).toBe('Desk wants to join as one of your PCs')
    expect(requestNote({ name: 'Desk' })).toMatch(/^It is already connected/)
    expect(allowNote({ name: 'Desk' })).toMatch(/^A PC that is already connected/)
    expect(ignoredNote({ name: 'Desk' })).toBe('Asked to join as one of your PCs. Syncer leaves it alone.')
    expect(namedBy({ name: 'Desk' })).toBe('The name it gives itself is')
    expect(checkWhere({ name: 'Desk' })).toMatch(/^Open Devices on your other PC/)
  })
})

describe('unlinking', () => {
  it('says an online PC drops the link too if Syncer runs there', () => {
    expect(unlinkNote('Laptop', true)).toBe('Saves stop syncing with that PC. If Syncer is running on Laptop, it drops the link too. Nothing is deleted on either side, and folders you sync with it outside Syncer keep syncing.')
  })
  it('says what an offline PC shows', () => {
    expect(unlinkNote('Laptop', false)).toContain("Laptop isn't connected right now. It will show this PC as offline until you unlink it there too.")
    expect(unlinkNote('', false)).toContain("That PC isn't connected right now.")
  })
  it('tells when the other PC did not get word', () => {
    expect(unlinkedMessage('Laptop', true, true)).toBe('Unlinked')
    expect(unlinkedMessage('Laptop', false, false)).toBe('Unlinked')
    expect(unlinkedMessage('Laptop', true, false)).toBe("Unlinked. Laptop didn't answer in time, so it will show this PC as offline until you unlink it there too.")
  })
})
