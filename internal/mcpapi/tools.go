package mcpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/markusfluer/steelpage/internal/api"
	"github.com/markusfluer/steelpage/internal/comments"
	"github.com/markusfluer/steelpage/internal/docs"
)

// toolError maps an Authorize status to a caller-safe error. Internal error
// strings never reach the model.
func toolError(status int) error {
	switch status {
	case http.StatusUnauthorized:
		return errors.New("authentication required: pass a machine token as 'Authorization: Bearer spt_...'")
	case http.StatusForbidden:
		return errors.New("permission denied")
	default:
		return errors.New("request denied")
	}
}

type searchIn struct {
	Query string `json:"query" jsonschema:"full-text search query"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum results, default 20, max 100"`
}

type searchHit struct {
	Path           string  `json:"path"`
	Title          string  `json:"title"`
	HeadingSnippet string  `json:"heading_snippet,omitempty"`
	BodySnippet    string  `json:"body_snippet,omitempty"`
	Rank           float64 `json:"rank"`
	URL            string  `json:"url"`
	BotReadyURL    string  `json:"botready_url"`
}

type searchOut struct {
	Results []searchHit `json:"results"`
}

func (e *env) search(ctx context.Context, _ *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, searchOut, error) {
	out := searchOut{Results: []searchHit{}}
	if in.Query == "" {
		return nil, out, errors.New("query is required")
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	// Over-fetch so permission filtering can still fill the limit — the same
	// tradeoff the REST Search handler makes.
	results, err := e.api.SearchStore.Search(in.Query, limit*3)
	if err != nil {
		return nil, out, errors.New("search failed")
	}
	ctx = e.ctx(ctx)
	for _, res := range results {
		if !e.api.CanRead(ctx, res.Path) {
			continue
		}
		u := e.api.DocURL(res.Path)
		out.Results = append(out.Results, searchHit{
			Path:           res.Path,
			Title:          res.Title,
			HeadingSnippet: res.HeadingSnippet,
			BodySnippet:    res.BodySnippet,
			Rank:           res.Rank,
			URL:            u,
			BotReadyURL:    u + "?botready=1",
		})
		if len(out.Results) >= limit {
			break
		}
	}
	return nil, out, nil
}

type getDocumentIn struct {
	Path   string `json:"path" jsonschema:"document path, e.g. guides/setup.md"`
	Format string `json:"format,omitempty" jsonschema:"'json' (default, structured) or 'markdown' (flat bot-ready text)"`
}

type getDocumentOut struct {
	Path         string              `json:"path"`
	Title        string              `json:"title"`
	Frontmatter  map[string]any      `json:"frontmatter,omitempty"`
	Markdown     string              `json:"markdown"`
	SHA          string              `json:"sha,omitempty"`
	Updated      string              `json:"updated,omitempty"`
	OpenComments []*comments.Comment `json:"open_comments"`
	URL          string              `json:"url"`
	BotReadyURL  string              `json:"botready_url"`
}

func (e *env) getDocument(ctx context.Context, _ *mcp.CallToolRequest, in getDocumentIn) (*mcp.CallToolResult, getDocumentOut, error) {
	var out getDocumentOut
	if _, status := e.api.Authorize(e.ctx(ctx), in.Path, "read"); status != 0 {
		return nil, out, toolError(status)
	}
	resp, err := e.api.BuildDocument(in.Path)
	if err != nil {
		if errors.Is(err, docs.ErrNotFound) || errors.Is(err, docs.ErrOutsideRoot) {
			return nil, out, errors.New("document not found: " + in.Path)
		}
		return nil, out, errors.New("failed to load document")
	}
	all, _ := e.api.Comments.ListByPath(in.Path)
	open := api.FilterActive(all)
	if open == nil {
		open = []*comments.Comment{}
	}

	u := e.api.DocURL(in.Path)
	out = getDocumentOut{
		Path:         resp.Path,
		Title:        resp.Title,
		Frontmatter:  resp.Frontmatter,
		Markdown:     resp.Markdown,
		SHA:          resp.SHA,
		Updated:      resp.Updated,
		OpenComments: open,
		URL:          u,
		BotReadyURL:  u + "?botready=1",
	}
	if in.Format == "markdown" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: api.BotReadyMarkdown(resp, all)}},
		}, out, nil
	}
	return nil, out, nil
}

type similarIn struct {
	Path  string `json:"path" jsonschema:"document path to find related pages for"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum results, default 10, max 50"`
}

type similarHit struct {
	Path    string   `json:"path"`
	Title   string   `json:"title"`
	Score   float64  `json:"score"`
	Reasons []string `json:"reasons"`
	URL     string   `json:"url"`
}

type similarOut struct {
	Pages []similarHit `json:"pages"`
}

func (e *env) similarPages(ctx context.Context, _ *mcp.CallToolRequest, in similarIn) (*mcp.CallToolResult, similarOut, error) {
	out := similarOut{Pages: []similarHit{}}
	ctx = e.ctx(ctx)
	if _, status := e.api.Authorize(ctx, in.Path, "read"); status != 0 {
		return nil, out, toolError(status)
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	results, err := e.api.SearchStore.RelatedTo(in.Path, limit*3)
	if err != nil {
		return nil, out, errors.New("document not indexed: " + in.Path)
	}
	for _, res := range results {
		// Related pages the caller may not read must not leak, not even
		// their titles.
		if !e.api.CanRead(ctx, res.Path) {
			continue
		}
		out.Pages = append(out.Pages, similarHit{
			Path:    res.Path,
			Title:   res.Title,
			Score:   res.Score,
			Reasons: res.Reasons,
			URL:     e.api.DocURL(res.Path),
		})
		if len(out.Pages) >= limit {
			break
		}
	}
	return nil, out, nil
}

type listTreeIn struct{}

type treeEntry struct {
	Path string `json:"path"`
	URL  string `json:"url"`
}

type listTreeOut struct {
	Entries []treeEntry `json:"entries"`
}

func (e *env) listTree(ctx context.Context, _ *mcp.CallToolRequest, _ listTreeIn) (*mcp.CallToolResult, listTreeOut, error) {
	out := listTreeOut{Entries: []treeEntry{}}
	entries, err := docs.Walk(e.api.Cfg.Repo.Path)
	if err != nil {
		return nil, out, errors.New("tree walk failed")
	}
	ctx = e.ctx(ctx)
	for _, entry := range entries {
		if !e.api.CanRead(ctx, entry.Path) {
			continue
		}
		out.Entries = append(out.Entries, treeEntry{Path: entry.Path, URL: e.api.DocURL(entry.Path)})
	}
	return nil, out, nil
}

type addCommentIn struct {
	Path       string `json:"path" jsonschema:"document path"`
	LineStart  int    `json:"line_start" jsonschema:"first source line the comment anchors to (1-based)"`
	LineEnd    int    `json:"line_end" jsonschema:"last source line (>= line_start)"`
	AnchorText string `json:"anchor_text,omitempty" jsonschema:"the anchored source text, used to re-anchor after edits"`
	Body       string `json:"body" jsonschema:"comment body (markdown, @mentions supported)"`
	ReplyTo    *int64 `json:"reply_to,omitempty" jsonschema:"parent comment id when replying"`
}

type addCommentOut struct {
	Comment *comments.Comment `json:"comment"`
}

func (e *env) addComment(ctx context.Context, _ *mcp.CallToolRequest, in addCommentIn) (*mcp.CallToolResult, addCommentOut, error) {
	var out addCommentOut
	ctx = e.ctx(ctx)
	u, status := e.api.Authorize(ctx, in.Path, "comment")
	if status != 0 {
		return nil, out, toolError(status)
	}
	// Comments need an author row — anonymous read mode can never comment.
	if u == nil {
		return nil, out, toolError(http.StatusUnauthorized)
	}

	sha, _ := e.api.Git.HeadSHA(in.Path)
	c, err := e.api.Comments.Create(comments.CreateInput{
		Path:        in.Path,
		LineStart:   in.LineStart,
		LineEnd:     in.LineEnd,
		AnchorText:  in.AnchorText,
		DocumentSHA: sha,
		AuthorID:    u.ID,
		Body:        in.Body,
		ReplyTo:     in.ReplyTo,
	})
	if err != nil {
		if errors.Is(err, comments.ErrInvalid) {
			return nil, out, errors.New("invalid comment payload: check path, line range, and body")
		}
		return nil, out, errors.New("failed to create comment")
	}
	go e.api.NotifyForComment(u, c, "", false)
	out.Comment = c
	return nil, out, nil
}
