import { describe, expect, it } from 'vitest'
import { idGroups, selfName } from './devices'

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
