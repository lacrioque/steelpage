package search

import (
	"regexp"
	"strings"
)

var (
	headingRE     = regexp.MustCompile(`(?m)^[ \t]{0,3}#{1,6}[ \t]+(.+?)[ \t]*#*[ \t]*$`)
	fencedCodeRE  = regexp.MustCompile("(?ms)^[ \\t]{0,3}```[\\w-]*[ \\t]*\\r?\\n.*?\\r?\\n[ \\t]{0,3}```[ \\t]*$")
	indentedCodeRE = regexp.MustCompile(`(?m)^(?: {4,}|\t).*$`)
	htmlTagRE     = regexp.MustCompile(`<[^>]+>`)
	whitespaceRE  = regexp.MustCompile(`\s+`)
)

// Extracted holds the fields we feed into the search index.
type Extracted struct {
	Title    string
	Headings string
	Body     string
	Tags     string
}

// FromBody builds the searchable view of a markdown body. The fallbackTitle
// is used when no frontmatter title is available and the body has no H1.
func FromBody(body string, frontmatter map[string]any, fallbackTitle string) Extracted {
	title := titleFromFrontmatter(frontmatter)
	if title == "" {
		title = firstH1(body)
	}
	if title == "" {
		title = fallbackTitle
	}

	headings := collectHeadings(body)
	tags := tagsFromFrontmatter(frontmatter)
	plain := stripForBody(body)

	return Extracted{
		Title:    title,
		Headings: headings,
		Body:     plain,
		Tags:     tags,
	}
}

func titleFromFrontmatter(fm map[string]any) string {
	if v, ok := fm["title"].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func tagsFromFrontmatter(fm map[string]any) string {
	v, ok := fm["tags"]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case []any:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					parts = append(parts, trimmed)
				}
			}
		}
		return strings.Join(parts, " ")
	case string:
		return strings.TrimSpace(t)
	default:
		return ""
	}
}

func firstH1(body string) string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
		}
	}
	return ""
}

func collectHeadings(body string) string {
	matches := headingRE.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return ""
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) >= 2 {
			text := strings.TrimSpace(m[1])
			if text != "" {
				out = append(out, text)
			}
		}
	}
	return strings.Join(out, "\n")
}

// stripForBody removes fenced/indented code blocks and HTML tags so the body
// index focuses on prose. Headings and tags are indexed separately, so we
// don't need to keep them here.
func stripForBody(body string) string {
	stripped := fencedCodeRE.ReplaceAllString(body, " ")
	stripped = indentedCodeRE.ReplaceAllString(stripped, " ")
	stripped = htmlTagRE.ReplaceAllString(stripped, " ")
	stripped = whitespaceRE.ReplaceAllString(stripped, " ")
	return strings.TrimSpace(stripped)
}
