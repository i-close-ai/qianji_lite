#!/bin/sh
#
# Qianji Lite installer.
#
# This script should be run via curl:
#   sh -c "$(curl -fsSL https://raw.githubusercontent.com/i-close-ai/qianji_lite/main/tools/install.sh)"
#
# or via wget:
#   sh -c "$(wget -qO- https://raw.githubusercontent.com/i-close-ai/qianji_lite/main/tools/install.sh)"
#
# From a local clone:
#   sh tools/install.sh
#
# Environment (all optional):
#   REPO        GitHub repo (default: i-close-ai/qianji_lite)
#   REMOTE      git remote URL (default: https://github.com/${REPO}.git)
#   BRANCH      branch to install (default: main)
#   QIANJI_DIR  clone destination when piped from curl (default: $HOME/.qianji-lite)
#   PREFIX      binary prefix (default: $HOME/.local)
#   SKIP_GO=1   do not try to install Go
#   SKIP_NODE=1 do not try to install Node.js
#   SKIP_PI=1   do not install the Pi CLI
#   SKIP_SKILL=1
#   SKIP_INIT=1 do not run `qianji init`
#   GOPROXY     forwarded to `go build` if set
#
# This script never writes API keys and never touches ~/.pi/agent/models.json
# or auth.json.

set -e

USER=${USER:-$(id -u -n)}
HOME="${HOME:-$(eval echo ~"$USER")}"

REPO=${REPO:-i-close-ai/qianji_lite}
REMOTE=${REMOTE:-https://github.com/${REPO}.git}
BRANCH=${BRANCH:-main}
QIANJI_DIR=${QIANJI_DIR:-$HOME/.qianji-lite}
PREFIX=${PREFIX:-$HOME/.local}
BIN_DIR="$PREFIX/bin"
PI_PACKAGE=${PI_PACKAGE:-@earendil-works/pi-coding-agent}

fmt_error() {
  printf 'qianji-lite: error: %s\n' "$*" >&2
}

fmt_info() {
  printf 'qianji-lite: %s\n' "$*"
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

resolve_source() {
  case "$0" in
    */install.sh|install.sh)
      root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
      if [ -f "$root/go.mod" ] && [ -d "$root/cmd/qianji" ]; then
        SOURCE=$root
        return
      fi
      ;;
  esac
  SOURCE=
}

go_is_new_enough() {
  command_exists go || return 1
  ver=$(go version 2>/dev/null | awk '{print $3}' | sed 's/^go//')
  major=$(printf '%s\n' "$ver" | cut -d. -f1)
  minor=$(printf '%s\n' "$ver" | cut -d. -f2)
  [ -n "$major" ] && [ -n "$minor" ] || return 1
  [ "$major" -gt 1 ] || { [ "$major" -eq 1 ] && [ "$minor" -ge 22 ]; }
}

install_with_brew() {
  pkg=$1
  command_exists brew || return 1
  fmt_info "installing $pkg with Homebrew"
  brew install "$pkg"
}

ensure_go() {
  if go_is_new_enough; then
    fmt_info "Go $(go version | awk '{print $3}') ok"
    return 0
  fi
  if [ "${SKIP_GO:-}" = 1 ]; then
    fmt_error "Go 1.22+ is required. Install it from https://go.dev/dl/"
    exit 1
  fi
  if install_with_brew go && go_is_new_enough; then
    return 0
  fi
  fmt_error "Go 1.22+ is required. Install it from https://go.dev/dl/ and re-run this script."
  exit 1
}

ensure_node() {
  if command_exists npm && command_exists node; then
    fmt_info "Node.js $(node --version) ok"
    return 0
  fi
  if [ "${SKIP_NODE:-}" = 1 ] || [ "${SKIP_PI:-}" = 1 ]; then
    return 0
  fi
  if install_with_brew node && command_exists npm; then
    return 0
  fi
  fmt_error "Node.js + npm are required to install Pi. Install Node from https://nodejs.org/ and re-run."
  exit 1
}

ensure_git() {
  if command_exists git; then
    return 0
  fi
  if install_with_brew git; then
    return 0
  fi
  fmt_error "git is required"
  exit 1
}

ensure_pi() {
  if [ "${SKIP_PI:-}" = 1 ]; then
    fmt_info "skipping Pi install (SKIP_PI=1)"
    return 0
  fi
  if command_exists pi; then
    fmt_info "Pi already on PATH: $(command -v pi)"
    return 0
  fi
  if [ -x "$HOME/.local/bin/pi" ]; then
    fmt_info "Pi already at $HOME/.local/bin/pi"
    return 0
  fi
  if ! command_exists npm; then
    fmt_error "npm not found; cannot install Pi"
    exit 1
  fi
  fmt_info "installing official Pi CLI ($PI_PACKAGE)"
  if npm install -g --ignore-scripts "$PI_PACKAGE"; then
    :
  else
    fmt_info "retrying Pi install with prefix $HOME/.local"
    npm install -g --prefix "$HOME/.local" --ignore-scripts "$PI_PACKAGE"
  fi
  if command_exists pi || [ -x "$HOME/.local/bin/pi" ]; then
    return 0
  fi
  fmt_error "Pi installed but not found on PATH. Add $BIN_DIR to PATH and re-run."
  exit 1
}

ensure_path_file() {
  file=$1
  line='export PATH="$HOME/.local/bin:$PATH"'
  marker='# qianji-lite'
  [ -f "$file" ] || return 0
  if grep -F "$BIN_DIR" "$file" >/dev/null 2>&1 || grep -F '$HOME/.local/bin' "$file" >/dev/null 2>&1; then
    return 0
  fi
  fmt_info "adding $BIN_DIR to PATH in $file"
  printf '\n%s\n%s\n' "$marker" "$line" >>"$file"
}

ensure_path() {
  mkdir -p "$BIN_DIR"
  case ":$PATH:" in
    *":$BIN_DIR:"*) ;;
    *)
      PATH="$BIN_DIR:$PATH"
      export PATH
      ;;
  esac
  shell_name=$(basename "${SHELL:-sh}")
  case "$shell_name" in
    zsh) ensure_path_file "$HOME/.zshrc" ;;
    bash)
      if [ -f "$HOME/.bashrc" ]; then
        ensure_path_file "$HOME/.bashrc"
      else
        ensure_path_file "$HOME/.bash_profile"
      fi
      ;;
    *)
      ensure_path_file "$HOME/.profile"
      ;;
  esac
}

clone_or_update() {
  if [ -n "$SOURCE" ]; then
    fmt_info "using local source $SOURCE"
    return 0
  fi
  ensure_git
  if [ -d "$QIANJI_DIR/.git" ]; then
    fmt_info "updating $QIANJI_DIR"
    git -C "$QIANJI_DIR" fetch --depth=1 origin "$BRANCH"
    git -C "$QIANJI_DIR" checkout "$BRANCH"
    git -C "$QIANJI_DIR" merge --ff-only "origin/$BRANCH"
  else
    fmt_info "cloning $REMOTE ($BRANCH) into $QIANJI_DIR"
    umask go-w
    mkdir -p "$(dirname "$QIANJI_DIR")"
    git clone --depth=1 --branch "$BRANCH" "$REMOTE" "$QIANJI_DIR"
  fi
  SOURCE=$QIANJI_DIR
}

build_qianji() {
  fmt_info "building qianji into $BIN_DIR/qianji"
  mkdir -p "$BIN_DIR"
  (
    cd "$SOURCE"
    if [ -n "${GOPROXY:-}" ]; then
      export GOPROXY
    fi
    if ! go build -ldflags="-s -w" -o "$BIN_DIR/qianji" ./cmd/qianji; then
      fmt_info "build failed; retrying with GOPROXY=https://goproxy.cn,direct"
      GOPROXY=https://goproxy.cn,direct go build -ldflags="-s -w" -o "$BIN_DIR/qianji" ./cmd/qianji
    fi
  )
  chmod 755 "$BIN_DIR/qianji"
}

install_skill() {
  if [ "${SKIP_SKILL:-}" = 1 ]; then
    return 0
  fi
  fmt_info "installing host-agnostic skill"
  "$BIN_DIR/qianji" skill install
}

maybe_init() {
  if [ "${SKIP_INIT:-}" = 1 ]; then
    return 0
  fi
  if ! command_exists pi && [ ! -x "$HOME/.local/bin/pi" ]; then
    fmt_info "Pi not on PATH yet; skip qianji init. After Pi is configured, run: qianji init"
    return 0
  fi
  fmt_info "importing Pi catalog (qianji init)"
  if ! "$BIN_DIR/qianji" init; then
    cat <<'EOF'

qianji-lite: binary and skill are installed, but `qianji init` found no Pi models.
Configure official Pi (no keys belong in Qianji), then run:

  pi --list-models
  qianji init
  qianji doctor

Pi models docs:
  https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/models.md
EOF
  fi
}

print_done() {
  cat <<EOF

Qianji Lite is installed.

  binary : $BIN_DIR/qianji
  source : $SOURCE
  config : \$HOME/.qianji/config.toml   (created by qianji init)
  skill  : \$HOME/.qianji/skill

If the qianji command is not found, open a new terminal or:

  export PATH="$BIN_DIR:\$PATH"

Then:

  qianji doctor
  qianji status

Qianji never stores provider API keys. Keep them in official Pi only.
EOF
}

main() {
  resolve_source
  ensure_path
  ensure_go
  ensure_node
  ensure_pi
  clone_or_update
  build_qianji
  install_skill
  maybe_init
  print_done
}

main "$@"
