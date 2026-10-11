import { describe, expect, it } from 'vitest'
import { googleBackendOn, googleRetired, googleSignInOffered, issueView, modsPageOn, updatesByRebuild } from './features'

const build = (features: object, settings: object = {}, google: object = {}) =>
  ({ features, settings, google }) as any

const pub = { mods: false, googleSignIn: false, launchers: false, personal: false }
const personal = { mods: true, googleSignIn: true, launchers: true, personal: true }

describe('features', () => {
  it('shows the Mods page only with the feature and finding mods on', () => {
    expect(modsPageOn(build(pub, { findMods: true }))).toBe(false)
    expect(modsPageOn(build(personal, { findMods: false }))).toBe(false)
    expect(modsPageOn(build(personal, { findMods: true }))).toBe(true)
    expect(modsPageOn(null)).toBe(false)
  })

  it('treats a Google backend as the folder backup without the sign-in', () => {
    const s = { backupBackend: 'google' }
    expect(googleBackendOn(build(pub, s))).toBe(false)
    expect(googleBackendOn(build(personal, s))).toBe(true)
    expect(googleSignInOffered(build(pub, s, { available: true }))).toBe(false)
    expect(googleSignInOffered(build(personal, s, { available: true }))).toBe(true)
  })

  it('reports a retired Google backend', () => {
    expect(googleRetired(build(pub, {}, { retired: true }))).toBe(true)
    expect(googleRetired(build(personal, {}, { retired: false }))).toBe(false)
  })

  it('updates the personal build by rebuilding', () => {
    expect(updatesByRebuild(build(personal))).toBe(true)
    expect(updatesByRebuild(build(pub))).toBe(false)
  })

  it('routes an issue to a page this build has', () => {
    const on = build(personal, { findMods: true })
    expect(issueView({ launcher: true, mod: false }, on)).toBe('settings')
    expect(issueView({ launcher: false, mod: true }, on)).toBe('mods')
    expect(issueView({ launcher: true, mod: false }, build(pub))).toBe('games')
    expect(issueView({ launcher: false, mod: true }, build(pub, { findMods: true }))).toBe('games')
  })
})
