#!/usr/bin/env bash
#
# Provision a machine to build and test margin.
#
# margin embeds a real nvim as its comment composer, so the test suite needs
# nvim on PATH. Without it roughly two thirds of the tests call t.Skip() and the
# run still reports ok — a green suite in an unprovisioned environment proves
# almost nothing. That is what the verify step at the bottom exists to catch.
#
# Usage:
#   ./scripts/setup-env.sh            install what is missing, then verify
#   ./scripts/setup-env.sh --verify   verify only, install nothing
#
# Safe to re-run: every step is a no-op when the requirement is already met.

set -euo pipefail

NVIM_VERSION="${NVIM_VERSION:-v0.12.4}"
GO_MIN="${GO_MIN:-1.24.2}"
VERIFY_ONLY=false
[[ "${1:-}" == "--verify" ]] && VERIFY_ONLY=true

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m warn\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31m fail\033[0m %s\n' "$*" >&2; exit 1; }

SUDO=""
if [[ $EUID -ne 0 ]]; then
  command -v sudo >/dev/null 2>&1 && SUDO="sudo" || die "not root and no sudo available"
fi

case "$(uname -m)" in
  x86_64|amd64) NVIM_ARCH="x86_64"; GO_ARCH="amd64" ;;
  aarch64|arm64) NVIM_ARCH="arm64"; GO_ARCH="arm64"  ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac

# at_least PROVIDED REQUIRED — true when PROVIDED >= REQUIRED, semver-ish.
at_least() {
  [[ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -1)" == "$2" ]]
}

# ---------------------------------------------------------------- packages ---

install_base() {
  command -v curl >/dev/null 2>&1 &&
    command -v git >/dev/null 2>&1 &&
    command -v make >/dev/null 2>&1 &&
    command -v cc >/dev/null 2>&1 && return 0

  log "installing base packages"
  if command -v apt-get >/dev/null 2>&1; then
    $SUDO apt-get update -qq
    # gcc is required by `go test -race`, which margin's composer tests need:
    # the emulator is written from the pty goroutine and read from the render
    # goroutine, and that class of bug is invisible without the race detector.
    $SUDO apt-get install -y -qq curl ca-certificates git make gcc tar >/dev/null
  elif command -v dnf >/dev/null 2>&1; then
    $SUDO dnf install -y -q curl ca-certificates git make gcc tar
  elif command -v apk >/dev/null 2>&1; then
    $SUDO apk add --no-cache curl ca-certificates git make gcc musl-dev tar
  else
    warn "no known package manager; assuming curl/git/make/cc are present"
  fi
}

# -------------------------------------------------------------------- nvim ---

install_nvim() {
  if command -v nvim >/dev/null 2>&1; then
    local have
    have="$(nvim --version | head -1 | sed 's/^NVIM v//')"
    # cmdheight=0 needs 0.8+; the composer also uses nvim_create_user_command
    # and once=true autocmds, both 0.7+.
    if at_least "$have" "0.8.0"; then
      log "nvim $have already present"
      return 0
    fi
    warn "nvim $have is too old for the composer; installing $NVIM_VERSION"
  fi

  log "installing nvim $NVIM_VERSION ($NVIM_ARCH)"
  local tarball="nvim-linux-${NVIM_ARCH}.tar.gz"
  local url="https://github.com/neovim/neovim/releases/download/${NVIM_VERSION}/${tarball}"
  local tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN

  if ! curl -fsSL "$url" -o "$tmp/$tarball"; then
    # Asset naming changed at 0.10.4: older releases used nvim-linux64.tar.gz.
    warn "asset $tarball not found; trying the pre-0.10.4 name"
    url="https://github.com/neovim/neovim/releases/download/${NVIM_VERSION}/nvim-linux64.tar.gz"
    curl -fsSL "$url" -o "$tmp/$tarball" || die "could not download nvim $NVIM_VERSION"
  fi

  # The tarball, not the AppImage: AppImages need FUSE, which containers
  # usually lack.
  $SUDO rm -rf /opt/nvim
  $SUDO mkdir -p /opt/nvim
  $SUDO tar -xzf "$tmp/$tarball" -C /opt/nvim --strip-components=1
  $SUDO ln -sf /opt/nvim/bin/nvim /usr/local/bin/nvim
}

install_spell_suggestions() {
  # The tarball ships runtime/spell/en.utf-8.spl, so spell-check works and nvim
  # will not prompt to download anything. It does not ship the .sug companion,
  # which only affects the quality of `z=` suggestions — nice to have, never
  # fatal, so a failure here is not an error.
  local dir="/opt/nvim/share/nvim/runtime/spell"
  [[ -d "$dir" ]] || return 0
  [[ -f "$dir/en.utf-8.sug" ]] && return 0
  log "fetching en.utf-8.sug (optional, improves z= suggestions)"
  $SUDO curl -fsSL --max-time 30 \
    "https://ftp.nluug.nl/pub/vim/runtime/spell/en.utf-8.sug" \
    -o "$dir/en.utf-8.sug" || warn "could not fetch .sug; z= suggestions will be weaker"
}

# ---------------------------------------------------------------------- go ---

install_go() {
  if command -v go >/dev/null 2>&1; then
    local have
    have="$(go version | awk '{print $3}' | sed 's/^go//')"
    if at_least "$have" "$GO_MIN"; then
      log "go $have already present"
      return 0
    fi
    warn "go $have is older than $GO_MIN"
  fi

  local ver
  ver="$(curl -fsSL https://go.dev/VERSION?m=text | head -1)" || die "could not resolve latest Go"
  log "installing $ver ($GO_ARCH)"
  local tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN
  curl -fsSL "https://go.dev/dl/${ver}.linux-${GO_ARCH}.tar.gz" -o "$tmp/go.tgz" \
    || die "could not download $ver"
  $SUDO rm -rf /usr/local/go
  $SUDO tar -C /usr/local -xzf "$tmp/go.tgz"
  $SUDO ln -sf /usr/local/go/bin/go /usr/local/bin/go
  $SUDO ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
}

# ------------------------------------------------------------------ verify ---

verify() {
  log "verifying"
  local failed=0

  command -v go >/dev/null 2>&1 \
    && echo "  go       $(go version | awk '{print $3}')" \
    || { echo "  go       MISSING"; failed=1; }

  command -v cc >/dev/null 2>&1 \
    && echo "  cc       $(cc --version 2>/dev/null | head -1)" \
    || { echo "  cc       MISSING (go test -race will not work)"; failed=1; }

  if command -v nvim >/dev/null 2>&1; then
    echo "  nvim     $(nvim --version | head -1)"
  else
    echo "  nvim     MISSING"
    failed=1
  fi

  # Spell-check is on in the composer. Without a spell file nvim prompts to
  # download one, and a modal prompt inside a ten-row pane would wedge it.
  if command -v nvim >/dev/null 2>&1; then
    local spl
    spl="$(nvim --headless -c 'echo globpath(&rtp, "spell/en*.spl")' -c 'q' 2>&1 | tr -d '\r')"
    if [[ -n "$spl" ]]; then
      echo "  spell    ok"
    else
      echo "  spell    MISSING — the composer would prompt to download one"
      failed=1
    fi
  fi

  [[ $failed -eq 0 ]] || die "environment is not ready"

  # The real acceptance test. This one calls requireNvim(), so it SKIPS rather
  # than fails when nvim is absent — which is exactly the false green this
  # script exists to prevent. Demand an explicit PASS.
  if [[ -f go.mod ]] && grep -q 'module github.com/neuroplastio/margin' go.mod 2>/dev/null; then
    log "running a composer test to prove it does not skip"
    local out
    out="$(go test ./internal/review/ -run TestNewCommentStartsInInsertMode -v 2>&1)" || {
      echo "$out" | tail -20
      die "composer test failed"
    }
    if grep -q -- "--- SKIP" <<<"$out"; then
      echo "$out" | grep -- "--- SKIP" | head -3
      die "composer test SKIPPED — nvim is not usable from the test harness"
    fi
    grep -q -- "--- PASS" <<<"$out" || die "composer test neither passed nor skipped"
    echo "  composer PASS (did not skip)"
  else
    warn "not in the margin checkout; skipped the composer acceptance test"
  fi

  log "environment is ready"
}

# -------------------------------------------------------------------- main ---

if [[ "$VERIFY_ONLY" == false ]]; then
  install_base
  install_nvim
  install_spell_suggestions
  install_go
fi
verify
