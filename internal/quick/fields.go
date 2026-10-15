package quick

import (
	"fmt"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/shortsyntax"
)

// fieldDef describes one person field that "set" can change. Values are typed strings: the same
// text is used to show old and new values and to restore an old value on undo.
type fieldDef struct {
	name    string
	aliases []string
	// get reads the current value as a typed string ("" when empty).
	get func(p *model.Person) string
	// norm validates and normalizes a typed value. "" always means clear.
	norm func(v string) (string, error)
	// set puts a normalized value into the update input.
	set func(in *model.UpdatePersonInput, v string)
}

func textField(name string, aliases []string, get func(p *model.Person) *string, set func(in *model.UpdatePersonInput, v *string)) fieldDef {
	return fieldDef{
		name: name, aliases: aliases,
		get: func(p *model.Person) string {
			if v := get(p); v != nil {
				return *v
			}
			return ""
		},
		norm: func(v string) (string, error) { return strings.TrimSpace(v), nil },
		set:  func(in *model.UpdatePersonInput, v string) { set(in, &v) },
	}
}

// tierNames are the priority tier names by tier number (1 to 5).
var tierNames = []string{"", "top", "high", "medium", "low", "none"}

var personFields = []fieldDef{
	{
		name: "name", aliases: []string{"fullname"},
		get: func(p *model.Person) string { return strings.TrimSpace(p.FirstName + " " + strOf(p.LastName)) },
		norm: func(v string) (string, error) {
			v = strings.Join(strings.Fields(v), " ")
			if v == "" {
				return "", fmt.Errorf("a name cannot be empty: %w", model.ErrValidation)
			}
			return v, nil
		},
		set: func(in *model.UpdatePersonInput, v string) {
			first, last, _ := strings.Cut(v, " ")
			in.FirstName, in.LastName = &first, &last
		},
	},
	{
		name: "first", aliases: []string{"firstname", "first_name"},
		get: func(p *model.Person) string { return p.FirstName },
		norm: func(v string) (string, error) {
			v = strings.TrimSpace(v)
			if v == "" {
				return "", fmt.Errorf("a first name cannot be empty: %w", model.ErrValidation)
			}
			return v, nil
		},
		set: func(in *model.UpdatePersonInput, v string) { in.FirstName = &v },
	},
	textField("last", []string{"lastname", "last_name", "surname"},
		func(p *model.Person) *string { return p.LastName }, func(in *model.UpdatePersonInput, v *string) { in.LastName = v }),
	textField("company", []string{"org", "employer"},
		func(p *model.Person) *string { return p.Company }, func(in *model.UpdatePersonInput, v *string) { in.Company = v }),
	textField("title", []string{"role", "job"},
		func(p *model.Person) *string { return p.Title }, func(in *model.UpdatePersonInput, v *string) { in.Title = v }),
	textField("email", []string{"mail"},
		func(p *model.Person) *string { return p.Email }, func(in *model.UpdatePersonInput, v *string) { in.Email = v }),
	textField("phone", []string{"tel", "mobile"},
		func(p *model.Person) *string { return p.Phone }, func(in *model.UpdatePersonInput, v *string) { in.Phone = v }),
	textField("location", []string{"city", "place"},
		func(p *model.Person) *string { return p.Location }, func(in *model.UpdatePersonInput, v *string) { in.Location = v }),
	textField("notes", []string{"note"},
		func(p *model.Person) *string { return p.Notes }, func(in *model.UpdatePersonInput, v *string) { in.Notes = v }),
	textField("summary", nil,
		func(p *model.Person) *string { return p.Summary }, func(in *model.UpdatePersonInput, v *string) { in.Summary = v }),
	{
		name: "birthday", aliases: []string{"bday", "birth"},
		get: func(p *model.Person) string {
			if p.BirthdayMonth == nil || p.BirthdayDay == nil {
				return ""
			}
			if p.BirthdayYear != nil {
				return fmt.Sprintf("%04d-%02d-%02d", *p.BirthdayYear, *p.BirthdayMonth, *p.BirthdayDay)
			}
			return fmt.Sprintf("--%02d-%02d", *p.BirthdayMonth, *p.BirthdayDay)
		},
		norm: func(v string) (string, error) {
			v = strings.TrimSpace(v)
			if v == "" {
				return "", nil
			}
			out, ok := shortsyntax.ParseBirthday(v)
			if !ok {
				return "", fmt.Errorf("birthday %q not understood (try Mar 4, 1985-03-04 or --03-04): %w", v, model.ErrValidation)
			}
			return out, nil
		},
		set: func(in *model.UpdatePersonInput, v string) { in.Birthday = &v },
	},
	{
		name: "cadence", aliases: []string{"kit", "keep-in-touch", "keepintouch", "keep_in_touch", "freq", "frequency"},
		get: func(p *model.Person) string { return strOf(p.KeepInTouch) },
		norm: func(v string) (string, error) {
			v = strings.TrimSpace(v)
			if v == "" || strings.EqualFold(v, "unset") || strings.EqualFold(v, "clear") {
				return "", nil
			}
			c, ok := shortsyntax.ParseCadence(v)
			if !ok {
				return "", fmt.Errorf("cadence %q not understood (use one of: %s)", v, strings.Join(model.KeepInTouchCadences, ", "))
			}
			return c, nil
		},
		set: func(in *model.UpdatePersonInput, v string) { in.KeepInTouch = &v },
	},
	{
		name: "tier", aliases: []string{"priority", "prio"},
		get: func(p *model.Person) string {
			if p.PriorityTier == nil || *p.PriorityTier < 1 || *p.PriorityTier >= len(tierNames) {
				return ""
			}
			return tierNames[*p.PriorityTier]
		},
		norm: func(v string) (string, error) {
			v = strings.TrimSpace(v)
			if v == "" || strings.EqualFold(v, "clear") {
				return "", nil
			}
			pr, ok := shortsyntax.ParsePriority(v)
			if !ok {
				return "", fmt.Errorf("priority %q not understood (use top, high, medium, low or none)", v)
			}
			return pr, nil
		},
		set: func(in *model.UpdatePersonInput, v string) {
			n := 0
			for i, name := range tierNames {
				if name == v && i > 0 {
					n = i
				}
			}
			in.PriorityTier = &n
		},
	},
	{
		name: "favorite", aliases: []string{"fav", "star", "starred"},
		get: func(p *model.Person) string {
			if p.IsFavorite {
				return "yes"
			}
			return "no"
		},
		norm: func(v string) (string, error) {
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "yes", "y", "true", "1", "on":
				return "yes", nil
			case "no", "n", "false", "0", "off", "":
				return "no", nil
			}
			return "", fmt.Errorf("favorite %q not understood (use yes or no): %w", v, model.ErrValidation)
		},
		set: func(in *model.UpdatePersonInput, v string) { b := v == "yes"; in.IsFavorite = &b },
	},
	{
		name: "snooze", aliases: []string{"snoozed"},
		get:  func(p *model.Person) string { return strOf(p.KeepInTouchSnoozedUntil) },
		norm: func(v string) (string, error) { return strings.TrimSpace(v), nil },
		set:  func(in *model.UpdatePersonInput, v string) { in.SnoozedUntil = &v },
	},
}

func strOf(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// lookupField finds a field by name or alias (ignoring case).
func lookupField(key string) (*fieldDef, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	for i := range personFields {
		f := &personFields[i]
		if f.name == key {
			return f, true
		}
		for _, a := range f.aliases {
			if a == key {
				return f, true
			}
		}
	}
	return nil, false
}

// handleKey reports whether key is a social handle kind (linkedin, website, ...).
func handleKey(key string) (string, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "web" || key == "site" || key == "url" {
		key = "website"
	}
	if model.ValidHandleKind(key) {
		return key, true
	}
	return "", false
}

// handleValue is the current value of a handle kind on a loaded person.
func handleValue(p *model.Person, kind string) string {
	for _, h := range p.Handles {
		if h.Kind == kind {
			return h.Value
		}
	}
	return ""
}

// fieldKeys lists every key "set" understands, longest first, for splitting "key=value" runs.
func fieldKeys() []string {
	var keys []string
	for _, f := range personFields {
		keys = append(keys, f.name)
		keys = append(keys, f.aliases...)
	}
	keys = append(keys, model.HandleKinds...)
	keys = append(keys, "web", "site", "url")
	return keys
}

// tierText is the display form of a tier number, used by the TUI too.
func tierText(n int) string {
	if n < 1 || n >= len(tierNames) {
		return ""
	}
	return tierNames[n]
}

