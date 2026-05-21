package groups

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("group not found")
	ErrInvalid  = errors.New("group name must be 1-64 characters")
	ErrTaken    = errors.New("group name already taken")
)

type Group struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
	Members     []int64 `json:"members,omitempty"`
}

type Store struct {
	DB *sql.DB
}

func New(db *sql.DB) *Store { return &Store{DB: db} }

func (s *Store) Create(name, description string) (*Group, error) {
	clean := strings.TrimSpace(name)
	if clean == "" || len(clean) > 64 {
		return nil, ErrInvalid
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.Exec(
		`INSERT INTO groups(name, description, created_at) VALUES(?, ?, ?)`,
		clean, description, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrTaken
		}
		return nil, fmt.Errorf("insert group: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetByID(id)
}

func (s *Store) GetByID(id int64) (*Group, error) {
	row := s.DB.QueryRow(`SELECT id, name, COALESCE(description,''), created_at FROM groups WHERE id = ?`, id)
	var g Group
	if err := row.Scan(&g.ID, &g.Name, &g.Description, &g.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &g, nil
}

func (s *Store) List() ([]*Group, error) {
	rows, err := s.DB.Query(`SELECT id, name, COALESCE(description,''), created_at FROM groups ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.CreatedAt); err != nil {
			return nil, err
		}
		members, err := s.MembersOf(g.ID)
		if err != nil {
			return nil, err
		}
		g.Members = members
		out = append(out, &g)
	}
	return out, rows.Err()
}

func (s *Store) Delete(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM groups WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) AddMember(userID, groupID int64) error {
	_, err := s.DB.Exec(
		`INSERT OR IGNORE INTO user_groups(user_id, group_id) VALUES(?, ?)`,
		userID, groupID,
	)
	return err
}

func (s *Store) RemoveMember(userID, groupID int64) error {
	_, err := s.DB.Exec(
		`DELETE FROM user_groups WHERE user_id = ? AND group_id = ?`,
		userID, groupID,
	)
	return err
}

func (s *Store) MembersOf(groupID int64) ([]int64, error) {
	rows, err := s.DB.Query(`SELECT user_id FROM user_groups WHERE group_id = ?`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// GroupsOf returns the names of groups the user belongs to.
func (s *Store) GroupsOf(userID int64) ([]string, error) {
	rows, err := s.DB.Query(`
		SELECT g.name
		FROM groups g
		JOIN user_groups ug ON ug.group_id = g.id
		WHERE ug.user_id = ?
		ORDER BY g.name ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}
