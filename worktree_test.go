package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWorktreeOf(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "my-app")
	wt := filepath.Join(root, "my-app-feature")
	run := func(dir string, args ...string) {
		t.Helper()
		c := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	os.Mkdir(repo, 0o755)
	run(repo, "init", "-q")
	run(repo, "commit", "-q", "--allow-empty", "-m", "first")
	run(repo, "worktree", "add", "-q", "-b", "feature", wt)

	if !worktreeOf(repo, wt) {
		t.Error("listed worktree refused")
	}
	for _, bad := range []string{"", "/etc", repo, repo + "/../x", wt + "/..", wt + "/sub", filepath.Join(root, "other")} {
		if worktreeOf(repo, bad) {
			t.Errorf("%q accepted", bad)
		}
	}
	if got := worktrees(repo, ""); len(got) != 1 || got[0].Path != wt {
		t.Errorf("worktrees %+v", got)
	}
}
