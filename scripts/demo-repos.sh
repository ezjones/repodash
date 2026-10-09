#!/usr/bin/env bash
# Builds ~20 throwaway git repos in different states (clean, dirty, unpushed, behind,
# diverged, conflicted, stale, stashed, worktrees...) for README screenshots, so no real
# project shows up. Dates are relative to now, so rerun it before a screenshot session.
#
#   scripts/demo-repos.sh [dir]        default dir: ~/demo-repos (wiped and rebuilt)
#
# Then point a separate instance at it, with its own settings and layout so the real
# ones are untouched:
#   XDG_DATA_HOME=/tmp/demo-data REPODASH_CONFIG=/tmp/demo-data/repodash.json \
#     ./repodash -root ~/demo-repos -addr 127.0.0.1:8093
set -euo pipefail

ROOT="${1:-$HOME/demo-repos}"
case "$ROOT" in /|"$HOME"|"$HOME/") echo "refusing to wipe $ROOT" >&2; exit 1;; esac
rm -rf "$ROOT"
mkdir -p "$ROOT/.remotes"
cd "$ROOT"

export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null
export GIT_AUTHOR_NAME="Demo Dev" GIT_AUTHOR_EMAIL="demo@example.com"
export GIT_COMMITTER_NAME="Demo Dev" GIT_COMMITTER_EMAIL="demo@example.com"

MSGS=("Initial commit" "Add config loading" "Fix off-by-one in parser" "Refactor handlers"
      "Add retry with backoff" "Update README" "Handle empty input" "Bump dependencies"
      "Add tests for edge cases" "Improve error messages" "Cache lookups" "Tidy up logging"
      "Add --verbose flag" "Fix race on shutdown" "Speed up startup")

# at <hours-ago>: pins both git dates to that moment.
at() { local d; d=$(date -d "$1 hours ago" '+%Y-%m-%dT%H:%M:%S'); export GIT_AUTHOR_DATE="$d" GIT_COMMITTER_DATE="$d"; }

# commit <repo> <hours-ago> <message> [file]: appends a line to a file and commits it.
commit() {
  local repo=$1 h=$2 msg=$3 f=${4:-}
  [ -n "$f" ] || f=$(git -C "$repo" ls-files | grep -v README | head -1)
  at "$h"
  echo "// $msg ($RANDOM)" >> "$repo/$f"
  git -C "$repo" add -A
  git -C "$repo" commit -q -m "$msg"
}

# seed <name> <ext> <first-days-ago> <last-hours-ago> <commits> [remote=1]
seed() {
  local name=$1 ext=$2 first=$3 last=$4 n=$5 remote=${6:-1}
  git init -q -b main "$name"
  printf '# %s\n\nA demo project.\n' "$name" > "$name/README.md"
  printf 'package main\n' > "$name/main.$ext"
  printf 'package util\n' > "$name/util.$ext"
  printf 'package core\n' > "$name/core.$ext"
  local span=$(( first * 24 - last )) i h
  for ((i = 0; i < n; i++)); do
    h=$(( first * 24 - span * i / (n > 1 ? n - 1 : 1) ))
    commit "$name" "$h" "${MSGS[i % ${#MSGS[@]}]}" "main.$ext"
  done
  if [ "$remote" = 1 ]; then
    git init -q --bare -b main ".remotes/$name.git"
    git -C "$name" remote add origin "$ROOT/.remotes/$name.git"
    git -C "$name" push -q -u origin main
    git -C "$name" remote set-head origin main
  fi
}

# upstream <name> <commits>: pushes commits to the remote from a second clone, then
# fetches, so the local repo is behind.
upstream() {
  local name=$1 n=$2 i
  git clone -q ".remotes/$name.git" ".remotes/$name.clone"
  for ((i = 0; i < n; i++)); do commit ".remotes/$name.clone" 3 "Upstream change $((i + 1))" "main.go"; done
  git -C ".remotes/$name.clone" push -q origin main
  rm -rf ".remotes/$name.clone"
  git -C "$name" fetch -q
}

# ── clean and healthy ────────────────────────────────────────────────────────
seed lumen-api      go     60  2  14
seed tidepool       rs     45  5  11
seed quill-cli      go     30  20  8
seed nimbus-dash    svelte 40  9  12
seed ember-notes    swift  25  30 9
seed orbit-bot      py     50  1  13
seed mosaic-ui      tsx    35  12 10

# ── needs attention ──────────────────────────────────────────────────────────
# unpushed commits
seed pixel-forge ts 20 3 9
for h in 2 1 0; do commit pixel-forge "$h" "WIP: layer blending ($h)"; done

# no remote at all
seed scratchpad py 12 6 5 0

# uncommitted: modified + untracked
seed harbor-sync sh 28 4 8
echo "retry=3" >> harbor-sync/main.sh; echo "# todo" >> harbor-sync/util.sh
touch harbor-sync/notes.txt harbor-sync/debug.log

# staged only
seed paperplane py 18 15 6
echo "# staged" >> paperplane/core.py; git -C paperplane add core.py

# behind its remote
seed kestrel rs 33 26 10
upstream kestrel 4

# diverged: ahead and behind
seed foundry cpp 22 8 9
upstream foundry 2
commit foundry 1 "Local experiment"

# merge conflict in progress
seed lattice zig 15 7 6
git -C lattice checkout -q -b feature
sed -i '$ s/.*/\/\/ feature side/' lattice/main.zig
at 6; git -C lattice commit -qam "Feature edit"
git -C lattice checkout -q main
sed -i '$ s/.*/\/\/ main side/' lattice/main.zig
at 5; git -C lattice commit -qam "Main edit"
git -C lattice push -q
git -C lattice merge feature >/dev/null 2>&1 || true

# stashes
seed cobalt-docs py 40 50 7
for i in 1 2; do echo "// stash $i" >> cobalt-docs/main.py; git -C cobalt-docs stash -q; done

# feature branch never pushed
seed beacon go 14 10 6
git -C beacon checkout -q -b feat/webhooks
commit beacon 9 "Add webhook receiver"

# merged worktree (safe to remove)
seed tangram svelte 21 14 7
git -C tangram checkout -q -b feat/palette
commit tangram 20 "Add palette picker"
git -C tangram checkout -q main
at 18; git -C tangram merge -q --no-ff feat/palette -m "Merge feat/palette"
git -C tangram push -q
git -C tangram worktree add -q "../tangram-palette" feat/palette

# unmerged worktree with uncommitted work
seed vellum swift 19 22 6
git -C vellum worktree add -q -b feat/export "../vellum-export"
commit vellum-export 21 "Start PDF export" main.swift
echo "// half done" >> vellum-export/core.swift

# ── quiet / stale ────────────────────────────────────────────────────────────
seed atlas-maps tsx 200 24*90 8
seed glacier-backup sh 300 24*140 5
seed sprout dart 400 24*210 6

# busiest: lots of commits this week
seed pulse-metrics go 10 1 15

echo "built $(find "$ROOT" -maxdepth 1 -mindepth 1 -type d ! -name '.*' | wc -l) entries in $ROOT"
