#!/usr/bin/env bash
# Steelpage updater.
#
# Quickstart:
#   curl -fsSL https://raw.githubusercontent.com/lacrioque/steelpage/main/scripts/update.sh | bash
#
# What it does:
#   1. Reads the installed version from $INSTALL_DIR/.steelpage-version
#      (written by install.sh on fresh installs; empty for pre-1.1.0).
#   2. Looks up the latest release tag on GitHub.
#   3. If they match: exits clean.
#   4. Otherwise: confirms, downloads + sha256-verifies the new tarball,
#      stops the systemd service (when active), swaps the binary in
#      place (old one kept as steelpage.prev for rollback), updates the
#      version file, and starts the service back up.
#
# config.yaml and the content repo are never touched.
#
# Env vars:
#   INSTALL_DIR  default /opt/steelpage
#   ASSUME_YES   if "yes", skip the confirm prompt (for cron use)

set -euo pipefail

REPO="lacrioque/steelpage"
INSTALL_DIR="${INSTALL_DIR:-/opt/steelpage}"
SERVICE="steelpage"
ASSUME_YES="${ASSUME_YES:-no}"
TMPDIR_REAL=$(mktemp -d)
trap 'rm -rf "$TMPDIR_REAL"' EXIT

# Colors + tty + prompt helpers (same shape as install.sh)
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
  C_BOLD=$'\033[1m'; C_DIM=$'\033[2m'; C_RED=$'\033[31m'
  C_GREEN=$'\033[32m'; C_YELLOW=$'\033[33m'; C_RESET=$'\033[0m'
else
  C_BOLD=""; C_DIM=""; C_RED=""; C_GREEN=""; C_YELLOW=""; C_RESET=""
fi
say()  { printf "%s==>%s %s\n" "$C_BOLD" "$C_RESET" "$*"; }
warn() { printf "%swarn:%s %s\n" "$C_YELLOW" "$C_RESET" "$*" >&2; }
err()  { printf "%serror:%s %s\n" "$C_RED" "$C_RESET" "$*" >&2; }
ok()   { printf "%s✓%s %s\n" "$C_GREEN" "$C_RESET" "$*"; }

TTY=/dev/tty

confirm() {
  local question="$1" default="${2:-Y}" answer hint
  case "$default" in Y|y) hint="[Y/n]";; *) hint="[y/N]";; esac
  if [ "$ASSUME_YES" = "yes" ]; then return 0; fi
  printf "%s %s " "$question" "$hint" >"$TTY"
  read -r answer <"$TTY"
  [ -z "$answer" ] && answer="$default"
  case "$answer" in Y|y|yes|Yes|YES) return 0 ;; *) return 1 ;; esac
}

maybe_sudo=()
if [ "$(id -u)" -ne 0 ] && command -v sudo >/dev/null 2>&1; then
  maybe_sudo=(sudo)
fi
asroot() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
  elif [ ${#maybe_sudo[@]} -gt 0 ]; then
    "${maybe_sudo[@]}" "$@"
  else
    err "this step needs root: $*"
    exit 1
  fi
}

# ───────── detect platform ─────────

OS_RAW=$(uname -s)
ARCH_RAW=$(uname -m)
case "$OS_RAW" in
  Linux)  OS=linux ;;
  Darwin) OS=darwin ;;
  *) err "unsupported OS: $OS_RAW"; exit 1 ;;
esac
case "$ARCH_RAW" in
  x86_64|amd64)  ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) err "unsupported architecture: $ARCH_RAW"; exit 1 ;;
esac

for cmd in curl tar; do
  command -v "$cmd" >/dev/null 2>&1 || { err "missing $cmd"; exit 1; }
done
if command -v sha256sum >/dev/null 2>&1; then
  SHA_CMD=(sha256sum -c)
elif command -v shasum >/dev/null 2>&1; then
  SHA_CMD=(shasum -a 256 -c)
else
  err "need sha256sum or shasum"; exit 1
fi

# ───────── current vs latest ─────────

if [ ! -d "$INSTALL_DIR" ]; then
  err "$INSTALL_DIR doesn't exist. Run install.sh first, or set INSTALL_DIR."
  exit 1
fi
if [ ! -x "$INSTALL_DIR/steelpage" ]; then
  err "$INSTALL_DIR/steelpage is missing or not executable. Looks like a broken install."
  exit 1
fi

CURRENT=""
if [ -f "$INSTALL_DIR/.steelpage-version" ]; then
  CURRENT=$(cat "$INSTALL_DIR/.steelpage-version")
fi
if [ -z "$CURRENT" ]; then
  warn "couldn't read $INSTALL_DIR/.steelpage-version — proceeding will record the new version anyway"
fi

say "Looking up the latest release"
RELEASE_JSON=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest")
TAG=$(printf "%s" "$RELEASE_JSON" | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name"[^"]*"([^"]+)".*/\1/')
if [ -z "$TAG" ]; then
  err "couldn't parse a tag from the GitHub API response"; exit 1
fi

if [ -n "$CURRENT" ] && [ "$CURRENT" = "$TAG" ]; then
  ok "already on $TAG — nothing to do."
  exit 0
fi

if [ -n "$CURRENT" ]; then
  printf "  Installed: %s%s%s\n" "$C_DIM" "$CURRENT" "$C_RESET"
fi
printf "  Latest:    %s%s%s\n" "$C_BOLD" "$TAG" "$C_RESET"

if ! confirm "Update now?" "Y"; then
  ok "Aborted by user."
  exit 0
fi

# ───────── download + verify ─────────

ASSET="steelpage-${TAG}-${OS}-${ARCH}.tar.gz"
ASSET_URL="https://github.com/$REPO/releases/download/$TAG/$ASSET"

say "Downloading $ASSET"
curl -fsSL --progress-bar "$ASSET_URL" -o "$TMPDIR_REAL/$ASSET"
curl -fsSL "${ASSET_URL}.sha256" -o "$TMPDIR_REAL/${ASSET}.sha256"

say "Verifying checksum"
( cd "$TMPDIR_REAL" && "${SHA_CMD[@]}" "${ASSET}.sha256" >/dev/null ) \
  || { err "sha256 mismatch — aborting"; exit 1; }
ok "checksum ok"

mkdir -p "$TMPDIR_REAL/extract"
tar -xzf "$TMPDIR_REAL/$ASSET" -C "$TMPDIR_REAL/extract"

# ───────── stop / swap / start ─────────

SERVICE_WAS_ACTIVE="no"
if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet "$SERVICE"; then
  SERVICE_WAS_ACTIVE="yes"
  say "Stopping $SERVICE"
  asroot systemctl stop "$SERVICE"
else
  warn "$SERVICE service not active — assuming you're running the binary manually."
  warn "Stop the process yourself before continuing if it's still running."
  if ! confirm "Continue with the binary swap?" "Y"; then
    ok "Aborted by user."
    exit 0
  fi
fi

say "Swapping binary"
asroot mv -f "$INSTALL_DIR/steelpage" "$INSTALL_DIR/steelpage.prev"
asroot install -m 0755 "$TMPDIR_REAL/extract/steelpage" "$INSTALL_DIR/steelpage"

# Match ownership to the previous binary so systemd's User= keeps working.
prev_owner=$(stat -c '%U:%G' "$INSTALL_DIR/steelpage.prev" 2>/dev/null || stat -f '%Su:%Sg' "$INSTALL_DIR/steelpage.prev")
asroot chown "$prev_owner" "$INSTALL_DIR/steelpage"

say "Recording new version"
printf "%s\n" "$TAG" | asroot tee "$INSTALL_DIR/.steelpage-version" >/dev/null

if [ "$SERVICE_WAS_ACTIVE" = "yes" ]; then
  say "Starting $SERVICE"
  asroot systemctl start "$SERVICE"
  # Give it a moment to fail fast.
  sleep 1
  if systemctl is-active --quiet "$SERVICE"; then
    ok "$SERVICE is running."
  else
    err "$SERVICE failed to start after update."
    err "Rollback: sudo mv $INSTALL_DIR/steelpage.prev $INSTALL_DIR/steelpage && sudo systemctl start $SERVICE"
    err "Logs:     sudo journalctl -u $SERVICE -n 50 --no-pager"
    exit 1
  fi
fi

printf "\n"
ok "Updated ${CURRENT:-unknown} → $TAG"
printf "Previous binary kept at %s/steelpage.prev for rollback.\n" "$INSTALL_DIR"
