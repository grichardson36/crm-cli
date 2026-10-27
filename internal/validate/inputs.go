package validate

import (
	"github.com/jdanielnd/crm-cli/internal/model"
)

// The adapters below turn the typed inputs every door builds into an Object check, then write the
// cleaned values back into the input. They are the one place a write path is described, so the
// repositories (and through them the CLI, MCP, importers, DAV and device sync) all apply the same
// rules. Call them on a copy first to read warnings and suggestions, the repository calls them again.

// vals collects given values and writes cleaned ones back.
type vals struct {
	in  map[string]string
	out map[string]*string
	str map[string]*string
}

func newVals() *vals {
	return &vals{in: map[string]string{}, out: map[string]*string{}}
}

// ptr registers an optional field. A nil pointer is not given.
func (v *vals) ptr(key string, p **string) {
	if *p == nil {
		return
	}
	v.in[key] = **p
	v.out[key] = *p
}

// req registers a required string given by value.
func (v *vals) req(key string, p *string) {
	v.in[key] = *p
	v.out[key] = p
}

func (v *vals) run(kind string, partial bool, opt Options) Result {
	r := Object(kind, v.in, partial, opt)
	for k, p := range v.out {
		if cv, ok := r.Values[k]; ok && r.OK() {
			*p = cv
		}
	}
	return r
}

// PersonCreate checks a new person.
func PersonCreate(in *model.CreatePersonInput) Result {
	v := newVals()
	v.req("first", &in.FirstName)
	v.ptr("last", &in.LastName)
	v.ptr("title", &in.Title)
	v.ptr("company", &in.Company)
	v.ptr("location", &in.Location)
	v.ptr("notes", &in.Notes)
	v.ptr("email", &in.Email)
	v.ptr("phone", &in.Phone)
	return v.run("person", false, Options{})
}

// PersonUpdate checks the given fields of a person edit.
func PersonUpdate(in *model.UpdatePersonInput) Result {
	v := newVals()
	v.ptr("first", &in.FirstName)
	v.ptr("last", &in.LastName)
	v.ptr("title", &in.Title)
	v.ptr("company", &in.Company)
	v.ptr("location", &in.Location)
	v.ptr("notes", &in.Notes)
	v.ptr("email", &in.Email)
	v.ptr("phone", &in.Phone)
	return v.run("person", true, Options{})
}

// OrgCreate checks a new organization.
func OrgCreate(in *model.CreateOrgInput) Result {
	v := newVals()
	v.req("name", &in.Name)
	v.ptr("domain", &in.Domain)
	v.ptr("industry", &in.Industry)
	v.ptr("notes", &in.Notes)
	orgNative(v, &in.OrgNativeFields)
	return v.run("organization", false, Options{})
}

// OrgUpdate checks the given fields of an organization edit.
func OrgUpdate(in *model.UpdateOrgInput) Result {
	v := newVals()
	v.ptr("name", &in.Name)
	v.ptr("domain", &in.Domain)
	v.ptr("industry", &in.Industry)
	v.ptr("notes", &in.Notes)
	orgNative(v, &in.OrgNativeFields)
	return v.run("organization", true, Options{})
}

func orgNative(v *vals, f *model.OrgNativeFields) {
	v.ptr("website", &f.Website)
	v.ptr("size", &f.Size)
	v.ptr("linkedin", &f.LinkedinURL)
	v.ptr("description", &f.Description)
}

// DealCreate checks a new deal. An empty stage is left to the repository default.
func DealCreate(in *model.CreateDealInput) Result {
	v := newVals()
	v.req("title", &in.Title)
	if in.Stage != "" {
		v.req("stage", &in.Stage)
	}
	v.ptr("notes", &in.Notes)
	r := v.run("deal", false, Options{})
	checkValue(&r, in.Value)
	return r
}

// DealUpdate checks the given fields of a deal edit.
func DealUpdate(in *model.UpdateDealInput) Result {
	v := newVals()
	v.ptr("title", &in.Title)
	v.ptr("stage", &in.Stage)
	v.ptr("notes", &in.Notes)
	r := v.run("deal", true, Options{})
	checkValue(&r, in.Value)
	return r
}

func checkValue(r *Result, value *float64) {
	if value == nil {
		return
	}
	spec, _ := model.FieldNamed("deal", "value")
	fr := Field(spec, formatFloat(*value), Options{})
	r.Issues = append(r.Issues, fr.Issues...)
}

// TaskCreate checks a new task. An empty priority is left to the repository default.
func TaskCreate(in *model.CreateTaskInput) Result {
	v := newVals()
	v.req("title", &in.Title)
	if in.Priority != "" {
		v.req("priority", &in.Priority)
	}
	v.ptr("due", &in.DueAt)
	v.ptr("description", &in.Description)
	return v.run("task", false, Options{})
}

// TaskUpdate checks the given fields of a task edit. An empty due date clears it.
func TaskUpdate(in *model.UpdateTaskInput) Result {
	v := newVals()
	v.ptr("title", &in.Title)
	v.ptr("priority", &in.Priority)
	v.ptr("due", &in.DueAt)
	v.ptr("description", &in.Description)
	return v.run("task", true, Options{})
}

// InteractionCreate checks a new interaction (a note is one). The subject and content are cleaned
// and length-checked, and the direction must be one of model.InteractionDirections.
func InteractionCreate(in *model.CreateInteractionInput) Result {
	r := Result{Values: map[string]string{}}
	if in.Subject != nil {
		clean(&r, "subject", in.Subject, false)
	}
	if in.Content != nil {
		clean(&r, "text", in.Content, true)
	}
	if in.Direction != nil && *in.Direction != "" {
		if d, err := OneOf("direction", *in.Direction, model.InteractionDirections); err != nil {
			r.Issues = append(r.Issues, err.(*Error).Issues...)
		} else {
			*in.Direction = d
		}
	}
	return r
}

// InteractionUpdate checks the given fields of an interaction edit.
func InteractionUpdate(in *model.UpdateInteractionInput) Result {
	r := Result{Values: map[string]string{}}
	if in.Subject != nil {
		clean(&r, "subject", in.Subject, false)
	}
	if in.Content != nil {
		clean(&r, "text", in.Content, true)
	}
	if in.Direction != nil && *in.Direction != "" {
		if d, err := OneOf("direction", *in.Direction, model.InteractionDirections); err != nil {
			r.Issues = append(r.Issues, err.(*Error).Issues...)
		} else {
			*in.Direction = d
		}
	}
	return r
}

func clean(r *Result, field string, p *string, multiline bool) {
	max := DefaultTextMax
	if multiline {
		max = DefaultLongTextMax
	}
	spec := model.FieldSpec{Key: field, Label: field, Type: model.FieldText, Max: max}
	if multiline {
		spec.Type = model.FieldLongText
	}
	fr := Field(spec, *p, Options{})
	r.merge(fr)
	if fr.OK() {
		*p = fr.Value
	}
}
