#!/bin/sh
# Install simplex from this checkout, or clone it when the script is piped to a shell.
#
#   ./install.sh
#   curl -fsSL https://raw.githubusercontent.com/arcticfoxweb/simplex/main/install.sh | sh
#
# Windows uses install.ps1:
#   irm https://raw.githubusercontent.com/arcticfoxweb/simplex/main/install.ps1 | iex
#
# The curl form uses SIMPLEX_REPO_URL (default below). That repository has to
# exist before the pipe works. From a checkout, this script never clones.
set -eu

REPO_URL="${SIMPLEX_REPO_URL:-https://github.com/arcticfoxweb/simplex.git}"
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

ROOT=""
if ROOT=$(source_root); then
  printf 'building %s\n' "$ROOT"
else
  clone_source
  ROOT=$SRC_DIR
fi

if [ ! -f "$ROOT/herdr-plugin/build.sh" ]; then
  die "no herdr-plugin/build.sh in $ROOT"
fi

sh "$ROOT/herdr-plugin/build.sh"

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
