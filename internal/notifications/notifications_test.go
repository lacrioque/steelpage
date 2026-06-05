package notifications_test

import (
	"path/filepath"
	"testing"

	"github.com/markusfluer/steelpage/internal/comments"
	"github.com/markusfluer/steelpage/internal/db"
	"github.com/markusfluer/steelpage/internal/notifications"
	"github.com/markusfluer/steelpage/internal/users"
)

// setup spins up a temp SQLite DB with all migrations applied and seeds two
// users plus one comment to satisfy the foreign keys.
func setup(t *testing.T) (*notifications.Store, *users.User, *users.User, *comments.Comment) {
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

	us := users.New(d)
	alice, err := us.CreateLocal("alice@example.com", "Alice", "x", "user")
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, err := us.CreateLocal("bob@example.com", "Bob", "x", "user")
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}

	cs := comments.New(d)
	c, err := cs.Create(comments.CreateInput{
		Path: "guide/intro.md", LineStart: 1, LineEnd: 1,
		AnchorText: "# Intro", AuthorID: alice.ID, Body: "hello @Bob",
	})
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}

	return notifications.New(d), alice, bob, c
}

func TestCreateSkipsSelfNotification(t *testing.T) {
	ns, alice, _, c := setup(t)

	if err := ns.Create(alice.ID, alice.ID, c.ID, notifications.KindMention, c.Path); err != nil {
		t.Fatalf("create: %v", err)
	}
	n, err := ns.UnreadCount(alice.ID)
	if err != nil {
		t.Fatalf("unread: %v", err)
	}
	if n != 0 {
		t.Fatalf("self-notification was stored, unread = %d", n)
	}
}

func TestListAndUnread(t *testing.T) {
	ns, alice, bob, c := setup(t)

	if err := ns.Create(bob.ID, alice.ID, c.ID, notifications.KindMention, c.Path); err != nil {
		t.Fatalf("create mention: %v", err)
	}
	if err := ns.Create(bob.ID, alice.ID, c.ID, notifications.KindReply, c.Path); err != nil {
		t.Fatalf("create reply: %v", err)
	}

	items, err := ns.List(bob.ID, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(items))
	}
	// Newest first.
	if items[0].Kind != notifications.KindReply || items[1].Kind != notifications.KindMention {
		t.Fatalf("unexpected order: %s, %s", items[0].Kind, items[1].Kind)
	}
	if items[0].Actor.DisplayName != "Alice" {
		t.Fatalf("actor display name = %q", items[0].Actor.DisplayName)
	}
	if items[0].Path != c.Path || items[0].CommentID != c.ID {
		t.Fatalf("payload mismatch: %+v", items[0])
	}

	unread, err := ns.UnreadCount(bob.ID)
	if err != nil {
		t.Fatalf("unread: %v", err)
	}
	if unread != 2 {
		t.Fatalf("unread = %d, want 2", unread)
	}
	// Alice has none.
	if n, _ := ns.UnreadCount(alice.ID); n != 0 {
		t.Fatalf("alice unread = %d, want 0", n)
	}
}

func TestMarkReadEnforcesOwnership(t *testing.T) {
	ns, alice, bob, c := setup(t)

	if err := ns.Create(bob.ID, alice.ID, c.ID, notifications.KindMention, c.Path); err != nil {
		t.Fatalf("create: %v", err)
	}
	items, _ := ns.List(bob.ID, 10)
	id := items[0].ID

	// Alice cannot mark Bob's notification.
	if err := ns.MarkRead(alice.ID, id); err == nil {
		t.Fatalf("expected ErrNotFound marking someone else's notification")
	}
	if n, _ := ns.UnreadCount(bob.ID); n != 1 {
		t.Fatalf("foreign MarkRead changed unread count")
	}

	// Bob can.
	if err := ns.MarkRead(bob.ID, id); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if n, _ := ns.UnreadCount(bob.ID); n != 0 {
		t.Fatalf("unread after read = %d, want 0", n)
	}
	// Idempotent.
	if err := ns.MarkRead(bob.ID, id); err != nil {
		t.Fatalf("second mark read: %v", err)
	}
}

func TestMarkAllRead(t *testing.T) {
	ns, alice, bob, c := setup(t)

	for i := 0; i < 3; i++ {
		if err := ns.Create(bob.ID, alice.ID, c.ID, notifications.KindMention, c.Path); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	if err := ns.MarkAllRead(bob.ID); err != nil {
		t.Fatalf("mark all: %v", err)
	}
	if n, _ := ns.UnreadCount(bob.ID); n != 0 {
		t.Fatalf("unread = %d, want 0", n)
	}
	items, _ := ns.List(bob.ID, 10)
	for _, it := range items {
		if it.ReadAt == nil {
			t.Fatalf("notification %d still unread after MarkAllRead", it.ID)
		}
	}
}
