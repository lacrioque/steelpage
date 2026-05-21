package docs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrOutsideRoot = errors.New("path escapes content root")
	ErrNotFound    = errors.New("document not found")
)

type TreeEntry struct {
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

func SafeJoin(root, requested string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("abs root: %w", err)
	}

	cleaned := filepath.Clean("/" + requested)
	full := filepath.Join(rootAbs, cleaned)

	fullAbs, err := filepath.Abs(full)
	if err != nil {
		return "", fmt.Errorf("abs full: %w", err)
	}

	rel, err := filepath.Rel(rootAbs, fullAbs)
	if err != nil {
		return "", ErrOutsideRoot
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrOutsideRoot
	}

	return fullAbs, nil
}

func Load(repoPath, docPath string) ([]byte, error) {
	full, err := SafeJoin(repoPath, docPath)
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(full)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return body, nil
}

func Write(repoPath, docPath string, body []byte) error {
	full, err := SafeJoin(repoPath, docPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, body, 0o644)
}

func Walk(repoPath string) ([]TreeEntry, error) {
	rootAbs, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, err
	}

	var entries []TreeEntry
	err = filepath.WalkDir(rootAbs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if name == ".git" || strings.HasPrefix(name, ".") && path != rootAbs {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			return nil
		}
		rel, err := filepath.Rel(rootAbs, path)
		if err != nil {
			return err
		}
		entries = append(entries, TreeEntry{Path: filepath.ToSlash(rel)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}
