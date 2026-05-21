package search

import (
	"database/sql"
	"fmt"
	"strings"
)

type Result struct {
	Path           string  `json:"path"`
	Title          string  `json:"title"`
	HeadingSnippet string  `json:"heading_snippet"`
	BodySnippet    string  `json:"body_snippet"`
	Rank           float64 `json:"rank"`
}

type Store struct {
	DB *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{DB: db} }

const ftsQuery = `
SELECT
  path,
  title,
  snippet(documents_fts, 2, '<mark>', '</mark>', '…', 12) AS heading_snippet,
  snippet(documents_fts, 3, '<mark>', '</mark>', '…', 24) AS body_snippet,
  bm25(documents_fts, 10, 5, 1, 5) AS rank
FROM documents_fts
WHERE documents_fts MATCH ?
ORDER BY rank
LIMIT ?
`

// Search executes a FTS5 MATCH against the documents index. The query is
// sanitized to keep stray quotes from breaking syntax — we always wrap each
// token in quotes (so the user can search for things like "auth.mode" without
// triggering syntax errors) and use OR between tokens for forgiving matches.
func (s *Store) Search(q string, limit int) ([]Result, error) {
	clean := sanitize(q)
	if clean == "" {
		return []Result{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.DB.Query(ftsQuery, clean, limit)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	out := []Result{}
	for rows.Next() {
		var r Result
		if err := rows.Scan(&r.Path, &r.Title, &r.HeadingSnippet, &r.BodySnippet, &r.Rank); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// sanitize splits on whitespace, drops empty tokens, and wraps each in double
// quotes. Returns a single OR-joined FTS5 string, so "foo bar" → '"foo" OR "bar"'.
// Empty input returns "".
func sanitize(q string) string {
	parts := strings.Fields(q)
	if len(parts) == 0 {
		return ""
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		// FTS5 strings can contain anything except an embedded double quote,
		// which is escaped by doubling.
		escaped := strings.ReplaceAll(p, `"`, `""`)
		out = append(out, `"`+escaped+`"`)
	}
	return strings.Join(out, " OR ")
}
