package main

import (
	"testing"
	"time"
)

func TestBandColorsMatchWeb(t *testing.T) {
	for _, c := range []struct {
		name, bg, ink string
		h             uint32
	}{
		{"my-app", "#2A2F2B", "#ffffff", 4014489609},
		{"api-server", "#2E6F40", "#ffffff", 2484544505},
		{"a", "#2F5D62", "#ffffff", 3826002220},
		{"demo", "#F3E7E2", "#2E2A33", 0},
	} {
		bg, ink := bandColors(c.name)
		if bg != c.bg || ink != c.ink {
			t.Errorf("%s: %s %s, want %s %s", c.name, bg, ink, c.bg, c.ink)
		}
		if c.h != 0 && nameHash(c.name) != c.h {
			t.Errorf("hash %s = %d", c.name, nameHash(c.name))
		}
	}
	for l, want := range map[string]string{"Go": "#1a6172", "Rust": "#70331a", "Nope": "#3f4a54"} {
		if got := langBadgeColor(l); got != want {
			t.Errorf("%s: %s want %s", l, got, want)
		}
	}
}

func TestSortRepos(t *testing.T) {
	rs := []Repo{{Name: "b", Commits7d: 1, Language: "Go"}, {Name: "A", Commits7d: 5}, {Name: "c", Commits7d: 5, Language: "Rust"}}
	iss := map[string]RepoIssues{"b": {Open: 9}, "c": {Open: 2}}
	names := func(l []Repo) string {
		s := ""
		for _, r := range l {
			s += r.Name
		}
		return s
	}
	for _, c := range []struct {
		by   string
		rev  bool
		want string
	}{
		{"recent", false, "bAc"}, {"alpha", false, "Abc"}, {"activity", false, "Acb"},
		{"language", false, "bcA"}, {"issues", false, "bcA"}, {"issues", true, "Acb"},
	} {
		if got := names(sortRepos(rs, c.by, c.rev, iss)); got != c.want {
			t.Errorf("%s rev=%v: %s want %s", c.by, c.rev, got, c.want)
		}
	}
	if names(rs) != "bAc" {
		t.Error("input was modified")
	}
}

func TestFilterRepos(t *testing.T) {
	rs := []Repo{{Name: "my-app", Level: "bad"}, {Name: "api-server", Level: "ok"}}
	if got := filterRepos(rs, "bad", "", nil); len(got) != 1 || got[0].Name != "my-app" {
		t.Fatal(got)
	}
	if got := filterRepos(rs, "all", "API", nil); len(got) != 1 || got[0].Name != "api-server" {
		t.Fatal(got)
	}
	if got := filterRepos(rs, "all", "front", map[string]string{"my-app": "Frontend"}); len(got) != 1 {
		t.Fatal(got)
	}
	if got := filterRepos(rs, "stale", "", nil); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestRepoChipsAndAgo(t *testing.T) {
	c := repoChips(Repo{Ahead: 11, Untracked: 1, Stashes: 2})
	if len(c) != 3 || c[0] != (chip{"bad", "↑11 unpushed"}) || c[2] != (chip{"info", "2 stashes"}) {
		t.Fatal(c)
	}
	if c := repoChips(Repo{}); len(c) != 1 || c[0] != (chip{"ok", "in sync"}) {
		t.Fatal(c)
	}
	if c := repoChips(Repo{HasRemote: true, LastCommit: 1}); len(c) != 1 || c[0].Text != "not pushed" {
		t.Fatal(c)
	}
	now := time.Unix(1_000_000_000, 0)
	for ts, want := range map[int64]string{0: "never", 999_999_990: "just now", 999_999_520: "8m ago", 999_740_800: "3d ago"} {
		if got := ago(now, ts); got != want {
			t.Errorf("%d: %s want %s", ts, got, want)
		}
	}
}
