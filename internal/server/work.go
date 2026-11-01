package server

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/ezjones/repodash/internal/core"
)

var workTitles = map[string]string{"today": "Today", "yesterday": "Yesterday", "week": "This week", "month": "This month"}

// workFeed serves GET /api/work and caches each range for a few seconds, so several tabs
// do not each run a git log per repo.
type workFeed struct {
	hub *hub
	ttl time.Duration

	mu    sync.Mutex
	cache map[string]workEntry
}

type workEntry struct {
	at   time.Time
	body []byte
}

func newWorkFeed(h *hub) *workFeed {
	return &workFeed{hub: h, ttl: 10 * time.Second, cache: map[string]workEntry{}}
}

func (f *workFeed) get(w http.ResponseWriter, r *http.Request) {
	rng := r.URL.Query().Get("range")
	if rng == "" {
		rng = "today"
	}
	now := time.Now()
	from, to, ok := core.WorkBounds(rng, now)
	if !ok {
		http.Error(w, "unknown range", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock() // held during the git logs: a second tab waits and then hits the cache
	e, hit := f.cache[rng]
	if !hit || now.Sub(e.at) > f.ttl {
		var repos []core.Repo
		snap := f.hub.snapshot()
		_ = json.Unmarshal(snap, &repos)
		ws := core.Work(repos, from, to)
		if ws == nil {
			ws = []core.RepoWork{}
		}
		titles := map[string]string{}
		if m, ok := core.Lookup(core.Config().Raw, "repos").(map[string]any); ok {
			for name, v := range m {
				if rm, ok := v.(map[string]any); ok {
					if t, ok := rm["title"].(string); ok {
						titles[name] = t
					}
				}
			}
		}
		b, _ := json.Marshal(map[string]any{
			"range": rng, "from": from.Unix(), "to": to.Unix(), "repos": ws,
			"markdown": core.WorkMarkdown(workTitles[rng], ws, titles),
		})
		e = workEntry{at: now, body: b}
		if snap != nil { // no scan yet: answer, but do not keep the empty list
			f.cache[rng] = e
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(e.body)
}
