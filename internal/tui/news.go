package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ezjones/repodash/internal/core"
)

// newsBox is "What's new": CHANGELOG.md from this release back, in a box over the cards. It
// opens by itself on the first run after an update, and on a click on the footer's version.
type newsBox struct {
	scroll     int
	x, y, w, h int // where the box was last drawn, for mouse clicks
}

func (m *tuiModel) releases() []core.Release {
	if m.d.Changelog != nil {
		return m.d.Changelog()
	}
	return core.Changelog()
}

func (m *tuiModel) openNews() {
	if len(m.releases()) == 0 {
		m.msg = "no release notes in this build"
		return
	}
	m.news = &newsBox{}
}

// newsTitle names the box after the running release when the changelog has it.
func (m *tuiModel) newsTitle() string {
	for _, r := range m.releases() {
		if r.Tag == core.Version {
			return "What's new in " + r.Tag
		}
	}
	return "What's new"
}

// newsLines is the box's text at width w: the running release first (newer ones a dev build
// may list are skipped), then every earlier one.
func (m *tuiModel) newsLines(w int) []string {
	rs := m.releases()
	for i, r := range rs {
		if r.Tag == core.Version {
			rs = rs[i:]
			break
		}
	}
	tag, text := fg(cInfo).Bold(true), fg(cText)
	var out []string
	for i, r := range rs {
		if i > 0 {
			out = append(out, "")
		}
		h := r.Tag
		if i == 0 && r.Tag == core.Version {
			h += "  (this version)"
		} else if i == 1 && rs[0].Tag == core.Version {
			out = append(out, fg(cMuted).Render("Earlier"), "")
		}
		out = append(out, tag.Render(h))
		for _, it := range r.Items {
			for j, l := range wrapWords(strings.ReplaceAll(it, "`", ""), max(1, w-2)) {
				p := "• "
				if j > 0 {
					p = "  "
				}
				out = append(out, text.Render(p+l))
			}
		}
	}
	return out
}

func (m *tuiModel) newsRows() int {
	_, h := m.dialogSize()
	return max(1, h-3)
}

func (m *tuiModel) newsScroll(n int) {
	w, _ := m.dialogSize()
	ls := m.newsLines(w - 4)
	m.news.scroll = min(max(m.news.scroll+n, 0), max(0, len(ls)-m.newsRows()))
}

func (m *tuiModel) newsKey(s string) {
	switch s {
	case "esc", "enter", "q":
		m.news = nil
	case "j", "down":
		m.newsScroll(1)
	case "k", "up":
		m.newsScroll(-1)
	case "pgdown", "space", "ctrl+d":
		m.newsScroll(m.newsRows())
	case "pgup", "ctrl+u":
		m.newsScroll(-m.newsRows())
	case "g", "home":
		m.news.scroll = 0
	case "G", "end":
		m.newsScroll(1 << 20)
	}
}

// newsMouse scrolls on the wheel and closes on a click outside the box.
func (m *tuiModel) newsMouse(msg tea.Msg) {
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.newsScroll(-3)
		case tea.MouseWheelDown:
			m.newsScroll(3)
		}
	case tea.MouseClickMsg:
		b, n := tea.Mouse(msg), m.news
		if b.Button == tea.MouseLeft && (b.X < n.x || b.X >= n.x+n.w || b.Y < n.y || b.Y >= n.y+n.h) {
			m.news = nil
		}
	}
}

// overlayNews draws the box centred over the body lines.
func (m *tuiModel) overlayNews(body []string, bodyH int) []string {
	w, h := m.dialogSize()
	if w < 12 || h < 4 { // no window size yet (it opens before the first one), or too small
		return body
	}
	inner := w - 2
	bs := fg(cInfo)
	title := " " + trunc(m.newsTitle(), max(1, inner-2)) + " "
	top := bs.Render("╭") + bs.Bold(true).Render(title) + bs.Render(strings.Repeat("─", max(0, inner-lipgloss.Width(title)))+"╮")
	side := bs.Render("│")
	row := func(s string) string { return side + " " + padTo(s, inner-2) + " " + side }
	ls := m.newsLines(inner - 2)
	rows := h - 3
	n := m.news
	n.scroll = min(max(n.scroll, 0), max(0, len(ls)-rows))
	box := []string{top}
	for i := 0; i < rows; i++ {
		l := ""
		if j := n.scroll + i; j < len(ls) {
			l = ls[j]
		}
		box = append(box, row(l))
	}
	foot := "enter or esc close"
	if len(ls) > rows {
		foot = "j/k scroll · " + foot
	}
	box = append(box, row(fg(cDim).Render(trunc(foot, inner-2))), bs.Render("╰"+strings.Repeat("─", inner)+"╯"))

	x, y := (m.width-w)/2, (bodyH-h)/2
	n.x, n.y, n.w, n.h = x, y+1, w, h // +1 for the top bar
	for len(body) < bodyH {
		body = append(body, "")
	}
	for i, bl := range box {
		r := y + i
		if r < 0 || r >= len(body) {
			continue
		}
		l := body[r]
		left := ansi.Truncate(l, x, "")
		left += strings.Repeat(" ", max(0, x-lipgloss.Width(left)))
		body[r] = left + bl + ansi.TruncateLeft(l, x+w, "")
	}
	return body
}
