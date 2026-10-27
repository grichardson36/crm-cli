package model

import (
	"strings"
)

// The field-type vocabulary (owner decision 2026-10-28, patterns/field-types-drive-editors). The
// schema declares one type per field from a closed list. The type decides the editor in the TUI, the
// check at every door (CLI, MCP, imports) and the hint shown while typing. A program built on the
// framework registers its fields here and reuses the types.

// FieldType is one entry of the closed list of field types.
type FieldType string

const (
	// FieldText is one line of text.
	FieldText FieldType = "text"
	// FieldLongText is markdown, edited in place over several lines.
	FieldLongText FieldType = "longtext"
	// FieldDate is a date. It accepts natural phrases ("next friday") through the date parser.
	FieldDate FieldType = "date"
	// FieldChoice is one value from a fixed pick list.
	FieldChoice FieldType = "choice"
	// FieldNumber is a number. A leading $ and thousands separators are ignored.
	FieldNumber FieldType = "number"
	// FieldList is several values of one kind (phones, emails), each its own row.
	FieldList FieldType = "list"
	// FieldRelation is a link to another object, picked or created from a typeahead.
	FieldRelation FieldType = "relation"
)

// FieldTypes is the closed list. Nothing else is a field type.
var FieldTypes = []FieldType{FieldText, FieldLongText, FieldDate, FieldChoice, FieldNumber, FieldList, FieldRelation}

// ValidFieldType reports whether t is on the closed list.
func ValidFieldType(t FieldType) bool {
	for _, x := range FieldTypes {
		if x == t {
			return true
		}
	}
	return false
}

// Value shapes a field can require (FieldSpec.Format).
const (
	FormatEmail  = "email"
	FormatURL    = "url"
	FormatPhone  = "phone"
	FormatDomain = "domain"
)

// FieldChoiceOption is one entry of a choice field.
type FieldChoiceOption struct {
	// Value is what is stored and sent over the CLI and MCP. Label is what the picker shows.
	Value, Label string
}

// FieldSpec declares one field of an object kind.
type FieldSpec struct {
	Key   string
	Label string
	Type  FieldType
	// Required fields cannot be emptied.
	Required bool
	// Choices lists the options of a choice field, in pick order. Clear is the value that empties the field.
	Choices []FieldChoiceOption
	Clear   string
	// Kind is the object kind at the far end of a relation field ("organization", "person", "tag").
	Kind string
	// Item is the word for one entry of a list field ("email").
	Item string
	// YearOptional lets a date be month and day only (a birthday).
	YearOptional bool
	// Min is the lowest number a number field takes. Nil means no limit.
	Min *float64
	// Format is the shape a text or list entry must have: FormatEmail, FormatURL, FormatPhone or
	// FormatDomain. Empty means free text.
	Format string
	// Max is the longest value in characters. Zero means the default for the field type.
	Max int
	// Hint is the grayscale text shown after the cursor while the field is edited.
	Hint string
}

// DateParser turns a natural date phrase into the stored form. ok is false when it is not a date. The
// model cannot import the parser, so the caller passes it (shortsyntax in the TUI). The checks
// themselves live in internal/validate (patterns/one-validation-framework).
type DateParser func(raw string) (string, bool)

// ChoiceValues lists the stored values of a choice field, comma separated.
func (f FieldSpec) ChoiceValues() string {
	var out []string
	for _, c := range f.Choices {
		out = append(out, c.Value)
	}
	return strings.Join(out, ", ")
}

// ChoiceLabel is the label of a stored choice value, or the value itself.
func (f FieldSpec) ChoiceLabel(value string) string {
	for _, c := range f.Choices {
		if c.Value == value {
			return c.Label
		}
	}
	return value
}

// DefaultHint is the grayscale hint of a field: its own Hint, else one by type.
func (f FieldSpec) DefaultHint() string {
	if f.Hint != "" {
		return f.Hint
	}
	switch f.Type {
	case FieldDate:
		return "date: 2026-12-01 or next friday"
	case FieldNumber:
		return "number"
	case FieldChoice:
		return "up and down pick, enter or tab saves"
	case FieldLongText:
		return "markdown"
	case FieldRelation:
		return "type to search, enter picks, tab completes"
	}
	return ""
}

func tierChoices() []FieldChoiceOption {
	return []FieldChoiceOption{{"0", "clear"}, {"1", "Top"}, {"2", "High"}, {"3", "Medium"}, {"4", "Low"}, {"5", "None"}}
}

func stageChoices() []FieldChoiceOption {
	out := make([]FieldChoiceOption, len(DealStages))
	for i, s := range DealStages {
		out[i] = FieldChoiceOption{s, s}
	}
	return out
}

func priorityChoices() []FieldChoiceOption {
	out := make([]FieldChoiceOption, len(Priorities))
	for i, s := range Priorities {
		out[i] = FieldChoiceOption{s, s}
	}
	return out
}

var zero = 0.0

// Fields is the one list of fields per object kind. The TUI editors, the CLI and MCP checks and
// the docs read it. A kind that is not here has no declared fields.
var Fields = map[string][]FieldSpec{
	"person": {
		{Key: "first", Label: "first name", Type: FieldText, Required: true},
		{Key: "last", Label: "last name", Type: FieldText},
		{Key: "title", Label: "title", Type: FieldText},
		{Key: "company", Label: "company", Type: FieldText},
		{Key: "organization", Label: "organizations", Type: FieldRelation, Kind: "organization"},
		{Key: "location", Label: "location", Type: FieldText},
		{Key: "birthday", Label: "birthday", Type: FieldDate, YearOptional: true},
		{Key: "email", Label: "email", Type: FieldList, Item: "email", Format: FormatEmail},
		{Key: "phone", Label: "phone", Type: FieldList, Item: "phone", Format: FormatPhone},
		{Key: "tier", Label: "priority", Type: FieldChoice, Choices: tierChoices(), Clear: "0"},
		{Key: "notes", Label: "notes", Type: FieldLongText},
		{Key: "tag", Label: "tags", Type: FieldRelation, Kind: "tag"},
	},
	"organization": {
		{Key: "name", Label: "name", Type: FieldText, Required: true},
		{Key: "domain", Label: "domain", Type: FieldText, Format: FormatDomain},
		{Key: "website", Label: "website", Type: FieldText, Format: FormatURL},
		{Key: "industry", Label: "industry", Type: FieldText},
		{Key: "size", Label: "size", Type: FieldText},
		{Key: "location", Label: "location", Type: FieldText, Hint: "city, region, country"},
		{Key: "linkedin", Label: "linkedin", Type: FieldText, Format: FormatURL},
		{Key: "description", Label: "about", Type: FieldLongText},
		{Key: "notes", Label: "notes", Type: FieldLongText},
		{Key: "member", Label: "members", Type: FieldRelation, Kind: "person"},
	},
	"deal": {
		{Key: "title", Label: "title", Type: FieldText, Required: true},
		{Key: "stage", Label: "stage", Type: FieldChoice, Choices: stageChoices()},
		{Key: "value", Label: "value", Type: FieldNumber, Min: &zero},
		{Key: "notes", Label: "notes", Type: FieldLongText},
		{Key: "organization", Label: "organizations", Type: FieldRelation, Kind: "organization"},
		{Key: "person", Label: "people", Type: FieldRelation, Kind: "person"},
	},
	"task": {
		{Key: "title", Label: "title", Type: FieldText, Required: true},
		{Key: "due", Label: "due", Type: FieldDate},
		{Key: "priority", Label: "priority", Type: FieldChoice, Choices: priorityChoices()},
		{Key: "description", Label: "description", Type: FieldLongText},
	},
	NoteKind: {
		{Key: "text", Label: "text", Type: FieldLongText, Required: true},
	},
}

// FieldsOf lists the declared fields of an object kind.
func FieldsOf(kind string) []FieldSpec { return Fields[kind] }

// FieldNamed finds one declared field of a kind.
func FieldNamed(kind, key string) (FieldSpec, bool) {
	for _, f := range Fields[kind] {
		if f.Key == key {
			return f, true
		}
	}
	return FieldSpec{}, false
}
