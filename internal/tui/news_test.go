package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ezjones/repodash/internal/core"
)

func TestWhatsNew(t *testing.T) {
	old := core.Version
	core.Version = "v0.2.0"
	defer func() { core.Version = old }()
	f := newFakeTUI(t, false)
	f.m.d.Changelog = func() []core.Release {
		return []core.Release{
			{Tag: "v0.3.0", Items: []string{"Not out yet."}},
			{Tag: "v0.2.0", Items: []string{"Press `L` for lazygit."}},
			{Tag: "v0.1.0", Items: []string{"First release."}},
		}
	}
	// Opened before the first window size, as on start after an update: must not panic.
	f.m.width, f.m.height = 0, 0
	f.m.openNews()
	f.m.View()
	f.m.news = nil
	f.send(tea.WindowSizeMsg{Width: 120, Height: 40})

	// A click on the footer's version opens it.
	f.m.View()
	if f.m.verX0 < 0 {
		t.Fatal("version not drawn in the footer")
	}
	f.send(tea.MouseClickMsg{X: f.m.verX0, Y: f.m.height - 1, Button: tea.MouseLeft})
	if f.m.news == nil {
		t.Fatal("click on the version did not open What's new")
	}
	v := f.m.View().Content
	for _, want := range []string{"What's new in v0.2.0", "Press L for lazygit.", "Earlier", "v0.1.0", "First release."} {
		if !strings.Contains(v, want) {
			t.Errorf("box lacks %q", want)
		}
	}
	if strings.Contains(v, "Not out yet") {
		t.Error("a release newer than this one is shown")
	}
	// Keys go to the box, not the cards; Esc closes it, and so does Enter.
	f.keys("L")
	if f.m.news == nil {
		t.Fatal("a key closed the box")
	}
	f.keys("esc")
	if f.m.news != nil {
		t.Error("esc did not close the box")
	}
	f.m.openNews()
	f.keys("enter")
	if f.m.news != nil || len(f.opens) != 0 {
		t.Errorf("enter: box %v, opens %v", f.m.news, f.opens)
	}
}
