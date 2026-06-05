package notifications

import (
	"sort"
	"strings"
	"unicode"
)

// ParseMentions scans body for "@<display name>" tokens and returns the
// display names that matched, deduped, in order of first appearance.
//
// Display names may contain spaces and are not unique, so matching is done
// against the full candidate set with longest-match-wins after each '@':
// given users "Alice" and "Alice Smith", the text "@Alice Smith" mentions
// only "Alice Smith".
//
// A '@' only opens a mention when it sits at a word boundary (start of text,
// after whitespace, or after punctuation like '(') — so e-mail addresses in
// the text ("a@b.com") never match. A candidate only matches when the
// character following it is end-of-text, whitespace, or punctuation — so
// "@Alfred" never half-matches the user "Al".
func ParseMentions(body string, names []string) []string {
	if body == "" || len(names) == 0 {
		return nil
	}

	// Longest first so "Alice Smith" beats "Alice".
	candidates := make([]string, 0, len(names))
	for _, n := range names {
		if strings.TrimSpace(n) != "" {
			candidates = append(candidates, n)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return len(candidates[i]) > len(candidates[j])
	})

	runes := []rune(body)
	seen := make(map[string]bool)
	var out []string

	for i := 0; i < len(runes); i++ {
		if runes[i] != '@' {
			continue
		}
		if i > 0 && !isBoundary(runes[i-1]) {
			continue // mid-word '@' (e.g. inside an email address)
		}
		rest := string(runes[i+1:])
		for _, name := range candidates {
			if !strings.HasPrefix(rest, name) {
				continue
			}
			tail := rest[len(name):]
			if tail != "" {
				r := []rune(tail)[0]
				if unicode.IsLetter(r) || unicode.IsDigit(r) {
					continue // would split a longer word, e.g. @Alfred vs "Al"
				}
			}
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
			i += len([]rune(name)) // skip past the match (loop adds 1 for '@')
			break
		}
	}
	return out
}

// isBoundary reports whether r may directly precede the '@' of a mention.
func isBoundary(r rune) bool {
	return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
}
