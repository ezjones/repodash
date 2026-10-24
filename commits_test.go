package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCommitsGet(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "my-app")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "first"},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "second"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/commits/{name}", commitsGet(root))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/commits/my-app", nil))
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("code %d headers %v", rec.Code, rec.Header())
	}
	var got struct {
		Commits []struct {
			At      int64  `json:"at"`
			Subject string `json:"subject"`
		} `json:"commits"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Commits) != 2 || got.Commits[0].Subject != "second" || got.Commits[0].At == 0 {
		t.Fatalf("commits: %+v", got.Commits)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/commits/nope", nil))
	if rec.Code != 404 {
		t.Fatalf("unknown repo: %d", rec.Code)
	}
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", append([]string{"-c", "user.name=Pat Author", "-c", "user.email=t@example.com"}, args...)...)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

func TestCommitDetail(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "my-app")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "init", "-q")
	gitIn(t, dir, "remote", "add", "origin", "git@github.com:own/my-app.git")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "a.txt")
	gitIn(t, dir, "commit", "-q", "-m", "the subject\n\nthe body line")

	cs := recentCommits(dir, 5)
	if len(cs) != 1 || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(cs[0].Hash) {
		t.Fatalf("commits: %+v", cs)
	}
	ci, err := commitDetail(dir, cs[0].Hash)
	if err != nil {
		t.Fatal(err)
	}
	if ci.Hash != cs[0].Hash || ci.Author != "Pat Author" || ci.At == 0 || ci.Body != "the body line" {
		t.Fatalf("detail: %+v", ci)
	}
	if len(ci.Files) != 2 || !strings.Contains(ci.Files[0], "a.txt") || !strings.Contains(ci.Files[1], "1 file changed") {
		t.Fatalf("files: %q", ci.Files)
	}
	if ci2, err := commitDetail(dir, cs[0].Hash[:8]); err != nil || ci2.Hash != cs[0].Hash {
		t.Fatalf("short hash: %+v %v", ci2, err)
	}
	for _, bad := range []string{"--output=x", "HEAD", "zz", "", "ABCDEF1", cs[0].Hash + "0"} {
		if _, err := commitDetail(dir, bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "x")); err == nil {
		t.Error("option injection wrote a file")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/commits/{name}", commitsGet(root))
	mux.HandleFunc("GET /api/commit/{name}/{hash}", commitGet(root))
	get := func(u string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", u, nil))
		return rec
	}
	rec := get("/api/commits/my-app")
	var list struct {
		Commits []struct{ Hash string } `json:"commits"`
		Web     string                  `json:"web"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Commits) != 1 || list.Commits[0].Hash != cs[0].Hash {
		t.Fatalf("list: %s %v", rec.Body, err)
	}
	if list.Web != "https://github.com/own/my-app" {
		t.Fatalf("web: %q", list.Web)
	}
	rec = get("/api/commit/my-app/" + cs[0].Hash)
	var got commitInfo
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.Body != "the body line" {
		t.Fatalf("detail route: %d %v %s", rec.Code, rec.Header(), rec.Body)
	}
	if c := get("/api/commit/nope/" + cs[0].Hash).Code; c != 404 {
		t.Fatalf("unknown repo: %d", c)
	}
	if c := get("/api/commit/my-app/--output=x").Code; c != 400 {
		t.Fatalf("bad hash: %d", c)
	}
	if c := get("/api/commit/my-app/HEAD").Code; c != 400 {
		t.Fatalf("HEAD: %d", c)
	}
}
