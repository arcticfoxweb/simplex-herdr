#!/bin/sh
# Build the simplex CLI and download simplex-chat when it is not already present.
# Herdr runs this during `herdr plugin install` on Linux and macOS.
# `herdr plugin link` does not. The repo install.sh calls this too.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
while [ ! -f "$root/go.mod" ]; do
  parent=$(CDPATH= cd -- "$root/.." && pwd)
  if [ "$parent" = "$root" ]; then
    printf 'simplex build: no go.mod above %s\n' "$(dirname "$0")" >&2
    exit 1
  fi
  root=$parent
done

if ! command -v go >/dev/null 2>&1; then
  printf 'simplex build: need Go on PATH (1.22 or newer; the module uses 1.26)\n' >&2
  exit 1
fi

bindir="${HOME}/.local/bin"
mkdir -p "$bindir"
out="$bindir/simplex"
case "$(uname -s 2>/dev/null || printf unknown)" in
  MINGW*|MSYS*|CYGWIN*) out="$bindir/simplex.exe" ;;
esac

(
  cd "$root"
  go build -o "$out" ./cmd/simplex
)
printf 'built %s\n' "$out"

if [ -n "${SIMPLEX_CHAT_BIN:-}" ] && [ -f "$SIMPLEX_CHAT_BIN" ]; then
  printf 'using simplex-chat at %s\n' "$SIMPLEX_CHAT_BIN"
  exit 0
fi

share="${HOME}/.local/share/simplex/bin/simplex-chat"
if [ -x "$share" ] || [ -x "${share}.exe" ] || command -v simplex-chat >/dev/null 2>&1; then
  printf 'simplex-chat already installed; skipping download\n'
  exit 0
fi

"$out" install
