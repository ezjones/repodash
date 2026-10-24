package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var tuiSorts = []string{"recent", "activity", "language", "alpha", "issues"}

// sortRepos returns a sorted copy. The input is scanAll order (newest activity first) and every
// sort is stable, so ties keep that order, as on the page.
func sortRepos(rs []Repo, by string, rev bool, iss map[string]repoIssues) []Repo {
	l := append([]Repo(nil), rs...)
	switch by {
	case "alpha":
		sort.SliceStable(l, func(i, j int) bool { return strings.ToLower(l[i].Name) < strings.ToLower(l[j].Name) })
	case "activity":
		sort.SliceStable(l, func(i, j int) bool { return l[i].Commits7d > l[j].Commits7d })
	case "language":
		sort.SliceStable(l, func(i, j int) bool {
			a, b := l[i].Language, l[j].Language
			if (a == "") != (b == "") {
				return a != ""
			}
			return a < b
		})
	case "issues":
		sort.SliceStable(l, func(i, j int) bool { return iss[l[i].Name].Open > iss[l[j].Name].Open })
	}
	if rev {
		for i, j := 0, len(l)-1; i < j; i, j = i+1, j-1 {
			l[i], l[j] = l[j], l[i]
		}
	}
	return l
}

// filterRepos keeps repos at the status level ("all" keeps every level) whose name or title
// contains query, ignoring case.
func filterRepos(rs []Repo, level, query string, titles map[string]string) []Repo {
	q := strings.ToLower(strings.TrimSpace(query))
	out := []Repo{}
	for _, r := range rs {
		if level != "all" && r.Level != level {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(r.Name), q) && !strings.Contains(strings.ToLower(titles[r.Name]), q) {
			continue
		}
		out = append(out, r)
	}
	return out
}

type chip struct{ Kind, Text string }

// repoChips is the card's chip list, same entries and order as the page.
func repoChips(r Repo) []chip {
	var c []chip
	plural := func(n int, one, many string) string {
		if n > 1 {
			return strconv.Itoa(n) + " " + many
		}
		return strconv.Itoa(n) + " " + one
	}
	if r.Ahead > 0 {
		c = append(c, chip{"bad", fmt.Sprintf("↑%d unpushed", r.Ahead)})
	} else if r.HasRemote && !r.HasUpstream && r.LastCommit != 0 {
		c = append(c, chip{"bad", "not pushed"})
	}
	if r.Conflicts > 0 {
		c = append(c, chip{"bad", plural(r.Conflicts, "conflict", "conflicts")})
	}
	if r.Behind > 0 {
		c = append(c, chip{"warn", fmt.Sprintf("↓%d behind", r.Behind)})
	}
	if r.Staged > 0 {
		c = append(c, chip{"warn", fmt.Sprintf("%d staged", r.Staged)})
	}
	if r.Modified > 0 {
		c = append(c, chip{"warn", fmt.Sprintf("%d modified", r.Modified)})
	}
	if r.Untracked > 0 {
		c = append(c, chip{"warn", fmt.Sprintf("%d untracked", r.Untracked)})
	}
	if r.Stashes > 0 {
		c = append(c, chip{"info", plural(r.Stashes, "stash", "stashes")})
	}
	if len(c) == 0 {
		c = append(c, chip{"ok", "in sync"})
	}
	return c
}

// ago is the page's ago(): how long before now the unix time ts was.
func ago(now time.Time, ts int64) string {
	if ts == 0 {
		return "never"
	}
	d := now.Unix() - ts
	for _, u := range []struct {
		n int64
		s string
	}{{31536000, "y"}, {2592000, "mo"}, {86400, "d"}, {3600, "h"}, {60, "m"}} {
		if d >= u.n {
			return fmt.Sprintf("%d%s ago", d/u.n, u.s)
		}
	}
	return "just now"
}

type commitLine struct {
	Hash    string
	At      int64
	Subject string
}

// recentCommits lists the newest n commits of the repo at dir, newest first.
func recentCommits(dir string, n int) []commitLine {
	out := git(dir, "log", "-n", strconv.Itoa(n), "--format=%H%x09%ct%x09%s")
	var cs []commitLine
	for _, l := range strings.Split(out, "\n") {
		hash, rest, ok := strings.Cut(l, "\t")
		if !ok {
			continue
		}
		ts, subj, ok := strings.Cut(rest, "\t")
		if !ok {
			continue
		}
		at, err := strconv.ParseInt(ts, 10, 64)
		if err != nil {
			continue
		}
		cs = append(cs, commitLine{hash, at, subj})
	}
	return cs
}

// notesExcerpt returns the first lines non-empty lines of the repo's notes, or nil.
func notesExcerpt(name string, lines int) []string {
	b, err := os.ReadFile(filepath.Join(notesDir, name+".md"))
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
			if len(out) == lines {
				break
			}
		}
	}
	return out
}

// commitInfo is one commit's details, as the inspector shows them when it is expanded.
type commitInfo struct {
	Hash   string   `json:"hash"`
	Author string   `json:"author"`
	At     int64    `json:"at"`
	Body   string   `json:"body"`
	Files  []string `json:"files"`
}

var hashRe = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

const (
	maxCommitBody  = 4000
	maxCommitFiles = 30
)

// commitDetail reads author, date, message body and changed files of one commit. The hash is
// checked first, so it can never be taken for an option.
func commitDetail(dir, hash string) (commitInfo, error) {
	if !hashRe.MatchString(hash) {
		return commitInfo{}, errors.New("not a commit hash")
	}
	out, err := gitRaw(dir, "show", "--no-color", "--no-patch", "--format=%H%x00%an%x00%ct%x00%B", hash)
	if err != nil {
		return commitInfo{}, errors.New("commit not found")
	}
	f := strings.SplitN(strings.TrimRight(out, "\n"), "\x00", 4)
	if len(f) < 4 {
		return commitInfo{}, errors.New("unreadable commit")
	}
	ci := commitInfo{Hash: f[0], Author: f[1], Files: []string{}}
	ci.At, _ = strconv.ParseInt(f[2], 10, 64)
	body := strings.TrimSpace(f[3])
	// %B starts with the subject, which the caller already shows.
	_, body, _ = strings.Cut(body, "\n")
	body = strings.TrimSpace(body)
	if len(body) > maxCommitBody {
		body = strings.ToValidUTF8(body[:maxCommitBody], "") + "…"
	}
	ci.Body = body
	stat, _ := gitRaw(dir, "show", "--no-color", "--format=", "--stat=200", hash)
	ls := strings.Split(strings.TrimRight(stat, "\n"), "\n")
	for len(ls) > 0 && strings.TrimSpace(ls[0]) == "" {
		ls = ls[1:]
	}
	if len(ls) > 0 {
		last := ls[len(ls)-1]
		files := ls[:len(ls)-1]
		if len(files) > maxCommitFiles {
			files = files[:maxCommitFiles]
		}
		for _, l := range files {
			ci.Files = append(ci.Files, strings.TrimSpace(l))
		}
		ci.Files = append(ci.Files, strings.TrimSpace(last))
	}
	return ci, nil
}
