// Package tui is a keyboard-first terminal browser for the CRM. The mouse works too.
package tui

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

type viewKind int

const (
	viewPeople viewKind = iota
	viewDue
	viewBirthdays
	viewTasks
	numViews
)

var viewNames = [numViews]string{"People", "Due", "Birthdays", "Tasks"}

type mode int

const (
	modeNormal mode = iota
	modeSearch
	modePrompt
)

// row is one line in the list pane.
type row struct {
	personID int64
	taskID   int64
	title    string
	sub      string
	meta     string
}

// Model is the Bubble Tea model.
type Model struct {
	ctx  context.Context
	pr   *repo.PersonRepo
	ir   *repo.InteractionRepo
	tr   *repo.TaskRepo
	tagr *repo.TagRepo
	now  func() time.Time

	width, height int
	view          viewKind
	mode          mode
	help          bool

	people      []model.PersonSummary
	rows        []row
	cursor      int
	offset      int
	query       string
	sortRecent  bool
	detail      viewport.Model
	detailKey   string
	input       textinput.Model
	promptKind  string
	status      string
	statusIsErr bool
	readOnly    bool
}

// New creates the model and loads data.
func New(ctx context.Context, db *sql.DB) (*Model, error) {
	in := textinput.New()
	in.CharLimit = 500
	m := &Model{
		ctx:    ctx,
		pr:     repo.NewPersonRepo(db),
		ir:     repo.NewInteractionRepo(db),
		tr:     repo.NewTaskRepo(db),
		tagr:   repo.NewTagRepo(db),
		now:    time.Now,
		input:  in,
		detail: viewport.New(0, 0),
	}
	if err := m.reload(); err != nil {
		return nil, err
	}
	return m, nil
}

// Run starts the TUI.
func Run(ctx context.Context, db *sql.DB, readOnly bool) error {
	m, err := New(ctx, db)
	if err != nil {
		return err
	}
	m.readOnly = readOnly
	_, err = tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithContext(ctx)).Run()
	return err
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) setStatus(msg string, isErr bool) {
	m.status, m.statusIsErr = msg, isErr
}

// reload re-reads everything from the database and rebuilds the current view.
func (m *Model) reload() error {
	people, err := m.pr.ListSummaries(m.ctx)
	if err != nil {
		return err
	}
	m.people = people
	return m.rebuild(true)
}

// rebuild recomputes rows for the current view. With keep it leaves the cursor on the same item,
// or at the same position if that item is gone. Otherwise the cursor goes to the top.
func (m *Model) rebuild(keep bool) error {
	var keepPerson, keepTask int64
	oldCursor := m.cursor
	if keep && m.cursor < len(m.rows) {
		keepPerson, keepTask = m.rows[m.cursor].personID, m.rows[m.cursor].taskID
	}

	var rows []row
	switch m.view {
	case viewPeople:
		rows = m.peopleRows()
	case viewDue:
		due, err := m.pr.Due(m.ctx, 0, 1000)
		if err != nil {
			return err
		}
		for _, d := range due {
			rows = append(rows, row{personID: d.PersonID, title: d.Name, sub: d.KeepInTouch, meta: fmt.Sprintf("%dd overdue", d.DaysOverdue)})
		}
	case viewBirthdays:
		list, err := m.pr.Birthdays(m.ctx, 90, m.now())
		if err != nil {
			return err
		}
		for _, b := range list {
			meta := fmt.Sprintf("in %dd", b.DaysUntil)
			if b.DaysUntil == 0 {
				meta = "today"
			}
			sub := b.Date
			if b.Turning != nil {
				sub += fmt.Sprintf(" (turns %d)", *b.Turning)
			}
			rows = append(rows, row{personID: b.PersonID, title: b.Name, sub: sub, meta: meta})
		}
	case viewTasks:
		tasks, err := m.tr.FindAll(m.ctx, model.TaskFilters{})
		if err != nil {
			return err
		}
		for _, t := range tasks {
			r := row{taskID: t.ID, title: t.Title}
			if len(t.PersonIDs) > 0 {
				r.personID = t.PersonIDs[0]
			} else if t.PersonID != nil {
				r.personID = *t.PersonID
			}
			if t.DueAt != nil {
				r.meta = *t.DueAt
			}
			if t.Recurrence != nil {
				r.sub = "repeats " + *t.Recurrence
			}
			rows = append(rows, r)
		}
	}
	m.rows = rows

	m.cursor = 0
	if keep {
		m.cursor = oldCursor
		for i, r := range rows {
			if r.personID == keepPerson && r.taskID == keepTask && (keepPerson != 0 || keepTask != 0) {
				m.cursor = i
				break
			}
		}
	} else {
		m.offset = 0
	}
	m.clamp()
	m.refreshDetail(true)
	return nil
}

func (m *Model) peopleRows() []row {
	people := m.people
	if m.sortRecent {
		sorted := append([]model.PersonSummary(nil), people...)
		sortByUpdated(sorted)
		people = sorted
	}
	terms := strings.Fields(strings.ToLower(m.query))
	var rows []row
	for _, p := range people {
		if !matches(p, terms) {
			continue
		}
		name := p.Name
		if p.IsFavorite {
			name = "★ " + name
		}
		sub := p.Company
		if sub == "" {
			sub = p.Title
		}
		rows = append(rows, row{personID: p.ID, title: name, sub: sub, meta: p.KeepInTouch})
	}
	return rows
}

func matches(p model.PersonSummary, terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	hay := strings.ToLower(strings.Join([]string{p.Name, p.Company, p.Title, p.Email, p.AllEmails, p.Location}, " "))
	tags := strings.ToLower(p.Tags)
	for _, t := range terms {
		if strings.HasPrefix(t, "#") {
			if len(t) > 1 && !strings.Contains(tags, t[1:]) {
				return false
			}
			continue
		}
		if !strings.Contains(hay, t) && !strings.Contains(tags, t) {
			return false
		}
	}
	return true
}

func sortByUpdated(p []model.PersonSummary) {
	for i := 1; i < len(p); i++ {
		for j := i; j > 0 && p[j].UpdatedAt > p[j-1].UpdatedAt; j-- {
			p[j], p[j-1] = p[j-1], p[j]
		}
	}
}

func (m *Model) bodyHeight() int {
	h := m.height - 3
	if h < 3 {
		h = 3
	}
	return h
}

func (m *Model) listWidth() int {
	w := m.width * 2 / 5
	if w < 30 {
		w = 30
	}
	if w > m.width-20 {
		w = m.width - 20
	}
	if w < 10 {
		w = 10
	}
	return w
}

func (m *Model) clamp() {
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	h := m.bodyHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *Model) selected() (row, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return row{}, false
	}
	return m.rows[m.cursor], true
}

func (m *Model) move(delta int) {
	m.cursor += delta
	m.clamp()
	m.refreshDetail(false)
}

// refreshDetail rebuilds the detail pane when the selection changed (or when forced).
func (m *Model) refreshDetail(force bool) {
	r, ok := m.selected()
	key := fmt.Sprintf("%d/%d", r.personID, r.taskID)
	if !ok {
		key = ""
	}
	if !force && key == m.detailKey {
		return
	}
	m.detailKey = key
	m.detail.SetContent(m.detailText(r, ok))
	m.detail.GotoTop()
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.detail.Width = m.width - m.listWidth() - 3
		m.detail.Height = m.bodyHeight()
		m.clamp()
		m.refreshDetail(true)
		return m, nil
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.KeyMsg:
		if m.help {
			m.help = false
			return m, nil
		}
		switch m.mode {
		case modeSearch:
			return m.handleSearchKey(msg)
		case modePrompt:
			return m.handlePromptKey(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.mode != modeNormal || m.help {
		return m, nil
	}
	inList := msg.X < m.listWidth()
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if inList {
			m.move(-3)
		} else {
			m.detail.LineUp(3)
		}
	case tea.MouseButtonWheelDown:
		if inList {
			m.move(3)
		} else {
			m.detail.LineDown(3)
		}
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		if msg.Y == 0 {
			// Click on the tab bar.
			x := 0
			for i, name := range viewNames {
				w := len(tabLabel(i, name)) + 2
				if msg.X >= x && msg.X < x+w {
					m.switchView(viewKind(i))
					break
				}
				x += w
			}
			return m, nil
		}
		if inList {
			idx := m.offset + msg.Y - 1
			if idx >= 0 && idx < len(m.rows) && msg.Y-1 < m.bodyHeight() {
				m.cursor = idx
				m.clamp()
				m.refreshDetail(false)
			}
		}
	}
	return m, nil
}

func (m *Model) switchView(v viewKind) {
	if v == m.view {
		return
	}
	m.view = v
	m.cursor, m.offset = 0, 0
	m.setStatus("", false)
	if err := m.rebuild(false); err != nil {
		m.setStatus(err.Error(), true)
	}
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	half := m.bodyHeight() / 2
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.help = true
	case "j", "down", "ctrl+n":
		m.move(1)
	case "k", "up", "ctrl+p":
		m.move(-1)
	case "ctrl+d", "pgdown":
		m.move(half)
	case "ctrl+u", "pgup":
		m.move(-half)
	case "g", "home":
		m.cursor = 0
		m.clamp()
		m.refreshDetail(false)
	case "G", "end":
		m.cursor = len(m.rows) - 1
		m.clamp()
		m.refreshDetail(false)
	case "J":
		m.detail.LineDown(3)
	case "K":
		m.detail.LineUp(3)
	case "1", "2", "3", "4":
		n, _ := strconv.Atoi(msg.String())
		m.switchView(viewKind(n - 1))
	case "tab":
		m.switchView((m.view + 1) % numViews)
	case "shift+tab":
		m.switchView((m.view + numViews - 1) % numViews)
	case "/":
		if m.view != viewPeople {
			m.switchView(viewPeople)
		}
		m.mode = modeSearch
		m.input.Prompt = "/"
		m.input.SetValue(m.query)
		m.input.CursorEnd()
		return m, m.input.Focus()
	case "esc":
		if m.query != "" {
			m.query = ""
			m.cursor, m.offset = 0, 0
			m.report(m.rebuild(false))
		}
	case "o":
		if m.view == viewPeople {
			m.sortRecent = !m.sortRecent
			m.cursor, m.offset = 0, 0
			m.report(m.rebuild(false))
			if m.sortRecent {
				m.setStatus("sorted by recently updated", false)
			} else {
				m.setStatus("sorted by name", false)
			}
		}
	case "r":
		m.report(m.reload())
		m.setStatus("reloaded", false)
	case "c", "f", "x", "s", "n", "t":
		if m.readOnly {
			m.setStatus("read-only: this view cannot change data", true)
			return m, nil
		}
		return m.handleWriteKey(msg.String())
	}
	return m, nil
}

func (m *Model) handleWriteKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "c":
		m.markContacted()
	case "f":
		m.toggleFavorite()
	case "x":
		m.completeTask()
	case "s":
		m.startPrompt("snooze", "snooze days: ", "7")
		return m, m.input.Focus()
	case "n":
		m.startPrompt("note", "note: ", "")
		return m, m.input.Focus()
	case "t":
		m.startPrompt("task", "task (add a due date after a | e.g. call back | 2026-12-01): ", "")
		return m, m.input.Focus()
	}
	return m, nil
}

func (m *Model) report(err error) {
	if err != nil {
		m.setStatus(err.Error(), true)
	}
}

func (m *Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.mode = modeNormal
		m.input.Blur()
		return m, nil
	case "esc":
		m.mode = modeNormal
		m.input.Blur()
		m.query = ""
		m.cursor, m.offset = 0, 0
		m.report(m.rebuild(false))
		return m, nil
	case "down", "ctrl+n":
		m.move(1)
		return m, nil
	case "up", "ctrl+p":
		m.move(-1)
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if v := m.input.Value(); v != m.query {
		m.query = v
		m.cursor, m.offset = 0, 0
		m.report(m.rebuild(false))
	}
	return m, cmd
}

func (m *Model) startPrompt(kind, prompt, initial string) {
	if _, ok := m.selected(); !ok {
		return
	}
	m.mode = modePrompt
	m.promptKind = kind
	m.input.Prompt = prompt
	m.input.SetValue(initial)
	m.input.CursorEnd()
}

func (m *Model) handlePromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.input.Blur()
		return m, nil
	case "enter":
		value := strings.TrimSpace(m.input.Value())
		kind := m.promptKind
		m.mode = modeNormal
		m.input.Blur()
		if value != "" {
			m.runPrompt(kind, value)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) runPrompt(kind, value string) {
	r, ok := m.selected()
	if !ok || r.personID == 0 {
		m.setStatus("no person selected", true)
		return
	}
	switch kind {
	case "snooze":
		days, err := strconv.Atoi(value)
		if err != nil || days < 0 {
			m.setStatus("enter a number of days", true)
			return
		}
		until := m.now().UTC().AddDate(0, 0, days).Format("2006-01-02 15:04:05")
		if _, err := m.pr.Update(m.ctx, r.personID, model.UpdatePersonInput{SnoozedUntil: &until}); err != nil {
			m.setStatus(err.Error(), true)
			return
		}
		m.setStatus(fmt.Sprintf("snoozed for %d days", days), false)
	case "note":
		subject := "Note"
		if _, err := m.ir.Create(m.ctx, model.CreateInteractionInput{Type: "note", Subject: &subject, Content: &value, PersonIDs: []int64{r.personID}}); err != nil {
			m.setStatus(err.Error(), true)
			return
		}
		m.setStatus("note added", false)
	case "task":
		title, due, _ := strings.Cut(value, "|")
		in := model.CreateTaskInput{Title: strings.TrimSpace(title), Priority: "medium", PersonID: &r.personID}
		if d := strings.TrimSpace(due); d != "" {
			in.DueAt = &d
		}
		if _, err := m.tr.Create(m.ctx, in); err != nil {
			m.setStatus(err.Error(), true)
			return
		}
		m.setStatus("task added", false)
	}
	m.afterWrite()
}

// afterWrite refreshes lists and the detail pane after a change.
func (m *Model) afterWrite() {
	m.report(m.reload())
}

func (m *Model) markContacted() {
	r, ok := m.selected()
	if !ok || r.personID == 0 {
		return
	}
	when := m.now().UTC().Format("2006-01-02 15:04:05")
	empty := ""
	if _, err := m.pr.Update(m.ctx, r.personID, model.UpdatePersonInput{LastContactedAt: &when, SnoozedUntil: &empty}); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.setStatus("marked as contacted", false)
	m.afterWrite()
}

func (m *Model) toggleFavorite() {
	r, ok := m.selected()
	if !ok || r.personID == 0 {
		return
	}
	p, err := m.pr.FindByID(m.ctx, r.personID)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	fav := !p.IsFavorite
	if _, err := m.pr.Update(m.ctx, r.personID, model.UpdatePersonInput{IsFavorite: &fav}); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.afterWrite()
}

func (m *Model) completeTask() {
	r, ok := m.selected()
	if !ok || r.taskID == 0 {
		m.setStatus("select a task in the Tasks view (4)", true)
		return
	}
	task, err := m.tr.Complete(m.ctx, r.taskID)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	msg := "task done"
	if task.NextTaskID != nil {
		msg += ", next occurrence created"
	}
	m.setStatus(msg, false)
	m.afterWrite()
}
