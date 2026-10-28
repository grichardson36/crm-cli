package tui

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/mattn/go-runewidth"
)

// Organizations and Deals: advanced views (BUSINESS.md: not in the default navigation). They open
// from the palette. Add, edit, delete, restore and stage changes are palette actions with view
// keys, every change goes on the undo stack, and a delete is the same soft delete as the CLI
// (archived), so ctrl+z and the Restore verb (u) bring it back.
//
// Pattern: one line with | between fields, as the task prompt does. Edit opens the
// same line filled with the current values. An organization is name | domain | industry | size | location | website | linkedin | description | notes (the old four field form still works).
// A deal is title | value | stage | organization | person | notes. An organization or person is
// an id or a name. On an edit, a field you empty is cleared.

// The prompts come from the object registry (objects.go): the label and the fields in order.
func orgPrompt() string  { return orgDef().prompt() }
func dealPrompt() string { return dealDef().prompt() }

func (m *Model) selOrg() int64 {
	if m.view != viewOrgs {
		return 0
	}
	if r, ok := m.selected(); ok {
		return r.orgID
	}
	return 0
}

func (m *Model) selDeal() int64 {
	if m.view != viewDeals {
		return 0
	}
	if r, ok := m.selected(); ok {
		return r.dealID
	}
	return 0
}

func whenOrgSel(m *Model) bool    { return m.selOrg() != 0 }
func whenDealSel(m *Model) bool   { return m.selDeal() != 0 }
func whenOrgsView(m *Model) bool  { return m.view == viewOrgs && m.focus == focusList }
func whenDealsView(m *Model) bool { return m.view == viewDeals && m.focus == focusList }

// --- rows and detail ---

func (m *Model) orgRows() ([]row, error) {
	list, err := m.orr.FindAll(m.ctx, 0)
	if err != nil {
		return nil, err
	}
	m.orgs = map[int64]*model.Organization{}
	m.orgSimilar = nil
	similar := m.similarOrgs()
	if !m.sortedRecent(viewOrgs) {
		sort.SliceStable(list, func(i, j int) bool { return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name) })
	}
	rows := make([]row, 0, len(list))
	for _, o := range list {
		m.orgs[o.ID] = o
		sub := cleanCell(str(o.Industry))
		if c := similar[o.ID]; len(c) > 0 { // a near-duplicate name: space marks both, M merges
			if sub != "" {
				sub += "  "
			}
			sub += "~ " + cleanCell(c[0].name)
		}
		rows = append(rows, row{orgID: o.ID, title: cleanCell(o.Name), sub: sub, meta: cleanCell(str(o.Domain))})
	}
	return rows, nil
}

// orgNear is an organization with a near-duplicate name to another one.
type orgNear struct {
	id     int64
	name   string
	reason string
}

// similarOrgs maps an organization id to the organizations whose name is close to it (merge
// candidates, never merged on their own). It is cached until the next write.
func (m *Model) similarOrgs() map[int64][]orgNear {
	if m.orgSimilar != nil {
		return m.orgSimilar
	}
	out := map[int64][]orgNear{}
	if c, err := m.orr.FindOrgCandidates(m.ctx, 0); err == nil {
		for _, x := range c {
			out[x.AID] = append(out[x.AID], orgNear{x.BID, x.BName, x.Reason})
			out[x.BID] = append(out[x.BID], orgNear{x.AID, x.AName, x.Reason})
		}
	}
	m.orgSimilar = out
	return out
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
	return m.st.Dim.Render(fit(label, labelWidth)) + cutTo(value, max(1, m.detail.Width()-labelWidth))
}

// orgDetail and dealDetail are shown only when a page cannot be built.
func (m *Model) orgDetail(r row) string  { return m.st.Dim.Render("Nothing selected.") }
func (m *Model) dealDetail(r row) string { return m.st.Dim.Render("Nothing selected.") }

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

// orgLine is the edit line: name | domain | industry | size | location | website | linkedin | description | notes.
// It always has every field so an empty one can be told from a missing one.
func orgLine(o *model.Organization) string {
	return strings.Join([]string{o.Name, cleanCell(str(o.Domain)), cleanCell(str(o.Industry)), cleanCell(str(o.Size)), cleanCell(o.Location()),
		cleanCell(str(o.Website)), cleanCell(str(o.LinkedinURL)), cleanCell(str(o.Description)), cleanCell(str(o.Notes))}, " | ")
}

// orgFromLine reads an add or edit line. A line of four parts or fewer is the old form
// name | domain | industry | notes, the native fields are then absent (nil).
func orgFromLine(line string) (o model.Organization, full bool) {
	if strings.Count(line, "|") < 4 {
		f := fields(line, 4)
		return model.Organization{Name: f[0], Domain: optional(f[1]), Industry: optional(f[2]), Notes: optional(f[3])}, false
	}
	f := fields(line, 9)
	o = model.Organization{Name: f[0], Domain: optional(f[1]), Industry: optional(f[2]), Size: optional(f[3]),
		Website: optional(f[5]), LinkedinURL: optional(f[6]), Description: optional(f[7]), Notes: optional(f[8])}
	o.City, o.Region, o.Country = model.SplitLocation(f[4])
	return o, true
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
	var orgs, people []string
	if d.OrgID != nil {
		orgs = append(orgs, m.orgName(*d.OrgID))
	}
	for _, id := range d.OrgIDs {
		if d.OrgID == nil || id != *d.OrgID {
			orgs = append(orgs, m.orgName(id))
		}
	}
	for _, id := range d.PersonIDs {
		people = append(people, m.personLabel(id))
	}
	org, person := strings.Join(orgs, ", "), strings.Join(people, ", ")
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
	if errors.Is(err, errPromptOpened) {
		return
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
	parsed, _ := orgFromLine(line)
	if parsed.Name == "" {
		return "", fmt.Errorf("an organization needs a name")
	}
	in := model.CreateOrgInput{Name: parsed.Name, Domain: parsed.Domain, Industry: parsed.Industry, Notes: parsed.Notes,
		OrgNativeFields: model.OrgNativeFields{Website: parsed.Website, Size: parsed.Size, City: parsed.City, Region: parsed.Region,
			Country: parsed.Country, LinkedinURL: parsed.LinkedinURL, Description: parsed.Description}}
	show := func(o *model.Organization) {
		if m.view == viewOrgs {
			m.afterWrite()
			m.cursorOn(func(r row) bool { return r.orgID == o.ID })
		}
	}
	if dup, reason, err := m.orr.FindDuplicate(m.ctx, in.Name, in.Domain); err != nil {
		return "", err
	} else if dup != nil {
		m.askOrgDuplicate(dup, reason, func(o *model.Organization) {
			show(o)
			m.setStatus("using the existing organization "+o.Name, false)
		}, func() {
			in.AllowDuplicate = true
			o, err := m.hCreateOrg(in)
			if err != nil {
				m.setStatus(err.Error(), true)
				return
			}
			show(o)
			m.afterWrite()
			m.setStatus("organization added: "+o.Name+" (ctrl+z undoes)", false)
		})
		return "", errPromptOpened
	}
	o, err := m.hCreateOrg(in)
	if err != nil {
		return "", err
	}
	show(o)
	return "organization added: " + o.Name + " (ctrl+z undoes)", nil
}

func (m *Model) saveOrgEdit(id int64, line string) (string, error) {
	after, full := orgFromLine(line)
	if after.Name == "" {
		return "", fmt.Errorf("an organization needs a name")
	}
	if !full { // the old four field line leaves the native fields as they are
		before, err := m.orr.FindByID(m.ctx, id)
		if err != nil {
			return "", err
		}
		after.Website, after.Size, after.City, after.Region, after.Country = before.Website, before.Size, before.City, before.Region, before.Country
		after.LinkedinURL, after.Description = before.LinkedinURL, before.Description
	}
	if after.LogoPath == nil {
		if before, err := m.orr.FindByID(m.ctx, id); err == nil {
			after.LogoPath = before.LogoPath // the line has no logo field
		}
	}
	if err := m.hOverwriteOrg(id, &after); err != nil {
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
	// An empty stage becomes the first open stage in the repository.
	stage, err := m.resolveStage(f[2])
	if err != nil {
		return "", err
	}
	// The organization and person segments hold several names: pipePrepare resolved them and
	// afterCreate links them, the first organization as the primary one (pipelink.go).
	in := model.CreateDealInput{Title: f[0], Value: val, Stage: stage, Notes: optional(f[5])}
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
	after.PersonIDs, after.OrgIDs = nil, nil
	after.Title = f[0]
	if after.Value, err = parseMoney(f[1]); err != nil {
		return "", err
	}
	if f[2] != "" {
		if after.Stage, err = m.resolveStage(f[2]); err != nil {
			return "", err
		}
	}
	after.Notes = optional(f[5])
	// The fields and the two links undo as one step.
	m.beginGroup()
	defer m.endGroup("edit deal " + truncateLabel(before.Title))
	if err := m.hOverwriteDeal(id, &after); err != nil {
		return "", err
	}
	deal := repo.Ref{Kind: "deal", ID: id}
	// The organization and person segments are the whole set of links: names taken out are unlinked.
	links := m.pipeLinks
	m.pipeLinks, m.pipeObj = nil, ""
	m.pipeRes = map[string]pendingLink{}
	for _, kind := range []string{"organization", "person"} {
		if err := m.syncLinks(deal, kind, links); err != nil {
			return "", err
		}
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

func (m *Model) dealTitle(id int64) string {
	if d, err := m.dlr.FindByID(m.ctx, id); err == nil {
		return d.Title
	}
	return fmt.Sprintf("deal %d", id)
}

// --- recorded writes (undo and redo) ---

func orgSame(a, b *model.Organization) bool {
	return a.Name == b.Name && str(a.Domain) == str(b.Domain) && str(a.Industry) == str(b.Industry) && str(a.Notes) == str(b.Notes) &&
		str(a.Website) == str(b.Website) && str(a.Size) == str(b.Size) && str(a.City) == str(b.City) && str(a.Region) == str(b.Region) &&
		str(a.Country) == str(b.Country) && str(a.LinkedinURL) == str(b.LinkedinURL) && str(a.Description) == str(b.Description) &&
		str(a.LogoPath) == str(b.LogoPath)
}

func dealSame(a, b *model.Deal) bool {
	return a.Title == b.Title && a.Stage == b.Stage && str(a.Notes) == str(b.Notes) && str(a.ClosedAt) == str(b.ClosedAt) &&
		eqFloat(a.Value, b.Value)
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
	m.afterCreate("organization", id)
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
	m.afterCreate("deal", id)
	return d, nil
}

func (m *Model) hOverwriteDeal(id int64, after *model.Deal) error {
	before, err := m.dlr.FindByID(m.ctx, id)
	if err != nil {
		return err
	}
	// closed_at follows the stage in every door (X6): set when it becomes won or lost, cleared on reopen.
	if before.Stage != after.Stage {
		// The outcome of the stage decides, not its name (patterns/deal-stages).
		switch {
		case m.stageClosed(after.Stage):
			now := m.now().UTC().Format("2006-01-02 15:04:05")
			after.ClosedAt = &now
		default:
			after.ClosedAt = nil
		}
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
