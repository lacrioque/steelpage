CREATE TABLE IF NOT EXISTS doc_links (
  src_path TEXT NOT NULL,
  dst_path TEXT NOT NULL,
  PRIMARY KEY (src_path, dst_path)
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS idx_doc_links_dst ON doc_links(dst_path);

CREATE TABLE IF NOT EXISTS doc_tags (
  path TEXT NOT NULL,
  tag  TEXT NOT NULL,
  PRIMARY KEY (path, tag)
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS idx_doc_tags_tag ON doc_tags(tag);
