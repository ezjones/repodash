package core

import "testing"

func TestPickMux(t *testing.T) {
	dir := "/r/my-app"
	up := func(string) bool { return true }
	only := func(m string) func(string) bool { return func(x string) bool { return x == m } }
	tm := Pane{cwd: dir, mux: "tmux"}
	hdSub := Pane{cwd: dir + "/web", mux: "herdr"}
	other := Pane{cwd: "/r/my-app2", mux: "herdr"} // prefix of a different repo must not match
	cases := []struct {
		name, mode string
		panes      []Pane
		running    func(string) bool
		mux        string
		exists     bool
		err        bool
	}{
		{"fixed herdr, tab in sub dir", "herdr", []Pane{hdSub}, up, "herdr", true, false},
		{"fixed herdr, none", "herdr", []Pane{tm, other}, up, "herdr", false, false},
		{"fixed herdr, server down", "herdr", nil, only("tmux"), "", false, true},
		{"prefer-tmux, tab only in herdr", "prefer-tmux", []Pane{hdSub}, up, "herdr", true, false},
		{"prefer-tmux, tabs in both", "prefer-tmux", []Pane{hdSub, tm}, up, "tmux", true, false},
		{"prefer-herdr, none, herdr down", "prefer-herdr", nil, only("tmux"), "tmux", false, false},
		{"prefer-herdr, none, both up", "prefer-herdr", []Pane{other}, up, "herdr", false, false},
		{"prefer-tmux, nothing running", "prefer-tmux", nil, func(string) bool { return false }, "", false, true},
	}
	for _, c := range cases {
		mux, ex, err := pickMux(c.mode, c.panes, c.running, dir)
		if mux != c.mux || ex != c.exists || (err != nil) != c.err {
			t.Errorf("%s: got %q %v %v, want %q %v err=%v", c.name, mux, ex, err, c.mux, c.exists, c.err)
		}
	}
}
