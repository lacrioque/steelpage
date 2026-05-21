package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Open opens (or creates) a SQLite database at the given path with safe
// pragmas for a single-process web app: WAL journaling, foreign keys on,
// a generous busy timeout, and NORMAL synchronous mode.
func Open(dbPath string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)", dbPath)
	d, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", dbPath, err)
	}
	if err := d.Ping(); err != nil {
		_ = d.Close()
		return nil, fmt.Errorf("ping sqlite %s: %w", dbPath, err)
	}
	return d, nil
}

// Migrate applies every embedded migration that hasn't been recorded in
// schema_migrations. Migrations are sorted by their numeric prefix
// (001_..., 002_..., etc.).
func Migrate(d *sql.DB) error {
	if _, err := d.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied, err := loadApplied(d)
	if err != nil {
		return err
	}

	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)

	applies := 0
	for _, name := range files {
		version, err := versionFromName(name)
		if err != nil {
			return fmt.Errorf("parse migration %s: %w", name, err)
		}
		if applied[version] {
			continue
		}
		body, err := fs.ReadFile(migrationFS, path.Join("migrations", name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if err := applyOne(d, version, string(body)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
		applies++
	}

	log.Printf("db: %d migration(s) applied", applies)
	return nil
}

func loadApplied(d *sql.DB) (map[int]bool, error) {
	rows, err := d.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("query schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

func applyOne(d *sql.DB, version int, body string) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(body); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`, version, time.Now().UTC().Format(time.RFC3339)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func versionFromName(name string) (int, error) {
	// Take digits up to the first underscore: "001_init.sql" -> 1
	parts := strings.SplitN(name, "_", 2)
	if len(parts) == 0 {
		return 0, fmt.Errorf("no version prefix")
	}
	return strconv.Atoi(parts[0])
}
