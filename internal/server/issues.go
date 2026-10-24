package server

import (
	"bytes"
	"encoding/json"
	"github.com/ezjones/repodash/internal/core"
	"sync"
	"time"
)

// issueFeed keeps the last fetch and decides when the next one is due.
type issueFeed struct {
	mu      sync.Mutex
	payload []byte // {"repos":{name:{done,next}},"error":""}; nil until the first fetch
	at      time.Time
	busy    bool
}

func (f *issueFeed) current() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.payload
}

// due reports whether a fetch should start now and, if so, marks one as running.
func (f *issueFeed) due() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.busy || time.Since(f.at) < core.IssuesEvery {
		return false
	}
	f.busy = true
	return true
}

// poll fetches issues for every repo (a few at a time) and broadcasts if anything changed.
func (h *hub) pollIssues() {
	h.mu.Lock()
	dirs := h.dirs
	h.mu.Unlock()
	f := &h.issues
	if len(dirs) == 0 { // the first scan has not finished: try again on the next tick
		f.mu.Lock()
		f.busy = false
		f.mu.Unlock()
		return
	}
	defer func() { f.mu.Lock(); f.busy = false; f.at = time.Now(); f.mu.Unlock() }()

	res, msg := core.FetchAllIssues(dirs)
	if msg == "" {
		if p := core.IssueCachePath(); p != "" {
			_ = core.WriteIssueCache(p, core.IssueCache{At: time.Now().UTC(), Repos: res})
		}
	}
	b, _ := json.Marshal(map[string]any{"repos": res, "error": msg})
	f.mu.Lock()
	changed := !bytes.Equal(b, f.payload)
	f.payload = b
	f.mu.Unlock()
	if changed {
		h.broadcast("issues", b)
	}
}
