package update

import "crypto/ed25519"

// SetPublicKeyForTest makes updates verify against key; it returns the undo.
func SetPublicKeyForTest(key ed25519.PublicKey) (restore func()) {
	prev := embeddedKey
	embeddedKey = key
	return func() { embeddedKey = prev }
}
