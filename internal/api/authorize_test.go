package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/markusfluer/steelpage/internal/config"
	"github.com/markusfluer/steelpage/internal/configsvc"
	"github.com/markusfluer/steelpage/internal/db"
	"github.com/markusfluer/steelpage/internal/middleware"
	"github.com/markusfluer/steelpage/internal/permissions"
	"github.com/markusfluer/steelpage/internal/tokens"
	"github.com/markusfluer/steelpage/internal/users"
)

// authzTestAPI builds the minimal API fixture the authorization core needs:
// temp SQLite with real stores plus a configsvc over `base`, so tests can flip
// live config keys at runtime. Cold-start default: allow_anonymous_read=true —
// tests that override it to false thereby prove Authorize/CanRead consult the
// LIVE snapshot, not the immutable a.Cfg baseline.
func authzTestAPI(t *testing.T) *API {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Migrate(d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	base := &config.Config{}
	base.Auth.AllowAnonymousRead = true
	svc, err := configsvc.New(d, base)
	if err != nil {
		t.Fatalf("configsvc: %v", err)
	}
	return &API{
		Cfg:         base,
		Users:       users.New(d),
		Permissions: permissions.New(d),
		Tokens:      tokens.New(d),
		Configsvc:   svc,
	}
}

func authzTestUser(t *testing.T, a *API, email string) *users.User {
	t.Helper()
	u, err := a.Users.CreateLocal(email, email, "hash", users.RoleUser)
	if err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}
	return u
}

func authzTestRule(t *testing.T, a *API, glob, subjType, subjValue, perm string) {
	t.Helper()
	_, err := a.Permissions.Create(permissions.Rule{
		PathGlob:     glob,
		SubjectType:  subjType,
		SubjectValue: subjValue,
		Permission:   perm,
	})
	if err != nil {
		t.Fatalf("create rule %s %s:%s %s: %v", glob, subjType, subjValue, perm, err)
	}
}

func authzTestSetAnonRead(t *testing.T, a *API, allowed bool) {
	t.Helper()
	raw := json.RawMessage("false")
	if allowed {
		raw = json.RawMessage("true")
	}
	if err := a.Configsvc.Set(nil, "auth.allow_anonymous_read", raw); err != nil {
		t.Fatalf("set auth.allow_anonymous_read: %v", err)
	}
}

// The fallback read branch must consult the live config on every call: an
// override flipped at runtime changes the outcome without a restart.
func TestAuthorizeAnonymousReadLiveConfig(t *testing.T) {
	a := authzTestAPI(t)
	ctx := context.Background()

	if _, status := a.Authorize(ctx, "docs/x.md", permissions.PermRead); status != 0 {
		t.Fatalf("anon read with allow_anonymous_read=true: got %d, want 0", status)
	}

	authzTestSetAnonRead(t, a, false)
	if _, status := a.Authorize(ctx, "docs/x.md", permissions.PermRead); status != http.StatusUnauthorized {
		t.Fatalf("anon read after live flip to false: got %d, want 401 (read cold-start a.Cfg?)", status)
	}

	authzTestSetAnonRead(t, a, true)
	if _, status := a.Authorize(ctx, "docs/x.md", permissions.PermRead); status != 0 {
		t.Fatalf("anon read after flip back to true: got %d, want 0", status)
	}
}

// A token-authenticated caller can do strictly less than its owner: the scope
// gate must deny actions outside the scopes even when the owner is permitted.
func TestAuthorizeTokenScopeGate(t *testing.T) {
	a := authzTestAPI(t)
	alice := authzTestUser(t, a, "alice@example.com")

	cases := []struct {
		name   string
		scopes []string // nil = session-authenticated
		action string
		want   int
	}{
		{"session write allowed by fallback", nil, permissions.PermWrite, 0},
		{"read-only token denies write", []string{"read"}, permissions.PermWrite, http.StatusForbidden},
		{"read-only token denies comment", []string{"read"}, permissions.PermComment, http.StatusForbidden},
		{"read-only token allows read", []string{"read"}, permissions.PermRead, 0},
		{"comment token allows read (rank)", []string{"comment"}, permissions.PermRead, 0},
		{"empty scope list denies everything", []string{}, permissions.PermRead, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := middleware.WithIdentity(context.Background(), alice, tc.scopes)
			if _, status := a.Authorize(ctx, "docs/x.md", tc.action); status != tc.want {
				t.Fatalf("got %d, want %d", status, tc.want)
			}
		})
	}
}

func TestAuthorizePathScopedScopes(t *testing.T) {
	a := authzTestAPI(t)
	alice := authzTestUser(t, a, "alice@example.com")

	cases := []struct {
		name   string
		scopes []string
		action string
		path   string
		want   int
	}{
		{"subtree scope matches inside", []string{"read:guides/**"}, permissions.PermRead, "guides/intro.md", 0},
		{"subtree scope matches nested", []string{"read:guides/**"}, permissions.PermRead, "guides/a/b.md", 0},
		{"subtree scope matches the root itself", []string{"read:guides/**"}, permissions.PermRead, "guides", 0},
		{"subtree scope denies outside", []string{"read:guides/**"}, permissions.PermRead, "other/x.md", http.StatusForbidden},
		{"subtree scope denies prefix sibling", []string{"read:guides/**"}, permissions.PermRead, "guides-old/x.md", http.StatusForbidden},
		{"exact scope matches exactly", []string{"read:notes/a.md"}, permissions.PermRead, "notes/a.md", 0},
		{"exact scope denies sibling", []string{"read:notes/a.md"}, permissions.PermRead, "notes/b.md", http.StatusForbidden},
		{"higher action scope grants read", []string{"write:guides/**"}, permissions.PermRead, "guides/intro.md", 0},
		{"scoped read never grants write", []string{"read:guides/**"}, permissions.PermWrite, "guides/intro.md", http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := middleware.WithIdentity(context.Background(), alice, tc.scopes)
			if _, status := a.Authorize(ctx, tc.path, tc.action); status != tc.want {
				t.Fatalf("got %d, want %d", status, tc.want)
			}
		})
	}
}

// "Replace defaults" semantics: once any rule matches a path, only the rules
// decide; unmatched paths fall back to the defaults.
func TestAuthorizeRuleMatchVsFallback(t *testing.T) {
	a := authzTestAPI(t)
	alice := authzTestUser(t, a, "alice@example.com")
	bob := authzTestUser(t, a, "bob@example.com")

	authzTestRule(t, a, "private/**", permissions.SubjectUser, fmt.Sprintf("%d", alice.ID), permissions.PermRead)
	authzTestRule(t, a, "wiki/**", permissions.SubjectAuthenticated, "", permissions.PermWrite)

	cases := []struct {
		name   string
		user   *users.User
		action string
		path   string
		want   int
	}{
		// private/** matched: only alice's read grant counts.
		{"rule grants owner read", alice, permissions.PermRead, "private/x.md", 0},
		{"rule denies other user", bob, permissions.PermRead, "private/x.md", http.StatusForbidden},
		{"rule denies anonymous with 401", nil, permissions.PermRead, "private/x.md", http.StatusUnauthorized},
		{"rule caps owner at granted level", alice, permissions.PermWrite, "private/x.md", http.StatusForbidden},
		// wiki/** matched: write grant implies read for authenticated.
		{"write rule implies read", bob, permissions.PermRead, "wiki/x.md", 0},
		{"write rule allows write", bob, permissions.PermWrite, "wiki/x.md", 0},
		{"write rule still blocks anonymous", nil, permissions.PermRead, "wiki/x.md", http.StatusUnauthorized},
		// No rule matches: fallback defaults.
		{"fallback anon read allowed", nil, permissions.PermRead, "public/x.md", 0},
		{"fallback anon comment denied", nil, permissions.PermComment, "public/x.md", http.StatusUnauthorized},
		{"fallback user write allowed", bob, permissions.PermWrite, "public/x.md", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.user != nil {
				ctx = middleware.WithIdentity(ctx, tc.user, nil)
			}
			if _, status := a.Authorize(ctx, tc.path, tc.action); status != tc.want {
				t.Fatalf("got %d, want %d", status, tc.want)
			}
		})
	}
}

// Scopes and permission rules intersect: a scope-wide token must not bypass a
// path rule that denies its owner.
func TestAuthorizeScopePassStillHonorsRules(t *testing.T) {
	a := authzTestAPI(t)
	alice := authzTestUser(t, a, "alice@example.com")
	bob := authzTestUser(t, a, "bob@example.com")
	authzTestRule(t, a, "private/**", permissions.SubjectUser, fmt.Sprintf("%d", alice.ID), permissions.PermRead)

	ctx := middleware.WithIdentity(context.Background(), bob, []string{"read"})
	if _, status := a.Authorize(ctx, "private/x.md", permissions.PermRead); status != http.StatusForbidden {
		t.Fatalf("scope-wide token bypassed the path rule: got %d, want 403", status)
	}
}
