package tui

import (
	"fmt"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/dedupe"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/mattn/go-runewidth"
)

// Merge view (duplicate groups) and the compare screen that merges people.

func whenCompare(m *Model) bool   { return m.cmp != nil }
func whenCmpEdit(m *Model) bool   { return m.cmp != nil && !m.cmp.confirm }
func whenCmpGroup(m *Model) bool  { return m.cmp != nil && len(m.cmp.members) > 2 }
func whenCmpNarrow(m *Model) bool { return m.cmp != nil && m.width < cmpWide }
func whenMergeList(m *Model) bool { return m.view == viewMerge && m.focus == focusList }
func whenMergeRows(m *Model) bool { return whenMergeList(m) && len(m.rows) > 0 }
func whenDismiss(m *Model) bool   { return m.cmp != nil || whenMergeRows(m) }
func whenArchivedRows(m *Model) bool {
	return m.view == viewArchived && m.focus == focusList && len(m.rows) > 0
}

// levelBadge is the short label shown in the list.
func levelBadge(l dedupe.Level) string {
	switch l {
	case dedupe.High:
		return "HIGH"
	case dedupe.Medium:
		return "MED"
	case dedupe.Low:
		return "LOW"
	case dedupe.Fuzzy:
		return "FUZZY"
	}
	return "?"
}

// mergeRows lists the candidate groups under one divider per confidence level (the grouped list of
// grouped.go, group field "confidence level"). A level with no groups gets no divider.
func (m *Model) mergeRows(groups []dedupe.Group) []row {
	type item struct {
		i int
		g dedupe.Group
	}
	counts := map[dedupe.Level]int{}
	items := make([]item, len(groups))
	for i, g := range groups {
		items[i] = item{i, g}
		counts[g.Level]++
	}
	return groupRows(items, func(it item) []groupHead {
		n := counts[it.g.Level]
		meta := fmt.Sprintf("%d groups", n)
		if n == 1 {
			meta = "1 group"
		}
		return []groupHead{{key: it.g.Level.String(), title: strings.ToUpper(it.g.Level.String()) + " confidence", meta: meta, rule: true}}
	}, func(it item) row {
		r := groupRow(it.g)
		r.dupe = it.i + 1
		return r
	}).rows
}

// cursorGroup is the duplicate group under the cursor in the Merge view.
func (m *Model) cursorGroup() (dedupe.Group, bool) {
	if m.view != viewMerge || m.cursor < 0 || m.cursor >= len(m.rows) {
		return dedupe.Group{}, false
	}
	i := m.rows[m.cursor].dupe - 1
	if i < 0 || i >= len(m.groups) {
		return dedupe.Group{}, false
	}
	return m.groups[i], true
}

func groupRow(g dedupe.Group) row {
	names := make([]string, len(g.Members))
	for i, mb := range g.Members {
		names[i] = cleanCell(mb.Name)
	}
	r := row{title: fmt.Sprintf("%-5s %s", levelBadge(g.Level), strings.Join(names, " | "))}
	if len(g.Reasons) > 0 {
		r.sub = cleanCell(g.Reasons[0])
	}
	return r
}

func fullName(p *model.Person) string {
	return strings.TrimSpace(p.FirstName + " " + str(p.LastName))
}

func dateOnly(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

// nameOf is a person's display name from whatever list knows them.
func (m *Model) nameOf(id int64) string {
	for _, p := range m.people {
		if p.ID == id {
			return p.Name
		}
	}
	if p, ok := m.arch[id]; ok {
		return fullName(p)
	}
	return fmt.Sprintf("#%d", id)
}

// --- list actions ---

func (m *Model) openGroupCompare() {
	g, ok := m.cursorGroup()
	if !ok {
		return
	}
	m.openCompare(g.IDs(), g.Level, g.Reasons, false)
}

func (m *Model) openMarkedCompare() {
	ids := m.markedIDs()
	if len(ids) < 2 {
		m.setStatus("mark at least two people first (space)", true)
		return
	}
	m.openCompare(ids, 0, []string{"marked by you"}, true)
}

// groupIDs are the people a dismiss applies to: the open compare, else the selected group.
func (m *Model) groupIDs() []int64 {
	if m.cmp != nil {
		ids := make([]int64, len(m.cmp.members))
		for i, mb := range m.cmp.members {
			ids[i] = mb.id
		}
		return ids
	}
	if g, ok := m.cursorGroup(); ok {
		return g.IDs()
	}
	return nil
}

// groupIDSets are the groups a dismiss applies to: the open compare, else the selected groups of the
// Merge view (bulk actions are always available), else the group under the cursor.
func (m *Model) groupIDSets() [][]int64 {
	if m.cmp == nil && m.view == viewMerge && m.focus == focusList {
		var sets [][]int64
		for _, ref := range m.markedRefs() {
			if ref.Kind == groupKind && int(ref.ID) <= len(m.groups) {
				sets = append(sets, m.groups[ref.ID-1].IDs())
			}
		}
		if len(sets) > 0 {
			return sets
		}
	}
	if ids := m.groupIDs(); len(ids) > 1 {
		return [][]int64{ids}
	}
	return nil
}

func (m *Model) dismissGroup() {
	sets := m.groupIDSets()
	if len(sets) == 0 {
		return
	}
	for _, ids := range sets {
		for i := 0; i < len(ids); i++ {
			for j := i + 1; j < len(ids); j++ {
				if err := m.dr.Dismiss(m.ctx, ids[i], ids[j]); err != nil {
					m.setStatus(err.Error(), true)
					return
				}
			}
		}
		m.dismissed = append(m.dismissed, ids)
	}
	manual := m.cmp != nil && m.cmp.manual
	m.cmp = nil
	if manual || len(sets) > 1 {
		m.marked = map[int64]bool{}
	}
	m.afterWrite()
	if len(sets) > 1 {
		m.setStatus(fmt.Sprintf("dismissed %d groups as not duplicates (undo dismiss in the palette brings them back one by one)", len(sets)), false)
		return
	}
	m.setStatus(fmt.Sprintf("dismissed %d people as not duplicates (u undoes)", len(sets[0])), false)
}

func (m *Model) undoDismiss() {
	if len(m.dismissed) == 0 {
		m.setStatus("no dismissal to undo in this session", true)
		return
	}
	ids := m.dismissed[len(m.dismissed)-1]
	m.dismissed = m.dismissed[:len(m.dismissed)-1]
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if err := m.dr.Undismiss(m.ctx, ids[i], ids[j]); err != nil && !isNotFound(err) {
				m.setStatus(err.Error(), true)
				return
			}
		}
	}
	m.afterWrite()
	m.setStatus(fmt.Sprintf("restored the group of %d people", len(ids)), false)
}

func isNotFound(err error) bool { return err != nil && strings.Contains(err.Error(), "not found") }

// --- detail text for views without a form ---

// emptyDetail is the detail pane content when no form is shown.
func (m *Model) emptyDetail() string {
	w := max(1, m.detail.Width())
	line := func(s string) string { return runewidth.Truncate(s, w, "…") }
	r, ok := m.selected()
	if !ok {
		return m.st.Dim.Render("Nothing selected.")
	}
	var b strings.Builder
	switch m.view {
	case viewSync:
		return m.syncItemDetail(r)
	case viewOrgs:
		return m.orgDetail(r)
	case viewDeals:
		return m.dealDetail(r)
	case viewMerge:
		g, ok := m.cursorGroup()
		if !ok {
			return ""
		}
		b.WriteString(m.st.Name.Render(fmt.Sprintf("%s confidence, %d people", levelBadge(g.Level), len(g.Members))) + "\n")
		for _, reason := range g.Reasons {
			b.WriteString(m.st.Dim.Render(line(reason)) + "\n")
		}
		for _, mb := range g.Members {
			b.WriteString("\n" + m.st.Head.Render(line(fmt.Sprintf("#%d %s", mb.ID, cleanCell(mb.Name)))) + "\n")
			for _, p := range m.people {
				if p.ID != mb.ID {
					continue
				}
				var role []string
				for _, v := range []string{cleanCell(p.Title), cleanCell(p.Company)} {
					if v != "" {
						role = append(role, v)
					}
				}
				if len(role) > 0 {
					b.WriteString(line(strings.Join(role, " at ")) + "\n")
				}
				if p.AllEmails != "" {
					b.WriteString(m.st.Dim.Render(line(p.AllEmails)) + "\n")
				}
				if p.Location != "" {
					b.WriteString(m.st.Dim.Render(line(cleanCell(p.Location))) + "\n")
				}
			}
		}
		b.WriteString("\n" + m.st.Dim.Render("enter compares and merges, d dismisses"))
	case viewArchived:
		if r.akind != "" && r.akind != "person" {
			it := m.archItems[fmt.Sprintf("%s:%d", r.akind, archivedRowID(r))]
			b.WriteString(m.st.Name.Render(line(cleanCell(it.Name))) + m.st.Dim.Render("  archived "+dateOnly(it.UpdatedAt)) + "\n\n")
			b.WriteString(m.st.Dim.Render(fit("type", labelWidth)) + cliNoun(it.Kind) + "\n")
			if it.Detail != "" {
				b.WriteString(m.st.Dim.Render(fit("detail", labelWidth)) + line(cleanCell(it.Detail)) + "\n")
			}
			b.WriteString("\n" + m.st.Dim.Render("r restores, ctrl+o filters by type"))
			return b.String()
		}
		p := m.arch[r.personID]
		if p == nil {
			return m.st.Dim.Render("Nothing selected.")
		}
		_ = m.pr.LoadDetails(m.ctx, p)
		b.WriteString(m.st.Name.Render(line(fullName(p))) + m.st.Dim.Render("  archived "+dateOnly(p.UpdatedAt)) + "\n\n")
		for _, kv := range [][2]string{{"title", str(p.Title)}, {"company", str(p.Company)}, {"location", str(p.Location)}, {"birthday", formatBirthdayParts(p)}} {
			if kv[1] != "" {
				b.WriteString(m.st.Dim.Render(fit(kv[0], labelWidth)) + line(kv[1]) + "\n")
			}
		}
		for _, e := range p.Emails {
			b.WriteString(m.st.Dim.Render(fit("email", labelWidth)) + line(e.Email) + "\n")
		}
		for _, ph := range p.Phones {
			b.WriteString(m.st.Dim.Render(fit("phone", labelWidth)) + line(ph.PhoneNumber) + "\n")
		}
		b.WriteString("\n" + m.st.Dim.Render("r restores, d purges forever, ctrl+o filters by type"))
	default:
		return m.st.Dim.Render("Nothing selected.")
	}
	return strings.TrimRight(b.String(), "\n")
}
