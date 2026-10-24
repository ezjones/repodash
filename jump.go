package main

import (
	"errors"
	"os"
	"strings"
)

// pickMux decides which multiplexer a jump to dir should use, without touching any of them.
// mode is one of multiplexers. exists reports whether that multiplexer already has a pane in dir
// (or below it). A fixed mode errors when its server is not running; the prefer modes pick the
// first multiplexer with a pane in dir, else the first one that is running.
func pickMux(mode string, panes []pane, running func(mux string) bool, dir string) (mux string, exists bool, err error) {
	has := func(m string) bool {
		for _, p := range panes {
			if p.mux == m && (p.cwd == dir || strings.HasPrefix(p.cwd, dir+"/")) {
				return true
			}
		}
		return false
	}
	switch mode {
	case "tmux", "herdr":
		if !running(mode) {
			return "", false, errors.New("no " + mode + " server running")
		}
		return mode, has(mode), nil
	}
	first, second := "tmux", "herdr"
	if mode == "prefer-herdr" {
		first, second = second, first
	}
	for _, m := range []string{first, second} {
		if has(m) {
			return m, true, nil
		}
	}
	for _, m := range []string{first, second} {
		if running(m) {
			return m, false, nil
		}
	}
	return "", false, errors.New("no tmux or herdr server running")
}

// findTarget is pickMux against the real terminals.
func findTarget(dir, mode string) (mux string, exists bool, err error) {
	running := func(m string) bool {
		if m == "tmux" {
			_, e := tmux("list-sessions")
			return e == nil
		}
		_, e := listHerdrPanes()
		return e == nil
	}
	return pickMux(mode, terminalPanes(), running, dir)
}

// openWith opens or focuses dir in the named multiplexer.
func openWith(mux, dir string) (action, window string, err error) {
	if mux == "herdr" {
		return openInHerdr(dir)
	}
	return openInTmux(dir)
}

// inHerdr reports whether this process runs inside a herdr pane.
func inHerdr() bool {
	return os.Getenv("HERDR_PANE_ID") != "" || os.Getenv("HERDR_ENV") != ""
}
