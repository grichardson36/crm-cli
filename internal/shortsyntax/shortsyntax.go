// Package shortsyntax parses inline short syntax out of free text: tags, people, dates,
// deadlines, durations, priorities, repeats and places. It is a pure function of the text, the
// options and a clock the caller injects, so the CLI, the MCP server, the TUI and agents read the
// same text the same way.
//
// The syntax follows what quick-add boxes in other tools do: a symbol for a named thing, plain
// English for time.
//
//	#tag            tag. Must start with a letter. Unknown tags are flagged "new" when KnownTags is set.
//	@Name           link a person: @Sam, @Sam_Lee, @"Sam Lee" (a known full name also matches @Sam Lee)
//	!high !!  !!!   priority: !top !high !medium !low !none, !! is medium and !!! is high
//	~30m  for 30m   duration: 30m, 1h, 1.5h, 1h30m
//	by fri          deadline: any date below after the word by
//	remind fri 3pm  due date or time, also inside notes (where bare dates are not read)
//	fri 3pm         due date or time as plain words (only when Options.Dates is set):
//	                today, tomorrow, fri, next week, in 2 weeks, mar 4, 2026-11-01, 3pm, 15:00, noon
//	every 3m        repeat. every day, every fri, every 2 weeks stay as raw text. A cadence that
//	                crm knows (every 1w, 2w, 6w, 1m, 3m, 6m, 1y) also sets the keep-in-touch cadence
//	at Place        location (only when Options.Phrases is set)
//	\#              a backslash before a sigil keeps the text literal. Text in "double quotes" is not read.
//
// A token counts as syntax only when it parses validly. "a@b.com", "#42", "~5 items" and
// "every time" are plain text. Whatever is recognized is removed from the title.
package shortsyntax

import (
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Kind names what a token sets.
type Kind string

// Token kinds.
const (
	KindTag        Kind = "tag"
	KindDue        Kind = "due"
	KindDeadline   Kind = "deadline"
	KindDuration   Kind = "duration"
	KindCadence    Kind = "cadence"
	KindPerson     Kind = "person"
	KindPriority   Kind = "priority"
	KindLocation   Kind = "location"
	KindRecurrence Kind = "recurrence"
	KindEscape     Kind = "escape"
)

// Span is one recognized token. Start and End are byte offsets into the input (End is exclusive).
type Span struct {
	Kind  Kind   `json:"kind"`
	Start int    `json:"start"`
	End   int    `json:"end"`
	Text  string `json:"text"`
	Value string `json:"value,omitempty"`
	// New marks a #tag that is not in Options.KnownTags.
	New bool `json:"new,omitempty"`
}

// When is a date, a time, or both. Date is YYYY-MM-DD and Time is HH:MM (24 hour).
type When struct {
	Date string `json:"date,omitempty"`
	Time string `json:"time,omitempty"`
}

// IsZero reports whether neither part is set.
func (w When) IsZero() bool { return w.Date == "" && w.Time == "" }

// String is "2006-01-02 15:04", or just the date for an all-day value.
func (w When) String() string {
	switch {
	case w.Date != "" && w.Time != "":
		return w.Date + " " + w.Time
	case w.Date != "":
		return w.Date
	}
	return w.Time
}

// Result is what Parse found.
type Result struct {
	Input string `json:"input"`
	// Title is the input without the recognized tokens.
	Title       string   `json:"title"`
	Tags        []string `json:"tags,omitempty"`
	NewTags     []string `json:"new_tags,omitempty"`
	Due         *When    `json:"due,omitempty"`
	Deadline    *When    `json:"deadline,omitempty"`
	DurationMin int      `json:"duration_min,omitempty"`
	// Cadence is a keep-in-touch cadence from model.KeepInTouchCadences, read from "every 3m".
	Cadence string `json:"cadence,omitempty"`
	// People are the names after @, as typed (underscores become spaces).
	People []string `json:"people,omitempty"`
	// Priority is top, high, medium, low or none.
	Priority string `json:"priority,omitempty"`
	Location string `json:"location,omitempty"`
	// Recurrence is the raw "every ..." phrase, lower case: "every fri", "every 2 weeks".
	Recurrence string `json:"recurrence,omitempty"`
	Spans      []Span `json:"spans,omitempty"`
}

// Empty reports whether no token was recognized.
func (r Result) Empty() bool {
	for _, s := range r.Spans {
		if s.Kind != KindEscape {
			return false
		}
	}
	return true
}

// Tier maps Priority to a person priority tier: 1 top, 2 high, 3 medium, 4 low, 5 none. 0 means unset.
func (r Result) Tier() int {
	switch r.Priority {
	case "top":
		return 1
	case "high":
		return 2
	case "medium":
		return 3
	case "low":
		return 4
	case "none":
		return 5
	}
	return 0
}

// TaskPriority maps Priority to a task priority (low, medium, high). It is "" when unset.
func (r Result) TaskPriority() string {
	switch r.Priority {
	case "top", "high":
		return "high"
	case "medium":
		return "medium"
	case "low", "none":
		return "low"
	}
	return ""
}

// Options control a parse. The zero value reads tags, people, priorities, durations, "by", "remind",
// "for" and "every". Plain-word dates and "at <place>" are opt-in.
type Options struct {
	// Now is the clock for relative dates. The zero value means time.Now().
	Now time.Time
	// Disabled turns the syntax off: the title is the input and nothing is recognized.
	Disabled bool
	// Dates reads dates and times written as plain words (fri, tomorrow 3pm, next week). Ordinary
	// sentences contain such words, so notes leave this off and use "remind fri" instead.
	Dates bool
	// Phrases enables "at <place>".
	Phrases bool
	// KnownTags, when not nil, flags tags outside the list as new. Matching ignores case.
	KnownTags []string
	// People are known full names. They let "@Sam Lee" match two words.
	People []string
	// Names is People already prepared by NewNameSet. Build it once when the list is long; it wins over People.
	Names *NameSet
	// Skip lists token kinds to leave as plain text.
	Skip []Kind
	// Dismiss lists token texts the user turned back into plain text (matched ignoring case).
	Dismiss []string
}

// Parse reads the short syntax in text.
func Parse(text string, opts Options) Result {
	res := Result{Input: text, Title: text}
	if opts.Disabled || text == "" {
		return res
	}
	var skipAt []rng
	for round := 0; round < 4; round++ {
		p := newParser(text, opts)
		p.skipAt = skipAt
		p.run()
		res = p.finish()
		if len(opts.Dismiss) == 0 {
			return res
		}
		dismissed := map[string]bool{}
		for _, d := range opts.Dismiss {
			dismissed[strings.ToLower(strings.TrimSpace(d))] = true
		}
		again := false
		for _, sp := range res.Spans {
			if sp.Kind == KindEscape || !dismissed[strings.ToLower(sp.Text)] {
				continue
			}
			seen := false
			for _, r := range skipAt {
				if r.a == sp.Start {
					seen = true
				}
			}
			if !seen {
				skipAt = append(skipAt, rng{sp.Start, sp.End})
				again = true
			}
		}
		if !again {
			return res
		}
	}
	return res
}

type rng struct{ a, b int }

type word struct {
	text       string
	start, end int
}

type parser struct {
	text   string
	words  []word
	quoted []bool
	opts   Options
	now    time.Time
	skip   map[Kind]bool
	skipAt []rng

	known    map[string]bool
	flagNew  bool
	people   map[string]bool
	maxNames int

	res      Result
	spans    []Span
	drop     []rng
	esc      []int
	due      *partial
	deadline *partial
	tagSeen  map[string]bool
	perSeen  map[string]bool
}

func newParser(text string, opts Options) *parser {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	p := &parser{text: text, opts: opts, now: now.Truncate(time.Minute), skip: map[Kind]bool{},
		tagSeen: map[string]bool{}, perSeen: map[string]bool{}}
	for _, k := range opts.Skip {
		p.skip[k] = true
	}
	if opts.KnownTags != nil {
		p.flagNew = true
		p.known = map[string]bool{}
		for _, t := range opts.KnownTags {
			p.known[Fold(t)] = true
		}
	}
	switch {
	case opts.Names != nil && len(opts.Names.set) > 0:
		p.people, p.maxNames = opts.Names.set, opts.Names.max
	case len(opts.People) > 0:
		ns := NewNameSet(opts.People)
		p.people, p.maxNames = ns.set, ns.max
	}
	p.res = Result{Input: text}
	p.words = splitWords(text)
	p.quoted = markQuoted(p.words)
	return p
}

// markQuoted flags the words inside "double quotes". They are plain text. An opening quote with
// no closing quote does not count. A quote right after @ belongs to a name: @"Sam Lee".
func markQuoted(ws []word) []bool {
	q := make([]bool, len(ws))
	open := -1
	for i, w := range ws {
		t := w.text
		if open < 0 {
			j := leadLen(t)
			if j < len(t) && t[j] == '"' {
				rest := t[j+1:]
				if strings.Contains(rest, `"`) {
					q[i] = true // "one word"
				} else {
					open = i
				}
			}
			continue
		}
		if strings.Contains(t, `"`) {
			for k := open; k <= i; k++ {
				q[k] = true
			}
			open = -1
		}
	}
	return q
}

func splitWords(s string) []word {
	var out []word
	start := -1
	for i, r := range s {
		if unicode.IsSpace(r) {
			if start >= 0 {
				out = append(out, word{s[start:i], start, i})
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, word{s[start:], start, len(s)})
	}
	return out
}

func (p *parser) run() {
	for i := 0; i < len(p.words); {
		n := p.token(i)
		if n <= 0 {
			n = 1
		}
		i += n
	}
}

// leadLen is the number of bytes of opening brackets and single quotes at the start of a word.
func leadLen(s string) int {
	n := 0
	for n < len(s) && strings.IndexByte("([{'", s[n]) >= 0 {
		n++
	}
	return n
}

func isSigil(c byte) bool { return strings.IndexByte("#@!~", c) >= 0 }

// token tries to read a token at word i and returns how many words it used (0 for none).
func (p *parser) token(i int) int {
	if p.quoted[i] {
		return 0
	}
	w := p.words[i]
	lead := leadLen(w.text)
	s := w.text[lead:]
	if s == "" {
		return 0
	}
	for _, r := range p.skipAt {
		if at := w.start + lead; at >= r.a && at < r.b {
			return 0
		}
	}
	if s[0] == '\\' && len(s) > 1 && isSigil(s[1]) {
		at := w.start + lead
		p.esc = append(p.esc, at)
		p.spans = append(p.spans, Span{Kind: KindEscape, Start: at, End: at + 2, Text: s[:2], Value: s[1:2]})
		return 1
	}
	switch s[0] {
	case '#':
		return p.tag(i, lead)
	case '@':
		return p.person(i, lead)
	case '!':
		return p.priority(i, lead)
	case '~':
		return p.duration(i, lead)
	}
	return p.keyword(i, lead)
}

// keyword reads the English tokens: by, remind, for, every, at, on and plain dates.
func (p *parser) keyword(i, lead int) int {
	w := p.words[i]
	s := w.text[lead:]
	core, tail := trimTrail(s)
	low := strings.ToLower(core)
	if tail == 0 {
		switch low {
		case "by":
			return p.dateWord(i, lead, i+1, KindDeadline)
		case "remind":
			j := i + 1
			if j < len(p.words) && strings.EqualFold(p.words[j].text, "me") {
				j++
			}
			return p.dateWord(i, lead, j, KindDue)
		case "for":
			return p.forDuration(i, lead)
		case "every":
			return p.recurrence(i)
		case "at", "on":
			if p.opts.Dates {
				if n := p.dateWord(i, lead, i+1, KindDue, true); n > 0 {
					return n
				}
			}
			if low == "at" && p.opts.Phrases {
				return p.location(i)
			}
			return 0
		}
	}
	if p.opts.Dates && p.plainDateOK(i, core, low) {
		return p.dateWord(i, lead, i, KindDue, true)
	}
	return 0
}

// dayAbbrev are short day names that are also names or words. As plain dates they must be typed in
// lower case, unless they start the text.
var dayAbbrev = map[string]bool{"mon": true, "tue": true, "tues": true, "wed": true, "weds": true,
	"thu": true, "thur": true, "thurs": true, "fri": true, "sat": true, "sun": true}

// ambiguousDay are day names that are also ordinary words (I sat down, the sun, Mon Mothma). They
// count as a date only when a date word comes with them: on sat, next sun, sat 3pm, or when the
// whole text is that one word.
var ambiguousDay = map[string]bool{"sat": true, "sun": true, "mon": true, "wed": true}

func (p *parser) plainDateOK(i int, orig, low string) bool {
	if p.skip[KindDue] {
		return false
	}
	if dayAbbrev[low] && i > 0 && orig != low {
		return false
	}
	if ambiguousDay[low] && len(p.words) > 1 {
		if _, n, _, ok := p.dateAt(i, true); !ok || n < 2 {
			return false
		}
	}
	return true
}

// dateWord reads a date expression that starts at word j, for the token that starts at word i. The
// token's span runs from word i to the end of the expression. strict is set for plain words,
// which accept fewer forms than the ones after by or remind.
func (p *parser) dateWord(i, lead, j int, kind Kind, strictArg ...bool) int {
	if p.skip[kind] || j >= len(p.words) {
		return 0
	}
	strict := len(strictArg) > 0 && strictArg[0]
	slot := &p.due
	if kind == KindDeadline {
		slot = &p.deadline
	}
	part, n, end, ok := p.dateAt(j, strict)
	if !ok {
		return 0
	}
	merged, ok := mergePartial(*slot, part)
	if !ok {
		return 0
	}
	*slot = &merged
	p.add(kind, p.words[i].start+lead, end, "")
	return (j - i) + n
}

// dateAt finds the longest date or time expression that starts at word j (five words at most).
// It returns the value, the number of words, and the byte offset where it ends.
func (p *parser) dateAt(j int, strict bool) (partial, int, int, bool) {
	avail := min(5, len(p.words)-j)
	for n := avail; n >= 1; n-- {
		words := make([]string, n)
		quotedAny := false
		for k := 0; k < n; k++ {
			if p.quoted[j+k] {
				quotedAny = true
			}
			t := p.words[j+k].text
			if k == 0 {
				t = t[leadLen(t):]
			}
			if k < n-1 {
				t = strings.TrimSuffix(t, ",")
			}
			words[k] = t
		}
		if quotedAny {
			continue
		}
		last, tail := trimTrail(words[n-1])
		if last == "" {
			continue
		}
		words[n-1] = last
		for k := range words {
			words[k] = strings.ToLower(words[k])
		}
		part, ok := parseWhen(words, p.now, strict)
		if !ok {
			continue
		}
		return part, n, p.words[j+n-1].end - tail, true
	}
	return partial{}, 0, 0, false
}

// forDuration reads "for 30m", "for 1h30m", "for 2 hours" and "for 45 min".
func (p *parser) forDuration(i, lead int) int {
	if p.skip[KindDuration] || p.res.DurationMin != 0 || i+1 >= len(p.words) || p.quoted[i+1] {
		return 0
	}
	first, tail1 := trimTrail(p.words[i+1].text)
	if first == "" || first[0] < '0' || first[0] > '9' {
		return 0
	}
	if min, ok := parseDuration(strings.ToLower(first)); ok {
		p.res.DurationMin = min
		p.add(KindDuration, p.words[i].start+lead, p.words[i+1].end-tail1, formatMinutes(min))
		return 2
	}
	if tail1 == 0 && i+2 < len(p.words) && !p.quoted[i+2] {
		second, tail2 := trimTrail(p.words[i+2].text)
		if min, ok := parseDuration(strings.ToLower(first + second)); ok {
			p.res.DurationMin = min
			p.add(KindDuration, p.words[i].start+lead, p.words[i+2].end-tail2, formatMinutes(min))
			return 3
		}
	}
	return 0
}

// add records a token: its span, and the range removed from the title.
func (p *parser) add(kind Kind, start, end int, value string) *Span {
	p.spans = append(p.spans, Span{Kind: kind, Start: start, End: end, Text: p.text[start:end], Value: value})
	p.drop = append(p.drop, rng{start, end})
	return &p.spans[len(p.spans)-1]
}

// trimTrail splits trailing sentence punctuation off s.
func trimTrail(s string) (core string, tail int) {
	end := len(s)
	for end > 0 && strings.IndexByte(",.;:!?)]}\"'", s[end-1]) >= 0 {
		end--
	}
	return s[:end], len(s) - end
}

func (p *parser) tag(i, lead int) int {
	if p.skip[KindTag] {
		return 0
	}
	w := p.words[i]
	body := w.text[lead+1:]
	j := 0
	for j < len(body) {
		r, size := utf8.DecodeRuneInString(body[j:])
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || unicode.Is(unicode.Mn, r)) {
			break
		}
		j += size
	}
	name, rest := body[:j], body[j:]
	if name == "" || len(name) > 64 {
		return 0
	}
	if r, _ := utf8.DecodeRuneInString(name); !unicode.IsLetter(r) {
		return 0
	}
	if strings.HasSuffix(name, "-") || strings.HasSuffix(name, "_") {
		return 0
	}
	if core, tail := trimTrail(rest); core != "" || tail != len(rest) {
		return 0
	}
	start := w.start + lead
	sp := p.add(KindTag, start, start+1+j, name)
	key := Fold(name)
	isNew := p.flagNew && !p.known[key]
	sp.New = isNew
	if p.tagSeen[key] {
		return 1
	}
	p.tagSeen[key] = true
	p.res.Tags = append(p.res.Tags, name)
	if isNew {
		p.res.NewTags = append(p.res.NewTags, name)
	}
	return 1
}

func (p *parser) duration(i, lead int) int {
	if p.skip[KindDuration] || p.res.DurationMin != 0 {
		return 0
	}
	w := p.words[i]
	core, tail := trimTrail(w.text[lead+1:])
	min, ok := parseDuration(strings.ToLower(core))
	if !ok {
		return 0
	}
	p.res.DurationMin = min
	p.add(KindDuration, w.start+lead, w.end-tail, formatMinutes(min))
	return 1
}

// priority reads !high, !top, !medium, !low, !none, !1 to !5, and the bare marks !! (medium) and !!! (high).
func (p *parser) priority(i, lead int) int {
	if p.skip[KindPriority] || p.res.Priority != "" {
		return 0
	}
	w := p.words[i]
	s := w.text[lead:]
	var pr string
	end := w.end
	if strings.Trim(s, "!") == "" {
		switch len(s) {
		case 2:
			pr = "medium"
		case 3:
			pr = "high"
		default:
			return 0
		}
	} else {
		core, tail := trimTrail(s[1:])
		v, ok := parsePriority(strings.ToLower(core))
		if !ok {
			return 0
		}
		pr, end = v, w.end-tail
	}
	p.res.Priority = pr
	p.add(KindPriority, w.start+lead, end, pr)
	return 1
}

func (p *parser) person(i, lead int) int {
	if p.skip[KindPerson] {
		return 0
	}
	w := p.words[i]
	s := w.text[lead:]
	rest := s[1:]
	if rest == "" {
		return 0
	}
	start := w.start + lead
	if rest[0] == '"' {
		close := strings.IndexByte(p.text[start+2:], '"')
		if close < 0 {
			return 0
		}
		name := strings.TrimSpace(p.text[start+2 : start+2+close])
		if name == "" || len(name) > 80 || strings.ContainsAny(name, "\r\n") {
			return 0
		}
		end := start + 2 + close + 1
		n := 0
		for k := i; k < len(p.words) && p.words[k].start < end; k++ {
			n++
		}
		p.addPerson(start, end, name)
		return n
	}
	core, tail := trimTrail(rest)
	if core == "" || len(core) > 64 || !validNameToken(core) {
		return 0
	}
	name := strings.ReplaceAll(core, "_", " ")
	end := w.end - tail
	used := 1
	if p.people != nil && tail == 0 {
		// Try a longer known full name: @Sam Lee.
		maxK := min(p.maxNames, len(p.words)-i)
		for k := maxK; k >= 2; k-- {
			parts := []string{name}
			ok := true
			var lastEnd int
			for m := 1; m < k; m++ {
				nw := p.words[i+m]
				c, t := trimTrail(nw.text)
				if c == "" || p.quoted[i+m] || !validNameToken(c) || (m < k-1 && t != 0) {
					ok = false
					break
				}
				parts = append(parts, strings.ReplaceAll(c, "_", " "))
				lastEnd = nw.end - t
			}
			if !ok {
				continue
			}
			cand := strings.Join(parts, " ")
			if p.people[Fold(cand)] {
				name, end, used = cand, lastEnd, k
				break
			}
		}
	}
	p.addPerson(start, end, name)
	return used
}

func (p *parser) addPerson(start, end int, name string) {
	p.add(KindPerson, start, end, name)
	key := Fold(name)
	if p.perSeen[key] {
		return
	}
	p.perSeen[key] = true
	p.res.People = append(p.res.People, name)
}

// validNameToken is true for a run of letters (with digits, apostrophes, hyphens and underscores
// after the first letter) that starts with a letter.
func validNameToken(s string) bool {
	first := true
	for _, r := range s {
		if first {
			if !unicode.IsLetter(r) {
				return false
			}
			first = false
			continue
		}
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mn, r) || r == '\'' || r == '’' || r == '-' || r == '_') {
			return false
		}
	}
	return !first
}

// location reads "at <place>": the words up to the next token, a comma, or five words.
func (p *parser) location(i int) int {
	if p.skip[KindLocation] || p.res.Location != "" {
		return 0
	}
	var parts []string
	n := 1
	end := p.words[i].end
	for k := i + 1; k < len(p.words) && len(parts) < 5; k++ {
		if p.quoted[k] || p.isTokenAt(k) {
			break
		}
		w := p.words[k]
		core, tail := trimTrail(w.text)
		if core == "" {
			break
		}
		if len(parts) == 0 {
			if _, ok := parseWhen([]string{strings.ToLower(core)}, p.now, false); ok {
				return 0
			}
			if r, _ := utf8.DecodeRuneInString(core); !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				return 0
			}
		}
		parts = append(parts, core)
		n++
		end = w.end - tail
		if tail > 0 && strings.ContainsAny(w.text[len(w.text)-tail:], ",;.:!?") {
			break
		}
	}
	if len(parts) == 0 {
		return 0
	}
	place := strings.Join(parts, " ")
	p.res.Location = place
	p.add(KindLocation, p.words[i].start, end, place)
	return n
}

// isTokenAt reports whether word k starts another token. It parses on a scratch copy so nothing is recorded.
func (p *parser) isTokenAt(k int) bool {
	scratch := newParser(p.text, p.opts)
	scratch.words, scratch.quoted = p.words, p.quoted
	scratch.opts.Phrases = false // "at" inside a place is part of the place
	return scratch.token(k) > 0
}

var recurrenceWords = map[string]bool{
	"day": true, "days": true, "weekday": true, "weekdays": true, "weekend": true, "weekends": true,
	"week": true, "weeks": true, "month": true, "months": true, "year": true, "years": true,
	"other": true, "and": true, "first": true, "second": true, "third": true, "fourth": true, "last": true,
	"mon": true, "monday": true, "tue": true, "tues": true, "tuesday": true, "wed": true, "weds": true, "wednesday": true,
	"thu": true, "thur": true, "thurs": true, "thursday": true, "fri": true, "friday": true,
	"sat": true, "saturday": true, "sun": true, "sunday": true,
}

// recurrence reads "every ...": every day, every weekday, every 2 weeks, every mon and wed, every 3m.
// The text is kept raw. When it names a keep-in-touch cadence (every 3m, every week) that is set too.
func (p *parser) recurrence(i int) int {
	if p.res.Recurrence != "" || p.res.Cadence != "" {
		return 0
	}
	parts := []string{"every"}
	n := 1
	end := p.words[i].end
	substantive := false
	for k := i + 1; k < len(p.words) && n < 8; k++ {
		if p.quoted[k] {
			break
		}
		w := p.words[k]
		core, tail := trimTrail(strings.ToLower(w.text))
		if core == "" {
			break
		}
		isNum := allDigits(core)
		_, compact := parseCompactUnit(core)
		if !recurrenceWords[core] && !isNum && !compact {
			break
		}
		if !isNum && core != "and" && core != "other" {
			substantive = true
		}
		parts = append(parts, core)
		n++
		end = w.end - tail
		if tail > 0 && w.text[len(w.text)-tail:] != "," {
			break
		}
	}
	if !substantive {
		return 0
	}
	raw := strings.Join(parts, " ")
	cad := cadenceOfEvery(parts[1:])
	start := p.words[i].start
	switch {
	case !p.skip[KindRecurrence]:
		p.res.Recurrence = raw
		if cad != "" && !p.skip[KindCadence] {
			p.res.Cadence = cad
		}
		p.add(KindRecurrence, start, end, raw)
	case cad != "" && !p.skip[KindCadence]:
		p.res.Cadence = cad
		p.add(KindCadence, start, end, cad)
	default:
		return 0
	}
	return n
}

// cadenceOfEvery maps the words after "every" to a keep-in-touch cadence, or "".
func cadenceOfEvery(words []string) string {
	joined := strings.Join(words, "")
	switch joined {
	case "day", "days", "weekday", "weekdays", "weekend", "weekends":
		return ""
	case "week":
		joined = "1week"
	case "month":
		joined = "1month"
	case "year":
		joined = "1year"
	case "otherweek":
		joined = "2weeks"
	}
	c, ok := parseCadence(joined)
	if !ok {
		return ""
	}
	return c
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// finish builds the title and the sorted spans.
func (p *parser) finish() Result {
	res := p.res
	if p.due != nil {
		w := p.due.finalize(p.now)
		res.Due = &w
	}
	if p.deadline != nil {
		w := p.deadline.finalize(p.now)
		res.Deadline = &w
	}
	for i := range p.spans {
		switch p.spans[i].Kind {
		case KindDue:
			p.spans[i].Value = res.Due.String()
		case KindDeadline:
			p.spans[i].Value = res.Deadline.String()
		}
	}
	sort.SliceStable(p.spans, func(a, b int) bool { return p.spans[a].Start < p.spans[b].Start })
	res.Spans = p.spans
	res.Title = p.title()
	return res
}

// title removes the dropped ranges (with the space before each) and the escape backslashes.
func (p *parser) title() string {
	if len(p.drop) == 0 && len(p.esc) == 0 {
		return strings.TrimSpace(p.text)
	}
	cut := make([]rng, 0, len(p.drop))
	for _, r := range p.drop {
		a, b := r.a, r.b
		j := a
		for j > 0 && (p.text[j-1] == ' ' || p.text[j-1] == '\t') {
			j--
		}
		if j == 0 {
			for b < len(p.text) && (p.text[b] == ' ' || p.text[b] == '\t') {
				b++
			}
		}
		cut = append(cut, rng{j, b})
	}
	for _, at := range p.esc {
		cut = append(cut, rng{at, at + 1})
	}
	sort.Slice(cut, func(a, b int) bool { return cut[a].a < cut[b].a })
	var b strings.Builder
	pos := 0
	for _, r := range cut {
		if r.a > pos {
			b.WriteString(p.text[pos:r.a])
		}
		if r.b > pos {
			pos = r.b
		}
	}
	if pos < len(p.text) {
		b.WriteString(p.text[pos:])
	}
	out := strings.TrimSpace(b.String())
	for _, empty := range []string{"()", "[]", "{}"} {
		out = strings.TrimSpace(strings.ReplaceAll(out, empty, ""))
	}
	return out
}

// Fold lowercases s and removes accents, for comparing names and tags.
func Fold(s string) string {
	return foldString(s)
}
