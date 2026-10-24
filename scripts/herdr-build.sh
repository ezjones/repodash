#!/bin/sh
# Build hook for the herdr plugin: puts ./repodash in the plugin root.
# With Go installed it builds from source; otherwise it fetches the release binary.
set -eu
cd "$(dirname "$0")/.."
if command -v go >/dev/null 2>&1; then
  CGO_ENABLED=0 go build -o repodash .
else
  PREFIX="$PWD" sh install.sh
fi
