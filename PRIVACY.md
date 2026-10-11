# Privacy

Last updated: 11 October 2026

Syncer is a free Windows app that keeps your PC game saves in sync between your own PCs and backs them up to a folder you choose. It runs on your PCs and has no server of its own. ApolloF, who publishes Syncer, receives no data from the app: no account, no telemetry, no crash reports.

This policy is also published at https://apps.apollof.nl/syncer/privacy/.

## Who is responsible

Syncer is made and published by ApolloF in the Netherlands. Contact: me@apollof.nl. Because the app never sends your data to ApolloF, ApolloF doesn't hold or process it; the data stays on your PCs, in your backup folder and with the services listed below. If you email ApolloF, ApolloF is the controller of that email (see Your rights).

## Your backup folder

The backup is optional. If you set one up, Syncer copies your saves into a `GameSaveBackup` folder inside a folder you choose on your PC: your Google Drive for desktop folder, a OneDrive folder, a NAS share or an external drive. Syncer only writes to that folder. It has no Google (or other cloud) sign-in of its own and never sends your saves to a cloud service itself; if the folder is kept by Google Drive for desktop, OneDrive or similar, that app uploads it under its own terms and privacy policy. Syncer doesn't see or store your account for it.

## Syncing between your PCs

Saves are synced directly between your PCs by [Syncthing](https://syncthing.net), end-to-end encrypted. Syncthing is a separate program with its own settings. To let your PCs find each other, it uses the Syncthing project's public discovery servers, which see each PC's device ID and IP address. When your PCs can't reach each other directly, Syncthing's public relays pass the encrypted data along without being able to read it. Syncthing also checks the Syncthing project's servers for its own updates, and only sends anonymous usage reports if you agree to them in Syncthing's own interface; Syncer leaves both at Syncthing's defaults. See [Syncthing's security notes](https://docs.syncthing.net/users/security.html).

If you let Syncer install Syncthing, it runs Windows Package Manager (winget), which downloads Syncthing from Microsoft's winget catalog and Syncthing's official release.

## Other network requests

- If your backup folder is kept by a cloud app such as Google Drive for desktop or OneDrive, Syncer only writes files into that folder on this PC; the app does the upload, under its own terms.
- Syncer downloads the [Ludusavi manifest](https://github.com/mtkennerly/ludusavi-manifest) from GitHub to recognise games.
- It checks GitHub once a day for a new Syncer release and downloads it from GitHub. You can turn the check off in Settings.
- Before deleting expired restore points, Syncer checks the time with Google (an empty request to google.com), so a wrong PC clock can't delete them early.
- These requests send nothing about you or your games. GitHub and Google see your IP address, as with any web request.

## What stays on your PC

Settings, sync state and logs stay in `%APPDATA%\Syncer` and `%LOCALAPPDATA%\Syncer`. Logs, and `crash.log` if Syncer ever crashes, can contain game names and folder paths (with your Windows user name). They leave your PC only if you send them to someone yourself, for example with a bug report.

## Third parties Syncer contacts

- **Google** (time check only): https://policies.google.com/privacy
- **Syncthing** discovery servers and relays: https://syncthing.net/
- **GitHub** (game manifest, updates): https://docs.github.com/en/site-policy/privacy-policies/github-general-privacy-statement
- **Microsoft** (winget, only when installing Syncthing): https://privacy.microsoft.com/privacystatement

## No selling, no sharing

Your data is never sold, rented, shared with advertisers or data brokers, or used for advertising. There are no ads, analytics or trackers in Syncer.

## How long data is kept

Everything Syncer stores stays on your PCs and in your backup folder until you delete it. Older versions in the backup are kept for the number of days you set (30 by default; restore points Syncer makes to protect your saves for a year) and then removed. ApolloF keeps nothing, because nothing is received.

## Deleting your data

- *Settings → Undo everything* stops syncing and can unlink your PCs and delete the backups. Your save files are never deleted.
- Uninstall Syncer from *Settings → Apps* in Windows. Uninstalling removes only Syncer's own files: it keeps `%APPDATA%\Syncer`, `%LOCALAPPDATA%\Syncer` and your backups, and Syncthing keeps syncing, so use *Undo everything* first if you want that to stop. Delete those two folders to remove everything from the PC, and delete the `GameSaveBackup` folder in your backup folder to remove the backups.

## Your rights

Under the GDPR you can ask for access to, correction or deletion of personal data about you, restrict or object to its use, and get a copy of it. Since Syncer sends nothing to ApolloF, the only personal data ApolloF may hold is an email you send; it is used only to answer you and deleted when it is no longer needed. Write to me@apollof.nl. You can also complain to the Dutch data protection authority, the [Autoriteit Persoonsgegevens](https://autoriteitpersoonsgegevens.nl/), or the authority where you live.

## Children

Syncer isn't directed at children under 16 and doesn't knowingly collect data from anyone.

## Changes

If this policy changes, the new version is published at https://apps.apollof.nl/syncer/privacy/ with a new date, and this file is updated to match.

## Contact

Questions about privacy: me@apollof.nl. Bugs and feature requests: https://github.com/ApolloF/syncer/issues.
