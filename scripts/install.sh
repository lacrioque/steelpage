#!/usr/bin/env bash
# Steelpage installer.
#
# Quickstart:
#   curl -fsSL https://raw.githubusercontent.com/lacrioque/steelpage/main/scripts/install.sh | bash
#   wget -qO- https://raw.githubusercontent.com/lacrioque/steelpage/main/scripts/install.sh | bash
#
# What it does (in order):
#   1. Detects OS/arch (linux/macOS, amd64/arm64).
#   2. Looks up the latest release on GitHub.
#   3. Downloads + sha256-verifies the matching tarball.
#   4. Walks the operator through the core config (host:port, base URL,
#      content repo path, bootstrap admin, anon-read).
#   5. Installs the binary + config.yaml into /opt/steelpage (override
#      with INSTALL_DIR=…).
#   6. Initializes the content git repo with a seed README.
#   7. Optionally generates + enables a hardened systemd unit (Linux).
#
# Privileges: runs as the calling user. Sudo is only invoked when writing
# to system paths (default /opt/steelpage, /etc/systemd/system, useradd).

set -euo pipefail

REPO="lacrioque/steelpage"
INSTALL_DIR="${INSTALL_DIR:-/opt/steelpage}"
SERVICE_USER="${SERVICE_USER:-steelpage}"
TMPDIR_REAL=$(mktemp -d)
trap 'rm -rf "$TMPDIR_REAL"' EXIT

# Colors. Honor NO_COLOR + non-tty.
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

# When piped from curl/wget, stdin is the script — read from the terminal.
TTY=/dev/tty
if [ ! -e "$TTY" ]; then
  err "no controlling terminal. Pipe the script through bash but make sure /dev/tty is available."
  exit 1
fi

prompt() {
  # prompt VAR "Question" [default]
  local var="$1" question="$2" default="${3:-}"
  local prompt_line answer
  if [ -n "$default" ]; then
    prompt_line="$question [$default]: "
  else
    prompt_line="$question: "
  fi
  printf "%s" "$prompt_line" >"$TTY"
  read -r answer <"$TTY"
  if [ -z "$answer" ] && [ -n "$default" ]; then
    answer="$default"
  fi
  printf -v "$var" "%s" "$answer"
}

prompt_secret() {
  local var="$1" question="$2" answer
  printf "%s: " "$question" >"$TTY"
  stty -echo <"$TTY"
  read -r answer <"$TTY"
  stty echo <"$TTY"
  printf "\n" >"$TTY"
  printf -v "$var" "%s" "$answer"
}

confirm() {
  # confirm "Question" [Y|N]
  local question="$1" default="${2:-Y}" answer
  local hint
  case "$default" in
    Y|y) hint="[Y/n]" ;;
    *)   hint="[y/N]" ;;
  esac
  printf "%s %s " "$question" "$hint" >"$TTY"
  read -r answer <"$TTY"
  if [ -z "$answer" ]; then
    answer="$default"
  fi
  case "$answer" in
    Y|y|yes|Yes|YES) return 0 ;;
    *) return 1 ;;
  esac
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || { err "missing required tool: $1"; exit 1; }
}

# sudo helper: passthrough when root, prepend sudo otherwise. If sudo is
# missing AND we're not root, fail loud.
maybe_sudo=()
if [ "$(id -u)" -ne 0 ]; then
  if command -v sudo >/dev/null 2>&1; then
    maybe_sudo=(sudo)
  fi
fi
asroot() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
  elif [ ${#maybe_sudo[@]} -gt 0 ]; then
    "${maybe_sudo[@]}" "$@"
  else
    err "this step needs root: $*"
    err "re-run with sudo, or set INSTALL_DIR to a path you own (e.g. INSTALL_DIR=\$HOME/steelpage)."
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

need_cmd curl
need_cmd tar
need_cmd git
if command -v sha256sum >/dev/null 2>&1; then
  SHA_CMD=(sha256sum -c)
elif command -v shasum >/dev/null 2>&1; then
  SHA_CMD=(shasum -a 256 -c)
else
  err "need sha256sum or shasum to verify the download"
  exit 1
fi

say "Detected $OS/$ARCH"

# ───────── resolve latest release ─────────

say "Looking up the latest release of $REPO"
RELEASE_JSON=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest")
TAG=$(printf "%s" "$RELEASE_JSON" | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name"[^"]*"([^"]+)".*/\1/')
if [ -z "$TAG" ]; then
  err "couldn't parse a tag from the GitHub API response"
  exit 1
fi
ok "latest tag: $TAG"

ASSET="steelpage-${TAG}-${OS}-${ARCH}.tar.gz"
ASSET_URL="https://github.com/$REPO/releases/download/$TAG/$ASSET"
SHA_URL="${ASSET_URL}.sha256"

say "Downloading $ASSET"
curl -fsSL --progress-bar "$ASSET_URL" -o "$TMPDIR_REAL/$ASSET"
curl -fsSL "$SHA_URL"   -o "$TMPDIR_REAL/${ASSET}.sha256"

say "Verifying checksum"
( cd "$TMPDIR_REAL" && "${SHA_CMD[@]}" "${ASSET}.sha256" >/dev/null ) \
  || { err "sha256 mismatch — abort"; exit 1; }
ok "checksum ok"

say "Extracting"
mkdir -p "$TMPDIR_REAL/extract"
tar -xzf "$TMPDIR_REAL/$ASSET" -C "$TMPDIR_REAL/extract"

# ───────── wizard ─────────

printf "\n%sCore configuration%s — press Enter to accept defaults.\n\n" "$C_BOLD" "$C_RESET" >"$TTY"

prompt INSTALL_DIR "Install directory" "$INSTALL_DIR"
prompt BIND       "Listening address" "127.0.0.1:8080"
prompt BASE_URL   "Public base URL (used in outgoing emails)" "http://localhost:8080"
prompt CONTENT_DIR "Content git repo directory" "$INSTALL_DIR/content"
prompt ADMIN_EMAIL "Bootstrap admin email" "admin@example.com"
prompt_secret ADMIN_PASSWORD "Bootstrap admin password (min 8 chars)"
while [ -z "$ADMIN_PASSWORD" ] || [ "${#ADMIN_PASSWORD}" -lt 8 ]; do
  warn "password must be at least 8 characters"
  prompt_secret ADMIN_PASSWORD "Bootstrap admin password (min 8 chars)"
done

ANON_DEFAULT="Y"
if confirm "Allow anonymous read access?" "$ANON_DEFAULT"; then
  ALLOW_ANON="true"
else
  ALLOW_ANON="false"
fi

if [ "$OS" = "linux" ]; then
  if confirm "Generate + enable a systemd service?" "Y"; then
    DO_SYSTEMD="yes"
  else
    DO_SYSTEMD="no"
  fi
else
  DO_SYSTEMD="no"
  warn "systemd is Linux-only — skipping service setup on $OS"
fi

# ───────── install files ─────────

say "Installing into $INSTALL_DIR"
asroot mkdir -p "$INSTALL_DIR" "$CONTENT_DIR"

asroot install -m 0755 "$TMPDIR_REAL/extract/steelpage" "$INSTALL_DIR/steelpage"
asroot install -m 0644 "$TMPDIR_REAL/extract/README.md" "$INSTALL_DIR/README.md" 2>/dev/null || true
asroot install -m 0644 "$TMPDIR_REAL/extract/LICENSE.md" "$INSTALL_DIR/LICENSE.md" 2>/dev/null || true

# Render config.yaml. We render to a tmp file as the current user, then
# move into place with sudo if needed.
CFG="$TMPDIR_REAL/config.yaml"
cat > "$CFG" <<EOF
# Generated by scripts/install.sh on $(date -u +%Y-%m-%dT%H:%M:%SZ).
# Live-editable values can also be changed from /admin → Settings.

repo:
  path: $CONTENT_DIR
  branch: main
  commit_author_name: Steelpage
  commit_author_email: steelpage@local
  auto_push: false
  push_remote: origin

server:
  bind: $BIND
  base_url: $BASE_URL

db:
  path: $INSTALL_DIR/steelpage.db

auth:
  mode: local
  allow_anonymous_read: $ALLOW_ANON
  local_enabled: true
  session:
    secure: $( [[ "$BASE_URL" == https://* ]] && echo "true" || echo "false" )
  oidc:
    enabled: false
    label: ""
    issuer_url: ""
    client_id: ""
    client_secret: ""
    redirect_url: ""
    scopes: ["openid", "email", "profile"]

render:
  allow_raw_html: false
  mermaid: true
  code_highlighting: true
  sanitize_html: true

search:
  engine: sqlite_fts5

frontend:
  embedded_dist: ""

email:
  host: ""
  port: 0
  encryption: starttls
  username: ""
  password: ""
  from_address: ""
  from_name: "Steelpage"
  reply_to: ""
  insecure_skip_verify: false
EOF

asroot install -m 0640 "$CFG" "$INSTALL_DIR/config.yaml"

# ───────── content repo ─────────

if [ ! -d "$CONTENT_DIR/.git" ]; then
  say "Initialising content repo at $CONTENT_DIR"
  asroot bash -c "
    cd '$CONTENT_DIR' && \
    git init -b main >/dev/null && \
    printf '# Welcome to Steelpage\n\nThis is your archive root. Edit me from the UI.\n' > README.md && \
    git -c user.name='Steelpage' -c user.email='steelpage@local' add README.md && \
    git -c user.name='Steelpage' -c user.email='steelpage@local' commit -m 'init' >/dev/null
  "
  ok "content repo ready"
else
  ok "content repo already initialized — leaving it alone"
fi

# ───────── systemd ─────────

if [ "$DO_SYSTEMD" = "yes" ]; then
  say "Creating service user '$SERVICE_USER' (if missing)"
  if ! id -u "$SERVICE_USER" >/dev/null 2>&1; then
    asroot useradd --system --home "$INSTALL_DIR" --shell /usr/sbin/nologin "$SERVICE_USER"
  else
    ok "user $SERVICE_USER already exists"
  fi

  say "Re-owning $INSTALL_DIR + $CONTENT_DIR for $SERVICE_USER"
  asroot chown -R "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR" "$CONTENT_DIR"

  UNIT="$TMPDIR_REAL/steelpage.service"
  cat > "$UNIT" <<EOF
[Unit]
Description=Steelpage — git-backed Markdown wiki
Documentation=https://github.com/$REPO
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_USER
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/steelpage
Restart=on-failure
RestartSec=5s

# Bootstrap admin — runs once. After your first sign-in works, edit this
# unit and remove the Environment line, then daemon-reload + restart.
Environment=STEELPAGE_BOOTSTRAP_ADMIN=$ADMIN_EMAIL:$ADMIN_PASSWORD

# Sandbox
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$INSTALL_DIR $CONTENT_DIR
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
MemoryDenyWriteExecute=true
RestrictRealtime=true
RestrictNamespaces=true

[Install]
WantedBy=multi-user.target
EOF

  say "Installing unit at /etc/systemd/system/steelpage.service"
  asroot install -m 0644 "$UNIT" /etc/systemd/system/steelpage.service
  asroot systemctl daemon-reload
  asroot systemctl enable --now steelpage
  ok "steelpage service enabled and started"
  printf "\n"
  printf "  Status: %ssudo systemctl status steelpage%s\n" "$C_DIM" "$C_RESET"
  printf "  Logs:   %ssudo journalctl -u steelpage -f%s\n" "$C_DIM" "$C_RESET"
else
  printf "\n"
  printf "%sTo start manually:%s\n" "$C_BOLD" "$C_RESET"
  printf "  cd %s\n" "$INSTALL_DIR"
  printf "  STEELPAGE_BOOTSTRAP_ADMIN=%s:<password> ./steelpage\n" "$ADMIN_EMAIL"
fi

printf "\n"
ok "Steelpage $TAG installed in $INSTALL_DIR"
printf "Open %s in your browser once the service is up.\n" "$BASE_URL"
