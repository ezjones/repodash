# Changelog

What each release added. The release workflow copies the section for the tag it builds onto the GitHub release page, so write it for people using RepoDash, not for its code. One line per item: GitHub shows every line break.

## Unreleased

- Fixed: right after the web server starts, the inspector's All tab could show an empty list until the next refresh.

## v0.7.12

- What's new: after an update, RepoDash shows what changed in this release and the ones before it, in a box in the middle of the screen (Esc or Enter closes it). Click the version number to see it again: bottom right in the terminal UI, top left on the web page.
- The inspector has two tabs: **This repo** (as before) and **All**, which lists what you did across all repos, with each repo's commits and uncommitted work. Press `t` to open it, or `1` / `2` to switch tabs with the inspector open. In All, `d` / `D` / `w` / `m` show today / yesterday / this week / this month, Enter jumps to a repo, and `y` (or *Copy as markdown* on the web page) copies the list for a standup note.
- Drag the line on the inspector's left edge to make it wider or narrower, in the terminal UI and on the web page. The width is remembered; on the web page a double-click resets it.

## v0.7.11

- Press `L` in the terminal UI to open lazygit in the selected repo. Inside herdr it opens as a popup; quit lazygit and you are back on the cards.

## v0.7.10

- Pin a repo with `p` and it stays at the top, whatever the sort or its direction. Works in the terminal UI and on the web page (click the pin on a card to unpin), and both share the same pins.

## v0.7.9

- See how full each Claude Code agent's context is, live: the card shows `82k` next to the agent icon (yellow from 70% of the window, red from 90%), and the inspector shows `82k of 1M (8%)`. Both the web page and the terminal UI. It reads what Claude Code gives its status line, so it needs a few lines added there (see the README).

## v0.7.8

- Terminal UI: an Appearance menu (`a`, or click ◐ in the top bar). Pick a card theme (colourful, plain, soft, ink, or coloured by status), show the status as words or just a coloured dot, turn the language badge on or off, and give any repo a colour of its own that stays under every theme. Works with keys and the mouse, and RepoDash remembers it all.
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
