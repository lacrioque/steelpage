#!/usr/bin/env bash
# Run the Go backend + Vite dev server side-by-side. Backend writes its log
# to scripts/.dev-backend.log; the SPA HMR runs in the foreground. Ctrl-C
# stops both.
set -euo pipefail

cd "$(dirname "$0")/.."

if ! command -v go >/dev/null 2>&1; then
  echo "error: go toolchain not found." >&2
  exit 1
fi
if [ ! -d frontend/node_modules ]; then
  echo "error: frontend/node_modules missing. Run scripts/setup_frontend.sh first." >&2
  exit 1
fi
if [ ! -f config.yaml ]; then
  echo "==> No config.yaml; copying config.example.yaml"
  cp config.example.yaml config.yaml
fi

BACKEND_LOG=scripts/.dev-backend.log
: >"$BACKEND_LOG"

# Bind we'll poll to know when the backend is ready. Reading it from
# config.yaml keeps the script honest if the operator moved the port.
BIND=$(awk -F'"' '/^[[:space:]]*bind:/ {print $2; exit}' config.yaml || true)
BIND=${BIND:-127.0.0.1:8080}

echo "==> Starting backend  (logs → $BACKEND_LOG, bind $BIND)"
go run ./cmd/steelpage >>"$BACKEND_LOG" 2>&1 &
BACKEND_PID=$!

cleanup() {
  echo ""
  echo "==> Stopping backend ($BACKEND_PID)"
  kill "$BACKEND_PID" 2>/dev/null || true
  wait "$BACKEND_PID" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

# Wait up to 30 s for the backend to answer. Bail with the captured log
# if it never comes up — saves the operator from staring at a blank Vite.
for _ in $(seq 1 60); do
  if curl -fsS "http://$BIND/api/auth/providers" >/dev/null 2>&1; then
    echo "==> Backend ready"
    break
  fi
  if ! kill -0 "$BACKEND_PID" 2>/dev/null; then
    echo "error: backend exited before becoming ready. Tail of log:" >&2
    tail -20 "$BACKEND_LOG" >&2
    exit 1
  fi
  sleep 0.5
done

echo "==> Starting Vite dev server (http://127.0.0.1:5173)"
cd frontend
exec npm run dev
