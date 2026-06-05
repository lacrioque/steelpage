package comments_test

import (
	"path/filepath"
	"testing"

	"github.com/markusfluer/steelpage/internal/comments"
	"github.com/markusfluer/steelpage/internal/db"
	"github.com/markusfluer/steelpage/internal/users"
)

// setup spins up a temp SQLite DB with all migrations applied and seeds three
// users for thread-participant scenarios.
func setup(t *testing.T) (*comments.Store, []*users.User) {
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
	var seeded []*users.User
	for _, name := range []string{"Alice", "Bob", "Carol"} {
		u, err := us.CreateLocal(name+"@example.com", name, "x", "user")
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		seeded = append(seeded, u)
	}
	return comments.New(d), seeded
}

func newComment(t *testing.T, s *comments.Store, authorID int64, replyTo *int64) *comments.Comment {
	t.Helper()
	c, err := s.Create(comments.CreateInput{
		Path: "guide/intro.md", LineStart: 1, LineEnd: 1,
		AnchorText: "# Intro", AuthorID: authorID, Body: "body", ReplyTo: replyTo,
	})
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}
	return c
}

func TestThreadParticipantIDs(t *testing.T) {
	s, u := setup(t)
	alice, bob, carol := u[0], u[1], u[2]

	root := newComment(t, s, alice.ID, nil)
	newComment(t, s, bob.ID, &root.ID)
	reply2 := newComment(t, s, carol.ID, &root.ID)
	// Reply to a reply — flattened to root, Carol replies twice (dedup check).
	newComment(t, s, carol.ID, &reply2.ID)

	ids, err := s.ThreadParticipantIDs(root.ID)
	if err != nil {
		t.Fatalf("participants: %v", err)
	}
	got := map[int64]bool{}
	for _, id := range ids {
		got[id] = true
	}
	if len(ids) != 3 || !got[alice.ID] || !got[bob.ID] || !got[carol.ID] {
		t.Fatalf("participants = %v, want {%d %d %d} deduped", ids, alice.ID, bob.ID, carol.ID)
	}
}

func TestThreadParticipantIDsSingleComment(t *testing.T) {
	s, u := setup(t)
	root := newComment(t, s, u[0].ID, nil)

	ids, err := s.ThreadParticipantIDs(root.ID)
	if err != nil {
		t.Fatalf("participants: %v", err)
	}
	if len(ids) != 1 || ids[0] != u[0].ID {
		t.Fatalf("participants = %v, want just the root author", ids)
	}
}
