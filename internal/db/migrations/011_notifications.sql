-- Notifications for @mentions and replies, plus per-user email preferences.
-- email_on_mention defaults ON, email_on_response defaults OFF.

ALTER TABLE users ADD COLUMN email_on_mention  INTEGER NOT NULL DEFAULT 1;
ALTER TABLE users ADD COLUMN email_on_response INTEGER NOT NULL DEFAULT 0;

CREATE TABLE notifications (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  recipient_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  actor_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind         TEXT    NOT NULL,            -- 'mention' | 'reply'
  comment_id   INTEGER NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
  path         TEXT    NOT NULL,            -- document path the comment lives on
  created_at   TEXT    NOT NULL,
  read_at      TEXT
);

CREATE INDEX idx_notifications_recipient ON notifications(recipient_id, read_at, id DESC);
