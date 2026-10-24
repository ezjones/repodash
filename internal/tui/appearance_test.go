package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ezjones/repodash/internal/core"
)

func TestBandFor(t *testing.T) {
	r := core.Repo{Name: "my-app", Level: "bad"}
	def, _ := bandColors("my-app")
	if bg, _ := bandFor(r, look{}); bg != def {
		t.Errorf("default theme %s, want %s", bg, def)
	}
	if bg, _ := bandFor(r, look{theme: "plain"}); bg != "#d8d8dc" {
		t.Errorf("plain %s", bg)
	}
	if bg, ink := bandFor(r, look{theme: "ink"}); bg != "" || ink == "" {
		t.Errorf("ink: bg %q ink %q", bg, ink)
	}
	if bg, _ := bandFor(r, look{theme: "status"}); bg != "#b5443a" {
		t.Errorf("status %s", bg)
	}
	if bg, ink := bandFor(r, look{theme: "plain", color: "#E9A23B"}); bg != "#E9A23B" || ink != cInk {
		t.Errorf("own colour: %s on %s, want dark ink on #E9A23B", ink, bg)
	}
	if validHex("#12345") != "" || validHex("red") != "" || validHex("#zzzzzz") != "" || validHex("#a1B2c3") == "" {
		t.Error("validHex")
	}
}

func lookFixture(t *testing.T) (*fakeTUI, *[]core.TUIPrefs) {
	f := newFakeTUI(t, false)
	var saved []core.TUIPrefs
	f.m.d.SavePrefs = func(p core.TUIPrefs) {
		c := p
		c.Colors = map[string]string{}
		for k, v := range p.Colors {
			c.Colors[k] = v
		}
		saved = append(saved, c)
	}
	f.send(tea.WindowSizeMsg{Width: 130, Height: 30})
	return f, &saved
}

func TestLookKeys(t *testing.T) {
	f, saved := lookFixture(t)
	f.keys("a")
	if !f.m.lookOpen {
		t.Fatal("a did not open the menu")
	}
	f.keys("l")      // theme: colourful -> plain
	f.keys("j", "l") // status: dot
	f.keys("j", "l") // language: hidden
	f.keys("j", "l") // this card: first swatch
	name := f.m.view[f.m.sel].Name
	last := (*saved)[len(*saved)-1]
	if last.Theme != "plain" || !last.StatusDot || !last.HideLanguage || last.Colors[name] != swatches[0] {
		t.Errorf("saved %+v", last)
	}
	f.keys("h") // back to the theme colour
	if _, ok := (*saved)[len(*saved)-1].Colors[name]; ok {
		t.Error("h on the first swatch kept the colour")
	}
	f.keys("k", "k", "k", "h") // theme back to colourful, which is saved as ""
	if (*saved)[len(*saved)-1].Theme != "" {
		t.Errorf("default theme saved as %q", (*saved)[len(*saved)-1].Theme)
	}
	f.keys("esc")
	if f.m.lookOpen {
		t.Error("esc did not close")
	}
	f.keys("l") // keys reach the grid again
	if f.m.sel != 1 {
		t.Errorf("sel %d after closing", f.m.sel)
	}
}

func TestLookMouse(t *testing.T) {
	f, saved := lookFixture(t)
	f.m.View()
	for _, z := range f.m.barZones {
		if z.act == "appearance" {
			f.send(tea.MouseClickMsg{X: z.x0, Y: 0, Button: tea.MouseLeft})
		}
	}
	if !f.m.lookOpen {
		t.Fatal("◐ did not open the menu")
	}
	scr := func() []string { return strings.Split(ansi.Strip(f.m.View().Content), "\n") }
	find := func(sub string) (int, int) {
		for y, l := range scr() {
			if i := strings.Index(l, sub); i >= 0 {
				return len([]rune(l[:i])), y
			}
		}
		t.Fatalf("%q not on screen", sub)
		return 0, 0
	}
	click := func(x, y int) { f.send(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}) }
	x, y := find(" › ")
	click(x+1, y)
	x, y = find("hidden")
	click(x, y)
	x, y = find(" ●  ")
	click(x+1, y)
	click(33, 2) // the second card, behind the menu
	if !f.m.lookOpen || f.m.sel != 1 {
		t.Fatalf("card click: open %v, sel %d", f.m.lookOpen, f.m.sel)
	}
	x, y = find("theme]")
	click(x+8, y) // first swatch
	last := (*saved)[len(*saved)-1]
	if last.Theme != "plain" || !last.HideLanguage || !last.StatusDot || last.Colors[f.m.view[1].Name] != swatches[0] {
		t.Errorf("saved %+v", last)
	}
	x, y = find("[ close ]")
	click(x+2, y)
	if f.m.lookOpen {
		t.Error("close did not close")
	}
}

func TestLookFromPrefs(t *testing.T) {
	f := newFakeTUI(t, false)
	f.m.prefs = core.TUIPrefs{StatusDot: true, Colors: map[string]string{"my-app": "#3B82F6", "api-server": "bogus"}}
	f.send(tea.WindowSizeMsg{Width: 130, Height: 30})
	band := strings.Split(ansi.Strip(f.m.View().Content), "\n")[2] // the first row of cards
	if strings.Contains(band, "Needs action") || !strings.Contains(band, "●") {
		t.Errorf("dot mode band %q", band)
	}
	if lk := f.m.lookFor(core.Repo{Name: "api-server"}); lk.color != "" {
		t.Errorf("a bad colour was used: %q", lk.color)
	}
}

func TestFooterKeepsVersion(t *testing.T) {
	f := newFakeTUI(t, false)
	for _, w := range []int{100, 130, 200} {
		f.send(tea.WindowSizeMsg{Width: w, Height: 30})
		rows := strings.Split(ansi.Strip(f.m.View().Content), "\n")
		last := strings.TrimRight(rows[len(rows)-1], " ")
		if !strings.HasSuffix(last, core.Version) || !strings.Contains(last, "? help") {
			t.Errorf("width %d footer %q", w, last)
		}
	}
}
