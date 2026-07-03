package users_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/markusfluer/steelpage/internal/users"
)

func TestCreateMachine(t *testing.T) {
	s := setup(t)

	u, err := s.CreateMachine("docs-bot")
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	if u.Role != users.RoleMachine {
		t.Fatalf("role = %q, want %q", u.Role, users.RoleMachine)
	}
	if u.Email != nil {
		t.Fatalf("machine user should have no email, got %v", *u.Email)
	}
	if u.PasswordHash != "" {
		t.Fatalf("machine user should have no password hash")
	}

	for _, bad := range []string{"", "   ", strings.Repeat("x", 65)} {
		if _, err := s.CreateMachine(bad); !errors.Is(err, users.ErrInvalidName) {
			t.Fatalf("CreateMachine(%q): want ErrInvalidName, got %v", bad, err)
		}
	}
}

func TestSetRoleRejectsMachine(t *testing.T) {
	s := setup(t)

	u, err := s.CreateLocal("iris@example.com", "Iris", "$2a$12$dummyhash", users.RoleUser)
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	if err := s.SetRole(u.ID, users.RoleMachine); !errors.Is(err, users.ErrInvalidRole) {
		t.Fatalf("SetRole to machine: want ErrInvalidRole, got %v", err)
	}

	// The reverse escape is blocked too: a machine row can't become a person.
	m, err := s.CreateMachine("escape-bot")
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	if err := s.SetRole(m.ID, "robot"); !errors.Is(err, users.ErrInvalidRole) {
		t.Fatalf("SetRole to unknown role: want ErrInvalidRole, got %v", err)
	}
}

func TestDeleteMachine(t *testing.T) {
	s := setup(t)

	clean, err := s.CreateMachine("clean-bot")
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	deleted, err := s.DeleteMachine(clean.ID)
	if err != nil {
		t.Fatalf("DeleteMachine: %v", err)
	}
	if !deleted {
		t.Fatalf("expected comment-free machine user to be deleted")
	}
	if _, err := s.GetByID(clean.ID); !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("row should be gone, got %v", err)
	}

	// A machine that authored comments must be retained — comments.author_id
	// has no ON DELETE clause.
	author, err := s.CreateMachine("author-bot")
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.DB.Exec(`
		INSERT INTO comments(path, line_start, line_end, author_id, body, status, created_at, updated_at)
		VALUES('doc.md', 1, 1, ?, 'beep', 'open', ?, ?)`,
		author.ID, now, now,
	); err != nil {
		t.Fatalf("insert comment: %v", err)
	}
	deleted, err = s.DeleteMachine(author.ID)
	if err != nil {
		t.Fatalf("DeleteMachine with comments: %v", err)
	}
	if deleted {
		t.Fatalf("machine user with comments must be retained")
	}
	if _, err := s.GetByID(author.ID); err != nil {
		t.Fatalf("retained row should still load: %v", err)
	}

	// Non-machine rows are never touched.
	human, err := s.CreateLocal("jane@example.com", "Jane", "$2a$12$dummyhash", users.RoleUser)
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	deleted, err = s.DeleteMachine(human.ID)
	if err != nil {
		t.Fatalf("DeleteMachine on human: %v", err)
	}
	if deleted {
		t.Fatalf("DeleteMachine must not delete non-machine users")
	}
}

func TestMentionableExcludesMachines(t *testing.T) {
	s := setup(t)

	if _, err := s.CreateLocal("kim@example.com", "Kim", "$2a$12$dummyhash", users.RoleUser); err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	if _, err := s.CreateMachine("silent-bot"); err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}

	list, err := s.Mentionable()
	if err != nil {
		t.Fatalf("Mentionable: %v", err)
	}
	if len(list) != 1 || list[0].DisplayName != "Kim" {
		t.Fatalf("expected only Kim mentionable, got %+v", list)
	}
}
