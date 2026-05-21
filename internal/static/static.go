package static

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Handler returns an http.Handler that serves the SPA build from the given
// filesystem. The caller embeds the directory and hands the root in.
//
//   - Files that exist in the FS are served directly with correct MIME types.
//   - SPA deep-link routes (paths that don't resolve to a file) fall back to
//     index.html so the client router can pick them up.
//   - If index.html is missing (no production build yet), a placeholder page
//     explains how to build the frontend; the rest of the server keeps working.
func Handler(root fs.FS) http.Handler {
	if _, err := fs.Stat(root, "index.html"); err != nil {
		return placeholder("Steelpage frontend not built yet — run `cd frontend && npm run build` (or `make build`), or start Vite for development.")
	}

	fileServer := http.FileServer(http.FS(root))
	indexBytes, _ := fs.ReadFile(root, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if reqPath == "" {
			serveIndex(w, indexBytes)
			return
		}

		if f, err := root.Open(reqPath); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		serveIndex(w, indexBytes)
	})
}

func serveIndex(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(body)
}

func placeholder(message string) http.Handler {
	page := `<!doctype html><html><head><meta charset="utf-8"><title>Steelpage</title>
<style>body{font-family:system-ui,sans-serif;max-width:42rem;margin:4rem auto;padding:0 1rem;color:#172033;line-height:1.55}code{background:#eee;padding:0.1rem 0.3rem;border-radius:0.25rem}</style>
</head><body><h1>Steelpage</h1><p>` + htmlEscape(message) + `</p>
<p>API endpoints are still served (e.g. <code>/api/docs/README.md</code>, <code>/docs/README.md?botready=1</code>).</p>
</body></html>`
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	})
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;")
	return r.Replace(s)
}
