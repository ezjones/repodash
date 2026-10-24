package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestGridLayout(t *testing.T) {
	cases := []struct {
		w       int
		panel   bool
		cols    int
		panelOn bool
	}{
		{40, true, 1, false}, {70, true, 2, false}, {120, true, 2, true}, {120, false, 3, false},
		{200, true, 5, true}, {200, false, 6, false}, {20, true, 1, false},
	}
	for _, c := range cases {
		cols, cardW, pw := gridLayout(c.w, c.panel)
		if cols != c.cols || (pw > 0) != c.panelOn {
			t.Errorf("gridLayout(%d,%v) = %d cols, panel %d; want %d, %v", c.w, c.panel, cols, pw, c.cols, c.panelOn)
		}
		if cols*(cardW+1)-1+pw > c.w && c.w >= 30 {
			t.Errorf("gridLayout(%d,%v): cards overflow (cardW %d)", c.w, c.panel, cardW)
		}
	}
}

func longRepo() Repo {
	return Repo{Name: "my-app", Path: "/r/my-app", Branch: strings.Repeat("feature-", 5), Level: "bad", Ahead: 2,
		LastCommit: 900, Commits7d: 4, Language: "Go", Claude: "busy",
		LastSubject: strings.Repeat("a very long commit subject ", 3)[:60]}
}

func TestRenderCardWidth(t *testing.T) {
	now := time.Unix(1000, 0)
	c := renderCard(longRepo(), "a-really-long-title-for-the-card-band-xxxxxxxx", nil, false, 36, now)
	for _, l := range strings.Split(c, "\n") {
		if w := lipgloss.Width(l); w != 38 {
			t.Errorf("line width %d: %q", w, l)
		}
	}
	if !strings.Contains(c, "…") {
		t.Error("no ellipsis")
	}
	if !strings.Contains(c, "this week") {
		t.Error("week line missing by default")
	}
	s, _ := parseSettings("", []byte(`{"card":{"week":false}}`))
	old := cfg.cur.Load()
	cfg.cur.Store(s)
	defer cfg.cur.Store(old)
	if c := renderCard(longRepo(), "x", nil, false, 36, now); strings.Contains(c, "this week") {
		t.Error("card.week=false still shows the week line")
	}
}

func sized(f *fakeTUI, w, h int) { f.send(tea.WindowSizeMsg{Width: w, Height: h}) }

func TestViewEmpty(t *testing.T) {
	f := newFakeTUI(t, false)
	f.send(scanMsg([]Repo{}))
	sized(f, 120, 40)
	if v := f.m.View().Content; !strings.Contains(v, "No repos match") {
		t.Error(v)
	}
	f.keys("enter", "G")
	f.send(tea.MouseClickMsg{X: 3, Y: 3, Button: tea.MouseLeft})
}

func manyRepos(n int) []Repo {
	var rs []Repo
	for i := 0; i < n; i++ {
		rs = append(rs, Repo{Name: fmt.Sprintf("repo-%02d", i), Path: fmt.Sprintf("/r/repo-%02d", i), Level: "ok", Branch: "main", Activity: int64(1000 - i)})
	}
	return rs
}

func TestResizeKeepsSelectionVisible(t *testing.T) {
	f := newFakeTUI(t, false)
	f.send(scanMsg(manyRepos(30)))
	sized(f, 200, 50)
	f.keys("G")
	sized(f, 80, 20)
	if v := f.m.View().Content; !strings.Contains(v, "repo-29") {
		t.Error("selected card not visible after resize")
	}
	sized(f, 200, 50)
	if v := f.m.View().Content; !strings.Contains(v, "repo-29") {
		t.Error("selected card not visible after growing")
	}
}

func TestViewNarrow(t *testing.T) {
	f := newFakeTUI(t, false)
	f.send(scanMsg(manyRepos(30)))
	for _, w := range []int{40, 30, 24} {
		sized(f, w, 30)
		f.keys("G")
		v := f.m.View().Content
		for _, l := range strings.Split(v, "\n") {
			if lipgloss.Width(l) > w {
				t.Fatalf("width %d: line %d wide: %q", w, lipgloss.Width(l), l)
			}
		}
		if !strings.Contains(v, "repo-29") {
			t.Errorf("width %d: selection not visible", w)
		}
		if f.m.cols != 1 {
			t.Errorf("width %d: cols %d", w, f.m.cols)
		}
		f.keys("g")
	}
	// Tab opens the panel full screen, Tab closes it.
	sized(f, 40, 30)
	f.send(tea.KeyPressMsg{Code: tea.KeyTab})
	if !f.m.full || !strings.Contains(f.m.View().Content, "branch") {
		t.Error("full panel not shown")
	}
	f.send(tea.KeyPressMsg{Code: tea.KeyTab})
	if f.m.full {
		t.Error("tab did not close the full panel")
	}
}

func TestMouseSelectAndDoubleClick(t *testing.T) {
	f := newFakeTUI(t, false)
	sized(f, 200, 50)
	f.m.View()
	cardW := f.m.cardW
	x, y := cardW+1+cardW/2, 1+f.m.cardH/2 // second card, first row
	f.send(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if f.m.sel != 1 {
		t.Fatalf("sel = %d", f.m.sel)
	}
	// the fake finds no tab for api-server, so a jump shows the create prompt
	f.send(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if f.m.prompt != "api-server" {
		t.Errorf("second click did not jump, prompt = %q", f.m.prompt)
	}
}

func TestMouseWheel(t *testing.T) {
	f := newFakeTUI(t, false)
	f.send(scanMsg(manyRepos(30)))
	sized(f, 80, 20)
	f.send(tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelDown})
	if f.m.scroll != 1 {
		t.Errorf("scroll = %d", f.m.scroll)
	}
}

func TestRenderCardHostileText(t *testing.T) {
	evil := "a\tb\x1b]0;x\x07c\x1b[2Jd"
	r := longRepo()
	r.LastSubject, r.Branch = evil, evil
	iss := &repoIssues{Open: 1, Done: &issueRef{Number: 1, Title: evil}, Next: &issueRef{Number: 2, Title: evil}}
	c := renderCard(r, evil, iss, false, 32, time.Unix(1000, 0))
	for _, l := range strings.Split(c, "\n") {
		if w := lipgloss.Width(l); w != 34 {
			t.Errorf("line width %d: %q", w, l)
		}
	}
	for _, bad := range []string{"\t", "\x07", "\x1b]", "\x1b[2J"} {
		if strings.Contains(c, bad) {
			t.Errorf("output contains %q", bad)
		}
	}
}

func TestCardBandKeepsName(t *testing.T) {
	r := Repo{Name: "pixel-forge-x", Level: "bad", Language: "TypeScript"}
	c := renderCard(r, r.Name, nil, false, 28, time.Unix(1000, 0))
	first := strings.Split(c, "\n")[1]
	if !strings.Contains(first, "pixel-forge-x") || !strings.Contains(first, "●") {
		t.Errorf("band lost the name or dot: %q", first)
	}
}
