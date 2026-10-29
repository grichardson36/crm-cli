package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/modules"
	"github.com/jdanielnd/crm-cli/internal/query"
	"github.com/jdanielnd/crm-cli/internal/shortsyntax"
)

// Saved views in the terminal UI (patterns/saved-views). A saved view is a module the user makes: it
// shows in the Views screen, takes a tab and a number key while it is shown, and renders with the list
// of its kind (People, Organizations, Deals, Tasks, Tags, or the Notes list) with its filter, sort and
// group applied. It is not a new kind of screen. m.view stays the view of the kind, so the bulk verbs,
// marks, search, the detail pane and the keys are the ones of that list. m.sv says that a saved view
// shapes the rows.

// svState is the saved view on screen. q is the stored query. The view options (ctrl+o) change the
// session choices below and never the stored view.
type svState struct {
	v repo.SavedView
	// The session tweaks of the view options. Empty means the saved one.
	filterOff bool
	sortKey   string // "name", "name:desc" ... or "none"
	groupKey  string // a field key or "none"
}

// svViewFor is the list view that shows a kind.
func svViewFor(kind string) viewKind {
	switch kind {
	case query.KindOrganization:
		return viewOrgs
	case query.KindDeal:
		return viewDeals
	case query.KindTask:
		return viewTasks
	case query.KindTag:
		return viewTags
	case query.KindNote:
		return viewNotes
	}
	return viewPeople
}

// svKindOf is the kind a list view stands for, for Save current view as.
func svKindOf(v viewKind) (string, bool) {
	switch v {
	case viewPeople:
		return query.KindPerson, true
	case viewOrgs:
		return query.KindOrganization, true
	case viewDeals:
		return query.KindDeal, true
	case viewTasks:
		return query.KindTask, true
	case viewTags:
		return query.KindTag, true
	case viewNotes:
		return query.KindNote, true
	}
	return "", false
}

// loadSaved reads the live and the deleted saved views.
func (m *Model) loadSaved() {
	if m.svr == nil {
		return
	}
	m.svs, _ = m.svr.List(m.ctx, false)
	m.svDeleted, _ = m.svr.List(m.ctx, true)
	if m.sv != nil {
		found := false
		for _, v := range m.svs {
			if v.UUID == m.sv.v.UUID {
				m.sv.v, found = v, true
			}
		}
		if !found { // deleted from the CLI or another device while it was on screen
			m.sv = nil
		}
	}
	m.refreshSavedActions()
}

func (m *Model) savedByUUID(uuid string) (repo.SavedView, bool) {
	for _, v := range m.svs {
		if v.UUID == uuid {
			return v, true
		}
	}
	return repo.SavedView{}, false
}

func (m *Model) savedByID(id int64) (repo.SavedView, bool) {
	for _, v := range m.svs {
		if v.ID == id {
			return v, true
		}
	}
	for _, v := range m.svDeleted {
		if v.ID == id {
			return v, true
		}
	}
	return repo.SavedView{}, false
}

// svShown lists the saved views that take a tab, in order.
func (m *Model) svShown() []repo.SavedView {
	var out []repo.SavedView
	for _, v := range m.svs {
		if v.ShowInTabs {
			out = append(out, v)
		}
	}
	return out
}

// svActive says a saved view shapes the rows of the current view.
func (m *Model) svActive() bool {
	return m.sv != nil && svViewFor(m.sv.v.Kind) == m.view
}

// svQuery is the query on screen: the stored one with the session tweaks of the view options.
func (m *Model) svQuery() query.Query {
	q := m.sv.v.Query.Clone()
	st := m.sv
	if st.filterOff {
		q.Filter, q.Search = nil, ""
	}
	switch st.sortKey {
	case "":
	case "none":
		q.Sort = nil
	default:
		s, _ := query.ParseSort(st.sortKey)
		q.Sort = []query.Sort{s}
	}
	switch st.groupKey {
	case "":
	case "none":
		q.GroupBy, q.Bucket = "", ""
	default:
		q.GroupBy = st.groupKey
		if st.groupKey != m.sv.v.Query.GroupBy {
			q.Bucket = ""
		}
	}
	return q
}

// openSaved goes to a saved view, shown or hidden.
func (m *Model) openSaved(uuid string) {
	v, ok := m.savedByUUID(uuid)
	if !ok {
		m.setStatus("that saved view is deleted, restore it first", true)
		return
	}
	m.sv = &svState{v: v}
	m.view = svViewFor(v.Kind)
	m.cursor, m.offset = 0, 0
	m.marked = map[int64]bool{}
	m.focus = focusList
	m.query = ""
	m.tagPage = ""
	m.setStatus("", false)
	if err := m.rebuild(false); err != nil {
		m.setStatus(err.Error(), true)
	}
}

// --- the tab entries ---

// tabEntry is one place in the tabs: a built-in view or a saved view.
type tabEntry struct {
	view viewKind
	sv   string // uuid of a saved view
}

// visibleEntries lists the tabs: the views that are on in the arrangement, then the saved views that
// are shown, in their order.
func (m *Model) visibleEntries() []tabEntry {
	var out []tabEntry
	for _, v := range m.visibleViews() {
		out = append(out, tabEntry{view: v})
	}
	for _, v := range m.svShown() {
		out = append(out, tabEntry{view: svViewFor(v.Kind), sv: v.UUID})
	}
	return out
}

func (m *Model) entryOn(e tabEntry) bool {
	if e.sv != "" {
		return m.sv != nil && m.sv.v.UUID == e.sv
	}
	return e.view == m.view && m.sv == nil
}

func (m *Model) goEntry(e tabEntry) {
	if e.sv != "" {
		m.openSaved(e.sv)
		return
	}
	m.switchView(e.view)
}

// stepView is [ and ]: the next or previous tab, saved views included. From a place that has no tab
// ] goes to the first and [ to the last.
func (m *Model) stepView(dir int) {
	es := m.visibleEntries()
	if len(es) == 0 {
		m.switchView(viewHome)
		return
	}
	for i, e := range es {
		if m.entryOn(e) {
			m.goEntry(es[(i+len(es)+dir)%len(es)])
			return
		}
	}
	if dir > 0 {
		m.goEntry(es[0])
	} else {
		m.goEntry(es[len(es)-1])
	}
}

// svName is the tab text of a saved view.
func (m *Model) svName(uuid string) string {
	if v, ok := m.savedByUUID(uuid); ok {
		return v.Name
	}
	return "view"
}

// refreshSavedActions gives every shown saved view an action in the palette and, while there is a
// number key left, that key. The built-in views take the first places (patchModuleActions).
func (m *Model) refreshSavedActions() {
	kept := m.actions[:0]
	for _, a := range m.actions {
		if !strings.HasPrefix(a.id, "view-saved-") {
			kept = append(kept, a)
		}
	}
	m.actions = kept
	base := len(m.mods.Visible())
	shown := map[string]int{}
	for i, v := range m.svShown() {
		shown[v.UUID] = base + i + 1
	}
	for _, v := range m.svShown() { // a hidden view is out of the palette like a hidden module
		uuid := v.UUID
		a := action{
			id: "view-saved-" + uuid, title: "View: " + v.Name, group: "Views", layer: layerFramework,
			desc: "open the saved " + v.Kind + " view " + v.Name + ": " + query.Describe(v.Kind, v.Query, nil),
			run:  func(m *Model) tea.Cmd { m.openSaved(uuid); return nil },
		}
		if pos, ok := shown[uuid]; ok {
			if k := modules.Key(pos); k != "" {
				a.keys, a.hint = []string{k}, k
			}
		}
		m.actions = append(m.actions, a)
	}
	annotateActions(m.actions)
}

// svKeyOf is the number key of a shown saved view, "" when it has none.
func (m *Model) svKeyOf(uuid string) string {
	base := len(m.mods.Visible())
	for i, v := range m.svShown() {
		if v.UUID == uuid {
			return modules.Key(base + i + 1)
		}
	}
	return ""
}

// --- rows ---

// savedRowsIfActive builds the rows of the saved view on screen. The second result is false when no
// saved view shapes the current view.
func (m *Model) savedRowsIfActive() ([]row, bool, error) {
	if !m.svActive() {
		return nil, false, nil
	}
	q := m.svQuery()
	kind := m.sv.v.Kind
	qrows, err := m.svr.RunQuery(m.ctx, kind, q, m.now())
	if err != nil {
		return nil, true, err
	}
	// The list builders of the kind give the rows, the query gives the order and the groups.
	byID := map[int64]row{}
	switch kind {
	case query.KindPerson:
		for _, p := range m.people {
			byID[p.ID] = personRowOf(p)
		}
	case query.KindOrganization:
		rs, err := m.orgRows()
		if err != nil {
			return nil, true, err
		}
		for _, r := range rs {
			byID[r.orgID] = r
		}
	case query.KindDeal:
		rs, err := m.dealRows()
		if err != nil {
			return nil, true, err
		}
		for _, r := range rs {
			byID[r.dealID] = r
		}
	case query.KindTask:
		rs, err := m.openTaskRows()
		if err != nil {
			return nil, true, err
		}
		for _, r := range rs {
			byID[r.taskID] = r
		}
	}
	terms := strings.Fields(strings.ToLower(m.query))
	var kept []query.Row
	rowOf := map[int64]row{}
	counts := map[string]int{}
	for _, qr := range qrows {
		var r row
		switch kind {
		case query.KindPerson, query.KindOrganization, query.KindDeal, query.KindTask:
			var ok bool
			if r, ok = byID[qr.ID]; !ok {
				continue // a completed task is not in the Tasks list rows, same as the Tasks view
			}
		case query.KindTag:
			r = row{tagID: qr.ID, tag: qr.Label, title: cleanCell(qr.Label), meta: qr.Meta}
		case query.KindNote:
			r = row{noteID: qr.ID, title: cleanCell(qr.Label), sub: cleanCell(qr.Sub), meta: shortDate(qr.Meta)}
		}
		if len(terms) > 0 && !m.svMatches(kind, qr.ID, r, terms) {
			continue
		}
		kept = append(kept, qr)
		rowOf[qr.ID] = r
		counts[qr.Group]++
	}
	g := groupRows(kept, func(qr query.Row) []groupHead {
		if q.GroupBy == "" {
			return nil
		}
		return []groupHead{{key: qr.Group, title: qr.GroupLabel, meta: fmt.Sprint(counts[qr.Group]), rule: true}}
	}, func(qr query.Row) row { return rowOf[qr.ID] })
	return g.rows, true, nil
}

// svMatches is the search of the bar on a saved view: every word must appear in the row.
func (m *Model) svMatches(kind string, id int64, r row, terms []string) bool {
	if kind == query.KindPerson {
		for _, p := range m.people {
			if p.ID == id {
				return matchesAlias(p, terms, m.aliasTable())
			}
		}
		return false
	}
	hay := strings.ToLower(r.title + " " + r.sub + " " + r.meta)
	for _, t := range terms {
		if !strings.Contains(hay, strings.TrimPrefix(t, "#")) {
			return false
		}
	}
	return true
}

// navUnit is the unit of ctrl+up and ctrl+down: a saved view with groups jumps by group.
func (m *Model) navUnit() jumpUnit {
	if m.svActive() {
		if m.svQuery().GroupBy != "" {
			return unitGroup
		}
		return unitPage
	}
	return navOf(m.view).unit
}

// svCountText is the right side of the tab bar in a saved view.
func (m *Model) svCountText() string {
	n := 0
	for _, r := range m.rows {
		if !r.skip {
			n++
		}
	}
	return fmt.Sprintf("%d %s", n, m.sv.v.Kind+"s")
}

// svEmptyText is what an empty saved view says.
func (m *Model) svEmptyText() string {
	return "Nothing matches " + m.sv.v.Name + ". ctrl+o shows or removes its filter, edit changes it."
}

// buildNoteListForm is the detail of a note row of a saved note view.
func (m *Model) buildNoteListForm(id int64) []formRow {
	m.formTitle = ""
	n, err := m.ir.FindByID(m.ctx, id)
	if err != nil {
		return nil
	}
	m.formTitle = n.Type + " " + shortDate(n.OccurredAt)
	var rows []formRow
	if n.Subject != nil && *n.Subject != "" {
		rows = append(rows, formRow{kind: rkInfo, label: "subject", value: *n.Subject})
	}
	if n.Content != nil {
		rows = append(rows, formRow{kind: rkInfo, label: "text", value: *n.Content})
	}
	return rows
}

// --- capturing the current view ---

// captureView is the kind and query of the list on screen, for Save current view as. A saved view on
// screen gives its query with the session tweaks. A built-in list gives its kind, its sort and its
// search. ok is false for a screen that is not a list of one kind.
func (m *Model) captureView() (kind string, q query.Query, ok bool) {
	if m.svActive() {
		kind, q = m.sv.v.Kind, m.svQuery()
		if strings.TrimSpace(m.query) != "" {
			q.Search = strings.TrimSpace(q.Search + " " + m.query)
		}
		return kind, q, true
	}
	kind, ok = svKindOf(m.view)
	if !ok || m.view == viewNotes {
		return "", q, false
	}
	switch m.view {
	case viewPeople, viewOrgs:
		if m.sortOf(m.view) == "recent" {
			q.Sort = []query.Sort{{Field: "updated", Dir: "desc"}}
		} else {
			q.Sort = []query.Sort{{Field: "name"}}
		}
	}
	if m.view == viewPeople {
		q.Filter, q.Search = m.searchToQuery(m.query)
	} else if strings.TrimSpace(m.query) != "" {
		q.Search = strings.TrimSpace(m.query)
	}
	return kind, q, true
}

// searchToQuery turns what was typed in the People search into conditions and words: #tag is a tag
// condition, !high a priority, the rest words that every person must contain.
func (m *Model) searchToQuery(text string) ([]query.Condition, string) {
	var conds []query.Condition
	var words []string
	for _, t := range strings.Fields(text) {
		switch {
		case strings.HasPrefix(t, "#") && len(t) > 1:
			conds = append(conds, query.Condition{Field: "tag", Op: query.OpHas, Values: []string{t[1:]}})
		case strings.HasPrefix(t, "!") && len(t) > 1:
			if pr, ok := shortsyntax.ParsePriority(t[1:]); ok {
				conds = append(conds, query.Condition{Field: "tier", Op: query.OpIs, Values: []string{fmt.Sprint(shortsyntax.TierOf(pr))}})
				continue
			}
			words = append(words, t)
		default:
			words = append(words, t)
		}
	}
	return conds, strings.Join(words, " ")
}

// startSaveView asks for the name of the new view in the footer.
func (m *Model) startSaveView() tea.Cmd {
	if _, _, ok := m.captureView(); !ok {
		m.setStatus("this screen is not a list of one kind of object, open People, Organizations, Deals, Tasks or Tags first", true)
		return nil
	}
	m.startInput("sview-name", "save view as: ", "")
	return m.input.Focus()
}

// runSaveViewName creates the view with the name that was typed.
func (m *Model) runSaveViewName(name string) {
	kind, q, ok := m.captureView()
	if !ok {
		return
	}
	v, err := m.hCreateSaved(repo.SavedViewInput{Name: name, Kind: kind, Query: q, ShowInTabs: true})
	if err != nil {
		m.setStatus(err.Error(), true)
		m.pendingCmd = m.showToast(err.Error(), true)
		return
	}
	m.setStatus(fmt.Sprintf("saved view %q (ctrl+z undoes)", v.Name), false)
	m.pendingCmd = m.showToast(fmt.Sprintf("saved view %q · v shows it, ctrl+z undoes", v.Name), false)
}

// --- writes with undo ---

func (m *Model) savedChanged() {
	m.loadSaved()
	m.report(m.rebuild(true))
}

func (m *Model) hCreateSaved(in repo.SavedViewInput) (*repo.SavedView, error) {
	v, err := m.svr.Create(m.ctx, in)
	if err != nil {
		return nil, err
	}
	id := v.ID
	m.record("save view "+truncateLabel(v.Name),
		func() error { err := m.svr.Delete(m.ctx, id); m.savedChanged(); return err },
		func() error { _, err := m.svr.Restore(m.ctx, id); m.savedChanged(); return err })
	m.savedChanged()
	return v, nil
}

func (m *Model) hDeleteSaved(id int64) error {
	v, err := m.svr.FindByID(m.ctx, id)
	if err != nil {
		return err
	}
	if err := m.svr.Delete(m.ctx, id); err != nil {
		return err
	}
	m.record("delete view "+truncateLabel(v.Name),
		func() error { _, err := m.svr.Restore(m.ctx, id); m.savedChanged(); return err },
		func() error { err := m.svr.Delete(m.ctx, id); m.savedChanged(); return err })
	m.savedChanged()
	return nil
}

func (m *Model) hRestoreSaved(id int64) error {
	v, err := m.svr.FindByID(m.ctx, id)
	if err != nil {
		return err
	}
	if _, err := m.svr.Restore(m.ctx, id); err != nil {
		return err
	}
	m.record("restore view "+truncateLabel(v.Name),
		func() error { err := m.svr.Delete(m.ctx, id); m.savedChanged(); return err },
		func() error { _, err := m.svr.Restore(m.ctx, id); m.savedChanged(); return err })
	m.savedChanged()
	return nil
}

// hUpdateSaved applies a patch and records how to put the old values back.
func (m *Model) hUpdateSaved(id int64, p repo.SavedViewPatch) (*repo.SavedView, error) {
	before, err := m.svr.FindByID(m.ctx, id)
	if err != nil {
		return nil, err
	}
	after, err := m.svr.Update(m.ctx, id, p)
	if err != nil {
		return nil, err
	}
	back := repo.SavedViewPatch{Name: &before.Name, Kind: &before.Kind, Query: &before.Query, ShowInTabs: &before.ShowInTabs}
	forth := repo.SavedViewPatch{Name: &after.Name, Kind: &after.Kind, Query: &after.Query, ShowInTabs: &after.ShowInTabs}
	m.record("edit view "+truncateLabel(before.Name),
		func() error { _, err := m.svr.Update(m.ctx, id, back); m.savedChanged(); return err },
		func() error { _, err := m.svr.Update(m.ctx, id, forth); m.savedChanged(); return err })
	m.savedChanged()
	return after, nil
}

// svMove moves a view among the views in its state (shown or hidden), so a view never jumps across
// the divider of the Views screen: it steps until it has passed one view of its own state.
func (m *Model) svMove(id int64, d int) {
	cur, ok := m.savedByID(id)
	if !ok {
		return
	}
	for range m.svs {
		idx := -1
		for j, v := range m.svs {
			if v.ID == id {
				idx = j
			}
		}
		next := idx + d
		if idx < 0 || next < 0 || next >= len(m.svs) {
			break
		}
		same := m.svs[next].ShowInTabs == cur.ShowInTabs
		if moved, err := m.svr.Move(m.ctx, id, d); err != nil || !moved {
			break
		}
		m.loadSaved()
		if same {
			break
		}
	}
}

// --- the view options of a saved view (ctrl+o) ---

// savedViewOptions are the options of the saved view on screen: its own filter, sort and group, which
// the user can change for this session without saving (the stored view is not touched).
func (m *Model) savedViewOptions() []viewOption {
	st := m.sv
	kind := st.v.Kind
	stored := st.v.Query
	lbl := func(prefix, text string) string {
		if text == "" {
			return prefix
		}
		return prefix + ": " + text
	}
	filter := viewOption{
		id: "filter", label: "Filter",
		choices: []optChoice{{"saved", lbl("saved filter", query.Describe(kind, stored, nil))}, {"off", "no filter (this session)"}},
		get: func(m *Model) string {
			if m.sv.filterOff {
				return "off"
			}
			return "saved"
		},
		set: func(m *Model, v string) { m.sv.filterOff = v == "off" },
	}
	sortChoices := []optChoice{{"", lbl("saved sort", orderOrDefault(stored))}, {"none", "default order"}}
	groupChoices := []optChoice{{"", lbl("saved group", groupText(stored))}, {"none", "no group"}}
	for _, f := range query.Fields(kind) {
		if f.Sortable() {
			sortChoices = append(sortChoices, optChoice{f.Key, f.Label}, optChoice{f.Key + ":desc", f.Label + " (descending)"})
		}
		if f.Groupable() {
			groupChoices = append(groupChoices, optChoice{f.Key, f.Label})
		}
	}
	return []viewOption{
		filter,
		{id: "sort", label: "Sort", choices: sortChoices,
			get: func(m *Model) string { return m.sv.sortKey },
			set: func(m *Model, v string) { m.sv.sortKey = v }},
		{id: "group", label: "Group by", choices: groupChoices,
			get: func(m *Model) string { return m.sv.groupKey },
			set: func(m *Model, v string) { m.sv.groupKey = v }},
	}
}

func orderOrDefault(q query.Query) string {
	if s := query.DescribeOrder(query.Query{Sort: q.Sort}); s != "" {
		return strings.TrimPrefix(s, "sorted by ")
	}
	return "default order"
}

func groupText(q query.Query) string {
	if q.GroupBy == "" {
		return "no group"
	}
	return q.GroupBy
}

// optionsForView is the option list of the view on screen.
func (m *Model) optionsForView() []viewOption {
	if m.svActive() {
		return m.savedViewOptions()
	}
	return viewOptionsFor(m.view)
}

// --- the object registry ---

// savedViewDef registers the saved view as an object kind. Edit, Delete and Restore are generated from
// it like for every object (palette entries, undo, toasts). Their bare keys stay with the list the
// view is shown in: d, e and r work in the Views screen (modules.go). Add is the palette action Save
// current view as.
func savedViewDef() *objDef {
	return &objDef{
		kind: model.SavedViewKind, label: "view", short: "sview", group: "Views", view: viewPeople,
		can: map[verb]bool{vEdit: true, vDelete: true, vRestore: true},
		ids: map[verb]string{vEdit: "sview-edit", vDelete: "sview-delete", vRestore: "sview-restore"},
		selected: func(m *Model) int64 {
			if m.svActive() {
				return m.sv.v.ID
			}
			return 0
		},
		nameOf: func(m *Model, id int64) string {
			if v, ok := m.savedByID(id); ok {
				return truncateLabel(v.Name)
			}
			return fmt.Sprintf("view %d", id)
		},
		edit:    func(m *Model, id int64) tea.Cmd { return m.openSVEditor(id) },
		archive: func(m *Model, id int64) error { return m.hDeleteSaved(id) },
		restore: func(m *Model, id int64) error { return m.hRestoreSaved(id) },
		keyHere: func(m *Model, v verb, key string) bool { return false },
	}
}

func savedViewActions() []action {
	return []action{
		{id: "sview-add", title: "Save current view as...", group: "Views", write: true,
			desc: "save the list on screen as a view: its kind, search, filter, sort and group (ctrl+o options too). It shows in the Views screen and the tabs (same as crm views save)",
			run:  func(m *Model) tea.Cmd { return m.startSaveView() }},
	}
}
