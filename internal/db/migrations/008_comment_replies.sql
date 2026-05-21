ALTER TABLE comments ADD COLUMN reply_to_id INTEGER REFERENCES comments(id) ON DELETE SET NULL;
CREATE INDEX idx_comments_reply_to ON comments(reply_to_id);
