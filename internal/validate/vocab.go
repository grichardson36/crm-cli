package validate

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// Checks for user-managed vocabularies (model.Vocabularies). A choice field with a Vocabulary takes
// its options from the live list. A value that is not on the list is unknown: Field refuses it with
// E_VOCAB_UNKNOWN, or with Options.Create accepts it and returns S_VOCAB_CREATE so the caller can
// create it. Every door (CLI, MCP, TUI, imports) goes through here.

// vocabChoice is the choice check of a field backed by a vocabulary.
func vocabChoice(r *Result, spec model.FieldSpec, v string, opt Options) {
	for _, c := range spec.Choices {
		if strings.EqualFold(v, c.Value) || strings.EqualFold(v, c.Label) {
			r.Value = c.Value
			return
		}
	}
	noun, cmd := "value", ""
	if vs, ok := model.VocabularyNamed(spec.Vocabulary); ok {
		noun, cmd = vs.Noun, vs.Command
	}
	if opt.Create {
		if i := vocabNameIssue(spec.Vocabulary, spec.Label, v); i != nil {
			r.add(*i)
			return
		}
		r.Value = model.CleanVocabName(v)
		r.Suggestions = append(r.Suggestions, Suggestion{Code: CodeVocabCreate, Field: spec.Label, Target: spec.Key, Value: r.Value,
			Message: fmt.Sprintf("Create %s %s", noun, r.Value)})
		return
	}
	expected := "one of the listed " + noun + "s"
	if len(spec.Choices) > 0 {
		expected = "one of " + spec.ChoiceValues()
	}
	fix := fmt.Sprintf("pick a listed %s or create it by passing the create option", noun)
	if cmd != "" {
		fix = fmt.Sprintf("add it with `%s add %q`, or pass --create (create: true in MCP) to add it while saving", cmd, v)
	}
	r.add(Issue{Code: CodeVocabUnknown, Field: spec.Label, Expected: expected, Received: v, Fix: fix})
}

// VocabName checks a new or renamed value of a vocabulary and returns its cleaned form.
func VocabName(kind, raw string) (string, error) {
	vs, ok := model.VocabularyNamed(kind)
	label := "name"
	if ok {
		label = vs.Noun
	}
	v := model.CleanVocabName(raw)
	if i := vocabNameIssue(kind, label, v); i != nil {
		return "", Fail(*i)
	}
	return v, nil
}

func vocabNameIssue(kind, label, v string) *Issue {
	v = model.CleanVocabName(v)
	max := 80
	if vs, ok := model.VocabularyNamed(kind); ok && vs.MaxLen > 0 {
		max = vs.MaxLen
	}
	switch {
	case v == "":
		return &Issue{Code: CodeVocabInvalid, Field: label, Expected: "a name", Received: "nothing", Fix: "type the name of the " + label}
	case utf8.RuneCountInString(v) > max:
		return &Issue{Code: CodeVocabInvalid, Field: label, Expected: fmt.Sprintf("at most %d characters", max),
			Received: fmt.Sprintf("%d characters", utf8.RuneCountInString(v)), Fix: "shorten it"}
	case strings.ContainsRune(v, '|') || strings.IndexFunc(v, unicode.IsControl) >= 0:
		return &Issue{Code: CodeVocabInvalid, Field: label, Expected: "plain text without | or control characters", Received: v, Fix: "remove the | or the control characters"}
	}
	return nil
}

// VocabDuplicate is the error for a name that is already on the list.
func VocabDuplicate(kind, name string) error {
	noun := "value"
	if vs, ok := model.VocabularyNamed(kind); ok {
		noun = vs.Noun
	}
	return Fail(Issue{Code: CodeVocabDuplicate, Field: noun, Expected: "a name that is not on the list", Received: name, Fix: "use the existing " + noun + " or pick another name"})
}

// VocabInUse is the error for removing a value that rows still use.
func VocabInUse(kind, name string, n int) error {
	vs, _ := model.VocabularyNamed(kind)
	cmd := vs.Command
	return Fail(Issue{Code: CodeVocabInUse, Field: vs.Noun, Expected: "a value no record uses, a replacement or --keep",
		Received: fmt.Sprintf("%q is used by %d record(s)", name, n),
		Fix:      fmt.Sprintf("`%s remove %q --replace-with <other>` moves them, `--keep` leaves them as plain text", cmd, name)})
}
