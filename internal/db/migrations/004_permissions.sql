CREATE TABLE permissions (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  path_glob     TEXT NOT NULL,
  subject_type  TEXT NOT NULL,
  subject_value TEXT NOT NULL DEFAULT '',
  permission    TEXT NOT NULL,
  created_at    TEXT NOT NULL
);

CREATE INDEX idx_permissions_glob ON permissions(path_glob);

-- Unique on the (glob, subject, permission) tuple so duplicate inserts are
-- rejected at the DB layer rather than the API.
CREATE UNIQUE INDEX uniq_permissions_rule
  ON permissions(path_glob, subject_type, subject_value, permission);
