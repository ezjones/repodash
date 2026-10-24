# RepoDash TUI and herdr plugin: design

Date: 2026-10-24. Status: built and merged.

**Changes after the build, at the owner's request:** herdr opens the TUI in its own tab, not a
popup (a herdr popup takes all input until it closes), and the open action reuses that tab; the
TUI keeps running after a jump. The inspector (side panel) starts hidden and `i` toggles it at
any width. Plain `repodash` from a terminal is now the TUI (`w` opens the web page, starting the
server if needed; `-web` is the server). The web page gained the same inspector. Where this
document says popup, read tab.

## Intent

The owner lives in herdr. When they want to switch projects, they want to see every repo the
way the web Grid shows it, pick one with the keyboard or mouse, and land in that repo's herdr
workspace, or create one if it has none. Today that means leaving the terminal for the browser.
The goal is RepoDash's Grid view, in the terminal, opened from a key inside herdr.

Prior art the owner pointed at: `jeffarese/herdr-agent-grid` and `furkankly/zoetrope`. Both are
Rust/ratatui herdr plugins: a `herdr-plugin.toml` declares a full-size popup pane running a
native binary, and a key in `~/.config/herdr/config.toml` opens it.

## Goals

- A terminal view of the repo grid with the web cards' information, in colour, with a details
  panel on the right.
- Vim-style keys plus arrows, and mouse support.
- Enter jumps to the repo's workspace; if none exists, asks, then creates it and jumps.
- A herdr plugin so it opens as a popup from a key.
- Same settings as the web page: one `repodash.json`, so both views agree.

## Non-goals

- Canvas and Kanban in the terminal. They stay in the browser.
- Editing anything: no notes editing, no cover images, no layout changes, no issue changes.
- An "issues" sort or open-issue count on the web page (could come later, see below).
- Windows-native support. Linux and macOS, like the release builds.

## Anti-goals (a failure even if it works)

- The popup feels slow to open. It must draw a usable grid fast (see Constraints).
- The two views disagree: different status, different colours for the same repo, or settings
  that apply to one and not the other.
- It writes to the user's herdr config, a repo, or `layout.json` without being asked.
- It breaks the existing binary's behaviour, flags, or web page.

## Constraints

- Go, inside this repo and this binary. Owner decided: **Bubble Tea is allowed** (Charm's
  libraries, pure Go). `CGO_ENABLED=0` must still build a static binary. This is the first
  dependency; AGENTS.md's "standard library only" convention changes to "standard library plus
  Charm's TUI libraries (Bubble Tea, Lip Gloss, and Bubbles if useful) for the TUI only".
  Server code keeps to the standard library.
- Read-only toward repos, as today (`GIT_OPTIONAL_LOCKS=0`, `core.fsmonitor=false`, never fetch).
- Public project: no real repo names, hostnames or home paths in code, tests, docs or commits.
- A full scan of ~40 repos takes about 0.3 s today (`repodash -json`). The popup should show the
  grid within about half a second of opening; slower data (issues) fills in after.
- Testing must not disturb the owner's real herdr or tmux sessions (see Testing).

## What the owner decided

### Look: compact cards (mockup option B)

A grid of bordered cards filling the left side; a details panel on the right; a top bar; a key
hint line at the bottom. Each card:

```
╭────────────────────────────────────╮
│ my-app        ● Needs action  Go   │  <- band: name bold on the repo's colour, status pill, language badge, agent icon
│ main                               │  <- branch (and "base <branch>" for worktree checkouts, as on the web)
│ [↑11 unpushed] [1 untracked]       │  <- chips, same wording and colours as the web
│ 8m ago  progress row with a bar…   │  <- last commit (or "edited Xd ago  uncommitted work"), truncated
│ ✓ #41 login fix · 2d               │  <- issues: last closed (Done)
│ → #44 dark mode                    │  <- issues: next open
│ ▮▮▮▮▮▮▮▮▮ 509 this week            │  <- commit bar
╰────────────────────────────────────╯
```

The highlighted card has an accent-coloured border. The band colour is the same colour the web
page gives that repo in its `name` cover style (port `hash`, `rng` and `PAL` from
`web/index.html` so a repo has the same colour in both). The language badge uses the web's
`LANG_COLOR`, darkened the same way (`mixHex(..., '#000000', .38)`). Status colours come from
`statuses.<k>.darkColor` in the settings.

Top bar: title, a live dot, current sort (and reversed marker), status filter counts in their
colours, the search text when searching, and the agent summary ("2 working · 1 waiting").

Right panel for the highlighted repo: the name (bold, repo colour), status pill and language,
the status reasons in words, branch, agent state, whether a herdr workspace / tmux window is
open, the last 5 commits (relative time + subject), issues (open count, Done, Next, up to 5
open issues), worktrees (branch, merged/dirty), and the first lines of the repo's notes,
read-only. Panel and grid scroll independently.

The mockup the owner picked is kept at
`.superpowers/brainstorm/290352-1792820030/content/layout-v2.html` (option B), local only.

### Keys and mouse

| Key | Does |
|---|---|
| `h j k l`, arrows | Move between cards |
| `g` / `G` | First / last card |
| `ctrl-d` / `ctrl-u` | Half a page down / up |
| `Enter` | Jump (see below) |
| `n` | Create a new workspace for the repo without asking, then jump |
| `/` | Search by name (live filter; `Enter` keeps it, `Esc` clears) |
| `s` / `S` | Next sort / reverse sort |
| `f` / `F` | Next / previous status filter (all, then `topbar.filters` order) |
| `r` | Rescan now |
| `Tab` | Hide / show the right panel |
| `?` | Help overlay |
| `q`, `Esc` | Close (Esc first clears search or closes help) |

Mouse: click selects a card, double-click jumps, wheel scrolls the grid (or the panel when the
pointer is over it).

### Sorts

The four web sorts with the same rules (`recent`, `activity`, `language`, `alpha`; ties fall back
to `recent`), plus **`issues`: most open issues first**, ties by `recent`. Initial sort and filter
come from `defaults.sort`, `defaults.reverse` and `defaults.filter`.

### Jumping

- Inside herdr (`HERDR_ENV` or `HERDR_PANE_ID` set): always herdr. Enter on a repo that has a
  pane in some workspace focuses that workspace and tab (`openInHerdr`'s existing match: exact
  cwd beats a subdirectory), then the TUI exits so the popup closes. A repo with no pane gets a
  one-line prompt in the TUI, "Create a workspace for my-app? y/n"; `y` creates it
  (`workspace create --cwd --label --focus`, as `openInHerdr` does today) and exits; `n` or `Esc`
  returns to the grid.
- Outside herdr (my call; the owner did not object): Enter follows `terminal.multiplexer` exactly
  like the web terminal button (`openInTerminal`). The "create?" prompt still applies when the
  chosen multiplexer has no tab for the repo. After a jump outside herdr, the TUI stays open
  (there is no popup to close).
- Errors (no server running, herdr command fails) show as a message line; the TUI keeps running.
- The "does it have a workspace?" check must not act, so `openInHerdr` and `openInTmux` may be
  split into a find step and a focus/create step (my call; the web `/api/open` behaviour must stay
  the same). The TUI exits explicitly after a successful jump inside herdr.

### How it is started

- `repodash -tui` starts the TUI (my call: a flag, matching the existing `-json`, `-check`,
  `-print-config` style). It honours `-root` and `-config` and the usual settings path rules. It
  never opens a browser and never listens on a port.
- A `herdr-plugin.toml` at the repo root makes this repo a herdr plugin: one `[[panes]]` entry
  with `placement = "popup"`, 100% width and height, running the TUI, and one `[[actions]]`
  entry (`open`, contexts global) so it can be bound to a key. The README documents the key
  binding to add to `~/.config/herdr/config.toml` and the `herdr plugin action invoke` command;
  RepoDash does not edit herdr's config itself.

### The plugin in detail

Modelled on herdr-agent-grid's working manifest (herdr 0.9.x):

```toml
id = "repodash"
name = "RepoDash"
min_herdr_version = "0.9.0"
platforms = ["linux", "macos"]

[[build]]
platforms = ["linux", "macos"]
command = [...]          # builds ./repodash in the plugin root (see below)

[[actions]]
id = "open"
title = "Open RepoDash"
contexts = ["global"]
command = [...]          # opens the "grid" pane

[[panes]]
id = "grid"
title = "RepoDash"
placement = "popup"
width = "100%"
height = "100%"
command = ["./repodash", "-tui"]
```

herdr resolves `./` programs against the plugin root. The owner's case is
`herdr plugin link ~/gitrepos/repodash`, so the plugin root is the source checkout and the
binary is the one `go build -o repodash .` puts there; the existing settings-path rule then finds
the checkout's `repodash.json` with no extra flags (my call: build into the root, not `./bin/`,
for exactly this reason). For `herdr plugin install` from GitHub, the build hook builds with Go
when it is installed and otherwise fetches the release binary with `install.sh` into the plugin
root; that copy then uses `~/.config/repodash/repodash.json` as any installed binary does.

First thing the builder verifies with a real popup (a read-only check, with the owner's go-ahead):
which `HERDR_*` variables and cwd the pane process gets, and that the popup closes when the
process exits. If `HERDR_ENV`/`HERDR_PANE_ID` are missing in plugin panes, the pane command
passes an explicit flag instead (e.g. `-tui -mux herdr`); the design does not depend on the
environment check being the only signal.

## Data

The TUI runs in-process with the existing code; it does not need the web server running.

- Repos: `scanAll(root)` on start, then every `scan.intervalSeconds`, and on `r`.
- Agent state: refreshed every second from `terminalPanes()`, like `pollAgents`.
- Settings: `cfg.load()` on start and `cfg.poll()` while open, so edits to `repodash.json` apply
  live, as on the web. Problems are shown as a short warning in the top bar.
- Recent commits for the panel: `git log -5` for the highlighted repo, fetched when the selection
  changes, cached per repo for the scan interval.
- Notes: read from the notes folder (`notesDir`, same as the server).
- Issues: the current feed only keeps Done and Next. The TUI needs the open count (for the sort)
  and up to 5 open issues (for the panel). Extend `fetchIssues` / `repoIssues` with `Open int`
  and `OpenList []issueRef` (open count must exclude pull requests, as the web does today). The
  web page ignores the new fields. Because a popup is opened and closed often, issue results are
  cached on disk at `$XDG_CACHE_HOME/repodash/issues.json` (default `~/.cache/...`) with the
  fetch time; the TUI shows cached data at once and refetches in the background when older than
  the 5-minute interval. The web server may read and write the same cache (my call; the builder
  may keep the server as it is if that is simpler). `card.issues: false` means `gh` is never run
  by either.
- Exact open counts (my call): one `gh api graphql` query per repo returning
  `issues(states: OPEN) { totalCount }` and the 5 oldest open issues (GraphQL `issues` never
  includes pull requests, and `totalCount` is not capped by page size). Done can stay on the
  existing REST call or move into the same query, builder's choice.
- Cache writes are atomic (write a temp file, rename); several processes sharing it is
  last-writer-wins, which is fine because every writer writes complete, fresh data.
- Agent state polls both tmux and herdr, as `pollAgents` does (a repo can have an agent in either).

## Settings that apply

The TUI honours: `scan.*`, `defaults.sort` / `reverse` / `filter`, `topbar.filters`,
`statuses.*` (label, darkColor, show), `colors.accentDark`, `card.branch` / `chips` / `activity` /
`week` / `worktrees` / `agent` / `issues`, `repos.<name>.title` / `note` / `hidden`, `title`,
`terminal.multiplexer`. Card notes (`repos.<name>.note`) show on the card as on the web.

No new settings are added for the first version (my call). If one turns out to be needed, the
five-step "Adding a setting" checklist in AGENTS.md applies.

## Layout rules (builder's call within these)

- Card width about 36 columns; as many columns of cards as fit beside the panel.
- The panel is about 44 columns and hides itself when the terminal is too narrow for one card
  column plus the panel; `Tab` can still bring it up as a full-width view.
- Long names, branches and subjects are cut with `…`, never wrapped into the next card.
- Truecolor when the terminal supports it; Lip Gloss's colour downgrading otherwise.
- Always the dark palette (my call; the owner's terminals are dark). No terminal-background
  query, and `theme` does not apply to the TUI.
- Raising the `go` line in `go.mod` to what Bubble Tea needs is fine. The module rename on the
  release to-do list is separate and unaffected.

## Testing

- Unit tests (`go test ./...`): sort and filter order (including `issues` and reverse), the colour
  port matching the web page for a few fixed names (expected values computed once from the JS),
  card text building and truncation, and the herdr/tmux choice logic with fake pane lists.
- A Bubble Tea model test driving key presses (move, search, sort, filter, Enter on a repo with
  and without a workspace) against a fake jump function.
- Manual run against the demo repos (`scripts/demo-repos.sh`) with a scratch `XDG_DATA_HOME`.
- Never create workspaces or move tabs in the owner's real herdr or tmux. The tmux path is
  tested with a throwaway server (`env -u TMUX`, private `TMUX_TMPDIR`, as AGENTS.md describes).
  herdr creation/focus is exercised only through the fake jump function in tests; a real herdr
  check is done by the owner, or with their go-ahead.
- The usual gate: `CGO_ENABLED=0 go build`, `gofmt -l .`, `go vet ./...`, `go test ./...`, the
  page script parse check, and `repodash -json` / the web page still working unchanged.

## Docs to update

AGENTS.md (flags, the dependency convention, code map rows for the new files, the plugin, the
issues cache path in Quick facts) and README (a TUI section with the herdr key binding).

## Left to the builder

File layout (e.g. `tui.go`, `tui_card.go`), Bubble Tea major version (current stable), model
structure, exact colours for borders and muted text (dark-theme defaults consistent with the web
dark theme), how the help overlay looks, and the plugin build hook (how `herdr plugin install`
gets a binary: building with Go if present, or fetching the release binary via `install.sh`).
