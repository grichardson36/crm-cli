package tui

import (
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/model"
	sg "github.com/jdanielnd/crm-cli/internal/suggest"
)

// The autocomplete source of a pipe line (patterns/autocomplete-component). The rows come from the field
// type of the segment under the cursor:
//
//	relation  objects of the allowed kinds, ranked with nicknames and typo tolerance (internal/suggest)
//	choice    its options
//	date      natural phrases and the dates tasks already use
//	tag       the existing tags
//	text      none
//
// A highlighted object of the add picker is the first row of the segment that takes its kind.

// pipeSession is the state of the pipe line being typed: its segments, the popup and caches.
type pipeSession struct {
	kind  string
	pl    *pipeLine
	ac    *Autocomplete
	cand  map[string][]overlayItem
	dates []Suggestion
	// datesRead says the recent dates were loaded.
	datesRead bool
	// idx and direct cache the live objects by id and the links read for the related segment (related.go).
	idx    map[string]map[int64]overlayItem
	direct map[string][]relCand
}

// pipeCompleter is the Completer of a pipe line.
type pipeCompleter struct {
	m *Model
	s *pipeSession
}

// pipeInput gives the input a pipe line is typed in, and the prompt kind that describes it.
func (m *Model) pipeInput() (*textinput.Model, string) {
	switch {
	case m.mode == modePrompt:
		return &m.input, m.promptKind
	case m.mode == modeEdit && m.creating && m.edit != nil && m.edit.typed == nil && m.edit.ta == nil:
		return &m.rowInput, personAddKind
	}
	return nil, ""
}

// pipeSessionFor returns the session of the pipe line in the input, opening one the first time.
func (m *Model) pipeSessionFor() (*pipeSession, *textinput.Model) {
	in, kind := m.pipeInput()
	if in == nil {
		m.pipe = nil
		return nil, nil
	}
	if m.pipe != nil && m.pipe.kind == kind {
		return m.pipe, in
	}
	pl := m.pipeFor(kind)
	if pl == nil {
		m.pipe = nil
		return nil, nil
	}
	s := &pipeSession{kind: kind, pl: pl, cand: map[string][]overlayItem{}}
	s.ac = NewAutocomplete(&pipeCompleter{m: m, s: s})
	s.ac.limit = relatedMaxRows
	m.pipe = s
	return s, in
}

// pipeRefresh brings the popup up to date with the text and the cursor.
func (m *Model) pipeRefresh() *pipeSession {
	s, in := m.pipeSessionFor()
	if s == nil {
		return nil
	}
	s.ac.Refresh(in.Value(), in.Position())
	return s
}

// pipeKey gives the pipe line the first look at a key of its input. handled says the key is done.
// up, down, tab and esc work on the popup only while it is open, and typing a pipe right after an
// accepted name drops the comma that was added for the next name.
func (m *Model) pipeKey(msg tea.KeyPressMsg) (handled bool) {
	s := m.pipeRefresh()
	if s == nil {
		return false
	}
	_, in := m.pipeSessionFor()
	if consumed, acc := s.ac.Key(keyName(msg)); consumed {
		if acc != nil {
			m.pipeAccept(s, in, *acc)
		}
		return true
	}
	if msg.Text == "|" {
		rs := []rune(in.Value())
		cur := clampInt(in.Position(), 0, len(rs))
		idx, _, _ := segBounds(rs, cur)
		if idx < len(s.pl.segs) && s.pl.segs[idx].Link {
			before := strings.TrimRight(string(rs[:cur]), " ")
			if strings.HasSuffix(before, ",") {
				before = strings.TrimRight(strings.TrimSuffix(before, ","), " ") + " "
				in.SetValue(before + "|" + string(rs[cur:]))
				in.SetCursor(len([]rune(before)) + 1)
				s.ac.sig = ""
				m.pipeRefresh()
				return true
			}
		}
	}
	return false
}

// pipeAccept puts an accepted row into the input and remembers which object the name stands for.
func (m *Model) pipeAccept(s *pipeSession, in *textinput.Model, row Suggestion) {
	rs := []rune(in.Value())
	idx, _, _ := segBounds(rs, in.Position())
	if idx < len(s.pl.segs) && s.pl.segs[idx].Link {
		seg := s.pl.segs[idx]
		switch {
		case row.Tag != "":
			m.pipeRes[resKey(seg.Kinds, row.Label)] = pendingLink{tag: row.Tag, label: "#" + row.Tag}
		case row.Ref.ID != 0:
			m.pipeRes[resKey(seg.Kinds, row.Label)] = pendingLink{ref: row.Ref, label: row.Label}
		}
	}
	s.ac.Accept(in, row)
	m.pipeRefresh()
}

// Suggest implements Completer.
func (c *pipeCompleter) Suggest(text string, cursor int) []Suggestion {
	m, s := c.m, c.s
	rs := []rune(text)
	idx, start, end := segBounds(rs, cursor)
	if idx >= len(s.pl.segs) {
		return nil
	}
	seg := s.pl.segs[idx]
	typed := strings.TrimSpace(string(rs[start:clampInt(cursor, start, end)]))
	var rows []Suggestion
	switch {
	case seg.Link:
		ts, _ := tokenBounds(rs, start, end, cursor)
		tok := strings.TrimSpace(string(rs[ts:clampInt(cursor, ts, end)]))
		rows = m.pipeLinkRows(s, seg, tok, text, idx)
		typed = strings.TrimPrefix(tok, "#")
	case seg.Spec.Type == model.FieldRelation && seg.Spec.Kind == tagKind:
		rows = m.pipeLinkRows(s, pipeSeg{Link: true, Kinds: []string{tagKind}}, typed, "", -1)
	case seg.Spec.Type == model.FieldChoice:
		for _, o := range seg.Spec.Choices {
			if seg.Spec.Clear != "" && o.Value == seg.Spec.Clear {
				continue
			}
			rows = append(rows, Suggestion{Label: o.Value, Desc: descIf(o.Label != o.Value, o.Label)})
		}
		rows = sg.Rank(rows, typed, func(r Suggestion) string { return r.Label }, sg.Options{Aliases: m.aliasTable(), Limit: acMaxRows})
	case seg.Spec.Type == model.FieldDate:
		rows = m.dateRows(s, typed)
	}
	// A row equal to the text typed has nothing left to complete.
	var out []Suggestion
	for _, r := range rows {
		if typed != "" && strings.EqualFold(r.Label, typed) && r.Ref.ID == 0 {
			continue
		}
		out = append(out, r)
	}
	return out
}

func descIf(ok bool, s string) string {
	if ok {
		return s
	}
	return ""
}

// pipeCandidates lists the objects of a kind once per session: every live object, or the tags.
func (m *Model) pipeCandidates(s *pipeSession, kind string) []overlayItem {
	if c, ok := s.cand[kind]; ok {
		return c
	}
	var out []overlayItem
	if kind == tagKind {
		for _, t := range m.quickEnv().Tags {
			out = append(out, overlayItem{id: "tag:" + t, label: t, value: t})
		}
	} else if o := objectByKind(kind); o != nil && o.items != nil {
		out = o.items(m)
	}
	s.cand[kind] = out
	return out
}

// dateOrder is the order of the phrases offered before anything is typed.
var dateOrder = []string{"today", "tomorrow", "fri", "next week", "next month", "+1w"}

// dateRows are the natural phrases and the dates tasks already use that fit the text typed.
func (m *Model) dateRows(s *pipeSession, typed string) []Suggestion {
	var all []Suggestion
	phrases := sg.DatePhrases
	if typed == "" {
		phrases = dateOrder
	}
	for _, p := range phrases {
		r := Suggestion{Label: p}
		if v, ok := m.parseDateField(p); ok {
			r.Desc = prettyDate(v)
		}
		all = append(all, r)
	}
	for _, d := range m.recentDates(s) {
		all = append(all, d)
	}
	if typed == "" {
		if len(all) > acMaxRows {
			all = all[:acMaxRows]
		}
		return all
	}
	return sg.Rank(all, typed, func(r Suggestion) string { return r.Label }, sg.Options{Limit: acMaxRows})
}

// recentDates are the due dates the open tasks use, soonest first, as rows.
func (m *Model) recentDates(s *pipeSession) []Suggestion {
	if s.datesRead {
		return s.dates
	}
	s.datesRead = true
	if m.tr == nil {
		return nil
	}
	tasks, err := m.tr.FindAll(m.ctx, model.TaskFilters{})
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var days []string
	for _, t := range tasks {
		if t.Completed || t.DueAt == nil || len(*t.DueAt) < 10 {
			continue
		}
		d := (*t.DueAt)[:10]
		if !seen[d] {
			seen[d] = true
			days = append(days, d)
		}
	}
	sort.Strings(days)
	if len(days) > 4 {
		days = days[:4]
	}
	for _, d := range days {
		s.dates = append(s.dates, Suggestion{Label: d, Desc: prettyDate(d)})
	}
	return s.dates
}

// Accept implements Completer: the row replaces the name under the cursor in a link segment, or the
// whole segment otherwise. After a name a comma and a space are added for the next one.
func (c *pipeCompleter) Accept(text string, cursor int, row Suggestion) (string, int) {
	rs := []rune(text)
	idx, start, end := segBounds(rs, cursor)
	label := []rune(row.Label)
	if idx < len(c.s.pl.segs) && c.s.pl.segs[idx].Link {
		ts, te := tokenBounds(rs, start, end, cursor)
		head := string(rs[:ts])
		if ts > 0 && rs[ts-1] == '|' {
			head += " " // a space after the pipe
		}
		if row.Tag != "" && len(c.s.pl.segs[idx].Kinds) > 1 {
			head += "#" // in a segment of several kinds a tag is written #name
		}
		head += string(label)
		tail := strings.TrimLeft(string(rs[te:]), " ")
		switch {
		case tail == "":
			return head + ", ", len([]rune(head)) + 2
		case strings.HasPrefix(tail, ","):
			rest := strings.TrimLeft(strings.TrimPrefix(tail, ","), " ")
			return head + ", " + rest, len([]rune(head)) + 2
		default: // a pipe follows: no comma
			return head + " " + tail, len([]rune(head))
		}
	}
	lead := ""
	for _, r := range rs[start:end] {
		if r != ' ' {
			break
		}
		lead += " "
	}
	if idx > 0 && lead == "" {
		lead = " "
	}
	head := string(rs[:start]) + lead + row.Label
	tail := string(rs[end:])
	if tail != "" {
		tail = " " + tail
	}
	return head + tail, len([]rune(head))
}
