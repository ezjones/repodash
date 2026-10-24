package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIssueCacheRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "issues.json")
	in := IssueCache{At: time.Unix(1700000000, 0).UTC(), Repos: map[string]RepoIssues{
		"my-app": {Open: 4, Next: &IssueRef{Number: 44, Title: "dark mode"},
			OpenList: []IssueRef{{Number: 44, Title: "dark mode"}, {Number: 45, Title: "export csv"}}}}}
	if err := WriteIssueCache(p, in); err != nil {
		t.Fatal(err)
	}
	out, ok := ReadIssueCache(p)
	if !ok || out.Repos["my-app"].Open != 4 || len(out.Repos["my-app"].OpenList) != 2 || !out.At.Equal(in.At) {
		t.Fatalf("round trip: %+v %v", out, ok)
	}
	if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("cache file mode: %v %v", fi, err)
	}
	if es, _ := os.ReadDir(filepath.Dir(p)); len(es) != 1 {
		t.Fatalf("temp file left behind: %v", es)
	}
}

func TestIssueCacheBad(t *testing.T) {
	d := t.TempDir()
	if _, ok := ReadIssueCache(filepath.Join(d, "missing.json")); ok {
		t.Fatal("missing file read as ok")
	}
	bad := filepath.Join(d, "bad.json")
	os.WriteFile(bad, []byte("{not json"), 0o644)
	if _, ok := ReadIssueCache(bad); ok {
		t.Fatal("corrupt file read as ok")
	}
}

func TestIssueCachePath(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/x/cache")
	if got := IssueCachePath(); got != "/x/cache/repodash/issues.json" {
		t.Fatal(got)
	}
}
