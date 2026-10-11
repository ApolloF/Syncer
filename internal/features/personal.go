//go:build personal

package features

const (
	// Mods: mod folder sync, deployed mods, Vortex sharing, mod settings.
	Mods = true
	// GoogleSignIn: backing up to Drive through Syncer's own Google sign-in.
	GoogleSignIn = true
	// Launchers: the launcher API and the launchers' own data.
	Launchers = true
	// Personal: this is the personal build, which updates by being rebuilt.
	Personal = true
)
