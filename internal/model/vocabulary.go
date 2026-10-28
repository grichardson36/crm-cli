package model

import "strings"

// User-managed vocabularies (patterns/deal-roles). A vocabulary is a pick list the user adds to,
// renames, reorders and removes values from, kept in the one table `vocabulary` keyed by kind. A field
// is backed by a vocabulary when its FieldSpec names the kind (FieldSpec.Vocabulary). The registry below
// is the only thing a new vocabulary needs: the repository, the validation and the editors are generic.
// Deal stages and note types are hardcoded or have their own table today and can move here later.

// VocabUsage says where the text of a vocabulary value is stored in other tables, so a rename can
// rewrite it and a removal can count it. Names come from this registry, never from input.
type VocabUsage struct {
	Table, Column string
	// Live is an extra SQL condition that selects the live rows (empty for all rows).
	Live string
}

// VocabSpec declares one vocabulary.
type VocabSpec struct {
	// Kind is the key in the vocabulary table.
	Kind string
	// Label names the vocabulary ("deal role"), Noun one value of it ("role").
	Label, Noun string
	// Command is the CLI command that manages it, shown in fixes ("crm deal roles").
	Command string
	// Seeds are the values a new database starts with, in pick order. Migration 013 inserts the same list.
	Seeds []string
	// Usages are the columns that hold values of the vocabulary.
	Usages []VocabUsage
	// MaxLen is the longest value in characters.
	MaxLen int
}

// DealRoleKind is the vocabulary kind of a person's sales role on a deal.
const DealRoleKind = "deal_role"

// Vocabularies is the one list of vocabularies.
var Vocabularies = []VocabSpec{
	{
		Kind: DealRoleKind, Label: "deal role", Noun: "role", Command: "crm deal roles",
		Seeds:  []string{"champion", "decision maker", "economic buyer", "influencer", "blocker", "end user", "signer", "billing contact"},
		Usages: []VocabUsage{{Table: "deal_person", Column: "role", Live: "deleted_at IS NULL"}},
		MaxLen: 80,
	},
}

// VocabularyNamed finds a vocabulary by kind.
func VocabularyNamed(kind string) (VocabSpec, bool) {
	for _, v := range Vocabularies {
		if v.Kind == kind {
			return v, true
		}
	}
	return VocabSpec{}, false
}

// VocabEntry is one value of a vocabulary.
type VocabEntry struct {
	ID       int64  `json:"id"`
	UUID     string `json:"uuid"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Position int    `json:"position"`
	// Removed values stay in the table so rows that hold the text keep showing it.
	Removed   bool   `json:"removed"`
	InUse     int    `json:"in_use"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// DealPersonKind is the key of the deal link fields in Fields (the role of a person on a deal).
const DealPersonKind = "deal_person"

// ChoicesFromNames turns vocabulary values into the options of a choice field.
func ChoicesFromNames(names []string) []FieldChoiceOption {
	out := make([]FieldChoiceOption, len(names))
	for i, n := range names {
		out[i] = FieldChoiceOption{Value: n, Label: n}
	}
	return out
}

// WithVocabulary returns the field with its options set to the current values of its vocabulary.
func (f FieldSpec) WithVocabulary(names []string) FieldSpec {
	f.Choices = ChoicesFromNames(names)
	return f
}

// CleanVocabName trims a value and folds inner runs of white space to one space.
func CleanVocabName(s string) string { return strings.Join(strings.Fields(s), " ") }
