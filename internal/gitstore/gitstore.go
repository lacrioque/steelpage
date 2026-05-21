package gitstore

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Store struct {
	RepoPath string

	mu         sync.Mutex
	lastSync   *SyncResult
	lastSyncAt time.Time
}

// SyncResult captures the outcome of a Sync (pull --rebase + optional push).
type SyncResult struct {
	Pulled        bool     `json:"pulled"`
	Pushed        bool     `json:"pushed"`
	Conflict      bool     `json:"conflict"`
	RebaseAborted bool     `json:"rebase_aborted"`
	Error         string   `json:"error,omitempty"`
	Files         []string `json:"files,omitempty"`
	At            string   `json:"at"`
}

// Status is a snapshot of repo state used by the admin UI.
type Status struct {
	Remote           string      `json:"remote"`
	HasRemote        bool        `json:"has_remote"`
	Branch           string      `json:"branch"`
	Ahead            int         `json:"ahead"`
	Behind           int         `json:"behind"`
	RebaseInProgress bool        `json:"rebase_in_progress"`
	ConflictFiles    []string    `json:"conflict_files,omitempty"`
	LastSync         *SyncResult `json:"last_sync,omitempty"`
}

func New(repoPath string) *Store {
	return &Store{RepoPath: repoPath}
}

func (s *Store) run(args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", s.RepoPath}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func (s *Store) HeadSHA(docPath string) (string, error) {
	out, err := s.run("log", "-1", "--format=%H", "--", docPath)
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(out))
	return sha, nil
}

func (s *Store) LastModified(docPath string) (time.Time, error) {
	out, err := s.run("log", "-1", "--format=%cI", "--", docPath)
	if err != nil {
		return time.Time{}, err
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, raw)
}

// Push pushes the configured branch to the given remote (default "origin").
// Errors are surfaced verbatim — callers usually run this in the background
// and log failures rather than failing the user's save.
func (s *Store) Push(remote string) error {
	if remote == "" {
		remote = "origin"
	}
	_, err := s.run("push", remote)
	return err
}

// HasRemote reports whether the working tree has a remote configured. Used
// to skip auto-push when the operator hasn't wired one up yet.
func (s *Store) HasRemote(remote string) bool {
	if remote == "" {
		remote = "origin"
	}
	_, err := s.run("remote", "get-url", remote)
	return err == nil
}

func (s *Store) Commit(docPath, message, authorName, authorEmail string) (string, error) {
	if _, err := s.run("add", "--", docPath); err != nil {
		return "", err
	}
	if _, err := s.run("diff", "--cached", "--quiet", "--", docPath); err == nil {
		return s.HeadSHA(docPath)
	} else {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return "", err
		}
	}

	author := fmt.Sprintf("%s <%s>", authorName, authorEmail)
	if _, err := s.run("commit", "--author", author, "-m", message); err != nil {
		return "", err
	}
	return s.HeadSHA(docPath)
}

// HistoryEntry is a single commit that touched a document.
type HistoryEntry struct {
	SHA         string `json:"sha"`
	AuthorName  string `json:"author_name"`
	AuthorEmail string `json:"author_email"`
	Date        string `json:"date"`
	Message     string `json:"message"`
}

// History returns up to `limit` recent commits that touched docPath. Uses
// --follow so renames are tracked across the file's lifetime.
func (s *Store) History(docPath string, limit int) ([]HistoryEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 10
	}
	out, err := s.run(
		"log",
		"--follow",
		fmt.Sprintf("-%d", limit),
		"--format=%H%x1f%an%x1f%ae%x1f%cI%x1f%s",
		"--",
		docPath,
	)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return []HistoryEntry{}, nil
	}
	lines := strings.Split(text, "\n")
	entries := make([]HistoryEntry, 0, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(line, "\x1f", 5)
		if len(parts) < 5 {
			continue
		}
		entries = append(entries, HistoryEntry{
			SHA:         parts[0],
			AuthorName:  parts[1],
			AuthorEmail: parts[2],
			Date:        parts[3],
			Message:     parts[4],
		})
	}
	return entries, nil
}

// ReadAtRef returns the file content at a given commit. `ref` must be a hex
// SHA (7-40 chars) — anything else is rejected so we can't shell out to
// arbitrary refspecs.
func (s *Store) ReadAtRef(docPath, ref string) ([]byte, error) {
	if !isHexSHA(ref) {
		return nil, fmt.Errorf("invalid ref")
	}
	return s.run("show", ref+":"+docPath)
}

func isHexSHA(s string) bool {
	if len(s) < 7 || len(s) > 40 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

// MoveFile runs `git mv from to` followed by a commit. Both paths must already
// be safe-joined by the caller. Intermediate destination directories are
// created on demand since git mv refuses to land in a missing dir.
func (s *Store) MoveFile(from, to, message, authorName, authorEmail string) (string, error) {
	destDir := filepath.Dir(to)
	if destDir != "" && destDir != "." {
		absDest := filepath.Join(s.RepoPath, destDir)
		if err := os.MkdirAll(absDest, 0o755); err != nil {
			return "", fmt.Errorf("mkdir dest: %w", err)
		}
	}
	if _, err := s.run("mv", "--", from, to); err != nil {
		return "", fmt.Errorf("git mv: %w", err)
	}
	author := fmt.Sprintf("%s <%s>", authorName, authorEmail)
	if _, err := s.run("commit", "--author", author, "-m", message); err != nil {
		return "", err
	}
	return s.HeadSHA(to)
}

// RemoveFile runs `git rm path` then commits.
func (s *Store) RemoveFile(docPath, message, authorName, authorEmail string) error {
	if _, err := s.run("rm", "--", docPath); err != nil {
		return fmt.Errorf("git rm: %w", err)
	}
	author := fmt.Sprintf("%s <%s>", authorName, authorEmail)
	if _, err := s.run("commit", "--author", author, "-m", message); err != nil {
		return err
	}
	return nil
}

// IsRebaseInProgress checks for git's rebase marker directories. When a
// `pull --rebase` hits a conflict, git leaves these behind until the user
// resolves or aborts.
func (s *Store) IsRebaseInProgress() bool {
	for _, name := range []string{"rebase-merge", "rebase-apply"} {
		if _, err := os.Stat(filepath.Join(s.RepoPath, ".git", name)); err == nil {
			return true
		}
	}
	return false
}

// ConflictFiles returns paths with unmerged entries (status 'U').
func (s *Store) ConflictFiles() []string {
	out, err := s.run("diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

// AbortRebase undoes an in-progress rebase, leaving the tree at the
// pre-pull state. Safe to call when no rebase is active.
func (s *Store) AbortRebase() error {
	if !s.IsRebaseInProgress() {
		return nil
	}
	_, err := s.run("rebase", "--abort")
	return err
}

// CurrentBranch returns the short name of HEAD's branch, or "HEAD" when
// detached.
func (s *Store) CurrentBranch() string {
	out, err := s.run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "HEAD"
	}
	return strings.TrimSpace(string(out))
}

// AheadBehind returns the number of commits HEAD is ahead/behind the
// remote-tracking branch. Falls back to (0, 0) when no upstream is set.
func (s *Store) AheadBehind() (ahead, behind int, err error) {
	out, err := s.run("rev-list", "--left-right", "--count", "HEAD...@{u}")
	if err != nil {
		// No upstream configured — not actually an error.
		return 0, 0, nil
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) != 2 {
		return 0, 0, nil
	}
	ahead, _ = strconv.Atoi(parts[0])
	behind, _ = strconv.Atoi(parts[1])
	return ahead, behind, nil
}

// PullRebase fetches the remote and rebases the current branch onto its
// upstream. Returns conflictFiles when the rebase paused due to merge
// conflicts; in that case `err` is nil but the caller must NOT proceed
// to push and should expose the conflict to the operator.
func (s *Store) PullRebase(remote string) (conflictFiles []string, err error) {
	if remote == "" {
		remote = "origin"
	}
	if !s.HasRemote(remote) {
		return nil, fmt.Errorf("remote %q not configured", remote)
	}
	if s.IsRebaseInProgress() {
		return s.ConflictFiles(), fmt.Errorf("rebase already in progress; resolve or abort first")
	}
	_, err = s.run("pull", "--rebase", remote)
	if err != nil {
		// A conflict leaves the rebase state behind — surface the files.
		if s.IsRebaseInProgress() {
			return s.ConflictFiles(), nil
		}
		return nil, err
	}
	return nil, nil
}

// Sync runs PullRebase then Push, recording the outcome for the status API.
// Conflicts halt the push and leave the rebase visible to the admin.
func (s *Store) Sync(remote string) SyncResult {
	res := SyncResult{At: time.Now().UTC().Format(time.RFC3339)}
	defer func() {
		s.mu.Lock()
		copy := res
		s.lastSync = &copy
		s.lastSyncAt = time.Now()
		s.mu.Unlock()
	}()

	if remote == "" {
		remote = "origin"
	}
	if !s.HasRemote(remote) {
		res.Error = fmt.Sprintf("remote %q not configured", remote)
		return res
	}
	conflicts, err := s.PullRebase(remote)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if len(conflicts) > 0 {
		res.Pulled = false
		res.Conflict = true
		res.Files = conflicts
		return res
	}
	res.Pulled = true
	if err := s.Push(remote); err != nil {
		res.Error = err.Error()
		return res
	}
	res.Pushed = true
	return res
}

// LastSync returns the most recent SyncResult, or nil if Sync hasn't run.
func (s *Store) LastSync() *SyncResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastSync == nil {
		return nil
	}
	copy := *s.lastSync
	return &copy
}

// SnapshotStatus collects all the bits the admin UI cares about.
func (s *Store) SnapshotStatus(remote string) Status {
	if remote == "" {
		remote = "origin"
	}
	status := Status{
		Remote:           remote,
		HasRemote:        s.HasRemote(remote),
		Branch:           s.CurrentBranch(),
		RebaseInProgress: s.IsRebaseInProgress(),
		LastSync:         s.LastSync(),
	}
	if status.HasRemote {
		ahead, behind, _ := s.AheadBehind()
		status.Ahead = ahead
		status.Behind = behind
	}
	if status.RebaseInProgress {
		status.ConflictFiles = s.ConflictFiles()
	}
	return status
}
