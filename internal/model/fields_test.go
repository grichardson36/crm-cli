package model_test

import (
	"errors"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fakeDates(raw string) (string, bool) {
	if raw == "next friday" {
		return "2026-11-06", true
	}
	return "", false
}

func TestFieldNormalize(t *testing.T) {
	stage, _ := model.FieldNamed("deal", "stage")
	value, _ := model.FieldNamed("deal", "value")
	due, _ := model.FieldNamed("task", "due")
	bday, _ := model.FieldNamed("person", "birthday")
	first, _ := model.FieldNamed("person", "first")
	last, _ := model.FieldNamed("person", "last")
	tier, _ := model.FieldNamed("person", "tier")
	notes, _ := model.FieldNamed("person", "notes")
	email, _ := model.FieldNamed("person", "email")
	org, _ := model.FieldNamed("person", "organization")

	tests := []struct {
		name     string
		field    model.FieldSpec
		in, want string
		code     string
	}{
		{"text trims", last, "  Smith ", "Smith", ""},
		{"optional text clears", last, "   ", "", ""},
		{"required text refuses empty", first, "", "", "required"},
		{"longtext keeps markdown", notes, "# hi\n- a", "# hi\n- a", ""},
		{"number plain", value, "12500", "12500", ""},
		{"number with money marks", value, "$12,500.50", "12500.5", ""},
		{"number rejects words", value, "lots", "", "not_a_number"},
		{"number below min", value, "-5", "", "below_min"},
		{"choice by value", stage, "won", "won", ""},
		{"choice by label, any case", stage, "NEGOTIATION", "negotiation", ""},
		{"choice by number", stage, "2", "prospect", ""},
		{"choice rejects other", stage, "maybe", "", "not_a_choice"},
		{"choice with a clear value takes labels", tier, "high", "2", ""},
		{"choice with a clear value takes no positions", tier, "7", "", "not_a_choice"},
		{"date iso", due, "2026-12-01", "2026-12-01", ""},
		{"date with time", due, "2026-12-01 09:30", "2026-12-01 09:30", ""},
		{"date natural phrase", due, "next friday", "2026-11-06", ""},
		{"date rejects junk", due, "someday", "", "not_a_date"},
		{"birthday month and day", bday, "03-14", "03-14", ""},
		{"birthday with year", bday, "1985-03-14", "1985-03-14", ""},
		{"month and day is not a due date", due, "03-14", "", "not_a_date"},
		{"list refuses a scalar", email, "a@b.com", "", "not_a_value"},
		{"relation refuses a scalar", org, "Acme", "", "not_a_value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.field.Normalize(tt.in, fakeDates)
			if tt.code != "" {
				require.Error(t, err)
				fe, ok := model.IsFieldError(err)
				require.True(t, ok)
				assert.Equal(t, tt.code, fe.Code)
				assert.True(t, errors.Is(err, model.ErrValidation), "exit code 2 like every validation error")
				assert.NotEmpty(t, fe.Expected)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

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
				assert.NotEmpty(t, f.Choices, id)
			case model.FieldRelation:
				assert.NotEmpty(t, f.Kind, id)
			case model.FieldList:
				assert.NotEmpty(t, f.Item, id)
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
