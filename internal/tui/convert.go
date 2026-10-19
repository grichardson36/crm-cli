package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// The company review (owner decision Q21): company text becomes organizations in passes, never by
// itself. A row is a group of people whose text normalizes the same way. Keys stage a decision for the
// group (c create, l link, e rename then create, s skip, x clear), A links every exact match after a
// confirm, enter applies everything staged as ONE undo step. See patterns/convert-text-to-records-in-review-passes.

func whenConvertView(m *Model) bool { return m.view == viewConvert && m.focus == focusList }

func (m *Model) selConvert() (repo.ConvertGroup, bool) {
	if m.view != viewConvert {
		return repo.ConvertGroup{}, false
	}
	r, ok := m.selected()
	if !ok || r.convKey == "" {
		return repo.ConvertGroup{}, false
	}
	for _, g := range m.convGroups {
		if g.Key == r.convKey {
			return g, true
		}
	}
	return repo.ConvertGroup{}, false
}

func whenConvertGroup(m *Model) bool { _, ok := m.selConvert(); return ok && m.focus == focusList }

func whenConvertStaged(m *Model) bool { return whenConvertView(m) && len(m.convStaged) > 0 }

func whenConvertExact(m *Model) bool {
	if !whenConvertView(m) {
		return false
	}
	for _, g := range m.convGroups {
		if g.Exact() {
			return true
		}
	}
	return false
}

// convertActions are the review screen's verbs. They come first in the registry so their keys win
// in this view and nowhere else.
func convertActions() []action {
	return []action{
		{id: "convert-create", title: "Company review: create organization", group: "Company review", hint: "c", keys: []string{"c"},
			desc:  "stage: create an organization from this company text and link every person in the group (title becomes the role). Applied with enter. Same as crm org convert --create",
			write: true, foot: 10, footIn: inList, footLabel: "create", when: whenConvertGroup, run: func(m *Model) tea.Cmd { m.convStage(repo.ConvertCreate, 0, ""); return nil }},
		{id: "convert-link", title: "Company review: link to organization", group: "Company review", hint: "l", keys: []string{"l"},
			desc:  "stage: link the group to an existing organization (pick, or type a new name to create). Same as crm org convert --link",
			write: true, foot: 11, footIn: inList, footLabel: "link", when: whenConvertGroup, run: func(m *Model) tea.Cmd { m.convPickOrg(); return nil }},
		{id: "convert-rename", title: "Company review: rename then create", group: "Company review", hint: "e", keys: []string{"e"},
			desc:  "stage: create the organization under a name you edit first",
			write: true, foot: 12, footIn: inList, footLabel: "rename", when: whenConvertGroup, run: func(m *Model) tea.Cmd { return m.convAskName() }},
		{id: "convert-skip", title: "Company review: skip", group: "Company review", hint: "s", keys: []string{"s"},
			desc:  "stage: skip the group, it stays away until its text changes. Same as crm org convert --skip",
			write: true, foot: 13, footIn: inList, footLabel: "skip", when: whenConvertGroup, run: func(m *Model) tea.Cmd { m.convStage(repo.ConvertSkip, 0, ""); return nil }},
		{id: "convert-clear", title: "Company review: clear decision", group: "Company review", hint: "x", keys: []string{"x"},
			desc: "remove the staged decision of this group",
			foot: 14, footIn: inList, footLabel: "clear", when: func(m *Model) bool {
				g, ok := m.selConvert()
				_, staged := m.convStaged[g.Key]
				return ok && staged && m.focus == focusList
			}, run: func(m *Model) tea.Cmd {
				if g, ok := m.selConvert(); ok {
					delete(m.convStaged, g.Key)
					m.report(m.rebuild(true))
				}
				return nil
			}},
		{id: "convert-exact", title: "Company review: accept all exact matches", group: "Company review", hint: "A", keys: []string{"A"},
			desc:  "link every group that matches an existing organization exactly (or through a merge alias), after one confirm. One undo step. Same as crm org convert --accept-exact",
			write: true, foot: 15, footIn: inList, footLabel: "accept exact", when: whenConvertExact, run: func(m *Model) tea.Cmd { m.convAskExact(); return nil }},
		{id: "convert-apply", title: "Company review: apply staged decisions", group: "Company review", hint: "enter", keys: []string{"enter"},
			desc:  "apply every staged decision in one batch after a confirm. ctrl+z undoes the whole batch",
			write: true, foot: 9, footIn: inList, footLabel: "apply", when: whenConvertStaged, run: func(m *Model) tea.Cmd { m.convAskApply(); return nil }},
		{id: "convert-open", title: "Company review", group: "Organizations", hint: "C", keys: []string{"C"},
			desc: "open the company review: turn the people's company text into organizations in passes (also in the palette)",
			keyIf: func(m *Model) bool { return m.view == viewOrgs && m.focus == focusList },
			run:   func(m *Model) tea.Cmd { m.switchView(viewConvert); return nil }},
	}
}

// convRows lists the groups as rows. A staged decision shows after the count.
func (m *Model) convRows() ([]row, error) {
	groups, stats, err := m.conv.Groups(m.ctx, false)
	if err != nil {
		return nil, err
	}
	m.convGroups, m.convStats = groups, stats
	live := map[string]bool{}
	rows := make([]row, 0, len(groups))
	for _, g := range groups {
		live[g.Key] = true
		sub := fmt.Sprintf("%d people", g.Count)
		if g.Review > 0 {
			sub += ", from phone"
		}
		meta := ""
		switch {
		case g.Match != nil && g.Exact():
			meta = "= " + g.Match.Name
		case g.Match != nil:
			meta = "~ " + g.Match.Name
		case g.DomainMatch != nil:
			meta = "@ " + g.DomainMatch.Name
		}
		if d, ok := m.convStaged[g.Key]; ok {
			meta = "→ " + m.convDecisionText(d)
		}
		rows = append(rows, row{convKey: g.Key, title: cleanCell(g.Display), sub: sub, meta: cleanCell(meta)})
	}
	for k := range m.convStaged { // a group that disappeared drops its decision
		if !live[k] {
			delete(m.convStaged, k)
		}
	}
	return rows, nil
}

func (m *Model) convDecisionText(d repo.ConvertDecision) string {
	switch d.Action {
	case repo.ConvertSkip:
		return "skip"
	case repo.ConvertLink:
		return "link " + m.orgName(d.OrgID)
	}
	text := "create"
	if d.Name != "" {
		text += " " + d.Name
	}
	if g, ok := m.convGroupByKey(d.Key); ok {
		dom := d.Domain
		if dom == "" {
			dom = g.Domain
		}
		if dom != "" {
			text += " (" + dom + ")"
		}
	}
	return text
}

func (m *Model) convGroupByKey(key string) (repo.ConvertGroup, bool) {
	for _, g := range m.convGroups {
		if g.Key == key {
			return g, true
		}
	}
	return repo.ConvertGroup{}, false
}

// convStage records a decision for the group under the cursor and moves down.
func (m *Model) convStage(action string, orgID int64, name string) {
	g, ok := m.selConvert()
	if !ok {
		return
	}
	if m.convStaged == nil {
		m.convStaged = map[string]repo.ConvertDecision{}
	}
	m.convStaged[g.Key] = repo.ConvertDecision{Key: g.Key, Action: action, OrgID: orgID, Name: name}
	m.report(m.rebuild(true))
	m.step(1)
	m.setStatus(fmt.Sprintf("%d staged, enter applies them", len(m.convStaged)), false)
}

// convPickOrg picks the organization to link: the suggestion first, or a typed name to create.
func (m *Model) convPickOrg() {
	g, ok := m.selConvert()
	if !ok {
		return
	}
	list, _ := m.orr.FindAll(m.ctx, 0)
	var items []overlayItem
	if g.Match != nil {
		items = append(items, overlayItem{id: "pick", label: cleanCell(g.Match.Name), desc: "suggested (" + g.Match.Kind + ")", ref: g.Match.OrgID, value: g.Match.Name})
	}
	if g.DomainMatch != nil && (g.Match == nil || g.Match.OrgID != g.DomainMatch.OrgID) {
		items = append(items, overlayItem{id: "pick", label: cleanCell(g.DomainMatch.Name), desc: "domain " + g.Domain, ref: g.DomainMatch.OrgID, value: g.DomainMatch.Name})
	}
	for _, o := range list {
		if g.Match != nil && o.ID == g.Match.OrgID || g.DomainMatch != nil && o.ID == g.DomainMatch.OrgID {
			continue
		}
		items = append(items, overlayItem{id: "pick", label: cleanCell(o.Name), desc: cleanCell(str(o.Industry)), ref: o.ID, value: o.Name})
	}
	m.openPicker("Link "+cleanCell(g.Display)+" to", items, func(m *Model, it overlayItem) {
		if it.id == "create" {
			m.convStage(repo.ConvertCreate, 0, it.value)
			return
		}
		m.convStage(repo.ConvertLink, it.ref, "")
	})
	m.ov.create = func(q string) (overlayItem, bool) {
		q = strings.TrimSpace(q)
		if q == "" {
			return overlayItem{}, false
		}
		return overlayItem{id: "create", label: fmt.Sprintf("Create organization %q", q), desc: "staged, made when you apply", value: q}, true
	}
	m.ov.refilter()
}

func (m *Model) convAskName() tea.Cmd {
	g, ok := m.selConvert()
	if !ok {
		return nil
	}
	initial := g.Display
	if d, ok := m.convStaged[g.Key]; ok && d.Name != "" {
		initial = d.Name
	}
	m.startInput("convert-name", "organization name: ", initial)
	return m.input.Focus()
}

// runConvertName stages create-with-name for the group under the cursor.
func (m *Model) runConvertName(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		m.setStatus("an organization needs a name", true)
		return
	}
	m.convStage(repo.ConvertCreate, 0, name)
}

func (m *Model) convDecisions() []repo.ConvertDecision {
	var out []repo.ConvertDecision
	for _, g := range m.convGroups { // the order of the list
		if d, ok := m.convStaged[g.Key]; ok {
			out = append(out, d)
		}
	}
	return out
}

func (m *Model) convAskApply() {
	ds := m.convDecisions()
	if len(ds) == 0 {
		return
	}
	creates, people := 0, 0
	by := map[string]repo.ConvertGroup{}
	for _, g := range m.convGroups {
		by[g.Key] = g
	}
	for _, d := range ds {
		if d.Action == repo.ConvertCreate {
			creates++
		}
		if d.Action != repo.ConvertSkip {
			people += by[d.Key].Count
		}
	}
	m.mode = modeConfirm
	m.confirm = &confirmState{
		prompt: fmt.Sprintf("apply %d decisions: %d people, %d new organizations? y/n", len(ds), people, creates),
		yes:    func() { m.convApply(ds, "company review") },
	}
}

func (m *Model) convAskExact() {
	var ds []repo.ConvertDecision
	people := 0
	for _, g := range m.convGroups {
		if g.Exact() {
			ds = append(ds, repo.ConvertDecision{Key: g.Key, Action: repo.ConvertLink, OrgID: g.Match.OrgID})
			people += g.Count
		}
	}
	if len(ds) == 0 {
		return
	}
	m.mode = modeConfirm
	m.confirm = &confirmState{
		prompt: fmt.Sprintf("link %d exact matches (%d people) to their organizations? y/n", len(ds), people),
		yes:    func() { m.convApply(ds, "accept exact matches") },
	}
}

// convApply writes the decisions and records them as one undo step.
func (m *Model) convApply(ds []repo.ConvertDecision, label string) {
	ch, err := m.conv.Apply(m.ctx, ds)
	if err != nil {
		m.setStatus(err.Error(), true)
		m.pendingCmd = m.showToast(err.Error(), true)
		return
	}
	for _, d := range ds {
		delete(m.convStaged, d.Key)
	}
	m.record(label+": "+fmt.Sprintf("%d people, %d groups", ch.PeopleDone, ch.GroupsDone),
		func() error { return m.conv.Undo(m.ctx, ch) },
		func() error { return m.conv.Redo(m.ctx, ch) })
	m.afterWrite()
	msg := fmt.Sprintf("converted %d people in %d groups, created %d organizations", ch.PeopleDone, ch.GroupsDone, len(ch.OrgsCreated))
	m.setStatus(msg+" (ctrl+z undoes)", false)
	m.pendingCmd = m.showToast(msg+" · ctrl+z undoes", false)
}

// convProgress is the count text of the tab bar.
func (m *Model) convProgress() string {
	return fmt.Sprintf("%d people, %d groups, %d matched, %d staged", m.convStats.People, m.convStats.Groups, m.convStats.Matched, len(m.convStaged))
}

// convDetail is the detail pane of a group.
func (m *Model) convDetail(r row) string {
	g, ok := m.selConvert()
	if !ok {
		return m.st.Dim.Render("Nothing selected.")
	}
	w := max(1, m.detail.Width)
	var b strings.Builder
	b.WriteString(m.st.Name.Render(cutTo(cleanCell(g.Display), w)) + m.st.Dim.Render(fmt.Sprintf("  %d people", g.Count)) + "\n\n")
	if len(g.Variants) > 1 {
		b.WriteString(m.st.Head.Render("Spellings") + "\n")
		for _, v := range g.Variants {
			b.WriteString(cutTo(fmt.Sprintf("%d  %s", v.People, cleanCell(v.Text)), w) + "\n")
		}
		b.WriteString("\n")
	}
	switch {
	case g.Match != nil && g.Exact():
		b.WriteString(m.detailLine("matches", fmt.Sprintf("%s (#%d, %s)", g.Match.Name, g.Match.OrgID, g.Match.Kind)) + "\n")
	case g.Match != nil:
		b.WriteString(m.detailLine("close to", fmt.Sprintf("%s (#%d), a suggestion only", g.Match.Name, g.Match.OrgID)) + "\n")
	default:
		b.WriteString(m.detailLine("matches", "no organization yet") + "\n")
	}
	if g.Domain != "" {
		b.WriteString(m.detailLine("email domain", g.Domain) + "\n")
		if g.DomainMatch != nil {
			b.WriteString(m.detailLine("domain match", fmt.Sprintf("%s (#%d), a suggestion only", g.DomainMatch.Name, g.DomainMatch.OrgID)) + "\n")
		} else {
			b.WriteString(m.detailLine("new org domain", g.Domain+", set when created") + "\n")
		}
	}
	if d, ok := m.convStaged[g.Key]; ok {
		b.WriteString(m.detailLine("staged", m.convDecisionText(d)) + "\n")
	}
	b.WriteString("\n" + m.st.Head.Render("People") + "\n")
	for i, p := range g.People {
		if i == 12 {
			b.WriteString(m.st.Dim.Render(fmt.Sprintf("… and %d more", len(g.People)-i)) + "\n")
			break
		}
		line := cleanCell(p.Name)
		if p.Title != "" {
			line += m.st.Dim.Render("  " + cleanCell(p.Title))
		}
		if p.Review {
			line += m.st.Dim.Render("  from phone")
		}
		if p.HasPrimary {
			line += m.st.Dim.Render("  has a primary organization")
		}
		b.WriteString(cutTo(line, w+20) + "\n")
	}
	b.WriteString("\n" + m.st.Dim.Render("c create, l link, e rename, s skip, x clear, A accept exact, enter apply"))
	return strings.TrimRight(b.String(), "\n")
}

var _ = model.ErrValidation
