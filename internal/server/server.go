package server

import (
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ezjones/repodash/internal/core"
)

//go:embed web
var webFS embed.FS

// Options configures Serve.
type Options struct {
	Root, Addr, AllowHost, LayoutPath string
	NoOpen                            bool
}

// Serve runs the web dashboard until the listener fails.
func Serve(o Options) error {
	layout := &layoutStore{path: o.LayoutPath}
	hub := newHub(func() []core.Repo { return core.ScanAll(o.Root) })
	go hub.run()
	upd := newUpdater(hub)
	hub.upd = upd
	if core.Config().UpdateCheck {
		go upd.loop()
	}

	sub, _ := fs.Sub(webFS, "web")
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServer(http.FS(sub)))
	mux.HandleFunc("GET /api/repos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(hub.snapshot())
	})
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(core.Config().Payload())
	})
	mux.HandleFunc("POST /api/refresh", func(w http.ResponseWriter, r *http.Request) {
		hub.refresh()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/open", func(w http.ResponseWriter, r *http.Request) {
		// Same CORS-preflight guard as the layout PUT: other web pages cannot drive tmux.
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)
			return
		}
		var req struct {
			Name string `json:"name"`
			Mux  string `json:"mux"` // this browser's choice; empty = the settings file
			// Worktree, when set, is a linked worktree of the repo to open instead of the repo.
			Worktree string `json:"worktree"`
		}
		json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
		// Only a directory that is really a repo under the root can be opened, never an arbitrary path.
		dir, ok := core.ValidRepo(o.Root, req.Name)
		if !ok {
			http.Error(w, "not a repo", http.StatusNotFound)
			return
		}
		if req.Worktree != "" {
			// Never open a path from the request unless git itself lists it as a worktree of this repo.
			if !core.WorktreeOf(dir, req.Worktree) {
				http.Error(w, "not a worktree of this repo", http.StatusNotFound)
				return
			}
			dir = req.Worktree
		}
		action, win, err := core.OpenInTerminal(dir, req.Mux)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"action": action, "window": win})
	})
	mux.HandleFunc("GET /img/{name}", serveImage(o.Root))
	mux.HandleFunc("PUT /api/image/{name}", imageWrite(putImage(o.Root)))
	mux.HandleFunc("DELETE /api/image/{name}", deleteImage(o.Root))
	mux.HandleFunc("GET /api/info/{name}", infoGet(o.Root))
	mux.HandleFunc("GET /api/commits/{name}", commitsGet(o.Root))
	mux.HandleFunc("GET /api/commit/{name}/{hash}", commitGet(o.Root))
	mux.HandleFunc("PUT /api/notes/{name}", notesPut(o.Root))
	mux.HandleFunc("GET /api/update", upd.get)
	mux.HandleFunc("POST /api/update", upd.post)
	mux.HandleFunc("GET /events", hub.serveSSE)
	mux.HandleFunc("GET /api/layout", layout.get)
	mux.HandleFunc("PUT /api/layout", layout.put)

	log.Printf("RepoDash: %s  (repos in %s, settings %s)", o.Addr, o.Root, core.ConfigPath())
	for _, w := range core.Config().Warnings {
		log.Printf("settings: %s", w)
	}
	allowed := map[string]bool{}
	if h, _, err := net.SplitHostPort(o.Addr); err == nil && h != "" {
		allowed[strings.ToLower(h)] = true
	}
	for _, h := range strings.Split(o.AllowHost, ",") {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			allowed[h] = true
		}
	}
	ln, err := net.Listen("tcp", o.Addr)
	if err != nil {
		return err
	}
	if !o.NoOpen && core.Interactive() {
		core.OpenBrowser(webURL(o.Addr))
	}
	return http.Serve(ln, guard(allowed, mux))
}

// layoutStore persists card positions, groups and kanban columns as one opaque
// JSON document; the browser owns its shape.
type layoutStore struct {
	mu   sync.Mutex
	path string
}

func (l *layoutStore) get(w http.ResponseWriter, r *http.Request) {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, err := os.ReadFile(l.path)
	if err != nil {
		b = []byte("{}")
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

func (l *layoutStore) put(w http.ResponseWriter, r *http.Request) {
	// Requiring a JSON content type forces a CORS preflight on cross-site
	// requests, which we never answer, so other web pages cannot overwrite it.
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)
		return
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil || !json.Valid(b) {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmp := l.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.Rename(tmp, l.path); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// imageWrite applies the same cross-site guard as the layout PUT: image/* is not
// a CORS-safelisted content type, so a foreign page's request needs a preflight
// that we never answer.
func imageWrite(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "image/") {
			http.Error(w, "content type must be image/*", http.StatusUnsupportedMediaType)
			return
		}
		next(w, r)
	}
}

// guard answers only requests that were really addressed to this server.
//
// DNS rebinding lets a web page on evil.example make the victim's browser resolve
// evil.example to 127.0.0.1 and then talk to us as if it were same-origin, which
// would defeat the content-type guards on the write routes and let the page read
// the repo list. The browser still sends Host: evil.example, so we refuse any
// host name we were not told about. IP literals cannot be rebound, so they pass;
// so does localhost. A browser also sends Origin on cross-site writes, which must
// match Host.
func guard(allowed map[string]bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.ToLower(strings.Trim(host, "[]"))
		if host != "localhost" && net.ParseIP(host) == nil && !allowed[host] {
			http.Error(w, "unrecognised Host header "+r.Host+"; start with -allow-host "+host+" if you meant to use this name", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && r.Method != http.MethodGet && r.Method != http.MethodHead {
			if u, err := url.Parse(o); err != nil || !strings.EqualFold(u.Host, r.Host) {
				http.Error(w, "cross-origin request refused", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
