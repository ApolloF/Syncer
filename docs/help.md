# Syncer help

The [README](../README.md) has the short version. This page has setup details and answers to common questions.

## Linking a second PC

1. Install Syncer on the second PC.
2. On one PC, open **Devices** and paste the other PC's ID.
3. On the other PC, choose **Review** on the request, check that the full ID shown matches the one on the first PC's Devices page, and link.

Syncer only shares your saves with PCs you linked or allowed. A PC that is already paired in Syncthing and wants to join shows up on Devices with **Allow** and **Ignore**; check its ID the same way before you allow it.

Every synced game then shows up on the other PC at the right local path, even if the user name or Documents location differs.

## Install details

Syncer isn't code-signed yet, so Windows SmartScreen may warn the first time: click *More info → Run anyway*. To check your download, compare it with `SHA256SUMS.txt` on the release page (in PowerShell: `Get-FileHash Syncer-amd64-installer.exe`), or check where it was built with the [GitHub CLI](https://cli.github.com): `gh attestation verify Syncer-amd64-installer.exe -R ApolloF/syncer`. The built-in updater also checks its downloads against the release's SHA-256. You only install once; after that Syncer updates itself (you can turn that off in Settings).

**Syncthing won't install?** The one-click install needs `winget`, which comes with current Windows 10 and 11. If it isn't available, Syncer shows an error with a link to [install Syncthing by hand](https://syncthing.net/downloads/). Syncer finds it once it's installed.

**Updating from 0.x?** If your backup used *Sign in with Google*, Syncer asks you to choose a backup folder (you get a one-time notification). Your old backup in Google Drive stays where it is. Mod folders you synced are paused and left as they are, with a one-time notice.

## Questions

**Can this get me banned by anti-cheat?**
Syncing saves only touches save files, the same files the game writes itself. Syncer doesn't change the game's own files and doesn't inject anything into it. Saves of online games usually live on the game's servers anyway, so there's nothing for Syncer to sync.

**Which games does it support?**
About 14,000 games whose save locations are listed on PCGamingWiki, through the Ludusavi manifest, from any store: Steam, Epic, GOG, EA, Ubisoft, Xbox, or no store at all. Games that aren't listed can be added by hand with any folder. If a game's save location is missing or wrong, fixing it on PCGamingWiki fixes it for everyone; Syncer refreshes the list every week. You can also [request a game](https://github.com/ApolloF/syncer/issues/new?template=game_request.yml).

**Why not just use Steam Cloud?**
Use it where it works: Syncer leaves folders that Steam Cloud really keeps to Steam, and checks this per folder rather than trusting the store page. Syncer is for everything else: games from other stores or no store, games without cloud saves, mod saves Steam skips (such as SKSE co-saves), and a history of restore points Steam Cloud doesn't keep.

**What happens when two PCs changed the same save?**
Syncer doesn't guess. The game shows **2 versions**, and you pick which one to keep, file by file or all of one PC's files at once. The other version goes into the backup history instead of being deleted.

**How do I restore an older save?**
On the Games page, click the history button (*Restore from backup*) next to the game. Pick the latest backup or any earlier restore point. Your current files are saved as a restore point first, so a restore can be undone.

**Does Syncer get access to my Google account?**
No. Syncer doesn't connect to Google at all (apart from an empty time check). It writes the backup into a folder on your PC; if that folder belongs to Google Drive for desktop, OneDrive or similar, that app uploads it under its own account and permissions.

**Do I need a backup folder or a cloud account?**
No. Syncthing works on its own between PCs that are on at the same time, and with two PCs linked each one holds a full copy. A backup adds restore points and lets a PC pick up saves from a PC that is switched off. It can be any folder: Google Drive for desktop, OneDrive, a NAS share or an external drive.

**My PCs are linked but don't connect.**
The first time Syncthing listens, Windows asks whether it may use the network. If that was dismissed, Windows keeps blocking it and your other PCs can't reach this one. Open *Windows Security → Firewall & network protection → Allow an app through firewall*, choose *Change settings*, and tick *Syncthing* for *Private* networks (add it with *Allow another app* if it's missing). Do this on each PC.

**How is this different from Ludusavi?**
[Ludusavi](https://github.com/mtkennerly/ludusavi) is a backup tool: you back up and restore saves, on one or more PCs. Syncer keeps saves in sync between PCs continuously, in the background, and keeps a backup with history next to that. Syncer uses Ludusavi's manifest to find saves.

**How do I stop using it?**
*Settings → Undo everything* stops all syncing on this PC and can also unlink your PCs, stop or uninstall Syncthing, and delete the backups. Then uninstall Syncer from *Windows Settings → Apps*. Your save files are never deleted.

Uninstalling alone removes only Syncer's own files. Your settings and backups stay, and Syncthing keeps running and syncing your saves, so use *Undo everything* first if you want that to stop. Folders Syncer had paused are resumed when you uninstall, if Syncthing is running then.


## Report a problem

[Open an issue](https://github.com/ApolloF/syncer/issues/new/choose). The activity log is under *Settings → Activity log*, and the full log file is `%APPDATA%\Syncer\syncer.log`. If Syncer crashed, the details are in `crash.log` in the same folder. Please add your Syncer version (bottom of *Settings*) and Windows version. Logs can contain game names and folder paths (with your Windows user name), so check them before you post. See [CONTRIBUTING.md](../CONTRIBUTING.md) if you'd like to help.

## Credits

Game save locations come from the [Ludusavi manifest](https://github.com/mtkennerly/ludusavi-manifest), which is built from [PCGamingWiki](https://www.pcgamingwiki.com) (CC BY-NC-SA 3.0). Syncer downloads it at runtime and doesn't include it. Syncing is done by [Syncthing](https://syncthing.net) (MPL-2.0), installed separately.

Syncer is not affiliated with Valve, Google, Microsoft or the Syncthing Foundation. Syncthing is a trademark of the Syncthing Foundation; Google Drive is a trademark of Google LLC.
