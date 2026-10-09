#!/bin/sh
# RepoDash installer: downloads the release binary for this OS/arch, verifies
# its checksum, and installs it. No sudo needed by default.
#
#   curl -fsSL https://github.com/ezjones/repodash/releases/latest/download/install.sh | sh
#
# Environment:
#   VERSION=v0.1.0   install this release instead of the latest
#   PREFIX=DIR       install into DIR (default: ~/.local/bin)
#   BASE_URL=URL     download from URL instead of GitHub (for testing)
set -eu

REPO="ezjones/repodash"
PREFIX="${PREFIX:-$HOME/.local/bin}"

say() { printf '%s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

case "$(uname -s)" in
  Linux)  os=linux ;;
  Darwin) os=darwin ;;
  *) die "unsupported OS: $(uname -s). RepoDash supports Linux, macOS and Windows via WSL." ;;
esac
case "$(uname -m)" in
  x86_64|amd64)  arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac
if [ "$os" = linux ] && grep -qi microsoft /proc/version 2>/dev/null; then
  say "Detected Windows (WSL): installing the Linux build."
fi

if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -qO "$2" "$1"; }
else
  die "need curl or wget"
fi

if [ -n "${BASE_URL:-}" ]; then
  base="$BASE_URL"
elif [ -n "${VERSION:-}" ]; then
  base="https://github.com/$REPO/releases/download/$VERSION"
else
  base="https://github.com/$REPO/releases/latest/download"
fi

asset="repodash_${os}_${arch}.tar.gz"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

say "Downloading $asset ..."
fetch "$base/$asset" "$tmp/$asset" || die "download failed: $base/$asset"
fetch "$base/checksums.txt" "$tmp/checksums.txt" || die "download failed: $base/checksums.txt"

want="$(grep " $asset\$" "$tmp/checksums.txt" | cut -d' ' -f1)"
[ -n "$want" ] || die "$asset not listed in checksums.txt"
if command -v sha256sum >/dev/null 2>&1; then
  got="$(sha256sum "$tmp/$asset" | cut -d' ' -f1)"
else
  got="$(shasum -a 256 "$tmp/$asset" | cut -d' ' -f1)"
fi
[ "$want" = "$got" ] || die "checksum mismatch for $asset"

tar -xzf "$tmp/$asset" -C "$tmp" repodash
mkdir -p "$PREFIX"
install -m 0755 "$tmp/repodash" "$PREFIX/repodash"

say "Installed: $("$PREFIX/repodash" -version) -> $PREFIX/repodash"
case ":$PATH:" in
  *":$PREFIX:"*) ;;
  *) say "Note: $PREFIX is not on your PATH. Add it, or run $PREFIX/repodash directly." ;;
esac
say "Run: repodash   then open http://127.0.0.1:8092"
