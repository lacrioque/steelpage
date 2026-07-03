package search

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/markusfluer/steelpage/internal/frontmatter"
	"github.com/markusfluer/steelpage/internal/gitstore"
)

type Indexer struct {
	DB  *sql.DB
	Git *gitstore.Store
}

func New(db *sql.DB, git *gitstore.Store) *Indexer {
	return &Indexer{DB: db, Git: git}
}

// IndexAll walks the repo and indexes every *.md file. Called on startup.
// Returns the number of indexed documents.
func (i *Indexer) IndexAll(repoPath string) (int, error) {
	rootAbs, err := filepath.Abs(repoPath)
	if err != nil {
		return 0, err
	}
	count := 0
	walked := map[string]bool{}
	err = filepath.WalkDir(rootAbs, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		name := d.Name()
		if d.IsDir() {
			if name == ".git" || (strings.HasPrefix(name, ".") && path != rootAbs) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			return nil
		}
		rel, rerr := filepath.Rel(rootAbs, path)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		// Marked before the read so a transient read error can't prune a
		// doc that still exists on disk.
		walked[rel] = true
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			log.Printf("search: read %s: %v", rel, rerr)
			return nil
		}
		if err := i.indexFromRaw(rel, raw); err != nil {
			log.Printf("search: index %s: %v", rel, err)
		}
		count++
		return nil
	})
	if err != nil {
		return count, err
	}
	// Prune index rows for docs deleted while the server was down.
	stale, err := i.stalePaths(walked)
	if err != nil {
		return count, err
	}
	for _, p := range stale {
		if err := i.Remove(p); err != nil {
			log.Printf("search: prune %s: %v", p, err)
		}
	}
	return count, nil
}

// stalePaths returns indexed paths that were not seen by the walk.
func (i *Indexer) stalePaths(walked map[string]bool) ([]string, error) {
	rows, err := i.DB.Query(`SELECT path FROM documents`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stale []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		if !walked[p] {
			stale = append(stale, p)
		}
	}
	return stale, rows.Err()
}

// IndexOne re-indexes a single document. `body` is the markdown body the
// user just saved (no frontmatter). Frontmatter is loaded from disk so the
// index sees the canonical post-save state.
func (i *Indexer) IndexOne(repoPath, docPath, body string) error {
	full := filepath.Join(repoPath, docPath)
	raw, err := os.ReadFile(full)
	if err != nil {
		return fmt.Errorf("read %s: %w", docPath, err)
	}
	header, _, hasHeader := frontmatter.Split(raw)
	var fm map[string]any
	if hasHeader {
		fm, _ = frontmatter.Parse(header)
	}
	if fm == nil {
		fm = map[string]any{}
	}
	return i.write(docPath, fm, body)
}

func (i *Indexer) indexFromRaw(docPath string, raw []byte) error {
	header, body, hasHeader := frontmatter.Split(raw)
	var fm map[string]any
	if hasHeader {
		fm, _ = frontmatter.Parse(header)
	}
	if fm == nil {
		fm = map[string]any{}
	}
	return i.write(docPath, fm, string(body))
}

// Remove drops a document from both the cache and FTS tables. Used when a
// file is deleted or moved (caller then Indexes the new path).
func (i *Indexer) Remove(docPath string) error {
	tx, err := i.DB.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM documents WHERE path = ?`, docPath); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`DELETE FROM documents_fts WHERE path = ?`, docPath); err != nil {
		_ = tx.Rollback()
		return err
	}
	// Drop the doc's outbound links and tags. Inbound doc_links rows stay —
	// they still exist in the source documents.
	if _, err := tx.Exec(`DELETE FROM doc_links WHERE src_path = ?`, docPath); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`DELETE FROM doc_tags WHERE path = ?`, docPath); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (i *Indexer) write(docPath string, fm map[string]any, body string) error {
	extracted := FromBody(body, fm, filepath.Base(docPath))
	sha, _ := i.Git.HeadSHA(docPath)
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := i.DB.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
		INSERT INTO documents(path, title, sha, headings, tags, body, indexed_at)
		VALUES(?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			title = excluded.title,
			sha = excluded.sha,
			headings = excluded.headings,
			tags = excluded.tags,
			body = excluded.body,
			indexed_at = excluded.indexed_at`,
		docPath, extracted.Title, sha, extracted.Headings, extracted.Tags, extracted.Body, now,
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("upsert documents: %w", err)
	}

	// FTS5 has no UPSERT — delete + insert keeps the row in sync.
	if _, err := tx.Exec(`DELETE FROM documents_fts WHERE path = ?`, docPath); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete fts row: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO documents_fts(path, title, headings, body, tags)
		VALUES(?, ?, ?, ?, ?)`,
		docPath, extracted.Title, extracted.Headings, extracted.Body, extracted.Tags,
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("insert fts row: %w", err)
	}

	// Link graph + tag rows for the related-pages engine. Delete + insert
	// keeps both in sync with the doc within the same transaction.
	if _, err := tx.Exec(`DELETE FROM doc_links WHERE src_path = ?`, docPath); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete doc_links: %w", err)
	}
	for _, dst := range ExtractLinks(docPath, []byte(body)) {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO doc_links(src_path, dst_path) VALUES(?, ?)`, docPath, dst); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert doc_links: %w", err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM doc_tags WHERE path = ?`, docPath); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete doc_tags: %w", err)
	}
	for _, tag := range tagListFromFrontmatter(fm) {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO doc_tags(path, tag) VALUES(?, ?)`, docPath, tag); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert doc_tags: %w", err)
		}
	}

	return tx.Commit()
}
