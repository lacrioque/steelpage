#!/usr/bin/env bash
# Generate a hardened systemd unit for Steelpage. Writes the file into the
# current directory rather than /etc — installing it is a one-line move
# the operator runs as root after reviewing.
#
# Usage: scripts/add_systemd_service.sh [user] [install_dir] [content_dir]
set -euo pipefail

USER_NAME="${1:-steelpage}"
INSTALL_DIR="${2:-/opt/steelpage}"
CONTENT_DIR="${3:-/var/lib/steelpage/content}"
OUT="steelpage.service"

cat > "$OUT" <<EOF
[Unit]
Description=Steelpage — git-backed Markdown wiki
Documentation=https://github.com/lacrioque/steelpage
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${USER_NAME}
Group=${USER_NAME}
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/steelpage
Restart=on-failure
RestartSec=5s

# Sandbox — the binary only needs to read/write its own dir and the
# content repo. Loosen ReadWritePaths if you put the SQLite DB elsewhere.
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${INSTALL_DIR} ${CONTENT_DIR}
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

cat <<EOF

Wrote ${OUT} (user=${USER_NAME}, install=${INSTALL_DIR}, content=${CONTENT_DIR}).

To install:

  sudo useradd --system --home ${INSTALL_DIR} --shell /usr/sbin/nologin ${USER_NAME} || true
  sudo install -d -o ${USER_NAME} -g ${USER_NAME} ${INSTALL_DIR} ${CONTENT_DIR}
  sudo install -o ${USER_NAME} -g ${USER_NAME} -m 0755 ./steelpage ${INSTALL_DIR}/
  sudo install -o ${USER_NAME} -g ${USER_NAME} -m 0640 ./config.yaml ${INSTALL_DIR}/
  sudo mv ${OUT} /etc/systemd/system/
  sudo systemctl daemon-reload
  sudo systemctl enable --now steelpage

Status: sudo systemctl status steelpage
Logs:   sudo journalctl -u steelpage -f
EOF
