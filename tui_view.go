package main

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	cMuted  = "#787c99"
	cDim    = "#565f89"
	cText   = "#c0caf5"
	cText2  = "#a9b1d6"
	cBorder = "#3b3f5c"
	cInfo   = "#7aa2f7"
	cInk    = "#1a1b26"
	cYellow = "#e0af68"
	cRed    = "#f7768e"
	cGreen  = "#9ece6a"

	minInner   = 28 // narrowest card interior
	panelFull  = 45 // panel (44) plus its separator
	doubleTime = 400 * time.Millisecond
)

type seg struct {
	s  string
	st lipgloss.Style
}

func sty(fg, bg string, bold bool) lipgloss.Style {
	s := lipgloss.NewStyle().Bold(bold)
	if fg != "" {
		s = s.Foreground(lipgloss.Color(fg))
	}
	if bg != "" {
		s = s.Background(lipgloss.Color(bg))
	}
	return s
}

func fg(c string) lipgloss.Style { return sty(c, "", false) }

// trunc cuts s to at most w cells, ending in an ellipsis when it had to cut.
func trunc(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	if w <= 0 {
		return ""
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if used+rw > w-1 {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String() + "…"
}

func segsWidth(segs []seg) int {
	n := 0
	for _, s := range segs {
		n += lipgloss.Width(s.s)
	}
	return n
}

// cleanText makes untrusted text safe to draw: tabs become spaces and every control
// rune (ESC, BEL, newline...) is dropped, so it cannot move the cursor or send escapes.
func cleanText(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// line renders segments into exactly w cells: cut with an ellipsis, then padded.
func line(w int, segs ...seg) string {
	var b strings.Builder
	left := w
	for _, s := range segs {
		if left <= 0 {
			break
		}
		t := trunc(cleanText(s.s), left)
		left -= lipgloss.Width(t)
		b.WriteString(s.st.Render(t))
	}
	if left > 0 {
		b.WriteString(strings.Repeat(" ", left))
	}
	return b.String()
}

func padTo(s string, w int) string {
	if n := w - lipgloss.Width(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// gridLayout splits the width between the cards and the right panel. Cards are at least
// minInner+2 wide and stretch to share the space; the panel needs room for one card column.
func gridLayout(width int, panelOn bool) (cols, cardW, panelW int) {
	if panelOn && width >= minInner+3+panelFull {
		panelW = panelFull
	}
	avail := width - panelW
	cols = max(1, avail/(minInner+3))
	cardW = avail/cols - 1
	if cols == 1 && avail < minInner+2 {
		cardW = avail
	}
	if cardW < 4 {
		cardW = 4
	}
	return
}

func cardOn(raw map[string]any, key string) bool {
	v, ok := get(raw, "card."+key).(bool)
	return !ok || v
}

func statusInfo(raw map[string]any, level string) (label, color string) {
	label, _ = get(raw, "statuses."+level+".label").(string)
	color, _ = get(raw, "statuses."+level+".darkColor").(string)
	if label == "" {
		label = level
	}
	if color == "" {
		color = cMuted
	}
	return
}

func chipColor(raw map[string]any, kind string) string {
	if kind == "info" {
		return cInfo
	}
	_, c := statusInfo(raw, kind)
	return c
}

func shortAgo(now time.Time, ts int64) string {
	return strings.TrimSuffix(ago(now, ts), " ago")
}

func closedAgo(now time.Time, at string) string {
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return ""
	}
	return shortAgo(now, t.Unix())
}

func agentIcon(claude string) (string, string) {
	switch claude {
	case "busy":
		return "◌", cYellow
	case "waiting":
		return "●", cRed
	case "idle":
		return "●", cGreen
	}
	return "", ""
}

// cardBody is the card's lines without the border, each exactly inner cells wide.
func cardBody(r Repo, title string, iss *repoIssues, inner int, now time.Time) []string {
	raw := tuiSettings().Raw
	var out []string
	add := func(segs ...seg) { out = append(out, line(inner, segs...)) }

	// band
	bg, ink := bandColors(r.Name)
	lbl, col := statusInfo(raw, r.Level)
	var icon seg
	if cardOn(raw, "agent") {
		if g, c := agentIcon(r.Claude); g != "" {
			icon = seg{" " + g, sty(c, bg, true)}
		}
	}
	pillFull := seg{" ● " + lbl + " ", sty(cInk, col, true)}
	pillDot := seg{" ● ", sty(cInk, col, true)}
	var badge seg
	if cardOn(raw, "languageBadge") && r.Language != "" {
		badge = seg{" " + r.Language + " ", sty("#ffffff", langBadgeColor(r.Language), true)}
	}
	var right []seg
	// the name gets its room first; then drop the badge, then the label, and cut the name last
	need := 1 + lipgloss.Width(cleanText(title))
	for _, try := range [][]seg{{icon, pillFull, badge}, {icon, pillFull}, {icon, pillDot}, {pillDot}} {
		right = try
		if inner-segsWidth(try)-1 >= need {
			break
		}
	}
	rw := segsWidth(right)
	nameW := max(1, inner-rw-1)
	name := " " + trunc(cleanText(title), nameW-1)
	bandSegs := []seg{{padTo(name, nameW), sty(ink, bg, true)}}
	bandSegs = append(bandSegs, right...)
	bandSegs = append(bandSegs, seg{" ", sty("", bg, false)})
	out = append(out, line(inner, bandSegs...))

	if cardOn(raw, "branch") {
		b := " " + r.Branch
		if r.Base != "" && r.Base != r.Branch {
			b += "  base " + r.Base
		}
		add(seg{b, fg(cMuted)})
	}
	if rs, ok := raw["repos"].(map[string]any); ok && cardOn(raw, "note") {
		if e, ok := rs[r.Name].(map[string]any); ok {
			if n, _ := e["note"].(string); n != "" {
				add(seg{" " + n, fg(cText2)})
			}
		}
	}
	if cardOn(raw, "chips") {
		segs := []seg{{" ", lipgloss.NewStyle()}}
		for _, c := range repoChips(r) {
			segs = append(segs, seg{"[" + c.Text + "]", fg(chipColor(raw, c.Kind))}, seg{" ", lipgloss.NewStyle()})
		}
		add(segs...)
	}
	if cardOn(raw, "activity") {
		if r.Edited != 0 && r.Edited >= r.LastCommit {
			add(seg{" edited " + shortAgo(now, r.Edited) + " ", fg(cYellow)}, seg{"uncommitted work", fg(cText)})
		}
		subj := r.LastSubject
		if subj == "" {
			subj = "no commits"
		}
		add(seg{" " + shortAgo(now, r.LastCommit) + " ", fg(cDim)}, seg{subj, fg(cText)})
	}
	if iss != nil && cardOn(raw, "issues") {
		if iss.Done != nil {
			t := fmt.Sprintf("#%d %s", iss.Done.Number, iss.Done.Title)
			if a := closedAgo(now, iss.Done.At); a != "" {
				t += " · " + a
			}
			add(seg{" ✓ ", fg(cGreen)}, seg{t, fg(cText2)})
		}
		if iss.Next != nil {
			add(seg{" → ", fg(cInfo)}, seg{fmt.Sprintf("#%d %s", iss.Next.Number, iss.Next.Title), fg(cText2)})
		}
	}
	if cardOn(raw, "week") {
		bars := strings.Repeat("▮", min(r.Commits7d, 12))
		if bars == "" {
			bars = "–"
		}
		add(seg{" " + bars, fg(cGreen)}, seg{fmt.Sprintf(" %d this week", r.Commits7d), fg(cDim)})
	}
	if cardOn(raw, "worktrees") && len(r.Worktrees) > 0 {
		t := fmt.Sprintf(" %d worktree", len(r.Worktrees))
		if len(r.Worktrees) > 1 {
			t += "s"
		}
		merged := 0
		for _, w := range r.Worktrees {
			if w.Merged {
				merged++
			}
		}
		if merged > 0 {
			t += fmt.Sprintf(" (%d merged)", merged)
		}
		add(seg{t, fg(cMuted)})
	}
	return out
}

func borderColor(sel bool) string {
	if sel {
		if c, _ := get(tuiSettings().Raw, "colors.accentDark").(string); c != "" {
			return c
		}
		return "#e5e7ed"
	}
	return cBorder
}

// wrapCard draws the border around body lines, padding with blank lines up to h body lines.
func wrapCard(body []string, h, inner int, sel bool) string {
	bs := fg(borderColor(sel))
	v := bs.Render("│")
	out := []string{bs.Render("╭" + strings.Repeat("─", inner) + "╮")}
	for i := 0; i < max(h, len(body)); i++ {
		l := strings.Repeat(" ", inner)
		if i < len(body) {
			l = body[i]
		}
		out = append(out, v+l+v)
	}
	out = append(out, bs.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return strings.Join(out, "\n")
}

func renderCard(r Repo, title string, iss *repoIssues, sel bool, inner int, now time.Time) string {
	return wrapCard(cardBody(r, title, iss, inner, now), 0, inner, sel)
}

// ---------- layout ----------

type visKey struct{ sel, w, h, n, cols int }

func (m *tuiModel) issueFor(name string) *repoIssues {
	if iss, ok := m.issues[name]; ok {
		return &iss
	}
	return nil
}

func (m *tuiModel) bodyH() int { return max(1, m.height-2) }

// relayout recomputes columns, card height and scroll for the current size.
func (m *tuiModel) relayout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	cols, cardW, panelW := gridLayout(m.width, m.panel)
	m.cols, m.cardW, m.panelW = cols, cardW, panelW
	m.full = m.panel && panelW == 0 // too narrow to sit beside the cards
	inner := max(1, cardW-2)
	h := 3
	now := m.d.Now()
	for _, r := range m.view {
		h = max(h, len(cardBody(r, m.title(r), m.issueFor(r.Name), inner, now)))
	}
	m.cardH = h + 2
	m.vis = max(1, m.bodyH()/m.cardH)
	m.pageRows = m.vis
	rows := (len(m.view) + cols - 1) / cols
	maxScroll := max(0, rows-m.vis)
	k := visKey{m.sel, m.width, m.height, len(m.view), cols}
	if k != m.visKey {
		m.visKey = k
		row := m.sel / cols
		if row < m.scroll {
			m.scroll = row
		}
		if row >= m.scroll+m.vis {
			m.scroll = row - m.vis + 1
		}
	}
	m.scroll = min(max(m.scroll, 0), maxScroll)
}

// cardAt maps a screen cell to a view index using the last layout.
func (m *tuiModel) cardAt(x, y int) (int, bool) {
	if m.full || m.help || m.cardH == 0 || x < 0 || y < 1 || y >= 1+m.vis*m.cardH {
		return 0, false
	}
	gridW := m.width - m.panelW
	if x >= gridW {
		return 0, false
	}
	col, off := x/(m.cardW+1), x%(m.cardW+1)
	if off >= m.cardW || col >= m.cols {
		return 0, false
	}
	i := (m.scroll+(y-1)/m.cardH)*m.cols + col
	if i < 0 || i >= len(m.view) {
		return 0, false
	}
	return i, true
}

func (m *tuiModel) click(ms tea.Mouse) tea.Cmd {
	if ms.Button != tea.MouseLeft {
		return nil
	}
	if ms.Y == 0 && !m.help {
		m.barClick(ms.X)
		return nil
	}
	if !m.help && m.panelVisible() && (m.full || ms.X >= m.width-m.panelW+1) {
		m.panelClick(ms.Y - 1)
		return nil
	}
	i, ok := m.cardAt(ms.X, ms.Y)
	if !ok {
		return nil
	}
	now := m.d.Now()
	dbl := i == m.lastIdx && !m.lastAt.IsZero() && now.Sub(m.lastAt) <= doubleTime
	m.moveTo(i)
	m.msg = ""
	if dbl {
		m.lastAt = time.Time{}
		return m.jump(false)
	}
	m.lastIdx, m.lastAt = i, now
	return nil
}

func (m *tuiModel) wheel(ms tea.Mouse) {
	d := 0
	switch ms.Button {
	case tea.MouseWheelUp:
		d = -1
	case tea.MouseWheelDown:
		d = 1
	default:
		return
	}
	if m.full || (m.panelW > 0 && ms.X >= m.width-m.panelW+1) {
		pw := m.width
		if !m.full {
			pw = m.panelW - 1
		}
		n := len(panelLines(m, pw))
		m.panelScroll = min(max(m.panelScroll+3*d, 0), max(0, n-m.bodyH()))
		return
	}
	rows := (len(m.view) + m.cols1() - 1) / m.cols1()
	m.scroll = min(max(m.scroll+d, 0), max(0, rows-m.vis))
}

// ---------- panel ----------

// panelVisible reports whether the inspector is on screen.
func (m *tuiModel) panelVisible() bool {
	return m.panel && (m.full || m.panelW > 0)
}

// panelWidth is the width of the panel's lines.
func (m *tuiModel) panelWidth() int {
	if m.full {
		return m.width
	}
	return m.panelW - 1
}

// panelClick runs the action of the panel line at body row (0 = first row under the top bar).
func (m *tuiModel) panelClick(row int) {
	panelLines(m, m.panelWidth())
	i := row + m.panelScroll
	if row < 0 || i < 0 || i >= len(m.panelActs) || m.panelActs[i] == "" {
		return
	}
	m.panelCur = i
	m.runPanelAct(i)
}

// runPanelAct does what panel line i says: toggle a commit open, or open an issue in the browser.
func (m *tuiModel) runPanelAct(i int) {
	if !m.panelVisible() {
		return
	}
	panelLines(m, m.panelWidth())
	if i < 0 || i >= len(m.panelActs) {
		return
	}
	act := m.panelActs[i]
	switch {
	case strings.HasPrefix(act, "commit:"):
		h := strings.TrimPrefix(act, "commit:")
		if m.openCommit == h {
			m.openCommit = ""
		} else {
			m.openCommit = h
		}
	case strings.HasPrefix(act, "issue:"):
		u := strings.TrimPrefix(act, "issue:")
		if m.d.OpenURL == nil || !strings.HasPrefix(u, "https://") { // never hand xdg-open a file or scheme URL
			return
		}
		m.d.OpenURL(u)
		if n := u[strings.LastIndex(u, "/")+1:]; n != "" {
			m.msg = "opened #" + n + " in the browser"
		} else {
			m.msg = "opened the issue in the browser"
		}
	}
}

// movePanelCur moves the panel cursor to the next (d=1) or previous (d=-1) clickable line.
func (m *tuiModel) movePanelCur(d int) {
	if !m.panelVisible() {
		return
	}
	panelLines(m, m.panelWidth())
	for i := m.panelCur + d; i >= 0 && i < len(m.panelActs); i += d {
		if m.panelActs[i] != "" {
			m.panelCur = i
			h := m.bodyH()
			if i < m.panelScroll {
				m.panelScroll = i
			} else if i >= m.panelScroll+h {
				m.panelScroll = i - h + 1
			}
			return
		}
	}
}

// wrapWords splits s on spaces into lines of at most w cells (long words are cut).
func wrapWords(s string, w int) []string {
	if w < 1 {
		w = 1
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		cur := ""
		for _, word := range strings.Fields(para) {
			for lipgloss.Width(word) > w {
				if cur != "" {
					out = append(out, cur)
					cur = ""
				}
				cut := trunc(word, w)
				cut = strings.TrimSuffix(cut, "…")
				if cut == "" {
					cut = string([]rune(word)[:1])
				}
				out = append(out, cut)
				word = word[len(cut):]
			}
			switch {
			case cur == "":
				cur = word
			case lipgloss.Width(cur)+1+lipgloss.Width(word) <= w:
				cur += " " + word
			default:
				out = append(out, cur)
				cur = word
			}
		}
		out = append(out, cur)
	}
	return out
}

func panelLines(m *tuiModel, w int) []string {
	r, ok := m.selected()
	if !ok {
		m.panelActs = nil
		return nil
	}
	if r.Name != m.panelRepo {
		m.panelRepo = r.Name
		m.resetPanel()
	}
	raw := tuiSettings().Raw
	now := m.d.Now()
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
	head := func(t, extra string) {
		out = append(out, "")
		acts = append(acts, "")
		add(seg{t, fg(cInfo).Bold(true)}, seg{extra, fg(cDim)})
	}
	bg, ink := bandColors(r.Name)
	add(seg{" " + m.title(r) + " ", sty(ink, bg, true)})
	lbl, col := statusInfo(raw, r.Level)
	segs := []seg{{" ● " + lbl + " ", sty(cInk, col, true)}}
	if r.Language != "" {
		segs = append(segs, seg{" ", lipgloss.NewStyle()}, seg{" " + r.Language + " ", sty("#ffffff", langBadgeColor(r.Language), true)})
	}
	add(segs...)
	for _, s := range r.Reasons {
		add(seg{" " + s, fg(cText2)})
	}
	out = append(out, "")
	acts = append(acts, "")
	br := r.Branch
	if r.Base != "" && r.Base != r.Branch {
		br += "  base " + r.Base
	}
	add(seg{"branch  ", fg(cDim)}, seg{br, fg(cText)})
	agent := "no agent"
	switch r.Claude {
	case "busy":
		agent = "working"
	case "waiting":
		agent = "waiting for you"
	case "idle":
		agent = "idle"
	}
	add(seg{"agent   ", fg(cDim)}, seg{agent, fg(cText)})
	tabs := "no workspace open"
	switch {
	case r.Herdr && r.Tmux:
		tabs = "open in herdr and tmux"
	case r.Herdr:
		tabs = "open in herdr"
	case r.Tmux:
		tabs = "open in tmux"
	}
	add(seg{"tab     ", fg(cDim)}, seg{tabs, fg(cText)})

	if cs := m.selCommits(); len(cs) > 0 {
		head("RECENT COMMITS", "")
		for _, c := range cs {
			if c.Hash == "" {
				add(seg{fmt.Sprintf("%5s ", shortAgo(now, c.At)), fg(cDim)}, seg{c.Subject, fg(cText)})
				continue
			}
			mark := "▸"
			if m.openCommit == c.Hash {
				mark = "▾"
			}
			push("commit:"+c.Hash, seg{mark, fg(cInfo)}, seg{fmt.Sprintf("%4s ", shortAgo(now, c.At)), fg(cDim)}, seg{c.Subject, fg(cText)})
			if m.openCommit == c.Hash {
				for _, l := range m.commitDetailLines(r, c.Hash, w-3) {
					add(l...)
				}
			}
		}
	}
	if iss := m.issueFor(r.Name); iss != nil && cardOn(tuiSettings().Raw, "issues") && (iss.Open > 0 || iss.Done != nil) {
		head("ISSUES  ", fmt.Sprintf("%d open", iss.Open))
		if iss.Done != nil {
			push(issueAct(iss.Done.URL), seg{" ✓ ", fg(cGreen)}, seg{fmt.Sprintf("#%d %s", iss.Done.Number, iss.Done.Title), fg(cText2)}, seg{"  " + closedAgo(now, iss.Done.At), fg(cDim)})
		}
		list := iss.OpenList
		if len(list) == 0 && iss.Next != nil {
			list = []issueRef{*iss.Next}
		}
		for i, is := range list {
			mark, c := " · ", cDim
			if i == 0 {
				mark, c = " → ", cInfo
			}
			push(issueAct(is.URL), seg{mark, fg(c)}, seg{fmt.Sprintf("#%d %s", is.Number, is.Title), fg(cText2)})
		}
	}
	if len(r.Worktrees) > 0 {
		head("WORKTREES  ", fmt.Sprint(len(r.Worktrees)))
		for _, wt := range r.Worktrees {
			s := []seg{{" " + wt.Branch, fg(cText)}}
			if wt.Merged {
				s = append(s, seg{" merged", fg(cYellow)})
			}
			if wt.Dirty > 0 {
				s = append(s, seg{fmt.Sprintf(" %d dirty", wt.Dirty), fg(cYellow)})
			}
			add(s...)
		}
	}
	if m.d.Notes != nil {
		if ns := m.d.Notes(r.Name); len(ns) > 0 {
			head("NOTES", "")
			for _, n := range ns {
				add(seg{" " + n, fg(cText2)})
			}
		}
	}
	return out
}

func issueAct(url string) string {
	if url == "" {
		return ""
	}
	return "issue:" + url
}

// commitDetailLines is the expanded part under a commit line: author and date, the message
// body wrapped to w, then the changed files dimmed. The detail is cached, a commit never changes.
func (m *tuiModel) commitDetailLines(r Repo, hash string, w int) [][]seg {
	if m.d.CommitDetail == nil {
		return nil
	}
	dc, ok := m.details[hash]
	if !ok {
		dc.ci, dc.err = m.d.CommitDetail(r.Path, hash)
		m.details[hash] = dc
	}
	ind := seg{"   ", lipgloss.NewStyle()}
	if dc.err != nil {
		return [][]seg{{ind, {dc.err.Error(), fg(cDim)}}}
	}
	ci := dc.ci
	ls := [][]seg{{ind, {ci.Author + "  " + time.Unix(ci.At, 0).Format("2006-01-02 15:04"), fg(cText2)}}}
	if ci.Body != "" {
		for _, l := range wrapWords(ci.Body, w) {
			ls = append(ls, []seg{ind, {l, fg(cText)}})
		}
	}
	for _, f := range ci.Files {
		ls = append(ls, []seg{ind, {f, fg(cDim)}})
	}
	return ls
}

func renderPanel(m *tuiModel, w, h int) string {
	ls := panelLines(m, w)
	m.panelScroll = min(max(m.panelScroll, 0), max(0, len(ls)-h))
	out := make([]string, h)
	for i := range out {
		if j := m.panelScroll + i; j < len(ls) {
			out[i] = ls[j]
		} else {
			out[i] = strings.Repeat(" ", w)
		}
	}
	return strings.Join(out, "\n")
}

// ---------- chrome ----------

func (m *tuiModel) topBar() string {
	raw := tuiSettings().Raw
	title, _ := get(raw, "title").(string)
	if title == "" {
		title = "RepoDash"
	}
	sp := bseg{seg: seg{" ", lipgloss.NewStyle()}}
	head := []bseg{{seg: seg{" " + title, fg(cText).Bold(true)}}, sp, {seg: seg{"●", fg(cGreen)}}, sp}
	arrow := "↕"
	if m.rev {
		arrow = "↓"
	}
	counts := map[string]int{}
	for _, r := range m.repos {
		counts[r.Level]++
	}
	filt := func(mode int) []bseg { // 0 labels and counts, 1 counts only, 2 active filter only
		var s []bseg
		for _, f := range m.filters {
			if mode == 2 && f != m.filter {
				continue
			}
			label, col, n := "all", cText, len(m.repos)
			if f != "all" {
				label, col = statusInfo(raw, f)
				n = counts[f]
			}
			t := fmt.Sprintf("%s %d", label, n)
			if mode == 1 {
				t = fmt.Sprint(n)
			}
			st := fg(col)
			if f == m.filter {
				st = st.Bold(true).Underline(true)
			}
			s = append(s, bseg{seg{t, st}, "filter:" + f}, sp)
		}
		return s
	}
	var search []bseg
	switch {
	case m.searching:
		search = []bseg{{seg: seg{"/" + m.query + "█", fg(cText)}}, sp}
	case m.query != "":
		search = []bseg{{seg: seg{"/" + m.query, fg(cText)}}, sp}
	}
	var agents string
	busy, wait := 0, 0
	for _, r := range m.repos {
		switch r.Claude {
		case "busy":
			busy++
		case "waiting":
			wait++
		}
	}
	if busy+wait > 0 {
		var p []string
		if busy > 0 {
			p = append(p, fmt.Sprintf("%d working", busy))
		}
		if wait > 0 {
			p = append(p, fmt.Sprintf("%d waiting", wait))
		}
		agents = strings.Join(p, " · ") + " "
	}
	warn := []bseg{}
	if n := len(tuiSettings().Warnings); n > 0 {
		warn = []bseg{{seg: seg{fmt.Sprintf("⚠ %d ", n), fg(cRed)}}}
	}
	sortSeg := []bseg{{seg{"sort " + m.sortBy, fg(cMuted)}, "sort"}, sp, {seg{arrow, fg(cMuted)}, "reverse"}, sp, sp}
	cat := func(parts ...[]bseg) []bseg {
		var o []bseg
		for _, p := range parts {
			o = append(o, p...)
		}
		return o
	}
	try := [][]bseg{
		cat(head, sortSeg, filt(0), search, warn),
		cat(head, sortSeg, filt(1), search, warn),
		cat(head, sortSeg, filt(2), search),
		cat(head, sortSeg, search),
	}
	pick := try[len(try)-1]
	extra := ""
	for i, c := range try {
		w := segsWidth(plain(c))
		if i == 0 && agents != "" && w+lipgloss.Width(agents) <= m.width {
			pick, extra = c, agents
			break
		}
		if w <= m.width {
			pick = c
			break
		}
	}
	// Remember where the clickable words are, for mouse clicks on the bar.
	m.barZones = m.barZones[:0]
	x := 0
	for _, b := range pick {
		w := lipgloss.Width(b.s)
		if b.act != "" {
			m.barZones = append(m.barZones, barZone{x, x + w, b.act})
		}
		x += w
	}
	segs := plain(pick)
	if extra != "" {
		pad := m.width - segsWidth(segs) - lipgloss.Width(extra)
		segs = append(segs, seg{strings.Repeat(" ", pad), lipgloss.NewStyle()}, seg{extra, fg(cYellow)})
	}
	return line(m.width, segs...)
}

// bseg is a top-bar segment; act names what a click on it does ("sort", "reverse",
// "filter:<level>"), or "" when it is not clickable.
type bseg struct {
	seg
	act string
}

type barZone struct {
	x0, x1 int
	act    string
}

func plain(bs []bseg) []seg {
	out := make([]seg, len(bs))
	for i, b := range bs {
		out[i] = b.seg
	}
	return out
}

// barClick runs the top-bar action under column x, if any.
func (m *tuiModel) barClick(x int) {
	for _, z := range m.barZones {
		if x < z.x0 || x >= z.x1 {
			continue
		}
		switch {
		case z.act == "sort":
			m.sortBy = cycle(tuiSorts, m.sortBy, 1)
		case z.act == "reverse":
			m.rev = !m.rev
		case strings.HasPrefix(z.act, "filter:"):
			f := strings.TrimPrefix(z.act, "filter:")
			if f == m.filter {
				f = "all"
			}
			m.filter = f
		}
		m.rebuild()
		return
	}
}

func (m *tuiModel) footerLine() string {
	if f := m.footer(); f != "" {
		st := fg(cText)
		if m.prompt != "" {
			st = fg(cYellow).Bold(true)
		}
		return line(m.width, seg{" " + f, st})
	}
	key, desc := fg(cInfo).Bold(true), fg(cDim)
	sp := seg{"  ", lipgloss.NewStyle()}
	var full []seg
	for _, h := range [][2]string{{"hjkl", "move"}, {"⏎", "jump"}, {"n", "new workspace"}, {"s", "sort"}, {"f", "filter"}, {"/", "search"}, {"i", "inspector"}, {"w", "web"}, {"?", "help"}, {"q", "close"}} {
		full = append(full, seg{h[0], key}, seg{" " + h[1], desc}, sp)
	}
	if segsWidth(full)+1 <= m.width {
		return line(m.width, append([]seg{{" ", desc}}, full...)...)
	}
	return line(m.width, seg{" ?", key}, seg{" help  ", desc}, seg{"q", key}, seg{" close", desc})
}

var helpText = [][2]string{
	{"h j k l / arrows", "move between cards"},
	{"g / G", "first / last card"},
	{"ctrl-d / ctrl-u", "half a page down / up"},
	{"Enter", "jump to the repo's workspace"},
	{"n", "new workspace, no question"},
	{"/", "search by name (Enter keeps, Esc clears)"},
	{"s / S", "next sort / reverse"},
	{"f / F", "next / previous status filter"},
	{"r", "rescan now"},
	{"w", "open the web page (starts the server if needed)"},
	{"J / K", "inspector: next / previous commit or issue"},
	{"o", "inspector: expand the commit or open the issue under the cursor"},
	{"i / Tab", "show / hide the inspector (beside the cards, or full screen when narrow)"},
	{"?", "this help"},
	{"q / Esc", "close"},
	{"mouse", "click selects, double-click jumps, wheel scrolls"},
}

func (m *tuiModel) helpBody(h int) []string {
	var ls []string
	ls = append(ls, line(m.width, seg{" Keys", fg(cInfo).Bold(true)}))
	for _, e := range helpText {
		ls = append(ls, line(m.width, seg{fmt.Sprintf(" %-18s", e[0]), fg(cInfo)}, seg{e[1], fg(cText2)}))
	}
	return ls
}

func (m *tuiModel) gridBody(h int) []string {
	gridW := m.width - m.panelW
	if len(m.view) == 0 {
		t := "No repos match"
		if !m.loaded {
			t = "Scanning…"
		}
		return []string{line(gridW, seg{" " + t, fg(cMuted)})}
	}
	inner := max(1, m.cardW-2)
	now := m.d.Now()
	var out []string
	for row := m.scroll; row < m.scroll+m.vis; row++ {
		var cards [][]string
		for c := 0; c < m.cols; c++ {
			i := row*m.cols + c
			if i >= len(m.view) {
				break
			}
			r := m.view[i]
			body := cardBody(r, m.title(r), m.issueFor(r.Name), inner, now)
			cards = append(cards, strings.Split(wrapCard(body, m.cardH-2, inner, i == m.sel), "\n"))
		}
		if len(cards) == 0 {
			break
		}
		for l := 0; l < m.cardH; l++ {
			parts := make([]string, len(cards))
			for c := range cards {
				parts[c] = cards[c][l]
			}
			out = append(out, padTo(strings.Join(parts, " "), gridW))
		}
	}
	return out
}

// View draws the whole screen.
func (m *tuiModel) View() tea.View {
	m.relayout()
	h := m.bodyH()
	var body []string
	switch {
	case m.help:
		body = m.helpBody(h)
	case m.full:
		if strings.TrimSpace(strings.Join(panelLines(m, m.width), "")) == "" {
			body = []string{line(m.width, seg{" No repos match", fg(cMuted)})}
		} else {
			body = strings.Split(renderPanel(m, m.width, h), "\n")
		}
	default:
		body = m.gridBody(h)
		if m.panelW > 0 {
			gridW := m.width - m.panelW
			var pl []string
			if len(panelLines(m, m.panelW-1)) > 0 {
				pl = strings.Split(renderPanel(m, m.panelW-1, h), "\n")
			}
			sep := fg(cBorder).Render("│")
			for i := 0; i < h; i++ {
				l := strings.Repeat(" ", gridW)
				if i < len(body) {
					l = body[i]
				}
				p := strings.Repeat(" ", m.panelW-1)
				if i < len(pl) {
					p = pl[i]
				}
				if i < len(body) {
					body[i] = l + sep + p
				} else {
					body = append(body, l+sep+p)
				}
			}
		}
	}
	if len(body) > h {
		body = body[:h]
	}
	for len(body) < h {
		body = append(body, "")
	}
	all := append([]string{m.topBar()}, body...)
	all = append(all, m.footerLine())
	v := tea.NewView(strings.Join(all, "\n"))
	v.AltScreen = true
	v.WindowTitle = tuiTitle
	v.MouseMode = tea.MouseModeCellMotion
	return v
}
