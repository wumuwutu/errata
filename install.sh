#!/bin/sh
# errata installer — detect OS/arch, download the matching release tarball
# from GitHub, verify SHA256, install the err binary.
#
#   curl -fsSL https://raw.githubusercontent.com/wumuwutu/errata/main/install.sh | sh
#
# Env overrides:
#   ERR_VERSION=v0.1.14   install a specific version (default: latest)
#   ERR_INSTALL_DIR=...   install directory (default: ~/.local/bin when it
#                         exists and is on PATH, else /usr/local/bin)
#   GITHUB_TOKEN=...      only needed while the repository is private
set -eu

REPO="wumuwutu/errata"

fail() { echo "install: $*" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || fail "missing required tool: $1"; }
need curl
need tar

dl() { # download helper: adds auth only when GITHUB_TOKEN is set
  if [ "${GITHUB_TOKEN:-}" ]; then
    curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" "$@"
  else
    curl -fsSL "$@"
  fi
}

os=$(uname -s)
arch=$(uname -m)
case "$os" in
  Linux)  os=linux ;;
  Darwin) os=darwin ;;
  *)      fail "unsupported OS: $os (errata is Unix-only; on Windows use WSL)" ;;
esac
case "$arch" in
  x86_64|amd64)  arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *)             fail "unsupported architecture: $arch" ;;
esac

# asset_id NAME — print the release-asset id of NAME from the release JSON
# on stdin (an asset's id precedes its name; the release's own id is never
# followed by a "name" line for a tarball, and uploader objects have none).
asset_id() {
  awk -v want="$1" '
    /^[[:space:]]*"id":/ { id=$2; gsub(/[^0-9]/, "", id) }
    $0 ~ "\"name\": \"" want "\"" { print id; exit }'
}

if [ "${GITHUB_TOKEN:-}" ]; then
  # Authenticated (private-repo) path: the browser URLs answer 404 to API
  # tokens, so resolve and download through the API instead.
  api="https://api.github.com/repos/$REPO"
  if [ "${ERR_VERSION:-}" ]; then
    tag="v${ERR_VERSION#v}"
    release_json=$(dl "$api/releases/tags/$tag") || fail "release $tag not found"
  else
    release_json=$(dl "$api/releases/latest") || fail "cannot resolve the latest release"
    tag=$(printf '%s' "$release_json" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)
  fi
  [ "$tag" ] || fail "cannot resolve the release tag"
else
  if [ "${ERR_VERSION:-}" ]; then
    tag="v${ERR_VERSION#v}"
  else
    # Follow the /releases/latest redirect: no API call, no rate limit.
    tag=$(curl -fsSIL -o /dev/null -w '%{url_effective}' \
      "https://github.com/$REPO/releases/latest" | sed 's/.*\///') || fail "cannot resolve the latest release"
  fi
  [ "$tag" ] || fail "cannot resolve the latest release"
fi
version="${tag#v}"

tarball="err_${version}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "install: downloading err $tag ($os/$arch)"
if [ "${GITHUB_TOKEN:-}" ]; then
  for name in "$tarball" checksums.txt; do
    id=$(printf '%s' "$release_json" | asset_id "$name")
    [ "$id" ] || fail "asset $name not found in release $tag"
    dl -H "Accept: application/octet-stream" "$api/releases/assets/$id" -o "$tmp/$name" || fail "download of $name failed"
  done
else
  dl "https://github.com/$REPO/releases/download/$tag/$tarball" -o "$tmp/$tarball" || fail "download failed"
  dl "https://github.com/$REPO/releases/download/$tag/checksums.txt" -o "$tmp/checksums.txt" || fail "checksum download failed"
fi

want=$(awk -v f="$tarball" '$2 == f {print $1}' "$tmp/checksums.txt")
[ "$want" ] || fail "no checksum found for $tarball"
if command -v sha256sum >/dev/null 2>&1; then
  got=$(sha256sum "$tmp/$tarball" | awk '{print $1}')
else
  got=$(shasum -a 256 "$tmp/$tarball" | awk '{print $1}')
fi
[ "$got" = "$want" ] || fail "SHA256 mismatch for $tarball"

tar -xzf "$tmp/$tarball" -C "$tmp"
[ -f "$tmp/err" ] || fail "tarball did not contain an err binary"

if [ "${ERR_INSTALL_DIR:-}" ]; then
  dest="$ERR_INSTALL_DIR"
elif [ -d "$HOME/.local/bin" ] && case ":$PATH:" in *":$HOME/.local/bin:"*) true ;; *) false ;; esac; then
  dest="$HOME/.local/bin"
else
  dest="/usr/local/bin"
fi

mkdir -p "$dest" 2>/dev/null || true
if [ -w "$dest" ]; then
  mv "$tmp/err" "$dest/err" || fail "cannot write $dest/err"
else
  echo "install: $dest is not writable — asking sudo to install"
  sudo mv "$tmp/err" "$dest/err" || fail "cannot write $dest/err"
fi
chmod +x "$dest/err" 2>/dev/null || sudo chmod +x "$dest/err"

echo "install: err $tag installed to $dest/err"
echo
echo "next step — capture errors automatically, add to your ~/.zshrc or ~/.bashrc:"
echo '  eval "$(err init zsh)"   # zsh'
echo '  eval "$(err init bash)"   # bash (3.2+)'
echo "(or let err write it for you: err init zsh --write)"
