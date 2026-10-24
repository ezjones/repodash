package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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
	Open     int        `json:"open"`                // exact open issue count
	Done     *issueRef  `json:"done,omitempty"`      // most recently closed issue
	Next     *issueRef  `json:"next,omitempty"`      // oldest open issue
	OpenList []issueRef `json:"open_list,omitempty"` // up to 5 oldest open issues
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
	for i, o := range open {
		if i == 0 {
			ri.Next = &issueRef{Number: o.Number, Title: o.Title, URL: o.URL}
		}
		if i < 5 {
			ri.OpenList = append(ri.OpenList, issueRef{Number: o.Number, Title: o.Title, URL: o.URL})
		}
	}
	ri.Open = len(open) // fallback: the page is capped at 30
	if n, err := ghOpenCount(slug); err == nil {
		ri.Open = n
	} else if errors.Is(err, errGHAuth) {
		return ri, err
	}
	return ri, nil
}

// ghOpenCount returns the exact number of open issues (pull requests are not issues in GraphQL).
func ghOpenCount(slug string) (int, error) {
	own, name, ok := strings.Cut(slug, "/")
	if !ok {
		return 0, errors.New("bad repo slug")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "api", "graphql",
		"-f", "query=query($o:String!,$n:String!){repository(owner:$o,name:$n){issues(states:OPEN){totalCount}}}",
		"-F", "o="+own, "-F", "n="+name,
		"--jq", ".data.repository.issues.totalCount")
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1", "NO_COLOR=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if s := stderr.String(); strings.Contains(s, "gh auth login") || strings.Contains(s, "HTTP 401") {
			return 0, errGHAuth
		}
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}

// issueCache is the last successful fetch, kept on disk so the terminal UI can show issues at once.
type issueCache struct {
	At    time.Time             `json:"at"`
	Repos map[string]repoIssues `json:"repos"`
}

func issueCachePath() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "repodash", "issues.json")
}

func readIssueCache(path string) (issueCache, bool) {
	var c issueCache
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, &c) != nil {
		return issueCache{}, false
	}
	return c, true
}

func writeIssueCache(path string, c issueCache) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "issues-*.json")
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		os.Remove(f.Name())
	}
	return err
}

// fetchAllIssues looks up issues for every repo in dirs (name to path), a few at a time.
// msg is a user-facing problem (gh missing or logged out), empty when all went well.
func fetchAllIssues(dirs map[string]string) (res map[string]repoIssues, msg string) {
	res = map[string]repoIssues{}
	if _, err := exec.LookPath("gh"); err != nil {
		return res, "GitHub issues: the gh command was not found. Install the GitHub CLI and run gh auth login, or set card.issues to false."
	}
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
	return res, msg
}
