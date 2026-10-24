package main

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

type CommitLine struct {
	Hash    string
	At      int64
	Subject string
}

// RecentCommits lists the newest n commits of the repo at dir, newest first.
func RecentCommits(dir string, n int) []CommitLine {
	out := Git(dir, "log", "-n", strconv.Itoa(n), "--format=%H%x09%ct%x09%s")
	var cs []CommitLine
	for _, l := range strings.Split(out, "\n") {
		hash, rest, ok := strings.Cut(l, "\t")
		if !ok {
			continue
		}
		ts, subj, ok := strings.Cut(rest, "\t")
		if !ok {
			continue
		}
		at, err := strconv.ParseInt(ts, 10, 64)
		if err != nil {
			continue
		}
		cs = append(cs, CommitLine{Hash: hash, At: at, Subject: subj})
	}
	return cs
}

// CommitInfo is one commit's details, as the inspector shows them when it is expanded.
type CommitInfo struct {
	Hash   string   `json:"hash"`
	Author string   `json:"author"`
	At     int64    `json:"at"`
	Body   string   `json:"body"`
	Files  []string `json:"files"`
}

var hashRe = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

const (
	maxCommitBody  = 4000
	maxCommitFiles = 30
)

// CommitDetail reads author, date, message body and changed files of one commit. The hash is
// checked first, so it can never be taken for an option.
func CommitDetail(dir, hash string) (CommitInfo, error) {
	if !hashRe.MatchString(hash) {
		return CommitInfo{}, errors.New("not a commit hash")
	}
	out, err := gitRaw(dir, "show", "--no-color", "--no-patch", "--format=%H%x00%an%x00%ct%x00%B", hash)
	if err != nil {
		return CommitInfo{}, errors.New("commit not found")
	}
	f := strings.SplitN(strings.TrimRight(out, "\n"), "\x00", 4)
	if len(f) < 4 {
		return CommitInfo{}, errors.New("unreadable commit")
	}
	ci := CommitInfo{Hash: f[0], Author: f[1], Files: []string{}}
	ci.At, _ = strconv.ParseInt(f[2], 10, 64)
	body := strings.TrimSpace(f[3])
	// %B starts with the subject, which the caller already shows.
	_, body, _ = strings.Cut(body, "\n")
	body = strings.TrimSpace(body)
	if len(body) > maxCommitBody {
		body = strings.ToValidUTF8(body[:maxCommitBody], "") + "…"
	}
	ci.Body = body
	stat, _ := gitRaw(dir, "show", "--no-color", "--format=", "--stat=200", hash)
	ls := strings.Split(strings.TrimRight(stat, "\n"), "\n")
	for len(ls) > 0 && strings.TrimSpace(ls[0]) == "" {
		ls = ls[1:]
	}
	if len(ls) > 0 {
		last := ls[len(ls)-1]
		files := ls[:len(ls)-1]
		if len(files) > maxCommitFiles {
			files = files[:maxCommitFiles]
		}
		for _, l := range files {
			ci.Files = append(ci.Files, strings.TrimSpace(l))
		}
		ci.Files = append(ci.Files, strings.TrimSpace(last))
	}
	return ci, nil
}

// ValidHash reports whether h looks like a commit hash (7 to 40 lowercase hex digits).
func ValidHash(h string) bool { return hashRe.MatchString(h) }
