package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/markusfluer/steelpage/internal/search"
)

// SimilarDocs returns pages similar or connected to the requested document
// (link graph + shared tags + content terms), filtered by the caller's read
// permission like Search is.
func (a *API) SimilarDocs(w http.ResponseWriter, r *http.Request) {
	docPath := chi.URLParam(r, "*")
	if _, status := a.Authorize(r.Context(), docPath, "read"); !denyOrContinue(w, status) {
		return
	}

	limit := 10
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 50 {
		limit = 50
	}

	// Over-fetch so permission filtering can still fill the limit — same
	// tradeoff as Search.
	results, err := a.SearchStore.RelatedTo(docPath, limit*3)
	if err != nil {
		if errors.Is(err, search.ErrNotIndexed) {
			writeError(w, http.StatusNotFound, "document not indexed")
			return
		}
		logError("related pages", err)
		writeError(w, http.StatusInternalServerError, "related lookup failed")
		return
	}

	filtered := make([]search.RelatedPage, 0, limit)
	for _, res := range results {
		if a.CanRead(r.Context(), res.Path) {
			filtered = append(filtered, res)
			if len(filtered) >= limit {
				break
			}
		}
	}
	writeJSON(w, http.StatusOK, filtered)
}

// reindexAll rebuilds the whole search index (documents, FTS, links, tags)
// from the working tree. Concurrent triggers collapse into one run.
func (a *API) reindexAll() {
	if !a.reindexing.CompareAndSwap(false, true) {
		return
	}
	defer a.reindexing.Store(false)
	if _, err := a.Indexer.IndexAll(a.Cfg.Repo.Path); err != nil {
		logError("reindex after sync", err)
	}
}

// syncAndReindex runs a background pull-rebase + push and, when the pull
// actually incorporated remote commits, rebuilds the index so search and the
// link graph reflect remote edits without a restart.
func (a *API) syncAndReindex(remote string) {
	go func() {
		result := a.Git.Sync(remote)
		if result.Error != "" {
			logError("git sync", fmt.Errorf("%s", result.Error))
		}
		if result.Conflict {
			logError("git sync conflict", fmt.Errorf("conflict on %v", result.Files))
		}
		if result.Pulled && result.RemoteChanged {
			a.reindexAll()
		}
	}()
}
