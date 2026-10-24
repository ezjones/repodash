package core

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Version is the release this binary was built as ("dev" for a source build); main sets it.
var Version = "dev"

const (
	defaultReleasesURL = "https://github.com/ezjones/repodash/releases"
	updateEvery        = 24 * time.Hour
	maxTarball         = 64 << 20
	maxBinary          = 128 << 20
)

var tagRe = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// releasesURL is the release source; REPODASH_UPDATE_URL replaces it (tests only).
func releasesURL() string {
	if u := os.Getenv("REPODASH_UPDATE_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return defaultReleasesURL
}

func parseVersion(v string) ([3]int, bool) {
	var n [3]int
	if !tagRe.MatchString(v) {
		return n, false
	}
	for i, p := range strings.Split(v[1:], ".") {
		x, err := strconv.Atoi(p)
		if err != nil {
			return n, false
		}
		n[i] = x
	}
	return n, true
}

// NewerVersion reports whether latest is a higher vX.Y.Z than cur. Either not parsing gives false.
func NewerVersion(cur, latest string) bool {
	a, ok1 := parseVersion(cur)
	b, ok2 := parseVersion(latest)
	if !ok1 || !ok2 {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return b[i] > a[i]
		}
	}
	return false
}

// updateClient asks where /latest points; it must not follow the redirect.
func updateClient() *http.Client {
	return &http.Client{
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// downloadClient fetches release files. GitHub answers them with a 302 to its asset host, so it
// follows a few redirects, never from https down to http. The checksum vouches for the bytes.
func downloadClient() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
				return errors.New("redirect away from https refused")
			}
			return nil
		},
	}
}

// latestTag asks the releases page where /latest redirects to, without following it.
func latestTag(base string) (string, error) {
	resp, err := updateClient().Get(base + "/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	loc := resp.Header.Get("Location")
	if resp.StatusCode < 300 || resp.StatusCode > 399 || loc == "" {
		return "", fmt.Errorf("latest release: unexpected answer %s", resp.Status)
	}
	tag := loc[strings.LastIndex(loc, "/")+1:]
	if !tagRe.MatchString(tag) {
		return "", fmt.Errorf("latest release: odd tag %q", tag)
	}
	return tag, nil
}

type updateCache struct {
	At     time.Time `json:"at"`
	Latest string    `json:"latest"`
}

func updateCachePath() string {
	p := IssueCachePath()
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), "update.json")
}

// CheckUpdate returns the newest release tag when it is newer than Version, else "". It asks
// the network at most once a day (REPODASH_UPDATE_URL set: always, and the cache is not used).
func CheckUpdate(now time.Time) (string, error) {
	if _, ok := parseVersion(Version); !ok {
		return "", nil
	}
	useCache := os.Getenv("REPODASH_UPDATE_URL") == ""
	path := ""
	if useCache {
		path = updateCachePath()
	}
	latest := ""
	fresh := false
	if path != "" {
		var c updateCache
		if b, err := os.ReadFile(path); err == nil && json.Unmarshal(b, &c) == nil &&
			!c.At.After(now) && now.Sub(c.At) < updateEvery {
			latest, fresh = c.Latest, true
		}
	}
	if !fresh {
		tag, err := latestTag(releasesURL())
		if err != nil {
			return "", err
		}
		latest = tag
		if path != "" {
			if b, err := json.Marshal(updateCache{now, latest}); err == nil {
				if os.MkdirAll(filepath.Dir(path), 0o755) == nil {
					os.WriteFile(path, b, 0o600)
				}
			}
		}
	}
	if NewerVersion(Version, latest) {
		return latest, nil
	}
	return "", nil
}

func releaseSupported() bool {
	return (runtime.GOOS == "linux" || runtime.GOOS == "darwin") &&
		(runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64")
}

// UpdateBlocker returns "" when this binary can replace itself, else a short sentence saying why not.
func UpdateBlocker(exe string) string {
	if strings.Contains(filepath.ToSlash(exe), "/herdr/plugins/") {
		return "installed by herdr: run herdr plugin update repodash"
	}
	if _, ok := parseVersion(Version); !ok {
		return "built from source: git pull and rebuild"
	}
	dir := filepath.Dir(exe)
	f, err := os.CreateTemp(dir, ".repodash-write-test-*")
	if err != nil {
		return "can't write " + dir + ": rerun the install command"
	}
	f.Close()
	os.Remove(f.Name())
	if !releaseSupported() {
		return "no release build for " + runtime.GOOS + "/" + runtime.GOARCH
	}
	return ""
}

func download(url string, limit int64) ([]byte, error) {
	resp, err := downloadClient().Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url[strings.LastIndex(url, "/")+1:], resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("download too large")
	}
	return b, nil
}

// ApplyUpdate downloads release tag, checks it and replaces the file exe with it.
// On any error exe is untouched.
func ApplyUpdate(tag, exe string) error {
	if !tagRe.MatchString(tag) {
		return fmt.Errorf("bad release tag %q", tag)
	}
	asset := "repodash_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	base := releasesURL() + "/download/" + tag + "/"
	tarball, err := download(base+asset, maxTarball)
	if err != nil {
		return err
	}
	sums, err := download(base+"checksums.txt", maxTarball)
	if err != nil {
		return err
	}
	want := ""
	for _, ln := range strings.Split(string(sums), "\n") {
		f := strings.Fields(ln)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == asset {
			want = strings.ToLower(f[0])
		}
	}
	if want == "" {
		return fmt.Errorf("checksums.txt has no line for %s", asset)
	}
	sum := sha256.Sum256(tarball)
	if hex.EncodeToString(sum[:]) != want {
		return errors.New("checksum mismatch: download refused")
	}
	zr, err := gzip.NewReader(bytes.NewReader(tarball))
	if err != nil {
		return err
	}
	tr := tar.NewReader(zr)
	var bin []byte
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg || h.Name != "repodash" {
			continue
		}
		bin, err = io.ReadAll(io.LimitReader(tr, maxBinary+1))
		if err != nil {
			return err
		}
		if len(bin) > maxBinary {
			return errors.New("binary in the release is too large")
		}
		break
	}
	if bin == nil {
		return errors.New("no repodash file in the release")
	}
	f, err := os.CreateTemp(filepath.Dir(exe), ".repodash-update-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(tmp)
		}
	}()
	if _, err := f.Write(bin); err != nil {
		f.Close()
		return err
	}
	mode := os.FileMode(0o755)
	if st, err := os.Stat(exe); err == nil {
		mode = st.Mode().Perm() | 0o100
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, tmp, "-version").Output()
	if err != nil {
		return fmt.Errorf("new binary will not start: %w", err)
	}
	if got := strings.TrimSpace(string(out)); got != "repodash "+tag {
		return fmt.Errorf("new binary says %q, expected %q", got, "repodash "+tag)
	}
	if err := os.Rename(tmp, exe); err != nil {
		return err
	}
	ok = true
	return nil
}

// Executable is the running binary's real path.
func Executable() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(p)
}
