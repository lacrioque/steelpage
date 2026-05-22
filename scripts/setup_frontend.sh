#!/usr/bin/env bash
# Install SPA dependencies. Safe to re-run; npm install is idempotent.
set -euo pipefail

cd "$(dirname "$0")/../frontend"

if ! command -v npm >/dev/null 2>&1; then
  echo "error: npm is not installed. Install Node.js 20+ first." >&2
  exit 1
fi

node_major=$(node -p 'process.versions.node.split(".")[0]')
if [ "$node_major" -lt 20 ]; then
  echo "error: Node.js 20 or newer is required (found v$(node -v))." >&2
  exit 1
fi

echo "==> Installing frontend dependencies"
npm install --no-audit --no-fund

echo ""
echo "Done."
echo "  Dev server: scripts/start_dev.sh"
echo "  Build SPA:  cd frontend && npm run build"
