// Package notifications stores per-user notifications for @mentions and
// replies. It is a leaf package (database/sql only); orchestration — deciding
// who gets notified and whether an email goes out — lives in the api layer.
package notifications

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Kind string

const (
	KindMention Kind = "mention"
	KindReply   Kind = "reply"
)

var ErrNotFound = errors.New("notification not found")

// Actor is the user who triggered the notification.
type Actor struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"display_name"`
}

type Notification struct {
	ID        int64   `json:"id"`
	Kind      Kind    `json:"kind"`
	Path      string  `json:"path"`
	CommentID int64   `json:"comment_id"`
	Actor     Actor   `json:"actor"`
	CreatedAt string  `json:"created_at"`
	ReadAt    *string `json:"read_at"`
}

type Store struct {
	DB *sql.DB
}

func New(db *sql.DB) *Store { return &Store{DB: db} }

// Create inserts a notification. Self-notifications (recipient == actor) are
// silently skipped — you never need to be told about your own comment.
func (s *Store) Create(recipientID, actorID, commentID int64, kind Kind, path string) error {
	if recipientID == actorID {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`
		INSERT INTO notifications (recipient_id, actor_id, kind, comment_id, path, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		recipientID, actorID, string(kind), commentID, path, now,
	)
	if err != nil {
		return fmt.Errorf("insert notification: %w", err)
	}
	return nil
}

// List returns the newest notifications for a user, newest first.
func (s *Store) List(recipientID int64, limit int) ([]*Notification, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.DB.Query(`
		SELECT n.id, n.kind, n.path, n.comment_id, n.actor_id, u.display_name, n.created_at, n.read_at
		FROM notifications n
		JOIN users u ON u.id = n.actor_id
		WHERE n.recipient_id = ?
		ORDER BY n.id DESC
		LIMIT ?`,
		recipientID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Notification
	for rows.Next() {
		var n Notification
		var kind string
		var readAt sql.NullString
		if err := rows.Scan(&n.ID, &kind, &n.Path, &n.CommentID, &n.Actor.ID, &n.Actor.DisplayName, &n.CreatedAt, &readAt); err != nil {
			return nil, err
		}
		n.Kind = Kind(kind)
		if readAt.Valid {
			v := readAt.String
			n.ReadAt = &v
		}
		out = append(out, &n)
	}
	return out, rows.Err()
}

// UnreadCount returns how many notifications the user has not read yet.
func (s *Store) UnreadCount(recipientID int64) (int, error) {
	row := s.DB.QueryRow(`SELECT COUNT(*) FROM notifications WHERE recipient_id = ? AND read_at IS NULL`, recipientID)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// MarkRead sets read_at on one notification. The recipient_id guard enforces
// ownership — marking someone else's notification returns ErrNotFound.
func (s *Store) MarkRead(recipientID, id int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.Exec(
		`UPDATE notifications SET read_at = ? WHERE id = ? AND recipient_id = ? AND read_at IS NULL`,
		now, id, recipientID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// Already read or not yours — check which for a precise error.
		row := s.DB.QueryRow(`SELECT 1 FROM notifications WHERE id = ? AND recipient_id = ?`, id, recipientID)
		var x int
		if scanErr := row.Scan(&x); scanErr != nil {
			return ErrNotFound
		}
	}
	return nil
}

// MarkAllRead sets read_at on every unread notification of the user.
func (s *Store) MarkAllRead(recipientID int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(
		`UPDATE notifications SET read_at = ? WHERE recipient_id = ? AND read_at IS NULL`,
		now, recipientID,
	)
	return err
}
