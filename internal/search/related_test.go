package search_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/markusfluer/steelpage/internal/db"
	"github.com/markusfluer/steelpage/internal/gitstore"
	"github.com/markusfluer/steelpage/internal/search"
)

// setupIndexed writes the fixture files into a temp repo, indexes them into a
// temp SQLite DB with all migrations applied, and returns the pieces. The
// repo is not a git repo — HeadSHA errors are swallowed by the indexer.
func setupIndexed(t *testing.T, files map[string]string) (string, *search.Indexer, *search.Store, *sql.DB) {
	t.Helper()
	repo := t.TempDir()
	for p, content := range files {
		full := filepath.Join(repo, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.Migrate(d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	idx := search.New(d, gitstore.New(repo))
	if _, err := idx.IndexAll(repo); err != nil {
		t.Fatalf("index all: %v", err)
	}
	return repo, idx, search.NewStore(d), d
}

// rankingFixture isolates one signal per page relative to hub.md. The 2-rune
// tags (ml, q1, z9) are deliberately too short to become more-like-this
// terms, so tag pages get no content-signal noise.
func rankingFixture() map[string]string {
	return map[string]string{
		"hub.md": `---
title: Wombat Handbook
tags:
  - ml
  - q1
  - z9
---
# Wombat Handbook

## Wombat care

See [a](a.md) and [b](b.md).
`,
		// Bidirectional: hub links a, a links hub. Score 3+3+2 = 8.
		"a.md": "---\ntitle: Alpha\n---\nBack to [hub](hub.md).\n",
		// One-way outbound only. Score 3.
		"b.md": "---\ntitle: Beta\n---\nNothing else.\n",
		// Three shared tags. Score 1.5*3 = 4.5.
		"c2.md": "---\ntitle: Gamma\ntags: [ml, q1, z9]\n---\nUnrelated text.\n",
		// One shared tag. Score 1.5.
		"c1.md": "---\ntitle: Epsilon\ntags: [ml]\n---\nUnrelated text.\n",
		// Content-only match on hub's title/heading terms. Score 2.5*1.0.
		"d.md": "---\ntitle: Delta\n---\nThe wombat handbook explains wombat care.\n",
	}
}

func TestRelatedToRankingAndReasons(t *testing.T) {
	_, _, store, _ := setupIndexed(t, rankingFixture())

	got, err := store.RelatedTo("hub.md", 0)
	if err != nil {
		t.Fatalf("RelatedTo: %v", err)
	}

	wantOrder := []string{"a.md", "c2.md", "b.md", "d.md", "c1.md"}
	var gotOrder []string
	for _, r := range got {
		gotOrder = append(gotOrder, r.Path)
	}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Fatalf("order = %v, want %v (results %+v)", gotOrder, wantOrder, got)
	}

	wantReasons := map[string][]string{
		"a.md":  {"links-to", "linked-from"},
		"b.md":  {"links-to"},
		"c2.md": {"shared-tag:ml", "shared-tag:q1", "shared-tag:z9"},
		"c1.md": {"shared-tag:ml"},
		"d.md":  {"similar-content"},
	}
	wantScores := map[string]float64{
		"a.md": 8.0, "b.md": 3.0, "c2.md": 4.5, "c1.md": 1.5, "d.md": 2.5,
	}
	for _, r := range got {
		if !reflect.DeepEqual(r.Reasons, wantReasons[r.Path]) {
			t.Errorf("%s reasons = %v, want %v", r.Path, r.Reasons, wantReasons[r.Path])
		}
		if r.Score != wantScores[r.Path] {
			t.Errorf("%s score = %v, want %v", r.Path, r.Score, wantScores[r.Path])
		}
	}
	for _, r := range got {
		if r.Path == "a.md" && r.Title != "Alpha" {
			t.Errorf("a.md title = %q, want Alpha", r.Title)
		}
	}
}

func TestRelatedToLimit(t *testing.T) {
	_, _, store, _ := setupIndexed(t, rankingFixture())
	got, err := store.RelatedTo("hub.md", 2)
	if err != nil {
		t.Fatalf("RelatedTo: %v", err)
	}
	if len(got) != 2 || got[0].Path != "a.md" || got[1].Path != "c2.md" {
		t.Fatalf("limit 2 = %+v, want top two a.md, c2.md", got)
	}
}

func TestRelatedToMultiWordTagStaysWhole(t *testing.T) {
	_, _, store, _ := setupIndexed(t, map[string]string{
		"ml1.md": "---\ntitle: One\ntags:\n  - Machine Learning\n---\nBody one.\n",
		"ml2.md": "---\ntitle: Two\ntags:\n  - machine learning\n---\nBody two.\n",
	})
	got, err := store.RelatedTo("ml1.md", 0)
	if err != nil {
		t.Fatalf("RelatedTo: %v", err)
	}
	if len(got) != 1 || got[0].Path != "ml2.md" {
		t.Fatalf("results = %+v, want just ml2.md", got)
	}
	found := false
	for _, reason := range got[0].Reasons {
		if reason == "shared-tag:machine learning" {
			found = true
		}
		if strings.HasPrefix(reason, "shared-tag:") && reason != "shared-tag:machine learning" {
			t.Errorf("fractured tag reason %q", reason)
		}
	}
	if !found {
		t.Fatalf("reasons = %v, want shared-tag:machine learning (whole, lowercased)", got[0].Reasons)
	}
}

func TestRelatedToSharedTagCap(t *testing.T) {
	_, _, store, _ := setupIndexed(t, map[string]string{
		"t1.md": "---\ntitle: TagsOne\ntags: [b1, b2, b3, b4, b5, b6]\n---\nx\n",
		"t2.md": "---\ntitle: TagsTwo\ntags: [b1, b2, b3, b4, b5, b6]\n---\ny\n",
	})
	got, err := store.RelatedTo("t1.md", 0)
	if err != nil {
		t.Fatalf("RelatedTo: %v", err)
	}
	if len(got) != 1 || got[0].Path != "t2.md" {
		t.Fatalf("results = %+v, want just t2.md", got)
	}
	want := []string{"shared-tag:b1", "shared-tag:b2", "shared-tag:b3", "shared-tag:b4"}
	if !reflect.DeepEqual(got[0].Reasons, want) {
		t.Fatalf("reasons = %v, want %v (cap 4, sorted)", got[0].Reasons, want)
	}
	if got[0].Score != 6.0 {
		t.Fatalf("score = %v, want 6.0 (1.5 * capped 4)", got[0].Score)
	}
}

func TestRelatedToNotIndexed(t *testing.T) {
	_, _, store, _ := setupIndexed(t, rankingFixture())
	if _, err := store.RelatedTo("nope.md", 0); !errors.Is(err, search.ErrNotIndexed) {
		t.Fatalf("err = %v, want ErrNotIndexed", err)
	}
}

func TestRelatedToDeterministic(t *testing.T) {
	_, _, store, _ := setupIndexed(t, rankingFixture())
	first, err := store.RelatedTo("hub.md", 0)
	if err != nil {
		t.Fatalf("RelatedTo: %v", err)
	}
	second, err := store.RelatedTo("hub.md", 0)
	if err != nil {
		t.Fatalf("RelatedTo: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("non-deterministic results:\n%+v\n%+v", first, second)
	}
}

func TestIndexAllPrunesStaleDocs(t *testing.T) {
	repo, idx, store, _ := setupIndexed(t, rankingFixture())

	if err := os.Remove(filepath.Join(repo, "d.md")); err != nil {
		t.Fatalf("remove fixture: %v", err)
	}
	if _, err := idx.IndexAll(repo); err != nil {
		t.Fatalf("reindex: %v", err)
	}

	if _, err := store.RelatedTo("d.md", 0); !errors.Is(err, search.ErrNotIndexed) {
		t.Fatalf("err = %v, want ErrNotIndexed after prune", err)
	}
	got, err := store.RelatedTo("hub.md", 0)
	if err != nil {
		t.Fatalf("RelatedTo: %v", err)
	}
	for _, r := range got {
		if r.Path == "d.md" {
			t.Fatalf("pruned doc still in results: %+v", got)
		}
	}
}

func TestRemoveCleansLinkAndTagRows(t *testing.T) {
	_, idx, _, d := setupIndexed(t, rankingFixture())

	if err := idx.Remove("hub.md"); err != nil {
		t.Fatalf("remove: %v", err)
	}

	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := d.QueryRow(query, args...).Scan(&n); err != nil {
			t.Fatalf("count query: %v", err)
		}
		return n
	}
	if n := count(`SELECT COUNT(*) FROM doc_links WHERE src_path = ?`, "hub.md"); n != 0 {
		t.Errorf("outbound links after Remove = %d, want 0", n)
	}
	if n := count(`SELECT COUNT(*) FROM doc_tags WHERE path = ?`, "hub.md"); n != 0 {
		t.Errorf("tag rows after Remove = %d, want 0", n)
	}
	// Inbound rows belong to the linking docs and must survive.
	if n := count(`SELECT COUNT(*) FROM doc_links WHERE dst_path = ?`, "hub.md"); n == 0 {
		t.Errorf("inbound links after Remove = 0, want a.md's link retained")
	}
}
