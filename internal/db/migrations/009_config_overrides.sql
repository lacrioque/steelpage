CREATE TABLE config_overrides (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  updated_by INTEGER REFERENCES users(id) ON DELETE SET NULL
);

CREATE TABLE config_audit (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  actor_user_id   INTEGER REFERENCES users(id) ON DELETE SET NULL,
  key             TEXT NOT NULL,
  old_value       TEXT,
  new_value       TEXT,
  at              TEXT NOT NULL
);

CREATE INDEX idx_config_audit_at ON config_audit(at DESC);
