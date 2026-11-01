package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ezjones/repodash/internal/core"
)

func TestWorkGet(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "my-app")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "first change"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	cfg, _ := core.ParseSettings("", []byte(`{"repos":{"my-app":{"title":"My App"}}}`))
	old := core.Config()
	core.SetConfig(cfg)
	defer core.SetConfig(old)
	h := newHub(func() []core.Repo { return core.ScanAll(root) })
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/work", newWorkFeed(h).get)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/work?range=bogus", nil))
	if rec.Code != 400 {
		t.Fatalf("bad range: %d", rec.Code)
	}

	for _, url := range []string{"/api/work", "/api/work?range=today"} {
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", url, nil))
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", url, rec.Code, rec.Body)
		}
		var got struct {
			Range    string `json:"range"`
			From, To int64
			Repos    []struct {
				Name    string `json:"name"`
				Commits []struct {
					Subject string `json:"subject"`
				} `json:"commits"`
			} `json:"repos"`
			Markdown string `json:"markdown"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Range != "today" || len(got.Repos) != 1 || got.Repos[0].Name != "my-app" ||
			len(got.Repos[0].Commits) != 1 || got.Repos[0].Commits[0].Subject != "first change" ||
			!strings.HasPrefix(got.Markdown, "## Today") || !strings.Contains(got.Markdown, "### My App") {
			t.Fatalf("%s: %+v", url, got)
		}
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/work?range=yesterday", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"repos":[]`) {
		t.Fatalf("yesterday: %d %s", rec.Code, rec.Body)
	}
}
