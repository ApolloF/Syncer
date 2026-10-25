package update

import (
	"crypto/ed25519"
	"encoding/base64"
)

// releasePublicKey is the base64 ed25519 public key that releases are signed
// with (printed by `go run ./tools/release-sign -keygen -out <file>`). While
// it is not a valid key, no update verifies, so none is installed. CI checks
// the signature it just made against this very line.
const releasePublicKey = "UNSET: paste the public key from release-sign -keygen"

// embeddedKey is releasePublicKey decoded; nil when that isn't a valid key.
var embeddedKey = decodeKey(releasePublicKey)

func decodeKey(s string) ed25519.PublicKey {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil
	}
	return b
}
