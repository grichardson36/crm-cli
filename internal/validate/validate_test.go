package validate_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/help"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/validate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fakeDates(raw string) (string, bool) {
	if raw == "next friday" {
		return "2026-11-06", true
	}
	return "", false
}

func spec(t *testing.T, kind, key string) model.FieldSpec {
	t.Helper()
	s, ok := model.FieldNamed(kind, key)
	require.True(t, ok, kind+"."+key)
	return s
}

func TestFieldTable(t *testing.T) {
	long := strings.Repeat("a", validate.DefaultTextMax+1)
	tests := []struct {
		name       string
		kind, key  string
		in, want   string
		code       string
		wantNoCode bool
	}{
		// text and longtext
		{"text trims and collapses", "person", "last", "  Van   Der  Berg ", "Van Der Berg", "", true},
		{"text strips control characters", "person", "last", "Sm\x00it\x07h", "Smith", "", true},
		{"optional text clears", "person", "last", "   ", "", "", true},
		{"required text refuses empty", "person", "first", "", "", validate.CodeFieldRequired, false},
		{"text too long", "person", "last", long, "", validate.CodeFieldTooLong, false},
		{"longtext keeps lines", "person", "notes", "# hi\n\n\n\n- a  b", "# hi\n\n- a b", "", true},
		// formats
		{"email ok", "person", "email", " a@b.com ", "a@b.com", "", true},
		{"email without at", "person", "email", "a.b.com", "", validate.CodeEmailInvalid, false},
		{"email without domain dot", "person", "email", "a@b", "", validate.CodeEmailInvalid, false},
		{"phone ok", "person", "phone", "+1 (555) 010-0100", "+1 (555) 010-0100", "", true},
		{"phone with extension", "person", "phone", "555 0100 x12", "555 0100 x12", "", true},
		{"phone with words", "person", "phone", "call me", "", validate.CodePhoneInvalid, false},
		{"phone too few digits", "person", "phone", "12", "", validate.CodePhoneInvalid, false},
		{"url with scheme", "organization", "website", "https://acme.com/a?b=1", "https://acme.com/a?b=1", "", true},
		{"url bare host", "organization", "website", "acme.com", "acme.com", "", true},
		{"url with space", "organization", "website", "not a url", "", validate.CodeURLInvalid, false},
		{"domain ok", "organization", "domain", "acme.co.uk", "acme.co.uk", "", true},
		{"domain with space", "organization", "domain", "acme dot com", "", validate.CodeDomainInvalid, false},
		// number
		{"number plain", "deal", "value", "12500", "12500", "", true},
		{"number with money marks", "deal", "value", "$12,500.50", "12500.5", "", true},
		{"number rejects words", "deal", "value", "lots", "", validate.CodeNumberInvalid, false},
		{"number below min", "deal", "value", "-5", "", validate.CodeNumberTooLow, false},
		// choice
		{"choice by value", "deal", "stage", "won", "won", "", true},
		{"choice by label any case", "deal", "stage", "NEGOTIATION", "negotiation", "", true},
		{"choice by position", "deal", "stage", "2", "prospect", "", true},
		{"choice rejects other", "deal", "stage", "maybe", "", validate.CodeChoiceInvalid, false},
		{"choice with clear value takes labels", "person", "tier", "high", "2", "", true},
		{"choice with clear value takes no positions", "person", "tier", "7", "", validate.CodeChoiceInvalid, false},
		// date
		{"date iso", "task", "due", "2026-12-01", "2026-12-01", "", true},
		{"date with time", "task", "due", "2026-12-01 09:30", "2026-12-01 09:30", "", true},
		{"date with zone", "task", "due", "2026-12-01T09:30:00Z", "2026-12-01T09:30:00Z", "", true},
		{"date natural phrase", "task", "due", "next friday", "2026-11-06", "", true},
		{"date rejects junk", "task", "due", "someday", "", validate.CodeDateInvalid, false},
		{"date rejects impossible day", "task", "due", "2026-02-31", "", validate.CodeDateInvalid, false},
		{"birthday month and day", "person", "birthday", "03-14", "03-14", "", true},
		{"birthday impossible", "person", "birthday", "02-31", "", validate.CodeDateInvalid, false},
		{"month and day is not a due date", "task", "due", "03-14", "", validate.CodeDateInvalid, false},
		// list entry and relation name
		{"list entry checks its format", "person", "email", "nope", "", validate.CodeEmailInvalid, false},
		{"relation takes a name", "person", "organization", " Acme ", "Acme", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := validate.Field(spec(t, tt.kind, tt.key), tt.in, validate.Options{Kind: tt.kind, ParseDate: fakeDates})
			if tt.code != "" {
				require.False(t, res.OK())
				is := res.Errors()[0]
				assert.Equal(t, tt.code, is.Code)
				assert.NotEmpty(t, is.Field)
				assert.NotEmpty(t, is.Expected)
				assert.NotEmpty(t, is.Fix)
				err := res.Err()
				assert.True(t, errors.Is(err, model.ErrValidation), "exit code 2 like every validation error")
				assert.Equal(t, 2, model.ExitCode(err))
				return
			}
			require.True(t, res.OK(), res.Err())
			assert.Equal(t, tt.want, res.Value)
		})
	}
}

func TestReceivedIsReported(t *testing.T) {
	res := validate.Field(spec(t, "person", "email"), "nope", validate.Options{})
	require.False(t, res.OK())
	assert.Equal(t, "nope", res.Errors()[0].Received)
	assert.Contains(t, res.Err().Error(), validate.CodeEmailInvalid)
}

func TestEveryErrorCodeIsReachable(t *testing.T) {
	seen := map[string]bool{}
	note := func(err error) {
		if e, ok := validate.As(err); ok {
			for _, i := range e.Issues {
				seen[i.Code] = true
			}
		}
	}
	for _, c := range []struct {
		kind, key, v string
	}{
		{"person", "first", ""}, {"person", "last", strings.Repeat("x", 600)}, {"person", "email", "x"},
		{"organization", "website", "a b"}, {"person", "phone", "abc"}, {"organization", "domain", "a b"},
		{"task", "due", "zzz"}, {"deal", "stage", "zzz"}, {"deal", "value", "zzz"}, {"deal", "value", "-1"},
	} {
		note(validate.Field(spec(t, c.kind, c.key), c.v, validate.Options{}).Err())
	}
	note(validate.Object("person", map[string]string{"nonsense": "x"}, true, validate.Options{}).Err())
	note(validate.DateOrder("start date", "2026-02-01", "end date", "2026-01-01"))
	note(validate.OrgParentCycle(1, 2, true))
	note(validate.OnePrimaryMembership(7, 2))
	roleSpec := spec(t, "deal_person", "role").WithVocabulary([]string{"champion"})
	note(validate.Field(roleSpec, "zzz", validate.Options{}).Err())
	_, vErr := validate.VocabName("deal_role", "  ")
	note(vErr)
	note(validate.VocabDuplicate("deal_role", "champion"))
	note(validate.VocabInUse("deal_role", "champion", 2))
	for _, c := range validate.Codes {
		if c.Kind == validate.KindError {
			assert.True(t, seen[c.Code], "no test reaches "+c.Code)
		}
	}
}

func TestCodeVocabulary(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range validate.Codes {
		assert.False(t, seen[c.Code], "duplicate "+c.Code)
		seen[c.Code] = true
		assert.NotEmpty(t, c.Meaning)
		prefix := map[string]string{validate.KindError: "E_", validate.KindWarning: "W_", validate.KindSuggestion: "S_"}[c.Kind]
		assert.True(t, strings.HasPrefix(c.Code, prefix), c.Code)
		assert.Equal(t, c.Kind, validate.KindOf(c.Code))
	}
	assert.Len(t, validate.Codes, 23)
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		name, in, want string
		multi          bool
		warn           bool
	}{
		{"trim", "  a  ", "a", false, false},
		{"collapse", "a \t b\n c", "a b c", false, false},
		{"control characters", "a\x00b\x1fc​d", "abc​d", false, false},
		{"byte order mark", string(rune(0xFEFF))+"Acme", "Acme", false, false},
		{"multiline keeps breaks", "a  b\r\n\r\n\r\n\r\nc", "a b\n\nc", true, false},
		{"damaged encoding is kept and flagged", "Acme � Corp", "Acme � Corp", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, issues, sugg := validate.Sanitize("company", tt.in, tt.multi)
			assert.Equal(t, tt.want, got)
			if tt.warn {
				require.Len(t, issues, 1)
				assert.Equal(t, validate.CodeEncodingDamaged, issues[0].Code)
				assert.False(t, issues[0].IsError(), "a warning never fails the write")
				require.Len(t, sugg, 1)
				assert.Equal(t, "Acme Corp", sugg[0].Value)
				assert.Equal(t, "company", sugg[0].Target)
				return
			}
			assert.Empty(t, issues)
			assert.Empty(t, sugg)
		})
	}
}

func TestDamagedEncodingDoesNotFailAndIsNotRewritten(t *testing.T) {
	res := validate.Field(spec(t, "person", "company"), "Caf� Roma", validate.Options{Kind: "person"})
	assert.True(t, res.OK())
	assert.Equal(t, "Caf� Roma", res.Value)
	require.Len(t, res.Warnings(), 1)
	require.NotEmpty(t, res.Suggestions)
	assert.Equal(t, "Caf Roma", res.Suggestions[0].Value)
}

func TestSuggestions(t *testing.T) {
	tests := []struct {
		name, kind, key, in string
		code, target        string
	}{
		{"phone in company", "person", "company", "+1 (555) 010-0100", validate.CodePhoneInTextField, "phone"},
		{"plain digits phone in title", "person", "title", "555 010 0100", validate.CodePhoneInTextField, "phone"},
		{"email in company", "person", "company", "jane@acme.com", validate.CodeEmailInTextField, "email"},
		{"email in last name", "person", "last", "jane@acme.com", validate.CodeEmailInTextField, "email"},
		{"url in company", "person", "company", "https://acme.com", validate.CodeURLInTextField, "website"},
		{"www in org name", "organization", "name", "www.acme.com", validate.CodeURLInTextField, "website"},
		{"email in org industry", "organization", "industry", "a@b.com", "", ""},
		{"nothing to say about a name", "person", "company", "Acme Inc", "", ""},
		{"a year is not a phone", "person", "title", "2026", "", ""},
		{"a phone in a phone field is fine", "person", "phone", "555 010 0100", "", ""},
		{"a url in a url field is fine", "organization", "website", "https://acme.com", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := validate.Field(spec(t, tt.kind, tt.key), tt.in, validate.Options{Kind: tt.kind})
			assert.True(t, res.OK(), "a suggestion never fails the write")
			if tt.code == "" {
				assert.Empty(t, res.Suggestions)
				return
			}
			require.Len(t, res.Suggestions, 1)
			s := res.Suggestions[0]
			assert.Equal(t, tt.code, s.Code)
			assert.Equal(t, tt.target, s.Target)
			assert.Equal(t, tt.key, s.Field)
			assert.Equal(t, tt.in, s.Value)
			assert.Equal(t, tt.in, res.Value, "the value is never moved by the framework")
		})
	}
}

func TestObject(t *testing.T) {
	t.Run("create needs required fields", func(t *testing.T) {
		res := validate.Object("deal", map[string]string{"stage": "won"}, false, validate.Options{})
		require.False(t, res.OK())
		assert.Equal(t, validate.CodeFieldRequired, res.Errors()[0].Code)
	})
	t.Run("partial skips missing fields", func(t *testing.T) {
		res := validate.Object("deal", map[string]string{"stage": "won"}, true, validate.Options{})
		assert.True(t, res.OK())
		assert.Equal(t, "won", res.Values["stage"])
	})
	t.Run("partial refuses emptying a required field", func(t *testing.T) {
		res := validate.Object("deal", map[string]string{"title": " "}, true, validate.Options{})
		assert.False(t, res.OK())
	})
	t.Run("unknown field", func(t *testing.T) {
		res := validate.Object("deal", map[string]string{"colour": "red"}, true, validate.Options{})
		assert.Equal(t, validate.CodeFieldUnknown, res.Errors()[0].Code)
	})
	t.Run("collects every error", func(t *testing.T) {
		res := validate.Object("deal", map[string]string{"title": "", "stage": "zzz"}, false, validate.Options{})
		assert.Len(t, res.Errors(), 2)
	})
}

func TestInputAdapters(t *testing.T) {
	s := func(v string) *string { return &v }
	t.Run("person create cleans and writes back", func(t *testing.T) {
		in := model.CreatePersonInput{FirstName: "  Jane  ", Email: s(" jane@acme.com "), Company: s("+1 555 010 0100")}
		res := validate.PersonCreate(&in)
		require.True(t, res.OK())
		assert.Equal(t, "Jane", in.FirstName)
		assert.Equal(t, "jane@acme.com", *in.Email)
		require.Len(t, res.Suggestions, 1)
		assert.Equal(t, "phone", res.Suggestions[0].Target)
	})
	t.Run("person create rejects bad email", func(t *testing.T) {
		in := model.CreatePersonInput{FirstName: "Jane", Email: s("nope")}
		assert.Equal(t, validate.CodeEmailInvalid, validate.PersonCreate(&in).Errors()[0].Code)
	})
	t.Run("person update only checks given fields", func(t *testing.T) {
		assert.True(t, validate.PersonUpdate(&model.UpdatePersonInput{Title: s("CTO")}).OK())
		assert.False(t, validate.PersonUpdate(&model.UpdatePersonInput{FirstName: s("")}).OK())
	})
	t.Run("org", func(t *testing.T) {
		assert.False(t, validate.OrgCreate(&model.CreateOrgInput{Name: ""}).OK())
		assert.False(t, validate.OrgCreate(&model.CreateOrgInput{Name: "A", Domain: s("a b")}).OK())
		assert.False(t, validate.OrgUpdate(&model.UpdateOrgInput{OrgNativeFields: model.OrgNativeFields{Website: s("a b")}}).OK())
	})
	t.Run("deal", func(t *testing.T) {
		neg := -1.0
		assert.False(t, validate.DealCreate(&model.CreateDealInput{Title: "T", Stage: "prospect", Value: &neg}).OK())
		assert.False(t, validate.DealUpdate(&model.UpdateDealInput{Stage: s("zzz")}).OK())
		assert.True(t, validate.DealCreate(&model.CreateDealInput{Title: "T"}).OK())
	})
	t.Run("task", func(t *testing.T) {
		assert.False(t, validate.TaskCreate(&model.CreateTaskInput{Title: "T", DueAt: s("someday")}).OK())
		assert.False(t, validate.TaskUpdate(&model.UpdateTaskInput{Priority: s("zzz")}).OK())
		assert.True(t, validate.TaskUpdate(&model.UpdateTaskInput{DueAt: s("")}).OK())
	})
	t.Run("interaction", func(t *testing.T) {
		assert.False(t, validate.InteractionCreate(&model.CreateInteractionInput{Direction: s("sideways")}).OK())
		c := "  hello  "
		in := model.CreateInteractionInput{Content: &c}
		require.True(t, validate.InteractionCreate(&in).OK())
		assert.Equal(t, "hello", *in.Content)
		assert.False(t, validate.InteractionUpdate(&model.UpdateInteractionInput{Direction: s("sideways")}).OK())
	})
}

func TestBusinessRules(t *testing.T) {
	assert.NoError(t, validate.OnePrimaryMembership(1, 0))
	assert.NoError(t, validate.OnePrimaryMembership(1, 1))
	assert.Error(t, validate.OnePrimaryMembership(1, 2))
	assert.NoError(t, validate.OrgParentCycle(1, 2, false))
	assert.Error(t, validate.OrgParentCycle(1, 2, true))
	assert.NoError(t, validate.DateOrder("start date", "2026-01-01", "end date", "2026-01-01"))
	assert.NoError(t, validate.DateOrder("start date", "", "end date", "2026-01-01"))
	assert.Error(t, validate.DateOrder("start date", "2026-02-01", "end date", "2026-01-01"))
	v, err := validate.OneOf("direction", "INBOUND", model.InteractionDirections)
	assert.NoError(t, err)
	assert.Equal(t, "inbound", v)
}

func TestHelpTopicListsEveryCode(t *testing.T) {
	topic, ok := help.Get("validation")
	require.True(t, ok)
	for _, c := range validate.Codes {
		assert.Contains(t, topic.Body, c.Code)
	}
}
