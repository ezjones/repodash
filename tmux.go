package main

import (
	"errors"
	"os/exec"
	"strings"
)

func tmux(args ...string) (string, error) {
	b, err := exec.Command("tmux", args...).Output()
	return strings.TrimSpace(string(b)), err
}

// openInTmux brings the project at dir to the front of whatever tmux client you
// are attached to: it selects an existing window with a pane in that repo (an
// exact cwd match beats a subdirectory), or opens a new window there. This is
// the same rule as a tmux project picker: reuse the tab if one exists.
func openInTmux(dir string) (action, window string, err error) {
	out, err := tmux("list-panes", "-a", "-F", "#{session_name}\t#{window_id}\t#{window_index}\t#{pane_current_path}")
	if err != nil {
		return "", "", errors.New("no tmux server running")
	}
	var sub string
	for _, l := range strings.Split(out, "\n") {
		f := strings.SplitN(l, "\t", 4)
		if len(f) < 4 {
			continue
		}
		switch {
		case f[3] == dir:
			sub = ""
			if _, err := tmux("select-window", "-t", f[1]); err != nil {
				return "", "", err
			}
			return "jumped", f[2], nil
		case sub == "" && strings.HasPrefix(f[3], dir+"/"):
			sub = f[1] + "\t" + f[2]
		}
	}
	if sub != "" {
		id, idx, _ := strings.Cut(sub, "\t")
		if _, err := tmux("select-window", "-t", id); err != nil {
			return "", "", err
		}
		return "jumped", idx, nil
	}

	// No tab for it yet: open one in the session you used most recently.
	sess := ""
	if c, err := tmux("list-clients", "-F", "#{client_activity}\t#{session_name}"); err == nil && c != "" {
		best := ""
		for _, l := range strings.Split(c, "\n") {
			if a, s, ok := strings.Cut(l, "\t"); ok && a >= best {
				best, sess = a, s
			}
		}
	}
	if sess == "" {
		sess, _, _ = strings.Cut(strings.SplitN(out, "\n", 2)[0], "\t")
	}
	idx, err := tmux("new-window", "-t", sess+":", "-c", dir, "-P", "-F", "#{window_index}")
	if err != nil {
		return "", "", err
	}
	return "opened", idx, nil
}

// openInTerminal picks the multiplexer for terminal.multiplexer. The "prefer" modes look at both:
// an existing tab in the preferred one wins, then an existing tab in the other; with none, a new
// tab opens in the preferred one if its server is running, else in the other.
func openInTerminal(dir, override string) (action, window string, err error) {
	m := cfg.get().multiplexer
	if contains(multiplexers, override) {
		m = override
	}
	switch m {
	case "tmux":
		return openInTmux(dir)
	case "herdr":
		return openInHerdr(dir)
	}
	first, second := "tmux", "herdr"
	if m == "prefer-herdr" {
		first, second = second, first
	}
	open := map[string]func(string) (string, string, error){"tmux": openInTmux, "herdr": openInHerdr}
	running := map[string]func() bool{
		"tmux":  func() bool { _, e := tmux("list-sessions"); return e == nil },
		"herdr": func() bool { _, e := listHerdrPanes(); return e == nil },
	}
	panes := terminalPanes()
	for _, mux := range []string{first, second} {
		for _, p := range panes {
			if p.mux == mux && (p.cwd == dir || strings.HasPrefix(p.cwd, dir+"/")) {
				return open[mux](dir)
			}
		}
	}
	for _, mux := range []string{first, second} {
		if running[mux]() {
			return open[mux](dir)
		}
	}
	return "", "", errors.New("no tmux or herdr server running")
}
