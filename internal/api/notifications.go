package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/markusfluer/steelpage/internal/middleware"
	"github.com/markusfluer/steelpage/internal/notifications"
	"github.com/markusfluer/steelpage/internal/users"
)

// listNotificationsResponse bundles the items with the unread count so the
// bell needs a single poll request.
type listNotificationsResponse struct {
	Items  []*notifications.Notification `json:"items"`
	Unread int                           `json:"unread"`
}

// sessionUser returns the session-authenticated user or writes the
// appropriate error. Notifications are a browser concern — API tokens are
// refused, mirroring PatchMe.
func (a *API) sessionUser(w http.ResponseWriter, r *http.Request) *users.User {
	u := middleware.FromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return nil
	}
	if middleware.TokenScopesFromContext(r.Context()) != nil {
		writeError(w, http.StatusForbidden, "session required")
		return nil
	}
	return u
}

// ListNotifications returns the caller's newest notifications plus the
// unread count.
func (a *API) ListNotifications(w http.ResponseWriter, r *http.Request) {
	u := a.sessionUser(w, r)
	if u == nil {
		return
	}
	items, err := a.Notifications.List(u.ID, 50)
	if err != nil {
		logError("list notifications", err)
		writeError(w, http.StatusInternalServerError, "failed to list notifications")
		return
	}
	if items == nil {
		items = []*notifications.Notification{}
	}
	unread, err := a.Notifications.UnreadCount(u.ID)
	if err != nil {
		logError("count notifications", err)
		writeError(w, http.StatusInternalServerError, "failed to count notifications")
		return
	}
	writeJSON(w, http.StatusOK, listNotificationsResponse{Items: items, Unread: unread})
}

// MarkNotificationRead marks one of the caller's notifications as read.
func (a *API) MarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	u := a.sessionUser(w, r)
	if u == nil {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := a.Notifications.MarkRead(u.ID, id); err != nil {
		if errors.Is(err, notifications.ErrNotFound) {
			writeError(w, http.StatusNotFound, "notification not found")
			return
		}
		logError("mark notification read", err)
		writeError(w, http.StatusInternalServerError, "failed to update notification")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MarkAllNotificationsRead marks every unread notification of the caller.
func (a *API) MarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	u := a.sessionUser(w, r)
	if u == nil {
		return
	}
	if err := a.Notifications.MarkAllRead(u.ID); err != nil {
		logError("mark all notifications read", err)
		writeError(w, http.StatusInternalServerError, "failed to update notifications")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MentionableUsers powers the @mention autocomplete in the comment composer.
// Gated by the "comment" permission on the document path, and returns only
// id + display_name — nothing the comment sidebar doesn't already show.
func (a *API) MentionableUsers(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "path query parameter required")
		return
	}
	u, status := a.authorize(r, path, "comment")
	if !denyOrContinue(w, status) {
		return
	}
	if u == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	list, err := a.Users.Mentionable()
	if err != nil {
		logError("list mentionable users", err)
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	if list == nil {
		list = []users.Mention{}
	}
	writeJSON(w, http.StatusOK, list)
}
