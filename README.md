<p align="center">
  <img src="frontend/public/logo.png" alt="Steelpage" width="160">
</p>

# Steelpage

> Git-backed Markdown wiki: backend-renders every page, anchors comments to source lines, ships an `?botready=1` JSON for AI agents, and gates everything with OIDC/MFA + path permissions. One Go binary, one SQLite DB, one Svelte/Carbon SPA.

Markdown is the source of truth. Git records every edit. SQLite holds the live context (comments, search index, sessions, tokens, permissions, config overrides). The Go backend is the canonical Markdown → HTML renderer; the Svelte SPA is just the cockpit.

---

## Features

- **Backend Markdown rendering** with Goldmark + Chroma syntax highlighting + bluemonday HTML sanitization. Mermaid blocks pass through and render client-side.
- **Git as the audit log** — every save becomes a commit authored by the signed-in user. Optional `auto_push` to a remote; admins can also trigger pull-rebase / push manually from `/admin → Settings`.
- **Line-anchored comments** with replies and a fuzzy re-anchor ladder on save (`exact line → ±10 line scan → fuzzy match → orphan`). A CodeMirror gutter marks lines with active comments.
- **Full-text search** via SQLite FTS5 with BM25-weighted snippets; tokenizer `unicode61` handles umlauts.
- **Auth** — local accounts (bcrypt), generic OIDC, TOTP MFA, cookie sessions backed by SQLite. Email verification + password reset over SMTP.
- **Path-based permissions** with doublestar globs. Subject types: `anonymous`, `authenticated`, `role:<role>`, `group:<name>`, `user:<id>`. Permissions imply lower ones (`write > comment > read`).
- **API tokens** — long-lived bearer tokens (read/comment/write, optionally path-scoped) for headless agents and per-page share links.
- **Live config editing** — admins edit runtime settings from `/admin → Settings`; subsystems hot-reload without restart. YAML stays the cold-start source of truth.
- **Bot-ready output** at `/docs/<path>?botready=1` for enriched Markdown, or `?botready=1&format=json` for structured JSON consumption by AI agents.
- **i18n** — English and German included.

---

## Install (one-liner)

```bash
curl -fsSL https://raw.githubusercontent.com/lacrioque/steelpage/main/scripts/install.sh | bash
```

Walks you through host/port, base URL, content repo path, and bootstrap admin, then (on Linux) drops a hardened systemd unit. Defaults install to `/opt/steelpage` — set `INSTALL_DIR=…` to override.

To update an existing install to the latest release:

```bash
curl -fsSL https://raw.githubusercontent.com/lacrioque/steelpage/main/scripts/update.sh | bash
```

Stops the service, swaps the binary (old one kept as `steelpage.prev`), restarts. `config.yaml` and content are never touched.

## Quick start (from source)

```bash
# 1. Clone the source
git clone https://github.com/lacrioque/steelpage.git
cd steelpage

# 2. Create the content repo (your Markdown lives here, separate from the source)
mkdir -p ../steelpage-content
git -C ../steelpage-content init -b main
cp content.example/README.md ../steelpage-content/
git -C ../steelpage-content add README.md
git -C ../steelpage-content -c user.name=Steelpage -c user.email=steelpage@local commit -m "init"

# 3. Configure
cp config.example.yaml config.yaml
# (the defaults work; edit repo.path if the content repo isn't a sibling)

# 4. Build (compiles the SPA + the Go binary, embedding the SPA)
make build

# 5. Bootstrap an admin user the first time and run
STEELPAGE_BOOTSTRAP_ADMIN=admin@example.com:hunter2hunter2 ./steelpage
```

Open `http://127.0.0.1:8080/` and sign in as `admin@example.com / hunter2hunter2`.

---

## Configuration

`config.example.yaml` is the full reference. Most live-editable fields are also available under `/admin → Settings`. A minimal config:

```yaml
repo:
  path: ../steelpage-content    # separate git repo for your Markdown
  branch: main
  commit_author_name: Steelpage # fallback when no user is signed in
  commit_author_email: steelpage@local
  auto_push: false              # push to remote after each commit
  push_remote: origin

server:
  bind: 127.0.0.1:8080
  base_url: http://localhost:8080  # used to build links inside emails

db:
  path: ./steelpage.db

auth:
  mode: local
  allow_anonymous_read: true    # guests can read; writes still require auth
  local_enabled: true
  session:
    secure: true                # set false only for plain HTTP dev
  oidc:
    enabled: false              # flip to true + fill below for SSO
    label: "Identity provider"
    issuer_url: "https://auth.example.com/application/o/steelpage/"
    client_id: ""
    client_secret: ""
    redirect_url: "https://docs.example.com/api/auth/oidc/callback"
    scopes: ["openid", "email", "profile"]

email:
  host: ""                      # leave empty to disable outgoing mail
  port: 0                       # 0 → defaults from encryption
  encryption: starttls          # none | starttls | tls
  username: ""
  password: ""
  from_address: ""
  from_name: "Steelpage"

render:
  allow_raw_html: false
  mermaid: true
  code_highlighting: true
  sanitize_html: true
```

### Restart-required vs live-editable

Most fields can be edited from `/admin → Settings` and apply immediately (mailer, repo flags, anon-read, etc.). A handful are locked to `config.yaml` because changing them mid-flight is dangerous:

- `server.bind` (the listening address that just served your request)
- `db.path` (the SQLite file we're actively using)
- `repo.path`, `repo.branch` (which content repo we're editing)
- `frontend.embedded_dist` (where the embedded SPA lives)

Those are flagged with a 🔒 in the UI.

### Bootstrap admin via env var

`STEELPAGE_BOOTSTRAP_ADMIN=email:password ./steelpage` creates an admin if no admin user exists yet. Safe to leave in your supervisor script — it's a no-op once an admin is present.

### Locked out of MFA?

Admins can reset another user's MFA from `/admin → Users`. If the only admin lost their authenticator, drop into SQLite directly:

```bash
sqlite3 steelpage.db "UPDATE users SET totp_secret=NULL, totp_enabled_at=NULL WHERE id=1"
```

---

## Development

```bash
# Backend, live
go run ./cmd/steelpage

# Frontend with HMR (in another terminal)
cd frontend && npm install && npm run dev
# → http://127.0.0.1:5173 (proxies /api and /docs to :8080)

# Tests
go test ./...
```

---

## Architecture

```
config.yaml ──┐
              │
              ▼
   ┌──────────────────────────────────────────────────────┐
   │  Go binary                                           │
   │   • chi router + scs session manager                 │
   │   • goldmark renderer (sanitized)                    │
   │   • git CLI (commit, push, pull --rebase, history)   │
   │   • SQLite:                                          │
   │     users, sessions, comments, documents_fts,        │
   │     api_tokens, permissions, config_overrides …      │
   │   • SMTP mailer (password reset / email verify)      │
   │   • embedded Svelte / Carbon SPA                     │
   └──────────────────────────────────────────────────────┘
              │
              ▼
   ../steelpage-content/   (separate git repo of Markdown)
```

The content directory is its own git repository — Steelpage doesn't manage the source tree, just the docs. That keeps `git log` / `git diff` honest as an audit trail and lets operators clone, fork, or push the content repo independently.

---

## License

[AGPL-3.0](./LICENSE.md). Modify and self-host freely; if you operate Steelpage as a network service, share your modifications back with your users under the same terms.
