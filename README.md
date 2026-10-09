# RepoDash

A live dashboard for every git repository in one folder. See at a glance which repos have
uncommitted work, unpushed commits, no remote, or branches that are already merged, then jump
straight to the one you want.

It is a single static Go binary with no dependencies. It scans a folder of repos, streams changes
to your browser, and serves one page. Nothing is installed in the repos and nothing in them is
ever modified: RepoDash only reads.

## What you get

- **Three views**
  - **Grid:** a card per repo, sorted by most recent activity or by name, in either direction.
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
- **Jump to a terminal**: one click selects that repo's tmux window, or opens a new one there.
  Another button copies `cd /path/to/repo`.
- **Everything is configurable** in one JSON file: the top bar, labels, colours, which parts of a
  card show, canvas and kanban layout, and per-repo titles, notes, covers and hiding. Edit the file
  and the open page updates within a second. Mistakes are reported in plain words and never break
  the page.

## Install and run

You need Go 1.22 or newer and git. tmux is optional (it powers the jump-to-terminal button).

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

It is developed and used on Windows through WSL2, which means the Linux build. Plain Linux works the
same way, and macOS should too. Native Windows (outside WSL) is untested. Under WSL2, `localhost` is
forwarded, so you can open <http://127.0.0.1:8092> in a Windows browser.

### Options

| Flag | Meaning |
|---|---|
| `-root DIR` | Folder that contains your repos (default `~/gitrepos`) |
| `-addr HOST:PORT` | Listen address (default `127.0.0.1:8092`) |
| `-allow-host a,b` | Names the page may be opened by, when it is reachable by name |
| `-config FILE` | Settings file (default: `repodash.json` next to the binary) |
| `-check` | Validate the settings file, print problems, exit 1 if there are any |
| `-json` | Scan once and print the result as JSON |
| `-print-config` | Print every setting with its default |

## Settings

The first run creates `repodash.json` next to the binary, with every setting at its default. Edit
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

If you use tmux and set a pane option named `@claude` to `busy`, `waiting` or `idle`, cards show
that state for the repos where such a pane is open. It is optional and invisible otherwise.

## Contributing

Standard library only, no JavaScript build step, one HTML file. See [AGENTS.md](AGENTS.md) for the
code map, how to add a setting, and how to test without touching your own layout.
