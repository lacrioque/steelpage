# AI integration guide

How to plug AI agents into Steelpage: mint a machine token, connect an MCP client (or fall back to plain HTTP), and understand exactly what a bot can and cannot do.

> One rule governs everything below: **a bot sees exactly what its token's owner may see**. MCP, botready output, and the REST API all run through the same authorization path as the UI — token scopes intersected with path permissions, evaluated live.

---

## 1. Mint a machine token

Machine tokens are admin-managed service identities — for shared bots that aren't owned by a person. (Personal API tokens from your account page still work everywhere a machine token does; use machine tokens when the bot outlives any single user.)

1. Sign in as an admin and open `/admin → Machine tokens`.
2. Click **Create**, give the bot a name (this becomes its display name on comments it writes), pick scopes, optionally set an expiry.
3. Copy the token from the reveal dialog. **It is shown exactly once** — Steelpage stores only a hash. If you lose it, revoke and mint a new one.

Tokens look like `spt_…` and are sent as a Bearer header:

```
Authorization: Bearer spt_...
```

### Choosing scopes

Grant the least the bot needs. A scope is `read`, `comment`, or `write`, optionally suffixed with a path:

| Scope | Grants |
|---|---|
| `read` | Read every path the identity's permissions allow |
| `read:guides/**` | Read only under `guides/` |
| `comment` | Read + comment everywhere (higher scopes imply lower ones) |
| `comment:projects/acme/**` | Read + comment only under `projects/acme/` |
| `write` | Full document write access — **rarely appropriate for a bot** |

Path suffixes support an exact path or a `/**` subtree suffix. Scopes only *narrow* access: the final answer is always token scopes **∩** path permissions, so a `read` token still can't see a subtree its identity has no permission rule for.

A sensible default for an AI assistant: `comment` — it can research everything it's allowed to see and leave line-anchored feedback, but never edit a document. To confine the feedback, combine `read` with a path-scoped comment grant like `comment:projects/acme/**`.

---

## 2. Connect an MCP client

Steelpage exposes an MCP server (streamable HTTP, stateless) at two endpoints:

- **`/mcp`** — SSE-formatted responses; the default modern streamable-HTTP flavor. Use this for MCP SDKs and clients.
- **`/mcp/rest`** — identical tools, but responses are plain `application/json`. Use this from curl, scripts, or HTTP clients that don't want to parse SSE frames.

Both re-resolve identity from the `Authorization` header on every request, and both ignore session cookies entirely (see [Security notes](#6-security-notes)).

### Claude Code

```bash
claude mcp add --transport http steelpage https://wiki.example.com/mcp \
  --header "Authorization: Bearer spt_..."
```

Then ask Claude to search the wiki, fetch a page, or leave a comment — the tools show up as `search`, `get_document`, and friends.

### claude.ai (custom connector)

In claude.ai, add a custom connector (Settings → Connectors → Add custom connector) pointing at `https://wiki.example.com/mcp`. Configure the `Authorization: Bearer spt_...` header where the connector setup asks for authentication/custom headers.

### Generic streamable-HTTP client config

Most MCP clients accept a JSON block like this:

```json
{
  "mcpServers": {
    "steelpage": {
      "type": "http",
      "url": "https://wiki.example.com/mcp",
      "headers": {
        "Authorization": "Bearer spt_..."
      }
    }
  }
}
```

### Plain-JSON clients: `/mcp/rest` + curl

`/mcp/rest` answers JSON-RPC over plain HTTP with `application/json` responses — no SSE parsing needed:

```bash
# Initialize (optional for stateless use, but a good smoke test)
curl -s https://wiki.example.com/mcp/rest \
  -H "Authorization: Bearer spt_..." \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"curl","version":"0"}}}'

# List the tools
curl -s https://wiki.example.com/mcp/rest \
  -H "Authorization: Bearer spt_..." \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'

# Call a tool
curl -s https://wiki.example.com/mcp/rest \
  -H "Authorization: Bearer spt_..." \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search","arguments":{"query":"deployment checklist"}}}'
```

**The `Accept` header matters.** The MCP SDK rejects any POST with `400` unless the `Accept` header covers *both* `application/json` and `text/event-stream` — even on `/mcp/rest`, where the response is always plain JSON. Bare `curl` happens to work without the header only because its default is `Accept: */*`; any HTTP library that sends `Accept: application/json` alone will get a 400. Always send:

```
Accept: application/json, text/event-stream
```

The same commands work against `/mcp` — the only difference is that responses come back as SSE frames (`Content-Type: text/event-stream`) instead of a bare JSON body.

---

## 3. Tool reference

| Tool | Arguments | Returns | Required scope |
|---|---|---|---|
| `search` | `query`, `limit?` | Ranked full-text hits with snippet, `url`, and `botready_url` per hit; results filtered to what the caller may read | `read` |
| `get_document` | `path`, `format?` (`markdown` \| default JSON) | Default: structured JSON (title, frontmatter, markdown, sha, updated, open comments, urls). `format=markdown`: the botready-style enriched Markdown as text | `read` |
| `similar_pages` | `path`, `limit?` | Similar/connected pages `{path, title, score, reasons, url}` — link graph + shared tags + content similarity; permission-filtered | `read` |
| `list_tree` | — | The document tree, filtered to readable entries, with urls for files | `read` |
| `add_comment` | `path`, `line_start`, `line_end`, `anchor_text`, `body`, `reply_to?` | The created line-anchored comment; triggers the same notifications as a UI comment; the bot's display name is the author | `comment` |

There are deliberately **no document write tools** — bots read and comment, humans edit.

---

## 4. Anonymous read

If `auth.allow_anonymous_read` is on (live-editable in `/admin → Settings`), the read tools — `search`, `get_document`, `similar_pages`, `list_tree` — work **without any token**, subject to your `anonymous` permission rules. This makes a public wiki usable as an MCP context source with zero credential handling.

`add_comment` always requires an authenticated identity (a token) — anonymous callers get a clear "authentication required" error.

Flip `allow_anonymous_read` off and anonymous MCP reads stop immediately — the check is per-request against live config.

---

## 5. Fallbacks without MCP

The same token works against every non-MCP surface.

### Bot-ready output

Any document, as enriched Markdown or structured JSON:

```
GET /docs/<path>?botready=1              → enriched Markdown (title, path, sha, tags, content, open comments)
GET /docs/<path>?botready=1&format=json  → the same as structured JSON
```

### Plain REST API

```bash
# Full-text search
curl -s "https://wiki.example.com/api/search?q=deployment&limit=10" \
  -H "Authorization: Bearer spt_..."

# Fetch a document (rendered HTML + raw markdown + comments)
curl -s "https://wiki.example.com/api/docs/guides/deploy.md" \
  -H "Authorization: Bearer spt_..."

# Similar / connected pages
curl -s "https://wiki.example.com/api/docs-similar/guides/deploy.md?limit=10" \
  -H "Authorization: Bearer spt_..."

# Document tree
curl -s "https://wiki.example.com/api/tree" \
  -H "Authorization: Bearer spt_..."

# Leave a line-anchored comment
curl -s -X POST "https://wiki.example.com/api/comments" \
  -H "Authorization: Bearer spt_..." \
  -H "Content-Type: application/json" \
  -d '{"path":"guides/deploy.md","line_start":42,"line_end":42,"anchor_text":"the original line text","body":"This step is outdated."}'
```

---

## 6. Security notes

- **Session cookies are ignored on `/mcp` and `/mcp/rest`.** A request carrying only a browser session is treated as anonymous, never as the signed-in user. This closes the CSRF door outright — a malicious page can't ride a victim's cookie into `add_comment`. MCP callers authenticate with Bearer tokens only.
- **Effective access = token scopes ∩ path permissions.** Neither alone is sufficient. Narrowing either side takes effect on the next request.
- **Kill switch:** `mcp.enabled` (default `true`) is live-editable under `/admin → Settings`. Flip it off and both `/mcp` and `/mcp/rest` return 404 immediately — no restart, and botready/REST access is unaffected.
- **Permission parity with the UI:** every MCP tool runs the exact same authorization code path as the REST API and the SPA. Search results, tree entries, and similar-page results are filtered per entry, so a restricted subtree leaks neither content nor titles.
- Revoking a machine token (Admin → Machine tokens → revoke) cuts off the bot on its next request; both MCP endpoints are stateless, so there is no session to outlive the token.
