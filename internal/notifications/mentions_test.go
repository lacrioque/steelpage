package notifications_test

import (
	"reflect"
	"testing"

	"github.com/markusfluer/steelpage/internal/notifications"
)

func TestParseMentions(t *testing.T) {
	names := []string{"Alice", "Alice Smith", "Bob", "Al", "Markus Flür"}

	cases := []struct {
		name string
		body string
		want []string
	}{
		{"simple", "hey @Bob look at this", []string{"Bob"}},
		{"start of text", "@Alice please review", []string{"Alice"}},
		{"name with space, longest wins", "ping @Alice Smith about it", []string{"Alice Smith"}},
		{"shorter name when no longer match", "ping @Alice about it", []string{"Alice"}},
		{"email does not match", "mail me at al@example.com", nil},
		{"trailing punctuation", "thanks @Bob, appreciated", []string{"Bob"}},
		{"prefix does not split word", "@Alfred is not a user", nil},
		{"short name exact", "@Al can you check?", []string{"Al"}},
		{"multiple mentions", "@Bob and @Alice Smith please sync", []string{"Bob", "Alice Smith"}},
		{"deduped", "@Bob @Bob @Bob", []string{"Bob"}},
		{"umlauts", "cc @Markus Flür on this", []string{"Markus Flür"}},
		{"mid-word at ignored", "see foo@Bob", nil},
		{"after parenthesis", "(@Bob)", []string{"Bob"}},
		{"empty body", "", nil},
		{"at end of text", "ping @Bob", []string{"Bob"}},
		{"newline boundary", "first line\n@Alice next", []string{"Alice"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := notifications.ParseMentions(tc.body, names)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ParseMentions(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}

func TestParseMentionsNoCandidates(t *testing.T) {
	if got := notifications.ParseMentions("@Bob hi", nil); got != nil {
		t.Fatalf("expected nil with no candidates, got %v", got)
	}
}
