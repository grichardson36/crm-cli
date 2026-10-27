package validate

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/model"
)

var (
	phoneLikeRe = regexp.MustCompile(`^\+?\(?\d[\d\s().\-/]{5,}$`)
	urlLikeRe   = regexp.MustCompile(`(?i)^(https?://|www\.)\S+$`)
)

// Suggest looks at text typed into a field and says where it probably belongs. It never changes
// anything: the interface shows the suggestion and the user accepts it. The first three corrections
// are a phone number, an email address and a web address in a field of another kind.
func Suggest(kind string, spec model.FieldSpec, v string) []Suggestion {
	v = strings.TrimSpace(v)
	var out []Suggestion
	add := func(code, format, what string) {
		if spec.Format == format {
			return
		}
		target := targetField(kind, format)
		if target == "" {
			return
		}
		out = append(out, Suggestion{Code: code, Field: spec.Key, Target: target, Value: v,
			Message: fmt.Sprintf("%q looks like %s, move it to %s?", v, what, target)})
	}
	switch {
	case IsEmail(v):
		add(CodeEmailInTextField, model.FormatEmail, "an email address")
	case urlLikeRe.MatchString(v):
		add(CodeURLInTextField, model.FormatURL, "a web address")
	case phoneLikeRe.MatchString(v) && IsPhone(v) && digitCount(v) >= 7:
		add(CodePhoneInTextField, model.FormatPhone, "a phone number")
	}
	return out
}

func digitCount(s string) int { return len(nonDigit.ReplaceAllString(s, "")) }

// targetField names the field of the object kind that takes the format. A person has no web address
// field, so a web address there points at the website handle.
func targetField(kind, format string) string {
	for _, f := range model.Fields[kind] {
		if f.Format == format {
			return f.Key
		}
	}
	if kind == "person" && format == model.FormatURL {
		return "website"
	}
	return ""
}
