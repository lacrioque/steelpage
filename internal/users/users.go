package users

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

var (
	ErrInvalidName  = errors.New("display name must be 1-64 characters")
	ErrInvalidEmail = errors.New("email must be 3-254 characters and contain '@'")
	ErrInvalidRole  = errors.New("role must be 'admin' or 'user'")
	ErrNotFound     = errors.New("user not found")
	ErrEmailTaken   = errors.New("email already registered")
	ErrOIDCMismatch = errors.New("email is already linked to a different identity provider account")
)

// User is the canonical identity. Pointer fields are NULL-friendly.
type User struct {
	ID              int64    `json:"id"`
	Email           *string  `json:"email"`
	DisplayName     string   `json:"display_name"`
	Role            string   `json:"role"`
	OIDCProvider    *string  `json:"oidc_provider,omitempty"`
	OIDCSubject     *string  `json:"oidc_subject,omitempty"`
	Groups          []string `json:"groups"`
	CreatedAt       string   `json:"created_at"`
	EmailVerifiedAt *string  `json:"email_verified_at"`
	TOTPEnabledAt   *string  `json:"totp_enabled_at"`
	TOTPSecret      string   `json:"-"`
	FontFamily      *string  `json:"font_family"`
	PasswordHash    string   `json:"-"`
}

// AllowedFonts is the closed set of font keys users can pick from the UI.
// The matching @font-face declarations are bundled by the frontend via
// @fontsource so we never call out to Google Fonts (GDPR).
var AllowedFonts = []string{
	"ibm-plex-sans",
	"ibm-plex-serif",
	"eb-garamond",
	"lato",
	"verdana",
}

func IsAllowedFont(s string) bool {
	for _, f := range AllowedFonts {
		if f == s {
			return true
		}
	}
	return false
}

type Store struct {
	DB *sql.DB
}

func New(db *sql.DB) *Store { return &Store{DB: db} }

func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func validateEmail(email string) error {
	clean := normalizeEmail(email)
	if len(clean) < 3 || len(clean) > 254 || !strings.Contains(clean, "@") {
		return ErrInvalidEmail
	}
	return nil
}

func validateDisplayName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || len(trimmed) > 64 {
		return ErrInvalidName
	}
	return nil
}

func validateRole(role string) error {
	switch role {
	case RoleAdmin, RoleUser:
		return nil
	default:
		return ErrInvalidRole
	}
}

// CreateLocal inserts a brand-new user with email + bcrypt-hashed password.
func (s *Store) CreateLocal(email, displayName, passwordHash, role string) (*User, error) {
	if err := validateEmail(email); err != nil {
		return nil, err
	}
	if err := validateDisplayName(displayName); err != nil {
		return nil, err
	}
	if role == "" {
		role = RoleUser
	}
	if err := validateRole(role); err != nil {
		return nil, err
	}
	emailNorm := normalizeEmail(email)
	displayClean := strings.TrimSpace(displayName)
	now := time.Now().UTC().Format(time.RFC3339)

	res, err := s.DB.Exec(
		`INSERT INTO users(email, display_name, password_hash, role, created_at) VALUES(?, ?, ?, ?, ?)`,
		emailNorm, displayClean, passwordHash, role, now,
	)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("insert user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetByID(id)
}

// FindByEmail returns ErrNotFound when no row exists.
func (s *Store) FindByEmail(email string) (*User, error) {
	row := s.DB.QueryRow(selectQuery+` WHERE email = ?`, normalizeEmail(email))
	return scanUser(row)
}

// FindByOIDC returns the user previously linked to a given provider+subject.
func (s *Store) FindByOIDC(provider, subject string) (*User, error) {
	row := s.DB.QueryRow(selectQuery+` WHERE oidc_provider = ? AND oidc_subject = ?`, provider, subject)
	return scanUser(row)
}

// EnsureFromOIDC resolves the user behind an OIDC sign-in. Three branches:
//
//  1. Already linked (provider+subject matches a row) → return it.
//  2. Email match — a local user exists with the same email and no OIDC
//     binding yet → bind this provider+subject to that row and return it.
//     This is the "link by email" case: someone who registered locally and
//     later signs in via SSO with the same email gets one merged account.
//  3. No match → create a new row.
//
// If a local user has the same email but is ALREADY linked to a different
// (provider, subject), we refuse with ErrOIDCMismatch — silently overwriting
// would let one IdP take over another's identity. The caller surfaces this
// to the operator (e.g. login page error banner).
func (s *Store) EnsureFromOIDC(provider, subject, email, displayName string) (*User, error) {
	if u, err := s.FindByOIDC(provider, subject); err == nil {
		return u, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	emailValid := email != "" && validateEmail(email) == nil
	emailNorm := ""
	if emailValid {
		emailNorm = normalizeEmail(email)
	}

	if emailNorm != "" {
		if existing, err := s.FindByEmail(emailNorm); err == nil {
			if existing.OIDCProvider != nil && existing.OIDCSubject != nil {
				// Already bound to some IdP record — refuse to clobber.
				if *existing.OIDCProvider == provider && *existing.OIDCSubject == subject {
					return existing, nil
				}
				return nil, ErrOIDCMismatch
			}
			if _, err := s.DB.Exec(
				`UPDATE users SET oidc_provider = ?, oidc_subject = ? WHERE id = ?`,
				provider, subject, existing.ID,
			); err != nil {
				return nil, fmt.Errorf("link oidc: %w", err)
			}
			return s.GetByID(existing.ID)
		} else if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}

	displayClean := strings.TrimSpace(displayName)
	if displayClean == "" {
		displayClean = "user"
	}
	emailPtr := sql.NullString{}
	if emailValid {
		emailPtr = sql.NullString{Valid: true, String: emailNorm}
	}
	now := time.Now().UTC().Format(time.RFC3339)

	res, err := s.DB.Exec(
		`INSERT INTO users(email, display_name, oidc_provider, oidc_subject, role, created_at) VALUES(?, ?, ?, ?, 'user', ?)`,
		emailPtr, displayClean, provider, subject, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert oidc user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetByID(id)
}

func (s *Store) GetByID(id int64) (*User, error) {
	row := s.DB.QueryRow(selectQuery+` WHERE id = ?`, id)
	return scanUser(row)
}

func (s *Store) ListAll() ([]*User, error) {
	rows, err := s.DB.Query(selectQuery + ` ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) SetRole(id int64, role string) error {
	if err := validateRole(role); err != nil {
		return err
	}
	res, err := s.DB.Exec(`UPDATE users SET role = ? WHERE id = ?`, role, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// AnyAdmin reports whether at least one admin exists. Used by bootstrap.
func (s *Store) AnyAdmin() (bool, error) {
	row := s.DB.QueryRow(`SELECT 1 FROM users WHERE role = 'admin' LIMIT 1`)
	var x int
	if err := row.Scan(&x); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

const selectQuery = `
SELECT id,
       email,
       display_name,
       COALESCE(password_hash, '') AS password_hash,
       oidc_provider,
       oidc_subject,
       role,
       created_at,
       email_verified_at,
       COALESCE(totp_secret, '') AS totp_secret,
       totp_enabled_at,
       font_family
FROM users
`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (*User, error) {
	var (
		u             User
		email         sql.NullString
		oidcProvider  sql.NullString
		oidcSubject   sql.NullString
		verifiedAt    sql.NullString
		totpEnabledAt sql.NullString
		fontFamily    sql.NullString
	)
	if err := row.Scan(&u.ID, &email, &u.DisplayName, &u.PasswordHash, &oidcProvider, &oidcSubject, &u.Role, &u.CreatedAt, &verifiedAt, &u.TOTPSecret, &totpEnabledAt, &fontFamily); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if email.Valid {
		v := email.String
		u.Email = &v
	}
	if oidcProvider.Valid {
		v := oidcProvider.String
		u.OIDCProvider = &v
	}
	if oidcSubject.Valid {
		v := oidcSubject.String
		u.OIDCSubject = &v
	}
	if verifiedAt.Valid {
		v := verifiedAt.String
		u.EmailVerifiedAt = &v
	}
	if totpEnabledAt.Valid {
		v := totpEnabledAt.String
		u.TOTPEnabledAt = &v
	}
	if fontFamily.Valid {
		v := fontFamily.String
		u.FontFamily = &v
	}
	return &u, nil
}

// SetTOTPSecret stores a fresh TOTP secret. Setup is two-step: this writes
// the secret BEFORE enabling MFA, so a half-finished setup doesn't lock the
// user out. ConfirmTOTP flips enabled_at once the user proves the code works.
func (s *Store) SetTOTPSecret(id int64, secret string) error {
	res, err := s.DB.Exec(`UPDATE users SET totp_secret = ?, totp_enabled_at = NULL WHERE id = ?`, secret, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ConfirmTOTP marks MFA as enabled — only called after the server verified
// a code generated from the stored secret.
func (s *Store) ConfirmTOTP(id int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.Exec(`UPDATE users SET totp_enabled_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DisableTOTP clears both the secret and the enabled timestamp.
func (s *Store) DisableTOTP(id int64) error {
	res, err := s.DB.Exec(`UPDATE users SET totp_secret = NULL, totp_enabled_at = NULL WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkEmailVerified sets email_verified_at = now for the given user. Idempotent.
func (s *Store) MarkEmailVerified(id int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.Exec(`UPDATE users SET email_verified_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetFontFamily updates the per-user font preference. Pass nil to clear it,
// which reverts the user to the system default (IBM Plex Sans).
func (s *Store) SetFontFamily(id int64, font *string) error {
	var val sql.NullString
	if font != nil {
		val = sql.NullString{Valid: true, String: *font}
	}
	res, err := s.DB.Exec(`UPDATE users SET font_family = ? WHERE id = ?`, val, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdatePasswordHash replaces the password_hash for a user. Used by the
// password-reset flow.
func (s *Store) UpdatePasswordHash(id int64, hash string) error {
	res, err := s.DB.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, hash, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// isUniqueConstraintErr inspects the driver-level error message for a UNIQUE
// constraint violation. modernc.org/sqlite returns wrapped errors so we match
// on the textual content.
func isUniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "constraint failed: UNIQUE")
}
