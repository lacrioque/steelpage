package tokens

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// TokenPrefix marks token strings so they're obvious in logs and code reviews.
const TokenPrefix = "spt_"

var (
	ErrNotFound = errors.New("token not found")
	ErrInvalid  = errors.New("invalid token payload")
)

// Token is what's stored/returned over the API; PlaintextSecret is only set on
// the response from Create() and is never persisted.
type Token struct {
	ID              int64    `json:"id"`
	UserID          int64    `json:"user_id"`
	Name            string   `json:"name"`
	Scopes          []string `json:"scopes"`
	ExpiresAt       *string  `json:"expires_at"`
	LastUsedAt      *string  `json:"last_used_at"`
	CreatedAt       string   `json:"created_at"`
	PlaintextSecret string   `json:"plaintext,omitempty"`
}

type Store struct {
	DB *sql.DB
}

func New(db *sql.DB) *Store { return &Store{DB: db} }

// Create mints a new token with the given name + scopes for the user.
// The plaintext secret is returned ONCE — store only the hash.
//
// Scope syntax:
//   - "read", "comment", "write"                  → action allowed on any path
//   - "read:path/to/doc.md", "comment:foo/*"     → action allowed on that path only
func (s *Store) Create(userID int64, name string, scopes []string, expiresAt *time.Time) (*Token, error) {
	clean := strings.TrimSpace(name)
	if clean == "" || len(clean) > 100 {
		return nil, ErrInvalid
	}
	for _, sc := range scopes {
		if !validScope(sc) {
			return nil, fmt.Errorf("%w: bad scope %q", ErrInvalid, sc)
		}
	}
	if len(scopes) == 0 {
		return nil, fmt.Errorf("%w: at least one scope required", ErrInvalid)
	}

	raw, err := generateSecret()
	if err != nil {
		return nil, err
	}
	plaintext := TokenPrefix + raw
	hash := hashSecret(plaintext)

	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return nil, err
	}

	var expiry sql.NullString
	if expiresAt != nil {
		expiry = sql.NullString{Valid: true, String: expiresAt.UTC().Format(time.RFC3339)}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.Exec(`
		INSERT INTO api_tokens(user_id, name, token_hash, scopes, expires_at, created_at)
		VALUES(?, ?, ?, ?, ?, ?)`,
		userID, clean, hash, string(scopesJSON), expiry, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert token: %w", err)
	}
	id, _ := res.LastInsertId()

	t := &Token{
		ID:              id,
		UserID:          userID,
		Name:            clean,
		Scopes:          scopes,
		CreatedAt:       now,
		PlaintextSecret: plaintext,
	}
	if expiresAt != nil {
		v := expiresAt.UTC().Format(time.RFC3339)
		t.ExpiresAt = &v
	}
	return t, nil
}

// FindByPlaintext looks up a token by its plaintext secret (sha256 + lookup).
// Expired tokens are not returned. Updates last_used_at on success.
func (s *Store) FindByPlaintext(plaintext string) (*Token, error) {
	if !strings.HasPrefix(plaintext, TokenPrefix) {
		return nil, ErrNotFound
	}
	hash := hashSecret(plaintext)
	row := s.DB.QueryRow(`
		SELECT id, user_id, name, scopes, expires_at, last_used_at, created_at
		FROM api_tokens WHERE token_hash = ?`,
		hash,
	)
	t, err := scan(row)
	if err != nil {
		return nil, err
	}
	if t.ExpiresAt != nil {
		if exp, perr := time.Parse(time.RFC3339, *t.ExpiresAt); perr == nil && time.Now().After(exp) {
			return nil, ErrNotFound
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.DB.Exec(`UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, now, t.ID)
	t.LastUsedAt = &now
	return t, nil
}

// ListForUser returns every token owned by the user, without plaintext.
func (s *Store) ListForUser(userID int64) ([]*Token, error) {
	rows, err := s.DB.Query(`
		SELECT id, user_id, name, scopes, expires_at, last_used_at, created_at
		FROM api_tokens WHERE user_id = ? ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Token{}
	for rows.Next() {
		t, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Delete revokes a token owned by userID. Returns ErrNotFound if no row matches
// (which also covers the case where the token belongs to a different user).
func (s *Store) Delete(userID, tokenID int64) error {
	res, err := s.DB.Exec(
		`DELETE FROM api_tokens WHERE id = ? AND user_id = ?`,
		tokenID, userID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// validScope accepts "read"/"comment"/"write" optionally suffixed with ":path".
func validScope(s string) bool {
	parts := strings.SplitN(s, ":", 2)
	switch parts[0] {
	case "read", "comment", "write":
	default:
		return false
	}
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1]) != ""
	}
	return true
}

// AllowsAction reports whether the token grants the given action on the given
// path. It only checks the token's own scopes — the caller still has to verify
// the owner's permissions on the path.
func AllowsAction(scopes []string, action, path string) bool {
	wanted := rank(action)
	if wanted == 0 {
		return false
	}
	for _, raw := range scopes {
		parts := strings.SplitN(raw, ":", 2)
		if rank(parts[0]) < wanted {
			continue
		}
		if len(parts) == 1 {
			// Scope without a path → applies to all paths.
			return true
		}
		if matchPath(parts[1], path) {
			return true
		}
	}
	return false
}

func rank(p string) int {
	switch p {
	case "read":
		return 1
	case "comment":
		return 2
	case "write":
		return 3
	default:
		return 0
	}
}

// matchPath supports exact match and the simple "/**" suffix for sub-trees.
// We deliberately don't pull in doublestar here so callers can store stable
// scope strings; richer matching can come later if needed.
func matchPath(scopePath, requestPath string) bool {
	if scopePath == requestPath {
		return true
	}
	if strings.HasSuffix(scopePath, "/**") {
		prefix := strings.TrimSuffix(scopePath, "/**")
		return strings.HasPrefix(requestPath, prefix+"/") || requestPath == prefix
	}
	return false
}

func generateSecret() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func hashSecret(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scan(row rowScanner) (*Token, error) {
	var (
		t         Token
		scopesRaw string
		expires   sql.NullString
		lastUsed  sql.NullString
	)
	if err := row.Scan(&t.ID, &t.UserID, &t.Name, &scopesRaw, &expires, &lastUsed, &t.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(scopesRaw), &t.Scopes); err != nil {
		return nil, err
	}
	if expires.Valid {
		v := expires.String
		t.ExpiresAt = &v
	}
	if lastUsed.Valid {
		v := lastUsed.String
		t.LastUsedAt = &v
	}
	return &t, nil
}
