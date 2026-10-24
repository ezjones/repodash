package server

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ezjones/repodash/internal/core"
)

// updater holds the server's self-update state. The core calls are fields so tests can fake them.
type updater struct {
	check      func(time.Time) (string, error)
	blockerFn  func(exe string) string
	apply      func(tag, exe string) error
	restart    func(exe string) error
	executable func() (string, error)
	hub        *hub
	delay      time.Duration // pause between the 200 answer and the restart

	mu      sync.Mutex
	latest  string
	blocker string
	failed  bool
	running bool
}

func newUpdater(h *hub) *updater {
	return &updater{
		check: core.CheckUpdate, blockerFn: core.UpdateBlocker, apply: core.ApplyUpdate,
		restart: core.Restart, executable: core.Executable, hub: h, delay: 300 * time.Millisecond,
	}
}

// payload is the JSON for GET /api/update and the SSE event.
func (u *updater) payload() []byte {
	u.mu.Lock()
	m := map[string]string{"version": core.Version, "latest": u.latest, "blocker": u.blocker}
	if u.failed {
		m["restart"] = "failed"
	}
	u.mu.Unlock()
	b, _ := json.Marshal(m)
	return b
}

// known reports whether a newer release is known (the SSE event is only sent then).
func (u *updater) known() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.latest != ""
}

// poll asks for a newer release and tells every open page when one is found.
func (u *updater) poll() {
	tag, err := u.check(time.Now())
	if err != nil || tag == "" {
		return
	}
	exe, err := u.executable()
	blocker := ""
	if err != nil {
		blocker = err.Error()
	} else {
		blocker = u.blockerFn(exe)
	}
	u.mu.Lock()
	u.latest, u.blocker = tag, blocker
	u.mu.Unlock()
	u.hub.broadcast("update", u.payload())
}

func (u *updater) loop() {
	time.Sleep(5 * time.Second)
	for {
		u.poll()
		time.Sleep(24 * time.Hour)
	}
}

func isLoopback(remote string) bool {
	h, _, err := net.SplitHostPort(remote)
	if err != nil {
		h = remote
	}
	ip := net.ParseIP(strings.Trim(h, "[]"))
	return ip != nil && ip.IsLoopback()
}

func (u *updater) get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(u.payload())
}

func (u *updater) post(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)
		return
	}
	if !isLoopback(r.RemoteAddr) {
		http.Error(w, "update only from the machine RepoDash runs on", http.StatusForbidden)
		return
	}
	u.mu.Lock()
	if u.running {
		u.mu.Unlock()
		http.Error(w, "update already running", http.StatusConflict)
		return
	}
	u.running = true
	u.mu.Unlock()
	done := false // set when the restart is scheduled; the lock then stays held
	defer func() {
		if !done {
			u.mu.Lock()
			u.running = false
			u.mu.Unlock()
		}
	}()

	exe, err := u.executable()
	if err != nil {
		http.Error(w, "update failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	if b := u.blockerFn(exe); b != "" {
		http.Error(w, b, http.StatusConflict)
		return
	}
	u.mu.Lock()
	tag := u.latest
	u.mu.Unlock()
	if tag == "" {
		if t, err := u.check(time.Now()); err == nil {
			tag = t
		}
		if tag == "" {
			http.Error(w, "up to date", http.StatusConflict)
			return
		}
	}
	if err := u.apply(tag, exe); err != nil {
		http.Error(w, "update failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	done = true
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"updated": tag})
	if fl, ok := w.(http.Flusher); ok {
		fl.Flush()
	}
	go func() {
		time.Sleep(u.delay)
		if err := u.restart(exe); err != nil {
			log.Printf("update: restart failed: %v", err)
			u.mu.Lock()
			u.failed = true
			u.running = false
			u.mu.Unlock()
		}
	}()
}
