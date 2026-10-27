package model

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
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
	// Hint is the grayscale text shown after the cursor while the field is edited.
	Hint string
}

// FieldError is a failed field check. Every door gets the same shape: a stable code, the field, what
// was expected, what was received and a fix (patterns/validate-and-correct-at-every-boundary).
type FieldError struct {
	Code     string
	Field    string
	Expected string
	Received string
	Fix      string
}

func (e *FieldError) Error() string {
	s := fmt.Sprintf("%s: %s", e.Field, e.Expected)
	if e.Received != "" {
		s += fmt.Sprintf(" (got %q)", e.Received)
	}
	if e.Fix != "" {
		s += ", " + e.Fix
	}
	return s
}

// Unwrap makes a field error a validation error (exit code 2).
func (e *FieldError) Unwrap() error { return ErrValidation }

// IsFieldError reports whether err is a FieldError and returns it.
func IsFieldError(err error) (*FieldError, bool) {
	var fe *FieldError
	if errors.As(err, &fe) {
		return fe, true
	}
	return nil, false
}

// DateParser turns a natural date phrase into the stored form. ok is false when it is not a date. The
// model cannot import the parser, so the caller passes it (shortsyntax in the TUI).
type DateParser func(raw string) (string, bool)

var (
	isoDate   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}([ T]\d{2}:\d{2}(:\d{2})?)?$`)
	monthDay  = regexp.MustCompile(`^(--)?\d{1,2}-\d{1,2}$`)
	moneyTrim = strings.NewReplacer("$", "", ",", "", "_", "")
)

// Normalize checks a value typed for the field and returns the form to store. An empty value is
// fine for an optional field (it clears it) and an error for a required one. List and relation
// fields are edited as rows and links, not as one value, so they refuse a scalar.
func (f FieldSpec) Normalize(raw string, parseDate DateParser) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		if f.Required {
			return "", &FieldError{Code: "required", Field: f.Label, Expected: "a value is required", Fix: "type one, esc cancels"}
		}
		return "", nil
	}
	switch f.Type {
	case FieldText, FieldLongText:
		return v, nil
	case FieldNumber:
		n, err := strconv.ParseFloat(moneyTrim.Replace(v), 64)
		if err != nil {
			return "", &FieldError{Code: "not_a_number", Field: f.Label, Expected: "a number", Received: v, Fix: "for example 12500 or $12,500"}
		}
		if f.Min != nil && n < *f.Min {
			return "", &FieldError{Code: "below_min", Field: f.Label, Expected: fmt.Sprintf("at least %g", *f.Min), Received: v}
		}
		return strconv.FormatFloat(n, 'f', -1, 64), nil
	case FieldChoice:
		for i, c := range f.Choices {
			if strings.EqualFold(v, c.Value) || strings.EqualFold(v, c.Label) || (v == strconv.Itoa(i+1) && f.Clear == "") {
				return c.Value, nil
			}
		}
		return "", &FieldError{Code: "not_a_choice", Field: f.Label, Expected: "one of " + f.choiceList(), Received: v, Fix: "pick one with up and down"}
	case FieldDate:
		if isoDate.MatchString(v) || (f.YearOptional && monthDay.MatchString(v)) {
			return v, nil
		}
		if parseDate != nil {
			if d, ok := parseDate(v); ok {
				return d, nil
			}
		}
		fix := "for example 2026-12-01 or next friday"
		if f.YearOptional {
			fix = "for example 1985-03-14, 03-14 or next friday"
		}
		return "", &FieldError{Code: "not_a_date", Field: f.Label, Expected: "a date", Received: v, Fix: fix}
	}
	return "", &FieldError{Code: "not_a_value", Field: f.Label, Expected: "to be edited as " + string(f.Type) + " entries", Fix: "use a on a row to add, u to remove"}
}

func (f FieldSpec) choiceList() string {
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
		{Key: "email", Label: "email", Type: FieldList, Item: "email"},
		{Key: "phone", Label: "phone", Type: FieldList, Item: "phone"},
		{Key: "tier", Label: "priority", Type: FieldChoice, Choices: tierChoices(), Clear: "0"},
		{Key: "notes", Label: "notes", Type: FieldLongText},
		{Key: "tag", Label: "tags", Type: FieldRelation, Kind: "tag"},
	},
	"organization": {
		{Key: "name", Label: "name", Type: FieldText, Required: true},
		{Key: "domain", Label: "domain", Type: FieldText},
		{Key: "website", Label: "website", Type: FieldText},
		{Key: "industry", Label: "industry", Type: FieldText},
		{Key: "size", Label: "size", Type: FieldText},
		{Key: "location", Label: "location", Type: FieldText, Hint: "city, region, country"},
		{Key: "linkedin", Label: "linkedin", Type: FieldText},
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
