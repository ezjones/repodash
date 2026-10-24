package main

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

type fakeTUI struct {
	m     *tuiModel
	opens [][2]string
	openE error
}

func newFakeTUI(t *testing.T, inHerdr bool) *fakeTUI {
	t.Helper()
	f := &fakeTUI{}
	repos := []Repo{
		{Name: "my-app", Path: "/r/my-app", Level: "bad", Activity: 3},
		{Name: "api-server", Path: "/r/api-server", Level: "ok", Activity: 2},
		{Name: "notes-cli", Path: "/r/notes-cli", Level: "warn", Activity: 1},
	}
	f.m = newTUIModel(tuiDeps{
		Scan:  func() []Repo { return repos },
		Panes: func() []pane { return nil },
		Find: func(dir string) (string, bool, error) {
			return "herdr", dir == "/r/my-app", nil
		},
		Open: func(mux, dir string) (string, string, error) {
			f.opens = append(f.opens, [2]string{mux, dir})
			return "jumped", "w", f.openE
		},
		Now:           func() time.Time { return time.Unix(1000, 0) },
		QuitAfterJump: inHerdr,
	})
	f.send(scanMsg(repos))
	return f
}

func (f *fakeTUI) send(msg tea.Msg) tea.Cmd {
	_, cmd := f.m.Update(msg)
	return cmd
}

func (f *fakeTUI) keys(ks ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range ks {
		var msg tea.KeyPressMsg
		switch k {
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "ctrl+d":
			msg = tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}
		default:
			r := []rune(k)[0]
			msg = tea.KeyPressMsg{Code: r, Text: k}
		}
		cmd = f.send(msg)
	}
	return cmd
}

func (f *fakeTUI) selectName(t *testing.T, name string) {
	t.Helper()
	for i, r := range f.m.view {
		if r.Name == name {
			f.m.sel = i
			return
		}
	}
	t.Fatalf("%s not in view", name)
}

func TestTUIMoveAndBounds(t *testing.T) {
	f := newFakeTUI(t, false)
	for i := 0; i < 5; i++ {
		f.keys("j")
		if f.m.sel < 0 || f.m.sel > 2 {
			t.Fatalf("sel %d out of range", f.m.sel)
		}
	}
	f.keys("g")
	if f.m.sel != 0 {
		t.Fatalf("g: sel %d", f.m.sel)
	}
	f.keys("G")
	if f.m.sel != 2 {
		t.Fatalf("G: sel %d", f.m.sel)
	}
	f.keys("ctrl+d")
	if f.m.sel != 2 {
		t.Fatalf("ctrl+d: sel %d", f.m.sel)
	}

	// Empty view: nothing panics, nothing opens.
	f.m.filter = "stale"
	f.m.rebuild()
	if len(f.m.view) != 0 {
		t.Fatal("expected an empty view")
	}
	f.keys("j", "G", "k", "ctrl+d", "enter", "n", "y")
	if cmd := f.keys("enter"); cmd != nil {
		t.Fatal("Enter on an empty view returned a command")
	}
	if f.m.prompt != "" || len(f.opens) != 0 {
		t.Fatal("empty view jumped or prompted")
	}
}

func TestTUISearch(t *testing.T) {
	f := newFakeTUI(t, false)
	f.keys("/", "a", "p", "i", "enter")
	if len(f.m.view) != 1 || f.m.view[0].Name != "api-server" {
		t.Fatalf("view after search: %+v", f.m.view)
	}
	f.keys("/", "esc")
	if len(f.m.view) != 3 {
		t.Fatalf("view after clear: %d", len(f.m.view))
	}
	// Live filtering, and a query with no match leaves an empty view without panicking.
	f.keys("/", "z", "z")
	if len(f.m.view) != 0 {
		t.Fatalf("no-match view: %d", len(f.m.view))
	}
	f.keys("enter", "enter", "G")
	f.keys("esc") // clears the kept query
	if len(f.m.view) != 3 || f.m.query != "" {
		t.Fatalf("esc did not clear the query: %d %q", len(f.m.view), f.m.query)
	}
}

func TestTUISortFilterCycle(t *testing.T) {
	f := newFakeTUI(t, false)
	if f.m.sortBy != "recent" {
		t.Fatalf("initial sort %q", f.m.sortBy)
	}
	for i := 0; i < 5; i++ {
		f.keys("s")
	}
	if f.m.sortBy != "recent" {
		t.Fatalf("sort after 5 presses: %q", f.m.sortBy)
	}
	var got []string
	for i := 0; i < 5; i++ {
		f.keys("f")
		got = append(got, f.m.filter)
	}
	want := []string{"bad", "warn", "ok", "stale", "all"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("f cycle: %v", got)
		}
	}
	f.keys("F")
	if f.m.filter != "stale" {
		t.Fatalf("F: %q", f.m.filter)
	}
	f.keys("S")
	if !f.m.rev {
		t.Fatal("S did not reverse")
	}
}

func TestTUIJumpExisting(t *testing.T) {
	f := newFakeTUI(t, true)
	f.selectName(t, "my-app")
	cmd := f.keys("enter")
	if cmd == nil || len(f.opens) != 0 {
		t.Fatal("Enter should return an Open command, not run it")
	}
	msg := cmd()
	if len(f.opens) != 1 || f.opens[0] != [2]string{"herdr", "/r/my-app"} {
		t.Fatalf("opens: %v", f.opens)
	}
	if cmd := f.send(msg); cmd == nil {
		t.Fatal("no quit command in herdr")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg, got %T", cmd())
	}

	// Outside herdr the TUI stays open and says what happened.
	g := newFakeTUI(t, false)
	g.selectName(t, "my-app")
	c := g.keys("enter")
	if cmd := g.send(c()); cmd != nil {
		t.Fatal("outside herdr must not quit")
	}
	if g.m.msg == "" {
		t.Fatal("no message after a jump")
	}
}

func TestTUIJumpPrompt(t *testing.T) {
	f := newFakeTUI(t, true)
	f.selectName(t, "api-server")
	if cmd := f.keys("enter"); cmd != nil {
		t.Fatal("Enter without a workspace must only prompt")
	}
	if f.m.prompt != "api-server" || len(f.opens) != 0 {
		t.Fatalf("prompt %q opens %v", f.m.prompt, f.opens)
	}
	if f.m.footer() != "Create a workspace for api-server? y/n" {
		t.Fatalf("footer %q", f.m.footer())
	}
	f.keys("n")
	if f.m.prompt != "" || len(f.opens) != 0 {
		t.Fatal("n should dismiss")
	}
	f.keys("enter")
	f.keys("esc")
	if f.m.prompt != "" || len(f.opens) != 0 {
		t.Fatal("esc should dismiss")
	}
	f.keys("enter")
	cmd := f.keys("y")
	if cmd == nil {
		t.Fatal("y returned no command")
	}
	cmd()
	if len(f.opens) != 1 || f.opens[0] != [2]string{"herdr", "/r/api-server"} {
		t.Fatalf("opens: %v", f.opens)
	}

	// n outside a prompt creates without asking.
	g := newFakeTUI(t, false)
	g.selectName(t, "notes-cli")
	if cmd := g.keys("n"); cmd == nil || g.m.prompt != "" {
		t.Fatal("n should open directly")
	}
}

func TestTUIJumpError(t *testing.T) {
	f := newFakeTUI(t, true)
	f.openE = errors.New("no herdr server running")
	f.selectName(t, "my-app")
	cmd := f.send(f.keys("enter")())
	if cmd != nil {
		t.Fatalf("an error must not quit")
	}
	if f.m.msg != "no herdr server running" {
		t.Fatalf("msg %q", f.m.msg)
	}

	// A Find error shows too, and keeps running.
	f.m.d.Find = func(string) (string, bool, error) { return "", false, errors.New("no tmux or herdr server running") }
	if cmd := f.keys("enter"); cmd != nil || f.m.msg == "" {
		t.Fatalf("find error: cmd=%v msg=%q", cmd, f.m.msg)
	}
}

func TestTUISelectionFollowsName(t *testing.T) {
	f := newFakeTUI(t, false)
	f.selectName(t, "notes-cli")
	f.send(scanMsg([]Repo{
		{Name: "notes-cli", Path: "/r/notes-cli", Level: "warn", Activity: 9},
		{Name: "my-app", Path: "/r/my-app", Level: "bad", Activity: 3},
	}))
	if r, _ := f.m.selected(); r.Name != "notes-cli" {
		t.Fatalf("selection moved to %q", r.Name)
	}
	f.send(scanMsg([]Repo{{Name: "my-app", Path: "/r/my-app", Level: "bad"}}))
	if f.m.sel < 0 || f.m.sel >= len(f.m.view) {
		t.Fatalf("sel %d out of range", f.m.sel)
	}
	f.send(scanMsg(nil))
	if _, ok := f.m.selected(); ok || f.m.sel != 0 {
		t.Fatal("empty scan should leave nothing selected")
	}
	_ = f.m.View()
}

func TestCachedIssuesShownAtOpen(t *testing.T) {
	m := newTUIModel(tuiDeps{
		Issues: func() map[string]repoIssues { return map[string]repoIssues{"my-app": {Open: 3}} },
		Now:    func() time.Time { return time.Unix(1000, 0) },
	})
	if m.issues["my-app"].Open != 3 {
		t.Fatalf("issues not loaded at open: %+v", m.issues)
	}
}

func TestStatusShowFalseSkipsFilter(t *testing.T) {
	s, _ := parseSettings("", []byte(`{"statuses":{"stale":{"show":false}}}`))
	old := Config()
	SetConfig(s)
	defer SetConfig(old)
	m := newTUIModel(tuiDeps{Now: time.Now})
	if slices.Contains(m.filters, "stale") || !slices.Contains(m.filters, "bad") {
		t.Fatalf("filters %v", m.filters)
	}
}

func TestTUIWebKey(t *testing.T) {
	f := newFakeTUI(t, false)
	calls := 0
	f.m.d.Web = func() (string, error) { calls++; return "http://127.0.0.1:8092", nil }
	cmd := f.keys("w")
	if cmd == nil {
		t.Fatal("w returned no command")
	}
	f.send(cmd())
	if calls != 1 || !strings.Contains(f.m.msg, "http://127.0.0.1:8092") {
		t.Errorf("calls %d, msg %q", calls, f.m.msg)
	}
	f.m.d.Web = func() (string, error) { return "", errors.New("port busy") }
	f.send(f.keys("w")())
	if f.m.msg != "port busy" {
		t.Errorf("msg %q", f.m.msg)
	}
}
