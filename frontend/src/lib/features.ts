import type { main } from '../../wailsjs/go/models'

// What this build of Syncer has (see internal/features). The views ask here
// instead of reading the flags, so one place decides what a flag hides.
type O = Pick<main.Overview, 'features' | 'settings' | 'google'> | null | undefined

export const modsAvailable = (o: O) => !!o?.features?.mods

/** The Mods page and its nav item: the build has mods and finding them is on. */
export const modsPageOn = (o: O) => modsAvailable(o) && !!o?.settings.findMods

export const launchersAvailable = (o: O) => !!o?.features?.launchers

/** Backups go through Syncer's own Google sign-in (the Google section of Backup). */
export const googleBackendOn = (o: O) => !!o?.features?.googleSignIn && o.settings.backupBackend === 'google'

/** "Sign in with Google" is offered. */
export const googleSignInOffered = (o: O) => !!o?.features?.googleSignIn && !!o.google.available

/** Backups were set to Google sign-in, which this build doesn't have: a folder must be chosen. */
export const googleRetired = (o: O) => !!o?.google?.retired

export const googleRetiredNotice =
  'Sign in with Google is no longer part of Syncer. Choose a backup folder (for example your Google Drive for desktop folder) to keep backing up.'

/** The personal build is rebuilt to update; it never installs the public release. */
export const updatesByRebuild = (o: O) => !!o?.features?.personal

/** The page that lists a folder with an issue. */
export function issueView(i: Pick<main.FolderIssue, 'launcher' | 'mod'>, o: O): 'settings' | 'mods' | 'games' {
  if (i.launcher && launchersAvailable(o)) return 'settings'
  if (i.mod && modsPageOn(o)) return 'mods'
  return 'games'
}

/** The pill for where backups go (Backup page, folder backup): not set up while the Google sign-in is gone. */
export function folderBackupStatus(o: Pick<main.Overview, 'features' | 'settings' | 'google' | 'drive'> | null | undefined): { kind: string; text: string } {
  if (!o) return { kind: 'err', text: '' }
  if (googleRetired(o)) return { kind: 'warn', text: 'Not set up' }
  const kind = o.settings.backupRoot || o.drive.running ? 'ok' : o.drive.found ? 'warn' : 'err'
  if (o.settings.backupRoot) return { kind, text: 'Custom folder' }
  if (!o.drive.found) return { kind, text: o.settings.driveRoot ? 'Chosen drive not found' : 'Not found' }
  return { kind, text: o.drive.running ? 'Connected' : 'Installed, not running' }
}
