package search

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrNotIndexed is returned by RelatedTo when the source doc is unknown.
var ErrNotIndexed = errors.New("document not indexed")

type RelatedPage struct {
	Path    string   `json:"path"`
	Title   string   `json:"title"`
	Score   float64  `json:"score"`
	Reasons []string `json:"reasons"`
}

// Signal weights. bm25 scores are normalized per result set — rank/rank_best
// maps the best FTS hit to 1.0 and the rest into (0,1] — so the content
// signal is only comparable within one RelatedTo call, never across docs.
const (
	weightLinksTo       = 3.0
	weightLinkedFrom    = 3.0
	weightBidirectional = 2.0
	weightSharedTag     = 1.5
	weightContent       = 2.5
	maxSharedTagReasons = 4
	maxMoreLikeTerms    = 24
)

// RelatedTo ranks pages similar or connected to docPath by merging three
// local signals: the markdown link graph, shared frontmatter tags, and an
// FTS5 more-like-this pass over the doc's title/tags/headings. Every
// candidate query JOINs documents, so dangling link targets never surface.
func (s *Store) RelatedTo(docPath string, limit int) ([]RelatedPage, error) {
	if limit <= 0 {
		limit = 30
	}
	if limit > 150 {
		limit = 150
	}

	var title, tags, headings sql.NullString
	err := s.DB.QueryRow(`SELECT title, tags, headings FROM documents WHERE path = ?`, docPath).
		Scan(&title, &tags, &headings)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotIndexed
	}
	if err != nil {
		return nil, fmt.Errorf("load source doc: %w", err)
	}

	type candidate struct {
		title      string
		outbound   bool
		inbound    bool
		sharedTags []string
		bm25norm   float64
		hasContent bool
	}
	pages := map[string]*candidate{}
	get := func(path, title string) *candidate {
		c, ok := pages[path]
		if !ok {
			c = &candidate{}
			pages[path] = c
		}
		if c.title == "" {
			c.title = title
		}
		return c
	}

	// Link graph: outbound + inbound neighbors in one pass.
	rows, err := s.DB.Query(`
		SELECT d.path, COALESCE(d.title, ''), 'out' AS dir
		FROM doc_links l JOIN documents d ON d.path = l.dst_path
		WHERE l.src_path = ?
		UNION ALL
		SELECT d.path, COALESCE(d.title, ''), 'in'
		FROM doc_links l JOIN documents d ON d.path = l.src_path
		WHERE l.dst_path = ?`, docPath, docPath)
	if err != nil {
		return nil, fmt.Errorf("link graph query: %w", err)
	}
	for rows.Next() {
		var path, title, dir string
		if err := rows.Scan(&path, &title, &dir); err != nil {
			rows.Close()
			return nil, err
		}
		if path == docPath {
			continue
		}
		c := get(path, title)
		if dir == "out" {
			c.outbound = true
		} else {
			c.inbound = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Shared frontmatter tags.
	rows, err = s.DB.Query(`
		SELECT d.path, COALESCE(d.title, ''), t2.tag
		FROM doc_tags t1
		JOIN doc_tags t2 ON t2.tag = t1.tag AND t2.path <> t1.path
		JOIN documents d ON d.path = t2.path
		WHERE t1.path = ?`, docPath)
	if err != nil {
		return nil, fmt.Errorf("shared tags query: %w", err)
	}
	for rows.Next() {
		var path, title, tag string
		if err := rows.Scan(&path, &title, &tag); err != nil {
			rows.Close()
			return nil, err
		}
		c := get(path, title)
		c.sharedTags = append(c.sharedTags, tag)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// More-like-this over title + tags + headings.
	if match := quoteTokens(moreLikeTerms(title.String, tags.String, headings.String)); match != "" {
		rows, err = s.DB.Query(`
			SELECT path, COALESCE(title, ''), bm25(documents_fts, 10, 5, 1, 5) AS rank
			FROM documents_fts
			WHERE documents_fts MATCH ? AND path <> ?
			ORDER BY rank
			LIMIT ?`, match, docPath, limit)
		if err != nil {
			return nil, fmt.Errorf("more-like-this query: %w", err)
		}
		var rankBest float64
		first := true
		for rows.Next() {
			var path, title string
			var rank float64
			if err := rows.Scan(&path, &title, &rank); err != nil {
				rows.Close()
				return nil, err
			}
			if first {
				rankBest = rank
				first = false
			}
			if rankBest >= 0 {
				// bm25 ranks are negative; a zero best rank would divide by
				// zero, so skip the content signal entirely.
				continue
			}
			c := get(path, title)
			c.bm25norm = rank / rankBest
			c.hasContent = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	out := make([]RelatedPage, 0, len(pages))
	for path, c := range pages {
		var score float64
		var reasons []string
		if c.outbound {
			score += weightLinksTo
			reasons = append(reasons, "links-to")
		}
		if c.inbound {
			score += weightLinkedFrom
			reasons = append(reasons, "linked-from")
		}
		if c.outbound && c.inbound {
			score += weightBidirectional
		}
		if len(c.sharedTags) > 0 {
			sort.Strings(c.sharedTags)
			shared := c.sharedTags
			if len(shared) > maxSharedTagReasons {
				shared = shared[:maxSharedTagReasons]
			}
			score += weightSharedTag * float64(len(shared))
			for _, tag := range shared {
				reasons = append(reasons, "shared-tag:"+tag)
			}
		}
		if c.hasContent {
			score += weightContent * c.bm25norm
			reasons = append(reasons, "similar-content")
		}
		out = append(out, RelatedPage{Path: path, Title: c.title, Score: score, Reasons: reasons})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Path < out[j].Path
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// moreLikeTerms tokenizes the doc's title/tags/headings into MATCH terms:
// lowercased, edge punctuation trimmed, ≥3 runes, not purely numeric,
// deduped in order, capped at maxMoreLikeTerms.
func moreLikeTerms(title, tags, headings string) []string {
	seen := map[string]bool{}
	var terms []string
	for _, src := range []string{title, tags, headings} {
		for _, field := range strings.Fields(src) {
			term := strings.ToLower(strings.TrimFunc(field, func(r rune) bool {
				return !unicode.IsLetter(r) && !unicode.IsNumber(r)
			}))
			if utf8.RuneCountInString(term) < 3 || isNumeric(term) || seen[term] {
				continue
			}
			seen[term] = true
			terms = append(terms, term)
			if len(terms) >= maxMoreLikeTerms {
				return terms
			}
		}
	}
	return terms
}

func isNumeric(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
