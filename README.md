<p align="center">
  <img src="https://github.com/user-attachments/assets/906d3625-d899-4a2a-a346-d881c93ca7e5" alt="RepoDash" width="260">
</p>

A live dashboard for every git repository in one folder, in your terminal and in your browser.
See at a glance which repos have uncommitted work, unpushed commits, open GitHub issues or
branches that are already merged, then jump straight into the one you want.

Run `repodash` and you get a grid of colour-coded cards right in the terminal, driven by vim keys
or the mouse. Inside [herdr](https://herdr.dev) it lives in its own tab: pick a repo, press Enter,
and you land in that repo's workspace. It is a single static Go binary. Nothing is installed in
the repos and nothing in them is ever modified: RepoDash only reads.

![RepoDash in the terminal: colour-coded repo cards with status, branch and last commit, and the inspector on the right](https://github.com/user-attachments/assets/170f793e-0d13-4152-958e-cbfb494bc586)

Press `w` and the same dashboard opens in your browser. The web version has everything the
terminal has, plus a free canvas where you arrange and group repos, kanban boards you drag them
between, cover images, and your own notes next to each README.

![The same repos in the browser: a card per repo with its status, branch, last commit and activity](https://github.com/user-attachments/assets/2f9d0c43-20ac-4921-bbc0-6a13411a2581)

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

## Install

```sh
curl -fsSL https://github.com/ezjones/repodash/releases/latest/download/install.sh | sh
repodash
```

Linux, macOS, or Windows through WSL2. It installs one binary in `~/.local/bin` without sudo; see
[Install and run](#install-and-run) for building from source and every option.

## Why I made this

I have a lot of git repos, and I kept losing track of where each one was at. I wanted a visual way
to see every repo's state at a glance, to move repos around on a canvas and group them the way I
think about them, and to sort them in different ways. So I built RepoDash.

## What you get

- **A terminal UI first**: `repodash` opens the cards in your terminal. Vim keys and the mouse
  both work, it adapts from a wide monitor down to a phone-width terminal, and `w` opens the web
  page when you want the bigger picture. See [Terminal UI and herdr](#terminal-ui-and-herdr).
- **herdr plugin**: one key opens RepoDash in its own herdr tab; Enter jumps to a repo's
  workspace, or offers to create one. See [herdr integration](#herdr-integration).
- **GitHub issues** on every card: the last issue you closed and the next one to do, plus an
  *issues* sort. See [GitHub issues](#github-issues).
- **Inspector**: a side panel with the selected repo's details, where commits expand, issues open
  on GitHub and worktrees jump to a terminal. See [Inspector](#inspector).
- **Three web views**
  - **Grid:** a card per repo, sorted by most recent change, git activity (commits this week), main language or name, in either direction.
  - **Canvas:** the same cards on a free canvas. Drag them anywhere, group them in labelled
    frames, pan with the middle mouse button, zoom with the wheel or a pinch.
  - **Kanban:** columns by status, or your own columns (Active, Paused, Ideas, ...) that you drag
    repos between.
- **Status at a glance**: *Needs action* (unpushed commits, unpushed branch, merge
  conflicts), *Loose ends* (uncommitted files, behind upstream, merged worktrees to clean up),
  *Clean*, and *Quiet* (clean and untouched for a while).
- **The same keys everywhere**: the web page takes the terminal UI's vim-style keys too; press `?`
  in either for the list.
- **Info and notes**: the ⓘ icon on a card opens one panel with the repo's README (rendered as
  markdown, read-only) on top and your own notes underneath. Notes are markdown and edit in place,
  Obsidian-style: click a block to edit its text, click away and it renders. There is a small toolbar,
  and `- [ ]` tasks tick with a click. Notes are saved under `~/.local/share/repodash/notes/`, never
  inside the repo.
- **Find repos with no remote**: one icon in the top bar shows only the repos that have no remote.
- **Live**: the page updates by itself while it is open, and idle costs nothing because nothing is
  scanned when no browser is watching.
- **A cover for every card**: an image you add, a logo or screenshot found in the repo, the first
  local image in its README, or generated pastel art.
- **Jump to a terminal**: one click selects that repo's tmux window or [herdr](https://herdr.dev)
  workspace, or opens a new one there, and cards show which repos already have a tab open and what a
  coding agent in it is doing ([tmux](#tmux-integration) and [herdr](#herdr-integration) integration). Another button copies `cd /path/to/repo`.
- **Everything is configurable** in one JSON file: the top bar, labels, colours, which parts of a
  card show, canvas and kanban layout, and per-repo titles, notes, covers and hiding. Edit the file
  and the open page updates within a second. Mistakes are reported in plain words and never break
  the page.

## Terminal UI and herdr

Run `repodash` in a terminal and the cards appear right there, no browser needed. Each card shows
the repo name on its own colour (the same colour as on the web page), its status, main language,
branch, what needs doing, the last commit and this week's activity. The layout fits the window:
several columns on a wide monitor, two or one on a narrow terminal or a phone.

It scans the repos in the folder you run it from, or `~/gitrepos` (see
[Install and run](#install-and-run)).

| Key | What it does |
|---|---|
| `h` `j` `k` `l` / arrows | Move between cards |
| `g` / `G`, `ctrl-d` / `ctrl-u` | First / last card, half a page down / up |
| Enter | Jump to the repo's herdr workspace or tmux window; asks before creating one |
| `n` | Create a workspace without asking |
| `/` | Search by name (Enter keeps it, Esc clears) |
| `s` / `S` | Next sort (recent, activity, language, alpha, issues) / reverse |
| `f` / `F` | Next / previous status filter |
| `i` or Tab | Show or hide the [inspector](#inspector) |
| `J` / `K`, `o` | Move over the inspector's commits, issues and worktrees, and open the one you're on |
| `w` | Open the web page in your browser, starting the web server in the background if it isn't running |
| `r` | Rescan now |
| `?` | All keys |
| `q` / Esc | Quit |

The mouse works too: click a card to select it, double-click to jump, scroll with the wheel, click
the sort name, the `↕` arrow or a status count in the top bar, and click inside the inspector.

The web page answers to the same keys (except `w`), so moving between the two is free.

## herdr integration

[herdr](https://herdr.dev) is a terminal workspace manager for AI coding agents: one workspace per
project, tabs and panes inside it, and it knows what the agent in each pane is doing. RepoDash fits
on top of it as the place you pick a project from. It talks to herdr only through the `herdr`
command, so all it needs is `herdr` (0.9 or newer) on your `PATH` and a running herdr server. Without
herdr, everything else works as before.

### The RepoDash tab

RepoDash ships as a herdr plugin (`herdr-plugin.toml` in this repo). Its one action, `repodash.open`,
opens the terminal UI in its own herdr tab, or, if that tab is already open anywhere, switches you
back to it. So the same key both opens RepoDash and brings you back to it, and you never end up
with two. It is a normal tab, not a popup: you can switch to other tabs and workspaces while it stays
open, and it keeps refreshing in the background. `q` closes it.

### Jumping to a repo

Press Enter on a card (or double-click it) and RepoDash looks through herdr's panes:

1. **A pane in the repo folder** wins: its workspace and tab are focused.
2. **A pane in a subfolder** (`my-app/src`, say) comes next, if no pane sits at the repo root.
3. **No pane at all**: RepoDash asks *Create a workspace for my-app? y/n*. `y` creates a herdr
   workspace named after the repo, in the repo folder, and focuses it. Press `n` on a card instead of
   Enter to create it without the question.

The RepoDash tab stays where it was, so the plugin key takes you straight back to pick the next one.
Worktrees work the same way: in the [inspector](#inspector), click a worktree (or `o` on it) to jump
to, or create, a workspace in that worktree's folder.

Inside herdr, Enter always uses herdr. Run outside herdr, the terminal UI follows
`terminal.multiplexer` (below), like the web page.

### Agent status on every card

RepoDash reads herdr's own agent detection for every pane, so no hook or setup is needed:

- a yellow ring when the agent in that repo is working,
- a red dot when it is waiting for you,
- a green dot when it is idle.

The terminal UI's top bar and the web page's top bar add them up ("2 working · 1 waiting"), and the
inspector says it in words, along with whether a herdr or tmux tab is open for that repo. The status
follows the agent within about a second. Turn the icons off with `card.agent: false`.

### Install the plugin

Link this folder as a plugin. herdr builds `./repodash` with Go if you have it, or downloads the
release binary otherwise:

```sh
herdr plugin link /path/to/repodash
herdr plugin action invoke repodash.open
```

To bind a key, add this to `~/.config/herdr/config.toml` (pick any free key), then run
`herdr server reload-config`:

```toml
[[keys.command]]
key = ["prefix+r"]
type = "plugin_action"
command = "repodash.open"
description = "repodash"
```

After you update RepoDash, press `q` in its tab and open it again to run the new version. If the key
does nothing, `herdr plugin list` shows whether the plugin is enabled, and `herdr plugin log list`
shows the last runs of its action with their errors.

### herdr and tmux together

In the web page, a card's terminal button does the same jump-or-create as Enter, and cards show
"tab open" for repos that already have a herdr pane. One button serves both herdr and tmux. Pick the
order with `terminal.multiplexer` in `repodash.json`:

- `prefer-tmux` (the default) or `prefer-herdr`: an existing tab wins, in the preferred one first;
  with no tab anywhere, a new one opens in the preferred one if its server is running, else the other.
- `tmux` or `herdr`: only that one.

Each browser can override this at the bottom of the gear (Settings) panel. Like tmux, RepoDash
selects the tab but cannot raise the terminal application itself, and it must run as the same user
as the herdr server.

## GitHub issues

For every repo whose `origin` is on github.com, RepoDash shows its GitHub issues:

- **On each card:** *Done*, the issue you closed most recently and how long ago, and *Next*, the
  oldest open issue. On the web page both link to GitHub.
- **In the inspector:** the number of open issues, the last one done and up to five open ones.
  Click one (or `o` on it) to open it on GitHub.
- **The *issues* sort** in the terminal UI puts the repos with the most open issues first.

The data comes from the [`gh`](https://cli.github.com) command line tool, using its own login, so
RepoDash stores no token. It never creates or edits issues, and pull requests are not counted. It
refreshes every few minutes, separately from the git scan so that stays fast, and the terminal UI
keeps a small private cache so issues show the moment it opens. If `gh` is missing or logged out,
the web page's gear button shows a red dot with the reason. Turn it off with `card.issues: false`
in the settings file, and `gh` is never run.

## Inspector

The inspector is a side panel with everything about the selected repo: its status and why,
branch, the coding agent's state, whether a tab is open, the last five commits, its GitHub issues,
its worktrees and the first lines of your notes.

- **Terminal UI:** `i` (or Tab) shows or hides it. It sits beside the cards when there is room and
  fills the screen on a narrow terminal.
- **Web page:** `i` or the panel button in the top bar, then click a card or move with `hjkl`. On a
  phone it fills the screen.

Nothing in it is just text:

- **Commits** expand in place to the full message, author, date and changed files, with a link to
  the commit on GitHub. Click again to fold it.
- **Issues** open on GitHub.
- **Worktrees** jump to a terminal in that worktree's folder, like Enter does for the repo.

`J` and `K` move over these lines and `o` opens the one you're on, so it all works from the
keyboard too. On the web page, *Open README & notes* opens the full README and your notes.

## tmux integration

If you keep one tmux window per project, RepoDash turns each card into a switchboard for them. It
is entirely optional: with no tmux server running, nothing changes except that the terminal button
reports "no tmux server running".

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

You need git. tmux or herdr is optional (they power the jump-to-terminal button).

The quickest way is the install script. It picks the right build for your OS and CPU (Linux, macOS,
or Windows through WSL2; amd64 or arm64), checks its checksum, and puts `repodash` in
`~/.local/bin` without needing sudo:

```sh
curl -fsSL https://github.com/ezjones/repodash/releases/latest/download/install.sh | sh
repodash
```

Set `VERSION=v0.1.0` to pin a release or `PREFIX=/some/dir` to install somewhere else. Installed
this way, your settings live in `~/.config/repodash/repodash.json`.

If `repodash` is not found afterwards, `~/.local/bin` is not on your `PATH`. Add this line to
`~/.bashrc` or `~/.zshrc` and open a new terminal:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

To update, run the same install command again. To check what you have, run `repodash -version`.
To uninstall, delete the binary and, if you want to drop your settings too, the config folder:

```sh
rm ~/.local/bin/repodash
rm -r ~/.config/repodash
```

To build from source instead you need Go 1.26 or newer:

```sh
git clone https://github.com/ezjones/repodash.git
cd repodash
CGO_ENABLED=0 go build -o repodash .
./repodash
```

Run from a terminal, `repodash` opens the [terminal UI](#terminal-ui-and-herdr). Press `w` there
for the web page: it starts the web server in the background if it isn't running and opens
<http://127.0.0.1:8092> in your browser (on WSL2, your Windows browser). The server keeps running
after you quit the terminal UI.

To start only the web server, run `repodash -web`; it opens the page in your browser when started
from a terminal (pass `-no-open` to stop that). Started without a terminal (`nohup`, systemd, a
login script), plain `repodash` is the web server too.

It scans the repos directly inside the folder you run it from, when that folder has any (your
home folder itself never counts); otherwise it scans `~/gitrepos`. Point it somewhere else with
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
| `-root DIR` | Folder that contains your repos (default: the current folder if it holds repos, else `~/gitrepos`) |
| `-web` | Web server instead of the terminal UI (also implied by `-addr`, `-allow-host`, `-no-open`, or no terminal) |
| `-tui` | Terminal UI (the default from a terminal) |
| `-addr HOST:PORT` | Listen address (default `127.0.0.1:8092`) |
| `-allow-host a,b` | Names the page may be opened by, when it is reachable by name |
| `-config FILE` | Settings file (default: `repodash.json` next to the binary in a source checkout, otherwise `~/.config/repodash/repodash.json`) |
| `-check` | Validate the settings file, print problems, exit 1 if there are any |
| `-json` | Scan once and print the result as JSON |
| `-no-open` | Do not open the page in a browser on start (it only opens when run from a terminal) |
| `-version` | Print the version and exit |
| `-print-config` | Print every setting with its default |

## Settings

The first run creates `repodash.json` (next to the binary in a source checkout, otherwise in `~/.config/repodash/`), as a nearly empty file. Put only the settings you want to change in it, and everything else follows the built-in defaults (`repodash -print-config` lists them all), so new releases add their features without you editing anything. Edit
it while RepoDash is running. `./repodash -check` validates the file and explains each problem.

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
The terminal UI scans in-process; the web server pushes results to the browser over server-sent
events. Canvas positions, groups, kanban
columns and covers you add are saved outside your repos, in `~/.local/share/repodash/`.

RepoDash never runs `git fetch`, so "behind" is only as fresh as each repo's last fetch.

## Contributing

Standard library only, no JavaScript build step, one HTML file. See [AGENTS.md](AGENTS.md) for the
code map, how to add a setting, and how to test without touching your own layout.
