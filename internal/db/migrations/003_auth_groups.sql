-- Recreate users with full auth columns. SQLite can't drop UNIQUE on
-- display_name in place, so we make a new table and copy rows.
CREATE TABLE users_new (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  email         TEXT,
  display_name  TEXT NOT NULL,
  password_hash TEXT,
  oidc_provider TEXT,
  oidc_subject  TEXT,
  role          TEXT NOT NULL DEFAULT 'user',
  created_at    TEXT NOT NULL
);

INSERT INTO users_new (id, display_name, created_at)
SELECT id, display_name, created_at FROM users;

DROP TABLE users;
ALTER TABLE users_new RENAME TO users;

CREATE UNIQUE INDEX uniq_users_email
  ON users(email)
  WHERE email IS NOT NULL;

CREATE UNIQUE INDEX uniq_users_oidc
  ON users(oidc_provider, oidc_subject)
  WHERE oidc_provider IS NOT NULL;

-- Groups + membership.
CREATE TABLE groups (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL UNIQUE,
  description TEXT,
  created_at  TEXT NOT NULL
);

CREATE TABLE user_groups (
  user_id  INTEGER NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
  group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  PRIMARY KEY (user_id, group_id)
);

-- scs sqlite3store schema. The store expects exactly these columns/types.
CREATE TABLE sessions (
  token  TEXT PRIMARY KEY,
  data   BLOB NOT NULL,
  expiry REAL NOT NULL
);

CREATE INDEX sessions_expiry_idx ON sessions(expiry);
