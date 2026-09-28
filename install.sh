#!/bin/sh
# Install simplex from this checkout, or clone it when the script is piped to a shell.
#
#   ./install.sh
#   curl -fsSL https://raw.githubusercontent.com/arcticfoxweb/simplex-herdr/main/install.sh | sh
#
# Windows uses install.ps1:
#   irm https://raw.githubusercontent.com/arcticfoxweb/simplex-herdr/main/install.ps1 | iex
#
# The curl form downloads the newest GitHub release binary, including an alpha
# pre-release, and checks SHA256SUMS. Set SIMPLEX_FROM_SOURCE=1 to compile.
# From a checkout, this script always compiles.
set -eu

REPO_URL="${SIMPLEX_REPO_URL:-https://github.com/arcticfoxweb/simplex-herdr.git}"
REF="${SIMPLEX_REF:-}"
SRC_DIR="${SIMPLEX_SRC_DIR:-${HOME}/.local/src/simplex}"

die() {
  printf 'install: %s\n' "$1" >&2
  exit 1
}

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    die "need $1 on PATH"
  fi
}

# Print the source root when this file sits next to go.mod. Empty otherwise,
# including when a shell is reading the script from a pipe.
source_root() {
  case "${0-}" in
    ""|sh|bash|-sh|-bash) return 1 ;;
  esac
  if [ ! -f "$0" ]; then
    return 1
  fi
  dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd) || return 1
  if [ -f "$dir/go.mod" ] && [ -f "$dir/herdr-plugin/build.sh" ]; then
    printf '%s\n' "$dir"
    return 0
  fi
  return 1
}

clone_source() {
  need git
  parent=$(dirname "$SRC_DIR")
  mkdir -p "$parent"
  if [ -d "$SRC_DIR/.git" ]; then
    printf 'updating %s\n' "$SRC_DIR" >&2
    git -C "$SRC_DIR" remote set-url origin "$REPO_URL"
    if [ -n "$REF" ]; then
      git -C "$SRC_DIR" fetch --depth 1 origin "$REF"
      git -C "$SRC_DIR" checkout --detach FETCH_HEAD
    else
      git -C "$SRC_DIR" pull --ff-only
    fi
  elif [ -e "$SRC_DIR" ]; then
    die "$SRC_DIR exists and is not a git checkout"
  else
    printf 'cloning %s\n' "$REPO_URL" >&2
    if [ -n "$REF" ]; then
      git clone --depth 1 --branch "$REF" "$REPO_URL" "$SRC_DIR"
    else
      git clone --depth 1 "$REPO_URL" "$SRC_DIR"
    fi
  fi
}

release_asset() {
  os=$(uname -s)
  mach=$(uname -m)
  case "$os-$mach" in
    Linux-x86_64) printf '%s\n' simplex-linux-amd64 ;;
    Linux-aarch64|Linux-arm64) printf '%s\n' simplex-linux-arm64 ;;
    Darwin-arm64) printf '%s\n' simplex-darwin-arm64 ;;
    Darwin-x86_64) printf '%s\n' simplex-darwin-amd64 ;;
    *) return 1 ;;
  esac
}

file_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

# Download the newest release binary, including a pre-release. GitHub's
# /releases/latest URL skips pre-releases, so this reads the releases API.
fetch_release_bin() {
  asset=$(release_asset) || return 1
  command -v curl >/dev/null 2>&1 || return 1
  api=$(curl -fsSL -H "User-Agent: simplex-install" -H "Accept: application/vnd.github+json" \
    "https://api.github.com/repos/arcticfoxweb/simplex-herdr/releases?per_page=10") || return 1
  # GitHub returns this list as one JSON line, so do not use a greedy sed match.
  urls=$(printf '%s\n' "$api" | grep -o 'https://github.com/arcticfoxweb/simplex-herdr/releases/download/[^"]*' || true)
  if [ -n "${SIMPLEX_VERSION:-}" ]; then
    url=$(printf '%s\n' "$urls" | grep "/${SIMPLEX_VERSION}/${asset}$" | head -n 1 || true)
    sums=$(printf '%s\n' "$urls" | grep "/${SIMPLEX_VERSION}/SHA256SUMS$" | head -n 1 || true)
  else
    url=$(printf '%s\n' "$urls" | grep "/${asset}$" | head -n 1 || true)
    sums=$(printf '%s\n' "$urls" | grep '/SHA256SUMS$' | head -n 1 || true)
  fi
  if [ -z "$url" ] || [ -z "$sums" ]; then
    return 1
  fi
  bindir="${HOME}/.local/bin"
  mkdir -p "$bindir"
  tmp=$(mktemp)
  sumtmp=$(mktemp)
  if ! curl -fsSL -H "User-Agent: simplex-install" -o "$tmp" "$url"; then
    rm -f "$tmp" "$sumtmp"
    return 1
  fi
  if ! curl -fsSL -H "User-Agent: simplex-install" -o "$sumtmp" "$sums"; then
    rm -f "$tmp" "$sumtmp"
    return 1
  fi
  want=$(awk -v name="$asset" '$NF == name || $NF == ("*" name) { print $1; exit }' "$sumtmp")
  got=$(file_sha256 "$tmp")
  if [ -z "$want" ] || [ "$want" != "$got" ]; then
    rm -f "$tmp" "$sumtmp"
    die "checksum mismatch for $asset"
  fi
  mv "$tmp" "$bindir/simplex"
  chmod 755 "$bindir/simplex"
  rm -f "$sumtmp"
  printf 'installed %s from the GitHub release\n' "$bindir/simplex"
}

ensure_chat() {
  bin="${HOME}/.local/bin/simplex"
  if [ -n "${SIMPLEX_CHAT_BIN:-}" ] && [ -f "$SIMPLEX_CHAT_BIN" ]; then
    printf 'using simplex-chat at %s\n' "$SIMPLEX_CHAT_BIN"
    return 0
  fi
  share="${HOME}/.local/share/simplex/bin/simplex-chat"
  if [ -x "$share" ] || [ -x "${share}.exe" ] || command -v simplex-chat >/dev/null 2>&1; then
    printf 'simplex-chat already installed; skipping download\n'
    return 0
  fi
  "$bin" install
}

ROOT=""
from_tree=0
if ROOT=$(source_root); then
  from_tree=1
  printf 'building %s\n' "$ROOT"
fi

got_bin=0
if [ "$from_tree" -eq 0 ] && [ "${SIMPLEX_FROM_SOURCE:-}" != 1 ]; then
  if fetch_release_bin; then
    got_bin=1
  else
    printf 'no release binary; building from source\n' >&2
  fi
fi

if [ -z "$ROOT" ]; then
  clone_source
  ROOT=$SRC_DIR
fi

if [ ! -f "$ROOT/herdr-plugin/build.sh" ]; then
  die "no herdr-plugin/build.sh in $ROOT"
fi

if [ "$got_bin" -eq 0 ]; then
  sh "$ROOT/herdr-plugin/build.sh"
else
  ensure_chat
fi

if [ "${SIMPLEX_SKIP_LINK:-}" = "1" ]; then
  printf 'skipping Herdr plugin link\n'
elif command -v herdr >/dev/null 2>&1; then
  herdr plugin link "$ROOT/herdr-plugin"
  printf 'linked Herdr plugin simplex.agents from %s\n' "$ROOT/herdr-plugin"
else
  printf 'herdr is not on PATH; the plugin was not linked\n'
fi

bindir="${HOME}/.local/bin"
cat <<EOF

Installed ${bindir}/simplex

From the Herdr pane where the agent is already running:

  simplex init
  herdr plugin action invoke simplex.agents.attach
  simplex qr

Send through the plugin:

  herdr plugin pane open --plugin simplex.agents --entrypoint send \\
    --env SIMPLEX_TO=NAME --env SIMPLEX_TEXT='hello'

${bindir} has to be on PATH. Restart Herdr after the first install so the
plugin startup hook runs. See README.md for groups, files, and MCP.
EOF
