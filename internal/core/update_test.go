package core

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func tarballWith(t *testing.T, script string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for _, e := range []struct{ name, body string }{{"README.md", "hi"}, {"repodash", script}} {
		tw.WriteHeader(&tar.Header{Name: e.name, Mode: 0o755, Size: int64(len(e.body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(e.body))
	}
	tw.Close()
	zw.Close()
	return buf.Bytes()
}

// releaseServer serves /latest as a 302 to tag, plus the tarball and checksums behind a redirect.
func releaseServer(t *testing.T, tag string, tarball []byte, sumOf []byte, hits *atomic.Int32) string {
	t.Helper()
	asset := "repodash_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	sum := sha256.Sum256(sumOf)
	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			hits.Add(1)
		}
		http.Redirect(w, r, "/tag/"+tag, http.StatusFound)
	})
	// Like GitHub, release downloads answer 302 to an asset host; the files live elsewhere.
	mux.HandleFunc("/download/"+tag+"/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/assets/"+strings.TrimPrefix(r.URL.Path, "/download/"+tag+"/"), http.StatusFound)
	})
	mux.HandleFunc("/assets/"+asset, func(w http.ResponseWriter, r *http.Request) { w.Write(tarball) })
	mux.HandleFunc("/assets/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%x  %s\n", sum, asset)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("REPODASH_UPDATE_URL", srv.URL)
	return srv.URL
}

func fakeExe(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "repodash")
	if err := os.WriteFile(p, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func onlyFile(t *testing.T, exe string) {
	t.Helper()
	es, _ := os.ReadDir(filepath.Dir(exe))
	if len(es) != 1 {
		t.Errorf("temp file left behind: %v", es)
	}
}

func TestLatestTag(t *testing.T) {
	releaseServer(t, "v1.2.3", nil, nil, nil)
	tag, err := latestTag(os.Getenv("REPODASH_UPDATE_URL"))
	if err != nil || tag != "v1.2.3" {
		t.Fatalf("got %q, %v", tag, err)
	}
	releaseServer(t, "latest-and-greatest", nil, nil, nil)
	if _, err := latestTag(os.Getenv("REPODASH_UPDATE_URL")); err == nil {
		t.Error("odd tag accepted")
	}
}

func TestNewerVersion(t *testing.T) {
	for _, c := range []struct {
		cur, latest string
		want        bool
	}{
		{"v0.7.9", "v0.7.10", true},
		{"v0.7.10", "v0.7.9", false},
		{"v0.7.3", "v0.7.3", false},
		{"v0.9.0", "v1.0.0", true},
		{"dev", "v1.0.0", false},
		{"v1.0.0", "dev", false},
		{"v1.0", "v1.0.1", false},
	} {
		if got := NewerVersion(c.cur, c.latest); got != c.want {
			t.Errorf("NewerVersion(%q,%q)=%v", c.cur, c.latest, got)
		}
	}
}

const goodScript = "#!/bin/sh\necho repodash v9.9.9\n"

func TestApplyUpdate(t *testing.T) {
	tb := tarballWith(t, goodScript)
	releaseServer(t, "v9.9.9", tb, tb, nil)
	exe := fakeExe(t)
	if err := ApplyUpdate("v9.9.9", exe); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(exe)
	if string(b) != goodScript {
		t.Errorf("exe not replaced: %q", b)
	}
	onlyFile(t, exe)
}

func TestApplyUpdateChecksumMismatch(t *testing.T) {
	tb := tarballWith(t, goodScript)
	releaseServer(t, "v9.9.9", tb, []byte("something else"), nil)
	exe := fakeExe(t)
	err := ApplyUpdate("v9.9.9", exe)
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %v", err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "old" {
		t.Error("exe changed")
	}
	onlyFile(t, exe)
}

func TestApplyUpdateWrongVersionOutput(t *testing.T) {
	tb := tarballWith(t, "#!/bin/sh\necho repodash v1.0.0\n")
	releaseServer(t, "v9.9.9", tb, tb, nil)
	exe := fakeExe(t)
	if err := ApplyUpdate("v9.9.9", exe); err == nil {
		t.Fatal("wrong version accepted")
	}
	if b, _ := os.ReadFile(exe); string(b) != "old" {
		t.Error("exe changed")
	}
	onlyFile(t, exe)
}

func TestUpdateBlocker(t *testing.T) {
	old := Version
	defer func() { Version = old }()
	dir := t.TempDir()
	exe := filepath.Join(dir, "repodash")

	Version = "dev"
	if got := UpdateBlocker(exe); !strings.HasPrefix(got, "built from source") {
		t.Errorf("dev: %q", got)
	}
	Version = "v1.0.0"
	if got := UpdateBlocker("/x/herdr/plugins/repodash/repodash"); !strings.HasPrefix(got, "installed by herdr") {
		t.Errorf("herdr: %q", got)
	}
	if got := UpdateBlocker(exe); got != "" && releaseSupported() {
		t.Errorf("writable dir: %q", got)
	}
	if os.Geteuid() != 0 {
		os.Chmod(dir, 0o555)
		defer os.Chmod(dir, 0o755)
		if got := UpdateBlocker(exe); !strings.HasPrefix(got, "can't write ") {
			t.Errorf("read-only: %q", got)
		}
	}
}

func TestCheckUpdate(t *testing.T) {
	old := Version
	defer func() { Version = old }()
	var hits atomic.Int32
	releaseServer(t, "v1.1.0", nil, nil, &hits)
	Version = "dev"
	if tag, err := CheckUpdate(time.Now()); tag != "" || err != nil || hits.Load() != 0 {
		t.Errorf("dev: %q %v hits=%d", tag, err, hits.Load())
	}
	Version = "v1.0.0"
	if tag, err := CheckUpdate(time.Now()); tag != "v1.1.0" || err != nil {
		t.Errorf("got %q %v", tag, err)
	}
	Version = "v1.1.0"
	if tag, _ := CheckUpdate(time.Now()); tag != "" {
		t.Errorf("up to date gave %q", tag)
	}
}

func TestCheckUpdateCache(t *testing.T) {
	old := Version
	defer func() { Version = old }()
	Version = "v1.0.0"
	var hits atomic.Int32
	releaseServer(t, "v1.1.0", nil, nil, &hits)
	t.Setenv("REPODASH_UPDATE_URL", "") // use the cache, in a temp folder
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	// the server URL is gone from the env, so fill the cache by hand and make sure nothing is asked
	now := time.Now()
	os.MkdirAll(filepath.Dir(updateCachePath()), 0o755)
	os.WriteFile(updateCachePath(), []byte(fmt.Sprintf(`{"at":%q,"latest":"v1.2.0"}`, now.Add(-time.Hour).Format(time.RFC3339))), 0o600)
	if tag, err := CheckUpdate(now); tag != "v1.2.0" || err != nil {
		t.Errorf("fresh cache: %q %v", tag, err)
	}
	if hits.Load() != 0 {
		t.Errorf("server hit %d times despite a fresh cache", hits.Load())
	}
}

func TestCheckUpdateNowSkipsCache(t *testing.T) {
	old := Version
	defer func() { Version = old; testReleasesURL = "" }()
	Version = "v1.0.0"
	var hits atomic.Int32
	testReleasesURL = releaseServer(t, "v1.1.0", nil, nil, &hits)
	t.Setenv("REPODASH_UPDATE_URL", "") // the cache is in use, in a temp folder
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	now := time.Now()
	os.MkdirAll(filepath.Dir(updateCachePath()), 0o755)
	os.WriteFile(updateCachePath(), []byte(fmt.Sprintf(`{"at":%q,"latest":"v1.0.0"}`, now.Add(-time.Hour).Format(time.RFC3339))), 0o600)
	if tag, _ := CheckUpdate(now); tag != "" || hits.Load() != 0 {
		t.Fatalf("daily check: %q, %d requests (the fresh cache should answer)", tag, hits.Load())
	}
	if tag, err := CheckUpdateNow(now); tag != "v1.1.0" || err != nil || hits.Load() != 1 {
		t.Fatalf("explicit check: %q %v, %d requests", tag, err, hits.Load())
	}
	if tag, _ := CheckUpdate(now); tag != "v1.1.0" || hits.Load() != 1 {
		t.Fatalf("cache not refreshed by the explicit check: %q, %d requests", tag, hits.Load())
	}
}
