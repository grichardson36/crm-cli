// Package tui is a keyboard-first terminal browser and editor for the CRM. The mouse works too.
package tui

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/dedupe"
	"github.com/jdanielnd/crm-cli/internal/keys"
	"github.com/jdanielnd/crm-cli/internal/modules"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/quick"
	"github.com/jdanielnd/crm-cli/internal/shortsyntax"
	crmsync "github.com/jdanielnd/crm-cli/internal/sync"
	"github.com/jdanielnd/crm-cli/internal/theme"
)

type viewKind int

const (
	viewHome viewKind = iota
	viewPeople
	viewDue
	viewTasks
	viewBirthdays
	viewMerge
	viewArchived
	viewSync
	numViews
)

// The advanced views have no number key and are not in the view switcher, [ or ]. They open from
// the palette ("Organizations view", "Deals view"). See orgdeal.go.
const (
	viewOrgs viewKind = numViews + iota
	viewDeals
	numAllViews
)

var viewNames = [numAllViews]string{"Home", "People", "Follow-up", "Tasks", "Birthdays", "Merge", "Archived", "Sync", "Organizations", "Deals"}

type mode int

const (
	modeNormal  mode = iota
	modePrompt       // footer input: snooze, note, task, tag
	modeEdit         // inline input on a detail row
	modeConfirm      // y/n question in the footer
	modeOverlay      // command palette or picker
	modeCapture      // waiting for the key to bind to an action
	modeNames        // list of names to settle before a line is saved
)

// focus says which pane receives movement keys.
type focus int

const (
	focusList focus = iota
	focusDetail
)

// row is one line in the list pane.
type row struct {
	personID int64
	taskID   int64
	orgID    int64
	dealID   int64
	title    string
	sub      string
	meta     string

	// skip rows (section headings, the status line) are drawn but never selected.
	skip bool
	// rule marks a skip row drawn as a divider: title at the left, a line, meta at the right edge.
	rule bool
	// akind is the object kind of a row in the Archived view.
	akind string
	// jump is a view number (view+1) that Enter opens for rows that stand for a whole view.
	jump int
	// sync carries the Sync view data of the row.
	sync *syncItem
}

// Model is the Bubble Tea model.
type Model struct {
	ctx  context.Context
	pr   *repo.PersonRepo
	ir   *repo.InteractionRepo
	tr   *repo.TaskRepo
	orr  *repo.OrgRepo
	dlr  *repo.DealRepo
	tagr *repo.TagRepo
	dr   *repo.DupeRepo
	// ml is the persistent merge log: person and organization merges undo and redo through it.
	ml *repo.MergeLogRepo
	ntr  *repo.NoteTypeRepo
	db   *sql.DB
	now  func() time.Time

	// Sync view: the adapters this process can run, the runner and the inbox over the database.
	registry *crmsync.Registry
	runner   *crmsync.Runner
	inbox    *crmsync.Inbox
	// busy names the background job in flight ("" when idle). spin animates the status bar.
	busy string
	spin spinner.Model
	// deleted is the session stack of deleted ids per object kind, for the Restore verb.
	deleted map[string][]int64
	// links makes, lists and removes links between objects.
	links *repo.LinkRepo
	// oa applies the company-text rule (company text creates or reuses an organization).
	oa *repo.OrgAutoRepo
	// orgSimilar maps an organization id to the organizations with a close name (merge candidates).
	orgSimilar map[int64][]orgNear
	// draft is the link being made in the Link form.
	draft *linkDraft
	// lineID is the record the open one-line edit prompt applies to. orgs and deals are the
	// records of the rows by id for the detail pane of the Organizations and Deals views.
	lineID int64
	orgs   map[int64]*model.Organization
	deals  map[int64]*model.Deal

	// clip copies text to the system clipboard (OSC52 by default). open launches a URL.
	clip func(string)
	open func(string) error

	width, height int
	view          viewKind
	mode          mode
	focus         focus
	help          bool
	helpVP        viewport.Model
	tv            *textView // read-only page: help topics, doctor, where, domains
	env           Env
	lastFilter    string // filter text of the overlay that was just picked
	actions       []action

	people      []model.PersonSummary
	rows        []row
	cursor      int
	offset      int
	query       string
	sortRecent  bool
	marked      map[int64]bool
	// orgCreateAnyway lets the next organization create skip the duplicate check (the user chose Create anyway).
	orgCreateAnyway bool
	detail      viewport.Model
	detailKey   string
	input       textinput.Model
	promptKind  string
	status      string
	statusIsErr bool
	readOnly    bool

	// Detail form.
	form       []formRow
	formTitle  string
	formPerson int64
	fcur       int
	lineRow    []int // screen line in the detail content -> form row index, -1 for none
	rowStart   []int
	rowEnd     []int
	creating   bool
	edit       *editState
	rowInput   textinput.Model

	confirm    *confirmState
	ov         *overlay
	pendingCmd tea.Cmd

	// Merge view: duplicate groups parallel to rows, the minimum level and the dismissals made
	// this session (newest last) for undo. cmp is the open compare screen, if any.
	groups    []dedupe.Group
	minLevel  dedupe.Level
	dismissed [][]int64
	cmp       *compareState

	// Archived view: the archived people by id, the other archived objects by kind and id, and the
	// type filter ("" shows every kind).
	arch      map[int64]*model.Person
	archItems map[string]repo.ArchivedItem
	archKind  string

	// noteType is the type used by the next note prompt (remembered between notes).
	noteType string

	// st holds every style of the active theme. Views draw only with it. See theme.go.
	st        theme.Styles
	themeName string // name of the active theme
	darkBG    bool   // terminal has a dark background (resolves the auto theme)
	// Command bar: caches for the quick package, the person the palette opened on, the changes of
	// command lines run this session (newest last) for Undo, and the short syntax switch.
	qix       *quick.Index
	qtags     []string
	qtypes    []string
	cmdPerson int64
	hist      []histEntry
	redoStack []histEntry
	grp       []histEntry
	grpDepth  int
	toast     *toast
	toastSeq  int
	noSyntax  bool

	// Key bindings changed by the user (config.toml [keys]), the problems found while loading
	// them, and the action being rebound while the next key press is captured.
	keyOverrides keys.Overrides
	// bdToday is the row of the first birthday from today on in the Birthdays view and bdTop the first
	// heading above it (its day divider, and its month heading when that sits right above).
	bdToday, bdTop int
	// barTag is the tag of the bar row a verb is running on (barverbs.go). barResume remembers a bar that
	// is waiting for a verb with a prompt, a confirm or a picker to finish.
	barTag    string
	// tagRenaming is the tag the rename prompt is for.
	tagRenaming string
	barResume *barResume
	// addResume remembers where a global add (ctrl+a) came from (globaladd.go).
	addResume *addReturn
	// addSugg is the highlighted object offered as the link of a new task or note (globaladd.go).
	addSugg *addSuggestion
	// barKind is the kind of the bar row whose verb is running (enterBarContext), "" outside the bar.
	barKind string
	// mods is the module arrangement (modules.go): which views are on and in what order.
	mods modules.Arrangement
	keyProblems  []string
	capture      *captureState

	// names is the open list of unsettled names, and the choices remembered in this session.
	names      *namesPanel
	nameMemory map[string]int64

	// recent are the bar items picked in this session, newest first.
	recent []overlayItem
}

// New creates the model and loads data.
func New(ctx context.Context, db *sql.DB) (*Model, error) {
	in := textinput.New()
	in.CharLimit = 500
	ri := textinput.New()
	ri.CharLimit = 2000
	ri.Prompt = ""
	registry := crmsync.DefaultRegistry(os.Getenv)
	m := &Model{
		ctx:        ctx,
		db:         db,
		registry:   registry,
		runner:     crmsync.NewRunner(db, registry),
		inbox:      crmsync.NewInbox(db),
		spin:       spinner.New(spinner.WithSpinner(spinner.Dot)),
		view:       viewHome,
		pr:         repo.NewPersonRepo(db),
		ir:         repo.NewInteractionRepo(db),
		tr:         repo.NewTaskRepo(db),
		orr:        repo.NewOrgRepo(db),
		dlr:        repo.NewDealRepo(db),
		tagr:       repo.NewTagRepo(db),
		links:      repo.NewLinkRepo(db),
		oa:         repo.NewOrgAutoRepo(db),
		deleted:    map[string][]int64{},
		dr:         repo.NewDupeRepo(db),
		ml:         repo.NewMergeLogRepo(db),
		ntr:        repo.NewNoteTypeRepo(db),
		minLevel:   dedupe.Medium,
		noteType:   "note",
		now:        time.Now,
		input:      in,
		rowInput:   ri,
		detail:     viewport.New(0, 0),
		helpVP:     viewport.New(0, 0),
		marked:     map[int64]bool{},
		nameMemory: map[string]int64{},
		actions:    buildActions(),
		mods:       modules.Default(),
		clip:       func(s string) { osc52Copy(os.Stdout, s) },
		open:       openURL,
		st:         defaultStyles(),
	}
	if err := m.reload(); err != nil {
		return nil, err
	}
	return m, nil
}

// Run starts the TUI.
func Run(ctx context.Context, db *sql.DB, readOnly bool, env Env) error {
	m, err := New(ctx, db)
	if err != nil {
		return err
	}
	m.readOnly = readOnly
	m.env = env
	m.applyStartupTheme()
	m.applyStartupKeys()
	m.applyStartupModules()
	_, err = tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithContext(ctx)).Run()
	return err
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) setStatus(msg string, isErr bool) {
	m.status, m.statusIsErr = msg, isErr
}

func (m *Model) report(err error) {
	if err != nil {
		m.setStatus(err.Error(), true)
	}
}

// reload re-reads everything from the database and rebuilds the current view.
func (m *Model) reload() error {
	people, err := m.pr.ListSummaries(m.ctx)
	if err != nil {
		return err
	}
	m.people = people
	m.qix, m.qtags, m.qtypes = nil, nil, nil
	return m.rebuild(true)
}

// afterWrite refreshes lists and the detail pane after a change.
func (m *Model) afterWrite() {
	m.orgSimilar = nil // merge candidates are recomputed when an organization is shown
	m.report(m.reload())
}

// rebuild recomputes rows for the current view. With keep it leaves the cursor on the same item,
// or at the same position if that item is gone. Otherwise the cursor goes to the top.
func (m *Model) rebuild(keep bool) error {
	var keepPerson, keepTask, keepOrg, keepDeal int64
	oldCursor := m.cursor
	if keep && m.cursor >= 0 && m.cursor < len(m.rows) {
		keepPerson, keepTask = m.rows[m.cursor].personID, m.rows[m.cursor].taskID
		keepOrg, keepDeal = m.rows[m.cursor].orgID, m.rows[m.cursor].dealID
	}

	var rows []row
	switch m.view {
	case viewHome:
		hr, err := m.homeRows()
		if err != nil {
			return err
		}
		rows = hr
	case viewSync:
		sr, err := m.syncRows()
		if err != nil {
			return err
		}
		rows = sr
	case viewPeople:
		rows = m.peopleRows()
	case viewDue:
		due, err := m.pr.Due(m.ctx, 0, 1000)
		if err != nil {
			return err
		}
		for _, d := range due {
			rows = append(rows, row{personID: d.PersonID, title: cleanCell(d.Name), sub: d.KeepInTouch, meta: fmt.Sprintf("%dd overdue", d.DaysOverdue)})
		}
	case viewBirthdays:
		br, err := m.birthdayRows()
		if err != nil {
			return err
		}
		rows = br
	case viewTasks:
		tasks, err := m.tr.FindAll(m.ctx, model.TaskFilters{})
		if err != nil {
			return err
		}
		for _, t := range tasks {
			r := row{taskID: t.ID, title: cleanCell(t.Title)}
			if len(t.PersonIDs) > 0 {
				r.personID = t.PersonIDs[0]
			}
			if t.DueAt != nil {
				r.meta = *t.DueAt
			}
			if t.Recurrence != nil {
				r.sub = "repeats " + *t.Recurrence
			}
			for _, tg := range t.Tags {
				if r.sub != "" {
					r.sub += " "
				}
				r.sub += "#" + tg
			}
			rows = append(rows, r)
		}
	case viewOrgs:
		or, err := m.orgRows()
		if err != nil {
			return err
		}
		rows = or
	case viewDeals:
		dr, err := m.dealRows()
		if err != nil {
			return err
		}
		rows = dr
	case viewMerge:
		groups, err := m.dr.Find(m.ctx, m.minLevel)
		if err != nil {
			return err
		}
		m.groups = groups
		for _, g := range groups {
			rows = append(rows, groupRow(g))
		}
	case viewArchived:
		ar, err := m.archivedRows()
		if err != nil {
			return err
		}
		rows = ar
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
			if (keepOrg != 0 && r.orgID == keepOrg) || (keepDeal != 0 && r.dealID == keepDeal) {
				m.cursor = i
				break
			}
		}
	} else {
		m.offset = 0
	}
	m.settle()
	if m.view == viewBirthdays && !keep {
		m.snapBirthdays()
	}
	m.clamp()
	m.refreshDetail(true)
	return nil
}

// settle moves the cursor off rows that cannot be selected (headings), forward first.
func (m *Model) settle() {
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 || !m.rows[m.cursor].skip {
		return
	}
	for i := m.cursor; i < len(m.rows); i++ {
		if !m.rows[i].skip {
			m.cursor = i
			return
		}
	}
	for i := m.cursor; i >= 0; i-- {
		if !m.rows[i].skip {
			m.cursor = i
			return
		}
	}
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
		name := cleanCell(p.Name)
		if p.IsFavorite {
			name = "★ " + name
		}
		sub := cleanCell(p.Company)
		if sub == "" {
			sub = cleanCell(p.Title)
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
	for i := 0; i < len(terms); i++ {
		t := terms[i]
		if strings.HasPrefix(t, "#") {
			if len(t) > 1 && !strings.Contains(tags, t[1:]) {
				return false
			}
			continue
		}
		// The short syntax tokens also filter: !high (priority tier) and every 3m (cadence).
		if strings.HasPrefix(t, "!") && len(t) > 1 {
			if pr, ok := shortsyntax.ParsePriority(t[1:]); ok {
				if p.PriorityTier != shortsyntax.TierOf(pr) {
					return false
				}
				continue
			}
		}
		if t == "every" && i+1 < len(terms) {
			if c, ok := shortsyntax.ParseCadence(terms[i+1]); ok {
				if p.KeepInTouch != c {
					return false
				}
				i++
				continue
			}
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
	// Keep the headings above the first selectable row in view.
	if m.cursor < len(m.rows) && m.cursor < h {
		onlyHeads := true
		for i := 0; i < m.cursor; i++ {
			if !m.rows[i].skip {
				onlyHeads = false
			}
		}
		if onlyHeads {
			m.offset = 0
		}
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

// curPerson is the person the actions apply to, or 0.
func (m *Model) curPerson() int64 {
	if m.creating {
		return 0
	}
	r, ok := m.selected()
	if !ok {
		return 0
	}
	return r.personID
}

func (m *Model) move(delta int) {
	dir := 1
	if delta < 0 {
		dir, delta = -1, -delta
	}
	cur := m.cursor
	for n := 0; n < delta; n++ {
		next := cur + dir
		for next >= 0 && next < len(m.rows) && m.rows[next].skip {
			next += dir
		}
		if next < 0 || next >= len(m.rows) {
			break
		}
		cur = next
	}
	m.cursor = cur
	m.clamp()
	m.refreshDetail(false)
}

// markedIDs lists the marked people that are visible in the current list, in list order.
func (m *Model) markedIDs() []int64 {
	if len(m.marked) == 0 {
		return nil
	}
	var ids []int64
	seen := map[int64]bool{}
	for _, r := range m.rows {
		if r.personID != 0 && m.marked[r.personID] && !seen[r.personID] {
			seen[r.personID] = true
			ids = append(ids, r.personID)
		}
	}
	return ids
}

// targets are the people a person action applies to: the marked ones in the list, else the current one.
func (m *Model) targets() []int64 {
	if m.focus == focusList {
		if ids := m.markedIDs(); len(ids) > 0 {
			return ids
		}
	}
	if p := m.curPerson(); p != 0 {
		return []int64{p}
	}
	return nil
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	mm, cmd := m.update(msg)
	if resume := m.resumeBar(); resume != nil {
		cmd = tea.Batch(cmd, resume)
	}
	return mm, cmd
}

func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.detail.Width = m.width - m.listWidth() - 3
		m.detail.Height = m.bodyHeight()
		m.helpVP.Width = m.width
		m.helpVP.Height = max(1, m.height-2)
		if m.tv != nil {
			m.tv.vp.Width, m.tv.vp.Height = m.width, max(1, m.height-2)
		}
		m.clamp()
		m.refreshDetail(true)
		return m, nil
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case toastExpireMsg:
		if m.toast != nil && m.toast.seq == msg.seq {
			m.toast = nil
		}
		return m, nil
	case syncDoneMsg:
		m.finishSync(msg)
		return m, nil
	case spinner.TickMsg:
		if m.busy == "" {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case tea.KeyMsg:
		if m.mode == modeOverlay {
			return m.handleOverlayKey(msg)
		}
		if m.mode == modeCapture {
			return m.handleCaptureKey(msg)
		}
		if m.mode == modeNames {
			return m.handleNamesKey(msg)
		}
		if m.tv != nil {
			return m.handleTextKey(msg)
		}
		if m.help {
			return m.handleHelpKey(msg)
		}
		switch m.mode {
		case modePrompt:
			return m.handlePromptKey(msg)
		case modeEdit:
			return m.handleEditKey(msg)
		case modeConfirm:
			return m.handleConfirmKey(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) switchView(v viewKind) {
	if v == m.view {
		return
	}
	m.view = v
	m.cursor, m.offset = 0, 0
	m.marked = map[int64]bool{}
	m.focus = focusList
	m.setStatus("", false)
	if err := m.rebuild(false); err != nil {
		m.setStatus(err.Error(), true)
	}
}

// handleKey dispatches a key in normal mode through the action registry.
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a := m.lookupKey(msg.String(), keyNormal); a != nil {
		return m, m.runAction(a)
	}
	return m, nil
}

// runAction runs an action, enforcing read-only mode.
func (m *Model) runAction(a *action) tea.Cmd {
	if a.write && m.readOnly {
		msg := "read-only: " + strings.ToLower(a.title) + " is disabled"
		m.setStatus(msg, true)
		return m.showToast(msg, true)
	}
	return a.run(m)
}

func (m *Model) actionByID(id string) *action {
	for i := range m.actions {
		if m.actions[i].id == id {
			return &m.actions[i]
		}
	}
	return nil
}

func (m *Model) startPrompt(kind, prompt, initial string) {
	if m.curPerson() == 0 {
		return
	}
	m.startInput(kind, prompt, initial)
}

// startInput opens the footer input for a prompt that needs no selected person.
func (m *Model) startInput(kind, prompt, initial string) {
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
		m.addSugg = nil
		return m, nil
	case "enter":
		value := strings.TrimSpace(m.input.Value())
		kind := m.promptKind
		m.mode = modeNormal
		m.input.Blur()
		m.addSugg = nil
		m.pendingCmd = nil
		if value != "" || kind == "link-attrs" {
			m.runPrompt(kind, value)
		}
		cmd := m.pendingCmd
		m.pendingCmd = nil
		return m, cmd
	case "tab", "shift+tab":
		// The text-entry keys come from the registry (prompt-tab, prompt-tab-back in verbs.go).
		if a := m.lookupKey(msg.String(), keyText); a != nil {
			return m, m.runAction(a)
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	c := m.confirm
	switch msg.String() {
	case "y", "Y":
		m.mode, m.confirm = modeNormal, nil
		m.pendingCmd = nil
		if c != nil {
			c.yes()
		}
		cmd := m.pendingCmd
		m.pendingCmd = nil
		return m, cmd
	case "n", "N", "esc":
		m.mode, m.confirm = modeNormal, nil
		m.setStatus("cancelled", false)
	}
	return m, nil
}

// bulk applies fn to every id and reports one status line. Marks clear afterwards.
func (m *Model) bulk(ids []int64, single, plural string, fn func(id int64) error) {
	ok, failed := 0, 0
	var first error
	m.beginGroup()
	for _, id := range ids {
		if err := fn(id); err != nil {
			failed++
			if first == nil {
				first = err
			}
			continue
		}
		ok++
	}
	if ok > 1 {
		m.endGroup(fmt.Sprintf("%s: %d people", plural, ok))
	} else {
		m.endGroup("")
	}
	if m.focus == focusList {
		m.marked = map[int64]bool{}
	}
	switch {
	case failed > 0:
		m.setStatus(fmt.Sprintf("%d done, %d failed: %v", ok, failed, first), true)
	case ok == 1:
		m.setStatus(single, false)
	default:
		m.setStatus(fmt.Sprintf("%s: %d people", plural, ok), false)
	}
	m.afterWrite()
}

func (m *Model) runPrompt(kind, value string) {
	if kind == "tag-rename" {
		m.runTagRename(value)
		return
	}
	if strings.HasPrefix(kind, "org-") || strings.HasPrefix(kind, "deal-") {
		m.runOrgDealPrompt(kind, value)
		return
	}
	switch kind {
	case "link-attrs":
		m.runLinkAttrs(value)
		return
	case "note-object":
		m.saveObjectNote(value)
		return
	case "note-free":
		m.saveFreeNote(value)
		return
	case "task-new":
		m.runTaskLine(0, value, "task-new")
		return
	case "task-page":
		m.runTaskLine(0, value, "task-page")
		return
	case "task-edit":
		m.saveTaskEdit(m.lineID, value)
		return
	case "notetype-add":
		m.addNoteType(value)
		return
	case "purge":
		m.purgeCurrent(value)
		return
	case "tasktag":
		m.tagTask(value)
		return
	case "help-search":
		m.openHelpSearch(value)
		return
	}
	ids := m.targets()
	if len(ids) == 0 {
		m.setStatus("no person selected", true)
		return
	}
	pid := m.curPerson()
	switch kind {
	case "snooze":
		days, err := parseDays(value)
		if err != nil {
			m.setStatus("enter a number of days", true)
			return
		}
		until := m.now().UTC().AddDate(0, 0, days).Format("2006-01-02 15:04:05")
		m.bulk(ids, fmt.Sprintf("snoozed for %d days", days), "snoozed", func(id int64) error {
			_, err := m.hUpdatePerson(m.ctx, id, model.UpdatePersonInput{SnoozedUntil: &until})
			return err
		})
		return
	case "tag":
		remove := strings.HasPrefix(value, "-")
		name := strings.TrimLeft(strings.TrimSpace(strings.TrimPrefix(value, "-")), "#")
		if name == "" {
			m.setStatus("enter a tag name (prefix with - to remove)", true)
			return
		}
		m.bulk(ids, "tag "+map[bool]string{true: "removed", false: "added"}[remove], "tagged "+name, func(id int64) error {
			if remove {
				return m.hTagRemove(m.ctx, "person", id, name)
			}
			return m.hTagApply(m.ctx, "person", id, name)
		})
		return
	case "note":
		if pid == 0 {
			m.setStatus("no person selected", true)
			return
		}
		// The note goes through the same reader as the command bar: #tag tags the note, +Name
		// links people, remind fri makes a reminder and every 3m sets the cadence.
		typ := m.currentNoteType()
		env := m.quickEnvFor(pid)
		env.NoteType = typ
		_, err := m.runLineAsk("note "+value, env,
			func() { m.startInput("note", typ+": ", value); _ = m.input.Focus() },
			func(plan *quick.Plan) tea.Cmd {
				m.noteType = typ
				m.setStatus(typ+" added"+extras(plan), false)
				return m.showToast(typ+" added"+extras(plan), false)
			})
		if err != nil {
			m.setStatus(err.Error(), true)
			return
		}
	case "task":
		if pid == 0 {
			m.setStatus("no person selected", true)
			return
		}
		m.runTaskLine(pid, value, "task")
		return
	}
	m.afterWrite()
}
