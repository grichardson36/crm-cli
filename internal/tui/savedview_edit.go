package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/query"
	"github.com/jdanielnd/crm-cli/internal/validate"
)

// The saved view editor (Edit view). It is the record page of a saved view: one row per field, edited
// in place with the field editors of the framework (patterns/field-types-drive-editors). A text, a
// number and a date are typed in the row with the hint of their type, a choice is picked from the options
// in the row, a relation or a tag is picked from a typeahead. The conditions are rows of cells (field,
// operator, value), a adds one, d removes it, enter edits the cell under the cursor. Nothing is
// stored until ctrl+s, which checks the whole query with the one validation framework.

type svRowKind int

const (
	svrName svRowKind = iota
	svrKind
	svrTabs
	svrSort
	svrGroup
	svrCond
	svrColumns
	svrSearch
)

type svRow struct {
	kind svRowKind
	idx  int // the sort key or the condition
}

// svCellEdit is the editor open on one cell.
type svCellEdit struct {
	mode    string // text, choice, multi (a choice with several values), rel (a relation or a tag)
	spec    model.FieldSpec
	choices []model.FieldChoiceOption
	choice  int
	picked  []string
	// the relation typeahead
	all    []overlayItem
	shown  []overlayItem
	cur    int
	far    string
	isTag  bool
	finish func(vals []string) error // takes the picked values (multi, rel)
	commit func(text string) error   // takes the text (text) or the one chosen value (choice)
}

type svEditor struct {
	id      int64
	name    string
	kind    string
	tabs    bool
	q       query.Query
	dirty   bool
	cur     int
	col     int
	ed      *svCellEdit
	discard bool
	msg     string
	isErr   bool
	labels  map[string]string // relation uuid -> name
}

// openSVEditor opens the editor on a live saved view.
func (m *Model) openSVEditor(id int64) tea.Cmd {
	v, ok := m.savedByID(id)
	if !ok || v.Deleted {
		m.setStatus("that saved view is deleted, restore it first", true)
		return nil
	}
	m.sve = &svEditor{id: v.ID, name: v.Name, kind: v.Kind, tabs: v.ShowInTabs, q: v.Query.Clone(), labels: map[string]string{}}
	m.sve.loadLabels(m)
	return nil
}

func (e *svEditor) loadLabels(m *Model) {
	for _, c := range e.q.Filter {
		f, ok := query.FieldOf(e.kind, c.Field)
		if ok && f.Class == query.ClassRelation {
			for k, v := range m.svr.RelationLabels(m.ctx, f.FarTable(), c.Values) {
				e.labels[k] = v
			}
		}
	}
}

func (e *svEditor) rows() []svRow {
	out := []svRow{{kind: svrName}, {kind: svrKind}, {kind: svrTabs}, {kind: svrSort, idx: 0}, {kind: svrSort, idx: 1}, {kind: svrGroup}}
	for i := range e.q.Filter {
		out = append(out, svRow{kind: svrCond, idx: i})
	}
	return append(out, svRow{kind: svrColumns}, svRow{kind: svrSearch})
}

func (e *svEditor) cellCount(r svRow) int {
	switch r.kind {
	case svrSort, svrGroup:
		return 2
	case svrCond:
		return 3
	}
	return 1
}

func (e *svEditor) label(r svRow) string {
	switch r.kind {
	case svrName:
		return "name"
	case svrKind:
		return "kind"
	case svrTabs:
		return "in the tabs"
	case svrSort:
		return fmt.Sprintf("sort %d", r.idx+1)
	case svrGroup:
		return "group by"
	case svrCond:
		if r.idx == 0 {
			return "filter"
		}
		return "and"
	case svrColumns:
		return "columns"
	}
	return "search"
}

func (e *svEditor) fieldLabel(key string) string {
	if key == "" {
		return "-"
	}
	if f, ok := query.FieldOf(e.kind, key); ok {
		return f.Label
	}
	return key
}

// valueText is the value cell of a condition as shown.
func (e *svEditor) valueText(c query.Condition) string {
	if query.NeedsNoValue(c.Op) {
		return "-"
	}
	f, _ := query.FieldOf(e.kind, c.Field)
	vals := make([]string, len(c.Values))
	for i, v := range c.Values {
		vals[i] = v
		switch f.Class {
		case query.ClassRelation:
			if l, ok := e.labels[v]; ok {
				vals[i] = l
			} else if uuidLike(v) {
				vals[i] = "(removed)"
			}
		case query.ClassChoice:
			vals[i] = f.ChoiceLabelOf(v)
		}
	}
	if len(vals) == 0 {
		return "(value)"
	}
	sep := " or "
	if c.Op == query.OpBetween {
		sep = " and "
	}
	return strings.Join(vals, sep)
}

func uuidLike(s string) bool { return len(s) == 36 && strings.Count(s, "-") == 4 }

// cells returns the text of each cell of a row.
func (e *svEditor) cells(r svRow) []string {
	switch r.kind {
	case svrName:
		return []string{e.name}
	case svrKind:
		return []string{e.kind}
	case svrTabs:
		if e.tabs {
			return []string{"yes"}
		}
		return []string{"no"}
	case svrSort:
		if r.idx < len(e.q.Sort) {
			s := e.q.Sort[r.idx]
			dir := s.Dir
			if dir == "" {
				dir = "asc"
			}
			return []string{e.fieldLabel(s.Field), dir}
		}
		return []string{"-", "-"}
	case svrGroup:
		if e.q.GroupBy == "" {
			return []string{"-", "-"}
		}
		b := "-"
		if f, ok := query.FieldOf(e.kind, e.q.GroupBy); ok && f.Class == query.ClassDate && !f.Birthday {
			b = e.q.Bucket
			if b == "" {
				b = query.BucketDay
			}
		}
		return []string{e.fieldLabel(e.q.GroupBy), b}
	case svrCond:
		c := e.q.Filter[r.idx]
		return []string{e.fieldLabel(c.Field), query.OperatorLabel(c.Op), e.valueText(c)}
	case svrColumns:
		if len(e.q.Columns) == 0 {
			return []string{"-"}
		}
		return []string{strings.Join(e.q.Columns, ", ")}
	}
	if e.q.Search == "" {
		return []string{"-"}
	}
	return []string{e.q.Search}
}

// --- keys ---

func (m *Model) handleSVEKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	e := m.sve
	key := keyName(msg)
	if e.ed != nil {
		return m.sveEditKey(msg, key)
	}
	rows := e.rows()
	e.cur = clampInt(e.cur, 0, len(rows)-1)
	r := rows[e.cur]
	e.msg = ""
	switch key {
	case "esc":
		if e.dirty && !e.discard {
			e.discard = true
			e.msg, e.isErr = "unsaved changes: ctrl+s saves, esc again discards them", true
			return m, nil
		}
		m.sve = nil
		m.savedChanged()
		return m, nil
	case "ctrl+s":
		m.sveSave()
	case "up", "k":
		e.cur = max(0, e.cur-1)
	case "down", "j":
		e.cur = min(len(rows)-1, e.cur+1)
	case "right", "tab":
		if e.col < e.cellCount(r)-1 {
			e.col++
		} else if key == "tab" && e.cur < len(rows)-1 {
			e.cur++
			e.col = 0
		}
	case "left", "shift+tab":
		if e.col > 0 {
			e.col--
		}
	case "enter", "e":
		return m, m.sveBeginCell(r)
	case "a":
		m.sveAddCond(r)
		return m, m.sveBeginCell(e.rows()[e.cur])
	case "d":
		m.sveRemove(r)
	}
	e.discard = e.discard && key == "esc"
	rows = e.rows()
	e.cur = clampInt(e.cur, 0, len(rows)-1)
	e.col = clampInt(e.col, 0, e.cellCount(rows[e.cur])-1)
	return m, nil
}

func (m *Model) sveAddCond(r svRow) {
	e := m.sve
	fields := query.Fields(e.kind)
	if len(fields) == 0 {
		return
	}
	f := fields[0]
	c := query.Condition{Field: f.Key, Op: query.OperatorsFor(f.Class)[0]}
	at := len(e.q.Filter)
	if r.kind == svrCond {
		at = r.idx + 1
	}
	e.q.Filter = append(e.q.Filter[:at], append([]query.Condition{c}, e.q.Filter[at:]...)...)
	e.dirty = true
	// the cursor goes to the new row, on its field cell
	for i, row := range e.rows() {
		if row.kind == svrCond && row.idx == at {
			e.cur, e.col = i, 0
		}
	}
}

func (m *Model) sveRemove(r svRow) {
	e := m.sve
	switch r.kind {
	case svrCond:
		e.q.Filter = append(e.q.Filter[:r.idx], e.q.Filter[r.idx+1:]...)
	case svrSort:
		if r.idx < len(e.q.Sort) {
			e.q.Sort = append(e.q.Sort[:r.idx], e.q.Sort[r.idx+1:]...)
		}
	case svrGroup:
		e.q.GroupBy, e.q.Bucket = "", ""
	case svrColumns:
		e.q.Columns = nil
	case svrSearch:
		e.q.Search = ""
	default:
		e.msg, e.isErr = e.label(r)+" cannot be removed", true
		return
	}
	e.dirty = true
}

func (m *Model) sveSave() {
	e := m.sve
	q := e.q.Clone()
	v, err := m.hUpdateSaved(e.id, repo.SavedViewPatch{Name: &e.name, Kind: &e.kind, Query: &q, ShowInTabs: &e.tabs})
	if err != nil {
		e.msg, e.isErr = err.Error(), true
		return
	}
	e.q, e.dirty, e.discard = v.Query.Clone(), false, false
	e.loadLabels(m)
	e.msg, e.isErr = "saved (ctrl+z undoes)", false
	if m.sv != nil && m.sv.v.UUID == v.UUID {
		m.sv.filterOff, m.sv.sortKey, m.sv.groupKey = false, "", ""
	}
}

// --- editing a cell ---

func choiceOpts(vals ...string) []model.FieldChoiceOption {
	out := make([]model.FieldChoiceOption, len(vals))
	for i, v := range vals {
		out[i] = model.FieldChoiceOption{Value: v, Label: v}
	}
	return out
}

// sveBeginCell opens the editor of the cell under the cursor, by the type of its field.
func (m *Model) sveBeginCell(r svRow) tea.Cmd {
	e := m.sve
	if m.readOnly {
		e.msg, e.isErr = "read-only: editing is disabled", true
		return nil
	}
	text := func(spec model.FieldSpec, cur string, commit func(string) error) tea.Cmd {
		e.ed = &svCellEdit{mode: "text", spec: spec, commit: commit}
		m.rowInput.SetValue(cur)
		m.rowInput.CursorEnd()
		m.rowInput.SetWidth(max(10, m.width-24))
		return m.rowInput.Focus()
	}
	pick := func(opts []model.FieldChoiceOption, cur string, commit func(string) error) tea.Cmd {
		ce := &svCellEdit{mode: "choice", choices: opts, commit: commit}
		for i, o := range opts {
			if o.Value == cur {
				ce.choice = i
			}
		}
		e.ed = ce
		return nil
	}
	switch r.kind {
	case svrName:
		return text(model.FieldSpec{Key: "name", Label: "name", Type: model.FieldText, Required: true}, e.name, func(v string) error {
			n, err := query.ValidName(v)
			if err != nil {
				return err
			}
			e.name, e.dirty = n, true
			return nil
		})
	case svrKind:
		return pick(choiceOpts(query.Kinds...), e.kind, func(v string) error {
			if v != e.kind {
				e.changeKind(v)
			}
			return nil
		})
	case svrTabs:
		return pick(choiceOpts("yes", "no"), map[bool]string{true: "yes", false: "no"}[e.tabs], func(v string) error {
			e.tabs, e.dirty = v == "yes", true
			return nil
		})
	case svrSort:
		if e.col == 0 {
			opts := []model.FieldChoiceOption{{Value: "", Label: "- (none)"}}
			cur := ""
			if r.idx < len(e.q.Sort) {
				cur = e.q.Sort[r.idx].Field
			}
			for _, f := range query.Fields(e.kind) {
				if f.Sortable() {
					opts = append(opts, model.FieldChoiceOption{Value: f.Key, Label: f.Label})
				}
			}
			return pick(opts, cur, func(v string) error {
				e.setSort(r.idx, v, "")
				return nil
			})
		}
		if r.idx >= len(e.q.Sort) {
			e.msg, e.isErr = "pick the sort field first", true
			return nil
		}
		return pick(choiceOpts("asc", "desc"), strings.ToLower(e.q.Sort[r.idx].Dir + "asc")[:3], func(v string) error {
			e.q.Sort[r.idx].Dir, e.dirty = v, true
			return nil
		})
	case svrGroup:
		if e.col == 0 {
			opts := []model.FieldChoiceOption{{Value: "", Label: "- (none)"}}
			for _, f := range query.Fields(e.kind) {
				if f.Groupable() {
					opts = append(opts, model.FieldChoiceOption{Value: f.Key, Label: f.Label})
				}
			}
			return pick(opts, e.q.GroupBy, func(v string) error {
				e.q.GroupBy, e.q.Bucket, e.dirty = v, "", true
				return nil
			})
		}
		f, ok := query.FieldOf(e.kind, e.q.GroupBy)
		if !ok || f.Class != query.ClassDate || f.Birthday {
			e.msg, e.isErr = "only a date group has a size", true
			return nil
		}
		return pick(choiceOpts(query.BucketDay, query.BucketWeek, query.BucketMonth), e.q.Bucket, func(v string) error {
			e.q.Bucket, e.dirty = v, true
			return nil
		})
	case svrCond:
		return m.sveBeginCond(r)
	case svrColumns:
		return text(model.FieldSpec{Key: "columns", Label: "columns", Type: model.FieldText, Hint: "field keys separated by commas, for example company, email"}, strings.Join(e.q.Columns, ", "), func(v string) error {
			var cols []string
			for _, c := range strings.Split(v, ",") {
				if c = strings.TrimSpace(c); c != "" {
					cols = append(cols, c)
				}
			}
			e.q.Columns, e.dirty = cols, true
			return nil
		})
	}
	return text(model.FieldSpec{Key: "search", Label: "search", Type: model.FieldText, Hint: "words every row must contain"}, e.q.Search, func(v string) error {
		e.q.Search, e.dirty = strings.TrimSpace(v), true
		return nil
	})
}

func (e *svEditor) setSort(i int, field, dir string) {
	e.dirty = true
	if field == "" {
		if i < len(e.q.Sort) {
			e.q.Sort = append(e.q.Sort[:i], e.q.Sort[i+1:]...)
		}
		return
	}
	if i < len(e.q.Sort) {
		e.q.Sort[i].Field = field
		return
	}
	e.q.Sort = append(e.q.Sort, query.Sort{Field: field, Dir: dir})
}

// changeKind moves the view to another kind and drops what the new kind does not have.
func (e *svEditor) changeKind(kind string) {
	e.kind, e.dirty = kind, true
	has := func(k string) bool { _, ok := query.FieldOf(kind, k); return ok }
	var conds []query.Condition
	for _, c := range e.q.Filter {
		if has(c.Field) {
			conds = append(conds, c)
		}
	}
	dropped := len(e.q.Filter) - len(conds)
	e.q.Filter = conds
	var sorts []query.Sort
	for _, s := range e.q.Sort {
		if has(s.Field) {
			sorts = append(sorts, s)
		}
	}
	e.q.Sort = sorts
	if !has(e.q.GroupBy) {
		e.q.GroupBy, e.q.Bucket = "", ""
	}
	var cols []string
	for _, c := range e.q.Columns {
		if has(c) {
			cols = append(cols, c)
		}
	}
	e.q.Columns = cols
	if dropped > 0 {
		e.msg, e.isErr = fmt.Sprintf("%d condition(s) on fields %s does not have were removed", dropped, kind), false
	}
}

// sveBeginCond opens the field, operator or value cell of a condition.
func (m *Model) sveBeginCond(r svRow) tea.Cmd {
	e := m.sve
	c := &e.q.Filter[r.idx]
	f, _ := query.FieldOf(e.kind, c.Field)
	switch e.col {
	case 0:
		var opts []model.FieldChoiceOption
		for _, ff := range query.Fields(e.kind) {
			opts = append(opts, model.FieldChoiceOption{Value: ff.Key, Label: ff.Label})
		}
		e.ed = &svCellEdit{mode: "choice", choices: opts, commit: func(v string) error {
			nf, _ := query.FieldOf(e.kind, v)
			c.Field = v
			if nf.Class != f.Class {
				c.Op, c.Values = query.OperatorsFor(nf.Class)[0], nil
			} else if !query.NeedsNoValue(c.Op) {
				c.Values = nil
			}
			e.dirty = true
			return nil
		}}
		for i, o := range opts {
			if o.Value == c.Field {
				e.ed.choice = i
			}
		}
	case 1:
		var opts []model.FieldChoiceOption
		for _, op := range query.OperatorsFor(f.Class) {
			opts = append(opts, model.FieldChoiceOption{Value: op, Label: query.OperatorLabel(op)})
		}
		e.ed = &svCellEdit{mode: "choice", choices: opts, commit: func(v string) error {
			c.Op = v
			if query.NeedsNoValue(v) {
				c.Values = nil
			}
			e.dirty = true
			return nil
		}}
		for i, o := range opts {
			if o.Value == c.Op {
				e.ed.choice = i
			}
		}
	default:
		return m.sveBeginValue(c, f)
	}
	return nil
}

// sveBeginValue opens the value cell: the editor depends on the type of the field. A relation or a tag
// uses the typeahead, a choice lists its options (space adds a second one: the condition holds for any
// of them), anything else is typed with the hint of its type, several values separated by |.
func (m *Model) sveBeginValue(c *query.Condition, f query.Field) tea.Cmd {
	e := m.sve
	if query.NeedsNoValue(c.Op) {
		e.msg, e.isErr = "this operator takes no value", true
		return nil
	}
	set := func(vals []string) error {
		trial := *c
		trial.Values = vals
		if err := query.ValidateCondition(e.kind, trial, m.svr.Env(m.ctx, m.now())); err != nil {
			return err
		}
		c.Values, e.dirty = vals, true
		return nil
	}
	switch f.Class {
	case query.ClassRelation, query.ClassTag:
		ce := &svCellEdit{mode: "rel", far: f.FarTable(), isTag: f.Class == query.ClassTag, picked: append([]string(nil), c.Values...), finish: set}
		if ce.isTag {
			for _, n := range m.svr.TagNames(m.ctx) {
				ce.all = append(ce.all, overlayItem{label: n, value: n})
			}
		} else {
			cands, _ := m.svr.RelationCandidates(m.ctx, f.FarTable())
			for _, p := range cands {
				ce.all = append(ce.all, overlayItem{label: p[1], value: p[0]})
				e.labels[p[0]] = p[1]
			}
		}
		e.ed = ce
		m.rowInput.SetValue("")
		m.rowInput.SetWidth(max(10, m.width-24))
		m.svTAMatches()
		return m.rowInput.Focus()
	case query.ClassChoice:
		if len(f.Choices) > 0 && c.Op != query.OpIs && c.Op != query.OpIsNot && c.Op != query.OpIn {
			break
		}
		if len(f.Choices) > 0 {
			ce := &svCellEdit{mode: "multi", choices: f.Choices, picked: append([]string(nil), c.Values...), finish: set}
			if len(c.Values) > 0 {
				for i, o := range f.Choices {
					if strings.EqualFold(o.Value, c.Values[0]) {
						ce.choice = i
					}
				}
			}
			e.ed = ce
			return nil
		}
	}
	spec := model.FieldSpec{Key: f.Key, Label: f.Label, Type: f.Type, Hint: "several values separated by |"}
	switch f.Class {
	case query.ClassDate:
		if c.Op == query.OpWithinLast || c.Op == query.OpInNext {
			spec.Hint = "a number of days: 30, 30d or 4w"
		} else {
			spec.Hint = "2026-12-01, 90 days ago, in 30 days, today, next friday"
		}
	case query.ClassNumber:
		spec.Hint = "a number, $ and thousands separators are ignored"
		if c.Op == query.OpBetween {
			spec.Hint = "low | high"
		}
	}
	e.ed = &svCellEdit{mode: "text", spec: spec, commit: func(v string) error {
		var vals []string
		for _, p := range strings.Split(v, "|") {
			if p = strings.TrimSpace(p); p != "" {
				vals = append(vals, p)
			}
		}
		return set(vals)
	}}
	m.rowInput.SetValue(strings.Join(c.Values, " | "))
	m.rowInput.CursorEnd()
	m.rowInput.SetWidth(max(10, m.width-24))
	return m.rowInput.Focus()
}

// svTAMatches ranks the candidates of the relation typeahead for the typed text.
func (m *Model) svTAMatches() {
	ce := m.sve.ed
	ce.shown = taMatches(ce.all, strings.TrimSpace(m.rowInput.Value()), m.aliasTable())
	if ce.cur >= len(ce.shown) {
		ce.cur = max(0, len(ce.shown)-1)
	}
}

func (m *Model) sveEditKey(msg tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	e := m.sve
	ce := e.ed
	fail := func(err error) {
		e.msg, e.isErr = err.Error(), true
	}
	done := func() { e.ed = nil; m.rowInput.Blur() }
	switch ce.mode {
	case "text":
		switch key {
		case "esc":
			done()
		case "enter", "tab":
			if err := ce.commit(strings.TrimSpace(m.rowInput.Value())); err != nil {
				fail(err)
				return m, nil
			}
			done()
		default:
			var cmd tea.Cmd
			m.rowInput, cmd = m.rowInput.Update(msg)
			return m, cmd
		}
	case "choice", "multi":
		n := len(ce.choices)
		switch key {
		case "esc":
			done()
		case "up", "left", "shift+tab":
			ce.choice = (ce.choice + n - 1) % n
		case "down", "right", "tab":
			ce.choice = (ce.choice + 1) % n
		case " ":
			if ce.mode == "multi" {
				ce.togglePicked(ce.choices[ce.choice].Value)
			}
		case "enter":
			if ce.mode == "multi" {
				if len(ce.picked) == 0 {
					ce.picked = []string{ce.choices[ce.choice].Value}
				}
				if err := ce.finish(ce.picked); err != nil {
					fail(err)
					return m, nil
				}
			} else if err := ce.commit(ce.choices[ce.choice].Value); err != nil {
				fail(err)
				return m, nil
			}
			done()
		default:
			if utf8.RuneCountInString(msg.Text) == 1 {
				s := strings.ToLower(msg.Text)
				for i := 1; i <= n; i++ {
					o := ce.choices[(ce.choice+i)%n]
					if strings.HasPrefix(strings.ToLower(o.Label), s) {
						ce.choice = (ce.choice + i) % n
						break
					}
				}
			}
		}
	case "rel":
		switch key {
		case "esc":
			if err := ce.finish(ce.picked); err != nil {
				fail(err)
				return m, nil
			}
			done()
		case "up", "shift+tab":
			if len(ce.shown) > 0 {
				ce.cur = (ce.cur + len(ce.shown) - 1) % len(ce.shown)
			}
		case "down":
			if len(ce.shown) > 0 {
				ce.cur = (ce.cur + 1) % len(ce.shown)
			}
		case "tab":
			if ce.cur < len(ce.shown) {
				m.rowInput.SetValue(ce.shown[ce.cur].label)
				m.rowInput.CursorEnd()
				m.svTAMatches()
			}
		case "enter":
			text := strings.TrimSpace(m.rowInput.Value())
			switch {
			case text == "" && len(ce.picked) > 0:
				if err := ce.finish(ce.picked); err != nil {
					fail(err)
					return m, nil
				}
				done()
			case ce.cur < len(ce.shown):
				ce.addPicked(ce.shown[ce.cur].value)
				m.rowInput.SetValue("")
				m.svTAMatches()
			case ce.isTag && text != "":
				ce.addPicked(text) // a tag nobody has yet is still a valid condition
				m.rowInput.SetValue("")
				m.svTAMatches()
			default:
				fail(fmt.Errorf("nothing matches %q", text))
			}
		case "backspace":
			if m.rowInput.Value() == "" && len(ce.picked) > 0 {
				ce.picked = ce.picked[:len(ce.picked)-1]
				return m, nil
			}
			var cmd tea.Cmd
			m.rowInput, cmd = m.rowInput.Update(msg)
			m.svTAMatches()
			return m, cmd
		default:
			var cmd tea.Cmd
			m.rowInput, cmd = m.rowInput.Update(msg)
			m.svTAMatches()
			return m, cmd
		}
	}
	if e.ed == nil {
		e.msg = ""
	}
	return m, nil
}

func (ce *svCellEdit) togglePicked(v string) {
	for i, p := range ce.picked {
		if strings.EqualFold(p, v) {
			ce.picked = append(ce.picked[:i], ce.picked[i+1:]...)
			return
		}
	}
	ce.picked = append(ce.picked, v)
}

func (ce *svCellEdit) addPicked(v string) {
	for _, p := range ce.picked {
		if strings.EqualFold(p, v) {
			return
		}
	}
	ce.picked = append(ce.picked, v)
}

// --- drawing ---

func (m *Model) sveView() string {
	e := m.sve
	w := max(20, m.width)
	var lines []string
	title := "Edit view: " + e.name
	if e.dirty {
		title += "  (unsaved)"
	}
	lines = append(lines, m.st.Head.Render(ansi.Truncate(title, w, "…")))
	lines = append(lines, m.st.Dim.Render(ansi.Truncate(fmt.Sprintf("%s view, %s", e.kind, describeFor(e.kind, e.q)), w, "…")))
	lines = append(lines, "")
	for i, r := range e.rows() {
		lines = append(lines, m.sveRowLine(i, r, w)...)
	}
	help := "up/down row  left/right cell  enter edit  a add condition  d remove  ctrl+s save  esc close"
	if e.ed != nil {
		switch e.ed.mode {
		case "text":
			help = "enter saves the cell  esc cancels"
		case "choice":
			help = "up/down pick  enter saves  esc cancels"
		case "multi":
			help = "up/down move  space adds or removes  enter saves  esc cancels"
		default:
			help = "type to search  up/down  enter picks  tab completes  enter on empty saves  esc saves"
		}
	}
	for len(lines) < m.height-2 {
		lines = append(lines, "")
	}
	if len(lines) > m.height-2 {
		lines = lines[:m.height-2]
	}
	status := m.st.Dim.Render(e.msg)
	if e.isErr {
		status = m.st.Err.Render(ansi.Truncate(e.msg, w, "…"))
	}
	return strings.Join(lines, "\n") + "\n" + status + "\n" + m.st.Dim.Render(ansi.Truncate(help, w, "…"))
}

const sveLabelW = 14

func (m *Model) sveRowLine(i int, r svRow, w int) []string {
	e := m.sve
	label := fit(e.label(r), sveLabelW)
	cur := i == e.cur
	cells := e.cells(r)
	widths := map[svRowKind][]int{svrCond: {16, 16, 0}, svrSort: {20, 6}, svrGroup: {20, 8}}[r.kind]
	var b strings.Builder
	b.WriteString(m.st.Dim.Render(label))
	if cur && e.ed != nil {
		line, extra := m.sveEditLine(label, r)
		return append([]string{lipgloss.NewStyle().MaxWidth(w).Render(line)}, extra...)
	}
	for c, txt := range cells {
		cw := 0
		if c < len(widths) {
			cw = widths[c]
		}
		if cw == 0 {
			cw = max(10, w-sveLabelW-sumInts(widths)-2*len(cells))
		}
		t := fit(ansi.Truncate(txt, cw, "…"), cw)
		if cur && c == e.col {
			b.WriteString(m.st.Sel.Render(t))
		} else {
			b.WriteString(t)
		}
		b.WriteString("  ")
	}
	return []string{lipgloss.NewStyle().MaxWidth(w).Render(strings.TrimRight(b.String(), " "))}
}

func sumInts(v []int) int {
	t := 0
	for _, x := range v {
		t += x
	}
	return t
}

// sveEditLine draws the row while a cell is edited: the input with its hint, the options in the row, or the
// typeahead with its matches under the row.
func (m *Model) sveEditLine(label string, r svRow) (string, []string) {
	e := m.sve
	ce := e.ed
	switch ce.mode {
	case "text":
		line := m.st.Dim.Render(label) + strings.TrimRight(m.rowInput.View(), " ")
		h := ce.spec.DefaultHint()
		if t := strings.TrimSpace(m.rowInput.Value()); t != "" && (ce.spec.Type == model.FieldDate || ce.spec.Type == model.FieldNumber) {
			if res := validate.Field(ce.spec, t, validate.Options{ParseDate: m.parseDateField}); !res.OK() {
				h = "? " + ce.spec.DefaultHint()
			} else {
				h = ""
			}
		}
		return line + m.st.Dim.Render(" "+h), nil
	case "choice", "multi":
		var parts []string
		for i, c := range ce.choices {
			txt := c.Label
			if ce.mode == "multi" {
				mark := " "
				for _, p := range ce.picked {
					if strings.EqualFold(p, c.Value) {
						mark = "+"
					}
				}
				txt = mark + txt
			}
			if i == ce.choice {
				parts = append(parts, m.st.Sel.Render(" "+txt+" "))
			} else {
				parts = append(parts, m.st.Dim.Render(txt))
			}
		}
		return m.st.Dim.Render(label) + strings.Join(parts, " "), nil
	}
	var chips []string
	for _, p := range ce.picked {
		name := p
		if l, ok := e.labels[p]; ok {
			name = l
		}
		chips = append(chips, "["+name+"]")
	}
	line := m.st.Dim.Render(label) + m.st.Name.Render(strings.Join(chips, " ")) + " " + strings.TrimRight(m.rowInput.View(), " ")
	var extra []string
	for i, it := range ce.shown {
		l := "  " + strings.Repeat(" ", sveLabelW) + it.label
		if i == ce.cur {
			extra = append(extra, m.st.Sel.Render(l))
		} else {
			extra = append(extra, m.st.Dim.Render(l))
		}
	}
	if len(ce.shown) == 0 && strings.TrimSpace(m.rowInput.Value()) != "" {
		hint := "nothing matches"
		if ce.isTag {
			hint = "enter uses this tag name"
		}
		extra = append(extra, m.st.Dim.Render("  "+strings.Repeat(" ", sveLabelW)+hint))
	}
	return line, extra
}
