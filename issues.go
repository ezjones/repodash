package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// GitHub issues on the cards: the last issue closed and the next one to start. The data comes
// from the `gh` CLI (its own login; RepoDash holds no token), only for repos whose origin is on
// github.com, and only while a browser is open. It is a separate, slow feed (issuesEvery) so the
// git scan stays fast. Read-only: nothing here creates or edits an issue.

const issuesEvery = 5 * time.Minute

type issueRef struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	At     string `json:"at,omitempty"` // closed time (RFC 3339), only for Done
}

type repoIssues struct {
	Open int       `json:"open"`           // open issue count (filled by the feed from Task 2 on)
	Done *issueRef `json:"done,omitempty"` // most recently closed issue
	Next *issueRef `json:"next,omitempty"` // oldest open issue
}

var ghName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// githubRepo returns "owner/name" when the remote URL points at github.com, else "".
func githubRepo(url string) string {
	url = strings.TrimSpace(url)
	for _, p := range []string{"https://github.com/", "http://github.com/", "ssh://git@github.com/", "git@github.com:", "git://github.com/"} {
		if rest, ok := strings.CutPrefix(url, p); ok {
			rest = strings.TrimSuffix(strings.TrimSuffix(rest, "/"), ".git")
			own, name, ok := strings.Cut(rest, "/")
			// Both parts end up in an API path, so they must be plain names (no "..", no "?").
			if ok && ghName.MatchString(own) && ghName.MatchString(name) && own != "." && own != ".." && name != "." && name != ".." {
				return own + "/" + name
			}
		}
	}
	return ""
}

type ghIssue struct {
	Number   int    `json:"number"`
	Title    string `json:"title"`
	URL      string `json:"html_url"`
	ClosedAt string `json:"closed_at"`
}

var errGHAuth = errors.New("gh is not logged in (run: gh auth login)")

// ghIssues lists issues of one state through the REST API, pull requests filtered out.
func ghIssues(slug, state, sort, dir string) ([]ghIssue, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "api",
		"repos/"+slug+"/issues?state="+state+"&sort="+sort+"&direction="+dir+"&per_page=30",
		"--jq", `[.[] | select(.pull_request == null) | {number, title, html_url, closed_at}]`)
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1", "NO_COLOR=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if s := stderr.String(); strings.Contains(s, "gh auth login") || strings.Contains(s, "HTTP 401") {
			return nil, errGHAuth
		}
		return nil, err // 404 (no access) and 410 (issues disabled) just mean "nothing to show"
	}
	var list []ghIssue
	err = json.Unmarshal(out, &list)
	return list, err
}

func fetchIssues(slug string) (repoIssues, error) {
	var ri repoIssues
	closed, err := ghIssues(slug, "closed", "updated", "desc")
	if errors.Is(err, errGHAuth) {
		return ri, err
	}
	// A closed issue that was commented on later sorts first by "updated", so take the newest
	// closed_at among the page instead of the first row.
	for _, c := range closed {
		if c.ClosedAt != "" && (ri.Done == nil || c.ClosedAt > ri.Done.At) {
			ri.Done = &issueRef{Number: c.Number, Title: c.Title, URL: c.URL, At: c.ClosedAt}
		}
	}
	open, err := ghIssues(slug, "open", "created", "asc")
	if errors.Is(err, errGHAuth) {
		return ri, err
	}
	if len(open) > 0 {
		ri.Next = &issueRef{Number: open[0].Number, Title: open[0].Title, URL: open[0].URL}
	}
	return ri, nil
}

// issueFeed keeps the last fetch and decides when the next one is due.
type issueFeed struct {
	mu      sync.Mutex
	payload []byte // {"repos":{name:{done,next}},"error":""}; nil until the first fetch
	at      time.Time
	busy    bool
}

func (f *issueFeed) current() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.payload
}

// due reports whether a fetch should start now and, if so, marks one as running.
func (f *issueFeed) due() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.busy || time.Since(f.at) < issuesEvery {
		return false
	}
	f.busy = true
	return true
}

// poll fetches issues for every repo (a few at a time) and broadcasts if anything changed.
func (h *hub) pollIssues() {
	h.mu.Lock()
	dirs := h.dirs
	h.mu.Unlock()
	f := &h.issues
	if len(dirs) == 0 { // the first scan has not finished: try again on the next tick
		f.mu.Lock()
		f.busy = false
		f.mu.Unlock()
		return
	}
	defer func() { f.mu.Lock(); f.busy = false; f.at = time.Now(); f.mu.Unlock() }()

	res := map[string]repoIssues{}
	msg := ""
	if _, err := exec.LookPath("gh"); err != nil {
		msg = "GitHub issues: the gh command was not found. Install the GitHub CLI and run gh auth login, or set card.issues to false."
	} else {
		var mu sync.Mutex
		sem := make(chan struct{}, 4)
		var wg sync.WaitGroup
		for name, dir := range dirs {
			slug := githubRepo(git(dir, "config", "--get", "remote.origin.url"))
			if slug == "" {
				continue
			}
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				ri, err := fetchIssues(slug)
				mu.Lock()
				defer mu.Unlock()
				if errors.Is(err, errGHAuth) {
					msg = "GitHub issues: " + err.Error() + "."
				}
				if ri.Done != nil || ri.Next != nil {
					res[name] = ri
				}
			}()
		}
		wg.Wait()
	}
	b, _ := json.Marshal(map[string]any{"repos": res, "error": msg})
	f.mu.Lock()
	changed := !bytes.Equal(b, f.payload)
	f.payload = b
	f.mu.Unlock()
	if changed {
		h.broadcast("issues", b)
	}
}
