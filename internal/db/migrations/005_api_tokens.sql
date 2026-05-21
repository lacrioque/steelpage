CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  token_hash   TEXT NOT NULL UNIQUE,
  scopes       TEXT NOT NULL DEFAULT '[]',
  expires_at   TEXT,
  last_used_at TEXT,
  created_at   TEXT NOT NULL
);

CREATE INDEX idx_api_tokens_user ON api_tokens(user_id);
