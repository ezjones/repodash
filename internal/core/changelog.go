package core

import "strings"

// Release is one version's section of CHANGELOG.md: its heading and its items.
type Release struct {
	Tag   string   `json:"tag"`
	Items []string `json:"items"`
}

var changelogText string

// SetChangelog gives core the CHANGELOG.md text main embeds (go:embed cannot reach the repo root
// from here).
func SetChangelog(s string) { changelogText = s }

// Changelog is every release in CHANGELOG.md, newest first, as the file lists them. Each "- "
// line starts an item; indented lines under it continue it. Sections with no items are left
// out, which drops an empty "Unreleased".
func Changelog() []Release { return parseChangelog(changelogText) }

func parseChangelog(s string) []Release {
	var rs []Release
	var cur *Release
	for _, l := range strings.Split(s, "\n") {
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(l, "## "):
			rs = append(rs, Release{Tag: strings.TrimSpace(l[3:])})
			cur = &rs[len(rs)-1]
		case cur == nil || t == "" || strings.HasPrefix(l, "#"):
		case strings.HasPrefix(t, "- ") && !strings.HasPrefix(l, "  "):
			cur.Items = append(cur.Items, strings.TrimSpace(t[2:]))
		case len(cur.Items) > 0:
			cur.Items[len(cur.Items)-1] += " " + strings.TrimPrefix(t, "- ")
		}
	}
	out := rs[:0]
	for _, r := range rs {
		if len(r.Items) > 0 {
			out = append(out, r)
		}
	}
	return out
}

// NewsDue says whether to show "What's new" on start: the running version is a release that
// was not seen last time. With nothing seen yet, only an existing user (known says they have
// saved choices from an older run) gets it; a fresh install starts quiet.
func NewsDue(seen, running string, known bool) bool {
	if _, ok := parseVersion(running); !ok || seen == running {
		return false
	}
	return seen != "" || known
}
