package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Organizations and Deals: advanced views (BUSINESS.md: not in the default navigation). They open
// from the palette. Add, edit, delete, restore and stage changes are palette actions with view
// keys, every change goes on the undo stack, and a delete is the same soft delete as the CLI
// (archived), so ctrl+z and the Restore verb (u) bring it back.
//
// Pattern: one line with | between fields, as the task prompt does. Edit opens the
// same line filled with the current values. An organization is name | domain | industry | notes.
// A deal is title | value | stage | organization | person | notes. An organization or person is
// an id or a name. On an edit, a field you empty is cleared.

// The prompts come from the object registry (objects.go): the label and the fields in order.
func orgPrompt() string  { return orgDef().prompt() }
func dealPrompt() string { return dealDef().prompt() }

// nextView is the view after v in the number-key cycle. Advanced views are outside the cycle: ]
// goes to Home from them and [ to Sync.
func nextView(v viewKind, dir int) viewKind {
	if v >= numViews {
		if dir > 0 {
			return viewHome
		}
		return numViews - 1
	}
	return (v + viewKind(numViews) + viewKind(dir)) % numViews
}

func notAdvancedView(m *Model) bool { return m.view < numViews }

func (m *Model) selOrg() int64 {
	if m.view != viewOrgs || m.focus != focusList {
		return 0
	}
	if r, ok := m.selected(); ok {
		return r.orgID
	}
	return 0
}

func (m *Model) selDeal() int64 {
	if m.view != viewDeals || m.focus != focusList {
		return 0
	}
	if r, ok := m.selected(); ok {
		return r.dealID
	}
	return 0
}

func whenOrgSel(m *Model) bool       { return m.selOrg() != 0 }
func whenDealSel(m *Model) bool      { return m.selDeal() != 0 }
func whenOrgsView(m *Model) bool     { return m.view == viewOrgs && m.focus == focusList }
func whenDealsView(m *Model) bool    { return m.view == viewDeals && m.focus == focusList }

// advancedViewActions open the advanced views from the palette. Their verbs (Add, Edit, Delete,
// Restore, Link) come from the object registry.
func advancedViewActions() []action {
	return []action{
		{id: "view-orgs", title: "Organizations view", group: "Organizations", desc: "advanced view: companies and groups. a adds, enter edits, d deletes, u restores, L links (same as crm org)",
			run: func(m *Model) tea.Cmd { m.switchView(viewOrgs); return nil }},
		{id: "view-deals", title: "Deals view", group: "Deals", desc: "advanced view: deals with stage and value. a adds, enter edits, s sets the stage, d deletes, u restores, L links (same as crm deal)",
			run: func(m *Model) tea.Cmd { m.switchView(viewDeals); return nil }},
	}
}

// --- rows and detail ---

func (m *Model) orgRows() ([]row, error) {
	list, err := m.orr.FindAll(m.ctx, 0)
	if err != nil {
		return nil, err
	}
	m.orgs = map[int64]*model.Organization{}
	rows := make([]row, 0, len(list))
	for _, o := range list {
		m.orgs[o.ID] = o
		rows = append(rows, row{orgID: o.ID, title: cleanCell(o.Name), sub: cleanCell(str(o.Industry)), meta: cleanCell(str(o.Domain))})
	}
	return rows, nil
}

func (m *Model) dealRows() ([]row, error) {
	list, err := m.dlr.FindAll(m.ctx, model.DealFilters{})
	if err != nil {
		return nil, err
	}
	m.deals = map[int64]*model.Deal{}
	rows := make([]row, 0, len(list))
	for _, d := range list {
		m.deals[d.ID] = d
		sub := d.Stage
		if d.OrgID != nil {
			sub += "  " + m.orgName(*d.OrgID)
		}
		rows = append(rows, row{dealID: d.ID, title: cleanCell(d.Title), sub: cleanCell(sub), meta: money(d.Value)})
	}
	return rows, nil
}

func (m *Model) orgName(id int64) string {
	if o, err := m.orr.FindByID(m.ctx, id); err == nil {
		return o.Name
	}
	return fmt.Sprintf("organization %d", id)
}

// money formats a deal value for a list: 12,500 or 99.50. Empty when there is no value.
func money(v *float64) string {
	if v == nil {
		return ""
	}
	s := strconv.FormatFloat(*v, 'f', 2, 64)
	if *v == float64(int64(*v)) {
		s = strconv.FormatInt(int64(*v), 10)
	}
	whole, frac, _ := strings.Cut(s, ".")
	neg := strings.HasPrefix(whole, "-")
	whole = strings.TrimPrefix(whole, "-")
	var b strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	out := b.String()
	if neg {
		out = "-" + out
	}
	if frac != "" {
		out += "." + frac
	}
	return out
}

func cutTo(s string, w int) string { return runewidth.Truncate(s, w, "…") }

func (m *Model) detailLine(label, value string) string {
	return m.st.Dim.Render(fit(label, labelWidth)) + cutTo(value, max(1, m.detail.Width-labelWidth))
}

func (m *Model) orgDetail(r row) string {
	o := m.orgs[r.orgID]
	if o == nil {
		return m.st.Dim.Render("Nothing selected.")
	}
	var b strings.Builder
	b.WriteString(m.st.Name.Render(cutTo(cleanCell(o.Name), max(1, m.detail.Width))) + m.st.Dim.Render(fmt.Sprintf("  #%d", o.ID)) + "\n\n")
	for _, kv := range [][2]string{{"domain", str(o.Domain)}, {"industry", str(o.Industry)}, {"notes", cleanCell(str(o.Notes))}} {
		if kv[1] != "" {
			b.WriteString(m.detailLine(kv[0], kv[1]) + "\n")
		}
	}
	if people, err := m.orr.FindPeople(m.ctx, o.ID); err == nil && len(people) > 0 {
		b.WriteString("\n" + m.st.Head.Render("People") + "\n")
		for _, p := range people {
			b.WriteString(cutTo(fullName(p), max(1, m.detail.Width)) + "\n")
		}
	}
	oid := o.ID
	if deals, err := m.dlr.FindAll(m.ctx, model.DealFilters{OrgID: &oid}); err == nil && len(deals) > 0 {
		b.WriteString("\n" + m.st.Head.Render("Deals") + "\n")
		for _, d := range deals {
			b.WriteString(cutTo(strings.TrimSpace(d.Title+"  "+d.Stage+"  "+money(d.Value)), max(1, m.detail.Width)) + "\n")
		}
	}
	b.WriteString("\n" + m.st.Dim.Render("enter edits, d deletes, a adds, L links"))
	return strings.TrimRight(b.String(), "\n")
}

func (m *Model) dealDetail(r row) string {
	d := m.deals[r.dealID]
	if d == nil {
		return m.st.Dim.Render("Nothing selected.")
	}
	var b strings.Builder
	b.WriteString(m.st.Name.Render(cutTo(cleanCell(d.Title), max(1, m.detail.Width))) + m.st.Dim.Render(fmt.Sprintf("  #%d", d.ID)) + "\n\n")
	b.WriteString(m.detailLine("stage", d.Stage) + "\n")
	if d.Value != nil {
		b.WriteString(m.detailLine("value", money(d.Value)) + "\n")
	}
	if d.OrgID != nil {
		b.WriteString(m.detailLine("organization", m.orgName(*d.OrgID)) + "\n")
	}
	if d.PersonID != nil {
		b.WriteString(m.detailLine("person", m.personLabel(*d.PersonID)) + "\n")
	}
	if d.ClosedAt != nil {
		b.WriteString(m.detailLine("closed", *d.ClosedAt) + "\n")
	}
	if d.Notes != nil && *d.Notes != "" {
		b.WriteString(m.detailLine("notes", cleanCell(*d.Notes)) + "\n")
	}
	b.WriteString("\n" + m.st.Dim.Render("enter edits, s sets the stage, d deletes, a adds, L links"))
	return strings.TrimRight(b.String(), "\n")
}

// --- prompts ---

func (m *Model) startOrgAdd() tea.Cmd {
	m.lineID = 0
	m.startInput("org-add", orgPrompt(), "")
	return m.input.Focus()
}

func (m *Model) startOrgEdit(id int64) tea.Cmd {
	o, err := m.orr.FindByID(m.ctx, id)
	if err != nil {
		m.setStatus(err.Error(), true)
		return nil
	}
	m.lineID = id
	m.startInput("org-edit", orgPrompt(), orgLine(o))
	return m.input.Focus()
}

func orgLine(o *model.Organization) string {
	return strings.TrimRight(strings.Join([]string{o.Name, cleanCell(str(o.Domain)), cleanCell(str(o.Industry)), cleanCell(str(o.Notes))}, " | "), " |")
}

func (m *Model) startDealAdd() tea.Cmd {
	m.lineID = 0
	m.startInput("deal-add", dealPrompt(), "")
	return m.input.Focus()
}

func (m *Model) startDealEdit(id int64) tea.Cmd {
	d, err := m.dlr.FindByID(m.ctx, id)
	if err != nil {
		m.setStatus(err.Error(), true)
		return nil
	}
	m.lineID = id
	org, person := "", ""
	if d.OrgID != nil {
		org = m.orgName(*d.OrgID)
	}
	if d.PersonID != nil {
		person = m.personLabel(*d.PersonID)
	}
	line := strings.Join([]string{d.Title, plainNumber(d.Value), d.Stage, org, person, cleanCell(str(d.Notes))}, " | ")
	m.startInput("deal-edit", dealPrompt(), strings.TrimRight(line, " |"))
	return m.input.Focus()
}

func plainNumber(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', -1, 64)
}

// runOrgDealPrompt saves the line of an add or edit prompt. A line that cannot be saved opens
// the prompt again with the text kept.
func (m *Model) runOrgDealPrompt(kind, value string) {
	var toast string
	var err error
	switch kind {
	case "org-add":
		toast, err = m.saveOrgAdd(value)
	case "org-edit":
		toast, err = m.saveOrgEdit(m.lineID, value)
	case "deal-add":
		toast, err = m.saveDealAdd(value)
	case "deal-edit":
		toast, err = m.saveDealEdit(m.lineID, value)
	}
	if err != nil {
		prompt := orgPrompt()
		if strings.HasPrefix(kind, "deal-") {
			prompt = dealPrompt()
		}
		m.startInput(kind, prompt, value)
		m.pendingCmd = tea.Batch(m.input.Focus(), m.showToast(err.Error(), true))
		m.setStatus(err.Error(), true)
		return
	}
	m.afterWrite()
	m.setStatus(toast, false)
	m.pendingCmd = m.showToast(toast, false)
}

// fields splits a prompt line into n trimmed fields, the last one keeps any later |.
func fields(line string, n int) []string {
	parts := strings.SplitN(line, "|", n)
	for len(parts) < n {
		parts = append(parts, "")
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (m *Model) saveOrgAdd(line string) (string, error) {
	f := fields(line, 4)
	if f[0] == "" {
		return "", fmt.Errorf("an organization needs a name")
	}
	o, err := m.hCreateOrg(model.CreateOrgInput{Name: f[0], Domain: optional(f[1]), Industry: optional(f[2]), Notes: optional(f[3])})
	if err != nil {
		return "", err
	}
	if m.view == viewOrgs {
		m.afterWrite()
		m.cursorOn(func(r row) bool { return r.orgID == o.ID })
	}
	return "organization added: " + o.Name + " (ctrl+z undoes)", nil
}

func (m *Model) saveOrgEdit(id int64, line string) (string, error) {
	f := fields(line, 4)
	if f[0] == "" {
		return "", fmt.Errorf("an organization needs a name")
	}
	after := &model.Organization{Name: f[0], Domain: optional(f[1]), Industry: optional(f[2]), Notes: optional(f[3])}
	if err := m.hOverwriteOrg(id, after); err != nil {
		return "", err
	}
	return "organization saved (ctrl+z undoes)", nil
}

func (m *Model) saveDealAdd(line string) (string, error) {
	f := fields(line, 6)
	if f[0] == "" {
		return "", fmt.Errorf("a deal needs a title")
	}
	val, err := parseMoney(f[1])
	if err != nil {
		return "", err
	}
	stage := strings.ToLower(f[2])
	if stage == "" {
		stage = "lead"
	}
	if !model.ValidDealStage(stage) {
		return "", fmt.Errorf("stage must be one of: %s", strings.Join(model.DealStages, ", "))
	}
	in := model.CreateDealInput{Title: f[0], Value: val, Stage: stage, Notes: optional(f[5])}
	if in.OrgID, err = m.resolveOrg(f[3], nil); err != nil {
		return "", err
	}
	if in.PersonID, err = m.resolvePerson(f[4], nil); err != nil {
		return "", err
	}
	d, err := m.hCreateDeal(in)
	if err != nil {
		return "", err
	}
	if m.view == viewDeals {
		m.afterWrite()
		m.cursorOn(func(r row) bool { return r.dealID == d.ID })
	}
	return "deal added: " + d.Title + " (ctrl+z undoes)", nil
}

func (m *Model) saveDealEdit(id int64, line string) (string, error) {
	before, err := m.dlr.FindByID(m.ctx, id)
	if err != nil {
		return "", err
	}
	f := fields(line, 6)
	if f[0] == "" {
		return "", fmt.Errorf("a deal needs a title")
	}
	after := *before
	after.Title = f[0]
	if after.Value, err = parseMoney(f[1]); err != nil {
		return "", err
	}
	if f[2] != "" {
		after.Stage = strings.ToLower(f[2])
	}
	if !model.ValidDealStage(after.Stage) {
		return "", fmt.Errorf("stage must be one of: %s", strings.Join(model.DealStages, ", "))
	}
	if after.OrgID, err = m.resolveOrg(f[3], before.OrgID); err != nil {
		return "", err
	}
	if after.PersonID, err = m.resolvePerson(f[4], before.PersonID); err != nil {
		return "", err
	}
	after.Notes = optional(f[5])
	if err := m.hOverwriteDeal(id, &after); err != nil {
		return "", err
	}
	return "deal saved (ctrl+z undoes)", nil
}

// parseMoney reads a deal value such as 12500, $12,500 or 99.5. Empty means no value.
func parseMoney(s string) (*float64, error) {
	s = strings.TrimSpace(strings.NewReplacer("$", "", ",", "", "_", "").Replace(s))
	if s == "" {
		return nil, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, fmt.Errorf("value %q is not a number", s)
	}
	return &v, nil
}

// resolveOrg turns an id or a name into an organization id. Empty means none. The name of the
// organization the deal has now keeps it, even when other organizations share that name.
func (m *Model) resolveOrg(s string, current *int64) (*int64, error) {
	if s == "" {
		return nil, nil
	}
	if current != nil && strings.EqualFold(s, m.orgName(*current)) {
		return current, nil
	}
	if id, err := strconv.ParseInt(s, 10, 64); err == nil {
		if _, err := m.orr.FindByID(m.ctx, id); err != nil {
			return nil, fmt.Errorf("no organization with id %d", id)
		}
		return &id, nil
	}
	list, err := m.orr.FindAll(m.ctx, 0)
	if err != nil {
		return nil, err
	}
	var exact, partial []int64
	for _, o := range list {
		switch {
		case strings.EqualFold(o.Name, s):
			exact = append(exact, o.ID)
		case strings.Contains(strings.ToLower(o.Name), strings.ToLower(s)):
			partial = append(partial, o.ID)
		}
	}
	return pickOne("organization", s, exact, partial)
}

func (m *Model) resolvePerson(s string, current *int64) (*int64, error) {
	if s == "" {
		return nil, nil
	}
	if current != nil && strings.EqualFold(s, m.personLabel(*current)) {
		return current, nil
	}
	if id, err := strconv.ParseInt(s, 10, 64); err == nil {
		if _, err := m.pr.FindByID(m.ctx, id); err != nil {
			return nil, fmt.Errorf("no person with id %d", id)
		}
		return &id, nil
	}
	var exact, partial []int64
	for _, p := range m.people {
		switch {
		case strings.EqualFold(p.Name, s):
			exact = append(exact, p.ID)
		case strings.Contains(strings.ToLower(p.Name), strings.ToLower(s)):
			partial = append(partial, p.ID)
		}
	}
	return pickOne("person", s, exact, partial)
}

func pickOne(what, s string, exact, partial []int64) (*int64, error) {
	ids := exact
	if len(ids) == 0 {
		ids = partial
	}
	switch len(ids) {
	case 0:
		return nil, fmt.Errorf("no %s matches %q", what, s)
	case 1:
		return &ids[0], nil
	}
	return nil, fmt.Errorf("%d %ss match %q, use the id", len(ids), what, s)
}

func (m *Model) pickDealStage(id int64) {
	d, err := m.dlr.FindByID(m.ctx, id)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	items := make([]overlayItem, 0, len(model.DealStages))
	for i, st := range model.DealStages {
		it := overlayItem{id: "stage-" + st, label: st, hint: strconv.Itoa(i + 1), value: st}
		if st == d.Stage {
			it.desc = "current stage"
		}
		items = append(items, it)
	}
	m.openPicker("Deal stage", items, func(m *Model, it overlayItem) {
		if it.value == d.Stage {
			return
		}
		after := *d
		after.Stage = it.value
		if err := m.hOverwriteDeal(id, &after); err != nil {
			m.setStatus(err.Error(), true)
			return
		}
		m.afterWrite()
		m.setStatus("deal moved to "+it.value+" (ctrl+z undoes)", false)
		m.pendingCmd = m.showToast("deal moved to "+it.value, false)
	})
	m.ov.digits = true
}

func (m *Model) dealTitle(id int64) string {
	if d, err := m.dlr.FindByID(m.ctx, id); err == nil {
		return d.Title
	}
	return fmt.Sprintf("deal %d", id)
}

// --- recorded writes (undo and redo) ---

func orgSame(a, b *model.Organization) bool {
	return a.Name == b.Name && str(a.Domain) == str(b.Domain) && str(a.Industry) == str(b.Industry) && str(a.Notes) == str(b.Notes)
}

func dealSame(a, b *model.Deal) bool {
	return a.Title == b.Title && a.Stage == b.Stage && str(a.Notes) == str(b.Notes) && str(a.ClosedAt) == str(b.ClosedAt) &&
		eqInt(a.PersonID, b.PersonID) && eqInt(a.OrgID, b.OrgID) && eqFloat(a.Value, b.Value)
}

func eqInt(a, b *int64) bool     { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
func eqFloat(a, b *float64) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }

func (m *Model) hCreateOrg(in model.CreateOrgInput) (*model.Organization, error) {
	o, err := m.orr.Create(m.ctx, in)
	if err != nil {
		return nil, err
	}
	id := o.ID
	m.record("add organization "+truncateLabel(o.Name),
		func() error { return m.orr.Archive(m.ctx, id) },
		func() error { return m.orr.Restore(m.ctx, id) })
	return o, nil
}

func (m *Model) hOverwriteOrg(id int64, after *model.Organization) error {
	before, err := m.orr.FindByID(m.ctx, id)
	if err != nil {
		return err
	}
	if orgSame(before, after) {
		return nil
	}
	if err := m.orr.Overwrite(m.ctx, id, after); err != nil {
		return err
	}
	name := truncateLabel(before.Name)
	guard := func(expected *model.Organization) error {
		cur, err := m.orr.FindByID(m.ctx, id)
		if err != nil {
			return staleError{name + " is gone or deleted"}
		}
		if !orgSame(cur, expected) {
			return staleError{name + " changed since"}
		}
		return nil
	}
	m.record("edit organization "+name,
		func() error {
			if err := guard(after); err != nil {
				return err
			}
			return m.orr.Overwrite(m.ctx, id, before)
		},
		func() error {
			if err := guard(before); err != nil {
				return err
			}
			return m.orr.Overwrite(m.ctx, id, after)
		})
	return nil
}

func (m *Model) hArchiveOrg(id int64) error {
	name := truncateLabel(m.orgName(id))
	if err := m.orr.Archive(m.ctx, id); err != nil {
		return err
	}
	m.record("delete organization "+name,
		func() error { return m.orr.Restore(m.ctx, id) },
		func() error { return m.orr.Archive(m.ctx, id) })
	return nil
}

func (m *Model) hRestoreOrg(id int64) error {
	if err := m.orr.Restore(m.ctx, id); err != nil {
		return err
	}
	m.record("restore organization "+truncateLabel(m.orgName(id)),
		func() error { return m.orr.Archive(m.ctx, id) },
		func() error { return m.orr.Restore(m.ctx, id) })
	return nil
}

func (m *Model) hCreateDeal(in model.CreateDealInput) (*model.Deal, error) {
	d, err := m.dlr.Create(m.ctx, in)
	if err != nil {
		return nil, err
	}
	id := d.ID
	m.record("add deal "+truncateLabel(d.Title),
		func() error { return m.dlr.Archive(m.ctx, id) },
		func() error { return m.dlr.Restore(m.ctx, id) })
	return d, nil
}

func (m *Model) hOverwriteDeal(id int64, after *model.Deal) error {
	before, err := m.dlr.FindByID(m.ctx, id)
	if err != nil {
		return err
	}
	if dealSame(before, after) {
		return nil
	}
	if err := m.dlr.Overwrite(m.ctx, id, after); err != nil {
		return err
	}
	title := truncateLabel(before.Title)
	label := "edit deal " + title
	if before.Stage != after.Stage && before.Title == after.Title && eqFloat(before.Value, after.Value) {
		label = "move deal " + title + " to " + after.Stage
	}
	guard := func(expected *model.Deal) error {
		cur, err := m.dlr.FindByID(m.ctx, id)
		if err != nil {
			return staleError{title + " is gone or deleted"}
		}
		if !dealSame(cur, expected) {
			return staleError{title + " changed since"}
		}
		return nil
	}
	m.record(label,
		func() error {
			if err := guard(after); err != nil {
				return err
			}
			return m.dlr.Overwrite(m.ctx, id, before)
		},
		func() error {
			if err := guard(before); err != nil {
				return err
			}
			return m.dlr.Overwrite(m.ctx, id, after)
		})
	return nil
}

func (m *Model) hArchiveDeal(id int64) error {
	title := truncateLabel(m.dealTitle(id))
	if err := m.dlr.Archive(m.ctx, id); err != nil {
		return err
	}
	m.record("delete deal "+title,
		func() error { return m.dlr.Restore(m.ctx, id) },
		func() error { return m.dlr.Archive(m.ctx, id) })
	return nil
}

func (m *Model) hRestoreDeal(id int64) error {
	if err := m.dlr.Restore(m.ctx, id); err != nil {
		return err
	}
	m.record("restore deal "+truncateLabel(m.dealTitle(id)),
		func() error { return m.dlr.Archive(m.ctx, id) },
		func() error { return m.dlr.Restore(m.ctx, id) })
	return nil
}
