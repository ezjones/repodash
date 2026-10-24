package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakes struct {
	applied   []string
	restarted chan string
	applyErr  error
	blocker   string
}

func newFake(f *fakes, latest string) *updater {
	f.restarted = make(chan string, 4)
	return &updater{
		check:      func(time.Time) (string, error) { return latest, nil },
		blockerFn:  func(string) string { return f.blocker },
		apply:      func(tag, exe string) error { f.applied = append(f.applied, tag); return f.applyErr },
		restart:    func(exe string) error { f.restarted <- exe; return nil },
		executable: func() (string, error) { return "/fake/repodash", nil },
		hub:        newHub(nil),
		latest:     latest,
	}
}

func post(u *updater, ct, remote string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/api/update", strings.NewReader("{}"))
	if ct != "" {
		r.Header.Set("Content-Type", ct)
	}
	r.RemoteAddr = remote
	r.Host = "127.0.0.1:8092"
	w := httptest.NewRecorder()
	u.post(w, r)
	return w
}

func TestUpdateGet(t *testing.T) {
	u := newFake(&fakes{}, "v9.9.9")
	u.blocker = "nope"
	w := httptest.NewRecorder()
	u.get(w, httptest.NewRequest("GET", "/api/update", nil))
	b := w.Body.String()
	if !strings.Contains(b, `"latest":"v9.9.9"`) || !strings.Contains(b, `"blocker":"nope"`) || !strings.Contains(b, `"version"`) {
		t.Fatal(b)
	}
}

func TestUpdatePostGuards(t *testing.T) {
	f := &fakes{}
	u := newFake(f, "v9.9.9")
	if w := post(u, "text/plain", "127.0.0.1:1"); w.Code != http.StatusUnsupportedMediaType {
		t.Fatal(w.Code)
	}
	if w := post(u, "application/json", "100.64.0.5:1"); w.Code != http.StatusForbidden {
		t.Fatal(w.Code)
	}
	// A local reverse proxy connects from loopback but carries the outside name.
	r := httptest.NewRequest("POST", "/api/update", strings.NewReader("{}"))
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr, r.Host = "127.0.0.1:5", "myhost.example.ts.net"
	pw := httptest.NewRecorder()
	u.post(pw, r)
	if pw.Code != http.StatusForbidden {
		t.Fatal("proxied request allowed:", pw.Code)
	}
	f.blocker = "built from source"
	if w := post(u, "application/json", "[::1]:1"); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "built from source") {
		t.Fatal(w.Code, w.Body)
	}
	if len(f.applied) != 0 {
		t.Fatal("apply called")
	}
}

func TestUpdateUpToDate(t *testing.T) {
	f := &fakes{}
	u := newFake(f, "")
	if w := post(u, "application/json", "127.0.0.1:1"); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "up to date") {
		t.Fatal(w.Code, w.Body)
	}
}

func TestUpdateSuccess(t *testing.T) {
	f := &fakes{}
	u := newFake(f, "v9.9.9")
	w := post(u, "application/json", "127.0.0.1:1")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"updated":"v9.9.9"`) {
		t.Fatal(w.Code, w.Body)
	}
	if len(f.applied) != 1 || f.applied[0] != "v9.9.9" {
		t.Fatal(f.applied)
	}
	select {
	case exe := <-f.restarted:
		if exe != "/fake/repodash" {
			t.Fatal(exe)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no restart")
	}
	if w := post(u, "application/json", "127.0.0.1:1"); w.Code != http.StatusConflict {
		t.Fatal("second update should be refused", w.Code)
	}
	if len(f.restarted) != 0 {
		t.Fatal("restarted twice")
	}
}

func TestUpdateApplyError(t *testing.T) {
	f := &fakes{applyErr: errors.New("boom")}
	u := newFake(f, "v9.9.9")
	w := post(u, "application/json", "127.0.0.1:1")
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "update failed: boom") {
		t.Fatal(w.Code, w.Body)
	}
	time.Sleep(50 * time.Millisecond)
	if len(f.restarted) != 0 {
		t.Fatal("restart called")
	}
}

func TestUpdateRestartFailure(t *testing.T) {
	f := &fakes{}
	u := newFake(f, "v9.9.9")
	u.restart = func(string) error { return errors.New("no exec") }
	post(u, "application/json", "127.0.0.1:1")
	time.Sleep(100 * time.Millisecond)
	if !strings.Contains(string(u.payload()), `"restart":"failed"`) {
		t.Fatal(string(u.payload()))
	}
}
