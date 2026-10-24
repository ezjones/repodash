package main

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

const (
	hashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	hashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// panelFake is a fake TUI with the inspector open on my-app, two commits and two issues.
func panelFake(t *testing.T) (*fakeTUI, *[]string) {
	t.Helper()
	f := newFakeTUI(t, false)
	var urls []string
	f.m.d.Commits = func(string) []commitLine {
		return []commitLine{{hashA, 900, "first-subject"}, {hashB, 800, "second-subject"}}
	}
	f.m.d.CommitDetail = func(dir, hash string) (commitInfo, error) {
		if hash == hashB {
			return commitInfo{}, errors.New("detail-broke")
		}
		return commitInfo{Hash: hash, Author: "Pat Author", At: 900, Body: "body-line-one and more", Files: []string{"a.go | 2 +-", "1 file changed"}}, nil
	}
	f.m.d.OpenURL = func(u string) { urls = append(urls, u) }
	f.m.issues = map[string]repoIssues{"my-app": {Open: 2, OpenList: []issueRef{
		{Number: 7, Title: "issue-seven", URL: "https://example.com/o/r/issues/7"},
		{Number: 9, Title: "issue-nine", URL: "https://example.com/o/r/issues/9"},
	}}}
	sized(f, 120, 40)
	f.keys("i")
	return f, &urls
}

// rowOf finds the screen row whose text contains s.
func rowOf(t *testing.T, f *fakeTUI, s string) int {
	t.Helper()
	for y, l := range strings.Split(f.m.View().Content, "\n") {
		if strings.Contains(l, s) && (strings.Contains(l, "│") || f.m.full) {
			// the card grid also sits on the row; the panel text is right of the separator
			if i := strings.LastIndex(l, "│"); i >= 0 && strings.Contains(l[i:], s) {
				return y
			}
		}
	}
	t.Fatalf("%q not on screen:\n%s", s, f.m.View().Content)
	return 0
}

func clickPanel(f *fakeTUI, y int) {
	f.send(tea.MouseClickMsg{X: f.m.width - 5, Y: y, Button: tea.MouseLeft})
}

func TestPanelClickCommit(t *testing.T) {
	f, _ := panelFake(t)
	if strings.Contains(f.m.View().Content, "body-line-one") {
		t.Fatal("detail shown before any click")
	}
	clickPanel(f, rowOf(t, f, "first-subject"))
	v := f.m.View().Content
	for _, want := range []string{"▾", "Pat Author", "body-line-one", "a.go | 2 +-"} {
		if !strings.Contains(v, want) {
			t.Errorf("%q missing after click:\n%s", want, v)
		}
	}
	if f.m.openCommit != hashA {
		t.Fatalf("openCommit %q", f.m.openCommit)
	}
	clickPanel(f, rowOf(t, f, "first-subject"))
	v = f.m.View().Content
	if f.m.openCommit != "" || strings.Contains(v, "body-line-one") || !strings.Contains(v, "▸") {
		t.Fatalf("second click did not close:\n%s", v)
	}
	// An error shows as text.
	clickPanel(f, rowOf(t, f, "second-subject"))
	if v := f.m.View().Content; !strings.Contains(v, "detail-broke") {
		t.Fatalf("error not shown:\n%s", v)
	}
	// Changing the selection closes it.
	f.keys("j")
	if f.m.openCommit != "" {
		t.Fatal("selection change kept the commit open")
	}
}

func TestPanelClickIssue(t *testing.T) {
	f, urls := panelFake(t)
	clickPanel(f, rowOf(t, f, "issue-nine"))
	if len(*urls) != 1 || (*urls)[0] != "https://example.com/o/r/issues/9" {
		t.Fatalf("opened %v", *urls)
	}
	if f.m.msg != "opened #9 in the browser" {
		t.Fatalf("msg %q", f.m.msg)
	}
	// A click on a line with no action does nothing.
	clickPanel(f, rowOf(t, f, "RECENT COMMITS"))
	if len(*urls) != 1 || f.m.openCommit != "" {
		t.Fatal("click on a heading did something")
	}
}

func TestPanelKeys(t *testing.T) {
	f, urls := panelFake(t)
	f.keys("J", "o")
	if f.m.openCommit != hashA {
		t.Fatalf("J o: openCommit %q", f.m.openCommit)
	}
	f.keys("o")
	if f.m.openCommit != "" {
		t.Fatal("o again did not close")
	}
	f.keys("J", "J", "o") // second commit, then first issue
	if len(*urls) != 1 || (*urls)[0] != "https://example.com/o/r/issues/7" {
		t.Fatalf("opened %v", *urls)
	}
	f.keys("K", "K", "K", "K")
	if f.m.panelCur < 0 || f.m.panelActs[f.m.panelCur] != "commit:"+hashA {
		t.Fatalf("K did not stop at the first commit: %d", f.m.panelCur)
	}
	f.keys("i") // hidden: J and o do nothing
	f.keys("J", "o")
	if f.m.openCommit != "" || len(*urls) != 1 {
		t.Fatal("keys acted with the panel hidden")
	}
}
