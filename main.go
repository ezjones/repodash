// RepoDash: a live dashboard of every git checkout under ~/gitrepos.
//
//	repodash                       serve the dashboard on 127.0.0.1:8092
//	repodash -addr :8092           reachable from other devices (tailnet, LAN)
//	repodash -json                 scan once, print JSON, exit (the future CLI starts here)
//	repodash -check                validate repodash.json, print problems, exit 1 if any
//	repodash -print-config         print every setting with its default value
//
// Settings live in repodash.json next to the binary (so they are part of the repo)
// and apply live. See AGENTS.md.
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
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
	"time"
)

//go:embed web
var webFS embed.FS

// version is set at release build time with -ldflags "-X main.buildVersion=...".
var buildVersion = "dev"

func main() {
	home, _ := os.UserHomeDir()
	root := flag.String("root", filepath.Join(home, "gitrepos"), "directory holding the repos")
	addr := flag.String("addr", "127.0.0.1:8092", "listen address")
	allowHost := flag.String("allow-host", "", "extra host names the page may be opened by, comma separated (needed when -addr is reachable by name, e.g. a tailnet name)")
	cfgPath := flag.String("config", "", "settings file (default: repodash.json next to the binary, or $REPODASH_CONFIG)")
	once := flag.Bool("json", false, "scan once, print JSON, exit")
	check := flag.Bool("check", false, "validate the settings file, print problems, exit 1 if there are any")
	noOpen := flag.Bool("no-open", false, "do not open the page in a browser on start")
	showVersion := flag.Bool("version", false, "print the version and exit")
	printCfg := flag.Bool("print-config", false, "print the default settings and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("repodash", buildVersion)
		return
	}

	if *printCfg {
		os.Stdout.Write(defaultConfigJSON)
		return
	}
	cfg.path = settingsPath(*cfgPath)
	if !*check && !*once {
		ensureConfigFile(cfg.path)
	}
	cfg.load()

	if *check {
		s := cfg.get()
		if len(s.Warnings) == 0 {
			fmt.Printf("%s: ok\n", cfg.path)
			return
		}
		fmt.Printf("%s: %d problem(s)\n", cfg.path, len(s.Warnings))
		for _, w := range s.Warnings {
			fmt.Println("  -", w)
		}
		os.Exit(1)
	}
	if *once {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(scanAll(*root))
		return
	}

	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		dataDir = filepath.Join(home, ".local", "share")
	}
	layout := &layoutStore{path: filepath.Join(dataDir, "repodash", "layout.json")}
	imageDir = filepath.Join(dataDir, "repodash", "images")
	hub := newHub(func() []Repo { return scanAll(*root) })
	go hub.run()

	sub, _ := fs.Sub(webFS, "web")
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServer(http.FS(sub)))
	mux.HandleFunc("GET /api/repos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(hub.snapshot())
	})
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(cfg.get().payload())
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
		}
		json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
		// Only a directory that is really a repo under the root can be opened, never an arbitrary path.
		dir, ok := validRepo(*root, req.Name)
		if !ok {
			http.Error(w, "not a repo", http.StatusNotFound)
			return
		}
		action, win, err := openInTerminal(dir, req.Mux)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"action": action, "window": win})
	})
	mux.HandleFunc("GET /img/{name}", serveImage(*root))
	mux.HandleFunc("PUT /api/image/{name}", imageWrite(putImage(*root)))
	mux.HandleFunc("DELETE /api/image/{name}", deleteImage(*root))
	mux.HandleFunc("GET /events", hub.serveSSE)
	mux.HandleFunc("GET /api/layout", layout.get)
	mux.HandleFunc("PUT /api/layout", layout.put)

	log.Printf("RepoDash: %s  (repos in %s, settings %s)", *addr, *root, cfg.path)
	for _, w := range cfg.get().Warnings {
		log.Printf("settings: %s", w)
	}
	allowed := map[string]bool{}
	if h, _, err := net.SplitHostPort(*addr); err == nil && h != "" {
		allowed[strings.ToLower(h)] = true
	}
	for _, h := range strings.Split(*allowHost, ",") {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			allowed[h] = true
		}
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	if !*noOpen && interactive() {
		openBrowser("http://" + *addr)
	}
	log.Fatal(http.Serve(ln, guard(allowed, mux)))
}

type event struct {
	name string
	data []byte
}

// hub keeps the latest scan and settings, and streams changes to every open
// browser tab. It only scans while someone is watching, so an idle dashboard
// costs nothing; it always watches the settings file (one stat per second).
type hub struct {
	scan func() []Repo

	mu       sync.Mutex
	last     []byte
	lastAt   time.Time
	clients  map[chan event]struct{}
	scanning bool
}

func newHub(scan func() []Repo) *hub {
	return &hub{scan: scan, clients: map[chan event]struct{}{}}
}

func (h *hub) run() {
	var lastScan time.Time
	for range time.Tick(time.Second) {
		if cfg.poll() {
			s := cfg.get()
			for _, w := range s.Warnings {
				log.Printf("settings: %s", w)
			}
			h.broadcast("config", s.payload())
			h.refresh() // skip/hidden/cover/stale rules may have changed
			lastScan = time.Now()
			continue
		}
		h.mu.Lock()
		n := len(h.clients)
		h.mu.Unlock()
		if n > 0 && time.Since(lastScan) >= cfg.get().interval {
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

// refresh rescans and broadcasts if anything changed. Overlapping calls collapse into one.
func (h *hub) refresh() {
	h.mu.Lock()
	if h.scanning {
		h.mu.Unlock()
		return
	}
	h.scanning = true
	h.mu.Unlock()

	b, _ := json.Marshal(h.scan())

	h.mu.Lock()
	h.scanning = false
	changed := string(b) != string(h.last)
	h.last, h.lastAt = b, time.Now()
	h.mu.Unlock()
	if changed {
		h.broadcast("repos", b)
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
	send(event{"config", cfg.get().payload()})
	if stale {
		go h.refresh()
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

// settingsPath picks the settings file: -config, then $REPODASH_CONFIG, then
// repodash.json beside the executable when that is a source checkout (go.mod
// next to it) or already has one, so a `go build` in the repo keeps its
// settings versioned with the code. An installed binary (bin dir) instead uses
// $XDG_CONFIG_HOME/repodash/repodash.json, so no config lands in a bin folder.
func settingsPath(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if v := os.Getenv("REPODASH_CONFIG"); v != "" {
		return v
	}
	beside := "repodash.json"
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		dir := filepath.Dir(exe)
		beside = filepath.Join(dir, "repodash.json")
		for _, marker := range []string{"go.mod", "repodash.json"} {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return beside
			}
		}
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "repodash", "repodash.json")
	}
	return beside
}
