package api

import (
	"context"
	"net/http"

	"github.com/markusfluer/steelpage/internal/docs"
	"github.com/markusfluer/steelpage/internal/middleware"
	"github.com/markusfluer/steelpage/internal/permissions"
	"github.com/markusfluer/steelpage/internal/tokens"
)

func (a *API) Tree(w http.ResponseWriter, r *http.Request) {
	entries, err := docs.Walk(a.Cfg.Repo.Path)
	if err != nil {
		logError("tree walk", err)
		writeError(w, http.StatusInternalServerError, "tree walk failed")
		return
	}
	filtered := make([]docs.TreeEntry, 0, len(entries))
	for _, e := range entries {
		if a.CanRead(r.Context(), e.Path) {
			filtered = append(filtered, e)
		}
	}
	writeJSON(w, http.StatusOK, filtered)
}

// CanRead is the lightweight predicate used to filter list endpoints (tree,
// search, MCP tools). It mirrors Authorize for action=read but returns a bool
// so we can drop entries instead of failing the whole request.
func (a *API) CanRead(ctx context.Context, path string) bool {
	user := middleware.FromContext(ctx)
	// Token-authenticated callers can only ever see what their scopes allow,
	// exactly like Authorize's scope gate.
	if scopes := middleware.TokenScopesFromContext(ctx); scopes != nil {
		if !tokens.AllowsAction(scopes, permissions.PermRead, path) {
			return false
		}
	}
	allowed, mustFallback, err := a.Permissions.Allows(path, user, permissions.PermRead)
	if err != nil {
		return false
	}
	if !mustFallback {
		return allowed
	}
	if a.LiveCfg().Auth.AllowAnonymousRead {
		return true
	}
	return user != nil
}
