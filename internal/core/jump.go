package core

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
)

// pickMux decides which multiplexer a jump to dir should use, without touching any of them.
// mode is one of multiplexers. exists reports whether that multiplexer already has a pane in dir
// (or below it). A fixed mode errors when its server is not running; the prefer modes pick the
// first multiplexer with a pane in dir, else the first one that is running.
func pickMux(mode string, panes []Pane, running func(mux string) bool, dir string) (mux string, exists bool, err error) {
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

// FindTarget is pickMux against the real terminals.
func FindTarget(dir, mode string) (mux string, exists bool, err error) {
	running := func(m string) bool {
		if m == "tmux" {
			_, e := tmux("list-sessions")
			return e == nil
		}
		_, e := listHerdrPanes()
		return e == nil
	}
	return pickMux(mode, TerminalPanes(), running, dir)
}

// OpenWith opens or focuses dir in the named multiplexer.
func OpenWith(mux, dir string) (action, window string, err error) {
	if mux == "herdr" {
		return openInHerdr(dir)
	}
	return openInTmux(dir)
}

// InHerdr reports whether this process runs inside a herdr pane.
func InHerdr() bool {
	return os.Getenv("HERDR_PANE_ID") != "" || os.Getenv("HERDR_ENV") != ""
}

// InHerdrPopup reports a herdr popup: herdr sets HERDR_ENV there but no pane id.
func InHerdrPopup() bool {
	return os.Getenv("HERDR_PANE_ID") == "" && os.Getenv("HERDR_ENV") != ""
}

// TUITitle is the terminal title the TUI sets, so herdrOpenTUI can find its tab again.
const TUITitle = "RepoDash"

// HerdrOpenTUI is the plugin's open action: focus the tab already showing the TUI, or open
// it in a new tab. A tab, not a popup: herdr popups take all input until they close.
func HerdrOpenTUI() error {
	ps, err := listHerdrPanes()
	if err != nil {
		return errors.New("no herdr server running")
	}
	for _, p := range ps {
		// The label is set at open; the title only once the TUI has drawn.
		if p.Label == TUITitle || p.Title == TUITitle {
			return herdrFocusTab(p.WorkspaceID, p.TabID)
		}
	}
	// Start in the home folder so the tab's cwd never matches a repo card.
	home, _ := os.UserHomeDir()
	b, err := herdr("plugin", "pane", "open", "--plugin", "repodash", "--entrypoint", "grid",
		"--placement", "tab", "--cwd", home, "--focus")
	if err != nil {
		return err
	}
	// --focus does not switch to the new tab, so focus it by id.
	var res struct {
		Result struct {
			PluginPane struct {
				Pane herdrPane `json:"pane"`
			} `json:"plugin_pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return err
	}
	p := res.Result.PluginPane.Pane
	return herdrFocusTab(p.WorkspaceID, p.TabID)
}

func herdrFocusTab(workspace, tab string) error {
	if _, err := herdr("workspace", "focus", workspace); err != nil {
		return err
	}
	_, err := herdr("tab", "focus", tab)
	return err
}
