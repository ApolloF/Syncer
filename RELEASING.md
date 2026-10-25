# Releasing Syncer

Installed copies update themselves from the newest *published* GitHub release, so nothing reaches users until you publish.

1. Make sure `main` is green and has what you want to ship.
2. Tag it and push the tag: `git tag v1.2.0 && git push origin v1.2.0`. Use `v1.2.0-rc1` for a release candidate (it is marked as a pre-release and is never offered by the updater).
3. CI (`build`) tests and builds both files, then signs `SHA256SUMS.txt` with the release key, and creates a **draft** release with `Syncer-amd64-installer.exe`, `Syncer.exe`, `SHA256SUMS.txt` and `SHA256SUMS.txt.sig`, and records build provenance for the two exes.
4. Download the draft's assets and check them:
   - `sha256sum -c SHA256SUMS.txt` (or `Get-FileHash`) matches the files.
   - `go run ./tools/release-sign -verify SHA256SUMS.txt -tag v1.2.0 -pubfile internal/update/pubkey.go` accepts the signature (CI runs the same check on the signature it makes, and fails the release if it doesn't pass). Installed copies only update to a release that passes this.
   - `gh attestation verify Syncer.exe --repo ApolloF/syncer` and the same for the installer.
5. Smoke-test on a clean VM or PC: a fresh install from the installer, and an upgrade over the previous release (settings and backups still there, the app restarts into the new version).
6. Edit the release notes if needed, then publish the draft. A release candidate stays a pre-release.
7. Copies of the previous release find the new one at their next check and update themselves. A copy running `v1.2.0-rc1` is offered `v1.2.0` once it is published.

If a published release is bad, mark it as a draft again (or delete it) so the updater stops offering it, then fix forward with a new tag.

## Release signing key

Releases are signed so that a tampered or replaced asset is never installed: the app embeds the public key and installs only a Syncer.exe whose hash is in a validly signed SHA256SUMS.txt. The private key is the base64 of an ed25519 seed, one line.

One-time setup:

1. `go run ./tools/release-sign -keygen -out <path outside the repo>` writes the private key (it refuses to overwrite) and prints only the public key.
2. `Get-Content -Raw <path> | gh secret set RELEASE_SIGNING_KEY --repo ApolloF/Syncer` gives it to CI (PowerShell has no `<`; in bash use `gh secret set RELEASE_SIGNING_KEY < <path>`). Don't use `--body`: the key would land in the process arguments and the shell history. The release job fails when the secret is empty.
3. Paste the printed public key into `releasePublicKey` in `internal/update/pubkey.go` and release that. Until then no update verifies, so none is installed.
4. Keep the private key file somewhere safe and offline, and delete the copy on the machine you generated it on. The file mode 0600 does nothing on Windows, so until it is offline keep it in a folder under your user profile.

Recommended: move the secret into a GitHub environment that only tags `v*` may deploy to (repository settings), so no other branch's workflow can read it.

If the key is lost or leaked: make a new one as above, put its public key in `pubkey.go`, and publish that release signed with the old key, so installed copies accept it and from then on trust the new key. CI checks each signature against the key in `pubkey.go`, which for the bridge release is the new one, so for that one tag keep a copy of the old `pubkey.go` in the repo and set the repository variable `RELEASE_PUBKEY_FILE` to its path; remove the variable afterwards. Switch the `RELEASE_SIGNING_KEY` secret to the new key only after the bridge release is out. A leaked key can't be revoked for copies that haven't updated yet: until they take the bridge release, whoever has the key can sign updates for them. If the old key is lost, nothing signed with a new key will be accepted by installed copies: users must download and install that release by hand.

## Personal build

`wails build -tags personal` makes the maintainer's build (see [CONTRIBUTING.md](CONTRIBUTING.md#builds-public-vs-personal)). It is built locally, with the Google client passed through `-ldflags`, and is never published or attached to a release. CI only builds the public one.

`scripts/build-personal.ps1` builds it with its installer into `build\bin`. It reads the Google client from `.env.personal` at the repo root (gitignored; `GDRIVE_CLIENT_ID=…` and `GDRIVE_CLIENT_SECRET=…` on their own lines) or from environment variables of the same names, and stamps the version as the last tag plus the commit (`v1.0.0+personal.3a79b1e`). The personal build doesn't update itself: when it says a new release is out, pull `main` and run the script again. The exe contains the Google client, so don't share it.
