// Package tui is a keyboard-first terminal browser and editor for the CRM. The mouse works too.
package tui

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/jdanielnd/crm-cli/internal/alias"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/dedupe"
	"github.com/jdanielnd/crm-cli/internal/keys"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/modules"
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
	viewTags
	viewSettings
	numAllViews
)

var viewNames = [numAllViews]string{"Home", "People", "Follow-up", "Tasks", "Birthdays", "Merge", "Archived", "Sync", "Organizations", "Deals", "Tags", "Settings"}

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
	noteID   int64
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
	// setting is the id of the setting a row of the Settings view stands for.
	setting string
	// dupe is the index of the duplicate group in m.groups plus one, 0 for a row that is not a group.
	dupe int
	// tag is the name of the tag a row of the Tags view stands for, tagID its id. On a tag page a row is an
	// object (akind, objID) that carries the tag. See tagview.go.
	tag   string
	tagID int64
	objID int64
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
	ml  *repo.MergeLogRepo
	ntr *repo.NoteTypeRepo
	// vr is the user-managed vocabularies (deal roles).
	vr  *repo.VocabRepo
	db  *sql.DB
	now func() time.Time

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

	people []model.PersonSummary
	rows   []row
	cursor int
	offset int
	query  string
	// sortRecentIn says per view which sort a list has: name or recent (view options, ctrl+o).
	sortChoice map[viewKind]string
	marked     map[int64]bool
	// extend is the selection extension in progress (shift+up, shift+down), nil otherwise.
	extend *extendState
	// orgCreateAnyway lets the next organization create skip the duplicate check (the user chose Create anyway).
	orgCreateAnyway bool
	// aliases is the name alias table, read once from the database (aliasTable).
	aliases *alias.Table
	// The pipe line being typed (pipeline.go): its popup session, the answers given to unresolved names
	// (pipeRes), and the links to write once the object is saved (pipeLinks, of the object kind pipeObj).
	pipe      *pipeSession
	pipeRes   map[string]pendingLink
	pipeLinks []pendingLink
	pipeObj   string
	// pipeFull is the whole line being saved, link segments included: a prompt that has to open again
	// (the names list was cancelled) puts this back, not the object's own part of it.
	pipeFull string
	// typed is the destructive bulk verb waiting for its confirmation word (bulk.go).
	typed *typedConfirm
	// linkAlso are the other selected objects of a bulk link, taken by the link form (linkmany.go).
	linkAlso []repo.Ref
	// setField is the bulk set-field waiting for its typed value (setfield.go).
	setField *setFieldState
	// setFieldDeal and setFieldCreate carry a bulk deal role: the deal it is set on and a role to add to the list first.
	setFieldDeal repo.Ref
	// vocabName is the vocabulary value a rename prompt is for.
	vocabName      string
	setFieldCreate string
	detail         viewport.Model
	detailKey      string
	input          textinput.Model
	promptKind     string
	status         string
	statusIsErr    bool
	readOnly       bool

	// Detail form.
	form       []formRow
	formTitle  string
	formPerson int64
	fcur       int
	fcol       int // the cell under the cursor in a row with cells (cells.go), 0 is the row itself

	lineRow  []int // screen line in the detail content -> form row index, -1 for none
	rowStart []int
	rowEnd   []int
	creating bool
	edit     *editState
	rowInput textinput.Model

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
	themeName string               // name of the active theme
	darkBG    bool                 // terminal has a dark background (resolves the auto theme)
	profile   colorprofile.Profile // what colors the terminal can show (NO_COLOR gives Ascii)
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
	barTag string
	// tagRenaming is the tag the rename prompt is for.
	tagRenaming string
	barResume   *barResume
	// addResume remembers where a global add (ctrl+a) came from (globaladd.go).
	addResume *addReturn
	// addSugg is the highlighted object the add flow started from, the first suggestion of the link segment that takes its kind (globaladd.go).
	addSugg *addSuggestion
	// barKind is the kind of the bar row whose verb is running (enterBarContext), "" outside the bar.
	barKind string
	// mods is the module arrangement (modules.go): which views are on and in what order.
	mods        modules.Arrangement
	// Tags view (tagview.go): the tag whose page is open ("" shows the list), the filter of the list
	// (view options) and the counts read for the rows.
	tagPage   string
	tagQuery  string
	tagFilter string
	tagStats  map[string]repo.TagStat
	tagDups   map[string]repo.TagDupGroup
	keyProblems []string
	capture     *captureState

	// names is the open list of unsettled names, and the choices remembered in this session.
	names      *namesPanel
	nameMemory map[string]int64

	// recent are the bar items picked in this session, newest first.
	recent []overlayItem
}

// New creates the model and loads data.
func New(ctx context.Context, db *sql.DB) (*Model, error) {
	in := newTextInput()
	in.CharLimit = 500
	ri := newTextInput()
	ri.CharLimit = 2000
	ri.Prompt = ""
	registry := crmsync.DefaultRegistry(os.Getenv)
	m := &Model{
		pipeRes:    map[string]pendingLink{},
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
		vr:         repo.NewVocabRepo(db),
		minLevel:   dedupe.Fuzzy, // the Merge view lists every candidate, grouped by level; ctrl+o narrows it
		noteType:   "note",
		now:        time.Now,
		input:      in,
		rowInput:   ri,
		detail:     viewport.New(),
		helpVP:     viewport.New(),
		profile:    colorprofile.TrueColor,
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
func Run(ctx context.Context, db *sql.DB, readOnly bool, noColor bool, env Env) error {
	m, err := New(ctx, db)
	if err != nil {
		return err
	}
	m.readOnly = readOnly
	m.env = env
	m.applyStartupTheme()
	m.applyStartupKeys()
	m.applyStartupModules()
	opts := []tea.ProgramOption{tea.WithContext(ctx)}
	if noColor {
		opts = append(opts, tea.WithColorProfile(colorprofile.Ascii))
	}
	_, err = tea.NewProgram(m, opts...).Run()
	return err
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd { return tea.RequestBackgroundColor }

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
	var keepPerson, keepTask, keepOrg, keepDeal, keepTag, keepObj int64
	keepObjKind := ""
	oldCursor := m.cursor
	if keep && m.cursor >= 0 && m.cursor < len(m.rows) {
		keepPerson, keepTask = m.rows[m.cursor].personID, m.rows[m.cursor].taskID
		keepOrg, keepDeal = m.rows[m.cursor].orgID, m.rows[m.cursor].dealID
		keepTag, keepObj, keepObjKind = m.rows[m.cursor].tagID, m.rows[m.cursor].objID, m.rows[m.cursor].akind
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
	case viewTags:
		tr, err := m.tagRows()
		if err != nil {
			return err
		}
		rows = tr
	case viewSettings:
		rows = m.settingRows()
	case viewMerge:
		groups, err := m.dr.Find(m.ctx, m.minLevel)
		if err != nil {
			return err
		}
		m.groups = groups
		rows = m.mergeRows(groups)
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
			if (keepTag != 0 && r.tagID == keepTag) || (keepObj != 0 && r.objID == keepObj && r.akind == keepObjKind) {
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
	if m.sortedRecent(viewPeople) {
		sorted := append([]model.PersonSummary(nil), people...)
		sortByUpdated(sorted)
		people = sorted
	}
	terms := strings.Fields(strings.ToLower(m.query))
	var rows []row
	for _, p := range people {
		if !matchesAlias(p, terms, m.aliasTable()) {
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

func matches(p model.PersonSummary, terms []string) bool { return matchesAlias(p, terms, nil) }

// matchesAlias is matches with each plain word expanded through the name aliases (Rick finds Richard).
func matchesAlias(p model.PersonSummary, terms []string, tbl *alias.Table) bool {
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
		found := false
		for _, n := range tbl.Expand(t) {
			if strings.Contains(hay, n) || strings.Contains(tags, n) {
				found = true
				break
			}
		}
		if !found {
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

// markedIDs lists the selected people that are visible in the current list, in list order.
func (m *Model) markedIDs() []int64 { return m.markedOfKind("person") }

// targets are the people a person action applies to: the marked ones in the list, else the current one.
func (m *Model) targets() []int64 {
	if m.focus == focusList && len(m.marked) > 0 {
		return m.markedIDs() // rows of another kind are selected: no person is the target
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
		m.detail.SetWidth(m.width - m.listWidth() - 3)
		m.detail.SetHeight(m.bodyHeight())
		m.helpVP.SetWidth(m.width)
		m.helpVP.SetHeight(max(1, m.height-2))
		if m.tv != nil {
			m.tv.vp.SetWidth(m.width)
			m.tv.vp.SetHeight(max(1, m.height-2))
		}
		m.clamp()
		m.refreshDetail(true)
		return m, nil
	case tea.MouseClickMsg:
		return m.handleMouse(mouseEvent{msg.X, msg.Y, msg.Button})
	case tea.MouseWheelMsg:
		return m.handleMouse(mouseEvent{msg.X, msg.Y, msg.Button})
	case tea.ColorProfileMsg:
		// Bubble Tea reports what the terminal can show (NO_COLOR and --no-color included).
		if msg.Profile != m.profile {
			m.profile = msg.Profile
			m.setTheme(m.st.Theme)
		}
		return m, nil
	case tea.BackgroundColorMsg:
		m.darkBG = msg.IsDark()
		if m.themeName == theme.AutoName {
			if t, err := m.themeCatalog().Find(theme.AutoName); err == nil {
				m.setTheme(t)
			}
		}
		return m, nil
	case tea.PasteMsg:
		return m.handlePaste(msg)
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
	case tea.KeyPressMsg:
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
func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if k := keyName(msg); k != "shift+up" && k != "shift+down" {
		m.extend = nil // any other key ends the extension of the selection
	}
	if a := m.lookupKey(keyName(msg), keyNormal); a != nil {
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

func (m *Model) handlePromptKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch keyName(msg) {
	case "enter":
		value := strings.TrimSpace(m.input.Value())
		kind := m.promptKind
		m.mode = modeNormal
		m.input.Blur()
		m.addSugg, m.pipe = nil, nil
		m.pendingCmd = nil
		if value != "" || kind == "link-attrs" {
			m.runPrompt(kind, value)
		}
		cmd := m.pendingCmd
		m.pendingCmd = nil
		return m, cmd
	}
	// The pipe line sees up, down, tab and esc first while its popup is open (pipecomplete.go).
	if m.pipeKey(msg) {
		return m, nil
	}
	switch keyName(msg) {
	case "esc":
		m.mode = modeNormal
		m.input.Blur()
		m.addSugg, m.pipe = nil, nil
		m.pipeRes, m.pipeLinks = map[string]pendingLink{}, nil
		return m, nil
	case "tab", "shift+tab":
		// The text-entry keys come from the registry (prompt-tab, prompt-tab-back in verbs.go).
		if a := m.lookupKey(keyName(msg), keyText); a != nil {
			return m, m.runAction(a)
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.pipeRefresh()
	return m, cmd
}

func (m *Model) handleConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	c := m.confirm
	switch keyName(msg) {
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
	if kind == personAddKind {
		// A picker for a name of the new person's line ended: back to the name row, which saves again.
		if m.creating && m.edit != nil {
			m.mode = modeEdit
			_ = m.rowInput.Focus()
			m.commitEdit()
		}
		return
	}
	// A pipe line is checked and its link segments resolved first (pipelink.go). The save code below
	// reads only the object's own segments.
	if pl := m.pipeFor(kind); pl != nil {
		prompt, full := m.input.Prompt, value
		m.pipeFull = full
		reopen := func() { m.startInput(kind, prompt, full); _ = m.input.Focus() }
		if !m.pipePrepare(kind, value, reopen) {
			return
		}
		value = pl.coreLine(value)
		defer func() {
			// A save that failed puts the line back: keep the link segments in it.
			if m.mode == modePrompt && m.promptKind == kind && m.input.Value() == value && full != value {
				m.input.SetValue(full)
				m.input.CursorEnd()
			}
		}()
	}
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
	case "tag-new":
		m.saveNewTag(value)
		return
	case "vocab-role-add":
		m.addDealRole(value)
		return
	case "vocab-role-rename":
		m.renameDealRole(value)
		return
	case "vocab-stage-add":
		m.addDealStage(value)
		return
	case "vocab-stage-rename":
		m.renameDealStage(value)
		return
	case "set-field":
		m.runSetField(value)
		return
	case "typed-confirm":
		m.runTypedConfirm(value)
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
		m.runNoteAs(pid, value, "note")
		return
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

// reopenLine is the text to put back into a prompt that opens again: the whole line when the prompt is a
// pipe line, else the text given.
func (m *Model) reopenLine(value string) string {
	if m.pipeFull != "" && strings.HasPrefix(m.pipeFull, value) {
		return m.pipeFull
	}
	return value
}

// runNoteAs saves a note line for a person through the same reader as the command bar: #tag tags the
// note, @Name links people, remind fri makes a reminder and every 3m sets the cadence. kind is the prompt
// the line is typed in, to open it again when the names list is cancelled.
func (m *Model) runNoteAs(pid int64, value, kind string) {
	typ := m.currentNoteType()
	env := m.quickEnvFor(pid)
	env.NoteType = typ
	_, err := m.runLineAsk("note "+value, env,
		func() { m.startInput(kind, typ+": ", m.reopenLine(value)); _ = m.input.Focus() },
		func(plan *quick.Plan) tea.Cmd {
			m.noteType = typ
			m.setStatus(typ+" added"+extras(plan), false)
			return m.showToast(typ+" added"+extras(plan), false)
		})
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.afterWrite()
}

// aliasTable is the name alias table of this database, read once per session. A failed read gives
// the built-in set.
func (m *Model) aliasTable() *alias.Table {
	if m.aliases == nil {
		m.aliases = alias.Default()
		if m.db != nil {
			if t, err := repo.NewAliasRepo(m.db).Table(m.ctx); err == nil {
				m.aliases = t
			}
		}
	}
	return m.aliases
}
