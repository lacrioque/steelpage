package comments

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	StatusOpen      = "open"
	StatusResolved  = "resolved"
	StatusOrphaned  = "orphaned"
	StatusRelocated = "relocated"
)

// Re-anchor tuning.
const (
	reanchorNearbyRadius = 10  // ±N lines for the exact-match scan (step 2)
	reanchorFuzzyRadius  = 20  // ±N lines for the fuzzy scan (step 3)
	reanchorFuzzyRatio   = 0.3 // Levenshtein / max-length allowed for "relocated"
)

var (
	ErrNotFound = errors.New("comment not found")
	ErrInvalid  = errors.New("invalid comment payload")
)

type Author struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"display_name"`
}

type Comment struct {
	ID          int64   `json:"id"`
	Path        string  `json:"path"`
	LineStart   int     `json:"line_start"`
	LineEnd     int     `json:"line_end"`
	AnchorText  string  `json:"anchor_text"`
	DocumentSHA string  `json:"document_sha"`
	Author      Author  `json:"author"`
	Body        string  `json:"body"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
	ResolvedAt  *string `json:"resolved_at"`
	ReplyTo     *int64  `json:"reply_to"`
}

type CreateInput struct {
	Path        string
	LineStart   int
	LineEnd     int
	AnchorText  string
	DocumentSHA string
	AuthorID    int64
	Body        string
	ReplyTo     *int64 // optional — when set, ties this comment to a parent
}

type UpdateInput struct {
	Body   *string
	Status *string
}

type Store struct {
	DB *sql.DB
}

func New(db *sql.DB) *Store { return &Store{DB: db} }

func (s *Store) Create(in CreateInput) (*Comment, error) {
	if in.Path == "" || in.LineStart < 1 || in.LineEnd < in.LineStart || strings.TrimSpace(in.Body) == "" || in.AuthorID == 0 {
		return nil, ErrInvalid
	}

	// Flatten replies: a reply-to-a-reply still points at the root comment.
	// That keeps the schema "comments can have ONE parent" instead of
	// allowing a tree, and gives the UI a clean "root → many replies"
	// pattern with no recursion.
	replyTo := in.ReplyTo
	if replyTo != nil {
		parent, err := s.GetByID(*replyTo)
		if err != nil {
			return nil, fmt.Errorf("%w: parent not found", ErrInvalid)
		}
		if parent.Path != in.Path {
			return nil, fmt.Errorf("%w: parent belongs to a different document", ErrInvalid)
		}
		if parent.ReplyTo != nil {
			// Parent is itself a reply — point at its root instead.
			replyTo = parent.ReplyTo
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var replyToVal sql.NullInt64
	if replyTo != nil {
		replyToVal = sql.NullInt64{Int64: *replyTo, Valid: true}
	}
	res, err := s.DB.Exec(`
		INSERT INTO comments
			(path, line_start, line_end, anchor_text, document_sha, author_id, body, status, created_at, updated_at, reply_to_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'open', ?, ?, ?)`,
		in.Path, in.LineStart, in.LineEnd, in.AnchorText, in.DocumentSHA, in.AuthorID, in.Body, now, now, replyToVal,
	)
	if err != nil {
		return nil, fmt.Errorf("insert comment: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetByID(id)
}

func (s *Store) GetByID(id int64) (*Comment, error) {
	row := s.DB.QueryRow(selectQuery+` WHERE c.id = ?`, id)
	return scanOne(row)
}

func (s *Store) ListByPath(path string) ([]*Comment, error) {
	rows, err := s.DB.Query(selectQuery+` WHERE c.path = ? ORDER BY c.line_start ASC, c.id ASC`, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Comment
	for rows.Next() {
		c, err := scanOne(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) Update(id int64, in UpdateInput) (*Comment, error) {
	existing, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	body := existing.Body
	status := existing.Status
	if in.Body != nil {
		if strings.TrimSpace(*in.Body) == "" {
			return nil, ErrInvalid
		}
		body = *in.Body
	}
	if in.Status != nil {
		switch *in.Status {
		case StatusOpen, StatusResolved, StatusOrphaned, StatusRelocated:
		default:
			return nil, ErrInvalid
		}
		status = *in.Status
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var resolvedAt sql.NullString
	if status == StatusResolved {
		if existing.Status != StatusResolved {
			resolvedAt = sql.NullString{Valid: true, String: now}
		} else if existing.ResolvedAt != nil {
			resolvedAt = sql.NullString{Valid: true, String: *existing.ResolvedAt}
		}
	}

	_, err = s.DB.Exec(`
		UPDATE comments
		SET body = ?, status = ?, updated_at = ?, resolved_at = ?
		WHERE id = ?`,
		body, status, now, resolvedAt, id,
	)
	if err != nil {
		return nil, fmt.Errorf("update comment: %w", err)
	}
	return s.GetByID(id)
}

// MovePath relocates every comment from one document path to another. Run
// inside the transaction that renames the file so they stay in sync.
func (s *Store) MovePath(from, to string) error {
	_, err := s.DB.Exec(`UPDATE comments SET path = ?, updated_at = ? WHERE path = ?`,
		to, time.Now().UTC().Format(time.RFC3339), from)
	return err
}

// DeletePath removes every comment associated with a document. Used on doc
// delete; comment history is lost but the git log still records who edited
// what.
func (s *Store) DeletePath(path string) error {
	_, err := s.DB.Exec(`DELETE FROM comments WHERE path = ?`, path)
	return err
}

// MarkPath re-anchors every active comment on a path after the document has
// been re-saved. Implements the doc §17 ladder:
//
//  1. Exact match at the recorded line          → status=open, sha bumped
//  2. Exact match within ±reanchorNearbyRadius  → status=open, line moved
//  3. Fuzzy match within ±reanchorFuzzyRadius   → status=relocated, line moved
//  4. No match                                   → status=orphaned
//
// Resolved comments are left alone — once closed, they don't follow the doc.
func (s *Store) MarkPath(path, newSHA string, lines []string) error {
	rows, err := s.DB.Query(`
		SELECT id, line_start, line_end, anchor_text, status
		FROM comments
		WHERE path = ? AND status IN ('open', 'orphaned', 'relocated')`,
		path,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	type plan struct {
		id        int64
		newStatus string
		newLine   int // 0 means "leave existing line"
	}
	var plans []plan

	for rows.Next() {
		var id int64
		var lineStart, lineEnd int
		var anchor sql.NullString
		var status string
		if err := rows.Scan(&id, &lineStart, &lineEnd, &anchor, &status); err != nil {
			return err
		}
		anchorText := ""
		if anchor.Valid {
			anchorText = anchor.String
		}
		nextStatus, nextLine := classify(lineStart, anchorText, lines)
		_ = lineEnd
		_ = status
		plans = append(plans, plan{id: id, newStatus: nextStatus, newLine: nextLine})
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if len(plans) == 0 {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	for _, p := range plans {
		var execErr error
		switch p.newStatus {
		case StatusOpen, StatusRelocated:
			if p.newLine > 0 {
				_, execErr = tx.Exec(
					`UPDATE comments SET status=?, line_start=?, line_end=?, document_sha=?, updated_at=? WHERE id=?`,
					p.newStatus, p.newLine, p.newLine, newSHA, now, p.id,
				)
			} else {
				_, execErr = tx.Exec(
					`UPDATE comments SET status=?, document_sha=?, updated_at=? WHERE id=?`,
					p.newStatus, newSHA, now, p.id,
				)
			}
		case StatusOrphaned:
			_, execErr = tx.Exec(
				`UPDATE comments SET status='orphaned', updated_at=? WHERE id=?`,
				now, p.id,
			)
		}
		if execErr != nil {
			_ = tx.Rollback()
			return execErr
		}
	}
	return tx.Commit()
}

// classify returns the new status and (optionally) a new line_start for one
// comment. newLine == 0 means "keep the existing line".
func classify(lineStart int, anchor string, lines []string) (string, int) {
	if anchor == "" {
		// No anchor recorded — treat as orphaned to be safe.
		return StatusOrphaned, 0
	}

	// Step 1 — exact match at the recorded line.
	if lineStart >= 1 && lineStart <= len(lines) && lines[lineStart-1] == anchor {
		return StatusOpen, 0
	}

	// Step 2 — exact match within ±reanchorNearbyRadius.
	if hit := nearbyExact(lines, lineStart, anchor, reanchorNearbyRadius); hit > 0 {
		return StatusOpen, hit
	}

	// Step 3 — fuzzy match within ±reanchorFuzzyRadius.
	if hit := nearbyFuzzy(lines, lineStart, anchor, reanchorFuzzyRadius, reanchorFuzzyRatio); hit > 0 {
		return StatusRelocated, hit
	}

	// Step 4 — give up.
	return StatusOrphaned, 0
}

func nearbyExact(lines []string, origin int, anchor string, radius int) int {
	lo := origin - radius
	if lo < 1 {
		lo = 1
	}
	hi := origin + radius
	if hi > len(lines) {
		hi = len(lines)
	}
	// Prefer the closest match to the original line.
	bestIdx := 0
	bestDist := -1
	for i := lo; i <= hi; i++ {
		if lines[i-1] != anchor {
			continue
		}
		d := i - origin
		if d < 0 {
			d = -d
		}
		if bestDist < 0 || d < bestDist {
			bestIdx = i
			bestDist = d
		}
	}
	return bestIdx
}

func nearbyFuzzy(lines []string, origin int, anchor string, radius int, ratio float64) int {
	lo := origin - radius
	if lo < 1 {
		lo = 1
	}
	hi := origin + radius
	if hi > len(lines) {
		hi = len(lines)
	}
	// Need at least some signal: skip lines made of only whitespace.
	bestIdx := 0
	bestDist := -1
	for i := lo; i <= hi; i++ {
		candidate := lines[i-1]
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if !SimilarTo(candidate, anchor, ratio) {
			continue
		}
		d := i - origin
		if d < 0 {
			d = -d
		}
		if bestDist < 0 || d < bestDist {
			bestIdx = i
			bestDist = d
		}
	}
	return bestIdx
}

const selectQuery = `
SELECT c.id, c.path, c.line_start, c.line_end,
       COALESCE(c.anchor_text, '') AS anchor_text,
       COALESCE(c.document_sha, '') AS document_sha,
       u.id AS author_id, u.display_name AS author_display,
       c.body, c.status, c.created_at, c.updated_at, c.resolved_at,
       c.reply_to_id
FROM comments c
JOIN users u ON u.id = c.author_id
`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanOne(row rowScanner) (*Comment, error) {
	var c Comment
	var resolved sql.NullString
	var replyTo sql.NullInt64
	if err := row.Scan(
		&c.ID, &c.Path, &c.LineStart, &c.LineEnd,
		&c.AnchorText, &c.DocumentSHA,
		&c.Author.ID, &c.Author.DisplayName,
		&c.Body, &c.Status, &c.CreatedAt, &c.UpdatedAt, &resolved, &replyTo,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if resolved.Valid {
		c.ResolvedAt = &resolved.String
	}
	if replyTo.Valid {
		v := replyTo.Int64
		c.ReplyTo = &v
	}
	return &c, nil
}
