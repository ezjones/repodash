package core

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// WorkRanges are the spans the inspector's Today mode offers, in its order.
var WorkRanges = []string{"today", "yesterday", "week", "month"}

// WorkBounds is the time span of a range name, in local time: today since midnight,
// yesterday the whole day before, week the last seven days including today, month the last 30.
func WorkBounds(name string, now time.Time) (from, to time.Time, ok bool) {
	mid := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := mid.AddDate(0, 0, 1)
	switch name {
	case "today":
		return mid, end, true
	case "yesterday":
		return mid.AddDate(0, 0, -1), mid, true
	case "week":
		return mid.AddDate(0, 0, -6), end, true
	case "month":
		return mid.AddDate(0, 0, -29), end, true
	}
	return time.Time{}, time.Time{}, false
}

// WorkCommit is one commit in a work summary.
type WorkCommit struct {
	Hash    string `json:"hash"`
	At      int64  `json:"at"` // committer time, unix seconds
	Subject string `json:"subject"`
}

// RepoWork is what happened in one repo during a range: its commits, newest first, and its
// uncommitted files when they were last edited inside the range.
type RepoWork struct {
	Name        string       `json:"name"`
	Commits     []WorkCommit `json:"commits"`
	Uncommitted int          `json:"uncommitted"` // staged + modified + untracked files; 0 when none or edited outside the range
	Edited      int64        `json:"edited"`      // newest edit of those files, 0 with Uncommitted
	Last        int64        `json:"last"`        // newest commit or edit in the range: the sort key
}

// maxWorkCommits caps the commits listed per repo; RepoWork says how many were cut by having
// exactly this many.
const maxWorkCommits = 50

// Work lists the repos with commits (on any local branch, so worktree branches count) or edits
// between from and to, most recent first. It runs one git log per repo, a few at a time.
func Work(repos []Repo, from, to time.Time) []RepoWork {
	out := make([]RepoWork, len(repos))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, r := range repos {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i] = repoWork(r, from, to)
		}()
	}
	wg.Wait()
	var ws []RepoWork
	for _, w := range out {
		if len(w.Commits) > 0 || w.Uncommitted > 0 {
			ws = append(ws, w)
		}
	}
	sort.SliceStable(ws, func(a, b int) bool { return ws[a].Last > ws[b].Last })
	return ws
}

func repoWork(r Repo, from, to time.Time) RepoWork {
	w := RepoWork{Name: r.Name, Commits: []WorkCommit{}}
	if n := r.Staged + r.Modified + r.Untracked; n > 0 && r.Edited >= from.Unix() && r.Edited < to.Unix() {
		w.Uncommitted, w.Edited, w.Last = n, r.Edited, r.Edited
	}
	out := Git(r.Path, "log", "--branches", "--since=@"+strconv.FormatInt(from.Unix(), 10),
		"--until=@"+strconv.FormatInt(to.Unix()-1, 10), "-n", strconv.Itoa(maxWorkCommits), "--format=%H%x09%ct%x09%s")
	for _, l := range strings.Split(out, "\n") {
		f := strings.SplitN(l, "\t", 3)
		if len(f) < 3 {
			continue
		}
		at, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil || at < from.Unix() || at >= to.Unix() {
			continue
		}
		w.Commits = append(w.Commits, WorkCommit{Hash: f[0], At: at, Subject: f[2]})
		w.Last = max(w.Last, at)
	}
	return w
}

// WorkMarkdown is a summary to paste into a standup note: one heading per repo, its commits
// as a list, then its uncommitted files. title heads it ("Today", "Yesterday", "This week").
func WorkMarkdown(title string, ws []RepoWork, titles map[string]string) string {
	var b strings.Builder
	b.WriteString("## " + title + "\n")
	if len(ws) == 0 {
		b.WriteString("\nNothing committed or edited.\n")
	}
	for _, w := range ws {
		name := w.Name
		if t := titles[w.Name]; t != "" {
			name = t
		}
		b.WriteString("\n### " + name + "\n\n")
		for _, c := range w.Commits {
			b.WriteString("- " + c.Subject + "\n")
		}
		if len(w.Commits) == maxWorkCommits {
			b.WriteString("- …and more\n")
		}
		if w.Uncommitted > 0 {
			fmt.Fprintf(&b, "- (uncommitted: %s)\n", plural(w.Uncommitted, "file"))
		}
	}
	return b.String()
}
