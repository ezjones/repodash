package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Worktree struct {
	Branch string `json:"branch"`
	Merged bool   `json:"merged"`
	Dirty  int    `json:"dirty"`
}

// Repo is everything the dashboard (and later the CLI) knows about one checkout.
type Repo struct {
	Name        string     `json:"name"`
	Path        string     `json:"path"`
	Branch      string     `json:"branch"`
	Base        string     `json:"base"`
	HasRemote   bool       `json:"has_remote"`
	HasUpstream bool       `json:"has_upstream"`
	Ahead       int        `json:"ahead"`
	Behind      int        `json:"behind"`
	Staged      int        `json:"staged"`
	Modified    int        `json:"modified"`
	Untracked   int        `json:"untracked"`
	Conflicts   int        `json:"conflicts"`
	Stashes     int        `json:"stashes"`
	LastCommit  int64      `json:"last_commit"` // unix seconds
	LastSubject string     `json:"last_subject"`
	Edited      int64      `json:"edited"`   // newest mtime among uncommitted files, 0 when clean
	Activity    int64      `json:"activity"` // max(LastCommit, Edited): what "recent" sorts by
	Commits7d   int        `json:"commits_7d"`
	Language    string     `json:"language"` // main language by tracked file count, "" when none is recognised
	Worktrees   []Worktree `json:"worktrees"`
	Image       string     `json:"image"`     // cover URL, "" when there is none (the page draws generated art)
	ImageSrc    string     `json:"image_src"` // custom | root | dir | readme
	ImageFit    string     `json:"image_fit"` // cover | contain
	Tmux        bool       `json:"tmux"`
	Herdr       bool       `json:"herdr"`
	Claude      string     `json:"claude"` // waiting | busy | idle | ""
	Level       string     `json:"level"`  // bad | warn | ok | stale
	Reasons     []string   `json:"reasons"`
}

// git runs one read-only git command. GIT_OPTIONAL_LOCKS=0 keeps `status` from
// taking index.lock, which would otherwise race with whatever you are doing in
// that repo while the dashboard polls it.
func git(dir string, args ...string) string {
	out, _ := gitRaw(dir, args...)
	return strings.TrimRight(out, "\n")
}

func gitRaw(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	// A repo's .git/config can name programs git will run (core.fsmonitor runs on every
	// `git status`), and repos under the root are not all ones we made. Switch that off.
	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.fsmonitor=false", "-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
	b, err := cmd.Output()
	return string(b), err
}

type pane struct{ cwd, claude, mux string }

func tmuxPanes() []pane {
	b, err := exec.Command("tmux", "list-panes", "-a", "-F", "#{pane_current_path}\t#{@claude}").Output()
	if err != nil {
		return nil
	}
	var ps []pane
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		cwd, st, _ := strings.Cut(l, "\t")
		if cwd != "" {
			ps = append(ps, pane{cwd, st, "tmux"})
		}
	}
	return ps
}

// terminalPanes gathers panes from every multiplexer. Which one a card uses is decided per
// browser (Settings panel) or by terminal.multiplexer, so the scan does not filter.
func terminalPanes() []pane { return append(tmuxPanes(), herdrPanes()...) }

// agentState folds every pane inside dir into: has a tmux tab, has a herdr tab, and the most
// urgent agent state (waiting beats busy beats idle).
func agentState(panes []pane, dir string) (tm, hd bool, claude string) {
	for _, p := range panes {
		if p.cwd != dir && !strings.HasPrefix(p.cwd, dir+"/") {
			continue
		}
		tm = tm || p.mux == "tmux"
		hd = hd || p.mux == "herdr"
		switch {
		case strings.Contains(p.claude, "waiting"):
			claude = "waiting"
		case strings.Contains(p.claude, "busy") && claude != "waiting":
			claude = "busy"
		case strings.Contains(p.claude, "idle") && claude == "":
			claude = "idle"
		}
	}
	return
}

func defaultBranch(dir string) string {
	if h := git(dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); h != "" {
		return h
	}
	for _, b := range []string{"main", "master"} {
		if git(dir, "rev-parse", "--verify", "--quiet", b) != "" {
			return b
		}
	}
	return ""
}

func isAncestor(dir, a, b string) bool {
	return exec.Command("git", "-c", "core.fsmonitor=false", "-C", dir, "merge-base", "--is-ancestor", a, b).Run() == nil
}

func worktrees(dir, base string) []Worktree {
	var res []Worktree
	var path, branch string
	flush := func(first bool) {
		if !first && path != "" && branch != "" {
			dirty := 0
			if st := git(path, "status", "--porcelain"); st != "" {
				dirty = len(strings.Split(st, "\n"))
			}
			res = append(res, Worktree{
				Branch: branch,
				Merged: base != "" && isAncestor(dir, branch, base),
				Dirty:  dirty,
			})
		}
		path, branch = "", ""
	}
	first := true
	for _, l := range append(strings.Split(git(dir, "worktree", "list", "--porcelain"), "\n"), "") {
		switch {
		case l == "":
			flush(first)
			first = false
		case strings.HasPrefix(l, "worktree "):
			path = strings.TrimPrefix(l, "worktree ")
		case strings.HasPrefix(l, "branch "):
			branch = strings.TrimPrefix(strings.TrimPrefix(l, "branch "), "refs/heads/")
		}
	}
	return res
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// parseStatus reads `git status --porcelain=v2 --branch -z`.
func parseStatus(r *Repo, out string) {
	recs := strings.Split(out, "\x00")
	var edited int64
	stats := 0
	touch := func(rel string) {
		if stats >= 300 || rel == "" {
			return
		}
		stats++
		if fi, err := os.Lstat(filepath.Join(r.Path, strings.TrimSuffix(rel, "/"))); err == nil {
			if t := fi.ModTime().Unix(); t > edited {
				edited = t
			}
		}
	}
	for i := 0; i < len(recs); i++ {
		rec := recs[i]
		switch {
		case strings.HasPrefix(rec, "# branch.head "):
			r.Branch = strings.TrimPrefix(rec, "# branch.head ")
			if r.Branch == "(detached)" {
				r.Branch = "detached"
			}
		case strings.HasPrefix(rec, "# branch.upstream "):
			r.HasUpstream = true
		case strings.HasPrefix(rec, "# branch.ab "):
			f := strings.Fields(rec)
			if len(f) >= 4 {
				r.Ahead, _ = strconv.Atoi(strings.TrimPrefix(f[2], "+"))
				r.Behind, _ = strconv.Atoi(strings.TrimPrefix(f[3], "-"))
			}
		case strings.HasPrefix(rec, "? "):
			r.Untracked++
			touch(rec[2:])
		case strings.HasPrefix(rec, "1 "), strings.HasPrefix(rec, "2 "), strings.HasPrefix(rec, "u "):
			n := map[byte]int{'1': 9, '2': 10, 'u': 11}[rec[0]]
			f := strings.SplitN(rec, " ", n)
			if len(f) < n {
				continue
			}
			xy := f[1]
			if rec[0] == 'u' {
				r.Conflicts++
			} else {
				if xy[0] != '.' {
					r.Staged++
				}
				if xy[1] != '.' {
					r.Modified++
				}
			}
			touch(f[n-1])
			if rec[0] == '2' {
				i++ // rename records carry the original path as one more NUL field
			}
		}
	}
	r.Edited = edited
}

func inspect(root, name string, panes []pane) Repo {
	dir := filepath.Join(root, name)
	r := Repo{Name: name, Path: dir, Worktrees: []Worktree{}, Reasons: []string{}}
	parseStatus(&r, git(dir, "status", "--porcelain=v2", "--branch", "-z"))

	if ts, subj, ok := strings.Cut(git(dir, "log", "-1", "--format=%ct\t%s"), "\t"); ok {
		r.LastCommit, _ = strconv.ParseInt(ts, 10, 64)
		r.LastSubject = subj
	}
	r.Activity = max(r.LastCommit, r.Edited)
	r.Base = strings.TrimPrefix(defaultBranch(dir), "origin/")
	r.HasRemote = git(dir, "remote") != ""
	r.Stashes = len(strings.Fields(git(dir, "stash", "list")))
	if n := git(dir, "rev-list", "--count", "--since=7.days.ago", "HEAD"); n != "" {
		r.Commits7d, _ = strconv.Atoi(n)
	}
	r.Language = mainLanguage(dir)
	// Compare against the same ref git would, so a fresh branch is not "merged" into itself.
	baseRef := defaultBranch(dir)
	r.Worktrees = append(r.Worktrees, worktrees(dir, baseRef)...)

	r.Tmux, r.Herdr, r.Claude = agentState(panes, dir)
	if c, ok := findCover(root, name); ok {
		r.Image, r.ImageSrc, r.ImageFit = c.url(name), c.src, c.fit
	}
	classify(&r)
	return r
}

func plural(n int, s string) string {
	if n == 1 {
		return strconv.Itoa(n) + " " + s
	}
	return strconv.Itoa(n) + " " + s + "s"
}

func classify(r *Repo) {
	var bad, warn []string
	if !r.HasRemote {
		bad = append(bad, "no remote")
	}
	if r.Ahead > 0 {
		bad = append(bad, strconv.Itoa(r.Ahead)+" unpushed")
	} else if r.HasRemote && !r.HasUpstream && r.LastCommit > 0 {
		bad = append(bad, "branch not pushed")
	}
	if r.Conflicts > 0 {
		bad = append(bad, plural(r.Conflicts, "conflict"))
	}
	if d := r.Staged + r.Modified + r.Untracked; d > 0 {
		warn = append(warn, strconv.Itoa(d)+" uncommitted")
	}
	if r.Behind > 0 {
		warn = append(warn, strconv.Itoa(r.Behind)+" behind")
	}
	merged := 0
	for _, w := range r.Worktrees {
		merged += b2i(w.Merged)
	}
	if merged > 0 {
		warn = append(warn, plural(merged, "merged worktree"))
	}
	r.Reasons = append(append([]string{}, bad...), warn...)
	switch {
	case len(bad) > 0:
		r.Level = "bad"
	case len(warn) > 0:
		r.Level = "warn"
	case r.LastCommit > 0 && time.Since(time.Unix(r.LastCommit, 0)) > time.Duration(cfg.get().staleDays)*24*time.Hour:
		r.Level = "stale"
	default:
		r.Level = "ok"
	}
}

// scanAll inspects every git checkout directly under root, newest activity first.
// Entries whose .git is a file are linked worktrees; they show up inside their
// main repo's card instead of getting one of their own.
func scanAll(root string) []Repo {
	s := cfg.get()
	ents, err := os.ReadDir(root)
	if err != nil {
		return []Repo{}
	}
	var names []string
	for _, e := range ents {
		n := e.Name()
		if !e.IsDir() || strings.HasPrefix(n, ".") || s.skip[n] || s.repos[n].Hidden {
			continue
		}
		if fi, err := os.Stat(filepath.Join(root, n, ".git")); err == nil && fi.IsDir() {
			names = append(names, n)
		}
	}
	panes := terminalPanes()
	repos := make([]Repo, len(names))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, n := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			repos[i] = inspect(root, n, panes)
		}()
	}
	wg.Wait()
	sort.SliceStable(repos, func(i, j int) bool {
		if repos[i].Activity != repos[j].Activity {
			return repos[i].Activity > repos[j].Activity
		}
		return strings.ToLower(repos[i].Name) < strings.ToLower(repos[j].Name)
	})
	return repos
}
