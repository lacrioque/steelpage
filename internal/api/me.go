package api

import (
	"net/http"

	"github.com/markusfluer/steelpage/internal/middleware"
)

// GetMe returns the currently authenticated user (with groups), or 204.
func (a *API) GetMe(w http.ResponseWriter, r *http.Request) {
	u := middleware.FromContext(r.Context())
	if u == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if names, err := a.Groups.GroupsOf(u.ID); err == nil {
		u.Groups = names
	}
	writeJSON(w, http.StatusOK, u)
}
