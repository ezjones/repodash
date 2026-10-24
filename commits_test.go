package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
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
