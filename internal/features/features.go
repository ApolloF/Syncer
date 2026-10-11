// Package features says which optional parts of Syncer this build has. The
// public release has none of them; the owner's personal build
// (go build -tags personal, wails build -tags personal) has all of them.
//
// The flags are constants, so the compiler drops the code behind a flag that
// is off. The code itself is always compiled and tested in both builds: what
// differs is only whether it is reachable. Gate at the entry points (what the
// UI can call, what runs by itself), and never clear what a build doesn't use:
// the same settings and files may be read by the other build later.
package features
