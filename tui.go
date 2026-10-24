package main

import (
	"fmt"
	"slices"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
)

// tuiDeps is everything the terminal UI touches outside itself, so tests can fake it.
type tuiDeps struct {
	Scan          func() []Repo
	Panes         func() []pane                // agent refresh, every 1 s
	Issues        func() map[string]repoIssues // current issues; may return nil
	Find          func(dir string) (mux string, exists bool, err error)
	Open          func(mux, dir string) (action, window string, err error)
	Commits       func(dir string) []commitLine
	CommitDetail  func(dir, hash string) (commitInfo, error)
	OpenURL       func(url string)
	Notes         func(name string) []string
	Now           func() time.Time
	Web           func() (url string, err error) // open the web page, starting the server if needed
	QuitAfterJump bool                           // in a herdr popup, which blocks herdr until it closes
}

type (
	scanMsg   []Repo
	panesMsg  []pane
	issuesMsg map[string]repoIssues
	jumpMsg   struct {
		name, action, window string
		err                  error
	}
	webMsg struct {
		url string
		err error
	}
	tickMsg string // "scan", "panes" or "issues"
)

type detailCache struct {
	ci  commitInfo
	err error
}

type commitCache struct {
	at time.Time
	cs []commitLine
}

type tuiModel struct {
	d tuiDeps

	repos  []Repo // all, as scanned
	view   []Repo // sorted and filtered
	issues map[string]repoIssues
	panes  []pane
	titles map[string]string

	sel         int
	sortBy      string
	rev         bool
	filter      string // all | bad | warn | ok | stale
	filters     []string
	query       string
	searching   bool
	panel       bool
	help        bool
	prompt      string // repo name awaiting y/n, "" when none
	promptMux   string
	promptDir   string // folder the prompt will open on y
	promptLabel string // what the prompt calls it
	msg         string
	width       int
	height      int
	scroll      int // first visible card row
	panelScroll int
	loaded      bool

	cols     int // cards per row; set by relayout
	pageRows int // rows per page, for ctrl+d / ctrl+u

	full          bool // the panel is on and fills the screen (narrow terminals); set by relayout
	cardW, panelW int
	barZones      []barZone // clickable words in the top bar, set by topBar
	cardH, vis    int
	visKey        visKey
	lastIdx       int
	lastAt        time.Time

	interval time.Duration
	commits  map[string]commitCache

	panelActs  []string // what each panel line does ("commit:<hash>", "issue:<url>", ""), set by panelLines
	panelCur   int      // panel cursor: a line index, -1 when none
	panelRepo  string   // repo the panel state below belongs to
	openCommit string   // hash of the expanded commit, "" when none
	details    map[string]detailCache
}

// tuiSettings returns the live settings, loading defaults when none were loaded yet.
func tuiSettings() *settings {
	if s := Config(); s != nil {
		return s
	}
	s, _ := parseSettings("", nil)
	SetConfig(s)
	return s
}

func newTUIModel(d tuiDeps) *tuiModel {
	if d.Now == nil {
		d.Now = time.Now
	}
	m := &tuiModel{d: d, cols: 1, pageRows: 6, lastIdx: -1, commits: map[string]commitCache{}, details: map[string]detailCache{}, panelCur: -1}
	m.applySettings(true)
	if d.Issues != nil {
		if is := d.Issues(); is != nil {
			m.issues = is
			m.rebuild()
		}
	}
	return m
}

// applySettings reads the settings the model depends on. first also sets the initial sort,
// order and filter from defaults.*; later reloads keep what the user picked.
func (m *tuiModel) applySettings(first bool) {
	s := tuiSettings()
	m.interval = s.interval
	m.filters = []string{"all"}
	for _, k := range strs(s.Raw, "topbar.filters") {
		if show, ok := get(s.Raw, "statuses."+k+".show").(bool); ok && !show {
			continue
		}
		m.filters = append(m.filters, k)
	}
	m.titles = map[string]string{}
	if rs, ok := s.Raw["repos"].(map[string]any); ok {
		for n, v := range rs {
			if e, ok := v.(map[string]any); ok {
				if t, ok := e["title"].(string); ok && t != "" {
					m.titles[n] = t
				}
			}
		}
	}
	if first {
		m.sortBy, _ = get(s.Raw, "defaults.sort").(string)
		if !slices.Contains(tuiSorts, m.sortBy) {
			m.sortBy = "recent"
		}
		m.rev, _ = get(s.Raw, "defaults.reverse").(bool)
		m.filter, _ = get(s.Raw, "defaults.filter").(string)
	}
	if !slices.Contains(m.filters, m.filter) {
		m.filter = "all"
	}
}

func (m *tuiModel) title(r Repo) string {
	if t := m.titles[r.Name]; t != "" {
		return t
	}
	return r.Name
}

// selected returns the highlighted repo, if any.
func (m *tuiModel) selected() (Repo, bool) {
	if m.sel < 0 || m.sel >= len(m.view) {
		return Repo{}, false
	}
	return m.view[m.sel], true
}

// rebuild recomputes view and keeps the selection on the same repo name; if that repo is gone
// it clamps the index.
func (m *tuiModel) rebuild() {
	name := ""
	if r, ok := m.selected(); ok {
		name = r.Name
	}
	l := sortRepos(m.repos, m.sortBy, m.rev, m.issues)
	m.view = filterRepos(l, m.filter, m.query, m.titles)
	for i, r := range m.view {
		if name != "" && r.Name == name {
			m.sel = i
			return
		}
	}
	m.clamp()
}

func (m *tuiModel) clamp() {
	if m.sel >= len(m.view) {
		m.sel = len(m.view) - 1
	}
	if m.sel < 0 {
		m.sel = 0
	}
}

func (m *tuiModel) moveTo(i int) {
	if len(m.view) == 0 {
		return
	}
	old := m.sel
	m.sel = i
	m.clamp()
	if m.sel != old {
		m.panelScroll = 0
		m.resetPanel()
	}
}

// resetPanel forgets the expanded commit and the panel cursor (the selection changed).
func (m *tuiModel) resetPanel() {
	m.openCommit, m.panelCur = "", -1
}

func (m *tuiModel) cols1() int {
	if m.cols < 1 {
		return 1
	}
	return m.cols
}

// selCommits returns the selected repo's recent commits, cached for the scan interval.
func (m *tuiModel) selCommits() []commitLine {
	r, ok := m.selected()
	if !ok || m.d.Commits == nil {
		return nil
	}
	now := m.d.Now()
	if c, ok := m.commits[r.Name]; ok && now.Sub(c.at) < m.interval {
		return c.cs
	}
	cs := m.d.Commits(r.Path)
	m.commits[r.Name] = commitCache{now, cs}
	return cs
}

func (m *tuiModel) applyPanes() {
	for i := range m.repos {
		m.repos[i].Tmux, m.repos[i].Herdr, m.repos[i].Claude = agentState(m.panes, m.repos[i].Path)
	}
}

func (m *tuiModel) Init() tea.Cmd {
	return tea.Batch(m.scanCmd(), m.panesCmd(), tick("scan", m.interval), tick("panes", time.Second), tick("issues", 30*time.Second))
}

func tick(kind string, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return tickMsg(kind) })
}

func (m *tuiModel) scanCmd() tea.Cmd {
	scan := m.d.Scan
	if scan == nil {
		return nil
	}
	return func() tea.Msg { return scanMsg(scan()) }
}

func (m *tuiModel) panesCmd() tea.Cmd {
	panes := m.d.Panes
	if panes == nil {
		return nil
	}
	return func() tea.Msg { return panesMsg(panes()) }
}

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer m.relayout()
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		return m, m.click(tea.Mouse(msg))
	case tea.MouseWheelMsg:
		m.wheel(tea.Mouse(msg))
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case scanMsg:
		m.repos = []Repo(msg)
		m.loaded = true
		m.applyPanes()
		m.rebuild()
	case panesMsg:
		m.panes = []pane(msg)
		m.applyPanes()
		m.rebuild()
	case issuesMsg:
		m.issues = map[string]repoIssues(msg)
		m.rebuild()
	case tickMsg:
		switch string(msg) {
		case "scan":
			return m, tea.Batch(m.scanCmd(), tick("scan", m.interval))
		case "panes":
			if PollConfig() {
				m.applySettings(false)
				m.rebuild()
			}
			return m, tea.Batch(m.panesCmd(), tick("panes", time.Second))
		case "issues":
			if m.d.Issues != nil {
				if is := m.d.Issues(); is != nil {
					m.issues = is
					m.rebuild()
				}
			}
			return m, tick("issues", 30*time.Second)
		}
	case webMsg:
		if msg.err != nil {
			m.msg = msg.err.Error()
		} else {
			m.msg = "web page at " + msg.url
		}
	case jumpMsg:
		if msg.err != nil {
			m.msg = msg.err.Error()
			return m, nil
		}
		if m.d.QuitAfterJump {
			return m, tea.Quit
		}
		verb := "opened"
		if msg.action == "jumped" {
			verb = "jumped to"
		}
		m.msg = verb + " " + msg.name
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m *tuiModel) openCmd(name, mux, dir string) tea.Cmd {
	open := m.d.Open
	return func() tea.Msg {
		a, w, err := open(mux, dir)
		return jumpMsg{name: name, action: a, window: w, err: err}
	}
}

func cycle(l []string, cur string, step int) string {
	i := 0
	for j, v := range l {
		if v == cur {
			i = j
		}
	}
	return l[((i+step)%len(l)+len(l))%len(l)]
}

func (m *tuiModel) key(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	m.msg = ""

	if m.prompt != "" {
		name, mux, dir := m.prompt, m.promptMux, m.promptDir
		switch s {
		case "y":
			m.prompt = ""
			if dir != "" {
				return m, m.openCmd(name, mux, dir)
			}
		case "n", "esc", "q":
			m.prompt = ""
		}
		return m, nil
	}

	if m.searching {
		switch s {
		case "enter":
			m.searching = false
		case "esc":
			m.searching, m.query = false, ""
			m.rebuild()
		case "backspace":
			if r := []rune(m.query); len(r) > 0 {
				m.query = string(r[:len(r)-1])
				m.rebuild()
			}
		default:
			if k.Text != "" && k.Mod&^tea.ModShift == 0 {
				m.query += k.Text
				m.rebuild()
			}
		}
		return m, nil
	}

	if m.help {
		switch s {
		case "?", "esc", "q", "enter":
			m.help = false
		}
		return m, nil
	}

	switch s {
	case "q":
		return m, tea.Quit
	case "esc":
		if m.full {
			m.panel = false
			return m, nil
		}
		if m.query != "" {
			m.query = ""
			m.rebuild()
			return m, nil
		}
		return m, tea.Quit
	case "?":
		m.help = true
	case "/":
		m.searching = true
	case "i", "tab":
		m.panel = !m.panel
	case "J":
		m.movePanelCur(1)
	case "K":
		m.movePanelCur(-1)
	case "o":
		return m, m.runPanelAct(m.panelCur)
	case "h", "left":
		m.moveTo(m.sel - 1)
	case "l", "right":
		m.moveTo(m.sel + 1)
	case "j", "down":
		m.moveTo(m.sel + m.cols1())
	case "k", "up":
		m.moveTo(m.sel - m.cols1())
	case "g", "home":
		m.moveTo(0)
	case "G", "end":
		m.moveTo(len(m.view) - 1)
	case "ctrl+d":
		m.moveTo(m.sel + m.half()*m.cols1())
	case "ctrl+u":
		m.moveTo(m.sel - m.half()*m.cols1())
	case "s":
		m.sortBy = cycle(tuiSorts, m.sortBy, 1)
		m.rebuild()
	case "S":
		m.rev = !m.rev
		m.rebuild()
	case "f":
		m.filter = cycle(m.filters, m.filter, 1)
		m.rebuild()
	case "F":
		m.filter = cycle(m.filters, m.filter, -1)
		m.rebuild()
	case "r":
		return m, m.scanCmd()
	case "w":
		if m.d.Web == nil {
			return m, nil
		}
		m.msg = "opening the web page…"
		web := m.d.Web
		return m, func() tea.Msg {
			u, err := web()
			return webMsg{u, err}
		}
	case "enter", "n":
		return m, m.jump(s == "n")
	}
	return m, nil
}

func (m *tuiModel) half() int {
	if h := m.pageRows / 2; h > 1 {
		return h
	}
	return 1
}

// jump handles Enter (ask if there is no tab yet) and n (create without asking).
func (m *tuiModel) jump(force bool) tea.Cmd {
	r, ok := m.selected()
	if !ok {
		return nil
	}
	return m.jumpTo(r.Name, m.title(r), r.Path, force)
}

// jumpTo focuses the terminal tab in dir, or asks (unless force) before opening one.
// name is what the success message calls it, label what the prompt calls it.
func (m *tuiModel) jumpTo(name, label, dir string, force bool) tea.Cmd {
	if m.d.Find == nil || m.d.Open == nil {
		return nil
	}
	mux, exists, err := m.d.Find(dir)
	if err != nil {
		m.msg = err.Error()
		return nil
	}
	if !exists && !force {
		m.prompt, m.promptMux, m.promptDir, m.promptLabel = name, mux, dir, label
		return nil
	}
	return m.openCmd(name, mux, dir)
}

// footer is the one-line prompt or message under the grid.
func (m *tuiModel) footer() string {
	switch {
	case m.prompt != "":
		t := m.promptLabel
		if t == "" {
			t = m.prompt
		}
		return fmt.Sprintf("Create a workspace for %s? y/n", t)
	case m.searching:
		return "/" + m.query
	}
	return m.msg
}

// runTUI runs the interface against the real repos and terminals. Settings must be loaded.
func runTUI(root string, web func() (string, error)) error {
	var mu sync.Mutex
	var last []Repo
	var issues map[string]repoIssues
	cachePath := issueCachePath()
	var fetchedAt time.Time
	if cachePath != "" {
		if c, ok := readIssueCache(cachePath); ok {
			issues, fetchedAt = c.Repos, c.At
		}
	}
	mode := func() string {
		if inHerdr() {
			return "herdr"
		}
		return tuiSettings().multiplexer
	}
	d := tuiDeps{
		Scan: func() []Repo {
			rs := scanAll(root)
			mu.Lock()
			last = rs
			mu.Unlock()
			return rs
		},
		Panes: terminalPanes,
		Issues: func() map[string]repoIssues {
			mu.Lock()
			defer mu.Unlock()
			return issues
		},
		Find:          func(dir string) (string, bool, error) { return findTarget(dir, mode()) },
		Open:          openWith,
		Commits:       func(dir string) []commitLine { return recentCommits(dir, 5) },
		CommitDetail:  commitDetail,
		OpenURL:       openBrowser,
		Notes:         func(name string) []string { return notesExcerpt(name, 5) },
		Now:           time.Now,
		QuitAfterJump: inHerdrPopup(),
		Web:           web,
	}
	p := tea.NewProgram(newTUIModel(d))

	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			mu.Lock()
			repos, at := last, fetchedAt
			mu.Unlock()
			if repos != nil && tuiSettings().issuesOn && time.Since(at) >= issuesEvery {
				dirs := map[string]string{}
				for _, r := range repos {
					dirs[r.Name] = r.Path
				}
				res, problem := fetchAllIssues(dirs)
				now := time.Now()
				mu.Lock()
				fetchedAt = now
				if problem == "" { // a failed or partial fetch keeps the cached issues
					issues = res
				}
				cur := issues
				mu.Unlock()
				if problem == "" {
					if cachePath != "" {
						writeIssueCache(cachePath, issueCache{At: now, Repos: res})
					}
					p.Send(issuesMsg(cur))
				}
			}
			select {
			case <-done:
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()
	_, err := p.Run()
	return err
}
