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

func (m *Model) cycleLevel() {
	next := m.minLevel - 1
	if next < dedupe.Fuzzy {
		next = dedupe.High
	}
	m.minLevel = next
	m.cursor, m.offset = 0, 0
	m.report(m.rebuild(false))
	m.setStatus("duplicates: "+m.minLevel.String()+" confidence and above", false)
}

func (m *Model) openGroupCompare() {
	if m.cursor < 0 || m.cursor >= len(m.groups) {
		return
	}
	g := m.groups[m.cursor]
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
	if m.cursor >= 0 && m.cursor < len(m.groups) && m.view == viewMerge {
		return m.groups[m.cursor].IDs()
	}
	return nil
}

func (m *Model) dismissGroup() {
	ids := m.groupIDs()
	if len(ids) < 2 {
		return
	}
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if err := m.dr.Dismiss(m.ctx, ids[i], ids[j]); err != nil {
				m.setStatus(err.Error(), true)
				return
			}
		}
	}
	m.dismissed = append(m.dismissed, ids)
	manual := m.cmp != nil && m.cmp.manual
	m.cmp = nil
	if manual {
		m.marked = map[int64]bool{}
	}
	m.afterWrite()
	m.setStatus(fmt.Sprintf("dismissed %d people as not duplicates (u undoes)", len(ids)), false)
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
	w := max(1, m.detail.Width)
	line := func(s string) string { return runewidth.Truncate(s, w, "…") }
	r, ok := m.selected()
	if !ok {
		return dimStyle.Render("Nothing selected.")
	}
	var b strings.Builder
	switch m.view {
	case viewSync:
		return m.syncItemDetail(r)
	case viewMerge:
		if m.cursor >= len(m.groups) {
			return ""
		}
		g := m.groups[m.cursor]
		b.WriteString(nameStyle.Render(fmt.Sprintf("%s confidence, %d people", levelBadge(g.Level), len(g.Members))) + "\n")
		for _, reason := range g.Reasons {
			b.WriteString(dimStyle.Render(line(reason)) + "\n")
		}
		for _, mb := range g.Members {
			b.WriteString("\n" + headStyle.Render(line(fmt.Sprintf("#%d %s", mb.ID, cleanCell(mb.Name)))) + "\n")
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
					b.WriteString(dimStyle.Render(line(p.AllEmails)) + "\n")
				}
				if p.Location != "" {
					b.WriteString(dimStyle.Render(line(cleanCell(p.Location))) + "\n")
				}
			}
		}
		b.WriteString("\n" + dimStyle.Render("enter compares and merges, D dismisses"))
	case viewArchived:
		p := m.arch[r.personID]
		if p == nil {
			return dimStyle.Render("Nothing selected.")
		}
		_ = m.pr.LoadDetails(m.ctx, p)
		b.WriteString(nameStyle.Render(line(fullName(p))) + dimStyle.Render("  archived "+dateOnly(p.UpdatedAt)) + "\n\n")
		for _, kv := range [][2]string{{"title", str(p.Title)}, {"company", str(p.Company)}, {"location", str(p.Location)}, {"birthday", formatBirthdayParts(p)}} {
			if kv[1] != "" {
				b.WriteString(dimStyle.Render(fit(kv[0], labelWidth)) + line(kv[1]) + "\n")
			}
		}
		for _, e := range p.Emails {
			b.WriteString(dimStyle.Render(fit("email", labelWidth)) + line(e.Email) + "\n")
		}
		for _, ph := range p.Phones {
			b.WriteString(dimStyle.Render(fit("phone", labelWidth)) + line(ph.PhoneNumber) + "\n")
		}
		b.WriteString("\n" + dimStyle.Render("u restores, X purges forever"))
	default:
		return dimStyle.Render("Nothing selected.")
	}
	return strings.TrimRight(b.String(), "\n")
}
