# Contributing to Syncer

Thanks for helping. Syncer is a one-person project, so small, focused changes get reviewed fastest.

## Reporting problems

- **Bugs:** use the [bug report](https://github.com/ApolloF/syncer/issues/new?template=bug_report.yml) form and include the part of `%APPDATA%\Syncer\syncer.log` around the problem (and `crash.log` from the same folder if Syncer crashed). Remove anything personal first (paths with your user name, your email address).
- **A game isn't found:** use the [game form](https://github.com/ApolloF/syncer/issues/new?template=game_request.yml). Save locations come from PCGamingWiki through the Ludusavi manifest, so fixing the game's PCGamingWiki page helps every tool that uses it.
- **Security issues:** don't open a public issue. Email me@apollof.nl instead.

## Code changes

1. Open an issue first for anything larger than a small fix, so we can agree on the approach before you spend time on it.
2. Fork, create a branch from `main`, and keep the change to one topic.
3. Before you open the pull request, run:

   ```bash
   go vet ./... && go vet -tags personal ./...
   go test ./... && go test -tags personal ./...
   cd frontend && npm ci && npm run check && npx vitest run
   cd .. && wails build
   ```

4. Add or update tests for behaviour you change (`*_test.go` next to the Go code, `*.test.ts` next to frontend helpers in `frontend/src/lib`).
5. Try the change in the real app with `wails dev`. Anything that touches syncing, backups or restores must never delete or overwrite a save without a restore point.

## Build and layout

Requirements: Go 1.26+, Node 22+, [Wails v2](https://wails.io) (`go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0`).

```bash
wails dev          # live-reloading app
go test ./...      # backend tests
go test -tags personal ./...   # the same tests for the personal build
wails build        # build/bin/Syncer.exe
```

`wails build` makes the public release: no mods, no Google sign-in and no launcher integration, switched off at compile time (see [below](#builds-public-vs-personal)). `wails build -tags personal` makes the maintainer's personal build with all of those on; it never installs the public release over itself. Google sign-in in a personal build needs a Google OAuth client id and secret passed at build time (`-ldflags "-X main.gdriveClientID=… -X main.gdriveClientSecret=…"`); without them it is hidden. Releases from CI are always the public build.

The repo root holds only `main.go`, which embeds the frontend and hands over to `internal/app` (the Wails `App`, tray, background runs, launcher API and feature glue). The rest of the backend lives in `internal/`: `syncthing` (REST client), `meta` (cross-PC folder sharing), `discover` (manifest, scan, installed games), `backup` (mirror/versions/restore), `gdrive` (Google sign-in and Drive sync, personal build only), `features` (public/personal switches), `tasks` (Task Scheduler), `paths` (portable paths, sync safety rules) and `winx` (Windows priority, full-screen and foreground checks). The Svelte 5 frontend is in `frontend/src`. Draft winget and Scoop manifests are in [packaging/](packaging/).

## Builds: public vs personal

The default build (`wails build`, no tags) is the public release. Mods (mod folder sync, Vortex sharing), signing in to Google for backups and the launcher integration are switched off in it at compile time; the code is still compiled and tested. `wails build -tags personal` makes the maintainer's build with all of them on; it checks GitHub for a new release but never installs it over itself. The switches are constants in `internal/features`.

When you touch one of those features, gate it where it is entered (a bound method, a background loop, the UI), keep the method's signature so the generated `frontend/wailsjs` stays the same, and never clear the user's stored settings or files. Run the Go tests for both builds: `go test ./...` and `go test -tags personal ./...`. Tests of a switched-off feature's internals go behind `//go:build personal`; tests that prove the public build leaves it alone go behind `//go:build !personal`.

## Style

- Follow the existing structure and naming; don't add a new library when the standard library or an existing dependency covers it.
- UI text is short and plain: say what happens, in the words a player uses.
- No telemetry, analytics or network calls beyond those listed in [PRIVACY.md](PRIVACY.md). A change that adds one needs the privacy policy updated in the same pull request.

## License

Syncer is licensed under the [AGPL-3.0](LICENSE). By sending a pull request you agree that your contribution is licensed under the same license.
