import { describe, expect, it } from 'vitest'
import type { conflict } from '../../wailsjs/go/models'
import { conflictBadge, conflictPCs, currentLabel, otherLabel } from './conflicts'

const copy = (device: string, deviceName = '') => ({ device, deviceName }) as conflict.Conflict

describe('conflictBadge', () => {
  it('calls a single conflict two versions', () => {
    expect(conflictBadge(1)).toBe('2 versions')
    expect(conflictBadge(4)).toBe('4 conflicts')
  })
})

describe('conflictPCs', () => {
  it('lists each PC once, in order', () => {
    expect(conflictPCs([copy('B', 'Laptop'), copy('A', 'Desktop'), copy('B', 'Laptop')])).toEqual([
      { device: 'B', name: 'Laptop' },
      { device: 'A', name: 'Desktop' },
    ])
  })
  it('calls a PC without a name "another PC", not its ID', () => {
    expect(conflictPCs([copy('ABC123')])).toEqual([{ device: 'ABC123', name: 'another PC' }])
  })
  it('is empty without conflicts', () => {
    expect(conflictPCs([])).toEqual([])
  })
})

describe('conflict captions', () => {
  const c = (o: Partial<conflict.Conflict>) => o as conflict.Conflict
  it('reads "Other version · from this PC" when this PC made the copy', () => {
    const x = c({ deviceName: 'this PC', currentName: 'SYNCER-B' })
    expect(otherLabel(x)).toBe('Other version · from this PC')
    expect(currentLabel(x)).toBe('Current · from SYNCER-B')
  })
  it('says "another PC" when the copy has no known PC', () => {
    expect(otherLabel(c({ deviceName: '' }))).toBe('Other version · from another PC')
  })
  it('leaves the PC off a deleted or unknown current file', () => {
    expect(currentLabel(c({ missing: true, currentName: 'SYNCER-B' }))).toBe('Current (deleted)')
    expect(currentLabel(c({}))).toBe('Current')
  })
})
