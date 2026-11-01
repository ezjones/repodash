package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkBounds(t *testing.T) {
	now := time.Date(2026, 11, 4, 15, 30, 0, 0, time.Local)
	mid := time.Date(2026, 11, 4, 0, 0, 0, 0, time.Local)
	for _, c := range []struct {
		name     string
		from, to time.Time
	}{
		{"today", mid, mid.AddDate(0, 0, 1)},
		{"yesterday", mid.AddDate(0, 0, -1), mid},
		{"week", mid.AddDate(0, 0, -6), mid.AddDate(0, 0, 1)},
		{"month", mid.AddDate(0, 0, -29), mid.AddDate(0, 0, 1)},
	} {
		f, to, ok := WorkBounds(c.name, now)
		if !ok || !f.Equal(c.from) || !to.Equal(c.to) {
			t.Errorf("%s: %v..%v ok=%v", c.name, f, to, ok)
		}
	}
	if _, _, ok := WorkBounds("year", now); ok {
		t.Error("unknown range accepted")
	}
}

func TestWork(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	from, to, _ := WorkBounds("today", now)
	commit := func(dir, msg string, at time.Time) {
		t.Helper()
		cmd := exec.Command("git", "-C", dir, "commit", "-q", "--allow-empty", "-m", msg)
		d := at.Format(time.RFC3339)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+d, "GIT_COMMITTER_DATE="+d,
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	mk := func(name string) string {
		d := filepath.Join(root, name)
		if out, err := exec.Command("git", "init", "-q", "-b", "main", d).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		return d
	}
	a, b, c := mk("app-a"), mk("app-b"), mk("app-c")
	commit(a, "old work", from.Add(-48*time.Hour))
	commit(a, "morning", from.Add(time.Minute))
	// a commit on another local branch (as in a worktree) counts too
	exec.Command("git", "-C", a, "checkout", "-q", "-b", "side").Run()
	commit(a, "on a branch", from.Add(2*time.Minute))
	commit(b, "yesterday only", from.Add(-time.Hour))
	commit(c, "later", from.Add(3*time.Minute))

	repos := []Repo{
		{Name: "app-a", Path: a},
		{Name: "app-b", Path: b, Modified: 2, Edited: from.Add(10 * time.Minute).Unix()},
		{Name: "app-c", Path: c, Untracked: 1, Edited: from.Add(-time.Hour).Unix()}, // edited yesterday: not today's work
	}
	ws := Work(repos, from, to)
	got := map[string]RepoWork{}
	var order []string
	for _, w := range ws {
		got[w.Name] = w
		order = append(order, w.Name)
	}
	if strings.Join(order, ",") != "app-b,app-c,app-a" {
		t.Errorf("order %v", order)
	}
	if w := got["app-a"]; len(w.Commits) != 2 || w.Commits[0].Subject != "on a branch" || w.Commits[1].Subject != "morning" {
		t.Errorf("app-a commits %+v", w.Commits)
	}
	if w := got["app-b"]; len(w.Commits) != 0 || w.Uncommitted != 2 {
		t.Errorf("app-b %+v", w)
	}
	if w := got["app-c"]; w.Uncommitted != 0 || len(w.Commits) != 1 {
		t.Errorf("app-c %+v", w)
	}

	md := WorkMarkdown("Today", ws, map[string]string{"app-a": "App A"})
	for _, want := range []string{"## Today", "### App A", "- on a branch", "- (uncommitted: 2 files)"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "old work") || strings.Contains(md, "yesterday only") {
		t.Errorf("markdown has work from other days:\n%s", md)
	}
}
