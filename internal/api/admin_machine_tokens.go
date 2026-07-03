package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/markusfluer/steelpage/internal/middleware"
	"github.com/markusfluer/steelpage/internal/tokens"
	"github.com/markusfluer/steelpage/internal/users"
)

// AdminListMachineTokens returns all machine-owned tokens (no plaintext).
func (a *API) AdminListMachineTokens(w http.ResponseWriter, _ *http.Request) {
	list, err := a.Tokens.ListMachine()
	if err != nil {
		logError("list machine tokens", err)
		writeError(w, http.StatusInternalServerError, "failed to list machine tokens")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type createMachineTokenRequest struct {
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	ExpiresAt *string  `json:"expires_at,omitempty"`
}

// AdminCreateMachineToken mints a machine identity plus its token in one
// step. The plaintext secret is returned EXACTLY ONCE.
func (a *API) AdminCreateMachineToken(w http.ResponseWriter, r *http.Request) {
	// Token-authenticated callers can't mint new tokens — too easy to
	// bootstrap a permanent backdoor that way (same rule as CreateMyToken).
	if middleware.TokenScopesFromContext(r.Context()) != nil {
		writeError(w, http.StatusForbidden, "session required to mint tokens")
		return
	}

	var req createMachineTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var expiry *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "expires_at must be RFC3339")
			return
		}
		expiry = &t
	}

	mu, err := a.Users.CreateMachine(req.Name)
	if err != nil {
		if errors.Is(err, users.ErrInvalidName) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		logError("create machine user", err)
		writeError(w, http.StatusInternalServerError, "failed to create machine identity")
		return
	}

	t, err := a.Tokens.Create(mu.ID, req.Name, req.Scopes, expiry)
	if err != nil {
		// Compensating delete — never leave a token-less machine identity
		// behind (it just created, so it can't have authored comments yet).
		if _, derr := a.Users.DeleteMachine(mu.ID); derr != nil {
			logError("rollback machine user", derr)
		}
		if errors.Is(err, tokens.ErrInvalid) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		logError("create machine token", err)
		writeError(w, http.StatusInternalServerError, "failed to create token")
		return
	}
	writeJSON(w, http.StatusCreated, &tokens.MachineToken{Token: *t, DisplayName: mu.DisplayName})
}

// AdminDeleteMachineToken revokes a machine token and retires its identity.
// Tokens owned by non-machine users 404 — this endpoint must not be able to
// revoke personal tokens.
func (a *API) AdminDeleteMachineToken(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	t, err := a.Tokens.GetByID(id)
	if err != nil {
		if errors.Is(err, tokens.ErrNotFound) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		logError("get machine token", err)
		writeError(w, http.StatusInternalServerError, "failed to load token")
		return
	}
	owner, err := a.Users.GetByID(t.UserID)
	if err != nil || owner.Role != users.RoleMachine {
		writeError(w, http.StatusNotFound, "token not found")
		return
	}
	// Token first so the credential is dead even if identity cleanup fails;
	// DeleteMachine is best-effort and keeps the row when it authored
	// comments (comments.author_id has no ON DELETE).
	if err := a.Tokens.Delete(t.UserID, t.ID); err != nil && !errors.Is(err, tokens.ErrNotFound) {
		logError("delete machine token", err)
		writeError(w, http.StatusInternalServerError, "failed to delete token")
		return
	}
	if _, err := a.Users.DeleteMachine(t.UserID); err != nil {
		logError("delete machine user", err)
	}
	w.WriteHeader(http.StatusNoContent)
}
