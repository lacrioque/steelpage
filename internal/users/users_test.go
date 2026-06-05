package users_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/markusfluer/steelpage/internal/db"
	"github.com/markusfluer/steelpage/internal/users"
)

// setup spins up a temp SQLite DB with all migrations applied and returns a
// fresh users.Store for the test.
func setup(t *testing.T) *users.Store {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Migrate(d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return users.New(d)
}

func TestEnsureFromOIDC_CreatesNewUser(t *testing.T) {
	s := setup(t)

	u, err := s.EnsureFromOIDC("openid-connect", "subject-42", "alice@example.com", "Alice")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if u.OIDCProvider == nil || *u.OIDCProvider != "openid-connect" {
		t.Fatalf("oidc_provider not set on fresh user")
	}
	if u.OIDCSubject == nil || *u.OIDCSubject != "subject-42" {
		t.Fatalf("oidc_subject not set on fresh user")
	}
	if u.Email == nil || *u.Email != "alice@example.com" {
		t.Fatalf("email not stored: %v", u.Email)
	}
}

func TestEnsureFromOIDC_IsIdempotent(t *testing.T) {
	s := setup(t)

	u1, err := s.EnsureFromOIDC("openid-connect", "subject-7", "carl@example.com", "Carl")
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	u2, err := s.EnsureFromOIDC("openid-connect", "subject-7", "carl@example.com", "Carl")
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if u1.ID != u2.ID {
		t.Fatalf("expected same user id, got %d and %d", u1.ID, u2.ID)
	}
}

func TestEnsureFromOIDC_LinksByEmail(t *testing.T) {
	s := setup(t)

	// Local user signs up with a password first.
	local, err := s.CreateLocal("dora@example.com", "Dora", "$2a$12$dummyhash", users.RoleUser)
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	if local.OIDCProvider != nil {
		t.Fatalf("local user should not have oidc_provider set")
	}

	// Later, Dora signs in via OIDC with the same email.
	linked, err := s.EnsureFromOIDC("openid-connect", "dora-sub-1", "dora@example.com", "Dora")
	if err != nil {
		t.Fatalf("link via oidc: %v", err)
	}
	if linked.ID != local.ID {
		t.Fatalf("expected linking to local user (%d), got new user (%d)", local.ID, linked.ID)
	}
	if linked.OIDCProvider == nil || *linked.OIDCProvider != "openid-connect" {
		t.Fatalf("link did not set oidc_provider")
	}
	if linked.OIDCSubject == nil || *linked.OIDCSubject != "dora-sub-1" {
		t.Fatalf("link did not set oidc_subject")
	}
}

func TestEnsureFromOIDC_RefusesMismatch(t *testing.T) {
	s := setup(t)

	// First link binds the email to subject-A.
	if _, err := s.EnsureFromOIDC("openid-connect", "subject-A", "eve@example.com", "Eve"); err != nil {
		t.Fatalf("initial ensure: %v", err)
	}
	// A second login arriving with the SAME email but a DIFFERENT subject must
	// not silently take over the account.
	_, err := s.EnsureFromOIDC("openid-connect", "subject-B", "eve@example.com", "Eve")
	if !errors.Is(err, users.ErrOIDCMismatch) {
		t.Fatalf("expected ErrOIDCMismatch, got: %v", err)
	}
}

func TestEnsureFromOIDC_EmailIsCaseInsensitive(t *testing.T) {
	s := setup(t)

	local, err := s.CreateLocal("Frank@Example.com", "Frank", "$2a$12$dummyhash", users.RoleUser)
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	linked, err := s.EnsureFromOIDC("openid-connect", "frank-sub", "FRANK@example.com", "Frank")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if linked.ID != local.ID {
		t.Fatalf("case-insensitive email match failed: %d vs %d", local.ID, linked.ID)
	}
}

func TestNotificationPrefDefaults(t *testing.T) {
	s := setup(t)

	u, err := s.CreateLocal("gina@example.com", "Gina", "$2a$12$dummyhash", users.RoleUser)
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	if !u.EmailOnMention {
		t.Fatalf("email_on_mention should default to true")
	}
	if u.EmailOnResponse {
		t.Fatalf("email_on_response should default to false")
	}
}

func TestSetNotificationPrefs(t *testing.T) {
	s := setup(t)

	u, err := s.CreateLocal("hugo@example.com", "Hugo", "$2a$12$dummyhash", users.RoleUser)
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}

	// Flip one field, leave the other untouched.
	off := false
	if err := s.SetNotificationPrefs(u.ID, &off, nil); err != nil {
		t.Fatalf("set prefs: %v", err)
	}
	got, err := s.GetByID(u.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.EmailOnMention {
		t.Fatalf("email_on_mention not turned off")
	}
	if got.EmailOnResponse {
		t.Fatalf("email_on_response changed without being set")
	}

	// Flip both.
	on := true
	if err := s.SetNotificationPrefs(u.ID, &on, &on); err != nil {
		t.Fatalf("set both prefs: %v", err)
	}
	got, _ = s.GetByID(u.ID)
	if !got.EmailOnMention || !got.EmailOnResponse {
		t.Fatalf("prefs not both on: mention=%v response=%v", got.EmailOnMention, got.EmailOnResponse)
	}

	// No-op call is fine; unknown user errors.
	if err := s.SetNotificationPrefs(u.ID, nil, nil); err != nil {
		t.Fatalf("no-op prefs: %v", err)
	}
	if err := s.SetNotificationPrefs(99999, &on, nil); !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown user, got %v", err)
	}
}
