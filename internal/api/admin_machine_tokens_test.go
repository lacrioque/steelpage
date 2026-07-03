package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/markusfluer/steelpage/internal/comments"
	"github.com/markusfluer/steelpage/internal/db"
	"github.com/markusfluer/steelpage/internal/groups"
	"github.com/markusfluer/steelpage/internal/middleware"
	"github.com/markusfluer/steelpage/internal/tokens"
	"github.com/markusfluer/steelpage/internal/users"
)

// machineTestSetup builds a minimal API (stores only — no git/render/search)
// plus a router mirroring the RequireAdmin group wiring for the endpoints
// under test.
func machineTestSetup(t *testing.T) (*API, http.Handler) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Migrate(d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	a := &API{
		Users:    users.New(d),
		Groups:   groups.New(d),
		Tokens:   tokens.New(d),
		Comments: comments.New(d),
		saveLks:  make(map[string]*sync.Mutex),
	}

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAdmin)
		r.Get("/api/admin/machine-tokens", a.AdminListMachineTokens)
		r.Post("/api/admin/machine-tokens", a.AdminCreateMachineToken)
		r.Delete("/api/admin/machine-tokens/{id}", a.AdminDeleteMachineToken)
		r.Get("/api/admin/users", a.AdminListUsers)
		r.Patch("/api/admin/users/{id}", a.AdminPatchUser)
		r.Post("/api/admin/users/{id}/mfa/disable", a.AdminDisableUserMFA)
	})
	return a, r
}

func machineTestAdmin(t *testing.T, a *API) *users.User {
	t.Helper()
	u, err := a.Users.CreateLocal("admin@example.com", "Admin", "$2a$12$dummyhash", users.RoleAdmin)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	return u
}

// machineTestDo issues a request with the given identity pre-attached, the
// way the Identity middleware would. scopes non-nil marks the caller as
// token-authenticated.
func machineTestDo(t *testing.T, h http.Handler, method, target string, body any, u *users.User, scopes []string) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rd = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, target, rd)
	req = req.WithContext(middleware.WithIdentity(req.Context(), u, scopes))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func machineTestCreate(t *testing.T, h http.Handler, admin *users.User, name string, scopes []string) *tokens.MachineToken {
	t.Helper()
	rec := machineTestDo(t, h, http.MethodPost, "/api/admin/machine-tokens",
		map[string]any{"name": name, "scopes": scopes}, admin, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create machine token: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var mt tokens.MachineToken
	if err := json.Unmarshal(rec.Body.Bytes(), &mt); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	return &mt
}

func TestAdminCreateMachineToken(t *testing.T) {
	a, h := machineTestSetup(t)
	admin := machineTestAdmin(t, a)

	mt := machineTestCreate(t, h, admin, "docs-bot", []string{"read", "comment:guides/**"})
	if !strings.HasPrefix(mt.PlaintextSecret, tokens.TokenPrefix) {
		t.Fatalf("plaintext = %q, want %q prefix", mt.PlaintextSecret, tokens.TokenPrefix)
	}
	if mt.DisplayName != "docs-bot" {
		t.Fatalf("display_name = %q, want docs-bot", mt.DisplayName)
	}
	owner, err := a.Users.GetByID(mt.UserID)
	if err != nil {
		t.Fatalf("backing user missing: %v", err)
	}
	if owner.Role != users.RoleMachine {
		t.Fatalf("owner role = %q, want machine", owner.Role)
	}

	// Machine identities stay invisible to the regular user surfaces.
	rec := machineTestDo(t, h, http.MethodGet, "/api/admin/users", nil, admin, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list users: status = %d", rec.Code)
	}
	var listed []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode users: %v", err)
	}
	for _, u := range listed {
		if u["role"] == users.RoleMachine {
			t.Fatalf("machine user leaked into admin users list: %v", u)
		}
	}
	mentions, err := a.Users.Mentionable()
	if err != nil {
		t.Fatalf("mentionable: %v", err)
	}
	for _, m := range mentions {
		if m.ID == mt.UserID {
			t.Fatalf("machine user leaked into mentionable list")
		}
	}
}

func TestAdminCreateMachineToken_BadInput(t *testing.T) {
	a, h := machineTestSetup(t)
	admin := machineTestAdmin(t, a)

	tests := []struct {
		name string
		body map[string]any
	}{
		{"empty name", map[string]any{"name": "", "scopes": []string{"read"}}},
		{"no scopes", map[string]any{"name": "bot", "scopes": []string{}}},
		{"bad scope", map[string]any{"name": "bot", "scopes": []string{"admin"}}},
		{"bad expiry", map[string]any{"name": "bot", "scopes": []string{"read"}, "expires_at": "tomorrow"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := machineTestDo(t, h, http.MethodPost, "/api/admin/machine-tokens", tc.body, admin, nil)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
		})
	}
	// Failed mints must not leave orphan machine identities behind.
	list, err := a.Tokens.ListMachine()
	if err != nil {
		t.Fatalf("list machine tokens: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no machine tokens, got %d", len(list))
	}
	all, err := a.Users.ListAll()
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	for _, u := range all {
		if u.Role == users.RoleMachine {
			t.Fatalf("orphan machine user left behind: %s", u.DisplayName)
		}
	}
}

func TestAdminCreateMachineToken_RefusesTokenAuth(t *testing.T) {
	a, h := machineTestSetup(t)
	admin := machineTestAdmin(t, a)

	rec := machineTestDo(t, h, http.MethodPost, "/api/admin/machine-tokens",
		map[string]any{"name": "bot", "scopes": []string{"read"}}, admin, []string{"read", "comment", "write"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("token-authenticated mint: status = %d, want 403", rec.Code)
	}
}

func TestAdminMachineTokens_RequireAdmin(t *testing.T) {
	a, h := machineTestSetup(t)
	user, err := a.Users.CreateLocal("user@example.com", "User", "$2a$12$dummyhash", users.RoleUser)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	rec := machineTestDo(t, h, http.MethodPost, "/api/admin/machine-tokens",
		map[string]any{"name": "bot", "scopes": []string{"read"}}, user, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin: status = %d, want 403", rec.Code)
	}
	rec = machineTestDo(t, h, http.MethodGet, "/api/admin/machine-tokens", nil, nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: status = %d, want 401", rec.Code)
	}
}

func TestAdminListMachineTokens(t *testing.T) {
	a, h := machineTestSetup(t)
	admin := machineTestAdmin(t, a)
	created := machineTestCreate(t, h, admin, "list-bot", []string{"read"})

	rec := machineTestDo(t, h, http.MethodGet, "/api/admin/machine-tokens", nil, admin, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status = %d", rec.Code)
	}
	var list []tokens.MachineToken
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID || list[0].DisplayName != "list-bot" {
		t.Fatalf("unexpected list: %+v", list)
	}
	if list[0].PlaintextSecret != "" || strings.Contains(rec.Body.String(), "plaintext") {
		t.Fatalf("list must never contain plaintext secrets: %s", rec.Body.String())
	}
}

func TestAdminDeleteMachineToken(t *testing.T) {
	a, h := machineTestSetup(t)
	admin := machineTestAdmin(t, a)
	mt := machineTestCreate(t, h, admin, "delete-bot", []string{"read"})

	rec := machineTestDo(t, h, http.MethodDelete, "/api/admin/machine-tokens/"+strconv.FormatInt(mt.ID, 10), nil, admin, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := a.Tokens.GetByID(mt.ID); !errors.Is(err, tokens.ErrNotFound) {
		t.Fatalf("token should be gone, got %v", err)
	}
	if _, err := a.Users.GetByID(mt.UserID); !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("comment-free machine user should be gone, got %v", err)
	}
}

func TestAdminDeleteMachineToken_RetainsCommentAuthor(t *testing.T) {
	a, h := machineTestSetup(t)
	admin := machineTestAdmin(t, a)
	mt := machineTestCreate(t, h, admin, "commenter-bot", []string{"read", "comment"})

	if _, err := a.Comments.Create(comments.CreateInput{
		Path: "doc.md", LineStart: 1, LineEnd: 1, AuthorID: mt.UserID, Body: "beep",
	}); err != nil {
		t.Fatalf("create comment: %v", err)
	}

	rec := machineTestDo(t, h, http.MethodDelete, "/api/admin/machine-tokens/"+strconv.FormatInt(mt.ID, 10), nil, admin, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d", rec.Code)
	}
	if _, err := a.Tokens.GetByID(mt.ID); !errors.Is(err, tokens.ErrNotFound) {
		t.Fatalf("token should be gone, got %v", err)
	}
	// The identity survives so the comment keeps its author.
	owner, err := a.Users.GetByID(mt.UserID)
	if err != nil {
		t.Fatalf("machine user with comments must be retained: %v", err)
	}
	if owner.Role != users.RoleMachine {
		t.Fatalf("retained user role = %q, want machine", owner.Role)
	}
}

func TestAdminDeleteMachineToken_RefusesPersonalTokens(t *testing.T) {
	a, h := machineTestSetup(t)
	admin := machineTestAdmin(t, a)
	personal, err := a.Tokens.Create(admin.ID, "my-token", []string{"read"}, nil)
	if err != nil {
		t.Fatalf("create personal token: %v", err)
	}

	rec := machineTestDo(t, h, http.MethodDelete, "/api/admin/machine-tokens/"+strconv.FormatInt(personal.ID, 10), nil, admin, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("personal token via machine endpoint: status = %d, want 404", rec.Code)
	}
	if _, err := a.Tokens.GetByID(personal.ID); err != nil {
		t.Fatalf("personal token must survive: %v", err)
	}

	rec = machineTestDo(t, h, http.MethodDelete, "/api/admin/machine-tokens/99999", nil, admin, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown token: status = %d, want 404", rec.Code)
	}
}

func TestAdminUserEndpointsHideMachineUsers(t *testing.T) {
	a, h := machineTestSetup(t)
	admin := machineTestAdmin(t, a)
	mt := machineTestCreate(t, h, admin, "hidden-bot", []string{"read"})
	id := strconv.FormatInt(mt.UserID, 10)

	// PATCH role=user would escape the machine lifecycle → must 404.
	rec := machineTestDo(t, h, http.MethodPatch, "/api/admin/users/"+id,
		map[string]any{"role": "user"}, admin, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("patch machine user: status = %d, want 404", rec.Code)
	}
	owner, err := a.Users.GetByID(mt.UserID)
	if err != nil || owner.Role != users.RoleMachine {
		t.Fatalf("machine role must be untouched: %+v, %v", owner, err)
	}

	rec = machineTestDo(t, h, http.MethodPost, "/api/admin/users/"+id+"/mfa/disable", nil, admin, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("mfa-disable machine user: status = %d, want 404", rec.Code)
	}
}
