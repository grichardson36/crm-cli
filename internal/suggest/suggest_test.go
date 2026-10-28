package suggest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func name(s string) string { return s }

func TestRankTable(t *testing.T) {
	people := []string{"Zachary Adams", "Bob Baker", "Richard Cole", "Cara Cole", "Mark Twain"}
	cases := []struct {
		label string
		text  string
		limit int
		want  []string
	}{
		{"empty text returns the first in order", "", 2, []string{"Zachary Adams", "Bob Baker"}},
		{"prefix", "bo", 0, []string{"Bob Baker"}},
		{"nickname finds the full name", "zach", 0, []string{"Zachary Adams"}},
		{"nickname both ways", "rick", 0, []string{"Richard Cole"}},
		{"word prefix ranks before substring", "co", 0, []string{"Richard Cole", "Cara Cole"}},
		{"typo is forgiven", "zachery", 0, []string{"Zachary Adams"}},
		{"no match", "qqq", 0, nil},
		{"hash prefix is ignored", "#bob", 0, []string{"Bob Baker"}},
		{"limit applies", "a", 2, []string{"Zachary Adams", "Bob Baker"}},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			got := Rank(people, tc.text, name, Options{Limit: tc.limit})
			if tc.want == nil {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPrefixKeepsOrderFoldsCaseAndDropsRepeats(t *testing.T) {
	assert.Equal(t, []string{"today", "tomorrow"}, Prefix([]string{"today", "tomorrow", "today", "mon"}, "TO", 0))
	assert.Equal(t, []string{"café"}, Prefix([]string{"café", "cat"}, "cafe", 0))
	assert.Len(t, Prefix([]string{"a1", "a2", "a3"}, "a", 2), 2)
}

func TestFuzzyMatchesEveryWordAndRanksEarlyAndLoose(t *testing.T) {
	r, ok := Fuzzy("", "Theme: choose")
	assert.True(t, ok)
	assert.Equal(t, 0, r)
	early, _ := Fuzzy("theme", "Theme: choose")
	late, _ := Fuzzy("choose", "Theme: choose")
	assert.Less(t, early, late, "a word found earlier ranks better")
	two, ok := Fuzzy("choose theme", "Theme: choose")
	assert.True(t, ok, "words may come in any order")
	assert.Less(t, two, FuzzyWeak)
	loose, ok := Fuzzy("thc", "Theme: choose")
	assert.True(t, ok, "letters in order still match")
	assert.GreaterOrEqual(t, loose, FuzzyWeak)
	_, ok = Fuzzy("zzz", "Theme: choose")
	assert.False(t, ok)
	_, ok = Fuzzy("theme zzz", "Theme: choose")
	assert.False(t, ok, "every word must match")
}
