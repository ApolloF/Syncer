// Package update looks for a newer Syncer release on GitHub, and replaces
// the running Syncer.exe with the release's.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ApolloF/syncer/internal/features"
)

// ReleasesURL is the only place a release link may point into.
const ReleasesURL = "https://github.com/ApolloF/syncer/releases/"

const (
	assetName = "Syncer.exe"
	sumsName  = "SHA256SUMS.txt"
	sigName   = sumsName + ".sig"
	maxSize   = 200 << 20
	maxSmall  = 64 << 10 // the sums and signature files are tiny
)

// Variables for tests.
var (
	// latestURL is GitHub's "latest release", which is never a draft or a
	// pre-release, and drafts aren't visible without a token anyway.
	latestURL = "https://api.github.com/repos/ApolloF/syncer/releases/latest"
	// downloadURL is the only place the new exe may come from.
	downloadURL = ReleasesURL + "download/"
	// personal: this is the personal build, which never installs a release.
	personal = features.Personal
)

// Release is a published Syncer release.
type Release struct {
	Tag  string // e.g. "v0.7.0"
	URL  string // release page
	Exe  *Asset // the release's Syncer.exe; nil when it has none
	Sums *Asset // SHA256SUMS.txt, which the signature covers; nil when missing
	Sig  *Asset // SHA256SUMS.txt.sig; nil when missing (the release is unsigned)
}

// Asset is a file attached to a release.
type Asset struct {
	URL    string
	Size   int64
	SHA256 string // hex, from GitHub's digest of the upload; an extra check only, the signed sums decide
}

// Variables for tests.
var (
	attemptTimeout = 10 * time.Second
	retryPause     = 2 * time.Second
)

const attempts = 3

// Latest asks GitHub for the newest release. A connection that stalls or
// drops, or a GitHub hiccup (5xx), is tried again a couple of times.
func Latest(ctx context.Context) (Release, error) {
	var err error
	for i := range attempts {
		if i > 0 {
			select {
			case <-ctx.Done():
				return Release{}, unreachable(err)
			case <-time.After(retryPause):
			}
		}
		var rel Release
		var retry bool
		if rel, retry, err = latestOnce(ctx); err == nil || !retry {
			return rel, err
		}
	}
	return Release{}, unreachable(err)
}

// unreachable turns a failed request into a message for people, not the
// raw "Get <url>: context deadline exceeded".
func unreachable(err error) error {
	var ue *url.Error
	if !errors.As(err, &ue) {
		return err
	}
	if ue.Timeout() {
		return errors.New("GitHub didn't answer in time; check the internet connection and try again")
	}
	return fmt.Errorf("couldn't reach GitHub (%v); check the internet connection and try again", ue.Err)
}

// latestOnce makes one request; retry tells whether another might succeed.
func latestOnce(ctx context.Context) (rel Release, retry bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestURL, nil)
	if err != nil {
		return Release{}, false, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Syncer")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Release{}, true, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, resp.StatusCode >= 500, errors.New("GitHub answered " + resp.Status)
	}
	rel, err = decodeRelease(resp.Body)
	return rel, false, err
}

func decodeRelease(body io.Reader) (Release, error) {
	var r struct {
		Tag     string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Draft   bool   `json:"draft"`
		Pre     bool   `json:"prerelease"`
		Assets  []struct {
			Name   string `json:"name"`
			URL    string `json:"browser_download_url"`
			Size   int64  `json:"size"`
			Digest string `json:"digest"` // "sha256:<hex>"
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(body, 1<<20)).Decode(&r); err != nil {
		return Release{}, err
	}
	if r.Draft || r.Pre {
		return Release{}, errors.New("GitHub's latest release is a draft or pre-release")
	}
	if _, ok := parse(r.Tag); !ok {
		return Release{}, errors.New("unexpected release tag " + strconv.Quote(r.Tag))
	}
	page := r.HTMLURL
	if !strings.HasPrefix(page, ReleasesURL) {
		page = ReleasesURL + "latest"
	}
	rel := Release{Tag: r.Tag, URL: page}
	for _, a := range r.Assets {
		if !strings.HasPrefix(a.URL, downloadURL+r.Tag+"/") || a.Size <= 0 || a.Size > maxSize {
			continue
		}
		asset := &Asset{URL: a.URL, Size: a.Size}
		switch a.Name {
		case assetName:
			// GitHub's digest is optional: the signed sums are what counts.
			if sum, ok := strings.CutPrefix(a.Digest, "sha256:"); ok {
				if b, err := hex.DecodeString(sum); err != nil || len(b) != sha256.Size {
					continue
				}
				asset.SHA256 = strings.ToLower(sum)
			}
			rel.Exe = asset
		case sumsName:
			rel.Sums = asset
		case sigName:
			rel.Sig = asset
		}
	}
	return rel, nil
}

// ErrPersonalBuild is returned instead of installing a release over a
// personal build: the public release lacks features that build has, so
// updating it would silently drop them. It is rebuilt from source instead.
var ErrPersonalBuild = errors.New("this is the personal build: rebuild it from the source to update (wails build -tags personal)")

// Download fetches the release's Syncer.exe into dir (next to the exe it
// replaces, so the swap is a rename on one volume) and returns the file. The
// exe must be one the release's signed SHA256SUMS.txt names; a release
// without a valid signature, or whose exe doesn't match, fails with
// ErrNotSigned and leaves nothing behind. The release must be newer than
// current, and all its files must sit under its own tag's download folder.
func Download(ctx context.Context, r Release, current, dir string) (string, error) {
	if personal {
		return "", ErrPersonalBuild
	}
	a := r.Exe
	if a == nil || !strings.HasPrefix(a.URL, downloadURL) {
		return "", errors.New("the release has no Syncer.exe to update from")
	}
	if !Newer(r.Tag, current) {
		return "", errors.New("this is already the newest version")
	}
	if r.Sums == nil || r.Sig == nil {
		return "", notSigned("the release has no signature")
	}
	for _, f := range []*Asset{a, r.Sums, r.Sig} {
		if !strings.HasPrefix(f.URL, downloadURL+r.Tag+"/") {
			return "", notSigned("the files aren't all from release " + r.Tag)
		}
	}
	key := embeddedKey
	sums, err := fetchSmall(ctx, r.Sums)
	if err != nil {
		return "", err
	}
	sig, err := fetchSmall(ctx, r.Sig)
	if err != nil {
		return "", err
	}
	hashes, err := Verify(r.Tag, sums, sig, key)
	if err != nil {
		return "", err
	}
	want, ok := hashes[assetName]
	if !ok {
		return "", notSigned(assetName + " isn't in the signed checksums")
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	resp, err := get(ctx, a.URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	path := filepath.Join(dir, newName)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, a.Size+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n != a.Size {
		err = errors.New("the download is incomplete")
	}
	if err == nil && ([sha256.Size]byte)(h.Sum(nil)) != want {
		err = notSigned("the download isn't the signed file")
	}
	if err == nil && a.SHA256 != "" && hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
		err = notSigned("the download doesn't match GitHub's checksum")
	}
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Syncer")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, errors.New("GitHub answered " + resp.Status)
	}
	return resp, nil
}

// fetchSmall reads a release's sums or signature file.
func fetchSmall(ctx context.Context, a *Asset) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()
	resp, err := get(ctx, a.URL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxSmall+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxSmall {
		return nil, notSigned("the signature files are too big")
	}
	return b, nil
}

// Names of the new exe while it downloads, and of the replaced one until it
// can be deleted (Windows lets a running exe be renamed, not deleted).
const (
	newName = "Syncer.exe.new"
	oldName = "Syncer.exe.old"
)

// Replace puts the downloaded file in place of exe. The running exe is moved
// aside first; Cleanup deletes it once no process uses it any more.
func Replace(exe, downloaded string) error {
	if personal {
		return ErrPersonalBuild
	}
	old := filepath.Join(filepath.Dir(exe), oldName)
	if err := os.Remove(old); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Still running from an earlier update (a background task, say):
		// move it out of the way under another name.
		if err := os.Rename(old, old+strconv.FormatInt(time.Now().UnixNano(), 36)); err != nil {
			return err
		}
	}
	if err := os.Rename(exe, old); err != nil {
		return err
	}
	if err := os.Rename(downloaded, exe); err != nil {
		if rerr := os.Rename(old, exe); rerr != nil {
			return errors.Join(err, rerr)
		}
		return err
	}
	return nil
}

// Cleanup deletes what earlier updates left next to exe. Files still in use
// stay until a later start.
func Cleanup(exe string) {
	dir := filepath.Dir(exe)
	ms, _ := filepath.Glob(filepath.Join(dir, oldName+"*"))
	for _, m := range append(ms, filepath.Join(dir, newName)) {
		_ = os.Remove(m)
	}
}

// Newer reports whether version tag a is newer than b ("v1.2.3", "1.2").
// A pre-release ("v1.2.0-rc1") is older than its release ("v1.2.0"), as in
// semantic versioning. Anything that isn't a version (like "dev") is never
// newer nor older.
func Newer(a, b string) bool {
	va, oka := parse(a)
	vb, okb := parse(b)
	if !oka || !okb {
		return false
	}
	return compare(va, vb) > 0
}

// Valid reports whether v is a release version (not e.g. "dev").
func Valid(v string) bool {
	_, ok := parse(v)
	return ok
}

type version struct {
	nums [3]int
	pre  []string // dot-separated pre-release identifiers; none for a release
}

func parse(v string) (version, bool) {
	var out version
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.Index(v, "+"); i >= 0 {
		v = v[:i] // build metadata
	}
	if i := strings.Index(v, "-"); i >= 0 {
		pre := v[i+1:]
		v = v[:i]
		out.pre = strings.Split(pre, ".")
		for _, id := range out.pre {
			if id == "" || strings.Trim(id, "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-") != "" {
				return version{}, false
			}
		}
	}
	parts := strings.Split(v, ".")
	if v == "" || len(parts) > 3 {
		return version{}, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return version{}, false
		}
		out.nums[i] = n
	}
	return out, true
}

// compare returns -1, 0 or 1 as a is older than, the same as or newer than b.
func compare(a, b version) int {
	for i := range a.nums {
		if a.nums[i] != b.nums[i] {
			return cmpInt(a.nums[i], b.nums[i])
		}
	}
	switch {
	case a.pre == nil && b.pre == nil:
		return 0
	case a.pre == nil:
		return 1 // a release is newer than its pre-releases
	case b.pre == nil:
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		if c := compareIdent(a.pre[i], b.pre[i]); c != 0 {
			return c
		}
	}
	return cmpInt(len(a.pre), len(b.pre))
}

// compareIdent orders pre-release identifiers: numbers numerically, before
// words, which compare as text.
func compareIdent(a, b string) int {
	na, erra := strconv.Atoi(a)
	nb, errb := strconv.Atoi(b)
	switch {
	case erra == nil && errb == nil:
		return cmpInt(na, nb)
	case erra == nil:
		return -1
	case errb == nil:
		return 1
	}
	return strings.Compare(a, b)
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
