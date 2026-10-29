package validate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// Default length limits by field type, in characters. A FieldSpec.Max overrides them.
const (
	DefaultTextMax     = 500
	DefaultLongTextMax = 100000
)

var (
	emailRe    = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
	urlRe      = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9+.-]*://)?[^\s/@:]+(\.[^\s/@:]+)+(:\d+)?([/?#]\S*)?$`)
	phoneRe    = regexp.MustCompile(`^\+?[\d\s().\-/]+((x|ext\.?|#)\s*\d+)?$`)
	isoDateRe  = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})([ T]\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:?\d{2})?)?$`)
	monthDayRe = regexp.MustCompile(`^(--)?(\d{1,2})-(\d{1,2})$`)
	moneyTrim  = strings.NewReplacer("$", "", ",", "", "_", "")
	nonDigit   = regexp.MustCompile(`\D`)
)

// Options tune a check.
type Options struct {
	// Kind is the object kind the field belongs to ("person"). Suggestions name a target field of it.
	Kind string
	// ParseDate turns a natural phrase ("next friday") into a stored date. Nil accepts ISO dates only.
	ParseDate model.DateParser
	// Create lets a field backed by a vocabulary take a value that is not on the list yet. The check
	// then passes and returns the S_VOCAB_CREATE suggestion, and the caller adds the value to the list.
	// Without it an unknown value is E_VOCAB_UNKNOWN.
	Create bool
}

// Field checks one value typed for a field. It sanitizes the text, then applies the check of the
// field type (and its Format), and returns the form to store in Result.Value. An empty value is fine
// for an optional field (it clears it) and E_FIELD_REQUIRED for a required one. A list field is
// checked one entry at a time. A relation field takes the name of the linked object.
func Field(spec model.FieldSpec, raw string, opt Options) Result {
	var r Result
	label := spec.Label
	v, issues, sugg := Sanitize(label, raw, spec.Type == model.FieldLongText)
	r.Issues = append(r.Issues, issues...)
	r.Suggestions = append(r.Suggestions, sugg...)
	if v == "" {
		if spec.Required {
			r.add(Issue{Code: CodeFieldRequired, Field: label, Expected: "a value is required", Fix: "type one, esc cancels"})
		}
		return r
	}
	if spec.Hierarchy == model.HierarchyPath {
		clean, i := checkPath(spec, v)
		if i != nil {
			r.add(*i)
			return r
		}
		v = clean
	}
	if max := maxLen(spec); max > 0 && utf8.RuneCountInString(v) > max {
		r.add(Issue{Code: CodeFieldTooLong, Field: label, Expected: fmt.Sprintf("at most %d characters", max),
			Received: fmt.Sprintf("%d characters", utf8.RuneCountInString(v)), Fix: "shorten it"})
		return r
	}
	switch spec.Type {
	case model.FieldText, model.FieldLongText, model.FieldList, model.FieldRelation:
		if i := checkFormat(spec, v); i != nil {
			r.add(*i)
			return r
		}
		r.Value = v
		if spec.Type == model.FieldText || spec.Type == model.FieldLongText {
			r.Suggestions = append(r.Suggestions, Suggest(opt.Kind, spec, v)...)
		}
	case model.FieldNumber:
		n, err := strconv.ParseFloat(moneyTrim.Replace(v), 64)
		if err != nil {
			r.add(Issue{Code: CodeNumberInvalid, Field: label, Expected: "a number", Received: v, Fix: "for example 12500 or $12,500"})
			return r
		}
		if spec.Min != nil && n < *spec.Min {
			r.add(Issue{Code: CodeNumberTooLow, Field: label, Expected: fmt.Sprintf("at least %g", *spec.Min), Received: v, Fix: fmt.Sprintf("use %g or more", *spec.Min)})
			return r
		}
		r.Value = strconv.FormatFloat(n, 'f', -1, 64)
	case model.FieldChoice:
		if spec.Vocabulary != "" {
			vocabChoice(&r, spec, v, opt)
			return r
		}
		for i, c := range spec.Choices {
			if strings.EqualFold(v, c.Value) || strings.EqualFold(v, c.Label) || (v == strconv.Itoa(i+1) && spec.Clear == "") {
				r.Value = c.Value
				return r
			}
		}
		r.add(Issue{Code: CodeChoiceInvalid, Field: label, Expected: "one of " + spec.ChoiceValues(), Received: v, Fix: "use one of the listed values"})
	case model.FieldDate:
		d, ok := parseDate(spec, v, opt.ParseDate)
		if !ok {
			fix := "for example 2026-12-01 or next friday"
			if spec.YearOptional {
				fix = "for example 1985-03-14, 03-14 or next friday"
			}
			r.add(Issue{Code: CodeDateInvalid, Field: label, Expected: "a date", Received: v, Fix: fix})
			return r
		}
		r.Value = d
	}
	return r
}

func maxLen(spec model.FieldSpec) int {
	if spec.Max > 0 {
		return spec.Max
	}
	switch spec.Type {
	case model.FieldLongText:
		return DefaultLongTextMax
	case model.FieldText, model.FieldList, model.FieldRelation:
		return DefaultTextMax
	}
	return 0
}

func checkFormat(spec model.FieldSpec, v string) *Issue {
	switch spec.Format {
	case model.FormatEmail:
		if !IsEmail(v) {
			return &Issue{Code: CodeEmailInvalid, Field: spec.Label, Expected: "an email address like name@example.com", Received: v, Fix: "check for a missing @ or domain"}
		}
	case model.FormatURL:
		if !IsURL(v) {
			return &Issue{Code: CodeURLInvalid, Field: spec.Label, Expected: "a web address like https://example.com", Received: v, Fix: "add the site name, for example example.com"}
		}
	case model.FormatDomain:
		if !IsURL(v) {
			return &Issue{Code: CodeDomainInvalid, Field: spec.Label, Expected: "a domain like example.com", Received: v, Fix: "use a site name such as example.com, no spaces"}
		}
	case model.FormatPhone:
		if !IsPhone(v) {
			return &Issue{Code: CodePhoneInvalid, Field: spec.Label, Expected: "a phone number like +1 555 010 0100", Received: v, Fix: "use digits, spaces and + ( ) - only"}
		}
	}
	return nil
}

// IsEmail reports whether s is shaped like an email address.
func IsEmail(s string) bool { return len(s) <= 254 && emailRe.MatchString(s) }

// IsURL reports whether s is shaped like a web address, with or without a scheme.
func IsURL(s string) bool { return urlRe.MatchString(s) }

// IsPhone reports whether s is shaped like a phone number: allowed characters only and 3 to 20 digits.
// This is the extension point for E.164 normalization, which is a separate roadmap item: add it here
// as a function that returns the stored form, and Field will store that instead of the typed text.
func IsPhone(s string) bool {
	if !phoneRe.MatchString(s) {
		return false
	}
	n := len(nonDigit.ReplaceAllString(s, ""))
	return n >= 3 && n <= 20
}

func parseDate(spec model.FieldSpec, v string, parse model.DateParser) (string, bool) {
	if m := isoDateRe.FindStringSubmatch(v); m != nil {
		if _, err := time.Parse("2006-01-02", m[1]); err == nil {
			return v, true
		}
		return "", false
	}
	if spec.YearOptional {
		if m := monthDayRe.FindStringSubmatch(v); m != nil {
			mo, _ := strconv.Atoi(m[2])
			d, _ := strconv.Atoi(m[3])
			if _, err := time.Parse("2006-01-02", fmt.Sprintf("2000-%02d-%02d", mo, d)); err == nil {
				return v, true
			}
			return "", false
		}
	}
	if parse != nil {
		if d, ok := parse(v); ok {
			return d, true
		}
	}
	return "", false
}

// OneOf checks that value is one of options (case-insensitive) and returns the listed spelling. It is
// the check for a fixed pick list that is not a schema field (an interaction direction, a cadence).
func OneOf(field, value string, options []string) (string, error) {
	for _, o := range options {
		if strings.EqualFold(strings.TrimSpace(value), o) {
			return o, nil
		}
	}
	return "", Fail(Issue{Code: CodeChoiceInvalid, Field: field, Expected: "one of " + strings.Join(options, ", "),
		Received: value, Fix: "use one of the listed values"})
}

// DateOrder checks that an end date is not before its start date. Empty dates pass.
func DateOrder(startField, start, endField, end string) error {
	if start == "" || end == "" || end >= start {
		return nil
	}
	return Fail(Issue{Code: CodeDateOrder, Field: endField, Expected: "a date on or after " + startField + " " + start,
		Received: end, Fix: "move " + endField + " later or " + startField + " earlier"})
}

// OrgParentCycle is the rule that an organization cannot become its own ancestor. The caller reports
// whether the would-be child is already above the would-be parent.
func OrgParentCycle(child, parent int64, childIsAncestor bool) error {
	if !childIsAncestor {
		return nil
	}
	return Fail(Issue{Code: CodeOrgParentCycle, Field: "parent",
		Expected: "a parent that does not make a cycle",
		Received: fmt.Sprintf("organization %d is already above organization %d", child, parent),
		Fix:      "remove the existing parent link first"})
}

// OnePrimaryMembership is the rule that a person has at most one primary membership.
func OnePrimaryMembership(person int64, primaries int) error {
	if primaries <= 1 {
		return nil
	}
	return Fail(Issue{Code: CodePrimaryMultiple, Field: "primary",
		Expected: "one primary organization per person",
		Received: fmt.Sprintf("person %d has %d", person, primaries),
		Fix:      "mark one membership primary and clear the others"})
}
