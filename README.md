<p align="center"><img src="build/appicon.png" width="96" alt=""></p>

<h1 align="center">Syncer</h1>

<p align="center">Back up your PC game saves and keep them in sync between your Windows PCs.</p>

<p align="center">
  <a href="https://github.com/ApolloF/syncer/releases/latest"><img src="https://img.shields.io/github/v/release/ApolloF/syncer?label=download" alt="Latest release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/ApolloF/syncer" alt="License: AGPL-3.0"></a>
  <a href="https://github.com/ApolloF/syncer/actions/workflows/build.yml"><img src="https://github.com/ApolloF/syncer/actions/workflows/build.yml/badge.svg" alt="Build"></a>
</p>

<p align="center"><img src="docs/images/syncer-games.png" width="820" alt="Syncer's Games page: each game with its save folder, size, sync state and Sync and Backup switches"></p>

## Features

- Finds the saves of about 14,000 games on its own, from any store or none.
- Keeps a versioned backup with restore points in any folder: your Google Drive or OneDrive folder, a NAS share or an external drive. Works with just one PC.
- Syncs saves between your PCs with [Syncthing](https://syncthing.net), straight from PC to PC, end-to-end encrypted.
- Never overwrites a save behind your back: when two PCs changed the same save, you pick which one to keep.
- Leaves what Steam Cloud, OneDrive and Ubisoft Connect already sync to them, and holds syncing while a game runs.
- Free and open source. No account, no telemetry.

Save locations come from the [Ludusavi manifest](https://github.com/mtkennerly/ludusavi-manifest), which is built from [PCGamingWiki](https://www.pcgamingwiki.com). If a game's save location is missing or wrong, fixing it on the wiki fixes it for everyone.

I made Syncer because I play on more than one PC and couldn't find anything that kept my saves safe and in sync on its own, for every game and every store.

## Install

**[Download Syncer](https://github.com/ApolloF/syncer/releases/latest/download/Syncer-amd64-installer.exe)** for Windows 10 and 11 (64-bit). It installs for your user only, with no admin prompt, and updates itself. `Syncer.exe` on the [releases page](https://github.com/ApolloF/syncer/releases/latest) is a portable copy.

Then open Syncer: it installs Syncthing with one click if you don't have it, and *Backup* lets you pick the backup folder. To add a second PC, install Syncer there and paste its ID on *Devices*.

Syncer isn't code-signed yet, so Windows may say "Windows protected your PC". Click *More info*, then *Run anyway*.

## Help

- [Setup details and questions](docs/help.md): linking PCs, restoring a save, anti-cheat, Steam Cloud, Ludusavi, firewall, uninstalling
- [All features and how it works](docs/features.md)
- [Privacy](PRIVACY.md): Syncer has no server of its own and the developer receives no data from it
- Something wrong, or a game not found? [Open an issue](https://github.com/ApolloF/syncer/issues/new/choose). The log is under *Settings → Activity log*.

## Limitations

- Windows 10 and 11 only. No macOS, Linux or Steam Deck version.
- Syncthing needs both PCs on at the same time; a PC that is off catches up through the backup.
- Saves stored in the Windows registry aren't synced.
- English only.

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[AGPL-3.0](LICENSE). Third-party code is listed in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md). Syncthing (MPL-2.0) is installed separately. Syncer is not affiliated with Valve, Google, Microsoft or the Syncthing Foundation.
