package search

import (
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

var schemeRE = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)

// ExtractLinks parses the markdown body and returns the repo-relative *.md
// paths it links to, deduped and sorted. Only inline/reference links count —
// autolinks and images are skipped. Dangling targets are kept on purpose:
// result queries JOIN documents, so they can never leak, and storing them
// keeps IndexAll order-independent. Case-sensitive targets and extensionless
// links are out of scope.
func ExtractLinks(docPath string, body []byte) []string {
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	doc := md.Parser().Parse(text.NewReader(body))

	seen := map[string]bool{}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		link, ok := n.(*ast.Link) // *ast.Image and *ast.AutoLink fail this assertion
		if !ok {
			return ast.WalkContinue, nil
		}
		for _, target := range resolveLink(docPath, string(link.Destination)) {
			seen[target] = true
		}
		return ast.WalkContinue, nil
	})

	out := make([]string, 0, len(seen))
	for target := range seen {
		out = append(out, target)
	}
	sort.Strings(out)
	return out
}

// resolveLink maps a raw link destination to zero or more repo-relative *.md
// candidates. Absolute destinations are repo-root relative; `/docs/foo.md`
// additionally yields `foo.md` because the SPA serves docs under that route.
func resolveLink(docPath, dst string) []string {
	dst = strings.TrimSpace(dst)
	if dst == "" || strings.HasPrefix(dst, "#") || schemeRE.MatchString(dst) {
		return nil
	}
	if i := strings.IndexAny(dst, "#?"); i >= 0 {
		dst = dst[:i]
	}
	if dst == "" {
		return nil
	}
	if unescaped, err := url.PathUnescape(dst); err == nil {
		dst = unescaped
	}

	var candidates []string
	if strings.HasPrefix(dst, "/") {
		cleaned := path.Clean(strings.TrimPrefix(dst, "/"))
		candidates = append(candidates, cleaned)
		if rest, ok := strings.CutPrefix(cleaned, "docs/"); ok && rest != "" {
			candidates = append(candidates, rest)
		}
	} else {
		candidates = append(candidates, path.Clean(path.Join(path.Dir(docPath), dst)))
	}

	out := candidates[:0]
	for _, c := range candidates {
		if c == "" || c == "." || c == docPath || strings.HasPrefix(c, "../") || c == ".." {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(c), ".md") {
			continue
		}
		out = append(out, c)
	}
	return out
}
