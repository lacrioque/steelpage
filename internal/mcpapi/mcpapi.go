// Package mcpapi serves the Model Context Protocol endpoints (/mcp and
// /mcp/rest) so AI systems can use Steelpage as a context source. It is a
// thin shell over the api.API facade: every tool call runs through the same
// Authorize/CanRead code path as the REST handlers, so token scopes, path
// permissions, and the live allow_anonymous_read flag apply identically.
package mcpapi

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/markusfluer/steelpage/internal/api"
	"github.com/markusfluer/steelpage/internal/middleware"
	"github.com/markusfluer/steelpage/internal/users"
)

const serverName = "steelpage"

// Handler serves streamable-HTTP MCP. Both mounts are stateless — identity
// is re-resolved from the Authorization header on every request, so no MCP
// session can outlive a token swap. jsonResponse selects the response
// flavor: false → text/event-stream (/mcp), true → application/json
// (/mcp/rest) for clients that can't consume SSE.
func Handler(a *api.API, jsonResponse bool) http.Handler {
	inner := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return buildServer(a, identityFrom(r))
	}, &mcp.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: jsonResponse,
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.LiveCfg().MCP.Enabled {
			http.Error(w, `{"error":"mcp disabled"}`, http.StatusNotFound)
			return
		}
		inner.ServeHTTP(w, r)
	})
}

// identity is the principal bound to one MCP HTTP request.
type identity struct {
	user   *users.User
	scopes []string // non-nil iff Bearer-token authenticated
}

// identityFrom accepts ONLY Bearer-token identity. A session cookie on the
// MCP endpoints is deliberately downgraded to anonymous (never 403'd): MCP
// clients don't use cookies, and ignoring them removes the whole class of
// cookie-authenticated cross-site state change (add_comment) from /mcp.
func identityFrom(r *http.Request) identity {
	if scopes := middleware.TokenScopesFromContext(r.Context()); scopes != nil {
		return identity{user: middleware.FromContext(r.Context()), scopes: scopes}
	}
	return identity{}
}

// env carries the API facade plus the request-bound identity into tool
// handlers. The SDK propagates the HTTP request context into tool calls, so
// ctx() must OVERRIDE any identity already stamped on it (WithIdentity always
// sets both keys) — otherwise a session cookie the endpoint means to ignore
// would leak back in through the request context.
type env struct {
	api *api.API
	id  identity
}

func (e *env) ctx(ctx context.Context) context.Context {
	return middleware.WithIdentity(ctx, e.id.user, e.id.scopes)
}

func buildServer(a *api.API, id identity) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: serverName, Title: "Steelpage wiki", Version: "1"}, nil)
	e := &env{api: a, id: id}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search",
		Description: "Full-text search over the wiki. Returns matching documents with snippets and canonical URLs, filtered by the caller's read permissions.",
	}, e.search)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_document",
		Description: "Fetch one document by path: title, frontmatter, full markdown, git sha/updated, and open line-anchored comments. Set format=\"markdown\" for a flat bot-ready text rendering instead of structured JSON.",
	}, e.getDocument)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "similar_pages",
		Description: "Pages similar or connected to a document, ranked by markdown links between pages, shared tags, and content-term overlap. Each hit carries reasons (links-to, linked-from, shared-tag:<tag>, similar-content).",
	}, e.similarPages)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_tree",
		Description: "List every document path the caller may read.",
	}, e.listTree)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "add_comment",
		Description: "Leave a line-anchored comment on a document. Requires an authenticated machine token with comment scope on the path.",
	}, e.addComment)
	return s
}
