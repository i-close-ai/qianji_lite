#!/bin/sh
# Remove the Qianji Lite binary and skill links.
# Does not delete ~/.qianji/config.toml, state.json, or Pi credentials.

set -e

USER=${USER:-$(id -u -n)}
HOME="${HOME:-$(eval echo ~"$USER")}"
PREFIX=${PREFIX:-$HOME/.local}
BIN_DIR="$PREFIX/bin"

rm -f "$BIN_DIR/qianji"

for link in \
  "$HOME/.agents/skills/qianji" \
  "$HOME/.cursor/skills/qianji" \
  "$HOME/.claude/skills/qianji" \
  "$HOME/.codex/skills/qianji"
do
  if [ -L "$link" ]; then
    rm -f "$link"
  fi
done

printf 'Removed %s/qianji and skill symlinks.\n' "$BIN_DIR"
printf 'Left in place: ~/.qianji (routing config/state) and ~/.pi (Pi credentials).\n'
printf 'Clone directory (if used): ~/.qianji-lite — delete it manually if you want.\n'
