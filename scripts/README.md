# scripts/

Operational helpers. Run them from the project root.

| Script | Purpose |
|---|---|
| `setup_frontend.sh` | `npm install` the SPA deps (checks for Node 20+). Safe to re-run. |
| `start_dev.sh` | Launches `go run ./cmd/steelpage` in the background and the Vite dev server in the foreground. Reads `server.bind` from `config.yaml` to know when the backend is up. Backend logs go to `scripts/.dev-backend.log`. Ctrl-C stops both. |
| `add_systemd_service.sh [user] [install_dir] [content_dir]` | Generates a hardened `steelpage.service` unit file in the current directory and prints the install commands. Doesn't touch `/etc` — that's an explicit operator step. |

All scripts are `set -euo pipefail` and resolve their own location, so they work from any CWD.
