# RepoDash

A live dashboard for every git repository in one folder. See at a glance which repos have
uncommitted work, unpushed commits, no remote, or branches that are already merged, then jump
straight to the one you want.

It is a single static Go binary with no dependencies. It scans a folder of repos, streams changes
to your browser, and serves one page. Nothing is installed in the repos and nothing in them is
ever modified: RepoDash only reads.

![RepoDash grid view: a card per repo with its status, branch, last commit and activity](https://github.com/user-attachments/assets/2f9d0c43-20ac-4921-bbc0-6a13411a2581)

## Why I made this

I have a lot of git repos, and I kept losing track of where each one was at. I wanted a visual way
to see every repo's state at a glance, to move repos around on a canvas and group them the way I
think about them, and to sort them in different ways. So I built RepoDash.

## What you get

- **Three views**
  - **Grid:** a card per repo, sorted by most recent change, git activity (commits this week), main language or name, in either direction.
  - **Canvas:** the same cards on a free canvas. Drag them anywhere, group them in labelled
    frames, pan with the middle mouse button, zoom with the wheel or a pinch.
  - **Kanban:** columns by status, or your own columns (Active, Paused, Ideas, ...) that you drag
    repos between.
- **Status at a glance**: *Needs action* (no remote, unpushed commits, unpushed branch, merge
  conflicts), *Loose ends* (uncommitted files, behind upstream, merged worktrees to clean up),
  *Clean*, and *Quiet* (clean and untouched for a while).
- **Live**: the page updates by itself while it is open, and idle costs nothing because nothing is
  scanned when no browser is watching.
- **A cover for every card**: an image you add, a logo or screenshot found in the repo, the first
  local image in its README, or generated pastel art.
- **Jump to a terminal**: one click selects that repo's tmux window, or opens a new one there, and
  cards show which repos already have a tab open and what a coding agent in it is doing
  ([tmux integration](#tmux-integration)). Another button copies `cd /path/to/repo`.
- **Everything is configurable** in one JSON file: the top bar, labels, colours, which parts of a
  card show, canvas and kanban layout, and per-repo titles, notes, covers and hiding. Edit the file
  and the open page updates within a second. Mistakes are reported in plain words and never break
  the page.

## Screenshots

### Grid

Every repo as a card, sorted by most recent change, git activity, main language or name. The
status buttons at the top filter to the repos that need your attention: unpushed commits, no
remote, merge conflicts, uncommitted work, or branches that are behind.

![Grid filtered to the repos that need action](https://github.com/user-attachments/assets/ef3a8b46-0e6c-40e0-84b2-330a6072e6ba)

### Canvas

A free-form board for your repos. Drag cards anywhere, right-click empty space to add a named
group, and cards inside a group move with it. Drag on the background to lasso several cards at
once, hold the middle mouse button to pan, and scroll or pinch to zoom. *Arrange* tidies loose
cards into a grid in your current sort order. Positions and groups are saved outside your repos.

![Canvas view with repos arranged in named groups](https://github.com/user-attachments/assets/fa707482-c11d-4e23-aad4-4c67b25213d5)

### Kanban

Make as many boards as you like and name the columns whatever suits the work: Todo, Doing and
Done, or something else entirely. Add, rename, move and delete columns, then drag repos between
them. Switch to *By status* for automatic columns that follow each repo's state.

![Kanban view with a board of Todo, In progress and Done columns](https://github.com/user-attachments/assets/047fafa1-7c4a-47e1-afdd-e440a02f5cae)

## tmux integration

If you keep one tmux window per project, RepoDash turns each card into a switchboard for them. It
is entirely optional: with no tmux server running, nothing changes except that the terminal button
reports "no tmux server running".

![A card marked "tab open" in the dashboard, next to the tmux window it jumps to](https://github.com/user-attachments/assets/e9f908b6-f97f-4f13-bd1a-b619c976f7cb)

### The terminal button

Every card has a terminal icon next to the copy button. Clicking it does one of three things, in
this order:

1. **Jump to the tab you already have.** If a pane's current directory is exactly the repo folder,
   that window is selected.
2. **Jump to a tab inside it.** If no pane sits at the root but one sits in a subfolder
   (`my-app/src`, say), that window is selected. An exact match always wins over a subfolder.
3. **Open a new one.** Otherwise a new window is created in the repo folder, in the tmux session
   you used most recently.

So you never end up with two tabs for the same project, and clicking a card is the same as picking
the project from a tmux project switcher. The page shows what happened ("jumped to window 3" or
"opened window 5").

RepoDash switches whichever tmux client is attached, so run it as the same user and on the same
machine as your tmux server. It does not matter whether RepoDash itself was started inside tmux.
It only ever acts on folders directly under `-root` that are git repos, never on an arbitrary path.

It selects the window; it cannot raise the terminal application. If your terminal is hidden behind
a browser, bring it forward yourself. This applies to WSL2 too: the button switches the tab in your
WSL tmux session, but the Windows terminal window stays where it is.

### Tab and agent badges

Cards read your panes every time the repos are scanned:

- A small **tab open** badge appears on any repo that has a pane in it (at the root or below).
- If that pane has a tmux pane option called `@claude`, the badge shows its value instead:
  `busy` (working), `waiting` (needs you) or `idle`. When several panes are in one repo, `waiting`
  beats `busy`, which beats `idle`.

RepoDash only reads `@claude`; whatever runs your agent sets it. With Claude Code, hooks do this
well. Add to `~/.claude/settings.json`:

```json
{
  "hooks": {
    "UserPromptSubmit": [{ "hooks": [{ "type": "command", "command": "[ -n \"$TMUX\" ] && tmux set -p @claude busy || true" }] }],
    "Notification":     [{ "hooks": [{ "type": "command", "command": "[ -n \"$TMUX\" ] && tmux set -p @claude waiting || true" }] }],
    "Stop":             [{ "hooks": [{ "type": "command", "command": "[ -n \"$TMUX\" ] && tmux set -p @claude idle || true" }] }]
  }
}
```

You can set it by hand to try it: `tmux set -p @claude waiting` in a pane inside a repo, and the card
changes within a scan interval. Any other tool works the same way, since it is only a pane option.
Pane options vanish with the pane, so a closed tab never leaves a stale badge behind.

### Turning it off or trimming it

- Remove the terminal button: `"card": { "actions": ["image", "copy"] }`.
- Hide the badges: `"card": { "agent": false }`.

## Install and run

You need git. tmux is optional (it powers the jump-to-terminal button).

The quickest way is the install script. It picks the right build for your OS and CPU (Linux, macOS,
or Windows through WSL2; amd64 or arm64), checks its checksum, and puts `repodash` in
`~/.local/bin` without needing sudo:

```sh
curl -fsSL https://github.com/ezjones/repodash/releases/latest/download/install.sh | sh
repodash
```

Set `VERSION=v0.1.0` to pin a release or `PREFIX=/some/dir` to install somewhere else. Installed
this way, your settings live in `~/.config/repodash/repodash.json`.

To build from source instead you need Go 1.22 or newer:

```sh
git clone https://github.com/ezjones/repodash.git
cd repodash
CGO_ENABLED=0 go build -o repodash .
./repodash
```

Then open <http://127.0.0.1:8092>.

By default it scans the folders directly inside `~/gitrepos`. Point it somewhere else with
`-root`:

```sh
./repodash -root ~/code
```

(If you installed with the script, run `repodash` rather than `./repodash`.)

It is developed and used on Windows through WSL2, which means the Linux build. Plain Linux works the
same way, and macOS should too. Native Windows (outside WSL) is untested. Under WSL2, `localhost` is
forwarded, so you can open <http://127.0.0.1:8092> in a Windows browser. See
[tmux integration](#tmux-integration) for what the terminal button can and cannot do from there.

### Options

| Flag | Meaning |
|---|---|
| `-root DIR` | Folder that contains your repos (default `~/gitrepos`) |
| `-addr HOST:PORT` | Listen address (default `127.0.0.1:8092`) |
| `-allow-host a,b` | Names the page may be opened by, when it is reachable by name |
| `-config FILE` | Settings file (default: `repodash.json` next to the binary in a source checkout, otherwise `~/.config/repodash/repodash.json`) |
| `-check` | Validate the settings file, print problems, exit 1 if there are any |
| `-json` | Scan once and print the result as JSON |
| `-version` | Print the version and exit |
| `-print-config` | Print every setting with its default |

## Settings

The first run creates `repodash.json` (next to the binary in a source checkout, otherwise in `~/.config/repodash/`), with every setting at its default. Edit
it while RepoDash is running. Every key is optional, so you can delete anything you do not want to
change. `./repodash -check` validates the file and explains each problem.

```json
{
  "title": "My repos",
  "defaults": { "view": "kanban", "kanbanMode": "board" },
  "kanban": { "columns": ["Now", "Next", "Later"] },
  "repos": {
    "my-app": { "title": "My App", "note": "the web frontend" },
    "scratch": { "hidden": true }
  }
}
```

All settings are listed in [AGENTS.md](AGENTS.md), which is written for people and coding agents
alike: it covers every key, the HTTP API, how the code is laid out, and how to change it.

## Using it from another device

By default RepoDash only listens on `127.0.0.1`. To open it from your phone or another computer
on a network you trust (a VPN or tailnet, say):

```sh
./repodash -addr :8092 -allow-host my-laptop,my-laptop.example.ts.net
```

`-allow-host` lists the names you will type in the browser's address bar. It exists to defend
against DNS-rebinding attacks, so requests addressed to an unknown name are refused. IP addresses
are always accepted.

RepoDash has no login. Do not expose it to the internet.

## How it works

It runs read-only git commands (`status`, `log`, `worktree`, `stash`, ...) in each repo, with
optional locks and repo-configured programs disabled so a scan never gets in the way of your work.
Results are pushed to the browser over server-sent events. Canvas positions, groups, kanban
columns and covers you add are saved outside your repos, in `~/.local/share/repodash/`.

RepoDash never runs `git fetch`, so "behind" is only as fresh as each repo's last fetch.

## Contributing

Standard library only, no JavaScript build step, one HTML file. See [AGENTS.md](AGENTS.md) for the
code map, how to add a setting, and how to test without touching your own layout.
