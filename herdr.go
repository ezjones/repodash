package main

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// herdr runs one herdr CLI command and returns its stdout. herdr talks to its own server over a
// socket, so, like tmux, this works from any shell of the same user; the timeout keeps a hung
// server from stalling a scan.
func herdr(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "herdr", args...).Output()
}

type herdrPane struct {
	AgentStatus string `json:"agent_status"` // idle | working | blocked | done | unknown
	Cwd         string `json:"cwd"`
	FgCwd       string `json:"foreground_cwd"`
	TabID       string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
}

func listHerdrPanes() ([]herdrPane, error) {
	b, err := herdr("pane", "list")
	if err != nil {
		return nil, err
	}
	var env struct {
		Result struct {
			Panes []herdrPane `json:"panes"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, err
	}
	return env.Result.Panes, nil
}

// herdrPanes is the herdr counterpart of tmuxPanes. Herdr's own agent detection fills the
// status, mapped onto the words the card already uses (busy / waiting / idle).
func herdrPanes() []pane {
	hp, err := listHerdrPanes()
	if err != nil {
		return nil
	}
	var ps []pane
	for _, p := range hp {
		st := map[string]string{"working": "busy", "blocked": "waiting", "idle": "idle", "done": "idle"}[p.AgentStatus]
		for _, d := range []string{p.Cwd, p.FgCwd} {
			if d != "" {
				ps = append(ps, pane{cwd: d, claude: st, mux: "herdr"})
			}
		}
	}
	return ps
}

// openInHerdr mirrors openInTmux: focus the tab that has a pane in the repo (exact cwd beats a
// subdirectory), otherwise open a workspace there.
func openInHerdr(dir string) (action, window string, err error) {
	ps, err := listHerdrPanes()
	if err != nil {
		return "", "", errors.New("no herdr server running")
	}
	var sub *herdrPane
	var exact *herdrPane
	for i, p := range ps {
		for _, d := range []string{p.FgCwd, p.Cwd} {
			if d == dir && exact == nil {
				exact = &ps[i]
			} else if sub == nil && strings.HasPrefix(d, dir+"/") {
				sub = &ps[i]
			}
		}
	}
	if exact == nil {
		exact = sub
	}
	if exact != nil {
		if _, err := herdr("workspace", "focus", exact.WorkspaceID); err != nil {
			return "", "", err
		}
		if _, err := herdr("tab", "focus", exact.TabID); err != nil {
			return "", "", err
		}
		return "jumped", exact.TabID, nil
	}
	label := filepath.Base(dir)
	if _, err := herdr("workspace", "create", "--cwd", dir, "--label", label, "--focus"); err != nil {
		return "", "", err
	}
	return "opened", label, nil
}
