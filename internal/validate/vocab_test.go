package validate_test

import (
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/validate"
	"github.com/stretchr/testify/assert"
)

func TestVocabChoiceField(t *testing.T) {
	roleSpec := spec(t, model.DealPersonKind, "role").WithVocabulary([]string{"champion", "decision maker"})
	tests := []struct {
		name       string
		raw        string
		create     bool
		wantValue  string
		wantCode   string
		wantCreate bool
	}{
		{"listed", "champion", false, "champion", "", false},
		{"listed other case", "Decision Maker", false, "decision maker", "", false},
		{"empty clears", "  ", false, "", "", false},
		{"unknown refused", "kingmaker", false, "", validate.CodeVocabUnknown, false},
		{"unknown offered when create is on", "  king   maker ", true, "king maker", "", true},
		{"listed with create does not offer", "champion", true, "champion", "", false},
		{"create checks the name", "a|b", true, "", validate.CodeVocabInvalid, false},
		{"too long name", string(make([]rune, 0)) + longName(), true, "", validate.CodeVocabInvalid, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := validate.Field(roleSpec, tt.raw, validate.Options{Create: tt.create})
			if tt.wantCode != "" {
				if assert.False(t, res.OK()) {
					assert.Equal(t, tt.wantCode, res.Errors()[0].Code)
				}
				return
			}
			assert.True(t, res.OK())
			assert.Equal(t, tt.wantValue, res.Value)
			offered := false
			for _, s := range res.Suggestions {
				if s.Code == validate.CodeVocabCreate {
					offered = true
					assert.Equal(t, "Create role "+tt.wantValue, s.Message)
				}
			}
			assert.Equal(t, tt.wantCreate, offered)
		})
	}
	t.Run("the unknown fix names the command and the switch", func(t *testing.T) {
		res := validate.Field(roleSpec, "kingmaker", validate.Options{})
		fix := res.Errors()[0].Fix
		assert.Contains(t, fix, "crm deal roles add")
		assert.Contains(t, fix, "--create")
		assert.Contains(t, res.Err().Error(), validate.CodeVocabUnknown)
	})
}

func longName() string {
	out := ""
	for i := 0; i < 600; i++ {
		out += "x"
	}
	return out
}

func TestVocabName(t *testing.T) {
	tests := []struct {
		name, in, want, wantCode string
	}{
		{"plain", "end user", "end user", ""},
		{"spaces folded", "  end   user ", "end user", ""},
		{"empty", "", "", validate.CodeVocabInvalid},
		{"pipe", "a|b", "", validate.CodeVocabInvalid},
		{"control", "a\x01b", "", validate.CodeVocabInvalid},
		{"81 characters", "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", "", validate.CodeVocabInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validate.VocabName(model.DealRoleKind, tt.in)
			if tt.wantCode != "" {
				e, ok := validate.As(err)
				if assert.True(t, ok) {
					assert.Equal(t, tt.wantCode, e.First().Code)
				}
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
