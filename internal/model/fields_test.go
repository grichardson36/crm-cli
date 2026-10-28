package model_test

import (
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFieldRegistryIsClosedAndComplete(t *testing.T) {
	assert.Len(t, model.FieldTypes, 7)
	for _, want := range []model.FieldType{"text", "longtext", "date", "choice", "number", "list", "relation"} {
		assert.True(t, model.ValidFieldType(want), want)
	}
	assert.False(t, model.ValidFieldType("bool"))
	for kind, specs := range model.Fields {
		seen := map[string]bool{}
		for _, f := range specs {
			id := kind + "." + f.Key
			assert.False(t, seen[f.Key], "duplicate "+id)
			seen[f.Key] = true
			assert.True(t, model.ValidFieldType(f.Type), id)
			assert.NotEmpty(t, f.Label, id)
			switch f.Type {
			case model.FieldChoice:
				if f.Vocabulary != "" {
					_, ok := model.VocabularyNamed(f.Vocabulary)
					assert.True(t, ok, id+" names a registered vocabulary")
					assert.Empty(t, f.Choices, id+" loads its options from the vocabulary")
				} else {
					assert.NotEmpty(t, f.Choices, id)
				}
			case model.FieldRelation:
				assert.NotEmpty(t, f.Kind, id)
			case model.FieldList:
				assert.NotEmpty(t, f.Item, id)
			}
			if f.Vocabulary != "" {
				assert.Equal(t, model.FieldChoice, f.Type, id)
			}
			if f.Type != model.FieldChoice {
				assert.Empty(t, f.Choices, id)
			}
		}
	}
	// Deal stages and the tier are choice fields, which is what removed their palette entries.
	stage, ok := model.FieldNamed("deal", "stage")
	require.True(t, ok)
	assert.Equal(t, model.FieldChoice, stage.Type)
	assert.Len(t, stage.Choices, len(model.DealStages))
	tier, _ := model.FieldNamed("person", "tier")
	assert.Equal(t, model.FieldChoice, tier.Type)
	assert.Equal(t, "Top", tier.ChoiceLabel("1"))
}
