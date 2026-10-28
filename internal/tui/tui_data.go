package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ezjones/repodash/internal/core"
)

var tuiSorts = []string{"recent", "activity", "language", "alpha", "issues"}

// sortRepos returns a sorted copy. The input is core.ScanAll order (newest activity first) and every
// sort is stable, so ties keep that order, as on the page. Pinned repos come first, in the same
// order among themselves, whatever the sort and its direction.
func sortRepos(rs []core.Repo, by string, rev bool, iss map[string]core.RepoIssues) []core.Repo {
	l := append([]core.Repo(nil), rs...)
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
	sort.SliceStable(l, func(i, j int) bool { return l[i].Pinned && !l[j].Pinned })
	return l
}

// filterRepos keeps repos at the status level ("all" keeps every level) whose name or title
// contains query, ignoring case.
func filterRepos(rs []core.Repo, level, query string, titles map[string]string) []core.Repo {
	q := strings.ToLower(strings.TrimSpace(query))
	out := []core.Repo{}
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
func repoChips(r core.Repo) []chip {
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
