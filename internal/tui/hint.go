package tui

import (
	"strings"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/quick"
	"github.com/mattn/go-runewidth"
)

// The inline syntax hint (patterns/show-syntax-inline-as-you-type). Any single-line input that
// takes a special syntax shows the shape it expects in dim grey after the cursor while the user
// types: the pipe line "title | due" and the command bar verbs. The text is drawn after the input,
// never put into it, so it is never submitted. A field drops out of the hint as soon as it has text.
//
// One component serves every input. The field lists come from the object registry (objDef.fields
// for pipe lines, objDef.barFields for the bar), so a new object type that registers fields and
// prompt kinds gets the hint without any drawing code of its own.

// hintField is one field of a bar line. A pipe line field is only a label.
type hintField struct {
	Label string
	// Marker is the token that fills the field in a bar line: "@" for @Name, "by " for a deadline.
	// An empty marker is the free text that follows the verb.
	Marker string
}

// pipeHint returns the dim text that continues a pipe line. typed is the line up to the cursor.
// The current field shows by name until it has text, then the hint continues with the fields after
// it. It is empty when every field is filled or the line has more pipes than fields.
func pipeHint(fields []string, typed string) string {
	if len(fields) == 0 {
		return ""
	}
	idx := strings.Count(typed, "|")
	if idx >= len(fields) {
		return ""
	}
	cur := typed[strings.LastIndex(typed, "|")+1:]
	filled := strings.TrimSpace(cur) != ""
	rest := fields[idx:]
	if filled {
		rest = fields[idx+1:]
	}
	if len(rest) == 0 {
		return ""
	}
	text := strings.Join(rest, " | ")
	if filled {
		text = "| " + text
	}
	return hintSpacing(typed) + text
}

// barHint returns the dim text that continues a bar line that starts with a verb. rest is the text
// after the verb word. Fields that already have a token or text drop out.
func barHint(fields []hintField, line, rest string) string {
	words := strings.Fields(rest)
	var left []string
	for _, f := range fields {
		if !hintFilled(f, words) {
			left = append(left, f.Label)
		}
	}
	if len(left) == 0 {
		return ""
	}
	return hintSpacing(line) + strings.Join(left, " ")
}

// hintFilled says whether the words typed after the verb already fill the field.
func hintFilled(f hintField, words []string) bool {
	if f.Marker == "" {
		for _, w := range words {
			if !strings.ContainsAny(w[:1], "#@!~") {
				return true
			}
		}
		return false
	}
	marker := strings.ToLower(strings.TrimSpace(f.Marker))
	for _, w := range words {
		if strings.HasPrefix(strings.ToLower(w), marker) {
			return true
		}
	}
	return false
}

// hintSpacing is the space between what is typed and the hint.
func hintSpacing(typed string) string {
	if typed == "" || strings.HasSuffix(typed, " ") {
		return ""
	}
	return " "
}

// barExtraShapes are the bar verbs that are not objects. Task and note come from the registry.
var barExtraShapes = map[string][]hintField{
	"set":    {{Label: "[name:]"}, {Label: "key=value", Marker: "="}, {Label: "every 3m", Marker: "every "}, {Label: "!priority", Marker: "!"}, {Label: "#tag", Marker: "#"}},
	"tag":    {{Label: "[name]"}, {Label: "tag ... -old"}},
	"snooze": {{Label: "[name]"}, {Label: "days"}},
}

// extraPromptFields are pipe prompts that belong to no object.
var extraPromptFields = map[string][]string{"notetype-add": {"name", "contact"}}

// promptFieldsFor lists the fields of a pipe prompt: the fields of the object that registered the
// prompt kind, the attributes of the link being made, or an extra prompt.
func (m *Model) promptFieldsFor(kind string) []string {
	for _, o := range objectDefs() {
		for _, k := range o.prompts {
			if k == kind {
				return o.fields
			}
		}
	}
	if kind == "link-attrs" && m.draft != nil {
		var labels []string
		for _, k := range m.draft.rel.AskAttrs() {
			labels = append(labels, strings.ToLower(model.AttrSpecs[k].Label))
		}
		return labels
	}
	return extraPromptFields[kind]
}

// promptHint is the dim text after the footer prompt, cut to the room left on the line. It shows
// only while the cursor is at the end of the line.
func (m *Model) promptHint(used int) string {
	val := m.input.Value()
	if m.input.Position() < len([]rune(val)) {
		return ""
	}
	return cutHint(pipeHint(m.promptFieldsFor(m.promptKind), val), m.width-used)
}

// barSyntaxHint is the dim text after the command bar line: the shape of the verb typed first.
func (m *Model) barSyntaxHint(line string, room int) string {
	trimmed := strings.TrimLeft(line, " \t")
	word, end := trimmed, len(trimmed)
	if i := strings.IndexAny(trimmed, " \t"); i >= 0 {
		word, end = trimmed[:i], i
	}
	verb, ok := quick.VerbOf(word)
	if !ok {
		return ""
	}
	fields := barExtraShapes[verb]
	for _, o := range objectDefs() {
		if o.barVerb == verb {
			fields = o.barFields
		}
	}
	return cutHint(barHint(fields, line, trimmed[end:]), room)
}

func cutHint(h string, room int) string {
	if h == "" || room <= 0 {
		return ""
	}
	return runewidth.Truncate(h, room, "…")
}
