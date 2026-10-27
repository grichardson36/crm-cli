package alias_test

import (
	"testing"

	"github.com/jdanielnd/crm-cli/internal/alias"
	"github.com/stretchr/testify/assert"
)

func TestExpand(t *testing.T) {
	tests := []struct {
		name  string
		table *alias.Table
		word  string
		want  []string
	}{
		{"nickname reaches full name", nil, "Zach", []string{"zach", "zachary"}},
		{"nickname rich", nil, "rich", []string{"rich", "richard"}},
		{"full name reaches nicknames", nil, "zachary", []string{"zachary", "zach", "zack", "zak"}},
		{"one hop only, rich does not reach dick", nil, "rich", []string{"rich", "richard"}},
		{"unknown word", nil, "xavier", []string{"xavier"}},
		{"empty", nil, "", []string{""}},
		{"user addition", alias.NewTable(map[string]string{"zeke": "zachary"}, nil), "zeke", []string{"zeke", "zachary"}},
		{"removed builtin", alias.NewTable(nil, map[string]bool{"zach": true}), "zach", []string{"zach"}},
		{"chain flattens to the root", alias.NewTable(map[string]string{"zeke": "zach"}, nil), "zeke", []string{"zeke", "zachary"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.table.Expand(tt.word)
			if tt.name == "full name reaches nicknames" {
				assert.ElementsMatch(t, tt.want, got)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCanonicalKeepsBuiltinNames(t *testing.T) {
	for nick, full := range alias.Builtin() {
		assert.Equal(t, full, alias.Default().Canonical(nick), nick)
	}
	assert.Equal(t, "bob2", alias.Default().Canonical("bob2"))
	assert.Equal(t, "katherine", alias.Default().Canonical("catherine"))
}

func TestKey(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Zach", "zach"}, {"  Rick ", "rick"}, {"", ""}, {"two words", ""}, {"a1", ""}, {"O'Neil", "o'neil"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, alias.Key(tt.in), tt.in)
	}
}
