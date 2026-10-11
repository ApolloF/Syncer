# Releasing Syncer

Installed copies update themselves from the newest *published* GitHub release, so nothing reaches users until you publish.

1. Make sure `main` is green and has what you want to ship.
2. Tag it and push the tag: `git tag v1.2.0 && git push origin v1.2.0`. Use `v1.2.0-rc1` for a release candidate (it is marked as a pre-release and is never offered by the updater).
3. CI (`build`) tests and builds both files, then creates a **draft** release with `Syncer-amd64-installer.exe`, `Syncer.exe` and `SHA256SUMS.txt`, and records build provenance for the two exes.
4. Download the draft's assets and check them:
   - `sha256sum -c SHA256SUMS.txt` (or `Get-FileHash`) matches the files.
   - `gh attestation verify Syncer.exe --repo ApolloF/syncer` and the same for the installer.
5. Smoke-test on a clean VM or PC: a fresh install from the installer, and an upgrade over the previous release (settings and backups still there, the app restarts into the new version).
6. Edit the release notes if needed, then publish the draft. A release candidate stays a pre-release.
7. Copies of the previous release find the new one at their next check and update themselves. A copy running `v1.2.0-rc1` is offered `v1.2.0` once it is published.

If a published release is bad, mark it as a draft again (or delete it) so the updater stops offering it, then fix forward with a new tag.

## Personal build

`wails build -tags personal` makes the maintainer's build (see [CONTRIBUTING.md](CONTRIBUTING.md#builds-public-vs-personal)). It is built locally, with the Google client passed through `-ldflags`, and is never published or attached to a release. CI only builds the public one.
