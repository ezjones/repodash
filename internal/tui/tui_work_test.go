package tui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ezjones/repodash/internal/core"
)

const workBase = int64(1_700_000_000)

// workFake is a fake TUI at 120x40 with a fake Work dep that records the ranges it was asked.
func workFake(t *testing.T) (*fakeTUI, *[]string, *[]core.TUIPrefs) {
	t.Helper()
	f := newFakeTUI(t, false)
	var asked []string
	var saved []core.TUIPrefs
	f.m.d.Work = func(repos []core.Repo, rng string) []core.RepoWork {
		asked = append(asked, rng)
		if rng == "yesterday" {
			return nil
		}
		return []core.RepoWork{
			{Name: "api-server", Last: workBase + 60, Commits: []core.WorkCommit{
				{Hash: "h1", At: workBase + 60, Subject: "fix-the-router"}, {Hash: "h2", At: workBase, Subject: "add-the-route"}}},
			{Name: "my-app", Last: workBase - 100, Uncommitted: 4, Edited: workBase - 100},
		}
	}
	f.m.d.SavePrefs = func(change func(*core.TUIPrefs)) error {
		var p core.TUIPrefs
		change(&p)
		saved = append(saved, p)
		return nil
	}
	sized(f, 120, 40)
	return f, &asked, &saved
}

// run executes a command (and the ones batched in it) and feeds the messages back.
func (f *fakeTUI) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			f.run(c)
		}
	case workMsg, jumpMsg:
		f.run(f.send(msg))
	}
}

var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

// screen is the drawn screen without colour codes.
func (f *fakeTUI) screen() string { return ansiRE.ReplaceAllString(f.m.View().Content, "") }

func TestTodayToggleCycle(t *testing.T) {
	f, asked, saved := workFake(t)
	if f.m.panel {
		t.Fatal("inspector should start hidden")
	}
	f.run(f.keys("t"))
	if !f.m.panel || f.m.panelMode != "today" {
		t.Fatalf("t from hidden: panel=%v mode=%q", f.m.panel, f.m.panelMode)
	}
	f.run(f.keys("t"))
	if !f.m.panel || f.m.panelMode != "" {
		t.Fatalf("t in Today should go back to Repo: panel=%v mode=%q", f.m.panel, f.m.panelMode)
	}
	f.run(f.keys("t"))
	if f.m.panelMode != "today" {
		t.Fatal("t in Repo should switch to Today")
	}
	if len(*asked) != 1 || (*asked)[0] != "today" {
		t.Fatalf("work should be computed once for today: %v", *asked)
	}
	if n := len(*saved); n != 3 || (*saved)[0].InspectorMode != "today" || (*saved)[1].InspectorMode != "" {
		t.Fatalf("mode saves: %+v", *saved)
	}
}

func TestTodayPanelContent(t *testing.T) {
	f, _, _ := workFake(t)
	cmd := f.keys("t")
	if s := f.screen(); !strings.Contains(s, "loading…") {
		t.Fatalf("expected loading… before the result:\n%s", s)
	}
	f.run(cmd)
	s := f.screen()
	hm := time.Unix(workBase+60, 0).Format("15:04")
	for _, want := range []string{"This repo", "All", "yesterday", "this week"[:4], "2 commits in 1 repo ·", "1 with uncommitted",
		"api-server", "2 commits · " + hm, hm + " fix-the-router", "4 files uncommitted, edited " + time.Unix(workBase-100, 0).Format("15:04")} {
		if !strings.Contains(s, want) {
			t.Errorf("panel lacks %q:\n%s", want, s)
		}
	}
	f.m.workRange, f.m.workFor = "week", "week"
	if !strings.Contains(f.screen(), time.Unix(workBase+60, 0).Format("Mon 15:04")+" fix-the-router") {
		t.Error("week range should show the weekday")
	}
}

func TestTodayRangeKeys(t *testing.T) {
	f, asked, saved := workFake(t)
	f.run(f.keys("t"))
	f.run(f.keys("]"))
	if f.m.workRange != "yesterday" || len(*asked) != 2 || (*asked)[1] != "yesterday" {
		t.Fatalf("] range=%q asked=%v", f.m.workRange, *asked)
	}
	if !strings.Contains(f.screen(), "Nothing committed or edited yesterday.") {
		t.Errorf("empty text missing:\n%s", f.screen())
	}
	f.run(f.keys("["))
	if f.m.workRange != "today" || len(*asked) != 3 {
		t.Fatalf("[ range=%q asked=%v", f.m.workRange, *asked)
	}
	if last := (*saved)[len(*saved)-1]; last.WorkRange != "" {
		t.Errorf("today is saved as empty: %+v", last)
	}
	f.run(f.keys("]", "]"))
	if f.m.workRange != "week" || (*saved)[len(*saved)-1].WorkRange != "week" {
		t.Errorf("week not reached or saved: %q %+v", f.m.workRange, *saved)
	}
}

func TestTodayThrottle(t *testing.T) {
	f, asked, _ := workFake(t)
	now := time.Unix(5000, 0)
	f.m.d.Now = func() time.Time { return now }
	f.run(f.keys("t"))
	f.run(f.send(scanMsg(f.m.repos)))
	if len(*asked) != 1 {
		t.Fatalf("a scan within 15 s must not recompute: %v", *asked)
	}
	now = now.Add(16 * time.Second)
	f.run(f.send(scanMsg(f.m.repos)))
	if len(*asked) != 2 {
		t.Fatalf("a scan after 15 s should recompute: %v", *asked)
	}
}

func TestTodayJump(t *testing.T) {
	f, _, _ := workFake(t)
	f.run(f.keys("t"))
	f.screen()
	f.keys("J") // first line: api-server header
	if r, _ := f.m.selected(); r.Name != "api-server" {
		t.Fatalf("moving onto a line selects its card, got %s", r.Name)
	}
	if f.m.panelMode != "today" {
		t.Fatal("must stay in Today")
	}
	f.keys("J", "J") // its two commits
	if r, _ := f.m.selected(); r.Name != "api-server" {
		t.Fatalf("selected %s, want api-server", r.Name)
	}
	f.keys("J") // my-app header
	if r, _ := f.m.selected(); r.Name != "my-app" {
		t.Fatalf("selected %s, want my-app", r.Name)
	}
	// my-app has a tab (the fake Find says so): o jumps straight away
	f.run(f.keys("o"))
	if len(f.opens) != 1 || f.opens[0][1] != "/r/my-app" {
		t.Fatalf("opens=%v", f.opens)
	}
	// api-server has none: o asks first
	f.keys("K")
	f.keys("o")
	if f.m.prompt == "" || f.m.promptLabel != "api-server" {
		t.Fatalf("expected the create-workspace prompt, got %q/%q", f.m.prompt, f.m.promptLabel)
	}
	if len(f.opens) != 1 {
		t.Fatalf("nothing may open before the answer: %v", f.opens)
	}
}

func TestTodayCopy(t *testing.T) {
	f, _, _ := workFake(t)
	if cmd := f.keys("y"); cmd != nil {
		t.Fatal("y outside Today does nothing")
	}
	f.run(f.keys("t"))
	cmd := f.keys("y")
	if cmd == nil {
		t.Fatal("y in Today should return the clipboard command")
	}
	if !strings.Contains(f.m.msg, "copied today as markdown") {
		t.Errorf("msg=%q", f.m.msg)
	}
	if got := cmd(); !strings.Contains(string(mustClip(t, got)), "- fix-the-router") {
		t.Errorf("clipboard text lacks the commit: %#v", got)
	}
}

func mustClip(t *testing.T, msg tea.Msg) string {
	t.Helper()
	// setClipboardMsg is bubbletea's private string type
	s, ok := any(msg).(interface{ String() string })
	if ok {
		return s.String()
	}
	return reflectString(msg)
}

func TestTodayClicksAndNarrow(t *testing.T) {
	f, asked, _ := workFake(t)
	f.run(f.keys("t"))
	f.screen()
	px := f.m.width - f.m.panelW + 1
	// the Repo tab, then the Today tab again
	f.run(f.send(tea.MouseClickMsg{Button: tea.MouseLeft, X: px + f.m.tabZones[0][0] + 1, Y: 1}))
	if f.m.panelMode != "" {
		t.Fatal("clicking Repo should switch to Repo")
	}
	f.screen()
	f.run(f.send(tea.MouseClickMsg{Button: tea.MouseLeft, X: px + f.m.tabZones[1][0] + 1, Y: 1}))
	if f.m.panelMode != "today" {
		t.Fatal("clicking Today should switch to Today")
	}
	f.panelRangeClick(t, px, 2)
	if f.m.workRange != "week" || (*asked)[len(*asked)-1] != "week" {
		t.Fatalf("range click: %q %v", f.m.workRange, *asked)
	}
	// narrow: full screen
	sized(f, 50, 30)
	if !f.m.full {
		t.Fatal("expected full-screen panel at 50 cols")
	}
	f.run(f.send(scanMsg(f.m.repos)))
	s := f.screen()
	if !strings.Contains(s, "This repo · All") || !strings.Contains(s, "api-server") {
		t.Errorf("narrow Today:\n%s", s)
	}
}

func (f *fakeTUI) panelRangeClick(t *testing.T, px, idx int) {
	t.Helper()
	f.screen()
	z := f.m.rangeZones[idx]
	f.run(f.send(tea.MouseClickMsg{Button: tea.MouseLeft, X: px + z[0] + 1, Y: 2}))
}

func TestTodayRemembered(t *testing.T) {
	d := tuiDeps{Prefs: core.TUIPrefs{InspectorMode: "today", WorkRange: "week"}}
	m := newTUIModel(d)
	if m.panelMode != "today" || m.workRange != "week" {
		t.Fatalf("mode=%q range=%q", m.panelMode, m.workRange)
	}
	m = newTUIModel(tuiDeps{Prefs: core.TUIPrefs{WorkRange: "bogus"}})
	if m.panelMode != "" || m.workRange != "today" {
		t.Fatalf("defaults wrong: %q %q", m.panelMode, m.workRange)
	}
}

func TestDragInspectorWidth(t *testing.T) {
	f := newFakeTUI(t, false)
	var saved int
	f.m.d.SavePrefs = func(fn func(*core.TUIPrefs)) error {
		var p core.TUIPrefs
		fn(&p)
		saved = p.PanelWidth
		return nil
	}
	f.send(tea.WindowSizeMsg{Width: 140, Height: 30})
	f.keys("i")
	f.m.View()
	if f.m.panelW != panelFull {
		t.Fatalf("default panel width %d", f.m.panelW)
	}
	edge := f.m.width - f.m.panelW
	f.send(tea.MouseClickMsg{X: edge, Y: 5, Button: tea.MouseLeft})
	if !f.m.dragPanel {
		t.Fatal("click on the separator did not start a drag")
	}
	f.send(tea.MouseMotionMsg{X: edge - 20, Y: 5, Button: tea.MouseLeft})
	f.send(tea.MouseReleaseMsg{X: edge - 20, Y: 5, Button: tea.MouseLeft})
	if f.m.dragPanel || f.m.panelW != panelFull+20 || saved != panelFull+20 {
		t.Errorf("after drag: dragging %v, width %d, saved %d", f.m.dragPanel, f.m.panelW, saved)
	}
	// Dragged too far either way: kept between the minimum and one card column.
	f.send(tea.MouseClickMsg{X: f.m.width - f.m.panelW, Y: 5, Button: tea.MouseLeft})
	f.send(tea.MouseReleaseMsg{X: 0, Y: 5, Button: tea.MouseLeft})
	if f.m.panelW != 140-(minInner+3) {
		t.Errorf("wide drag: width %d", f.m.panelW)
	}
	f.send(tea.MouseClickMsg{X: f.m.width - f.m.panelW, Y: 5, Button: tea.MouseLeft})
	f.send(tea.MouseReleaseMsg{X: 139, Y: 5, Button: tea.MouseLeft})
	if f.m.panelW != panelMin {
		t.Errorf("narrow drag: width %d", f.m.panelW)
	}
}

func TestInspectorTabAndRangeKeys(t *testing.T) {
	f := newFakeTUI(t, false)
	f.send(tea.WindowSizeMsg{Width: 140, Height: 30})
	f.keys("t")
	if f.m.panelMode != "today" || !f.m.panelFocus {
		t.Fatalf("t: mode %q, focused %v", f.m.panelMode, f.m.panelFocus)
	}
	for _, c := range []struct{ key, want string }{{"m", "month"}, {"D", "yesterday"}, {"w", "week"}, {"d", "today"}} {
		f.keys(c.key)
		if f.m.workRange != c.want {
			t.Errorf("%s: range %q, want %q", c.key, f.m.workRange, c.want)
		}
	}
	f.keys("1")
	if f.m.panelMode != "" {
		t.Errorf("1: mode %q", f.m.panelMode)
	}
	f.keys("m") // in This repo, m picks no range
	if f.m.workRange != "today" {
		t.Errorf("m on This repo changed the range to %q", f.m.workRange)
	}
	f.keys("2")
	if f.m.panelMode != "today" {
		t.Errorf("2: mode %q", f.m.panelMode)
	}
	// From the cards (inspector not focused) w is still the web page, not a range.
	f.keys("tab")
	if f.m.panelFocus {
		t.Fatal("tab did not leave the inspector")
	}
	f.keys("w")
	if f.m.workRange != "today" {
		t.Errorf("w from the cards changed the range to %q", f.m.workRange)
	}
}
