# scripts/

Operational helpers. Run them from the project root.

| Script | Purpose |
|---|---|
| `install.sh` | Curl/wget-able installer. Detects OS+arch, downloads the latest release tarball, sha256-verifies, then wizards through host/port, base URL, content path, and bootstrap admin. Installs to `/opt/steelpage` (override with `INSTALL_DIR=…`); on Linux can also write+enable a hardened systemd unit. Uses sudo only when writing to system paths. |
| `setup_frontend.sh` | `npm install` the SPA deps (checks for Node 20+). Safe to re-run. |
| `start_dev.sh` | Launches `go run ./cmd/steelpage` in the background and the Vite dev server in the foreground. Reads `server.bind` from `config.yaml` to know when the backend is up. Backend logs go to `scripts/.dev-backend.log`. Ctrl-C stops both. |
| `add_systemd_service.sh [user] [install_dir] [content_dir]` | Generates a hardened `steelpage.service` unit file in the current directory and prints the install commands. Doesn't touch `/etc` — that's an explicit operator step. Same template the installer uses. |

## One-line install

```bash
curl -fsSL https://raw.githubusercontent.com/lacrioque/steelpage/main/scripts/install.sh | bash
# or
wget -qO- https://raw.githubusercontent.com/lacrioque/steelpage/main/scripts/install.sh | bash
```

Environment overrides: `INSTALL_DIR`, `SERVICE_USER`, `NO_COLOR`. Reads prompts from `/dev/tty`, so piping through `bash` works fine.

All scripts are `set -euo pipefail` and resolve their own location, so they work from any CWD.
