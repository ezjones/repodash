# Changelog

What each release added. The release workflow copies the section for the tag it builds onto the GitHub release page, so write it for people using RepoDash, not for its code. One line per item: GitHub shows every line break.

## Unreleased

- The terminal UI lists `U update` in the key hints; `U` checks GitHub for a new release right away.

## v0.7.7

- Terminal UI: `H` hides the key hints at the bottom and gives the cards that row back. RepoDash remembers the choice. The row still appears for a search, a question, a message or an update offer.

## v0.7.6

- The update offer sits at the bottom right of the terminal UI, next to the running version.
- `U` (and the web page's Update button) asks GitHub fresh instead of using the once-a-day answer.

## v0.7.5

- The terminal UI shows the running version at the right of the footer.

## v0.7.4

- Self-update. Once a day RepoDash checks for a new release. `U` in the terminal UI, or the Update button on the web page, downloads it, checks it against the release's checksums, replaces the binary and restarts. A herdr install is told to use `herdr plugin update` instead.

## v0.7.3

- Terminal UI: Tab moves the keys into the inspector (commits, issues, worktrees) and back.

## v0.7.2

- Terminal UI: `n` opens a notes window to read, add or edit a repo's notes; `c` creates a workspace without asking.

## v0.7.1

- Terminal UI: an animated agent badge on each card; `e` edits notes in `$EDITOR`, and the inspector shows all of them.
- Plain `repodash` scans the current folder when it holds repos, else `~/gitrepos`.
- `go install github.com/ezjones/repodash@latest` works.
