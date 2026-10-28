package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/ezjones/repodash/internal/core"
)

type event struct {
	name string
	data []byte
}

// hub keeps the latest scan and settings, and streams changes to every open
// browser tab. It only scans while someone is watching, so an idle dashboard
// costs nothing; it always watches the settings file (one stat per second).
type hub struct {
	scan func() []core.Repo

	mu       sync.Mutex
	last     []byte
	lastAt   time.Time
	clients  map[chan event]struct{}
	scanning bool
	again    bool              // refresh was asked for during a scan: scan once more after it
	dirs     map[string]string // repo name -> path, from the last scan
	agents   []byte            // last agent-state event
	issues   issueFeed         // GitHub issues feed (issues.go)
	upd      *updater          // self-update state (update.go); nil in tests
}

func newHub(scan func() []core.Repo) *hub {
	return &hub{scan: scan, clients: map[chan event]struct{}{}}
}

func (h *hub) run() {
	var lastScan time.Time
	for range time.Tick(time.Second) {
		if core.PollConfig() {
			s := core.Config()
			for _, w := range s.Warnings {
				log.Printf("settings: %s", w)
			}
			h.broadcast("config", s.Payload())
			h.refresh() // skip/hidden/cover/stale rules may have changed
			lastScan = time.Now()
			continue
		}
		h.mu.Lock()
		n := len(h.clients)
		h.mu.Unlock()
		if n > 0 {
			h.pollAgents()
		}
		if n > 0 && core.Config().IssuesOn && h.issues.due() {
			go h.pollIssues()
		}
		if n > 0 && time.Since(lastScan) >= core.Config().Interval {
			h.refresh()
			lastScan = time.Now()
		}
	}
}

func (h *hub) broadcast(name string, b []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c <- event{name, b}:
		default: // slow client: it gets the next one
		}
	}
}

// refresh rescans and broadcasts if anything changed. Calls during a scan collapse into one more
// scan after it, so a change made while a scan was running (a pin) is never lost to it.
func (h *hub) refresh() {
	h.mu.Lock()
	if h.scanning {
		h.again = true
		h.mu.Unlock()
		return
	}
	h.scanning = true
	h.mu.Unlock()

	for {
		list := h.scan()
		b, _ := json.Marshal(list)
		dirs := make(map[string]string, len(list))
		for _, r := range list {
			dirs[r.Name] = r.Path
		}

		h.mu.Lock()
		h.dirs = dirs
		changed := string(b) != string(h.last)
		h.last, h.lastAt = b, time.Now()
		again := h.again
		h.again = false
		h.scanning = again
		h.mu.Unlock()
		if changed {
			h.broadcast("repos", b)
		}
		if !again {
			return
		}
	}
}

// pollAgents is the cheap, fast half of the scan: one look at the terminal multiplexers,
// no git. It runs every second while a browser is open and broadcasts only when an agent
// state, a tab or its context size changed, so the badge follows the agent within about a second.
func (h *hub) pollAgents() {
	h.mu.Lock()
	dirs := h.dirs
	h.mu.Unlock()
	if len(dirs) == 0 {
		return
	}
	type st struct {
		Claude      string `json:"claude"`
		Tmux        bool   `json:"tmux"`
		Herdr       bool   `json:"herdr"`
		Context     int    `json:"context"`
		ContextSize int    `json:"context_size"`
	}
	panes := core.TerminalPanes()
	ctxs := core.ClaudeContexts()
	out := make(map[string]st, len(dirs))
	for n, d := range dirs {
		var s st
		s.Tmux, s.Herdr, s.Claude = core.AgentState(panes, d)
		if s.Claude != "" {
			s.Context, s.ContextSize = core.ContextFor(ctxs, d)
		}
		out[n] = s
	}
	b, _ := json.Marshal(out)
	h.mu.Lock()
	changed := string(b) != string(h.agents)
	h.agents = b
	h.mu.Unlock()
	if changed {
		h.broadcast("agents", b)
	}
}

func (h *hub) snapshot() []byte {
	h.mu.Lock()
	have := h.last != nil
	h.mu.Unlock()
	if !have {
		h.refresh()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.last
}

func (h *hub) serveSSE(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := make(chan event, 4)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	stale := h.last == nil || time.Since(h.lastAt) > 3*time.Second
	cur := h.last
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.clients, ch)
		h.mu.Unlock()
	}()

	send := func(e event) { fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.name, e.data); fl.Flush() }
	send(event{"config", core.Config().Payload()})
	if stale {
		go h.refresh()
	}
	if core.Config().IssuesOn {
		if iss := h.issues.current(); iss != nil {
			send(event{"issues", iss})
		}
	}
	if h.upd != nil && h.upd.known() {
		send(event{"update", h.upd.payload()})
	}
	if cur != nil {
		send(event{"repos", cur})
	} else {
		fmt.Fprint(w, ": connected\n\n")
		fl.Flush()
	}

	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			send(e)
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}
