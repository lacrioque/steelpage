# Live Config Editing — Design Plan

Ticket: `steelpage-dap`

## Why

The `/admin → Settings` tab currently displays the running configuration read-only. Operators have to SSH in, edit `config.yaml`, and restart Steelpage for the smallest tweak — annoying for ops-friendly settings like `auto_push`, OIDC client IDs, or the SMTP host. This plan makes hot-swappable fields editable from the admin UI while keeping the things that genuinely need a restart safely in `config.yaml`.

## Locked-in design choices

| Question | Decision |
|---|---|
| Where do runtime values live? | **Hybrid**: `config.yaml` is the cold-start source of truth; a DB `config_overrides` table records admin edits. Effective config = YAML ⨯ overrides (overrides win). |
| How are secrets handled? | **Write-only, set/unset indicator.** The UI never shows the stored value. A green "configured" badge marks fields that have a value. Submitting empty keeps the old value; submitting a new value overwrites. |
| Restart-required fields (`server.bind`, `db.path`, `repo.path`, `repo.branch`, `frontend.embedded_dist`)? | **Read-only in UI, edit via `config.yaml` only.** No "pending restart" complexity. The Settings tab labels these as YAML-only with a small lock icon. |
| Audit trail? | **`config_audit` DB table.** Rows: `(id, actor_user_id, key, old_value_redacted, new_value_redacted, at)`. Secrets render as `***`. A "History" sub-section in the Settings tab lists recent edits. |

## Schema (`internal/db/migrations/009_config_overrides.sql`)

```sql
CREATE TABLE config_overrides (
  key        TEXT PRIMARY KEY,    -- e.g. "auth.oidc.enabled", "email.host"
  value      TEXT NOT NULL,       -- JSON-encoded so we can preserve types
  updated_at TEXT NOT NULL,
  updated_by INTEGER REFERENCES users(id) ON DELETE SET NULL
);

CREATE TABLE config_audit (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  actor_user_id   INTEGER REFERENCES users(id) ON DELETE SET NULL,
  key             TEXT NOT NULL,
  old_value       TEXT,           -- redacted to "***" for sensitive keys
  new_value       TEXT,           -- same
  at              TEXT NOT NULL
);
CREATE INDEX idx_config_audit_at ON config_audit(at DESC);
```

Values are stored as JSON strings so booleans, ints, and string slices round-trip cleanly. The override key is the **dotted path** into the Config struct (e.g. `"email.encryption"`).

## Backend architecture

### New package: `internal/configsvc/`

Holds the live-config state. Replaces direct `cfg *config.Config` access for fields that should be live-editable.

```go
type Service struct {
    base       *config.Config           // immutable, loaded once from YAML
    overrides  map[string]json.RawMessage
    db         *sql.DB
    listeners  []listener
    mu         sync.RWMutex
}

type listener struct {
    prefix string                                    // e.g. "email." or "auth.oidc."
    fn     func(snapshot *config.Config, changed []string)
}

// Snapshot returns the current effective config (base + overrides applied).
func (s *Service) Snapshot() *config.Config { ... }

// Set applies an override, writes to DB + audit, then notifies listeners.
func (s *Service) Set(actorID int64, key string, value any) error { ... }

// Unset removes an override, falling back to the YAML value.
func (s *Service) Unset(actorID int64, key string) error { ... }

// Subscribe registers a callback that fires whenever a key with the given
// prefix changes. The listener gets the new snapshot and the list of
// changed keys so it can decide what to rebuild.
func (s *Service) Subscribe(prefix string, fn func(*config.Config, []string)) { ... }

// Schema returns the editable-fields manifest the frontend uses to render
// the Settings tab (see "Editable fields manifest" below).
func (s *Service) Schema() FieldSchema { ... }
```

### Hot-reload integration

Subsystems that cache derived state subscribe at startup:

| Subsystem | Subscribes to | What it does on change |
|---|---|---|
| `mailer` | `email.*` | Rebuilds `Mailer` with the new SMTP settings; old one is GC'd. |
| `auth` (goth) | `auth.oidc.*` | Re-registers the goth `openidConnect` provider (or unregisters when `enabled` flips to false). |
| `render` | `render.*` | Rebuilds the Goldmark instance + sanitizer. |
| Permission/anon-read checks | n/a | Already read `cfg.Auth.AllowAnonymousRead` per-request; no subscribe needed. |
| Git commit author + auto-push | n/a | Read per-request. |

A subsystem reload that fails (e.g., bad OIDC issuer URL) logs the error and keeps the previous instance running — the admin sees the failure in the response and can revert.

### `api.API` evolution

The `API` struct currently holds `Cfg *config.Config` directly. New rule: handlers read live values via `a.Configsvc.Snapshot()` for any field that's live-editable. The `Cfg` reference stays for restart-required fields (bind, db.path, repo.path, etc.).

A small helper keeps call sites tidy:

```go
func (a *API) cfg() *config.Config {
    return a.Configsvc.Snapshot()
}
```

### Editable-fields manifest

A single map drives both the API and the UI. Each entry declares the type, secrecy, and reload class:

```go
type Field struct {
    Key      string  // "email.host"
    Type     string  // "string" | "int" | "bool" | "enum" | "stringSlice"
    Enum     []string
    Sensitive bool   // hides current value in API responses
    ReadOnly bool    // restart-required → not editable in UI
    Group    string  // "Email", "OIDC", "Repo", etc.
    Order    int
}
```

The manifest is the **single place** that knows what's settable. Anything not in it is implicitly read-only, even if it's in `Config`. New live fields = add an entry.

### API surface

| Endpoint | Behavior |
|---|---|
| `GET /api/admin/config/schema` | Returns the editable-fields manifest (key, type, group, etc.). |
| `GET /api/admin/config/effective` | Returns the effective config with sensitive values redacted (`***` or `null`) and `has_value: bool` for each sensitive field. |
| `PATCH /api/admin/config` | `{ key, value }` (or `{ key, unset: true }`). Validates, writes override, audits, notifies. |
| `GET /api/admin/config/audit?limit=50` | Recent audit rows, joined with user display names. |
| `GET /api/admin/config/export` | Returns the current effective config as a YAML document (Content-Disposition: attachment so the admin can save it to disk). |

All four are gated by `RequireAdmin`.

### Validation

Per-field validators run before writing:

- `int`: range check (e.g., `email.port` ∈ [1, 65535]).
- `enum`: must match `Enum` slice (e.g., `email.encryption` ∈ {none, starttls, tls}).
- `string`: optional regex (e.g., URL validators for `auth.oidc.issuer_url`).
- `stringSlice`: each element validated.

Bad input → 400 with an error message that names the field.

## Frontend

### Settings tab restructure

The Settings tab today is one long `StructuredList`. New layout:

1. **Email (SMTP)** group (already partially built)
2. **OIDC** group
3. **Repo / Git** group (auto_push, push_remote, commit author defaults, base_url)
4. **Auth flags** group (allow_anonymous_read, local_enabled, session.secure)
5. **Render** group
6. **History** (audit log table)
7. **Locked (config.yaml only)** group — read-only display of restart-required fields with a "lock" icon

Each group is a Carbon `Tile` with a `Form` inside. Each field renders based on `Type` from the schema:

| Type | Component | Notes |
|---|---|---|
| `string` | `TextInput` | placeholder = current YAML default |
| `string` (sensitive) | `PasswordInput` + green "configured" tag | empty submit = keep; new = overwrite; unset button |
| `int` | `NumberInput` | `min`/`max` from validator |
| `bool` | `Toggle` | inline label |
| `enum` | `Select` + `SelectItem`s | from `field.Enum` |
| `stringSlice` | `MultiSelect` or comma-input | depending on enum-ness |

### Per-field UX

- **"Inherits YAML" badge** when no override is set — tells the operator "you're seeing the default".
- **"Override" badge** when an override exists — and a small "↺ revert to default" button.
- **Save button** is per-group (one bulk save per Tile) — fewer round trips, fewer audit rows.
- **Last edited by** small line under each override (`updated_at` + actor display name).

### History sub-section

A Carbon `DataTable` reading from `/api/admin/config/audit`:

| When | Who | Key | Old → New |
|---|---|---|---|
| 12:04:33 | Markus | `email.host` | (default) → `smtp.fastmail.com` |
| 11:58:01 | Markus | `auth.oidc.enabled` | `false` → `true` |
| 11:55:12 | Markus | `email.password` | `***` → `***` |

### Export

A "Download current config as YAML" button in the History section. The download contains the *effective* config (YAML defaults + overrides applied). Operators paste it back into `config.yaml` when they want to bake overrides in.

## Field-by-field handling

### Live-editable

- `repo.commit_author_name`, `repo.commit_author_email`, `repo.auto_push`, `repo.push_remote`
- `server.base_url`
- `auth.mode` (informational), `allow_anonymous_read`, `local_enabled`
- `auth.session.secure`
- `auth.oidc.*` (whole block; client_secret is sensitive)
- `render.*`
- `search.engine` (single string for now)
- `email.*` (host, port, username, password\*, encryption, from_address, from_name, reply_to, insecure_skip_verify)
- \* `email.password` and `auth.oidc.client_secret` are sensitive

### Read-only (`config.yaml` edit + restart)

- `server.bind`
- `db.path`
- `repo.path`
- `repo.branch`
- `frontend.embedded_dist`

### Reasons for the split

| Field | Why YAML-only |
|---|---|
| `server.bind` | Changing the listening address mid-flight would break the connection that submitted the change. |
| `db.path` | Swapping SQLite files at runtime means re-opening a different DB; everything in the current connection state is gone. |
| `repo.path` | Re-indexing search, re-validating permissions, rewriting comments paths — a real migration, not a config tweak. |
| `repo.branch` | Branches imply different histories; switching mid-session means commits go to the wrong place. |
| `frontend.embedded_dist` | Embedded; no live alternative location. |

## Verification (acceptance criteria when this lands)

1. **Migration applies cleanly** on existing DBs.
2. **Effective config endpoint** returns the YAML values when no overrides exist; secrets read `null` with `has_value: false`.
3. **Override flow** — `PATCH /api/admin/config {"key":"email.host","value":"smtp.test"}` updates `email.host`, audits the change, and the next `GET /api/admin/config/effective` reflects it.
4. **Subsystem reload** — after setting `email.host` to a working host, `POST /api/admin/mailer/test` succeeds without a process restart.
5. **OIDC live-toggle** — flip `auth.oidc.enabled` to true with valid issuer + client; `GET /api/auth/providers` immediately lists the provider.
6. **Sensitive write-only** — `GET /api/admin/config/effective` always returns `email.password: null` even when set; `PATCH` with empty string keeps the existing value; `PATCH` with `{unset: true}` clears.
7. **Audit** — `GET /api/admin/config/audit` lists the operations with actor + redacted values for sensitive keys.
8. **Read-only fields** — attempting to `PATCH` `server.bind` → 400 `"field is config.yaml only"`. UI shows lock icon and disables input.
9. **Export** — `GET /api/admin/config/export` returns a valid YAML document that, when used as `config.yaml`, reproduces the current effective config on next boot.
10. **Failure-keeps-running** — `PATCH` with a bad OIDC issuer (e.g., 404) returns 400, leaves the previous provider registered, and the audit row is not written.

## Out of scope for v1 (file as follow-ups when needed)

- **Diff view between revisions** in the audit log.
- **Bulk import** of an external YAML into overrides (operators can keep doing the manual edit + restart for now).
- **Per-user-visible config** — there's no notion of "tenant" or "per-org" config yet; all overrides are global.
- **Config validation across fields** (e.g., refusing `auth.oidc.enabled=true` with empty `client_id`) — single-field validation only in v1; cross-field rules can be added when issues surface.
- **Schema versioning** — when a future migration renames a field, stale rows in `config_overrides` need a migration step. Defer until the first rename actually happens.
- **Live-reload of `server.base_url`** for in-flight email links — currently fine since URLs are minted per-message.

## Critical files when implementing

| File | Purpose |
|---|---|
| `internal/db/migrations/009_config_overrides.sql` | New schema. |
| `internal/configsvc/configsvc.go` | New package: Service + Snapshot + Set + Subscribe. |
| `internal/configsvc/schema.go` | Editable-fields manifest with type/sensitivity/group. |
| `internal/configsvc/validators.go` | Per-type validators. |
| `internal/api/admin_config.go` | New: schema / effective / patch / audit / export endpoints. |
| `internal/api/api.go` | API struct gains `Configsvc *configsvc.Service`; handlers refactor to read live where applicable. |
| `internal/mailer/mailer.go` | Gains a `Reload(cfg config.Email)` method; subscribes at boot. |
| `internal/auth/auth.go` | OIDC provider re-registration helper; subscribes at boot. |
| `internal/render/render.go` | Renderer becomes mutable: a small wrapper that holds a Goldmark instance + sanitizer + can swap them on change. |
| `cmd/steelpage/main.go` | Build `configsvc.Service`, register all subsystem listeners. |
| `frontend/src/routes/AdminView.svelte` | Settings tab refactor: schema-driven groups + per-field components. |
| `frontend/src/lib/admin-api.ts` | `getConfigSchema`, `getConfigEffective`, `patchConfig`, `unsetConfig`, `getConfigAudit`, `exportConfig`. |
| `frontend/src/lib/locales/{en,de}.json` | Group titles + per-field labels (or fetched from a small bundled label map). |

## Sizing & sequence

This is a **medium** implementation, not a small one — roughly:

- Backend: ~600–800 lines of new Go (configsvc + handlers + validators + subsystem reload hooks).
- Frontend: ~400–600 lines (schema-driven form + history table + new admin-api functions).
- 1 Go test file for configsvc (snapshot, set, unset, subscribe, redaction).

Suggested implementation order in a future session:

1. Migration + `configsvc` package + Snapshot/Set/Unset/Subscribe + tests.
2. Wire `configsvc` into `api.API`; keep all handlers reading from it.
3. Schema manifest + validators.
4. API endpoints (schema, effective, patch, audit, export).
5. Subsystem reload for **one** subsystem (recommend: mailer — easy to test via the existing "send test email" button).
6. Settings tab refactor — schema-driven rendering.
7. Add OIDC + render reloads.
8. History table + export button.

That's the plan. Two flag fields to revisit if scope shifts: (a) per-tenant config in the future, (b) richer cross-field validation as live editing reveals foot-guns.
