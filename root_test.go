package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultRoot(t *testing.T) {
	home := t.TempDir()
	repos := filepath.Join(home, "code")
	os.MkdirAll(filepath.Join(repos, "my-app", ".git"), 0o755)
	os.MkdirAll(filepath.Join(home, "empty"), 0o755)
	os.MkdirAll(filepath.Join(home, "dotfiles", ".git"), 0o755) // a repo right in home
	want := filepath.Join(home, "gitrepos")
	for cwd, exp := range map[string]string{
		repos:                          repos, // a folder of repos
		filepath.Join(repos, "my-app"): want,  // inside one repo
		filepath.Join(home, "empty"):   want,  // no repos here
		home:                           want,  // home never counts, even with a repo in it
		filepath.Join(home, "missing"): want,
		"":                             want,
	} {
		if got := defaultRoot(cwd, home); got != exp {
			t.Errorf("defaultRoot(%q) = %q, want %q", cwd, got, exp)
		}
	}
}
