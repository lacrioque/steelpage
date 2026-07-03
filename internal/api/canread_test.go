package api

import (
	"context"
	"testing"

	"github.com/markusfluer/steelpage/internal/middleware"
)

// Regression: the old canRead ignored token scopes entirely, so a scoped
// token could list/search documents its owner may read but the token may not.
func TestCanReadHonorsTokenScopes(t *testing.T) {
	a := authzTestAPI(t)
	alice := authzTestUser(t, a, "alice@example.com")

	// Real token round-trip so the scope strings come from the store, not a
	// hand-written literal.
	tok, err := a.Tokens.Create(alice.ID, "bot", []string{"read:a/**"}, nil)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	// The owner (session-authenticated, nil scopes) can read both paths.
	session := middleware.WithIdentity(context.Background(), alice, nil)
	if !a.CanRead(session, "b/x.md") {
		t.Fatalf("owner session should read b/x.md")
	}

	scoped := middleware.WithIdentity(context.Background(), alice, tok.Scopes)
	if !a.CanRead(scoped, "a/x.md") {
		t.Fatalf("scoped token should read a/x.md (inside read:a/**)")
	}
	if a.CanRead(scoped, "b/x.md") {
		t.Fatalf("scoped token must NOT read b/x.md even though the owner can")
	}
}

// Regression: the old canRead read the cold-start a.Cfg, so flipping
// auth.allow_anonymous_read at runtime had no effect until restart.
func TestCanReadFollowsLiveAnonymousRead(t *testing.T) {
	a := authzTestAPI(t)
	ctx := context.Background()

	if !a.CanRead(ctx, "docs/x.md") {
		t.Fatalf("anon CanRead with allow_anonymous_read=true: want true")
	}

	authzTestSetAnonRead(t, a, false)
	if a.CanRead(ctx, "docs/x.md") {
		t.Fatalf("anon CanRead after live flip to false: want false (read cold-start a.Cfg?)")
	}

	authzTestSetAnonRead(t, a, true)
	if !a.CanRead(ctx, "docs/x.md") {
		t.Fatalf("anon CanRead after flip back to true: want true")
	}

	// Authenticated users keep fallback read regardless of the flag.
	authzTestSetAnonRead(t, a, false)
	alice := authzTestUser(t, a, "alice@example.com")
	session := middleware.WithIdentity(context.Background(), alice, nil)
	if !a.CanRead(session, "docs/x.md") {
		t.Fatalf("authenticated CanRead with anon read off: want true")
	}
}
