package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ClaudeContext is how full one Claude Code session's context window is, as Claude Code itself
// reported it to its status line. A status line script saves that JSON into ContextFeedDir (see
// the README for the snippet); RepoDash only reads it.
type ClaudeContext struct {
	Dir  string    // the folder the session runs in
	Used int       // input + cache tokens of the last request: what the context holds now
	Size int       // the window, 200k or 1M
	At   time.Time // when the status line last wrote it
}

// ContextFeedDir is where status line scripts drop one <session_id>.json per session.
func ContextFeedDir() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if !filepath.IsAbs(base) { // the XDG spec says to ignore a relative path
		base = ""
	}
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "repodash", "claude")
}

// contextKeep is how long a session file counts. An idle session's file does not change, so this
// is generous; older files are removed (they are ours, written for us).
const contextKeep = 7 * 24 * time.Hour

var ctxCache = struct {
	sync.Mutex
	m map[string]ctxEntry
}{m: map[string]ctxEntry{}}

type ctxEntry struct {
	mod  time.Time
	size int64
	c    ClaudeContext
	ok   bool
}

// ClaudeContexts reads every session file in the feed folder. Files are parsed again only when
// they change, so calling this every second costs one directory read and a stat per session.
func ClaudeContexts() []ClaudeContext {
	dir := ContextFeedDir()
	if dir == "" {
		return nil
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	now := time.Now()
	var out []ClaudeContext
	seen := map[string]bool{}
	ctxCache.Lock()
	defer ctxCache.Unlock()
	for _, e := range ents {
		n := e.Name()
		if !strings.HasSuffix(n, ".json") || !e.Type().IsRegular() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		p := filepath.Join(dir, n)
		if now.Sub(fi.ModTime()) > contextKeep {
			os.Remove(p)
			continue
		}
		seen[p] = true
		ce, hit := ctxCache.m[p]
		if !hit || !ce.mod.Equal(fi.ModTime()) || ce.size != fi.Size() {
			ce = ctxEntry{mod: fi.ModTime(), size: fi.Size()}
			if b, err := os.ReadFile(p); err == nil && fi.Size() <= 1<<20 {
				ce.c, ce.ok = parseContext(b)
			}
			ce.c.At = fi.ModTime()
			ctxCache.m[p] = ce
		}
		if ce.ok {
			out = append(out, ce.c)
		}
	}
	for p := range ctxCache.m {
		if !seen[p] {
			delete(ctxCache.m, p)
		}
	}
	return out
}

// parseContext reads Claude Code's status line JSON (whole, or the fields the snippet keeps).
// current_usage is null until the session's first request; that is a valid file with Used 0.
func parseContext(b []byte) (ClaudeContext, bool) {
	var f struct {
		Cwd       string `json:"cwd"`
		Workspace struct {
			ProjectDir string `json:"project_dir"`
		} `json:"workspace"`
		ContextWindow struct {
			Size    int `json:"context_window_size"`
			Current *struct {
				Input       int `json:"input_tokens"`
				CacheCreate int `json:"cache_creation_input_tokens"`
				CacheRead   int `json:"cache_read_input_tokens"`
			} `json:"current_usage"`
		} `json:"context_window"`
	}
	if json.Unmarshal(b, &f) != nil {
		return ClaudeContext{}, false
	}
	c := ClaudeContext{Dir: f.Workspace.ProjectDir, Size: f.ContextWindow.Size}
	if c.Dir == "" {
		c.Dir = f.Cwd
	}
	if u := f.ContextWindow.Current; u != nil {
		c.Used = u.Input + u.CacheCreate + u.CacheRead
	}
	// sane numbers only, so percentages and colours cannot overflow
	const most = 1 << 30
	if c.Used < 0 || c.Used > most || c.Size < 0 || c.Size > most {
		return ClaudeContext{}, false
	}
	if c.Dir == "" || !filepath.IsAbs(c.Dir) {
		return ClaudeContext{}, false
	}
	c.Dir = filepath.Clean(c.Dir)
	return c, true
}

// ContextFor is the context of the session most recently active inside dir: tokens used and the
// window size, both 0 when there is none (or it has not sent a request yet, as after /clear).
// With two Claude sessions in one repo, the number follows whichever spoke last.
func ContextFor(cs []ClaudeContext, dir string) (used, size int) {
	var best *ClaudeContext
	for i, c := range cs {
		if c.Dir != dir && !strings.HasPrefix(c.Dir, dir+"/") {
			continue
		}
		if best == nil || c.At.After(best.At) {
			best = &cs[i]
		}
	}
	if best == nil {
		return 0, 0
	}
	return best.Used, best.Size
}

// FormatTokens writes a token count the way Claude Code's status line does: 950, 82k, 1.2M.
func FormatTokens(n int) string {
	switch {
	case n >= 999_500:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1e6), ".0") + "M"
	case n >= 1000:
		return fmt.Sprintf("%dk", (n+500)/1000)
	}
	return fmt.Sprint(n)
}
