package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ezjones/repodash/internal/core"
)

func TestPinPut(t *testing.T) {
	root, data := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "my-app", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	core.SetDataDirs(filepath.Join(data, "images"), filepath.Join(data, "notes"))
	defer core.SetDataDirs("", "")
	scans := make(chan struct{}, 4)
	h := newHub(func() []core.Repo { scans <- struct{}{}; return nil })
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/pins/{name}", pinPut(root, h))
	put := func(name, ctype, body string) int {
		req := httptest.NewRequest("PUT", "/api/pins/"+name, strings.NewReader(body))
		req.Header.Set("Content-Type", ctype)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}
	for _, c := range []struct {
		name, ctype, body string
		want              int
	}{
		{"my-app", "text/plain", `{"pinned":true}`, http.StatusUnsupportedMediaType},
		{"not-a-repo", "application/json", `{"pinned":true}`, http.StatusNotFound},
		{"my-app", "application/json", `{}`, http.StatusBadRequest},
		{"my-app", "application/json", `{"pinned":true}`, http.StatusNoContent},
	} {
		if got := put(c.name, c.ctype, c.body); got != c.want {
			t.Errorf("%s %s %s: %d want %d", c.name, c.ctype, c.body, got, c.want)
		}
	}
	if !core.ReadPins()["my-app"] {
		t.Error("my-app not pinned")
	}
	<-scans // the pin triggered a rescan
	if put("my-app", "application/json", `{"pinned":false}`) != http.StatusNoContent || core.ReadPins()["my-app"] {
		t.Error("my-app still pinned")
	}
}

func TestHiddenPut(t *testing.T) {
	root, data := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "my-app", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	core.SetDataDirs(filepath.Join(data, "images"), filepath.Join(data, "notes"))
	defer core.SetDataDirs("", "")
	scans := make(chan struct{}, 4)
	h := newHub(func() []core.Repo { scans <- struct{}{}; return nil })
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/hidden/{name}", hiddenPut(root, h))
	put := func(name, ctype, body string) int {
		req := httptest.NewRequest("PUT", "/api/hidden/"+name, strings.NewReader(body))
		req.Header.Set("Content-Type", ctype)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}
	for _, c := range []struct {
		name, ctype, body string
		want              int
	}{
		{"my-app", "text/plain", `{"hidden":true}`, http.StatusUnsupportedMediaType},
		{"not-a-repo", "application/json", `{"hidden":true}`, http.StatusNotFound},
		{"my-app", "application/json", `{}`, http.StatusBadRequest},
		{"my-app", "application/json", `{"hidden":true}`, http.StatusNoContent},
	} {
		if got := put(c.name, c.ctype, c.body); got != c.want {
			t.Errorf("%s %s %s: %d want %d", c.name, c.ctype, c.body, got, c.want)
		}
	}
	if !core.ReadHidden()["my-app"] {
		t.Error("my-app not hidden")
	}
	<-scans // the change triggered a rescan
	if put("my-app", "application/json", `{"hidden":false}`) != http.StatusNoContent || core.ReadHidden()["my-app"] {
		t.Error("my-app still hidden")
	}
}

// A refresh asked for while a scan runs is not dropped: one more scan follows.
func TestRefreshDuringScan(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	n := 0
	h := newHub(func() []core.Repo {
		n++
		if n == 1 {
			close(started)
			<-release
		}
		return nil
	})
	done := make(chan struct{})
	go func() { h.refresh(); close(done) }()
	<-started
	h.refresh()
	h.refresh()
	close(release)
	<-done
	if n != 2 {
		t.Errorf("%d scans, want 2", n)
	}
}
