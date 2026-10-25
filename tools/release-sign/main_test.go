package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runTool(t *testing.T, env map[string]string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var o, e bytes.Buffer
	code = run(args, func(k string) string { return env[k] }, &o, &e)
	return code, o.String(), e.String()
}

func TestKeygenSignVerifyRoundtrip(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key")
	code, pub, errOut := runTool(t, nil, "-keygen", "-out", keyFile)
	if code != 0 {
		t.Fatalf("keygen: %d %s", code, errOut)
	}
	priv, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(pub, strings.TrimSpace(string(priv))) || strings.Count(strings.TrimSpace(pub), "\n") != 0 {
		t.Fatalf("keygen printed %q", pub)
	}
	pub = strings.TrimSpace(pub)

	sums := filepath.Join(dir, "SHA256SUMS.txt")
	if err := os.WriteFile(sums, []byte("abc  Syncer.exe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"KEY": string(priv)}
	if code, out, errOut := runTool(t, env, "-sign", sums, "-tag", "v1.2.0", "-key-env", "KEY"); code != 0 || out != "" || strings.Contains(errOut, string(priv)) {
		t.Fatalf("sign: %d %q %s", code, out, errOut)
	}
	if code, _, errOut := runTool(t, nil, "-verify", sums, "-tag", "v1.2.0", "-pub", pub); code != 0 {
		t.Fatalf("verify: %d %s", code, errOut)
	}
	if code, _, _ := runTool(t, nil, "-verify", sums, "-tag", "v1.1.0", "-pub", pub); code == 0 {
		t.Fatal("a signature for v1.2.0 verified as v1.1.0")
	}
	if code, _, _ := runTool(t, nil, "-verify", sums, "-pub", pub); code == 0 {
		t.Fatal("verify without -tag succeeded")
	}

	pubFile := filepath.Join(dir, "pubkey.go")
	src := "package update\n\nconst releasePublicKey = \"" + pub + "\"\n"
	if err := os.WriteFile(pubFile, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := runTool(t, nil, "-verify", sums, "-tag", "v1.2.0", "-pubfile", pubFile); code != 0 {
		t.Fatalf("verify with -pubfile: %d %s", code, errOut)
	}
	// A Windows checkout has CRLF line endings.
	if err := os.WriteFile(pubFile, []byte(strings.ReplaceAll(src, "\n", "\r\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := runTool(t, nil, "-verify", sums, "-tag", "v1.2.0", "-pubfile", pubFile); code != 0 {
		t.Fatalf("verify with a CRLF -pubfile: %d %s", code, errOut)
	}
	placeholder := "package update\n\nconst releasePublicKey = \"UNSET: paste the public key\"\n"
	if err := os.WriteFile(pubFile, []byte(placeholder), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runTool(t, nil, "-verify", sums, "-tag", "v1.2.0", "-pubfile", pubFile); code == 0 {
		t.Fatal("the placeholder key verified")
	}

	if err := os.WriteFile(sums, []byte("abd  Syncer.exe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runTool(t, nil, "-verify", sums, "-tag", "v1.2.0", "-pub", pub); code == 0 {
		t.Fatal("a changed file verified")
	}
}

func TestKeygenRefusesToOverwrite(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(keyFile, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, _ := runTool(t, nil, "-keygen", "-out", keyFile)
	if code == 0 || out != "" {
		t.Fatalf("keygen over an existing file: %d %q", code, out)
	}
	if b, _ := os.ReadFile(keyFile); string(b) != "keep" {
		t.Fatalf("key file overwritten: %q", b)
	}
}

func TestSignNeedsAKeyInTheEnvironment(t *testing.T) {
	f := filepath.Join(t.TempDir(), "SHA256SUMS.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"", "  ", "not base64!", "AAAA"} {
		if code, _, _ := runTool(t, map[string]string{"KEY": key}, "-sign", f, "-tag", "v1.2.0", "-key-env", "KEY"); code == 0 {
			t.Errorf("signed with key %q", key)
		}
	}
	if _, err := os.Stat(f + ".sig"); !os.IsNotExist(err) {
		t.Error("a signature was written without a valid key")
	}
}
