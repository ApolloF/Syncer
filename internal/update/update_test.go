package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want bool
	}{
		{"v0.6.0", "v0.5.0", true},
		{"v0.10.0", "v0.9.9", true},
		{"v1.0", "v0.99.99", true},
		{"0.6.1", "v0.6.0", true},
		{"v0.6.0", "v0.6.0", false},
		{"v0.5.9", "v0.6.0", false},
		{"v0.7.0-beta", "v0.6.0", true},
		{"v1.1.0", "v1.1.0-rc1", true},
		{"v1.1.0-rc1", "v1.1.0", false},
		{"v1.1.0-rc2", "v1.1.0-rc1", true},
		{"v1.1.0-rc10", "v1.1.0-rc9", false}, // text, as in semver
		{"v1.1.0-rc.10", "v1.1.0-rc.9", true},
		{"v1.1.0-rc", "v1.1.0-rc.1", false},
		{"v1.1.0-1", "v1.1.0-rc", false},
		{"v1.1.0-rc1", "v1.0.9", true},
		{"v1.0.9", "v1.1.0-rc1", false},
		{"v1.1.0-rc1", "v1.1.0-rc1", false},
		{"v1.1.0+build5", "v1.1.0", false},
		{"v1.1.0-", "v1.0.0", false},
		{"v0.7.0", "dev", false},
		{"dev", "v0.1.0", false},
		{"v1.2.3.4", "v1.0.0", false},
		{"", "v1.0.0", false},
	} {
		if got := Newer(tt.a, tt.b); got != tt.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestLatestAndDownload(t *testing.T) {
	exe := []byte("new syncer")
	sum := sha256.Sum256(exe)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			fmt.Fprintf(w, `{"tag_name":"v1.2.0","html_url":%q,"assets":[
				{"name":"Syncer-amd64-installer.exe","browser_download_url":"%[2]s/download/v1.2.0/Syncer-amd64-installer.exe","size":5,"digest":"sha256:%[3]s"},
				{"name":"Syncer.exe","browser_download_url":"%[2]s/download/v1.2.0/Syncer.exe","size":%[4]d,"digest":"sha256:%[3]s"}]}`,
				ReleasesURL+"tag/v1.2.0", srv.URL, hex.EncodeToString(sum[:]), len(exe))
		case "/download/v1.2.0/Syncer.exe":
			w.Write(exe)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	defer func(l, d string, p bool) { latestURL, downloadURL, personal = l, d, p }(latestURL, downloadURL, personal)
	latestURL, downloadURL, personal = srv.URL+"/latest", srv.URL+"/download/", false

	r, err := Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r.Tag != "v1.2.0" || r.Exe == nil || r.Exe.URL != srv.URL+"/download/v1.2.0/Syncer.exe" {
		t.Fatalf("Latest = %+v", r)
	}
	dir := t.TempDir()
	path, err := Download(context.Background(), r.Exe, dir)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != string(exe) {
		t.Fatalf("downloaded %q", b)
	}

	bad := *r.Exe
	bad.SHA256 = strings.Repeat("0", 64)
	if _, err := Download(context.Background(), &bad, dir); err == nil {
		t.Fatal("a download with the wrong checksum was accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, newName)); !os.IsNotExist(err) {
		t.Fatal("a bad download was left behind")
	}
	bad = *r.Exe
	bad.URL = "https://example.com/Syncer.exe"
	if _, err := Download(context.Background(), &bad, dir); err == nil {
		t.Fatal("a download from outside the releases was accepted")
	}
}

func TestLatestRetries(t *testing.T) {
	defer func(l string, a, p time.Duration) { latestURL, attemptTimeout, retryPause = l, a, p }(latestURL, attemptTimeout, retryPause)
	attemptTimeout, retryPause = 200*time.Millisecond, time.Millisecond

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1: // stalls past the attempt's timeout
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
		case 2:
			w.WriteHeader(http.StatusBadGateway)
		default:
			fmt.Fprintf(w, `{"tag_name":"v1.2.0","html_url":%q,"assets":[]}`, ReleasesURL+"tag/v1.2.0")
		}
	}))
	defer srv.Close()
	latestURL = srv.URL

	r, err := Latest(context.Background())
	if err != nil || r.Tag != "v1.2.0" || calls.Load() != 3 {
		t.Fatalf("Latest = %+v, %v after %d calls", r, err, calls.Load())
	}

	calls.Store(0)
	notFound := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.NotFound(w, r)
	}))
	defer notFound.Close()
	latestURL = notFound.URL
	if _, err := Latest(context.Background()); err == nil || calls.Load() != 1 {
		t.Fatalf("a 404 gave %v after %d calls, want an error after 1", err, calls.Load())
	}

	stall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer stall.Close()
	latestURL = stall.URL
	_, err = Latest(context.Background())
	if err == nil || strings.Contains(err.Error(), "deadline") || !strings.Contains(err.Error(), "GitHub didn't answer") {
		t.Fatalf("a stalled GitHub gave %v", err)
	}
}

func TestReplaceAndCleanup(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "Syncer.exe")
	write := func(name, s string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("Syncer.exe", "old")
	write(oldName, "older")
	write(newName, "new")
	defer func(p bool) { personal = p }(personal)
	personal = false
	if err := Replace(exe, filepath.Join(dir, newName)); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "new" {
		t.Fatalf("exe = %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, oldName)); string(b) != "old" {
		t.Fatalf("old = %q", b)
	}
	Cleanup(exe)
	es, _ := os.ReadDir(dir)
	if len(es) != 1 || es[0].Name() != "Syncer.exe" {
		t.Fatalf("left after cleanup: %v", es)
	}
}

func TestPersonalBuildNeverInstalls(t *testing.T) {
	defer func(p bool) { personal = p }(personal)
	personal = true
	dir := t.TempDir()
	exe := filepath.Join(dir, "Syncer.exe")
	if err := os.WriteFile(exe, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Download(context.Background(), &Asset{URL: downloadURL + "v9.9.9/Syncer.exe", Size: 1}, dir); !errors.Is(err, ErrPersonalBuild) {
		t.Errorf("Download err = %v, want ErrPersonalBuild", err)
	}
	if err := Replace(exe, filepath.Join(dir, newName)); !errors.Is(err, ErrPersonalBuild) {
		t.Errorf("Replace err = %v, want ErrPersonalBuild", err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "mine" {
		t.Errorf("exe = %q, changed", b)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("files left in the folder: %v", entries)
	}
}

func TestLatestSkipsDraftsAndPrereleases(t *testing.T) {
	for _, flag := range []string{"draft", "prerelease"} {
		body := fmt.Sprintf(`{"tag_name":"v2.0.0-rc1",%q:true,"assets":[]}`, flag)
		if _, err := decodeRelease(strings.NewReader(body)); err == nil {
			t.Errorf("a %s was accepted as the latest release", flag)
		}
	}
	r, err := decodeRelease(strings.NewReader(`{"tag_name":"v2.0.0","draft":false,"prerelease":false,"assets":[]}`))
	if err != nil || r.Tag != "v2.0.0" {
		t.Fatalf("decodeRelease = %+v, %v", r, err)
	}
}
