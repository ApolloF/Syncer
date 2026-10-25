package update

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var testExe = []byte("new syncer")

// served describes a fake release. Zero values mean a correctly signed v1.2.0.
type served struct {
	key     ed25519.PrivateKey
	signTag string                               // tag the signature was made for
	sigTag  string                               // folder the signature is served from
	tweak   func(sums, sig *string, exe *[]byte) // changes what is served
}

func newKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

// release starts a server for s and returns what Latest finds there, with
// the download URLs pointed at it.
func release(t *testing.T, s served) Release {
	t.Helper()
	const tag = "v1.2.0"
	if s.signTag == "" {
		s.signTag = tag
	}
	if s.sigTag == "" {
		s.sigTag = tag
	}
	sum := sha256.Sum256(testExe)
	sums := hex.EncodeToString(sum[:]) + "  Syncer.exe\n" + strings.Repeat("0", 64) + "  Syncer-amd64-installer.exe\n"
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(s.key, SignedMessage(s.signTag, []byte(sums))))
	exe := testExe
	if s.tweak != nil {
		s.tweak(&sums, &sig, &exe)
	}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			assets := fmt.Sprintf(`{"name":"Syncer.exe","browser_download_url":"%[1]s/download/%[5]s/Syncer.exe","size":%[2]d,"digest":"sha256:%[3]s"},
				{"name":"SHA256SUMS.txt","browser_download_url":"%[1]s/download/%[5]s/SHA256SUMS.txt","size":%[4]d}`,
				srv.URL, len(exe), hex.EncodeToString(sum[:]), len(sums), tag)
			if sig != "" {
				assets += fmt.Sprintf(`,{"name":"SHA256SUMS.txt.sig","browser_download_url":"%s/download/%s/SHA256SUMS.txt.sig","size":%d}`, srv.URL, s.sigTag, len(sig))
			}
			fmt.Fprintf(w, `{"tag_name":%q,"html_url":%q,"assets":[%s]}`, tag, ReleasesURL+"tag/"+tag, assets)
		case "/download/" + tag + "/Syncer.exe":
			w.Write(exe)
		case "/download/" + tag + "/SHA256SUMS.txt":
			w.Write([]byte(sums))
		case "/download/" + s.sigTag + "/SHA256SUMS.txt.sig":
			w.Write([]byte(sig))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	prevL, prevD, prevP := latestURL, downloadURL, personal
	t.Cleanup(func() { latestURL, downloadURL, personal = prevL, prevD, prevP })
	latestURL, downloadURL, personal = srv.URL+"/latest", srv.URL+"/download/", false

	r, err := Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSignedReleaseDownloads(t *testing.T) {
	pub, priv := newKey(t)
	t.Cleanup(SetPublicKeyForTest(pub))
	r := release(t, served{key: priv})
	path, err := Download(context.Background(), r, "v1.1.0", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != string(testExe) {
		t.Fatalf("downloaded %q", b)
	}
}

func TestUnsignedReleasesAreNotInstalled(t *testing.T) {
	pub, priv := newKey(t)
	_, otherPriv := newKey(t)
	for name, s := range map[string]served{
		"no signature":           {key: priv, tweak: func(_, sig *string, _ *[]byte) { *sig = "" }},
		"bad signature":          {key: priv, tweak: func(_, sig *string, _ *[]byte) { *sig = "AAAA" }},
		"another key":            {key: otherPriv},
		"tampered sums":          {key: priv, tweak: func(sums, _ *string, _ *[]byte) { *sums = strings.Replace(*sums, "0", "1", 1) }},
		"exe hash differs":       {key: priv, tweak: func(_, _ *string, exe *[]byte) { *exe = []byte("evil syncer") }},
		"older release":          {key: priv, signTag: "v1.1.0"},
		"signed for another tag": {key: priv, signTag: "v1.2.0+evil"},
		"mixed tags":             {key: priv, sigTag: "v1.1.0"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Cleanup(SetPublicKeyForTest(pub))
			r := release(t, s)
			dir := t.TempDir()
			path, err := Download(context.Background(), r, "v1.1.0", dir)
			if !errors.Is(err, ErrNotSigned) || path != "" {
				t.Fatalf("Download = %q, %v; want ErrNotSigned", path, err)
			}
			if entries, _ := os.ReadDir(dir); len(entries) != 0 {
				t.Fatalf("files left in the folder: %v", entries)
			}
		})
	}
}

func TestOnlyNewerReleasesDownload(t *testing.T) {
	pub, priv := newKey(t)
	t.Cleanup(SetPublicKeyForTest(pub))
	r := release(t, served{key: priv})
	for _, current := range []string{"v1.2.0", "v2.0.0"} {
		if _, err := Download(context.Background(), r, current, t.TempDir()); err == nil {
			t.Errorf("v1.2.0 downloaded over %s", current)
		}
	}
}

func TestFilesMustComeFromTheReleasesOwnFolder(t *testing.T) {
	pub, priv := newKey(t)
	t.Cleanup(SetPublicKeyForTest(pub))
	r := release(t, served{key: priv})
	r.Exe.URL = strings.Replace(r.Exe.URL, "/v1.2.0/", "/v1.1.0/", 1)
	if _, err := Download(context.Background(), r, "v1.0.0", t.TempDir()); !errors.Is(err, ErrNotSigned) {
		t.Fatalf("Download = %v, want ErrNotSigned", err)
	}
}

func TestPlaceholderKeyNeverVerifies(t *testing.T) {
	_, priv := newKey(t)
	r := release(t, served{key: priv})
	if _, err := Download(context.Background(), r, "v1.1.0", t.TempDir()); !errors.Is(err, ErrNotSigned) {
		t.Fatalf("Download with the unset release key = %v, want ErrNotSigned", err)
	}
}

func TestDigestMismatchIsNotInstalled(t *testing.T) {
	pub, priv := newKey(t)
	t.Cleanup(SetPublicKeyForTest(pub))
	r := release(t, served{key: priv})
	r.Exe.SHA256 = strings.Repeat("0", 64)
	dir := t.TempDir()
	if _, err := Download(context.Background(), r, "v1.1.0", dir); !errors.Is(err, ErrNotSigned) {
		t.Fatalf("Download = %v, want ErrNotSigned", err)
	}
	if _, err := os.Stat(filepath.Join(dir, newName)); !os.IsNotExist(err) {
		t.Fatal("a bad download was left behind")
	}
	r.Exe.URL = "https://example.com/Syncer.exe"
	if _, err := Download(context.Background(), r, "v1.1.0", dir); err == nil {
		t.Fatal("a download from outside the releases was accepted")
	}
}

func TestVerifyParsesAndIsStrict(t *testing.T) {
	pub, priv := newKey(t)
	h := strings.Repeat("ab", 32)
	sign := func(s string) []byte {
		return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, SignedMessage("v1.2.0", []byte(s)))))
	}

	good := h + "  Syncer.exe\n" + h + " *Other.exe\n\n"
	m, err := Verify("v1.2.0", []byte(good), sign(good), pub)
	if s, o := m["Syncer.exe"], m["Other.exe"]; err != nil || len(m) != 2 || hex.EncodeToString(s[:]) != h || hex.EncodeToString(o[:]) != h {
		t.Fatalf("Verify = %v, %v", m, err)
	}
	for name, s := range map[string]string{
		"duplicate":    h + "  A.exe\n" + h + "  A.exe\n",
		"bad hex":      strings.Repeat("zz", 32) + "  A.exe\n",
		"short hash":   "abcd  A.exe\n",
		"slash":        h + "  dir/A.exe\n",
		"backslash":    h + `  dir\A.exe` + "\n",
		"one space":    h + " A.exe\n",
		"no name":      h + "  \n",
		"no separator": h + "\n",
	} {
		if _, err := Verify("v1.2.0", []byte(s), sign(s), pub); !errors.Is(err, ErrNotSigned) {
			t.Errorf("%s: Verify = %v, want ErrNotSigned", name, err)
		}
	}
	if _, err := Verify("v1.2.0", []byte(good), sign(good), nil); !errors.Is(err, ErrNotSigned) {
		t.Errorf("no key: Verify = %v", err)
	}
	if _, err := Verify("v1.2.0", []byte(good+"\n"), sign(good), pub); !errors.Is(err, ErrNotSigned) {
		t.Errorf("changed sums: Verify = %v", err)
	}
	if _, err := Verify("v1.3.0", []byte(good), sign(good), pub); !errors.Is(err, ErrNotSigned) {
		t.Errorf("another tag: Verify = %v", err)
	}
}
