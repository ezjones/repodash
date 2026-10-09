# RepoDash

A live dashboard of every git repo under `~/gitrepos`: status cards in a grid, a free canvas
you can arrange and group, and a kanban board. One static Go binary (standard library only)
serving one embedded HTML page. Everything the page shows is configurable in a JSON file that
people and agents can edit while it runs.

This file is for any agent working with the app. It covers how to **use** it, how to **change
its settings**, and how to **change the code**.

> **This will be a public project. Never commit real repo names, folder names, host names,
> tailnet names, paths under a home directory, or anything else about the owner's machines.**
> That covers code, comments, docs, examples, `repodash.json` and commit messages. Use placeholders
> (`my-app`, `myhost.example.ts.net`). Personal arrangement lives outside the repo, in
> `~/.local/share/repodash/` (layout, covers). Before committing, grep the diff for names from
> `ls ~/gitrepos`.

## Quick facts

| Thing | Where |
|---|---|
| Source | `~/gitrepos/repodash` (module `repodash`, Go 1.22+, no dependencies) |
| Run it | `./repodash` then open <http://127.0.0.1:8092> |
| Settings file | `repodash.json` **in this repo**, next to the binary (created on first run, tracked in git; an installed binary uses `~/.config/repodash/repodash.json` instead). Override with `-config FILE` or `$REPODASH_CONFIG` |
| Card positions, groups, kanban boards and columns | `~/.local/share/repodash/layout.json` (`$XDG_DATA_HOME` respected) |
| Images added in the UI | `~/.local/share/repodash/images/<repo>.<ext>` |
| Log when started in the background | `~/.local/share/repodash/repodash.log` |
| Defaults, embedded in the binary | `default-config.json` (`repodash -print-config` prints it) |

Flags: `-root DIR` (repos folder, default `~/gitrepos`), `-addr HOST:PORT` (default `127.0.0.1:8092`; use `:8092` to
reach it from other devices, only on a trusted network such as the tailnet), `-allow-host a,b`
(host names the page may be opened by, see Security notes), `-config FILE`, `-json` (scan once,
print JSON, exit), `-check` (validate settings, exit 1 on problems), `-print-config`.

## Changing settings (the common job)

1. Edit `~/gitrepos/repodash/repodash.json` (the file in this repo). It is plain JSON. Every key is optional; delete a key and
   its default is used. Keys starting with `_` are ignored (use them for notes to yourself).
2. Run `repodash -check`. It prints each problem in words and exits 1 if there are any.
3. Done. The running server notices the change within about a second and every open browser
   updates without a reload. There is no restart and no build step for settings.

A mistake never breaks the page:

- A wrong type, an unknown word or an out-of-range number is reported and **that one setting**
  falls back to its default. The rest of the file still applies.
- A file that is not valid JSON (for example mid-edit) keeps the **last good settings** and
  reports the line and column. Fix the file and it applies.
- Problems show as a red dot on the gear button in the top bar, with the list inside, and in
  `curl -s localhost:8092/api/config | jq .warnings`.

### Settings reference

All keys, with their defaults. Lists replace the default list entirely.

| Key | Type | Default | Meaning |
|---|---|---|---|
| `title` | string | `"RepoDash"` | Heading and browser tab title |
| `theme` | `auto` `light` `dark` | `auto` | `auto` follows the system |
| `scan.skip` | list of folder names | `[]` | Folders under the root to ignore |
| `scan.intervalSeconds` | 2-3600 | `15` | Rescan interval while a browser tab is open. Nothing is scanned when nobody is watching |
| `scan.staleDays` | 1-3650 | `30` | A clean repo with no commit for this long is "Quiet" |
| `defaults.view` | `auto` `grid` `canvas` `kanban` | `auto` | `auto` = canvas on wide screens, grid on phones |
| `defaults.kanbanMode` | `status` `board` | `board` | Kanban opens on your own boards, or on automatic columns by status |
| `defaults.sort` | `recent` `activity` `language` `alpha` | `recent` | Grid sort order. `recent` newest commit or edit first; `activity` most commits in the last 7 days first; `language` grouped by main language A-Z (no language last); `alpha` by name. Ties fall back to `recent`. Canvas *Arrange* and Kanban follow the same order |
| `defaults.reverse` | true/false | `false` | Reverse the sort |
| `defaults.filter` | `all` `bad` `warn` `ok` `stale` | `all` | Status filter on load |
| `defaults.remember` | true/false | `true` | Browsers remember their last view/sort. When `defaults.*` changes, the new defaults win once |
| `topbar.items` | list | see below | Which controls appear, in this order |
| `topbar.filters` | list of `bad` `warn` `ok` `stale` | all four | Which status buttons and kanban status columns, in this order |
| `labels.*` | strings | see file | Button text: `viewGrid` `viewCanvas` `viewKanban` `kanbanStatus` `kanbanBoard` `sortRecent` `sortActivity` `sortLanguage` `sortAlpha` `all` `search` `addGroup` `arrange` |
| `statuses.<bad\|warn\|ok\|stale>.label` | string | Needs action / Loose ends / Clean / Quiet | Name shown on pills, buttons, columns |
| `statuses.<k>.color`, `.darkColor` | hex `#rgb` or `#rrggbb` | see file | Light and dark theme colour. Text on filled buttons is chosen for contrast automatically |
| `statuses.<k>.show` | true/false | `true` | Show or hide that status button in the top bar |
| `colors.accent`, `colors.accentDark` | hex | `#1c1e23` / `#e5e7ed` | Selected-button colour, light and dark |
| `card.cover` | true/false | `true` | Cover image/art on cards. If off, the status pill moves into the header |
| `card.coverHeight` | 40-300 | `98` | Cover height in px |
| `card.coverStyle` | `aurora` `deep` `gradient` `name` | `name` | Generated cover for repos with no image. `aurora` glowing colour on near-black, `deep` dark three-stop gradient with grain, `gradient` soft pastel blobs, `name` the repo name big on a flat colour (fixed size, long names are cut off). Drawn from the repo name, so a repo always gets the same art. Per repo: `repos.<name>.coverStyle`. The top bar can override this and the badge for one browser |
| `card.languageBadge` | true/false | `true` | Small badge in the cover's lower-right corner with the repo's main language (by tracked file count; hidden when none is recognised) |
| `card.branch` `note` `chips` `activity` `week` `worktrees` `agent` | true/false | `true` | Show or hide each card section |
| `card.actions` | list of `image` `terminal` `copy` | all three | Which buttons a card has. `terminal` and `copy` are header icons, in this order. `image` sits in the top-right corner of the cover (shown on hover; always visible on touch), or in the header if `card.cover` is off |
| `canvas.columns` | 1-12 | `4` | Columns when arranging cards |
| `canvas.cardWidth` | 200-600 | `280` | Card width in px (also kanban column width) |
| `canvas.gap` | 0-200 | `30` | Space between cards when arranging |
| `kanban.columns` | list of names | `["Todo","Doing","Done"]` | Columns a **new** board starts with. Each board then has its own columns, which you add, rename, move and delete in the page (they are saved in `layout.json`, not here). Repos with no column sit in Unsorted |
| `kanban.unsortedLabel` | string | `"Unsorted"` | Name of that first column |
| `covers.names` | list of file base names | logo, icon, cover, banner, hero, screenshot, preview, thumbnail, og, social | Cover candidates (`logo.png`, `logo-navbar.png`, ...) |
| `covers.dirs` | list of folders | assets, docs, public, static, images, img, .github, resources | Folders searched after the repo root |
| `covers.readme` | true/false | `true` | Use the first local image the README references |
| `repos.<name>.title` | string | folder name | Display name |
| `repos.<name>.note` | string | none | One-line note on the card |
| `repos.<name>.image` | path inside the repo | none | Cover for that repo, relative to the repo folder |
| `repos.<name>.hidden` | true/false | `false` | Leave the repo out entirely |

`topbar.items` default: `title live view sort reverse kanbanMode filters search spacer addGroup
arrange appearance settings`. Allowed values are exactly those names plus `cover` and `theme`. Some only show in one view: `sort` and
`reverse` in Grid, `kanbanMode` in Kanban, `addGroup` and `arrange` in Canvas. `appearance` is a palette button that opens
a popover with the theme (auto, light, dark), the cover style (with previews) and the language-badge toggle. These save per
browser and override `theme`, `card.coverStyle` and `card.languageBadge` in the file until the browser's choice is cleared.
(`cover` and `theme` are the older separate controls: a cover-style select with a badge button, and a theme cycle button.)
On narrower windows the bar adapts by itself: sort buttons become a dropdown below 1640px, status buttons lose their
text below 1320px, view buttons below 1100px, and below 700px the status buttons become a dropdown, the search box shrinks to fit and Canvas is shown as the one-column Grid (your saved choice returns when the window widens).

The settings file is found next to the executable, so run the binary built in this folder (`go build`, then `./repodash`). `go run` builds into a temporary folder and would look for it there; use `-config` or `$REPODASH_CONFIG` in that case.

Cover order: image added in the UI, then `repos.<name>.image`, then a logo-like file in the repo
root, then the same names in `covers.dirs`, then the first README image (local files only, never
http), then generated art (`card.coverStyle`). Images over 4 MB and anything outside the repo are ignored.

### Recipes

```jsonc
// Rename a repo, add a note, hide another
"repos": {
  "my-app":     { "title": "My App", "note": "the web frontend" },
  "old-thing":  { "hidden": true }
}

// A simpler top bar: no search, no reverse, no gear
"topbar": { "items": ["title", "live", "view", "sort", "filters"] }

// Different kanban columns
"kanban": { "columns": ["Now", "Next", "Later"], "unsortedLabel": "Inbox" }

// Always open in Kanban, columns by status
"defaults": { "view": "kanban", "kanbanMode": "status" }

// Calmer card: just the cover, branch and status chips
"card": { "week": false, "activity": false, "worktrees": false, "actions": ["terminal"] }
```

(The file itself must be strict JSON: no comments, no trailing commas.)

## What the statuses mean

Computed in `scan.go` (`classify`); only `scan.staleDays` is configurable.

- **Needs action** (`bad`): no remote, unpushed commits, a branch that was never pushed, or merge conflicts.
- **Loose ends** (`warn`): uncommitted files, commits behind upstream, or worktrees whose branch is already merged.
- **Clean** (`ok`): none of the above.
- **Quiet** (`stale`): clean, and the last commit is older than `scan.staleDays`.

"Behind" is only as fresh as each repo's last `git fetch`; RepoDash never fetches. "Edited" time
comes from the modification times of uncommitted files.

## Using the app

- **Views** (top bar): Grid (sortable list), Canvas (free arrangement), Kanban (columns).
- **Canvas**: drag a card by any part of it; drag the background or hold the **middle mouse
  button** anywhere to pan; scroll or pinch to zoom. **Right-click** empty canvas (long-press on
  touch) for *New group here*, *Arrange cards*, *Reset view*. A group frame moves the cards whose
  centre is inside it. Rename a group by double-clicking its title bar. Positions are saved in
  `layout.json`.
- **Kanban**: *Boards* (the default) are yours: tabs across the top switch boards, **+ Board** adds one, and
  clicking the active tab opens rename / move / delete. In a board, **+ Column** adds a column, double-click a
  title to rename it, the **⋯** button (or right-click the title) moves or deletes it, and you drag a repo card
  to any column. New boards start with the columns in `kanban.columns` (Todo, Doing, Done). Everything is saved
  in `layout.json` under `boards` and `activeBoard`. *By status* is the automatic read-only alternative.
- **Card icons**: picture icon sets a cover (or drop an image file on the card); terminal icon
  jumps to that repo's tmux tab, or opens one; copy icon copies `cd <path>`.
- **Gear button**: shows the settings file path and any problems with it.

## HTTP API

Handy for scripts and tests. All local; see the security notes before exposing the port.

| Route | Purpose |
|---|---|
| `GET /api/repos` | Latest scan as JSON (same as `repodash -json`) |
| `GET /api/config` | `{config, warnings, path}`: merged, validated settings |
| `POST /api/refresh` | Rescan now |
| `GET /events` | Server-sent events: `config` and `repos` (each is the full JSON) |
| `GET /api/layout`, `PUT /api/layout` | Canvas/kanban state. PUT needs `Content-Type: application/json` |
| `POST /api/open` | `{"name": "<repo>"}`: select or open that repo's tmux window |
| `GET /img/<repo>` | The repo's cover image |
| `PUT /api/image/<repo>`, `DELETE /api/image/<repo>` | Set (body = image bytes, `Content-Type: image/*`, max 4 MB) or remove a custom cover |

## Code map

| File | Job |
|---|---|
| `main.go` | Flags, routes, the hub that rescans and pushes SSE, the layout store |
| `config.go` | Loads, merges, validates and live-reloads `repodash.json`; global `cfg` |
| `scan.go` | Runs git per repo (`git status --porcelain=v2`, `log`, `worktree`...), reads tmux panes, classifies status |
| `image.go` | Cover discovery and the upload/serve handlers |
| `tmux.go` | "Open in tmux" |
| `default-config.json` | Every setting and its default; embedded with `go:embed` |
| `web/index.html` | The entire UI (HTML, CSS, vanilla JS), embedded with `go:embed`. No framework, no build step |

## Changing the code

Build: `CGO_ENABLED=0 go build -o repodash .` Then `gofmt -l .` and `go vet ./...` must print nothing.
`web/` is embedded, so **any change to `index.html` needs a rebuild and a restart** to show up.
Check the page script parses: `node -e "const s=require('fs').readFileSync('web/index.html','utf8'); new Function(s.match(/<script>([\s\S]*)<\/script>/)[1])"`.

**Adding a setting** (do all five or it will drift):

1. Add the key with its default to `default-config.json`.
2. If it has a restricted set of values or a range, validate it in `validate()` in `config.go`.
   Unknown keys and wrong types are already rejected by `merge()`.
3. If the Go side needs it, read it in `parseSettings()` into the `settings` struct.
4. Use it in `web/index.html` through `C('path.to.key', fallback)`. Settings reach the page as one
   merged object; `applyConfig()` re-renders everything when they change.
5. Add a row to the reference table above.

Conventions: standard library only, `CGO_ENABLED=0`, no JS dependencies. Git is always run with
`GIT_OPTIONAL_LOCKS=0` so a scan never blocks someone's `git commit`. The app is read-only toward
the repos: it never fetches, checks out, or writes inside a repo.

### Restarting the real instance

```sh
# find it by its exact command line; never use `pkill -f`: the pattern also matches your own shell
for p in $(pgrep -x repodash); do [ "$(tr '\0' ' ' </proc/$p/cmdline)" = "./repodash " ] && kill $p; done
cd ~/gitrepos/repodash && (nohup ./repodash > ~/.local/share/repodash/repodash.log 2>&1 &)
```

### Testing without disturbing the real one

The real instance's `layout.json` is the user's hand-arranged work. **Do not drag, group or
assign cards in a browser pointed at the real port.** Use a scratch instance:

```sh
S=$(mktemp -d)
XDG_DATA_HOME=$S ./repodash -addr 127.0.0.1:8097 -config $S/repodash.json &   # own layout, images and settings
```

then drive `http://127.0.0.1:8097` (Playwright works well; a pty-attached `tmux` client works for
the tmux button). To test "Open in tmux" use a throwaway tmux server (`env -u TMUX TMUX_TMPDIR=$S/tmux tmux
new-session -d ...` and start that instance with the same `TMUX_TMPDIR`) so no real tab moves.
**Always prefix `env -u TMUX`**: inside a tmux pane `$TMUX` wins over `TMUX_TMPDIR`, so a plain
`tmux` command would hit your real server. Never run `tmux kill-server` here; end the demo with
`tmux kill-session -t <name>` using the same prefix.

Useful checks: `repodash -check -config FILE`; `curl -s localhost:8097/api/config | jq .warnings`;
break the settings file on purpose (invalid JSON, a bad enum) and confirm the page keeps working.

## Security notes

- The default bind is `127.0.0.1`. The API has no login; anyone who can reach the port can read
  repo names/status and open tmux windows. Expose it (`-addr :8092`) only on a trusted network.
- **Host check (DNS rebinding).** A hostile web page can make a browser resolve its own name to
  127.0.0.1 and then call this server as if same-origin. So requests are refused (403) unless the
  `Host` header is `localhost`, an IP address, the host part of `-addr`, or listed in
  `-allow-host`. Opening it by a name, for example a Tailscale name, therefore needs
  `./repodash -addr :8092 -allow-host myhost,myhost.example.ts.net`. Write requests that carry an
  `Origin` header must also match `Host`.
- Git is run with `core.fsmonitor=false`: a repo's own `.git/config` can name programs git runs on
  `git status`, and not every folder under the root is a repo we created.
- State-changing routes require a JSON or `image/*` content type. A web page on another origin
  cannot send those without a CORS preflight, which the server never answers.
- Repo names are validated (`validRepo`): a plain folder under the root containing `.git`. Cover
  paths are resolved through symlinks and must stay inside the repo.

## Not built yet

A command-line table (`repodash -json` is the data source for it), authentication, ordering of
cards within a kanban column, generated thumbnails (covers load at full size, capped at 4 MB),
and a real-device check of touch long-press on iOS.

## Releases and install

- `git tag vX.Y.Z && git push --tags` triggers `.github/workflows/release.yml`: it builds linux/darwin x amd64/arm64 (`CGO_ENABLED=0`, version via `-X main.buildVersion`), and publishes tarballs, `checksums.txt` and `install.sh` as release assets.
- `install.sh` (repo root) is what `curl ... | sh` runs. Test it offline with `BASE_URL=file:///dir PREFIX=/tmp/bin sh install.sh` against a dir holding the tarball and `checksums.txt`.
