package tui

import (
	"fmt"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/mattn/go-runewidth"
)

type rowKind int

const (
	rkInfo rowKind = iota
	rkField
	rkEmail
	rkPhone
	rkHandle
	rkCadence
	rkTier
	rkTag
	rkTask
	rkNote
)

// tierNames maps priority_tier (index) to a name. Index 0 clears the tier.
var tierNames = []string{"clear", "Top", "High", "Medium", "Low", "None"}

const labelWidth = 16

// formRow is one selectable line (or block) in the detail pane.
type formRow struct {
	kind    rowKind
	field   string // rkField: name first last title company location birthday notes
	label   string
	value   string // raw value used for editing and copying
	suffix  string // dim text after the value
	id      int64  // task or interaction id
	key     string // handle kind
	lbl     *string
	primary bool
	empty   bool // placeholder for an empty section
	head    string
	typ     string // rkNote: the interaction type
}

type editState struct {
	row         int
	add         bool
	kind        rowKind
	field       string
	handleKind  string
	kindTouched bool
	oldValue    string
	origKind    string // handle kind when the edit started
}

type confirmState struct {
	prompt string
	yes    func()
}

func addable(k rowKind) bool {
	switch k {
	case rkEmail, rkPhone, rkHandle, rkTag, rkTask, rkNote:
		return true
	}
	return false
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func plainSuffix(l *string, primary bool) string {
	var parts []string
	if l != nil && *l != "" {
		parts = append(parts, *l)
	}
	if primary {
		parts = append(parts, "primary")
	}
	if len(parts) == 0 {
		return ""
	}
	return "  (" + strings.Join(parts, ", ") + ")"
}

func tierText(t *int) string {
	if t == nil || *t <= 0 || *t >= len(tierNames) {
		return ""
	}
	return fmt.Sprintf("%s (%d)", tierNames[*t], *t)
}

// buildForm reads the database and returns the rows for a person (and optionally a task).
func (m *Model) buildForm(pid, taskID int64) []formRow {
	var rows []formRow
	m.formTitle = ""
	if taskID != 0 {
		rows = append(rows, m.taskInfoRows(taskID)...)
	}
	if m.creating {
		m.formTitle = "New contact"
		return append(rows, m.personRows(nil)...)
	}
	if pid == 0 {
		return rows
	}
	p, err := m.pr.FindByID(m.ctx, pid)
	if err == nil {
		err = m.pr.LoadDetails(m.ctx, p)
	}
	if err != nil {
		m.setStatus(err.Error(), true)
		return rows
	}
	name := p.FirstName
	if p.LastName != nil {
		name += " " + *p.LastName
	}
	if p.IsFavorite {
		name = "★ " + name
	}
	m.formTitle = fmt.Sprintf("%s  #%d", name, p.ID)
	return append(rows, m.personRows(p)...)
}

func (m *Model) taskInfoRows(id int64) []formRow {
	t, err := m.tr.FindByID(m.ctx, id)
	if err != nil {
		return []formRow{{kind: rkInfo, label: "task", value: err.Error()}}
	}
	rows := []formRow{{kind: rkInfo, label: "task", value: t.Title, head: "Task"}}
	if t.DueAt != nil {
		rows = append(rows, formRow{kind: rkInfo, label: "due", value: *t.DueAt})
	}
	if t.Recurrence != nil {
		rows = append(rows, formRow{kind: rkInfo, label: "repeats", value: "every " + *t.Recurrence})
	}
	rows = append(rows, formRow{kind: rkInfo, label: "priority", value: t.Priority})
	if t.Description != nil && *t.Description != "" {
		rows = append(rows, formRow{kind: rkInfo, label: "description", value: *t.Description})
	}
	return rows
}

// personRows builds the form for p. A nil p gives the blank form used to create a contact.
func (m *Model) personRows(p *model.Person) []formRow {
	var rows []formRow
	add := func(r formRow) { rows = append(rows, r) }
	section := func(kind rowKind, label string) {
		add(formRow{kind: kind, label: label, empty: true})
	}
	field := func(f, label, v string) formRow { return formRow{kind: rkField, field: f, label: label, value: v} }

	if p == nil {
		add(field("name", "name", ""))
	} else {
		add(field("first", "first name", p.FirstName))
		add(field("last", "last name", str(p.LastName)))
	}
	var title, company, location, notes string
	if p != nil {
		title, company, location, notes = str(p.Title), str(p.Company), str(p.Location), str(p.Notes)
	}
	add(field("title", "title", title))
	add(field("company", "company", company))
	add(field("location", "location", location))
	bd := ""
	if p != nil {
		bd = formatBirthdayParts(p)
	}
	add(field("birthday", "birthday", bd))

	if p != nil && len(p.Emails) > 0 {
		for _, e := range p.Emails {
			add(formRow{kind: rkEmail, label: "email", value: e.Email, suffix: plainSuffix(e.Label, e.IsPrimary), lbl: e.Label, primary: e.IsPrimary})
		}
	} else {
		section(rkEmail, "email")
	}
	if p != nil && len(p.Phones) > 0 {
		for _, ph := range p.Phones {
			add(formRow{kind: rkPhone, label: "phone", value: ph.PhoneNumber, suffix: plainSuffix(ph.Label, ph.IsPrimary), lbl: ph.Label, primary: ph.IsPrimary})
		}
	} else {
		section(rkPhone, "phone")
	}
	if p != nil && len(p.Handles) > 0 {
		for _, h := range p.Handles {
			add(formRow{kind: rkHandle, label: h.Kind, key: h.Kind, value: h.Value})
		}
	} else {
		section(rkHandle, "link")
	}

	kit, tier := "", ""
	if p != nil {
		kit, tier = str(p.KeepInTouch), tierText(p.PriorityTier)
	}
	add(formRow{kind: rkCadence, label: "keep in touch", value: kit})
	add(formRow{kind: rkTier, label: "priority", value: tier})
	if p != nil {
		if p.LastContactedAt != nil {
			add(formRow{kind: rkInfo, label: "last contacted", value: *p.LastContactedAt})
		}
		if p.KeepInTouchSnoozedUntil != nil && *p.KeepInTouchSnoozedUntil > m.now().UTC().Format("2006-01-02 15:04:05") {
			add(formRow{kind: rkInfo, label: "snoozed until", value: *p.KeepInTouchSnoozedUntil})
		}
	}
	add(field("notes", "notes", notes))

	// Tags, tasks and recent notes only exist for a saved person.
	tagHead, taskHead, recentHead := "Tags", "Open tasks", "Recent"
	var pid int64
	if p != nil {
		pid = p.ID
	}
	var tags []*model.Tag
	if pid != 0 {
		tags, _ = m.tagr.GetForEntity(m.ctx, "person", pid)
	}
	if len(tags) > 0 {
		for i, t := range tags {
			r := formRow{kind: rkTag, label: "tag", value: t.Name}
			if i == 0 {
				r.head = tagHead
			}
			add(r)
		}
	} else {
		add(formRow{kind: rkTag, label: "tag", empty: true, head: tagHead})
	}
	var tasks []*model.Task
	if pid != 0 {
		tasks, _ = m.tr.FindAll(m.ctx, model.TaskFilters{PersonID: &pid})
	}
	if len(tasks) > 0 {
		for i, t := range tasks {
			r := formRow{kind: rkTask, label: "task", value: t.Title, id: t.ID}
			if t.DueAt != nil {
				r.suffix = "  due " + *t.DueAt
			}
			if i == 0 {
				r.head = taskHead
			}
			add(r)
		}
	} else {
		add(formRow{kind: rkTask, label: "task", empty: true, head: taskHead})
	}
	var ints []*model.Interaction
	if pid != 0 {
		ints, _ = m.ir.FindAll(m.ctx, model.InteractionFilters{PersonID: &pid, Limit: 8})
	}
	if len(ints) > 0 {
		for i, it := range ints {
			date := it.OccurredAt
			if len(date) > 10 {
				date = date[:10]
			}
			val := it.Type
			if it.Subject != nil && *it.Subject != "" {
				val += ": " + *it.Subject
			}
			r := formRow{kind: rkNote, label: date, key: val, value: str(it.Content), id: it.ID, typ: it.Type}
			if i == 0 {
				r.head = recentHead
			}
			add(r)
		}
	} else {
		add(formRow{kind: rkNote, label: "note", empty: true, head: recentHead})
	}
	return rows
}

func (m *Model) firstSelectable() int {
	for i, r := range m.form {
		if r.kind != rkInfo {
			return i
		}
	}
	return 0
}

// selectRow moves the form cursor to the first row of a kind with the given value.
func (m *Model) selectRow(kind rowKind, value string) {
	for i, r := range m.form {
		if r.kind == kind && (r.value == value || r.field == value) {
			m.fcur = i
			m.syncDetail()
			return
		}
	}
}

// refreshDetail rebuilds the detail form when the selection changed (or when forced).
func (m *Model) refreshDetail(force bool) {
	if m.creating {
		return
	}
	m.calTrack()
	r, ok := m.selected()
	key := fmt.Sprintf("%d/%d", r.personID, r.taskID)
	if m.view == viewMerge || m.view == viewSync || m.view == viewCalendar {
		key += fmt.Sprintf("/%d", m.cursor)
	}
	if !ok {
		key = ""
	}
	if !force && key == m.detailKey {
		return
	}
	changed := key != m.detailKey
	m.detailKey = key
	m.formPerson = r.personID
	if ok && m.view != viewArchived {
		m.form = m.buildForm(r.personID, r.taskID)
	} else {
		m.form, m.formTitle = nil, ""
	}
	if changed {
		m.fcur = m.firstSelectable()
		m.detail.GotoTop()
	}
	m.fcur = clampInt(m.fcur, 0, len(m.form)-1)
	if len(m.form) == 0 {
		m.focus = focusList
	}
	m.syncDetail()
}

// syncDetail renders the form into the detail viewport and keeps the cursor row in view.
func (m *Model) syncDetail() {
	w := m.detail.Width
	if len(m.form) == 0 {
		m.lineRow, m.rowStart, m.rowEnd = nil, nil, nil
		m.detail.SetContent(m.emptyDetail())
		return
	}
	var lines []string
	m.lineRow = m.lineRow[:0]
	m.rowStart = make([]int, len(m.form))
	m.rowEnd = make([]int, len(m.form))
	push := func(s string, row int) {
		lines = append(lines, s)
		m.lineRow = append(m.lineRow, row)
	}
	if m.formTitle != "" {
		title := runewidth.Truncate(m.formTitle, max(1, w), "…")
		push(lipgloss.NewStyle().MaxWidth(max(1, w)).Render(m.st.Name.Render(title)), -1)
		push("", -1)
	}
	for i, r := range m.form {
		if r.head != "" {
			if len(lines) > 0 && lines[len(lines)-1] != "" {
				push("", -1)
			}
			push(m.st.Head.Render(runewidth.Truncate(r.head, max(1, w), "…")), -1)
		}
		m.rowStart[i] = len(lines)
		for _, l := range m.renderRow(i, r, w) {
			push(l, i)
		}
		m.rowEnd[i] = len(lines) - 1
	}
	m.detail.SetContent(strings.Join(lines, "\n"))
	if m.focus == focusDetail && m.fcur < len(m.rowStart) {
		top, bottom := m.rowStart[m.fcur], m.rowEnd[m.fcur]
		if top < m.detail.YOffset {
			m.detail.SetYOffset(top)
		} else if bottom >= m.detail.YOffset+m.detail.Height {
			m.detail.SetYOffset(bottom - m.detail.Height + 1)
		}
	}
}

// renderRow returns the screen lines of one form row, each at most w cells wide.
func (m *Model) renderRow(i int, r formRow, w int) []string {
	if w < 1 {
		return []string{""}
	}
	selected := m.focus == focusDetail && i == m.fcur
	label := fit(r.label, labelWidth)
	if len(r.label) > labelWidth-1 {
		label = fit(runewidth.Truncate(r.label, labelWidth-1, "…"), labelWidth)
	}

	if m.mode == modeEdit && m.edit != nil && m.edit.row == i {
		line := m.st.Dim.Render(label) + m.rowInput.View()
		return []string{lipgloss.NewStyle().MaxWidth(w).Render(line)}
	}

	first := r.value
	valueStyled := false
	switch {
	case r.kind == rkField:
		first, _ = splitFirst(r.value)
	case r.kind == rkNote && !r.empty:
		first = r.key
	}
	if r.empty || (first == "" && r.kind != rkInfo) {
		first = "-"
		valueStyled = true
	}
	suffix := r.suffix
	vw := w - labelWidth - runewidth.StringWidth(suffix)
	if vw < 1 {
		vw, suffix = max(1, w-labelWidth), ""
	}
	first = runewidth.Truncate(first, vw, "…")

	var out []string
	plain := label + first + suffix
	switch {
	case selected:
		out = append(out, m.st.Sel.Render(fit(plain, w)))
	case valueStyled:
		out = append(out, m.st.Dim.Render(label+first))
	default:
		out = append(out, m.st.Dim.Render(label)+first+m.st.Dim.Render(suffix))
	}

	var more []string
	if (r.kind == rkField && r.field == "notes") || (r.kind == rkNote && !r.empty) {
		_, rest := splitFirst(r.value)
		if r.kind == rkNote {
			rest = strings.Split(strings.TrimRight(r.value, "\n"), "\n")
			if r.value == "" {
				rest = nil
			}
		}
		if len(rest) > 3 {
			rest = append(rest[:3], "…")
		}
		more = rest
	}
	for _, l := range more {
		out = append(out, m.st.Dim.Render(fit(strings.Repeat(" ", labelWidth)+runewidth.Truncate(l, max(1, w-labelWidth), "…"), w)))
	}
	return out
}

func splitFirst(s string) (string, []string) {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return lines[0], lines[1:]
}

// --- editing ---

func (m *Model) rowFocused() (formRow, bool) {
	if m.fcur < 0 || m.fcur >= len(m.form) {
		return formRow{}, false
	}
	return m.form[m.fcur], true
}

func isNameRow(r formRow) bool { return r.kind == rkField && r.field == "name" }

func (m *Model) needName() bool {
	if m.creating {
		m.setStatus("enter a name first", true)
		return true
	}
	return false
}

// startEdit handles Enter on the focused row.
func (m *Model) startEdit() tea.Cmd {
	r, ok := m.rowFocused()
	if !ok {
		return nil
	}
	if m.creating && !isNameRow(r) {
		m.setStatus("enter a name first", true)
		return nil
	}
	switch {
	case r.empty && addable(r.kind):
		return m.addRow(r.kind)
	case r.kind == rkInfo:
		m.setStatus("this row is read-only", true)
	case r.kind == rkNote:
		return m.beginEdit(m.fcur, false)
	case r.kind == rkCadence:
		m.pickCadence([]int64{m.formPerson})
	case r.kind == rkTier:
		m.pickTier([]int64{m.formPerson})
	default:
		return m.beginEdit(m.fcur, false)
	}
	return nil
}

// beginEdit opens the inline input on form row idx.
func (m *Model) beginEdit(idx int, add bool) tea.Cmd {
	r := m.form[idx]
	m.edit = &editState{row: idx, add: add, kind: r.kind, field: r.field, handleKind: r.key, oldValue: r.value}
	m.edit.origKind = r.key
	if r.kind == rkHandle && add {
		m.edit.handleKind = "website"
		m.edit.origKind = ""
	}
	v := r.value
	if add {
		v = ""
	}
	v = strings.ReplaceAll(v, "\n", "⏎")
	m.rowInput.SetValue(v)
	m.rowInput.CursorEnd()
	m.rowInput.Width = max(5, m.detail.Width-labelWidth-1)
	m.focus = focusDetail
	m.mode = modeEdit
	m.setStatus("", false)
	m.syncDetail()
	return m.rowInput.Focus()
}

// addRow starts adding a row of the given kind to the current person.
func (m *Model) addRow(k rowKind) tea.Cmd {
	if m.needName() {
		return nil
	}
	if m.curPerson() == 0 {
		m.setStatus("no person selected", true)
		return nil
	}
	if k == rkNote {
		// Notes use the same footer prompt as the n key.
		return m.startNotePrompt()
	}
	m.focus = focusDetail
	// Find the section: the last row of this kind. Replace a placeholder or insert after.
	last := -1
	for i, r := range m.form {
		if r.kind == k {
			last = i
		}
	}
	if last < 0 {
		return nil
	}
	pending := formRow{kind: k, label: m.form[last].label}
	if k == rkHandle {
		pending.label = "link"
	}
	if m.form[last].empty {
		pending.head = m.form[last].head
		m.form[last] = pending
	} else {
		m.form = append(m.form[:last+1], append([]formRow{pending}, m.form[last+1:]...)...)
		last++
	}
	m.fcur = last
	return m.beginEdit(last, true)
}

func (m *Model) cancelEdit() {
	creating := m.creating
	m.mode = modeNormal
	m.edit = nil
	m.rowInput.Blur()
	if creating {
		m.creating = false
		m.focus = focusList
		m.detailKey = "stale"
	}
	m.refreshDetail(true)
	m.setStatus("", false)
}

func (m *Model) handleEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.cancelEdit()
		return m, nil
	case "enter":
		m.commitEdit()
		return m, nil
	case "tab", "shift+tab":
		if m.edit != nil && m.edit.kind == rkHandle {
			m.cycleHandleKind(msg.String() == "tab")
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.rowInput, cmd = m.rowInput.Update(msg)
	m.syncDetail()
	return m, cmd
}

func (m *Model) cycleHandleKind(forward bool) {
	e := m.edit
	cur := 0
	for i, k := range model.HandleKinds {
		if k == e.handleKind {
			cur = i
		}
	}
	n := len(model.HandleKinds)
	if forward {
		cur = (cur + 1) % n
	} else {
		cur = (cur + n - 1) % n
	}
	e.handleKind, e.kindTouched = model.HandleKinds[cur], true
	m.form[e.row].label = e.handleKind
	m.syncDetail()
}

var birthdayShort = regexp.MustCompile(`^\d{1,2}-\d{1,2}$`)

func (m *Model) commitEdit() {
	e := m.edit
	if e == nil {
		return
	}
	v := strings.TrimSpace(strings.ReplaceAll(m.rowInput.Value(), "⏎", "\n"))
	if err := m.applyEdit(e, v); err != nil {
		m.setStatus(err.Error(), true)
		return // stay in the edit box so the value can be fixed
	}
}

// finishEdit leaves edit mode after a successful save and reloads.
func (m *Model) finishEdit() {
	m.mode = modeNormal
	m.edit = nil
	m.rowInput.Blur()
	m.afterWrite()
}

func (m *Model) applyEdit(e *editState, v string) error {
	ctx := m.ctx
	pid := m.formPerson
	m.beginGroup() // an edit that adds then removes a value undoes as one step
	defer m.endGroup("")
	if m.creating {
		return m.createPerson(v)
	}
	if v == "" && e.kind != rkField {
		m.cancelEdit()
		m.setStatus("nothing saved (use d to delete a row)", false)
		return nil
	}
	if v == e.oldValue && !e.add && !(e.kind == rkHandle && e.handleKind != e.origKind) {
		m.cancelEdit()
		return nil
	}
	switch e.kind {
	case rkField:
		in := model.UpdatePersonInput{}
		switch e.field {
		case "first":
			if v == "" {
				return fmt.Errorf("first name cannot be empty: %w", model.ErrValidation)
			}
			in.FirstName = &v
		case "last":
			in.LastName = &v
		case "title":
			in.Title = &v
		case "company":
			in.Company = &v
		case "location":
			in.Location = &v
		case "notes":
			in.Notes = &v
		case "birthday":
			if birthdayShort.MatchString(v) {
				v = "--" + v
			}
			in.Birthday = &v
		}
		if _, err := m.hUpdatePerson(ctx, pid, in); err != nil {
			return err
		}
		m.setStatus("saved "+e.field, false)
		m.finishEdit()
	case rkEmail:
		if e.add {
			if err := m.hAddEmail(ctx, pid, v, nil, false); err != nil {
				return err
			}
		} else {
			old := m.form[e.row]
			if err := m.hAddEmail(ctx, pid, v, old.lbl, old.primary); err != nil {
				return err
			}
			if err := m.hRemoveEmail(ctx, pid, e.oldValue); err != nil {
				return err
			}
		}
		m.setStatus("email saved", false)
		m.finishEdit()
		m.selectRow(rkEmail, v)
	case rkPhone:
		if e.add {
			if err := m.hAddPhone(ctx, pid, v, nil, false); err != nil {
				return err
			}
		} else {
			old := m.form[e.row]
			if err := m.hAddPhone(ctx, pid, v, old.lbl, old.primary); err != nil {
				return err
			}
			if err := m.hRemovePhone(ctx, pid, e.oldValue); err != nil {
				return err
			}
		}
		m.setStatus("phone saved", false)
		m.finishEdit()
		m.selectRow(rkPhone, v)
	case rkHandle:
		kind := e.handleKind
		if e.add && !e.kindTouched {
			kind = inferHandleKind(v)
		}
		if err := m.hSetHandle(ctx, pid, kind, v); err != nil {
			return err
		}
		if !e.add && kind != e.origKind {
			if err := m.hRemoveHandle(ctx, pid, e.origKind); err != nil {
				return err
			}
		}
		m.setStatus(kind+" saved", false)
		m.finishEdit()
		m.selectRow(rkHandle, v)
	case rkTag:
		name := strings.TrimLeft(v, "#")
		if e.add {
			if err := m.hTagApply(ctx, "person", pid, name); err != nil {
				return err
			}
		} else {
			if err := m.hTagApply(ctx, "person", pid, name); err != nil {
				return err
			}
			if name != e.oldValue {
				if err := m.hTagRemove(ctx, "person", pid, e.oldValue); err != nil {
					return err
				}
			}
		}
		m.setStatus("tag saved", false)
		m.finishEdit()
		m.selectRow(rkTag, name)
	case rkTask:
		title, due, hasDue := strings.Cut(v, "|")
		title, due = strings.TrimSpace(title), strings.TrimSpace(due)
		if title == "" {
			return fmt.Errorf("task title is empty: %w", model.ErrValidation)
		}
		if e.add {
			// New tasks read short syntax too: Call back @fri ^high ~30m +Name.
			env := m.quickEnvFor(pid)
			env.DueOverride = m.normalizeDue(due)
			if _, err := m.runLine("task "+title, env); err != nil {
				return err
			}
		} else {
			in := model.UpdateTaskInput{Title: &title}
			if hasDue {
				in.DueAt = &due
			}
			if _, err := m.hUpdateTask(ctx, m.form[e.row].id, in); err != nil {
				return err
			}
		}
		m.setStatus("task saved", false)
		m.finishEdit()
		m.selectRow(rkTask, title)
	case rkNote:
		id := m.form[e.row].id
		if _, err := m.hUpdateNote(ctx, id, model.UpdateInteractionInput{Content: &v}); err != nil {
			return err
		}
		m.setStatus("note saved", false)
		m.finishEdit()
		m.selectNote(id)
	default:
		m.cancelEdit()
	}
	return nil
}

// createPerson inserts the person on the first successful save of a name.
func (m *Model) createPerson(name string) error {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return fmt.Errorf("a name is required: %w", model.ErrValidation)
	}
	in := model.CreatePersonInput{FirstName: strings.Join(parts[:len(parts)-1], " ")}
	if len(parts) == 1 {
		in.FirstName = parts[0]
	} else {
		last := parts[len(parts)-1]
		in.LastName = &last
	}
	p, err := m.hCreatePerson(m.ctx, in)
	if err != nil {
		return err
	}
	m.creating = false
	m.mode = modeNormal
	m.edit = nil
	m.rowInput.Blur()
	// Show the new person: People view, no filter.
	m.view, m.query = viewPeople, ""
	m.marked = map[int64]bool{}
	m.people = nil
	m.report(m.reload())
	for i, r := range m.rows {
		if r.personID == p.ID {
			m.cursor = i
			break
		}
	}
	m.clamp()
	m.detailKey = "stale"
	m.refreshDetail(true)
	m.focus = focusDetail
	m.selectRow(rkField, "title")
	m.setStatus("contact created, fill in the rest", false)
	return nil
}

// startCreate opens a blank form with the name row in edit mode.
func (m *Model) startCreate() tea.Cmd {
	m.creating = true
	m.focus = focusDetail
	m.form = m.buildForm(0, 0)
	m.formPerson = 0
	m.fcur = 0
	m.syncDetail()
	return m.beginEdit(0, false)
}

// --- delete, copy, open ---

func (m *Model) askDelete() {
	r, ok := m.rowFocused()
	if !ok {
		return
	}
	if m.needName() {
		return
	}
	if r.empty {
		m.setStatus("nothing to delete here", true)
		return
	}
	pid := m.formPerson
	var what string
	var do func() error
	switch r.kind {
	case rkEmail:
		what, do = "email "+r.value, func() error { return m.hRemoveEmail(m.ctx, pid, r.value) }
	case rkPhone:
		what, do = "phone "+r.value, func() error { return m.hRemovePhone(m.ctx, pid, r.value) }
	case rkHandle:
		what, do = r.key+" link", func() error { return m.hRemoveHandle(m.ctx, pid, r.key) }
	case rkTag:
		what, do = "tag "+r.value, func() error { return m.hTagRemove(m.ctx, "person", pid, r.value) }
	case rkTask:
		what, do = "task "+r.value, func() error { return m.hArchiveTask(m.ctx, r.id) }
	case rkNote:
		m.askDeleteNote(r)
		return
	case rkInfo:
		m.setStatus("this row is read-only", true)
		return
	case rkCadence:
		what, do = "keep in touch", func() error {
			e := ""
			_, err := m.hUpdatePerson(m.ctx, pid, model.UpdatePersonInput{KeepInTouch: &e})
			return err
		}
	case rkTier:
		what, do = "priority", func() error {
			z := 0
			_, err := m.hUpdatePerson(m.ctx, pid, model.UpdatePersonInput{PriorityTier: &z})
			return err
		}
	case rkField:
		if r.field == "first" {
			m.setStatus("first name cannot be cleared", true)
			return
		}
		what, do = "clear "+r.label, func() error {
			e := ""
			in := model.UpdatePersonInput{}
			switch r.field {
			case "last":
				in.LastName = &e
			case "title":
				in.Title = &e
			case "company":
				in.Company = &e
			case "location":
				in.Location = &e
			case "notes":
				in.Notes = &e
			case "birthday":
				in.Birthday = &e
			}
			_, err := m.hUpdatePerson(m.ctx, pid, in)
			return err
		}
	}
	if do == nil {
		return
	}
	m.mode = modeConfirm
	m.confirm = &confirmState{
		prompt: fmt.Sprintf("delete %s? y/n", what),
		yes: func() {
			if err := do(); err != nil {
				m.setStatus(err.Error(), true)
				return
			}
			m.setStatus("deleted "+what, false)
			m.afterWrite()
		},
	}
}

func (m *Model) copyRow() {
	r, ok := m.rowFocused()
	if !ok {
		return
	}
	v := r.value
	if r.kind == rkNote && strings.TrimSpace(v) == "" {
		v = r.key
	}
	if strings.TrimSpace(v) == "" {
		m.setStatus("nothing to copy", true)
		return
	}
	if m.clip != nil {
		m.clip(v)
	}
	short := runewidth.Truncate(strings.ReplaceAll(v, "\n", " "), 40, "…")
	m.setStatus("copied: "+short, false)
}

var handleHosts = map[string]string{
	"linkedin": "https://www.linkedin.com/in/", "twitter": "https://x.com/", "facebook": "https://www.facebook.com/",
	"instagram": "https://www.instagram.com/", "telegram": "https://t.me/", "tiktok": "https://www.tiktok.com/@",
	"youtube": "https://www.youtube.com/@", "whatsapp": "https://wa.me/",
}

// linkFor builds an openable URL for a row, or "".
func linkFor(r formRow) string {
	v := strings.TrimSpace(r.value)
	if v == "" {
		return ""
	}
	switch r.kind {
	case rkEmail:
		return "mailto:" + v
	case rkPhone:
		return "tel:" + v
	case rkHandle:
		if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
			return v
		}
		if base, ok := handleHosts[r.key]; ok {
			return base + strings.TrimPrefix(v, "@")
		}
		return "https://" + v
	}
	return ""
}

func (m *Model) openRow() {
	r, ok := m.rowFocused()
	if !ok {
		return
	}
	u := linkFor(r)
	if u == "" {
		m.setStatus("nothing to open on this row", true)
		return
	}
	if m.open == nil {
		return
	}
	if err := m.open(u); err != nil {
		m.setStatus("open failed: "+err.Error(), true)
		return
	}
	m.setStatus("opened "+runewidth.Truncate(u, 50, "…"), false)
}

// inferHandleKind guesses the handle kind from a URL or username.
func inferHandleKind(v string) string {
	l := strings.ToLower(v)
	for _, p := range []struct{ sub, kind string }{
		{"linkedin.", "linkedin"}, {"twitter.", "twitter"}, {"x.com", "twitter"}, {"facebook.", "facebook"}, {"fb.com", "facebook"},
		{"instagram.", "instagram"}, {"t.me", "telegram"}, {"telegram.", "telegram"}, {"wa.me", "whatsapp"}, {"whatsapp.", "whatsapp"},
		{"tiktok.", "tiktok"}, {"youtube.", "youtube"}, {"youtu.be", "youtube"},
	} {
		if strings.Contains(l, p.sub) {
			return p.kind
		}
	}
	if strings.HasPrefix(l, "http") || strings.HasPrefix(l, "www.") || strings.Contains(l, ".") {
		return "website"
	}
	return "other"
}
