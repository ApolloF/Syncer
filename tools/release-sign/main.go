// Command release-sign makes and uses the key that signs Syncer releases.
//
//	release-sign -keygen -out <file>                  new key; prints the public key
//	release-sign -sign <file> -tag <tag> -key-env <variable>   writes <file>.sig
//	release-sign -verify <file> -tag <tag> -pub <public key>   checks <file> against <file>.sig
//	release-sign -verify <file> -tag <tag> -pubfile <pubkey.go>   same, with the key Syncer embeds
//
// The signature covers the release tag as well as the file, so a release's
// files can't be passed off under another tag.
//
// The private key is the base64 of the 32-byte ed25519 seed (one line), the
// public key and signatures are base64 too. The private key is only ever
// read from a file this tool wrote or from an environment variable, and is
// never printed.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/ApolloF/syncer/internal/update"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("release-sign", flag.ContinueOnError)
	fs.SetOutput(stderr)
	keygen := fs.Bool("keygen", false, "make a new key pair")
	out := fs.String("out", "", "with -keygen: file for the private key")
	sign := fs.String("sign", "", "file to sign")
	keyEnv := fs.String("key-env", "", "with -sign: environment variable holding the private key")
	verify := fs.String("verify", "", "file to check against its .sig")
	pub := fs.String("pub", "", "with -verify: public key")
	pubFile := fs.String("pubfile", "", "with -verify: internal/update/pubkey.go, to use the key embedded there")
	tag := fs.String("tag", "", "with -sign or -verify: the release tag")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	var err error
	switch {
	case *keygen && *sign == "" && *verify == "":
		err = genKey(*out, stdout)
	case *sign != "" && !*keygen && *verify == "":
		err = signFile(*sign, *tag, getenv(*keyEnv), *keyEnv)
	case *verify != "" && !*keygen && *sign == "":
		err = verifyFile(*verify, *tag, *pub, *pubFile)
	default:
		fmt.Fprintln(stderr, "use exactly one of -keygen, -sign or -verify")
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, "release-sign:", err)
		return 1
	}
	return 0
}

// genKey writes a new private key to path (never over an existing file) and
// prints the matching public key.
func genKey(path string, stdout io.Writer) error {
	if path == "" {
		return errors.New("-keygen needs -out <file>")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f, base64.StdEncoding.EncodeToString(priv.Seed()))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	_, err = fmt.Fprintln(stdout, base64.StdEncoding.EncodeToString(pub))
	return err
}

func parseKey(s string) (ed25519.PrivateKey, error) {
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("the private key isn't a base64 ed25519 seed")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func checkTag(tag string) error {
	if tag == "" || strings.ContainsAny(tag, "\r\n") {
		return errors.New("-tag needs the release tag, e.g. v1.2.0")
	}
	return nil
}

func signFile(path, tag, key, keyEnv string) error {
	if err := checkTag(tag); err != nil {
		return err
	}
	if keyEnv == "" {
		return errors.New("-sign needs -key-env <variable>")
	}
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("%s is empty", keyEnv)
	}
	priv, err := parseKey(key)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, update.SignedMessage(tag, data)))
	return os.WriteFile(path+".sig", []byte(sig+"\n"), 0o644)
}

var embeddedKeyLine = regexp.MustCompile(`(?m)^const releasePublicKey = "([^"]*)"\r?$`)

func verifyFile(path, tag, pubB64, pubFile string) error {
	if err := checkTag(tag); err != nil {
		return err
	}
	if (pubB64 == "") == (pubFile == "") {
		return errors.New("-verify needs one of -pub or -pubfile")
	}
	if pubFile != "" {
		src, err := os.ReadFile(pubFile)
		if err != nil {
			return err
		}
		m := embeddedKeyLine.FindSubmatch(src)
		if m == nil {
			return fmt.Errorf("no releasePublicKey in %s", pubFile)
		}
		pubB64 = string(m[1])
	}
	pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(pubB64))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("the public key isn't a base64 ed25519 public key (is the placeholder still in pubkey.go?)")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(path + ".sig")
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || !ed25519.Verify(pub, update.SignedMessage(tag, data), sig) {
		return errors.New("the signature doesn't match")
	}
	return nil
}
