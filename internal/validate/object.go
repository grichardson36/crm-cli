package validate

import (
	"sort"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// Object checks several fields of one object kind. vals maps a field key from model.Fields to the
// typed value. With partial set (an edit) only the given keys are checked, and a given key that is
// empty fails when the field is required. Without it (a create) every required field must be given
// and non-empty. Result.Values holds the cleaned form of every given key.
func Object(kind string, vals map[string]string, partial bool, opt Options) Result {
	r := Result{Values: map[string]string{}}
	opt.Kind = kind
	specs := model.Fields[kind]
	known := map[string]bool{}
	for _, s := range specs {
		known[s.Key] = true
	}
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !known[k] && len(specs) > 0 {
			r.add(Issue{Code: CodeFieldUnknown, Field: k, Expected: "one of the fields of " + kind, Received: k, Fix: "check the field name"})
		}
	}
	for _, s := range specs {
		v, given := vals[s.Key]
		if !given {
			if !partial && s.Required {
				r.add(Issue{Code: CodeFieldRequired, Field: s.Label, Expected: "a value is required", Fix: "give " + s.Label})
			}
			continue
		}
		fr := Field(s, v, opt)
		r.merge(fr)
		r.Values[s.Key] = fr.Value
	}
	return r
}
