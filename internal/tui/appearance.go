package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ezjones/repodash/internal/core"
)

// The Appearance menu (a, or ◐ in the top bar): a card theme, the status as words or a dot, the
// language badge, and a colour of the selected repo's own. Saved in tui.json through d.SavePrefs.

var themes = []string{"colourful", "plain", "soft", "ink", "status"}

// swatches are the colours a repo can be given in the menu. A hand-edited tui.json may hold others.
var swatches = []string{"#E05A8A", "#2A9D8F", "#7F52FF", "#E9A23B", "#3B82F6", "#C2452D"}

// look is how one card is drawn; the zero value is the default look.
type look struct {
	theme  string // one of themes; "" is colourful
	dot    bool   // status as a coloured dot, no words
	noLang bool   // no language badge
	color  string // the repo's own band colour, "" when it has none
}

func (m *tuiModel) lookFor(r core.Repo) look {
	return look{m.prefs.Theme, m.prefs.StatusDot, m.prefs.HideLanguage, validHex(m.prefs.Colors[r.Name])}
}

// validHex returns c when it is a #rrggbb colour, else "".
func validHex(c string) string {
	if len(c) != 7 || c[0] != '#' {
		return ""
	}
	if _, err := strconv.ParseUint(c[1:], 16, 32); err != nil {
		return ""
	}
	return c
}

// inkOn is white or near-black, whichever reads better on bg.
func inkOn(bg string) string {
	p := func(i int) float64 { v, _ := strconv.ParseUint(bg[i:i+2], 16, 8); return float64(v) }
	if .299*p(1)+.587*p(3)+.114*p(5) > 150 {
		return cInk
	}
	return "#ffffff"
}

// bandFor is the name band's background and text colour. An empty background means no band
// (the ink theme colours the name itself).
func bandFor(r core.Repo, lk look) (bg, ink string) {
	if lk.color != "" {
		return lk.color, inkOn(lk.color)
	}
	bg, ink = bandColors(r.Name)
	switch lk.theme {
	case "plain":
		return "#d8d8dc", cInk
	case "soft":
		return mixHex(bg, "#ffffff", .62), mixHex(bg, "#000000", .55)
	case "ink":
		h := nameHash(r.Name) // one of the palette's three bright colours, never its cream or near-black
		P := bandPalette[int(h)%len(bandPalette)]
		return "", mixHex(P[1+int(h>>8)%3], "#ffffff", .2)
	case "status":
		switch r.Level {
		case "bad":
			return "#b5443a", "#ffffff"
		case "warn":
			return "#b8862b", cInk
		case "stale":
			return "#4a5068", "#ffffff"
		}
		return "#3f7f4e", "#ffffff"
	}
	return
}

// ---------- the menu ----------

const lookRows = 4 // theme, status, language, this card

type lookZone struct {
	y, x0, x1 int
	act       string
}

func (m *tuiModel) openLook() { m.lookOpen, m.lookRow = true, 0 }

func (m *tuiModel) savePrefs() {
	if m.d.SavePrefs != nil {
		m.d.SavePrefs(m.prefs)
	}
}

func (m *tuiModel) themeIndex() int {
	for i, t := range themes {
		if t == m.prefs.Theme {
			return i
		}
	}
	return 0
}

func (m *tuiModel) stepTheme(step int) {
	t := themes[(m.themeIndex()+step+len(themes))%len(themes)]
	if t == themes[0] {
		t = "" // the default is not written to the file
	}
	m.prefs.Theme = t
}

// swatchIndex is the selected repo's colour in swatches, -1 for none and len(swatches) for a
// colour that is not one of them.
func (m *tuiModel) swatchIndex() int {
	c := m.selColor()
	if c == "" {
		return -1
	}
	for i, s := range swatches {
		if strings.EqualFold(s, c) {
			return i
		}
	}
	return len(swatches)
}

func (m *tuiModel) selColor() string {
	if m.sel < 0 || m.sel >= len(m.view) {
		return ""
	}
	return validHex(m.prefs.Colors[m.view[m.sel].Name])
}

// setColor gives the selected repo swatch i, or takes its colour away for i < 0.
func (m *tuiModel) setColor(i int) {
	if m.sel < 0 || m.sel >= len(m.view) {
		return
	}
	name := m.view[m.sel].Name
	if i < 0 || i >= len(swatches) {
		delete(m.prefs.Colors, name)
		return
	}
	if m.prefs.Colors == nil {
		m.prefs.Colors = map[string]string{}
	}
	m.prefs.Colors[name] = swatches[i]
}

func (m *tuiModel) lookKey(s string) {
	step := 0
	switch s {
	case "esc", "a", "q", "enter":
		m.lookOpen = false
		return
	case "j", "down", "tab":
		m.lookRow = (m.lookRow + 1) % lookRows
	case "k", "up", "shift+tab":
		m.lookRow = (m.lookRow + lookRows - 1) % lookRows
	case "l", "right", "space":
		step = 1
	case "h", "left":
		step = -1
	}
	if step == 0 {
		return
	}
	switch m.lookRow {
	case 0:
		m.stepTheme(step)
	case 1:
		m.prefs.StatusDot = !m.prefs.StatusDot
	case 2:
		m.prefs.HideLanguage = !m.prefs.HideLanguage
	case 3:
		if m.sel < 0 || m.sel >= len(m.view) {
			return
		}
		cur := m.swatchIndex()
		if cur == len(swatches) { // a colour from a hand-edited file steps as if it were "theme"
			cur = -1
		}
		k := len(swatches) + 1
		m.setColor((cur+1+step+k)%k - 1)
	}
	m.savePrefs()
}

// lookClick handles a click while the menu is open. A click on a card behind it selects that card
// and keeps the menu open, so cards can be coloured one after another.
func (m *tuiModel) lookClick(ms tea.Mouse) {
	if ms.Button != tea.MouseLeft {
		return
	}
	bx, by, bw, bh := m.lookBox[0], m.lookBox[1], m.lookBox[2], m.lookBox[3]
	if ms.X < bx || ms.X >= bx+bw || ms.Y < by || ms.Y >= by+bh {
		if i, ok := m.cardAt(ms.X, ms.Y); ok {
			m.sel, m.lookRow = i, 3
			return
		}
		m.lookOpen = false
		return
	}
	for _, z := range m.lookZones {
		if ms.Y != z.y || ms.X < z.x0 || ms.X >= z.x1 {
			continue
		}
		k, v, _ := strings.Cut(z.act, ":")
		n, _ := strconv.Atoi(v)
		switch k {
		case "close":
			m.lookOpen = false
			return
		case "row":
			m.lookRow = n
			return
		case "theme":
			m.lookRow = 0
			m.stepTheme(n)
		case "status":
			m.lookRow, m.prefs.StatusDot = 1, n == 1
		case "lang":
			m.lookRow, m.prefs.HideLanguage = 2, n == 1
		case "swatch":
			m.lookRow = 3
			m.setColor(n)
		}
		m.savePrefs()
		return
	}
}

func (m *tuiModel) lookMenu() (box []string, w int) {
	w = min(50, m.width)
	inner := w - 2
	bd := fg(cInfo)
	pad := func(segs ...seg) string {
		return bd.Render("│") + line(inner, segs...) + bd.Render("│")
	}
	label, val, dim, keyst := fg(cText2), fg(cText).Bold(true), fg(cDim), fg(cInfo).Bold(true)
	m.lookZones = m.lookZones[:0]
	// row draws one menu row; acts[i] is what a click on segs[i] does ("" = nothing).
	row := func(i int, name string, segs []seg, acts []string) string {
		mark, st := seg{"  ", label}, label
		if i == m.lookRow {
			mark, st = seg{"▸ ", keyst}, keyst
		}
		head := []seg{{" ", label}, mark, {fmt.Sprintf("%-11s", name), st}}
		x := 1 + segsWidth(head)
		y := len(box)
		m.lookZones = append(m.lookZones, lookZone{y, 1, x, fmt.Sprintf("row:%d", i)})
		for j, sg := range segs {
			w := lipgloss.Width(sg.s)
			if j < len(acts) && acts[j] != "" {
				m.lookZones = append(m.lookZones, lookZone{y, x, x + w, acts[j]})
			}
			x += w
		}
		return pad(append(head, segs...)...)
	}
	choice := func(key string, opts []string, cur int) ([]seg, []string) {
		var out []seg
		var acts []string
		for i, o := range opts {
			st := dim
			if i == cur {
				st = sty(cInk, cInfo, true)
			}
			out = append(out, seg{" " + o + " ", st}, seg{" ", dim})
			acts = append(acts, fmt.Sprintf("%s:%d", key, i), "")
		}
		return out, acts
	}
	b2i := func(b bool) int {
		if b {
			return 1
		}
		return 0
	}
	title := " Appearance "
	box = append(box, bd.Render("╭─"+title+strings.Repeat("─", max(0, inner-1-len(title)))+"╮"))
	box = append(box, pad())
	box = append(box, row(0, "Theme", []seg{{" ‹ ", dim}, {fmt.Sprintf("%-9s", themes[m.themeIndex()]), val}, {" › ", dim}},
		[]string{"theme:-1", "theme:1", "theme:1"}))
	sg, ac := choice("status", []string{"● words", "●"}, b2i(m.prefs.StatusDot))
	box = append(box, row(1, "Status", sg, ac))
	sg, ac = choice("lang", []string{"shown", "hidden"}, b2i(m.prefs.HideLanguage))
	box = append(box, row(2, "Language", sg, ac))

	cur := m.swatchIndex()
	var sw []seg
	sa := []string{"swatch:-1"}
	if cur < 0 {
		sw = append(sw, seg{"[theme]", val})
	} else {
		sw = append(sw, seg{" theme ", dim})
	}
	for i, c := range swatches {
		a := fmt.Sprintf("swatch:%d", i)
		if i == cur {
			sw = append(sw, seg{"[", val}, seg{"██", fg(c)}, seg{"]", val})
		} else {
			sw = append(sw, seg{" ", dim}, seg{"██", fg(c)}, seg{" ", dim})
		}
		sa = append(sa, a, a, a)
	}
	if cur == len(swatches) {
		sw = append(sw, seg{"[", val}, seg{"██", fg(m.selColor())}, seg{"]", val})
	}
	box = append(box, row(3, "This card", sw, sa))
	name := "no card selected"
	if m.sel >= 0 && m.sel < len(m.view) {
		name = cleanText(m.title(m.view[m.sel])) + "  (click a card to pick another)"
	}
	box = append(box, pad(seg{"   " + trunc(name, max(1, inner-4)), dim}))
	box = append(box, pad())
	hint := []seg{{" j/k ", keyst}, {"row  ", dim}, {"h/l ", keyst}, {"change  ", dim}, {"esc ", keyst}, {"done  ", dim}}
	cx := 1 + segsWidth(hint)
	box = append(box, pad(append(hint, seg{"[ close ]", keyst})...))
	m.lookZones = append(m.lookZones, lookZone{len(box) - 1, cx, cx + 9, "close"})
	box = append(box, bd.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return box, w
}

// overlayLook draws the menu over the middle of the body and records where it is on screen.
func (m *tuiModel) overlayLook(body []string, bodyH int) []string {
	box, w := m.lookMenu()
	x, y := max(0, (m.width-w)/2), max(0, (bodyH-len(box))/2)
	m.lookBox = [4]int{x, y + 1, w, len(box)} // +1 for the top bar
	for i := range m.lookZones {
		m.lookZones[i].y += y + 1
		m.lookZones[i].x0 += x
		m.lookZones[i].x1 += x
	}
	for len(body) < bodyH {
		body = append(body, "")
	}
	for i, bl := range box {
		r := y + i
		if r >= len(body) {
			break
		}
		l := body[r]
		left := ansi.Truncate(l, x, "")
		left += strings.Repeat(" ", max(0, x-lipgloss.Width(left)))
		body[r] = left + bl + ansi.TruncateLeft(l, x+w, "")
	}
	return body
}
