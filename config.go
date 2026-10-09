package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// The settings file (repodash.json, kept in the repo next to the binary) is meant to be edited
// by people and agents, so the rules are forgiving and loud:
//   - every key is optional and falls back to default-config.json (embedded below)
//   - a wrong type, an out-of-range number or an unknown word is reported in
//     `warnings` and replaced by its default; the rest of the file still applies
//   - a file that is not valid JSON (an agent mid-edit) keeps the last good settings
//   - changes are picked up within about a second, no restart

//go:embed default-config.json
var defaultConfigJSON []byte

var (
	statusKeys   = []string{"bad", "warn", "ok", "stale"}
	barItems     = []string{"title", "live", "view", "sort", "reverse", "kanbanMode", "filters", "search", "agents", "spacer", "addGroup", "arrange", "cover", "theme", "appearance", "settings"}
	multiplexers = []string{"prefer-tmux", "prefer-herdr", "tmux", "herdr"}
	cardActions  = []string{"image", "terminal", "copy"}
	coverStyles  = []string{"aurora", "deep", "gradient", "name"}
	hexColor     = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
	freeForm     = map[string]bool{"repos": true} // maps whose keys are user data, not settings
)

// cfg is the live settings. Everything that reads configuration goes through cfg.get().
var cfg = &configStore{}

type repoCfg struct {
	Hidden bool
	Image  string
}

type settings struct {
	Raw      map[string]any // merged with defaults and validated; the browser gets this
	Warnings []string
	Path     string

	// server-side views of Raw
	skip        map[string]bool
	staleDays   int
	interval    time.Duration
	coverNames  []string
	coverDirs   []string
	coverReadme bool
	multiplexer string // prefer-tmux | prefer-herdr | tmux | herdr
	repos       map[string]repoCfg
}

func (s *settings) payload() []byte {
	b, _ := json.Marshal(map[string]any{"config": s.Raw, "warnings": s.Warnings, "path": s.Path})
	return b
}

func defaults() map[string]any {
	var m map[string]any
	if err := json.Unmarshal(defaultConfigJSON, &m); err != nil {
		panic("default-config.json is invalid: " + err.Error())
	}
	return m
}

func kindOf(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "true/false"
	case []any:
		return "list"
	case map[string]any:
		return "object"
	case nil:
		return "null"
	}
	return "?"
}

// merge lays the user's settings over the defaults, rejecting wrong types and unknown keys.
func merge(def, usr map[string]any, path string, w *[]string) map[string]any {
	out := make(map[string]any, len(def))
	for k, v := range def {
		out[k] = v
	}
	for k, uv := range usr {
		p := k
		if path != "" {
			p = path + "." + k
		}
		if strings.HasPrefix(k, "_") {
			continue
		}
		dv, known := def[k]
		if !known {
			if freeForm[path] {
				out[k] = uv
				continue
			}
			*w = append(*w, fmt.Sprintf("%s: unknown setting (ignored)", p))
			continue
		}
		dm, dIsMap := dv.(map[string]any)
		um, uIsMap := uv.(map[string]any)
		switch {
		case dIsMap && uIsMap && freeForm[p]:
			out[k] = um
		case dIsMap && uIsMap:
			out[k] = merge(dm, um, p, w)
		case kindOf(dv) == kindOf(uv):
			out[k] = uv
		default:
			*w = append(*w, fmt.Sprintf("%s: expected %s, got %s (using the default)", p, kindOf(dv), kindOf(uv)))
		}
	}
	return out
}

func get(m map[string]any, path string) any {
	var cur any = m
	for _, k := range strings.Split(path, ".") {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[k]
	}
	return cur
}

func set(m map[string]any, path string, v any) {
	parts := strings.Split(path, ".")
	for _, k := range parts[:len(parts)-1] {
		m = m[k].(map[string]any)
	}
	m[parts[len(parts)-1]] = v
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// validate checks values that merge cannot: enums, ranges, colours, list members.
func validate(m, def map[string]any, w *[]string) {
	warn := func(f string, a ...any) { *w = append(*w, fmt.Sprintf(f, a...)) }
	reset := func(path, why string) {
		set(m, path, get(def, path))
		warn("%s: %s (using the default, %v)", path, why, get(def, path))
	}
	oneOf := func(path string, opts ...string) {
		if v, _ := get(m, path).(string); !contains(opts, v) {
			reset(path, "must be one of "+strings.Join(opts, ", "))
		}
	}
	num := func(path string, lo, hi float64) {
		v, _ := get(m, path).(float64)
		if v < lo || v > hi || math.IsNaN(v) {
			reset(path, fmt.Sprintf("must be a number from %g to %g", lo, hi))
			return
		}
		set(m, path, math.Round(v))
	}
	color := func(path string) {
		if v, _ := get(m, path).(string); !hexColor.MatchString(v) {
			reset(path, "must be a hex colour like #c8362b")
		}
	}
	list := func(path string, allowed []string) {
		arr, _ := get(m, path).([]any)
		keep := make([]any, 0, len(arr))
		for _, e := range arr {
			s, ok := e.(string)
			switch {
			case !ok:
				warn("%s: dropped %v (list entries must be strings)", path, e)
			case allowed != nil && !contains(allowed, s):
				warn("%s: dropped %q (allowed: %s)", path, s, strings.Join(allowed, ", "))
			default:
				keep = append(keep, s)
			}
		}
		set(m, path, keep)
	}

	oneOf("theme", "auto", "light", "dark")
	oneOf("defaults.view", "auto", "grid", "canvas", "kanban")
	oneOf("defaults.kanbanMode", "status", "board")
	oneOf("defaults.sort", "recent", "activity", "language", "alpha")
	oneOf("terminal.multiplexer", multiplexers...)
	oneOf("card.coverStyle", coverStyles...)
	oneOf("defaults.filter", append([]string{"all"}, statusKeys...)...)
	num("scan.intervalSeconds", 2, 3600)
	num("scan.staleDays", 1, 3650)
	list("scan.skip", nil)
	list("topbar.items", barItems)
	list("topbar.filters", statusKeys)
	list("card.actions", cardActions)
	list("kanban.columns", nil)
	list("covers.names", nil)
	list("covers.dirs", nil)
	num("card.coverHeight", 40, 300)
	num("canvas.columns", 1, 12)
	num("canvas.cardWidth", 200, 600)
	num("canvas.gap", 0, 200)
	color("colors.accent")
	color("colors.accentDark")
	for _, k := range statusKeys {
		color("statuses." + k + ".color")
		color("statuses." + k + ".darkColor")
	}

	repos, _ := m["repos"].(map[string]any)
	for name, v := range repos {
		e, ok := v.(map[string]any)
		if !ok {
			warn("repos.%s: expected an object like {\"title\": \"...\"}, got %s (ignored)", name, kindOf(v))
			delete(repos, name)
			continue
		}
		for k, x := range e {
			want := map[string]string{"title": "string", "image": "string", "note": "string", "hidden": "true/false", "coverStyle": "string"}[k]
			switch {
			case strings.HasPrefix(k, "_"):
			case want == "":
				warn("repos.%s.%s: unknown setting (allowed: title, note, image, hidden, coverStyle)", name, k)
				delete(e, k)
			case kindOf(x) != want:
				warn("repos.%s.%s: expected %s, got %s (ignored)", name, k, want, kindOf(x))
				delete(e, k)
			case k == "coverStyle" && !contains(coverStyles, x.(string)):
				warn("repos.%s.coverStyle: %q is not one of %s (ignored)", name, x, strings.Join(coverStyles, ", "))
				delete(e, k)
			}
		}
	}
}

func strs(m map[string]any, path string) []string {
	var out []string
	arr, _ := get(m, path).([]any)
	for _, e := range arr {
		out = append(out, e.(string))
	}
	return out
}

func parseSettings(path string, data []byte) (*settings, error) {
	def := defaults()
	var usr map[string]any
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &usr); err != nil {
			return nil, err
		}
	}
	warns := []string{}
	m := merge(def, usr, "", &warns)
	validate(m, def, &warns)

	s := &settings{Raw: m, Warnings: warns, Path: path, skip: map[string]bool{}, repos: map[string]repoCfg{}}
	for _, n := range strs(m, "scan.skip") {
		s.skip[n] = true
	}
	s.staleDays = int(get(m, "scan.staleDays").(float64))
	s.interval = time.Duration(get(m, "scan.intervalSeconds").(float64)) * time.Second
	s.coverNames, s.coverDirs = strs(m, "covers.names"), strs(m, "covers.dirs")
	s.coverReadme, _ = get(m, "covers.readme").(bool)
	s.multiplexer, _ = get(m, "terminal.multiplexer").(string)
	for name, v := range m["repos"].(map[string]any) {
		e := v.(map[string]any)
		rc := repoCfg{}
		rc.Hidden, _ = e["hidden"].(bool)
		rc.Image, _ = e["image"].(string)
		s.repos[name] = rc
	}
	return s, nil
}

// describeJSONError adds line and column, which is what an editor or agent needs to find the typo.
func describeJSONError(data []byte, err error) string {
	var se *json.SyntaxError
	if errors.As(err, &se) {
		line := 1 + bytes.Count(data[:min(int(se.Offset), len(data))], []byte("\n"))
		col := int(se.Offset) - (bytes.LastIndexByte(data[:min(int(se.Offset), len(data))], '\n') + 1)
		return fmt.Sprintf("%v (line %d, column %d)", err, line, col)
	}
	return err.Error()
}

type configStore struct {
	path     string
	cur      atomic.Pointer[settings]
	lastMod  time.Time
	lastSize int64
}

func (c *configStore) get() *settings { return c.cur.Load() }

// load reads the file. Invalid JSON never replaces working settings.
func (c *configStore) load() {
	data, err := os.ReadFile(c.path)
	if st, e := os.Stat(c.path); e == nil {
		c.lastMod, c.lastSize = st.ModTime(), st.Size()
	}
	if err != nil && !os.IsNotExist(err) {
		data = nil
	}
	s, perr := parseSettings(c.path, data)
	if perr != nil {
		msg := fmt.Sprintf("%s is not valid JSON: %s", filepath.Base(c.path), describeJSONError(data, perr))
		if prev := c.cur.Load(); prev != nil {
			cp := *prev
			cp.Warnings = []string{msg + ". Keeping the last good settings."}
			c.cur.Store(&cp)
			return
		}
		s, _ = parseSettings(c.path, nil)
		s.Warnings = []string{msg + ". Using defaults."}
	}
	c.cur.Store(s)
}

// poll reloads when the file changed and reports whether it did.
func (c *configStore) poll() bool {
	st, err := os.Stat(c.path)
	if err != nil || (st.ModTime().Equal(c.lastMod) && st.Size() == c.lastSize) {
		return false
	}
	c.load()
	return true
}

// ensureFile writes the full default file the first time, so there is something to edit.
func ensureConfigFile(path string) {
	if _, err := os.Stat(path); err == nil {
		return
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, defaultConfigJSON, 0o644)
}
