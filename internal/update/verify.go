package update

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// ErrNotSigned is what an update that can't be vouched for fails with: no
// signature, a wrong one, or an exe that isn't the one that was signed.
var ErrNotSigned = errors.New("This update isn't signed, so it wasn't installed.")

func notSigned(why string) error { return fmt.Errorf("%w (%s)", ErrNotSigned, why) }

// SignedMessage is what a release's signature covers: the tag too, so that
// an older release's signed files can't be passed off as a newer one.
func SignedMessage(tag string, sums []byte) []byte {
	return append([]byte("Syncer "+tag+"\n"), sums...)
}

// Verify checks sig (base64, as release-sign writes it) over tag and sums
// with pubKey and returns the SHA-256 of every file the sums name. sums is
// sha256sum's output: "<hex>  <name>" or "<hex> *<name>" per line.
func Verify(tag string, sums, sig []byte, pubKey ed25519.PublicKey) (map[string][sha256.Size]byte, error) {
	if len(pubKey) != ed25519.PublicKeySize {
		return nil, notSigned("no valid release key")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil || !ed25519.Verify(pubKey, SignedMessage(tag, sums), raw) {
		return nil, notSigned("bad signature")
	}
	out := map[string][sha256.Size]byte{}
	for _, line := range strings.Split(string(sums), "\n") {
		if line == "" {
			continue
		}
		hexSum, rest, ok := strings.Cut(line, " ")
		if !ok || len(rest) < 2 || (rest[0] != ' ' && rest[0] != '*') {
			return nil, notSigned("malformed checksums")
		}
		name := rest[1:]
		b, err := hex.DecodeString(hexSum)
		if err != nil || len(b) != sha256.Size || name == "" || strings.ContainsAny(name, `/\`) {
			return nil, notSigned("malformed checksums")
		}
		if _, dup := out[name]; dup {
			return nil, notSigned("duplicate checksum for " + name)
		}
		out[name] = [sha256.Size]byte(b)
	}
	return out, nil
}
