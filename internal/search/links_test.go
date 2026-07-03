package search_test

import (
	"reflect"
	"testing"

	"github.com/markusfluer/steelpage/internal/search"
)

func TestExtractLinks(t *testing.T) {
	cases := []struct {
		name    string
		docPath string
		body    string
		want    []string
	}{
		{"relative sibling", "guide/intro.md", "[s](setup.md)", []string{"guide/setup.md"}},
		{"relative subdir", "guide/intro.md", "[s](sub/deep.md)", []string{"guide/sub/deep.md"}},
		{"parent traversal", "guide/intro.md", "[r](../root.md)", []string{"root.md"}},
		{"escapes root", "guide/intro.md", "[e](../../evil.md)", nil},
		{"absolute", "guide/intro.md", "[n](/notes/a.md)", []string{"notes/a.md"}},
		{"docs route style emits both", "guide/intro.md", "[f](/docs/foo.md)", []string{"docs/foo.md", "foo.md"}},
		{"fragment cut", "guide/intro.md", "[o](other.md#section)", []string{"guide/other.md"}},
		{"pure fragment", "guide/intro.md", "[h](#here)", nil},
		{"query cut", "guide/intro.md", "[o](other.md?raw=1)", []string{"guide/other.md"}},
		{"http scheme dropped", "guide/intro.md", "[x](https://example.com/a.md)", nil},
		{"mailto scheme dropped", "guide/intro.md", "[m](mailto:a@b.example)", nil},
		{"image skipped", "guide/intro.md", "![d](diagram.md)", nil},
		{"autolink skipped", "guide/intro.md", "<https://example.com/a.md>", nil},
		{"reference link", "guide/intro.md", "[r][ref]\n\n[ref]: other.md\n", []string{"guide/other.md"}},
		{"percent encoding", "guide/intro.md", "[p](my%20page.md)", []string{"guide/my page.md"}},
		{"self link dropped", "guide/intro.md", "[i](intro.md)", nil},
		{"non md dropped", "guide/intro.md", "[t](file.txt)", nil},
		{"root doc relative", "readme.md", "[g](guide/intro.md)", []string{"guide/intro.md"}},
		{"dedupe and sort", "guide/intro.md", "[a](z.md) [b](a.md) [c](z.md)", []string{"guide/a.md", "guide/z.md"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := search.ExtractLinks(tc.docPath, []byte(tc.body))
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ExtractLinks(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}
