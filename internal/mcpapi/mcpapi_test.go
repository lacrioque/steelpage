package mcpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"

	"github.com/markusfluer/steelpage/internal/api"
	"github.com/markusfluer/steelpage/internal/comments"
	"github.com/markusfluer/steelpage/internal/config"
	"github.com/markusfluer/steelpage/internal/configsvc"
	"github.com/markusfluer/steelpage/internal/db"
	"github.com/markusfluer/steelpage/internal/gitstore"
	"github.com/markusfluer/steelpage/internal/groups"
	"github.com/markusfluer/steelpage/internal/middleware"
	"github.com/markusfluer/steelpage/internal/permissions"
	"github.com/markusfluer/steelpage/internal/render"
	"github.com/markusfluer/steelpage/internal/search"
	"github.com/markusfluer/steelpage/internal/tokens"
	"github.com/markusfluer/steelpage/internal/users"
)

type fixture struct {
	api    *api.API
	srv    *httptest.Server
	cfgsvc *configsvc.Service
	sm     *scs.SessionManager
}

// setup builds a real chain — scs + Identity middleware + both MCP mounts —
// over a temp content dir and temp SQLite, mirroring server.New's wiring.
func setup(t *testing.T) *fixture {
	t.Helper()

	repo := t.TempDir()
	mustWrite(t, repo, "a/x.md", "---\ntitle: Alpha\ntags: [zebra-topic]\n---\n# Alpha\n\nshared zebra keyword here. See [beta](../b/x.md).\n")
	mustWrite(t, repo, "b/x.md", "---\ntitle: Beta\ntags: [zebra-topic]\n---\n# Beta\n\nshared zebra keyword here too.\n")

	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Migrate(d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	cfg := &config.Config{
		Repo:   config.Repo{Path: repo},
		Server: config.Server{Bind: "127.0.0.1:0", BaseURL: "http://wiki.test"},
		Auth:   config.Auth{AllowAnonymousRead: true, LocalEnabled: true},
		MCP:    config.MCP{Enabled: true},
	}
	cfgsvc, err := configsvc.New(d, cfg)
	if err != nil {
		t.Fatalf("configsvc: %v", err)
	}

	g := gitstore.New(repo)
	ustore := users.New(d)
	gstore := groups.New(d)
	cstore := comments.New(d)
	idx := search.New(d, g)
	if _, err := idx.IndexAll(repo); err != nil {
		t.Fatalf("index: %v", err)
	}
	tstore := tokens.New(d)
	sm := scs.New()

	a := api.New(cfg, render.New(cfg.Render), g, ustore, gstore, cstore, idx,
		search.NewStore(d), sm, nil, permissions.New(d), tstore, nil, cfgsvc, nil)

	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)
	r.Use(middleware.Identity(sm, ustore, gstore, tstore))
	r.Handle("/mcp", Handler(a, false))
	r.Handle("/mcp/rest", Handler(a, true))
	// Test-only login endpoint to obtain a real session cookie.
	r.Post("/test-login/{id}", func(w http.ResponseWriter, r *http.Request) {
		var id int64
		fmt.Sscan(chi.URLParam(r, "id"), &id)
		sm.Put(r.Context(), middleware.SessionUserKey, id)
		w.WriteHeader(http.StatusNoContent)
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return &fixture{api: a, srv: srv, cfgsvc: cfgsvc, sm: sm}
}

func mustWrite(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) machineToken(t *testing.T, scopes ...string) string {
	t.Helper()
	mu, err := f.api.Users.CreateMachine("test-bot")
	if err != nil {
		t.Fatalf("create machine: %v", err)
	}
	tok, err := f.api.Tokens.Create(mu.ID, "test-bot", scopes, nil)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	return tok.PlaintextSecret
}

// rpc posts a JSON-RPC message and returns the decoded response body. For
// /mcp (SSE) the data frame is extracted first.
func (f *fixture) rpc(t *testing.T, endpoint, bearer string, cookie *http.Cookie, method string, params any) (map[string]any, *http.Response) {
	t.Helper()
	msg := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		msg["params"] = params
	}
	body, _ := json.Marshal(msg)
	req, _ := http.NewRequest(http.MethodPost, f.srv.URL+endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("rpc %s: %v", method, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	raw := buf.String()
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		raw = sseData(t, raw)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	return out, resp
}

func sseData(t *testing.T, stream string) string {
	t.Helper()
	for _, line := range strings.Split(stream, "\n") {
		if strings.HasPrefix(line, "data: ") {
			return strings.TrimPrefix(line, "data: ")
		}
	}
	t.Fatalf("no data frame in SSE stream: %q", stream)
	return ""
}

func callTool(t *testing.T, f *fixture, endpoint, bearer string, cookie *http.Cookie, tool string, args map[string]any) map[string]any {
	t.Helper()
	out, resp := f.rpc(t, endpoint, bearer, cookie, "tools/call", map[string]any{"name": tool, "arguments": args})
	if out == nil {
		t.Fatalf("tools/call %s: HTTP %d", tool, resp.StatusCode)
	}
	res, ok := out["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/call %s: no result in %v", tool, out)
	}
	return res
}

func toolIsError(res map[string]any) (bool, string) {
	isErr, _ := res["isError"].(bool)
	text := ""
	if cs, ok := res["content"].([]any); ok && len(cs) > 0 {
		if c, ok := cs[0].(map[string]any); ok {
			text, _ = c["text"].(string)
		}
	}
	return isErr, text
}

func TestInitializeAndToolsList(t *testing.T) {
	f := setup(t)
	out, _ := f.rpc(t, "/mcp/rest", "", nil, "initialize", map[string]any{})
	info := out["result"].(map[string]any)["serverInfo"].(map[string]any)
	if info["name"] != "steelpage" {
		t.Fatalf("serverInfo.name = %v", info["name"])
	}

	out, _ = f.rpc(t, "/mcp/rest", "", nil, "tools/list", nil)
	toolsAny := out["result"].(map[string]any)["tools"].([]any)
	if len(toolsAny) != 5 {
		t.Fatalf("expected 5 tools, got %d", len(toolsAny))
	}
	names := map[string]bool{}
	for _, ta := range toolsAny {
		names[ta.(map[string]any)["name"].(string)] = true
	}
	for _, want := range []string{"search", "get_document", "similar_pages", "list_tree", "add_comment"} {
		if !names[want] {
			t.Fatalf("missing tool %q in %v", want, names)
		}
	}
}

func TestSearchScopedTokenFiltersResults(t *testing.T) {
	f := setup(t)
	tok := f.machineToken(t, "read:a/**")
	res := callTool(t, f, "/mcp/rest", tok, nil, "search", map[string]any{"query": "zebra"})
	sc := res["structuredContent"].(map[string]any)
	hits := sc["results"].([]any)
	if len(hits) != 1 {
		t.Fatalf("expected exactly the a/** hit, got %d: %v", len(hits), hits)
	}
	hit := hits[0].(map[string]any)
	if hit["path"] != "a/x.md" {
		t.Fatalf("path = %v", hit["path"])
	}
	if !strings.HasPrefix(hit["url"].(string), "http://wiki.test/docs/") {
		t.Fatalf("url = %v", hit["url"])
	}
	if !strings.HasSuffix(hit["botready_url"].(string), "?botready=1") {
		t.Fatalf("botready_url = %v", hit["botready_url"])
	}
}

func TestGetDocumentAnonymousFollowsLiveConfig(t *testing.T) {
	f := setup(t)
	res := callTool(t, f, "/mcp/rest", "", nil, "get_document", map[string]any{"path": "a/x.md"})
	if isErr, text := toolIsError(res); isErr {
		t.Fatalf("anonymous read should pass with anon-read on: %s", text)
	}
	sc := res["structuredContent"].(map[string]any)
	if sc["title"] != "Alpha" || !strings.Contains(sc["markdown"].(string), "zebra") {
		t.Fatalf("unexpected doc payload: %v", sc)
	}

	if err := f.cfgsvc.Set(nil, "auth.allow_anonymous_read", json.RawMessage("false")); err != nil {
		t.Fatalf("flip anon read: %v", err)
	}
	res = callTool(t, f, "/mcp/rest", "", nil, "get_document", map[string]any{"path": "a/x.md"})
	if isErr, text := toolIsError(res); !isErr || !strings.Contains(text, "authentication required") {
		t.Fatalf("expected auth error after live flip, got isErr=%v text=%q", isErr, text)
	}
}

func TestGetDocumentMarkdownFormat(t *testing.T) {
	f := setup(t)
	res := callTool(t, f, "/mcp/rest", "", nil, "get_document", map[string]any{"path": "a/x.md", "format": "markdown"})
	cs := res["content"].([]any)
	text := cs[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "# Alpha") || !strings.Contains(text, "Path: a/x.md") {
		t.Fatalf("botready text missing header: %q", text)
	}
}

func TestSimilarPagesReasonsAndFiltering(t *testing.T) {
	f := setup(t)
	res := callTool(t, f, "/mcp/rest", "", nil, "similar_pages", map[string]any{"path": "a/x.md"})
	sc := res["structuredContent"].(map[string]any)
	pages := sc["pages"].([]any)
	if len(pages) != 1 {
		t.Fatalf("expected b/x.md as related, got %v", pages)
	}
	page := pages[0].(map[string]any)
	if page["path"] != "b/x.md" {
		t.Fatalf("path = %v", page["path"])
	}
	reasons := fmt.Sprint(page["reasons"])
	if !strings.Contains(reasons, "links-to") || !strings.Contains(reasons, "shared-tag:zebra-topic") {
		t.Fatalf("reasons = %v", reasons)
	}

	// A token scoped to a/** may read the source but must not see b/**.
	tok := f.machineToken(t, "read:a/**")
	res = callTool(t, f, "/mcp/rest", tok, nil, "similar_pages", map[string]any{"path": "a/x.md"})
	sc = res["structuredContent"].(map[string]any)
	if pages := sc["pages"].([]any); len(pages) != 0 {
		t.Fatalf("scoped token must not see b/** related pages: %v", pages)
	}
}

func TestListTree(t *testing.T) {
	f := setup(t)
	res := callTool(t, f, "/mcp/rest", "", nil, "list_tree", map[string]any{})
	sc := res["structuredContent"].(map[string]any)
	if entries := sc["entries"].([]any); len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %v", entries)
	}
}

func TestAddCommentAuthMatrix(t *testing.T) {
	f := setup(t)
	args := map[string]any{"path": "a/x.md", "line_start": 5, "line_end": 5, "anchor_text": "shared zebra keyword", "body": "bot note"}

	res := callTool(t, f, "/mcp/rest", "", nil, "add_comment", args)
	if isErr, text := toolIsError(res); !isErr || !strings.Contains(text, "authentication required") {
		t.Fatalf("anonymous add_comment must fail: isErr=%v text=%q", isErr, text)
	}

	readTok := f.machineToken(t, "read")
	res = callTool(t, f, "/mcp/rest", readTok, nil, "add_comment", args)
	if isErr, text := toolIsError(res); !isErr || !strings.Contains(text, "permission denied") {
		t.Fatalf("read-scoped add_comment must be denied: isErr=%v text=%q", isErr, text)
	}

	commentTok := f.machineToken(t, "comment")
	res = callTool(t, f, "/mcp/rest", commentTok, nil, "add_comment", args)
	if isErr, text := toolIsError(res); isErr {
		t.Fatalf("comment-scoped add_comment failed: %s", text)
	}
	list, err := f.api.Comments.ListByPath("a/x.md")
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 stored comment, got %v (%v)", list, err)
	}
	if list[0].Author.DisplayName != "test-bot" {
		t.Fatalf("comment author = %q", list[0].Author.DisplayName)
	}
}

func TestSessionCookieIsIgnored(t *testing.T) {
	f := setup(t)
	admin, err := f.api.Users.CreateLocal("admin@example.com", "Admin", "$2a$12$dummyhash", users.RoleAdmin)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	resp, err := http.Post(fmt.Sprintf("%s/test-login/%d", f.srv.URL, admin.ID), "", nil)
	if err != nil {
		t.Fatalf("test login: %v", err)
	}
	resp.Body.Close()
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "session" {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatalf("no session cookie from login; got %v", resp.Cookies())
	}

	args := map[string]any{"path": "a/x.md", "line_start": 5, "line_end": 5, "anchor_text": "shared zebra keyword", "body": "csrf attempt"}
	res := callTool(t, f, "/mcp/rest", "", cookie, "add_comment", args)
	if isErr, _ := toolIsError(res); !isErr {
		t.Fatal("session cookie must not authenticate MCP calls")
	}
}

func TestKillSwitchDisablesBothEndpoints(t *testing.T) {
	f := setup(t)
	if err := f.cfgsvc.Set(nil, "mcp.enabled", json.RawMessage("false")); err != nil {
		t.Fatalf("disable mcp: %v", err)
	}
	for _, ep := range []string{"/mcp", "/mcp/rest"} {
		_, resp := f.rpc(t, ep, "", nil, "tools/list", nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s: expected 404 when disabled, got %d", ep, resp.StatusCode)
		}
	}
}

func TestStreamAndRestParity(t *testing.T) {
	f := setup(t)
	for _, tc := range []struct {
		endpoint string
		wantCT   string
	}{
		{"/mcp", "text/event-stream"},
		{"/mcp/rest", "application/json"},
	} {
		out, resp := f.rpc(t, tc.endpoint, "", nil, "tools/call", map[string]any{
			"name": "get_document", "arguments": map[string]any{"path": "a/x.md"},
		})
		if out == nil {
			t.Fatalf("%s: HTTP %d", tc.endpoint, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, tc.wantCT) {
			t.Fatalf("%s: content-type = %q, want %q", tc.endpoint, ct, tc.wantCT)
		}
		sc := out["result"].(map[string]any)["structuredContent"].(map[string]any)
		if sc["title"] != "Alpha" {
			t.Fatalf("%s: title = %v", tc.endpoint, sc["title"])
		}
	}
}
