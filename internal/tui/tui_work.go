package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ezjones/repodash/internal/core"
)

// workEvery is the least time between two computations of the same range after a scan.
const workEvery = 15 * time.Second

// workMsg is the result of a Work computation for one range.
type workMsg struct {
	rng  string
	work []core.RepoWork
}

// initWork reads the remembered mode and range.
func (m *tuiModel) initWork() {
	m.workRange = "today"
	if slices.Contains(core.WorkRanges, m.prefs.WorkRange) {
		m.workRange = m.prefs.WorkRange
	}
	if m.prefs.InspectorMode == "today" {
		m.panelMode = "today"
	}
}

// setPanelMode switches the inspector between the repo ("") and Today, and remembers it.
func (m *tuiModel) setPanelMode(mode string) {
	if m.panelMode == mode {
		return
	}
	m.panelMode = mode
	m.panelScroll, m.panelCur = 0, -1
	m.openCommit = ""
	m.change(func(p *core.TUIPrefs) { p.InspectorMode = mode })
}

// toggleToday is the t key: show the inspector on All, else switch This repo <-> All. Landing
// on All also moves the keys into the inspector, so d, w and m pick the range at once.
func (m *tuiModel) toggleToday() {
	if !m.panel || m.panelMode != "today" {
		m.panel = true
		m.setPanelMode("today")
		m.panelFocus = true
		return
	}
	m.setPanelMode("")
}

// workRangeKey maps the inspector's range keys in All: d today, D yesterday (the day before),
// w this week, m this month.
var workRangeKey = map[string]string{"d": "today", "D": "yesterday", "w": "week", "m": "month"}

// setWorkRange picks the range Today lists; the next workCmd recomputes it.
func (m *tuiModel) setWorkRange(r string) {
	if r == m.workRange {
		return
	}
	m.workRange = r
	m.workFor = ""
	m.panelScroll, m.panelCur = 0, -1
	rng := r
	if rng == "today" {
		rng = ""
	}
	m.change(func(p *core.TUIPrefs) { p.WorkRange = rng })
}

// workCmd computes the work for the current range off the UI goroutine when Today is shown and
// there is no result for the range yet; with stale (after a scan) also when the result is older
// than workEvery.
func (m *tuiModel) workCmd(stale bool) tea.Cmd {
	if m.d.Work == nil || m.panelMode != "today" || !m.panelVisible() || !m.loaded {
		return nil
	}
	rng := m.workRange
	switch {
	case m.workBusy == rng:
		return nil
	case m.workFor == rng && !(stale && m.d.Now().Sub(m.workAt) >= workEvery):
		return nil
	}
	m.workBusy, m.workAt = rng, m.d.Now()
	repos := slices.Clone(m.repos)
	work := m.d.Work
	return func() tea.Msg { return workMsg{rng, work(repos, rng)} }
}

func (m *tuiModel) workDone(msg workMsg) {
	if msg.rng == m.workBusy {
		m.workBusy = ""
	}
	if msg.rng != m.workRange {
		return // the range changed while it was computed
	}
	m.work, m.workFor = msg.work, msg.rng
}

// selectRepo moves the card selection to the named repo without touching the panel.
func (m *tuiModel) selectRepo(name string) {
	for i, r := range m.view {
		if r.Name == name {
			m.sel = i
			return
		}
	}
}

// workAct is Enter / o / a click on a repo's line in Today: jump to its terminal.
func (m *tuiModel) workAct(name string) tea.Cmd {
	m.selectRepo(name)
	for _, r := range m.repos {
		if r.Name == name {
			return m.jumpTo(r.Name, m.title(r), r.Path, false)
		}
	}
	return nil
}

var rangeWord = map[string]string{"today": "today", "yesterday": "yesterday", "week": "this week", "month": "this month"}
var rangeTitle = map[string]string{"today": "Today", "yesterday": "Yesterday", "week": "This week", "month": "This month"}

// copyWork is y: the work as markdown on the clipboard.
func (m *tuiModel) copyWork() tea.Cmd {
	if m.panelMode != "today" || !m.panelVisible() {
		return nil
	}
	if m.workFor != m.workRange {
		m.msg = "still loading…"
		return nil
	}
	titles := map[string]string{}
	for _, r := range m.repos {
		titles[r.Name] = m.title(r)
	}
	md := core.WorkMarkdown(rangeTitle[m.workRange], m.work, titles)
	m.msg = "copied " + strings.ToLower(rangeTitle[m.workRange]) + " as markdown"
	return tea.SetClipboard(md)
}

// panelClickAt handles a click at panel column x, body row row: a tab, a range word or a line.
func (m *tuiModel) panelClickAt(x, row int) tea.Cmd {
	panelLines(m, m.panelWidth())
	i := row + m.panelScroll
	zoneAt := func(zs [][2]int) int {
		for j, z := range zs {
			if x >= z[0] && x < z[1] {
				return j
			}
		}
		return -1
	}
	if i == 0 {
		switch zoneAt(m.tabZones) {
		case 0:
			m.setPanelMode("")
		case 1:
			m.setPanelMode("today")
		}
		return nil
	}
	if m.panelMode == "today" && i == 1 {
		if j := zoneAt(m.rangeZones); j >= 0 {
			m.setWorkRange(core.WorkRanges[j])
		}
		return nil
	}
	return m.panelClick(row)
}

// labelRow lays out words as " word " cells separated by a dot, the active one highlighted, and
// returns the x range of each word.
func labelRow(words []string, active int) ([]seg, [][2]int) {
	segs := []seg{{" ", lipgloss.NewStyle()}}
	var zs [][2]int
	x := 1
	for i, w := range words {
		if i > 0 {
			segs = append(segs, seg{"·", fg(cDim)})
			x++
		}
		st := fg(cText2)
		if i == active {
			st = sty(cInk, cInfo, true)
		}
		t := " " + w + " "
		segs = append(segs, seg{t, st})
		zs = append(zs, [2]int{x, x + lipgloss.Width(t)})
		x += lipgloss.Width(t)
	}
	return segs, zs
}

func tabsRow(mode string) ([]seg, [][2]int) {
	a := 0
	if mode == "today" {
		a = 1
	}
	return labelRow([]string{"This repo", "All"}, a)
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// workTime is HH:MM, "Mon 14:02" for the week range, or "Oct 3 14:02" for the month.
func workTime(unix int64, rng string) string {
	t := time.Unix(unix, 0)
	if rng == "month" {
		return t.Format("Jan 2 15:04")
	}
	if rng == "week" {
		return t.Format("Mon 15:04")
	}
	return t.Format("15:04")
}

// workSummary is "14 commits in 4 repos · 2 with uncommitted work".
func workSummary(ws []core.RepoWork) string {
	commits, withCommits, dirty := 0, 0, 0
	for _, w := range ws {
		commits += len(w.Commits)
		if len(w.Commits) > 0 {
			withCommits++
		}
		if w.Uncommitted > 0 {
			dirty++
		}
	}
	s := "no commits"
	if commits > 0 {
		s = plural(commits, "commit") + " in " + plural(withCommits, "repo")
	}
	if dirty > 0 {
		s += " · " + fmt.Sprintf("%d with uncommitted work", dirty)
	}
	return s
}

// workLines is the inspector in Today mode.
func workLines(m *tuiModel, w int) []string {
	var out, acts []string
	push := func(act string, segs ...seg) {
		if len(out) == m.panelCur && act != "" {
			for i := range segs {
				segs[i].st = segs[i].st.Reverse(true)
			}
		}
		out = append(out, line(w, segs...))
		acts = append(acts, act)
	}
	defer func() { m.panelActs = acts }()
	add := func(segs ...seg) { push("", segs...) }
	tabs, zones := tabsRow(m.panelMode)
	m.tabZones = zones
	add(tabs...)
	rs, zs := labelRow(core.WorkRanges, slices.Index(core.WorkRanges, m.workRange))
	m.rangeZones = zs
	add(rs...)
	if m.workFor != m.workRange {
		add()
		add(seg{" loading…", fg(cDim)})
		return out
	}
	if len(m.work) == 0 {
		add()
		add(seg{" Nothing committed or edited " + rangeWord[m.workRange] + ".", fg(cDim)})
		return out
	}
	for _, l := range wrapWords(workSummary(m.work), w-1) {
		add(seg{" " + l, fg(cText2)})
	}
	titles := map[string]string{}
	for _, r := range m.repos {
		titles[r.Name] = m.title(r)
	}
	for _, rw := range m.work {
		name := rw.Name
		if t := titles[name]; t != "" {
			name = t
		}
		act := "work:" + rw.Name
		add()
		info := "  " + plural(len(rw.Commits), "commit")
		if len(rw.Commits) == 0 {
			info = "  no commits"
		}
		push(act, seg{" " + name, fg(cInfo).Bold(true)}, seg{info + " · " + workTime(rw.Last, m.workRange), fg(cDim)})
		for _, c := range rw.Commits {
			ts := workTime(c.At, m.workRange)
			push(act, seg{" " + ts + " ", fg(cDim)}, seg{c.Subject, fg(cText)})
		}
		if rw.Uncommitted > 0 {
			add(seg{" ● ", fg(cYellow)}, seg{fmt.Sprintf("%s uncommitted, edited %s", plural(rw.Uncommitted, "file"), workTime(rw.Edited, m.workRange)), fg(cYellow)})
		}
	}
	return out
}
