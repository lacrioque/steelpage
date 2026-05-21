CREATE TABLE IF NOT EXISTS documents (
  path        TEXT PRIMARY KEY,
  title       TEXT,
  sha         TEXT,
  headings    TEXT,
  tags        TEXT,
  body        TEXT,
  indexed_at  TEXT NOT NULL
);

CREATE VIRTUAL TABLE IF NOT EXISTS documents_fts USING fts5(
  path UNINDEXED,
  title,
  headings,
  body,
  tags,
  tokenize = 'unicode61 remove_diacritics 2'
);
