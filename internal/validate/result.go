package validate

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// Issue is one finding about a field: a stable code, the field, what was expected, what was received
// and a suggested fix.
type Issue struct {
	Code     string `json:"code"`
	Field    string `json:"field,omitempty"`
	Expected string `json:"expected,omitempty"`
	Received string `json:"received,omitempty"`
	Fix      string `json:"fix,omitempty"`
}

// IsError reports whether the issue fails the write.
func (i Issue) IsError() bool { return strings.HasPrefix(i.Code, "E_") }

// Suggestion is a smart correction. It is never applied by the framework: an interface shows it and
// the user accepts it (patterns/enrichment-is-suggested-then-accepted).
type Suggestion struct {
	Code string `json:"code"`
	// Field is the field the value was found in. Target is the field it probably belongs in.
	Field   string `json:"field"`
	Target  string `json:"target"`
	Value   string `json:"value"`
	Message string `json:"message"`
}

// Result is what a check returns: the cleaned values, every issue and every suggestion.
type Result struct {
	// Value is the cleaned and normalized value of a single-field check.
	Value string
	// Values holds the cleaned values of an object check, by field key.
	Values      map[string]string
	Issues      []Issue
	Suggestions []Suggestion
}

// Errors lists the issues that fail the write.
func (r Result) Errors() []Issue { return r.filter(true) }

// Warnings lists the issues that do not fail the write.
func (r Result) Warnings() []Issue { return r.filter(false) }

func (r Result) filter(errs bool) []Issue {
	var out []Issue
	for _, i := range r.Issues {
		if i.IsError() == errs {
			out = append(out, i)
		}
	}
	return out
}

// OK reports whether nothing failed. Warnings and suggestions do not count.
func (r Result) OK() bool { return len(r.Errors()) == 0 }

// Err is nil when the check passed, else an *Error carrying the errors, warnings and suggestions.
func (r Result) Err() error {
	if r.OK() {
		return nil
	}
	return &Error{Issues: r.Issues, Suggestions: r.Suggestions}
}

func (r *Result) add(i Issue) { r.Issues = append(r.Issues, i) }

func (r *Result) merge(o Result) {
	r.Issues = append(r.Issues, o.Issues...)
	r.Suggestions = append(r.Suggestions, o.Suggestions...)
}

// Error is a failed validation. It is a model.ErrValidation, so every door keeps its exit code (2),
// HTTP status (400) and so on.
type Error struct {
	Issues      []Issue
	Suggestions []Suggestion
}

// First is the first failing issue.
func (e *Error) First() Issue {
	for _, i := range e.Issues {
		if i.IsError() {
			return i
		}
	}
	return Issue{}
}

func (e *Error) Error() string {
	var parts []string
	for _, i := range e.Issues {
		if !i.IsError() {
			continue
		}
		s := i.Code
		if i.Field != "" {
			s += " " + i.Field
		}
		s += ": " + i.Expected
		if i.Received != "" {
			s += fmt.Sprintf(" (got %q)", i.Received)
		}
		if i.Fix != "" {
			s += ", " + i.Fix
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "; ")
}

// Unwrap makes a validation error a model.ErrValidation.
func (e *Error) Unwrap() error { return model.ErrValidation }

// As finds the validation error in err.
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// Fail builds an *Error for one issue, for rules that run outside a field check.
func Fail(i Issue) *Error { return &Error{Issues: []Issue{i}} }

func formatFloat(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
