package quick

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/shortsyntax"
)

// Env is what Parse reads. The caller fills it once per window of work, never per keystroke.
type Env struct {
	// Now is the clock. The zero value means time.Now().
	Now time.Time
	// Index finds people by name.
	Index *Index
	// Tags are the existing tag names. Set it (an empty list is fine) to flag new tags.
	Tags []string
	// NoteTypes are the active interaction types. Empty means the built-in five.
	NoteTypes []string
	// Context is the selected person. With it, no name is needed.
	Context int64
	// NoSyntax turns the inline tokens off. Verbs and names still work.
	NoSyntax bool
	// LoadPerson loads a person with details, to show old values. It may be nil.
	LoadPerson func(id int64) (*model.Person, error)
	// NoteType is the type of a note line. Empty means "note".
	NoteType string
	// DueOverride, when set, is the stored due date of a task line, whatever the text says.
	DueOverride string
	// TaskOrg and TaskDeal attach a task line to the organization or deal whose page it is typed on.
	TaskOrg, TaskDeal int64
	// Dismissed lists token texts the user turned back into plain text.
	Dismissed []string
	// Choices answer the names a plan asked about (Plan.Names), by NameRef.Key.
	Choices map[string]NameChoice
}

func (e Env) now() time.Time {
	if e.Now.IsZero() {
		return time.Now()
	}
	return e.Now
}

// NameRef is a name in the line that is not settled: it matched nobody, several people, or only
// the start of one name. The caller shows these in one list and replans with Env.Choices.
type NameRef struct {
	// Text is the name as typed. Token is the exact text in the line, such as @Sam_Lee.
	Text  string `json:"text"`
	Token string `json:"token"`
	// Key identifies the name in Env.Choices: the lower-case token for @names, lead:<name> for a
	// name at the start of the line.
	Key string `json:"key"`
	// Kind is link (an @name) or lead (a name that starts the line).
	Kind       string   `json:"kind"`
	Candidates []Person `json:"candidates,omitempty"`
	// Default is the candidate to preselect: 0 when there is exactly one, else -1 (keep as text).
	Default int `json:"default"`
	Start   int `json:"start"`
	End     int `json:"end"`
}

// NameChoice settles one NameRef. Action is link (connect to PersonID), text (keep the words as
// text) or create (the caller makes the person, then answers with link).
type NameChoice struct {
	Action   string `json:"action"`
	PersonID int64  `json:"person_id,omitempty"`
	Name     string `json:"name,omitempty"`
}

// Mark is one highlighted range of the input, as byte offsets (End is exclusive).
type Mark struct {
	// Kind is verb, subject, field, escape, bad or a shortsyntax kind (tag, due, person, ...).
	Kind  string `json:"kind"`
	Start int    `json:"start"`
	End   int    `json:"end"`
	New   bool   `json:"new,omitempty"`
	Bad   bool   `json:"bad,omitempty"`
}

// Problem stops a plan from running.
type Problem struct {
	// Code is one of no_person, ambiguous, not_found, invalid, empty, unknown_type.
	Code       string   `json:"code"`
	Message    string   `json:"message"`
	Candidates []Person `json:"candidates,omitempty"`
}

// Change is one write: planned in a Plan, done in a Result.
type Change struct {
	// Op is create, update, tag or untag.
	Op     string `json:"op"`
	Entity string `json:"entity"`
	ID     int64  `json:"id,omitempty"`
	Label  string `json:"label,omitempty"`
	Field  string `json:"field,omitempty"`
	Old    string `json:"old,omitempty"`
	New    string `json:"new,omitempty"`
	// Org is set on Op "org": the organization the company text created or reused and the
	// membership it made. Undo and redo use it.
	Org *repo.CompanyChange `json:"org,omitempty"`
}

// Plan is what a line would do.
type Plan struct {
	Input string `json:"input"`
	// Verb is note, task, log, set, tag or snooze. It is empty when the line is not a command.
	Verb     string              `json:"verb,omitempty"`
	Subject  *Person             `json:"subject,omitempty"`
	Links    []Person            `json:"links,omitempty"`
	Syntax   *shortsyntax.Result `json:"syntax,omitempty"`
	Problems []Problem           `json:"problems,omitempty"`
	// Names are the names that need a choice before the line can run.
	Names    []NameRef           `json:"names,omitempty"`
	Warnings []string            `json:"warnings,omitempty"`
	Changes  []Change            `json:"changes"`
	// Preview is one line that says what will happen.
	Preview string `json:"preview"`
	// NeedsConfirm is true for a multi-field change: show old and new, then ask.
	NeedsConfirm bool   `json:"needs_confirm,omitempty"`
	Marks        []Mark `json:"marks,omitempty"`

	ops []op
}

// Ready is true when the plan has something to do and nothing blocks it.
func (p *Plan) Ready() bool { return len(p.Problems) == 0 && len(p.ops) > 0 }

// Result is what Apply did.
type Result struct {
	Verb     string   `json:"verb"`
	Applied  bool     `json:"applied"`
	Changes  []Change `json:"changes"`
	Warnings []string `json:"warnings,omitempty"`
}

// Repos are the repositories Apply writes through.
type Repos struct {
	People       *repo.PersonRepo
	Interactions *repo.InteractionRepo
	Tasks        *repo.TaskRepo
	Tags         *repo.TagRepo
}

type runState struct{ interactionID, taskID int64 }

type op struct {
	preview []Change
	run     func(ctx context.Context, r Repos, st *runState) ([]Change, error)
}

var verbNames = map[string]string{
	"n": "note", "note": "note", "t": "task", "task": "task", "log": "log",
	"set": "set", "tag": "tag", "snooze": "snooze",
}

// Verbs are the command words, shortest first, for completion and help.
var Verbs = []string{"n", "note", "t", "task", "log", "set", "tag", "snooze"}

// VerbOf returns the verb a word stands for.
func VerbOf(word string) (string, bool) {
	v, ok := verbNames[strings.ToLower(word)]
	return v, ok
}

// IsCommand reports whether line starts with a verb followed by a space.
func IsCommand(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	w, end := firstWord(trimmed)
	if _, ok := VerbOf(w); !ok {
		return false
	}
	return end < len(trimmed)
}

type wspan struct {
	s          string
	start, end int
}

func wordsOf(s string, limit int) []wspan {
	var out []wspan
	start := -1
	for i, r := range s {
		if unicode.IsSpace(r) {
			if start >= 0 {
				out = append(out, wspan{s[start:i], start, i})
				start = -1
				if limit > 0 && len(out) >= limit {
					return out
				}
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, wspan{s[start:], start, len(s)})
	}
	return out
}

func firstWord(s string) (string, int) {
	for i, r := range s {
		if unicode.IsSpace(r) {
			return s[:i], i
		}
	}
	return s, len(s)
}

func skipSpace(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return i
}

func isSigilByte(c byte) bool { return strings.IndexByte("#@!~", c) >= 0 }

type builder struct {
	env    Env
	p      *Plan
	subj   *Person
	links  []Person
	loaded *model.Person
	tagMap map[string]string
	// soft: a "Name:" that matches nobody is plain text (note, task, log) instead of an error.
	soft bool
}

// Parse reads one line and returns what it would do. It writes nothing.
func Parse(input string, env Env) *Plan {
	p := &Plan{Input: input, Changes: []Change{}}
	b := &builder{env: env, p: p, tagMap: map[string]string{}}
	for _, t := range env.Tags {
		b.tagMap[shortsyntax.Fold(t)] = t
	}
	trimmed := strings.TrimLeft(input, " \t")
	lead := len(input) - len(trimmed)
	word, end := firstWord(trimmed)
	verb, ok := VerbOf(word)
	if !ok {
		return p
	}
	p.Verb = verb
	b.mark("verb", lead, lead+len(word))
	restOff := lead + skipSpace(trimmed, end)
	if lead+end > len(input) {
		restOff = len(input)
	}
	rest := input[min(restOff, len(input)):]
	switch verb {
	case "note", "log", "task":
		b.capture(verb, rest, restOff)
	case "set":
		b.set(rest, restOff)
	case "tag":
		b.tag(rest, restOff)
	case "snooze":
		b.snooze(rest, restOff)
	}
	sort.SliceStable(p.Marks, func(i, j int) bool { return p.Marks[i].Start < p.Marks[j].Start })
	p.NeedsConfirm = verb == "set" && len(p.Changes) >= 2
	return p
}

func (b *builder) mark(kind string, start, end int) {
	b.p.Marks = append(b.p.Marks, Mark{Kind: kind, Start: start, End: end})
}

func (b *builder) problem(code, msg string, cands ...Person) {
	b.p.Problems = append(b.p.Problems, Problem{Code: code, Message: msg, Candidates: cands})
}

func (b *builder) warn(format string, args ...any) {
	b.p.Warnings = append(b.p.Warnings, fmt.Sprintf(format, args...))
}

func (b *builder) contextPerson() *Person {
	if b.env.Context == 0 {
		return nil
	}
	if sp, ok := b.env.Index.Get(b.env.Context); ok {
		return &Person{ID: sp.ID, Name: sp.Name}
	}
	return &Person{ID: b.env.Context, Name: fmt.Sprintf("person %d", b.env.Context)}
}

// escapeLead handles a backslash before the first word: it turns off name matching for the line.
func (b *builder) escapeLead(rest string, off int) (string, int, bool) {
	if len(rest) > 1 && rest[0] == '\\' && !isSigilByte(rest[1]) {
		b.mark("escape", off, off+1)
		return rest[1:], off + 1, true
	}
	return rest, off, false
}

// leading reads a person name at the start of text. n is the number of bytes used including the
// spaces after the name, 0 when there is no name. A name followed by a colon is explicit and
// also wins over the selected person.
func (b *builder) leading(text string, off int) (n int, explicit bool) {
	ws := wordsOf(text, 4)
	var cands []wspan
	for _, w := range ws {
		if w.s == "" || isSigilByte(w.s[0]) || w.s[0] == '\\' || w.s[0] == '"' {
			break
		}
		cands = append(cands, w)
		if strings.HasSuffix(w.s, ":") {
			explicit = true
			break
		}
	}
	if len(cands) == 0 {
		return 0, false
	}
	nameOf := func(k int) string {
		parts := make([]string, k)
		for i := 0; i < k; i++ {
			parts[i] = cands[i].s
		}
		return strings.TrimRight(strings.Join(parts, " "), ":,")
	}
	use := func(k int, matches []Person) {
		w := cands[k-1]
		end := skipSpace(text, w.end)
		b.consumeName(off+cands[0].start, off+w.end, nameOf(k), matches)
		n = end
	}
	// A choice made in the names list settles a leading name: link it or keep the words as text.
	settled := func(k int) (done, skip bool) {
		ch, ok := b.env.Choices["lead:"+shortsyntax.Fold(nameOf(k))]
		if !ok {
			return false, false
		}
		if ch.Action == "link" {
			b.subj = &Person{ID: ch.PersonID, Name: ch.Name}
			b.p.Marks = append(b.p.Marks, Mark{Kind: "subject", Start: off + cands[0].start, End: off + cands[k-1].end})
			n = skipSpace(text, cands[k-1].end)
			return true, false
		}
		return false, true
	}
	if explicit && b.env.Index != nil {
		k := len(cands)
		if done, skip := settled(k); done {
			return n, true
		} else if skip {
			return 0, false
		}
		matches, _ := b.env.Index.ResolveLoose(nameOf(k))
		if len(matches) == 0 && b.soft {
			return 0, false
		}
		if len(matches) == 0 {
			b.problem("not_found", fmt.Sprintf("no person matches %q", nameOf(k)))
			b.p.Names = append(b.p.Names, NameRef{Text: nameOf(k), Token: text[cands[0].start:cands[k-1].end], Key: "lead:" + shortsyntax.Fold(nameOf(k)),
				Kind: "lead", Default: -1, Start: off + cands[0].start, End: off + cands[k-1].end})
			b.p.Marks = append(b.p.Marks, Mark{Kind: "subject", Start: off + cands[0].start, End: off + cands[k-1].end, Bad: true})
			return skipSpace(text, cands[k-1].end), true
		}
		use(k, matches)
		return n, true
	}
	if b.env.Index == nil {
		return 0, false
	}
	for k := len(cands); k >= 1; k-- {
		matches, exact := b.env.Index.Resolve(nameOf(k))
		if len(matches) == 0 || (k >= 2 && !exact) {
			continue
		}
		if done, skip := settled(k); done {
			return n, false
		} else if skip {
			continue
		}
		use(k, matches)
		return n, false
	}
	return 0, false
}

func (b *builder) consumeName(start, end int, name string, matches []Person) {
	if len(matches) == 1 {
		b.subj = &matches[0]
		b.p.Marks = append(b.p.Marks, Mark{Kind: "subject", Start: start, End: end})
		return
	}
	b.problem("ambiguous", fmt.Sprintf("%d people match %q", len(matches), name), matches...)
	b.p.Names = append(b.p.Names, NameRef{Text: name, Token: b.p.Input[start:end], Key: "lead:" + shortsyntax.Fold(name),
		Kind: "lead", Candidates: matches, Default: -1, Start: start, End: end})
	b.p.Marks = append(b.p.Marks, Mark{Kind: "subject", Start: start, End: end, Bad: true})
}

// subjectFor decides who the line is about: the selected person, a leading name, or the first +link.
// It returns the text that is left after a leading name.
func (b *builder) subjectFor(text string, off int, allowLead bool) (string, int) {
	b.subj = b.contextPerson()
	if !allowLead || b.env.Index == nil {
		return text, off
	}
	// An explicit "Name:" beats the selected person.
	explicitColon := false
	if ws := wordsOf(text, 4); len(ws) > 0 {
		for _, w := range ws {
			if w.s == "" || isSigilByte(w.s[0]) || w.s[0] == '\\' {
				break
			}
			if strings.HasSuffix(w.s, ":") {
				explicitColon = true
				break
			}
		}
	}
	if b.subj != nil && !explicitColon {
		return text, off
	}
	saved := b.subj
	b.subj = nil
	n, _ := b.leading(text, off)
	if b.subj == nil && len(b.p.Problems) == 0 {
		b.subj = saved
	}
	return text[n:], off + n
}

// syntax runs the short syntax parser and records the marks.
func (b *builder) syntax(text string, off int, dates bool, skip ...shortsyntax.Kind) shortsyntax.Result {
	dismiss := append([]string(nil), b.env.Dismissed...)
	for key, ch := range b.env.Choices {
		if ch.Action == "text" && strings.HasPrefix(key, "@") {
			dismiss = append(dismiss, key)
		}
	}
	opts := shortsyntax.Options{Now: b.env.now(), Disabled: b.env.NoSyntax, Dates: dates, Names: b.env.Index.NameSet(), Skip: skip, Dismiss: dismiss}
	if b.env.Tags != nil {
		opts.KnownTags = b.env.Tags
	}
	res := shortsyntax.Parse(text, opts)
	if plain := b.plainAtWords(res); len(plain) > 0 {
		opts.Dismiss = append(opts.Dismiss, plain...)
		res = shortsyntax.Parse(text, opts)
	}
	b.p.Syntax = &res
	for _, sp := range res.Spans {
		kind := string(sp.Kind)
		b.p.Marks = append(b.p.Marks, Mark{Kind: kind, Start: off + sp.Start, End: off + sp.End, New: sp.New})
	}
	return res
}

// plainAtWords lists the @word tokens that name nobody and are not a capitalized multi-word name
// such as @Sam_Lee. They stay plain text: no names list opens for them.
func (b *builder) plainAtWords(res shortsyntax.Result) []string {
	if b.env.Index == nil {
		return nil
	}
	var out []string
	for _, sp := range res.Spans {
		if sp.Kind != shortsyntax.KindPerson {
			continue
		}
		if m, _ := b.env.Index.ResolveLoose(sp.Value); len(m) > 0 {
			continue
		}
		if capitalizedMultiWord(sp.Value) {
			continue
		}
		if _, asked := b.env.Choices[strings.ToLower(sp.Text)]; asked {
			continue
		}
		out = append(out, sp.Text)
	}
	return out
}

// capitalizedMultiWord is true for two or more words that each start with a capital letter.
func capitalizedMultiWord(name string) bool {
	words := strings.Fields(strings.ReplaceAll(name, "_", " "))
	if len(words) < 2 {
		return false
	}
	for _, w := range words {
		r, _ := utf8.DecodeRuneInString(w)
		if !unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

// resolveLinks turns @Name tokens into people. A name that is settled (it matches exactly one
// person by full name, first name or last name) links at once. The rest become NameRefs for the
// caller to ask about, and block the plan until they are answered.
func (b *builder) resolveLinks(res shortsyntax.Result) {
	tokens := map[string]shortsyntax.Span{}
	for _, sp := range res.Spans {
		if sp.Kind == shortsyntax.KindPerson {
			if _, ok := tokens[shortsyntax.Fold(sp.Value)]; !ok {
				tokens[shortsyntax.Fold(sp.Value)] = sp
			}
		}
	}
	bad := map[string]bool{}
	for _, name := range res.People {
		sp := tokens[shortsyntax.Fold(name)]
		key := strings.ToLower(sp.Text)
		if ch, ok := b.env.Choices[key]; ok && ch.Action == "link" {
			b.addLink(Person{ID: ch.PersonID, Name: ch.Name})
			continue
		}
		matches, _ := b.env.Index.Resolve(name)
		if len(matches) == 1 {
			b.addLink(matches[0])
			continue
		}
		weak := false
		if len(matches) == 0 {
			matches, _ = b.env.Index.ResolveLoose(name)
			weak = len(matches) > 0
		}
		ref := NameRef{Text: name, Token: sp.Text, Key: key, Kind: "link", Candidates: matches, Default: -1}
		if len(matches) == 1 {
			ref.Default = 0
		}
		for _, m := range b.p.Marks {
			if m.Kind == "person" && b.p.Input[m.Start:m.End] == sp.Text {
				ref.Start, ref.End = m.Start, m.End
				break
			}
		}
		b.p.Names = append(b.p.Names, ref)
		bad[shortsyntax.Fold(name)] = true
		switch {
		case weak:
			b.problem("unconfirmed", fmt.Sprintf("@%s is only the start of a name: confirm %s", name, matches[0].Name), matches...)
		case len(matches) == 0:
			b.problem("not_found", fmt.Sprintf("no person matches @%s", name))
		default:
			b.problem("ambiguous", fmt.Sprintf("%d people match @%s", len(matches), name), matches...)
		}
	}
	if len(bad) == 0 {
		return
	}
	for i, m := range b.p.Marks {
		if m.Kind == "person" {
			name := strings.Trim(strings.TrimLeft(b.p.Input[m.Start:m.End], "@"), "\"")
			if bad[shortsyntax.Fold(strings.ReplaceAll(name, "_", " "))] {
				b.p.Marks[i].Bad = true
			}
		}
	}
}

func (b *builder) addLink(p Person) {
	if b.subj != nil && b.subj.ID == p.ID {
		return
	}
	for _, l := range b.links {
		if l.ID == p.ID {
			return
		}
	}
	b.links = append(b.links, p)
}
func (b *builder) person(id int64) (*model.Person, error) {
	if b.loaded != nil && b.loaded.ID == id {
		return b.loaded, nil
	}
	if b.env.LoadPerson == nil {
		return nil, fmt.Errorf("no way to load person %d", id)
	}
	p, err := b.env.LoadPerson(id)
	if err != nil {
		return nil, err
	}
	b.loaded = p
	return p, nil
}

func (b *builder) canonTag(name string) string {
	if t, ok := b.tagMap[shortsyntax.Fold(name)]; ok {
		return t
	}
	return name
}

// formatWhen writes a When as "Fri 16 Oct 15:00".
func formatWhen(w shortsyntax.When) string {
	if w.Date == "" {
		return w.Time
	}
	t, err := time.Parse("2006-01-02", w.Date)
	if err != nil {
		return w.String()
	}
	s := t.Format("Mon 2 Jan")
	if w.Time != "" {
		s += " " + w.Time
	}
	return s
}

func truncate(s string, n int) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n-1]) + "…"
}

func personNames(ps []Person) string {
	names := make([]string, len(ps))
	for i, p := range ps {
		names[i] = p.Name
	}
	return strings.Join(names, ", ")
}

func ids(subj *Person, links []Person) []int64 {
	var out []int64
	if subj != nil {
		out = append(out, subj.ID)
	}
	for _, l := range links {
		out = append(out, l.ID)
	}
	return out
}

// occurredAt is the stored time of a logged interaction: the date with the time or noon.
func occurredAt(w shortsyntax.When) string {
	t := w.Time
	if t == "" {
		t = "12:00"
	}
	return w.Date + " " + t + ":00"
}

func (b *builder) types() []string {
	if len(b.env.NoteTypes) > 0 {
		return b.env.NoteTypes
	}
	return model.InteractionTypes
}

// capture plans note, log and task lines.
func (b *builder) capture(verb, rest string, off int) {
	p := b.p
	typ := "note"
	if verb == "note" && b.env.NoteType != "" {
		typ = b.env.NoteType
	}
	b.soft = true
	if verb == "log" {
		w, end := firstWord(rest)
		if w == "" {
			p.Preview = "log <type> [name] text, e.g. log call Sam talked about the offsite  (types: " + strings.Join(b.types(), ", ") + ")"
			return
		}
		found := ""
		for _, t := range b.types() {
			if strings.EqualFold(t, w) {
				found = t
			}
		}
		if found == "" {
			b.problem("unknown_type", fmt.Sprintf("unknown type %q (types: %s)", w, strings.Join(b.types(), ", ")))
			b.p.Marks = append(b.p.Marks, Mark{Kind: "field", Start: off, End: off + len(w), Bad: true})
			return
		}
		typ = found
		b.mark("field", off, off+len(w))
		s := skipSpace(rest, end)
		rest, off = rest[s:], off+s
	}
	if strings.TrimSpace(rest) == "" {
		switch verb {
		case "task":
			p.Preview = "task [name] text fri 3pm by mon ~30m !high every 2 weeks @Name, e.g. t Sam call back fri"
		case "log":
			p.Preview = "log " + typ + " [name] text, e.g. log " + typ + " Sam talked about the offsite yesterday"
		default:
			p.Preview = "note [name] text #tag remind fri every 3m @Name, e.g. n Sam called about the deal #deal remind fri"
		}
		return
	}
	text, off, noLead := b.escapeLead(rest, off)
	text, off = b.subjectFor(text, off, !noLead)

	var res shortsyntax.Result
	switch verb {
	case "task":
		// Tasks read plain-word dates. A repeat is a task repeat, not a keep-in-touch cadence.
		res = b.syntax(text, off, true, shortsyntax.KindCadence)
	case "log":
		// For a log the date is when it happened. A repeat only counts when it is a cadence.
		res = b.syntax(text, off, true, shortsyntax.KindDeadline, shortsyntax.KindDuration, shortsyntax.KindPriority, shortsyntax.KindRecurrence)
	default:
		// Notes do not read bare words as dates. remind fri makes the reminder.
		res = b.syntax(text, off, false, shortsyntax.KindDeadline, shortsyntax.KindDuration, shortsyntax.KindPriority, shortsyntax.KindRecurrence)
	}
	b.resolveLinks(res)
	if b.subj == nil && len(b.links) > 0 && len(p.Problems) == 0 {
		b.subj, b.links = &b.links[0], b.links[1:]
	}
	p.Subject, p.Links = b.subj, b.links

	title := res.Title
	if title == "" {
		b.problem("empty", "nothing to save: type the text as well")
	}
	if len(p.Problems) > 0 {
		b.previewProblems()
		return
	}
	if verb == "task" {
		b.planTask(title, res)
		return
	}
	if b.subj == nil {
		b.problem("no_person", "who is this about? select a person, start with a name, or add +Name")
		b.previewProblems()
		return
	}
	b.planInteraction(typ, verb, title, res)
}

func (b *builder) previewProblems() {
	var msgs []string
	for _, pr := range b.p.Problems {
		msgs = append(msgs, pr.Message)
	}
	b.p.Preview = strings.Join(msgs, "; ")
}

func (b *builder) planInteraction(typ, verb, content string, res shortsyntax.Result) {
	p := b.p
	subj, links := b.subj, b.links
	who := subj.Name
	if len(links) > 0 {
		who += " with " + personNames(links)
	}
	var occurred string
	if verb == "log" && res.Due != nil {
		occurred = occurredAt(*res.Due)
	}
	pid := ids(subj, links)
	label := typ + " on " + who

	p.ops = append(p.ops, op{
		preview: []Change{{Op: "create", Entity: "interaction", Label: label, New: content}},
		run: func(ctx context.Context, r Repos, st *runState) ([]Change, error) {
			in := model.CreateInteractionInput{Type: typ, Content: &content, PersonIDs: pid}
			if occurred != "" {
				in.OccurredAt = &occurred
			}
			it, err := r.Interactions.Create(ctx, in)
			if err != nil {
				return nil, err
			}
			st.interactionID = it.ID
			return []Change{{Op: "create", Entity: "interaction", ID: it.ID, Label: label, New: content}}, nil
		},
	})
	parts := []string{fmt.Sprintf("%s on %s: %q", typ, who, truncate(content, 60))}
	if occurred != "" {
		parts[0] += " (" + formatWhen(*res.Due) + ")"
	}
	for _, t := range res.Tags {
		tag := b.canonTag(t)
		p.ops = append(p.ops, op{
			preview: []Change{{Op: "tag", Entity: "interaction", Field: "tag", New: tag}},
			run: func(ctx context.Context, r Repos, st *runState) ([]Change, error) {
				if err := r.Tags.Apply(ctx, "interaction", st.interactionID, tag); err != nil {
					return nil, err
				}
				return []Change{{Op: "tag", Entity: "interaction", ID: st.interactionID, Field: "tag", New: tag}}, nil
			},
		})
		s := "tag #" + tag
		if _, known := b.tagMap[shortsyntax.Fold(t)]; !known && b.env.Tags != nil {
			s += " (new)"
		}
		parts = append(parts, s)
	}
	if verb == "note" && res.Due != nil {
		due := res.Due.String()
		title := "Follow up: " + truncate(content, 60)
		prio := "medium"
		tlinks := links
		tsubj := subj
		p.ops = append(p.ops, op{
			preview: []Change{{Op: "create", Entity: "task", Label: title, Field: "due", New: due}},
			run: func(ctx context.Context, r Repos, st *runState) ([]Change, error) {
				in := model.CreateTaskInput{Title: title, Priority: prio, DueAt: &due, PersonID: &tsubj.ID}
				for _, l := range tlinks {
					in.PersonIDs = append(in.PersonIDs, l.ID)
				}
				t, err := r.Tasks.Create(ctx, in)
				if err != nil {
					return nil, err
				}
				return []Change{{Op: "create", Entity: "task", ID: t.ID, Label: title, Field: "due", New: due}}, nil
			},
		})
		parts = append(parts, "remind "+formatWhen(*res.Due))
	}
	if res.Cadence != "" {
		if c, ok := b.cadenceChange(subj, res.Cadence); ok {
			p.ops = append(p.ops, c)
			parts = append(parts, "keep in touch "+res.Cadence)
		}
	}
	for _, c := range p.ops {
		p.Changes = append(p.Changes, c.preview...)
	}
	p.Preview = strings.Join(parts, " | ")
}

// cadenceChange plans a keep-in-touch change on a person, or nothing when it is already set.
func (b *builder) cadenceChange(subj *Person, cadence string) (op, bool) {
	var out op
	old := ""
	if person, err := b.person(subj.ID); err == nil {
		old = strOf(person.KeepInTouch)
	}
	if old == cadence {
		b.warn("%s is already on %s", subj.Name, cadence)
		return out, false
	}
	def, _ := lookupField("cadence")
	ch := Change{Op: "update", Entity: "person", ID: subj.ID, Label: subj.Name, Field: "cadence", Old: old, New: cadence}
	out = op{
		preview: []Change{ch},
		run: func(ctx context.Context, r Repos, st *runState) ([]Change, error) {
			in := model.UpdatePersonInput{}
			def.set(&in, cadence)
			if _, err := r.People.Update(ctx, subj.ID, in); err != nil {
				return nil, err
			}
			return []Change{ch}, nil
		},
	}
	return out, true
}

func (b *builder) planTask(title string, res shortsyntax.Result) {
	p := b.p
	subj, links := b.subj, b.links
	if subj == nil && len(links) > 0 {
		subj, links = &links[0], links[1:]
		p.Subject, p.Links = subj, links
	}
	tf := TaskFieldsOf(res)
	prio, due, description := tf.Priority, tf.Due, tf.Description
	if b.env.DueOverride != "" {
		due = b.env.DueOverride
	}
	if res.Recurrence != "" && tf.Recurrence != "" && due == "" {
		b.problem("invalid", fmt.Sprintf("%q needs a date to start from: add one, such as fri", res.Recurrence))
		b.previewProblems()
		return
	}
	if res.Recurrence != "" && tf.Recurrence == "" {
		b.warn("%q is not an interval, so it is kept in the description", res.Recurrence)
	}
	pid := ids(subj, links)
	parts := []string{}
	if subj != nil {
		who := subj.Name
		if len(links) > 0 {
			who += " with " + personNames(links)
		}
		parts = append(parts, fmt.Sprintf("task for %s: %q", who, truncate(title, 60)))
	} else {
		parts = append(parts, fmt.Sprintf("task: %q", truncate(title, 60)))
	}
	if b.env.DueOverride != "" {
		parts = append(parts, "due "+b.env.DueOverride)
	} else if res.Due != nil {
		parts = append(parts, "due "+formatWhen(*res.Due))
	}
	if res.Deadline != nil && res.Due != nil {
		b.warn("the deadline %q is ignored because the task already has a due date", formatWhen(*res.Deadline))
	} else if res.Deadline != nil && b.env.DueOverride == "" {
		parts = append(parts, "due "+formatWhen(*res.Deadline))
	}
	if res.Recurrence != "" {
		parts = append(parts, "repeats "+strings.TrimPrefix(res.Recurrence, "every "))
	}
	if res.Priority != "" {
		parts = append(parts, res.TaskPriority()+" priority")
	}
	if res.DurationMin > 0 {
		b.warn("the duration %s is ignored: tasks have no duration field", formatMinutes(res.DurationMin))
	}
	ch := Change{Op: "create", Entity: "task", Label: title, Field: "due", New: due}
	p.ops = append(p.ops, op{
		preview: []Change{ch},
		run: func(ctx context.Context, r Repos, st *runState) ([]Change, error) {
			in := model.CreateTaskInput{Title: title, Priority: prio}
			if due != "" {
				in.DueAt = &due
			}
			if tf.Recurrence != "" {
				rec := tf.Recurrence
				in.Recurrence = &rec
			}
			if description != "" {
				in.Description = &description
			}
			if len(pid) > 0 {
				first := pid[0]
				in.PersonID = &first
				in.PersonIDs = pid[1:]
			}
			if b.env.TaskOrg != 0 {
				in.OrgIDs = []int64{b.env.TaskOrg}
			}
			if b.env.TaskDeal != 0 {
				in.DealIDs = []int64{b.env.TaskDeal}
			}
			t, err := r.Tasks.Create(ctx, in)
			if err != nil {
				return nil, err
			}
			st.taskID = t.ID
			done := ch
			done.ID = t.ID
			return []Change{done}, nil
		},
	})
	p.Changes = append(p.Changes, ch)
	for _, t := range res.Tags {
		tag := b.canonTag(t)
		tch := Change{Op: "tag", Entity: "task", Field: "tag", New: tag}
		p.ops = append(p.ops, op{
			preview: []Change{tch},
			run: func(ctx context.Context, r Repos, st *runState) ([]Change, error) {
				if err := r.Tags.Apply(ctx, "task", st.taskID, tag); err != nil {
					return nil, err
				}
				done := tch
				done.ID = st.taskID
				return []Change{done}, nil
			},
		})
		p.Changes = append(p.Changes, tch)
		s := "tag #" + tag
		if _, known := b.tagMap[shortsyntax.Fold(t)]; !known && b.env.Tags != nil {
			s += " (new)"
		}
		parts = append(parts, s)
	}
	p.Preview = strings.Join(parts, " | ")
}

// assignRe finds "key=" at the start of a word.
var assignRe = regexp.MustCompile(`(^|\s)([A-Za-z][A-Za-z_-]*)=`)

type assignment struct {
	key, value string
	start, end int // byte range of "key=value" in the text
}

func splitAssignments(text string) (prefix string, as []assignment) {
	locs := assignRe.FindAllStringSubmatchIndex(text, -1)
	if len(locs) == 0 {
		return text, nil
	}
	prefix = text[:locs[0][2]]
	for i, l := range locs {
		keyStart, keyEnd := l[4], l[5]
		valStart := l[1]
		valEnd := len(text)
		if i+1 < len(locs) {
			valEnd = locs[i+1][2]
		}
		v := strings.TrimSpace(text[valStart:valEnd])
		if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
			v = v[1 : len(v)-1]
		}
		as = append(as, assignment{key: text[keyStart:keyEnd], value: v, start: keyStart, end: valEnd})
	}
	return strings.TrimSpace(prefix), as
}

// set plans "set [Name:] key=value ... every 3m !high #tag".
func (b *builder) set(rest string, off int) {
	p := b.p
	if strings.TrimSpace(rest) == "" {
		p.Preview = "set [name:] company=Acme title=CTO birthday=Mar 4 every 3m !high #tag"
		return
	}
	text, off, noLead := b.escapeLead(rest, off)
	text, off = b.subjectFor(text, off, !noLead)
	res := b.syntax(text, off, false, shortsyntax.KindDue, shortsyntax.KindDeadline, shortsyntax.KindDuration, shortsyntax.KindPerson, shortsyntax.KindRecurrence)
	clean := res.Title
	prefix, as := splitAssignments(clean)
	if b.subj == nil && prefix != "" {
		// "set Samuel title=CTO": the words before the first key are the name.
		name := strings.TrimRight(prefix, ":, ")
		matches, _ := b.env.Index.ResolveLoose(name)
		switch {
		case len(matches) == 1:
			b.subj = &matches[0]
		case len(matches) > 1:
			b.problem("ambiguous", fmt.Sprintf("%d people match %q", len(matches), name), matches...)
		default:
			b.problem("not_found", fmt.Sprintf("no person matches %q", name))
		}
		prefix = ""
	}
	if b.subj == nil && len(p.Problems) == 0 {
		b.problem("no_person", "whose record? select a person or write set Name: field=value")
	}
	if prefix != "" {
		b.warn("ignored text: %s", prefix)
	}
	p.Subject = b.subj
	if len(p.Problems) > 0 {
		b.previewProblems()
		return
	}
	person, err := b.person(b.subj.ID)
	if err != nil {
		b.problem("not_found", err.Error())
		b.previewProblems()
		return
	}
	// Mark the keys in the input. Offsets: the cleaned text is not the input, so find each key.
	b.markKeys(text, off, as)

	update := model.UpdatePersonInput{}
	var fieldChanges []Change
	var handleChanges []Change
	seen := map[string]bool{}
	apply := func(field string, def *fieldDef, newVal string) {
		old := def.get(person)
		if old == newVal {
			b.warn("%s is already %s", field, orEmpty(old))
			return
		}
		def.set(&update, newVal)
		fieldChanges = append(fieldChanges, Change{Op: "update", Entity: "person", ID: b.subj.ID, Label: b.subj.Name, Field: field, Old: old, New: newVal})
	}
	for _, a := range as {
		if kind, ok := handleKey(a.key); ok {
			if seen[kind] {
				continue
			}
			seen[kind] = true
			old := handleValue(person, kind)
			nv := strings.TrimSpace(a.value)
			if old == nv {
				b.warn("%s is already %s", kind, orEmpty(old))
				continue
			}
			handleChanges = append(handleChanges, Change{Op: "update", Entity: "person", ID: b.subj.ID, Label: b.subj.Name, Field: kind, Old: old, New: nv})
			continue
		}
		def, ok := lookupField(a.key)
		if !ok {
			b.problem("invalid", fmt.Sprintf("unknown field %q (try company, title, email, phone, location, birthday, cadence, tier, favorite, notes, name)", a.key))
			continue
		}
		if seen[def.name] {
			continue
		}
		seen[def.name] = true
		nv, err := def.norm(a.value)
		if err != nil {
			b.problem("invalid", err.Error())
			continue
		}
		apply(def.name, def, nv)
	}
	if res.Cadence != "" && !seen["cadence"] {
		def, _ := lookupField("cadence")
		apply("cadence", def, res.Cadence)
	}
	if res.Priority != "" && !seen["tier"] {
		def, _ := lookupField("tier")
		apply("tier", def, res.Priority)
	}
	if len(p.Problems) > 0 {
		b.previewProblems()
		return
	}

	subj := b.subj
	if len(fieldChanges) > 0 {
		in := update
		changes := fieldChanges
		p.ops = append(p.ops, op{
			preview: changes,
			run: func(ctx context.Context, r Repos, st *runState) ([]Change, error) {
				if _, err := r.People.Update(ctx, subj.ID, in); err != nil {
					return nil, err
				}
				if in.Company == nil || *in.Company == "" {
					return changes, nil
				}
				// Company text creates or reuses an organization (undone with the same step).
				oc, err := repo.NewOrgAutoRepo(r.People.DB()).ApplyCompany(ctx, subj.ID, *in.Company, repo.ModePrimary)
				if err != nil {
					return nil, err
				}
				out := append([]Change{}, changes...)
				if !oc.Empty() {
					out = append(out, Change{Op: "org", Entity: "person", ID: subj.ID, Label: subj.Name, Field: "organization", New: oc.OrgName, Org: &oc})
				}
				return out, nil
			},
		})
	}
	for _, hc := range handleChanges {
		hc := hc
		p.ops = append(p.ops, op{
			preview: []Change{hc},
			run: func(ctx context.Context, r Repos, st *runState) ([]Change, error) {
				var err error
				if hc.New == "" {
					err = r.People.RemoveHandle(ctx, hc.ID, hc.Field)
				} else {
					err = r.People.SetHandle(ctx, hc.ID, hc.Field, hc.New)
				}
				if err != nil {
					return nil, err
				}
				return []Change{hc}, nil
			},
		})
	}
	var tagParts []string
	for _, t := range res.Tags {
		tag := b.canonTag(t)
		if b.env.Index.HasTag(subj.ID, tag) {
			b.warn("%s already has #%s", subj.Name, tag)
			continue
		}
		p.ops = append(p.ops, b.tagOp(subj, tag, false))
		s := "#" + tag
		if _, known := b.tagMap[shortsyntax.Fold(t)]; !known && b.env.Tags != nil {
			s += " (new)"
		}
		tagParts = append(tagParts, s)
	}
	for _, o := range p.ops {
		p.Changes = append(p.Changes, o.preview...)
	}
	if len(p.ops) == 0 {
		if len(as) == 0 && res.Cadence == "" && res.Priority == "" && len(res.Tags) == 0 {
			p.Preview = "set " + subj.Name + ": field=value ... (company, title, email, phone, location, birthday, cadence, tier, favorite, notes, name)"
		} else {
			p.Preview = "nothing to change for " + subj.Name
		}
		return
	}
	var summary []string
	for _, c := range append(append([]Change{}, fieldChanges...), handleChanges...) {
		summary = append(summary, fmt.Sprintf("%s %s", c.Field, orEmpty(c.New)))
	}
	summary = append(summary, tagParts...)
	p.Preview = "set " + subj.Name + ": " + strings.Join(summary, ", ")
}

func orEmpty(s string) string {
	if s == "" {
		return "(empty)"
	}
	return s
}

// markKeys highlights each "key=" in the input.
func (b *builder) markKeys(text string, off int, as []assignment) {
	for _, a := range as {
		// The assignment offsets are in the cleaned text, so search the input for the key.
		idx := strings.Index(strings.ToLower(text), strings.ToLower(a.key)+"=")
		if idx < 0 {
			continue
		}
		b.p.Marks = append(b.p.Marks, Mark{Kind: "field", Start: off + idx, End: off + idx + len(a.key) + 1})
		// Blank the key so the next identical key is found later.
		text = text[:idx] + strings.Repeat("\x00", len(a.key)+1) + text[idx+len(a.key)+1:]
	}
}

func (b *builder) tagOp(subj *Person, tag string, remove bool) op {
	opName := "tag"
	if remove {
		opName = "untag"
	}
	ch := Change{Op: opName, Entity: "person", ID: subj.ID, Label: subj.Name, Field: "tag", New: tag}
	return op{
		preview: []Change{ch},
		run: func(ctx context.Context, r Repos, st *runState) ([]Change, error) {
			var err error
			if remove {
				err = r.Tags.Remove(ctx, "person", subj.ID, tag)
			} else {
				err = r.Tags.Apply(ctx, "person", subj.ID, tag)
			}
			if err != nil {
				return nil, err
			}
			return []Change{ch}, nil
		},
	}
}

var tagNameRe = regexp.MustCompile(`^[\p{L}][\p{L}\p{N}_-]*$`)

// tag plans "tag [Name] friend vip -old".
func (b *builder) tag(rest string, off int) {
	p := b.p
	if strings.TrimSpace(rest) == "" {
		p.Preview = "tag [name] friend vip -old  (a leading - removes a tag)"
		return
	}
	text, off, noLead := b.escapeLead(rest, off)
	text, off = b.subjectFor(text, off, !noLead)
	if b.subj == nil && len(p.Problems) == 0 {
		b.problem("no_person", "who? select a person or start with a name")
	}
	p.Subject = b.subj
	if len(p.Problems) > 0 {
		b.previewProblems()
		return
	}
	subj := b.subj
	var parts []string
	for _, w := range wordsOf(text, 0) {
		s := w.s
		remove := false
		if strings.HasPrefix(s, "-") {
			remove, s = true, s[1:]
		}
		s = strings.TrimLeft(s, "#")
		s = strings.TrimRight(s, ",;")
		if s == "" {
			continue
		}
		if !tagNameRe.MatchString(s) {
			b.problem("invalid", fmt.Sprintf("%q is not a tag name (letters, digits, - and _)", s))
			b.p.Marks = append(b.p.Marks, Mark{Kind: "tag", Start: off + w.start, End: off + w.end, Bad: true})
			continue
		}
		tag := b.canonTag(s)
		mk := Mark{Kind: "tag", Start: off + w.start, End: off + w.end}
		if remove {
			if !b.env.Index.HasTag(subj.ID, tag) {
				b.warn("%s does not have #%s", subj.Name, tag)
				p.Marks = append(p.Marks, mk)
				continue
			}
			p.ops = append(p.ops, b.tagOp(subj, tag, true))
			parts = append(parts, "-#"+tag)
		} else {
			_, known := b.tagMap[shortsyntax.Fold(s)]
			mk.New = !known && b.env.Tags != nil
			if b.env.Index.HasTag(subj.ID, tag) {
				b.warn("%s already has #%s", subj.Name, tag)
				p.Marks = append(p.Marks, mk)
				continue
			}
			p.ops = append(p.ops, b.tagOp(subj, tag, false))
			label := "+#" + tag
			if mk.New {
				label += " (new)"
			}
			parts = append(parts, label)
		}
		p.Marks = append(p.Marks, mk)
	}
	if len(p.Problems) > 0 {
		b.previewProblems()
		return
	}
	for _, o := range p.ops {
		p.Changes = append(p.Changes, o.preview...)
	}
	if len(p.ops) == 0 {
		p.Preview = "tag " + subj.Name + ": nothing to change"
		return
	}
	p.Preview = "tag " + subj.Name + ": " + strings.Join(parts, " ")
}

var daysRe = regexp.MustCompile(`^\d{1,4}$`)

// snooze plans "snooze [Name] 7", "snooze Sam 2w" or "snooze Sam fri".
func (b *builder) snooze(rest string, off int) {
	p := b.p
	if strings.TrimSpace(rest) == "" {
		p.Preview = "snooze [name] 7  (days, or 2w, or fri)"
		return
	}
	text, off, noLead := b.escapeLead(rest, off)
	text, off = b.subjectFor(text, off, !noLead)
	if b.subj == nil && len(p.Problems) == 0 {
		b.problem("no_person", "who? select a person or start with a name")
	}
	p.Subject = b.subj
	if len(p.Problems) > 0 {
		b.previewProblems()
		return
	}
	arg := strings.TrimSpace(text)
	if arg == "" {
		p.Preview = "snooze " + b.subj.Name + " 7  (days, or 2w, or fri)"
		return
	}
	now := b.env.now()
	var until, human string
	if daysRe.MatchString(arg) {
		var n int
		fmt.Sscanf(arg, "%d", &n)
		until = now.UTC().AddDate(0, 0, n).Format("2006-01-02 15:04:05")
		human = fmt.Sprintf("%d days", n)
		b.mark("due", off, off+len(arg))
	} else {
		// "by <date>" takes every date form, so 2w and next week work as well as fri.
		sres := shortsyntax.Parse("by "+arg, shortsyntax.Options{Now: now, Skip: []shortsyntax.Kind{
			shortsyntax.KindTag, shortsyntax.KindDuration, shortsyntax.KindCadence, shortsyntax.KindPriority, shortsyntax.KindPerson, shortsyntax.KindRecurrence}})
		res := sres
		res.Due = sres.Deadline
		if res.Due == nil || res.Title != "" {
			b.problem("invalid", fmt.Sprintf("cannot read %q as a number of days or a date", arg))
			b.p.Marks = append(b.p.Marks, Mark{Kind: "due", Start: off, End: off + len(arg), Bad: true})
			b.previewProblems()
			return
		}
		t := res.Due.Time
		if t == "" {
			t = "00:00"
		}
		until = res.Due.Date + " " + t + ":00"
		human = "until " + formatWhen(*res.Due)
		b.mark("due", off, off+len(arg))
	}
	subj := b.subj
	old := ""
	if person, err := b.person(subj.ID); err == nil {
		old = strOf(person.KeepInTouchSnoozedUntil)
	}
	def, _ := lookupField("snooze")
	ch := Change{Op: "update", Entity: "person", ID: subj.ID, Label: subj.Name, Field: "snooze", Old: old, New: until}
	p.ops = append(p.ops, op{
		preview: []Change{ch},
		run: func(ctx context.Context, r Repos, st *runState) ([]Change, error) {
			in := model.UpdatePersonInput{}
			def.set(&in, until)
			if _, err := r.People.Update(ctx, subj.ID, in); err != nil {
				return nil, err
			}
			return []Change{ch}, nil
		},
	})
	p.Changes = append(p.Changes, ch)
	p.Preview = "snooze " + subj.Name + " " + human
}
