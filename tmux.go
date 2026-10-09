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
