package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/mattn/go-runewidth"
)

var (
	accent    = lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#9D9BFF"}
	dim       = lipgloss.AdaptiveColor{Light: "#777777", Dark: "#888888"}
	danger    = lipgloss.AdaptiveColor{Light: "#C0392B", Dark: "#FF7B72"}
	good      = lipgloss.AdaptiveColor{Light: "#1E7E34", Dark: "#7EE787"}
	tabOn     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#5A56E0")).Padding(0, 1)
	tabOff    = lipgloss.NewStyle().Foreground(dim).Padding(0, 1)
	selStyle  = lipgloss.NewStyle().Reverse(true)
	dimStyle  = lipgloss.NewStyle().Foreground(dim)
	nameStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	headStyle = lipgloss.NewStyle().Bold(true)
	errStyle  = lipgloss.NewStyle().Foreground(danger)
	okStyle   = lipgloss.NewStyle().Foreground(good)
)

func tabLabel(i int, name string) string { return fmt.Sprintf("%d %s", i+1, name) }

// cleanCell drops emoji and the joiners/selectors that glue them together.
// Terminals disagree with go-runewidth on how wide these are, which shifts
// every column to their right, so list cells show only plain text.
func cleanCell(s string) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r == 0x200D, r >= 0xFE00 && r <= 0xFE0F, r == 0x20E3: // ZWJ, variation selectors, keycap
			return -1
		case r >= 0x1F000, r >= 0x2600 && r <= 0x27BF, r >= 0x2B00 && r <= 0x2BFF: // emoji, symbols, dingbats, arrows
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(clean), " ")
}

// fit truncates s to width cells and pads it with spaces.
func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = runewidth.Truncate(s, width, "…")
	if pad := width - runewidth.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// View implements tea.Model.
func (m *Model) View() string {
	if m.width == 0 {
		return "loading..."
	}
	if m.help {
		return m.helpView()
	}
	listW := m.listWidth()
	bodyH := m.bodyHeight()
	detailW := m.width - listW - 1
	if detailW < 1 {
		detailW = 1
	}

	list := lipgloss.NewStyle().Width(listW).Height(bodyH).MaxHeight(bodyH).Render(m.listView(listW, bodyH))
	sep := lipgloss.NewStyle().Foreground(dim).Render(strings.TrimRight(strings.Repeat("│\n", bodyH), "\n"))
	detail := lipgloss.NewStyle().Width(detailW).Height(bodyH).MaxHeight(bodyH).PaddingLeft(1).Render(m.detail.View())
	body := lipgloss.JoinHorizontal(lipgloss.Top, list, sep, detail)

	return m.tabsView() + "\n" + body + "\n" + m.footerView()
}

func (m *Model) tabsView() string {
	var tabs []string
	for i, name := range viewNames {
		label := tabLabel(i, name)
		if viewKind(i) == m.view {
			tabs = append(tabs, tabOn.Render(label))
		} else {
			tabs = append(tabs, tabOff.Render(label))
		}
	}
	line := strings.Join(tabs, "")
	count := dimStyle.Render(fmt.Sprintf("%d shown / %d total", len(m.rows), len(m.people)))
	gap := m.width - lipgloss.Width(line) - lipgloss.Width(count)
	if gap < 1 {
		return line
	}
	return line + strings.Repeat(" ", gap) + count
}

func (m *Model) listView(w, h int) string {
	if len(m.rows) == 0 {
		switch {
		case m.view == viewPeople && m.query != "":
			return dimStyle.Render("No matches.")
		case m.view == viewDue:
			return dimStyle.Render("Nobody is due. Set a cadence with: crm person edit <id> --keep-in-touch \"3 mons\"")
		case m.view == viewBirthdays:
			return dimStyle.Render("No birthdays in the next 90 days.")
		case m.view == viewTasks:
			return dimStyle.Render("No open tasks. Press t on a person to add one.")
		}
		return dimStyle.Render("Nothing here yet.")
	}
	var b strings.Builder
	end := m.offset + h
	if end > len(m.rows) {
		end = len(m.rows)
	}
	for i := m.offset; i < end; i++ {
		r := m.rows[i]
		title, sub := cleanCell(r.title), cleanCell(r.sub)
		meta := runewidth.Truncate(cleanCell(r.meta), w/2, "…")
		metaW := runewidth.StringWidth(meta)
		left := title
		if sub != "" {
			left += "  " + sub
		}
		line := fit(left, w-metaW-1) + " " + meta
		line = fit(line, w)
		if i == m.cursor {
			b.WriteString(selStyle.Render(line))
		} else {
			// Dim the subtitle by rendering name and rest separately.
			name := runewidth.Truncate(title, w-metaW-1, "…")
			rest := ""
			if sub != "" && runewidth.StringWidth(name) == runewidth.StringWidth(title) {
				rest = fit("  "+sub, w-metaW-1-runewidth.StringWidth(name))
			} else {
				rest = strings.Repeat(" ", max(0, w-metaW-1-runewidth.StringWidth(name)))
			}
			b.WriteString(name + dimStyle.Render(rest+" "+meta))
		}
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func (m *Model) footerView() string {
	var status string
	switch m.mode {
	case modeSearch, modePrompt:
		status = m.input.View()
	default:
		switch {
		case m.status != "" && m.statusIsErr:
			status = errStyle.Render(m.status)
		case m.status != "":
			status = okStyle.Render(m.status)
		case m.query != "":
			status = dimStyle.Render("filter: " + m.query + "  (esc clears)")
		}
	}
	hints := "j/k move  / search  1-4 views  c contacted  n note  t task  s snooze  f favorite  x done  ? help  q quit"
	if m.readOnly {
		hints = "READ-ONLY  j/k move  / search  1-4 views  J/K scroll detail  ? help  q quit"
	}
	if m.mode != modeNormal {
		hints = "enter confirm  esc cancel"
	}
	return fit(status, m.width) + "\n" + dimStyle.Render(runewidth.Truncate(hints, m.width, "…"))
}

func (m *Model) helpView() string {
	rows := [][2]string{
		{"j / k, ↑ / ↓", "move"},
		{"g / G", "top / bottom"},
		{"ctrl+d / ctrl+u", "half page down / up"},
		{"J / K", "scroll the detail pane"},
		{"/", "search people (name, company, email, location, tags). Use #tag to filter by tag"},
		{"esc", "clear the search"},
		{"1 2 3 4, tab", "People, Due, Birthdays, Tasks"},
		{"c", "mark contacted now (resets the keep-in-touch clock)"},
		{"s", "snooze keep-in-touch for N days"},
		{"n", "add a note to the selected person"},
		{"t", "add a task: title | optional due date"},
		{"f", "toggle favorite"},
		{"x", "complete the selected task (Tasks view)"},
		{"o", "toggle sort: name / recently updated"},
		{"r", "reload from the database"},
		{"q", "quit"},
		{"mouse", "click to select, click a tab to switch, wheel to scroll"},
	}
	var b strings.Builder
	b.WriteString(headStyle.Render("Keys") + "\n\n")
	for _, r := range rows {
		b.WriteString(nameStyle.Render(fit(r[0], 18)) + r[1] + "\n")
	}
	b.WriteString("\n" + dimStyle.Render("press any key to close"))
	return b.String()
}

// detailText renders the right-hand pane for the selected row.
func (m *Model) detailText(r row, ok bool) string {
	if !ok {
		return dimStyle.Render("Nothing selected.")
	}
	var b strings.Builder
	if r.taskID != 0 {
		m.writeTask(&b, r.taskID)
	}
	if r.personID != 0 {
		m.writePerson(&b, r.personID)
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m *Model) writeTask(b *strings.Builder, id int64) {
	t, err := m.tr.FindByID(m.ctx, id)
	if err != nil {
		b.WriteString(errStyle.Render(err.Error()) + "\n")
		return
	}
	b.WriteString(nameStyle.Render(t.Title) + "\n")
	if t.DueAt != nil {
		b.WriteString("due " + *t.DueAt + "\n")
	}
	if t.Recurrence != nil {
		b.WriteString("repeats every " + *t.Recurrence + "\n")
	}
	b.WriteString("priority " + t.Priority + "\n")
	if t.Description != nil {
		b.WriteString(*t.Description + "\n")
	}
	b.WriteString("\n")
}

func (m *Model) writePerson(b *strings.Builder, id int64) {
	p, err := m.pr.FindByID(m.ctx, id)
	if err != nil {
		b.WriteString(errStyle.Render(err.Error()) + "\n")
		return
	}
	if err := m.pr.LoadDetails(m.ctx, p); err != nil {
		b.WriteString(errStyle.Render(err.Error()) + "\n")
		return
	}
	name := p.FirstName
	if p.LastName != nil {
		name += " " + *p.LastName
	}
	if p.IsFavorite {
		name = "★ " + name
	}
	b.WriteString(nameStyle.Render(name) + dimStyle.Render(fmt.Sprintf("  #%d", p.ID)) + "\n")

	var line []string
	if p.Title != nil {
		line = append(line, *p.Title)
	}
	if p.Company != nil {
		line = append(line, *p.Company)
	}
	if len(line) > 0 {
		b.WriteString(strings.Join(line, " at ") + "\n")
	}
	if p.Location != nil {
		b.WriteString(dimStyle.Render(*p.Location) + "\n")
	}
	b.WriteString("\n")

	for _, e := range p.Emails {
		b.WriteString(label("email") + e.Email + labelSuffix(e.Label, e.IsPrimary) + "\n")
	}
	for _, ph := range p.Phones {
		b.WriteString(label("phone") + ph.PhoneNumber + labelSuffix(ph.Label, ph.IsPrimary) + "\n")
	}
	for _, h := range p.Handles {
		b.WriteString(label(h.Kind) + h.Value + "\n")
	}

	if bd := formatBirthdayParts(p); bd != "" {
		b.WriteString(label("birthday") + bd + "\n")
	}
	if p.KeepInTouch != nil {
		line := *p.KeepInTouch
		if p.LastContactedAt != nil {
			line += ", last contacted " + *p.LastContactedAt
		} else {
			line += ", never contacted"
		}
		if p.KeepInTouchSnoozedUntil != nil && *p.KeepInTouchSnoozedUntil > m.now().UTC().Format("2006-01-02 15:04:05") {
			line += ", snoozed until " + *p.KeepInTouchSnoozedUntil
		}
		b.WriteString(label("keep in touch") + line + "\n")
	} else if p.LastContactedAt != nil {
		b.WriteString(label("last contacted") + *p.LastContactedAt + "\n")
	}

	if tags, err := m.tagr.GetForEntity(m.ctx, "person", p.ID); err == nil && len(tags) > 0 {
		names := make([]string, len(tags))
		for i, t := range tags {
			names[i] = t.Name
		}
		b.WriteString(label("tags") + strings.Join(names, ", ") + "\n")
	}

	if p.Notes != nil && *p.Notes != "" {
		b.WriteString("\n" + headStyle.Render("Notes") + "\n" + *p.Notes + "\n")
	}
	if p.Summary != nil && *p.Summary != "" {
		b.WriteString("\n" + headStyle.Render("Summary") + "\n" + *p.Summary + "\n")
	}

	if tasks, err := m.tr.FindAll(m.ctx, model.TaskFilters{PersonID: &p.ID}); err == nil && len(tasks) > 0 {
		b.WriteString("\n" + headStyle.Render("Open tasks") + "\n")
		for _, t := range tasks {
			due := ""
			if t.DueAt != nil {
				due = dimStyle.Render("  due " + *t.DueAt)
			}
			b.WriteString("• " + t.Title + due + "\n")
		}
	}

	if ints, err := m.ir.FindAll(m.ctx, model.InteractionFilters{PersonID: &p.ID, Limit: 8}); err == nil && len(ints) > 0 {
		b.WriteString("\n" + headStyle.Render("Recent") + "\n")
		for _, i := range ints {
			head := dimStyle.Render(i.OccurredAt) + "  " + i.Type
			if i.Subject != nil {
				head += ": " + *i.Subject
			}
			b.WriteString(head + "\n")
			if i.Content != nil && *i.Content != "" {
				b.WriteString("  " + truncateLines(*i.Content, 4) + "\n")
			}
		}
	}
}

func label(s string) string { return dimStyle.Render(fit(s, 14)) }

func labelSuffix(l *string, primary bool) string {
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
	return dimStyle.Render("  (" + strings.Join(parts, ", ") + ")")
}

func formatBirthdayParts(p *model.Person) string {
	if p.BirthdayMonth == nil || p.BirthdayDay == nil {
		return ""
	}
	if p.BirthdayYear != nil {
		return fmt.Sprintf("%04d-%02d-%02d", *p.BirthdayYear, *p.BirthdayMonth, *p.BirthdayDay)
	}
	return fmt.Sprintf("%02d-%02d", *p.BirthdayMonth, *p.BirthdayDay)
}

func truncateLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = append(lines[:n], "…")
	}
	return strings.Join(lines, "\n  ")
}
