package core

import (
	"reflect"
	"testing"
)

func TestParseChangelog(t *testing.T) {
	got := parseChangelog("# Changelog\n\nIntro line.\n\n## Unreleased\n\n## v0.2.0\n\n- New `L` key.\n- Two\n  lines.\n\n## v0.1.0\n\n- First.\n")
	want := []Release{{"v0.2.0", []string{"New `L` key.", "Two lines."}}, {"v0.1.0", []string{"First."}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v", got)
	}
}

func TestNewsDue(t *testing.T) {
	for _, c := range []struct {
		seen, run   string
		known, want bool
	}{
		{"v0.7.10", "v0.7.11", false, true},
		{"v0.7.11", "v0.7.11", true, false},
		{"", "v0.7.11", true, true},   // existing user, first run of a build that remembers
		{"", "v0.7.11", false, false}, // fresh install
		{"v0.7.10", "dev", true, false},
	} {
		if got := NewsDue(c.seen, c.run, c.known); got != c.want {
			t.Errorf("NewsDue(%q, %q, %v) = %v", c.seen, c.run, c.known, got)
		}
	}
}
