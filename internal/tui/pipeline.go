package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/validate"
)

// Pipe lines (owner decisions 2026-10-25 and 2026-10-29). Small objects add and edit on one line, fields
// split by a pipe. This file describes a line as segments typed by the schema, one segment per field,
// so that everything that depends on a field type comes from one place:
//
//   - the live check of a closed segment through internal/validate (below),
//   - the soft indicator of the segment being typed (below),
//   - the autocomplete rows of the segment being typed (pipecomplete.go),
//   - the links, which are segments too (pipelink.go).
//
// A segment is a model.FieldSpec. The fields of an object come from model.Fields, the link segments
// from the relation registry (model.Relations), so a new object that registers fields and relations
// gets live validation, a popup and links without code of its own.

// pipeSeg is one segment of a pipe line.
type pipeSeg struct {
	Key   string
	Label string // the word the grayscale hint shows
	Spec  model.FieldSpec
	// Link segments hold object names separated by commas. On enter every name is linked to the new
	// or edited object. Kinds lists the object kinds a name may belong to (tagKind for a tag).
	Link  bool
	Kinds []string
	// Related marks the one related segment of an add line (related.go): its suggestions come from the
	// context and its kinds are grouped in the popup and the chips.
	Related bool
}

// pipeLine is the segments of one prompt kind.
type pipeLine struct {
	kind string // prompt kind: org-add, deal-edit, task-new ...
	obj  string // object kind the line makes or edits
	segs []pipeSeg
	// core is the number of segments the object's own save code reads. The segments after it are link
	// segments added to an add prompt.
	core int
	add  bool
}

// labels are the grayscale hint words, one per segment.
func (pl *pipeLine) labels() []string {
	out := make([]string, len(pl.segs))
	for i, s := range pl.segs {
		out[i] = s.Label
	}
	return out
}

// pipeAddKinds are the prompt kinds that make a new object. Their lines carry the link segments.
var pipeAddKinds = map[string]bool{
	"org-add": true, "deal-add": true, "task-new": true, "task": true, "task-page": true,
	"note": true, "note-object": true, "note-free": true,
}

// pipeFor describes the pipe line of a prompt kind, or nil when the prompt is not an object line.
func (m *Model) pipeFor(kind string) *pipeLine {
	switch kind {
	case "tag-new":
		return tagNewLine()
	case personAddKind:
		return personAddLine()
	}
	for _, o := range objectDefs() {
		for _, k := range o.prompts {
			if k != kind || len(o.fields) == 0 {
				continue
			}
			pl := &pipeLine{kind: kind, obj: o.kind, add: pipeAddKinds[kind]}
			for _, key := range o.fields {
				seg := coreSeg(o.kind, key)
				if seg.Spec.Vocabulary != "" {
					seg.Spec, _ = m.fieldSpec(o.kind, key)
				}
				pl.segs = append(pl.segs, seg)
			}
			pl.core = len(pl.segs)
			if pl.add {
				if r, ok := relatedSeg(o.kind); ok {
					pl.segs = append(pl.segs, r)
				}
			}
			return pl
		}
	}
	return nil
}

// coreSeg is the segment of a declared field. A field the schema does not declare is plain text.
func coreSeg(objKind, key string) pipeSeg {
	spec, ok := model.FieldNamed(objKind, key)
	if !ok {
		spec = model.FieldSpec{Key: key, Label: key, Type: model.FieldText}
	}
	s := pipeSeg{Key: key, Label: key, Spec: spec}
	if spec.Type == model.FieldRelation {
		s.Link, s.Kinds = true, []string{spec.Kind}
	}
	return s
}

// linkWords are what the picker titles call the objects of a kind.
var linkWords = map[string]string{"person": "people", "organization": "organizations", "deal": "deals", "task": "tasks", model.NoteKind: "notes", tagKind: "tags"}

func linkWord(kind string) string {
	if w := linkWords[kind]; w != "" {
		return w
	}
	return kind + "s"
}

// tagNewLine is the add-tag prompt: the tag's name, then the objects to put it on.
func tagNewLine() *pipeLine {
	on := pipeSeg{Key: "related", Label: "related", Link: true, Related: true, Spec: model.FieldSpec{Key: "related", Label: "related", Type: model.FieldRelation}}
	for _, k := range model.ObjectKinds {
		if model.ValidTaggable(k) {
			on.Kinds = append(on.Kinds, k)
		}
	}
	name := pipeSeg{Key: "name", Label: "name", Spec: model.FieldSpec{Key: "name", Label: "name", Type: model.FieldRelation, Kind: tagKind, Required: true}}
	return &pipeLine{kind: "tag-new", obj: tagKind, segs: []pipeSeg{name, on}, core: 1, add: true}
}

// personAddKind is the pseudo prompt kind of the name row of a new person.
const personAddKind = "person-add"

// personAddLine is the name row of a new person: the name, then the links.
func personAddLine() *pipeLine {
	name := pipeSeg{Key: "name", Label: "name", Spec: model.FieldSpec{Key: "name", Label: "name", Type: model.FieldText, Required: true}}
	pl := &pipeLine{kind: personAddKind, obj: "person", segs: []pipeSeg{name}, core: 1, add: true}
	if r, ok := relatedSeg("person"); ok {
		pl.segs = append(pl.segs, r)
	}
	return pl
}

// --- text geometry (rune positions) ---

// segBounds finds the segment the cursor is in: its index and rune range [start, end).
func segBounds(rs []rune, cursor int) (idx, start, end int) {
	cursor = clampInt(cursor, 0, len(rs))
	for i := 0; i < cursor; i++ {
		if rs[i] == '|' {
			idx, start = idx+1, i+1
		}
	}
	end = len(rs)
	for i := cursor; i < len(rs); i++ {
		if rs[i] == '|' {
			end = i
			break
		}
	}
	return
}

// tokenBounds finds the comma separated name the cursor is in, inside a segment. Leading spaces are skipped.
func tokenBounds(rs []rune, segStart, segEnd, cursor int) (start, end int) {
	cursor = clampInt(cursor, segStart, segEnd)
	start = segStart
	for i := segStart; i < cursor; i++ {
		if rs[i] == ',' {
			start = i + 1
		}
	}
	end = segEnd
	for i := cursor; i < segEnd; i++ {
		if rs[i] == ',' {
			end = i
			break
		}
	}
	for start < cursor && rs[start] == ' ' {
		start++
	}
	return
}

// splitCommas splits a link segment into its names: trimmed, no empty ones, each once (any case). A
// leading # is kept, it says the name is a tag.
func splitCommas(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" || p == "#" || seen[strings.ToLower(p)] {
			continue
		}
		seen[strings.ToLower(p)] = true
		out = append(out, p)
	}
	return out
}

// parts are the trimmed segments of a line. The result always has at least len(segs) entries.
func (pl *pipeLine) parts(line string) []string {
	raw := strings.Split(line, "|")
	for len(raw) < len(pl.segs) {
		raw = append(raw, "")
	}
	for i := range raw {
		raw[i] = strings.TrimSpace(raw[i])
	}
	return raw
}

// coreLine is the line the object's own save code reads: everything before the first link segment.
func (pl *pipeLine) coreLine(line string) string {
	if pl.core >= len(pl.segs) {
		return line
	}
	seen := 0
	for i, r := range line {
		if r == '|' {
			seen++
			if seen == pl.core {
				return strings.TrimRight(line[:i], " ")
			}
		}
	}
	return line
}

// pipeTok is one name typed in a link segment.
type pipeTok struct {
	Seg  int
	Text string
}

// tokens are the names of every link segment, in line order.
func (pl *pipeLine) tokens(line string) []pipeTok {
	parts := pl.parts(line)
	var out []pipeTok
	for i, s := range pl.segs {
		if !s.Link || i >= len(parts) {
			continue
		}
		for _, t := range splitCommas(parts[i]) {
			out = append(out, pipeTok{Seg: i, Text: t})
		}
	}
	return out
}

// --- the live report: closed segment checks and the soft indicator ---

// pipeReport is what the line says about itself while it is typed.
type pipeReport struct {
	cur int // segment the cursor is in
	// errSeg is the closed segment that failed, errIssue what failed. hasErr is false when none did.
	hasErr   bool
	errSeg   int
	errIssue validate.Issue
	// suggest is a smart correction (a phone number typed in the name), shown as a hint.
	suggest string
	// soft is the indicator of the segment being typed. It is never an error.
	soft string
}

// issueText words an issue the way every field error reads: code, field, what was expected, what was
// received and the fix.
func issueText(i validate.Issue) string {
	s := i.Code
	if i.Field != "" {
		s += " " + i.Field
	}
	if i.Expected != "" {
		s += ": " + i.Expected
	}
	if i.Received != "" {
		s += fmt.Sprintf(" (got %q)", i.Received)
	}
	if i.Fix != "" {
		s += ", " + i.Fix
	}
	return s
}

// checkSeg runs the one validation framework over a segment's text. Link segments hold object names,
// which are resolved on enter, so they have no check here.
func (m *Model) checkSeg(pl *pipeLine, i int, text string) validate.Result {
	if i >= len(pl.segs) || pl.segs[i].Link {
		return validate.Result{}
	}
	return validate.Field(pl.segs[i].Spec, text, validate.Options{Kind: pl.obj, ParseDate: m.parseDateField})
}

// pipeFirstError finds the first failing segment of the line other than skip (the one being typed).
// The segment just before skip is looked at first, so an error shows the moment a pipe closes it.
func (m *Model) pipeFirstError(pl *pipeLine, line string, skip int) (seg int, issue validate.Issue, ok bool) {
	parts := pl.parts(line)
	check := func(i int) bool {
		if i == skip || i >= len(pl.segs) || i >= len(parts) {
			return false
		}
		if errs := m.checkSeg(pl, i, parts[i]).Errors(); len(errs) > 0 {
			seg, issue, ok = i, errs[0], true
			return true
		}
		return false
	}
	if skip > 0 && check(skip-1) {
		return
	}
	for i := range parts {
		if check(i) {
			return
		}
	}
	return
}

// pipeReportFor builds the live report for the line and cursor (rune position).
func (m *Model) pipeReportFor(s *pipeSession, line string, cursor int) pipeReport {
	pl := s.pl
	rs := []rune(line)
	cur, start, end := segBounds(rs, cursor)
	r := pipeReport{cur: cur}
	r.errSeg, r.errIssue, r.hasErr = m.pipeFirstError(pl, line, cur)
	if cur >= len(pl.segs) {
		return r
	}
	seg := pl.segs[cur]
	text := strings.TrimSpace(string(rs[start:end]))
	// Smart corrections come from the same framework: any segment that was left or is being typed.
	parts := pl.parts(line)
	for i := range parts {
		if i == cur && text == "" {
			continue
		}
		if res := m.checkSeg(pl, i, parts[i]); len(res.Suggestions) > 0 {
			r.suggest = res.Suggestions[0].Code + ": " + res.Suggestions[0].Message
			break
		}
	}
	r.soft = m.pipeSoft(s, seg, rs, start, end, cursor)
	return r
}

// pipeSoft is the soft indicator of the segment being typed: what the text means so far. It is a hint
// and never an error.
func (m *Model) pipeSoft(s *pipeSession, seg pipeSeg, rs []rune, start, end, cursor int) string {
	text := strings.TrimSpace(string(rs[start:end]))
	if seg.Link {
		ts, te := tokenBounds(rs, start, end, cursor)
		tok := strings.TrimSpace(string(rs[ts:te]))
		if tok == "" {
			return ""
		}
		res := m.autoResolve(s, seg, tok)
		switch {
		case res.state == resolved:
			return "-> " + res.link.label
		case res.state == ambiguous:
			return fmt.Sprintf("%d match %q, enter asks which", len(res.matches), tok)
		case len(seg.Kinds) == 1 && seg.Kinds[0] != tagKind:
			return fmt.Sprintf("no %s named %q yet, enter offers to create it", relationLabel(seg.Kinds[0]), tok)
		case len(seg.Kinds) > 1:
			return fmt.Sprintf("nothing named %q yet, enter offers to create it", tok)
		}
		return ""
	}
	if text == "" {
		return ""
	}
	switch seg.Spec.Type {
	case model.FieldDate:
		if v, ok := m.parseDateField(text); ok {
			return text + " -> " + prettyDate(v)
		}
		return "a date: 2026-12-01 or next friday"
	case model.FieldChoice:
		var names []string
		for _, c := range seg.Spec.Choices {
			if strings.HasPrefix(strings.ToLower(c.Value), strings.ToLower(text)) || strings.HasPrefix(strings.ToLower(c.Label), strings.ToLower(text)) {
				names = append(names, c.Value)
			}
		}
		if len(names) == 0 {
			return "none of " + seg.Spec.ChoiceValues()
		}
		return "matches " + strings.Join(names, ", ")
	case model.FieldNumber:
		if res := validate.Field(seg.Spec, text, validate.Options{}); res.OK() && res.Value != text {
			return "-> " + res.Value
		}
	}
	return ""
}

// prettyDate words a stored date: Fri Nov 7, or Fri Nov 7 15:00 when it has a time.
func prettyDate(v string) string {
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02T15:04:05Z", "2006-01-02T15:04"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.Format("Mon Jan 2 15:04")
		}
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t.Format("Mon Jan 2")
	}
	return v
}
