package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// The Birthdays view lists the days a window back from today and ahead of it, grouped by month with
// a divider row per day. It opens at today with the past above.
const (
	birthdayDaysBack  = 90
	birthdayDaysAhead = 365
)

// birthdayRows builds the list: month headings (OCTOBER, with the year when it is not this year), a
// divider per day (date at the left, relative time at the right edge) and the people below it, each
// with the age they turn that day at the right when the birth year is known.
func (m *Model) birthdayRows() ([]row, error) {
	list, err := m.pr.BirthdaysWindow(m.ctx, birthdayDaysBack, birthdayDaysAhead, m.now())
	if err != nil {
		return nil, err
	}
	thisYear := m.now().Year()
	for _, b := range list {
		if _, err := time.Parse("2006-01-02", b.Date); err != nil {
			return nil, fmt.Errorf("parse birthday date %q: %w", b.Date, err)
		}
	}
	// The view groups by day (the group field) under a month heading, through the shared grouped list.
	g := groupRows(list, func(b model.UpcomingBirthday) []groupHead {
		when, _ := time.Parse("2006-01-02", b.Date)
		head := strings.ToUpper(when.Month().String())
		if when.Year() != thisYear {
			head += fmt.Sprintf(" %d", when.Year())
		}
		return []groupHead{
			{key: when.Format("2006-01"), title: head},
			{key: b.Date, title: b.Date, meta: b.Relative, rule: true},
		}
	}, func(b model.UpcomingBirthday) row {
		age := ""
		if b.Turning != nil {
			age = fmt.Sprintf("%d yrs", *b.Turning)
			if *b.Turning == 1 {
				age = "1 yr"
			}
		}
		return row{personID: b.PersonID, title: cleanCell(b.Name), meta: age}
	})
	m.bdToday, m.bdTop = -1, 0
	for i, b := range list {
		if b.DaysUntil >= 0 {
			m.bdToday = g.at[i]
			m.bdTop = g.headsAt[i][1]
			if g.headsAt[i][0] == m.bdTop-1 {
				m.bdTop = g.headsAt[i][0]
			}
			break
		}
	}
	if m.bdToday < 0 && len(g.rows) > 0 { // every birthday is in the past
		last := len(list) - 1
		m.bdToday, m.bdTop = len(g.rows)-1, max(0, g.headsAt[last][1])
	}
	return g.rows, nil
}

// snapBirthdays puts the cursor on the first birthday from today on and scrolls so that its day sits
// at the top, with the past above.
func (m *Model) snapBirthdays() {
	if m.view != viewBirthdays || m.bdToday < 0 || m.bdToday >= len(m.rows) {
		return
	}
	m.cursor = m.bdToday
	m.offset = max(0, min(m.bdTop, len(m.rows)-m.bodyHeight()))
}

// todayActions is Go to today, one action for every module that has an anchor. It has no key: ctrl+t
// and ctrl+up are retired. ctrl+shift+up and ctrl+shift+down go to the home destination a view
// declares (today in Birthdays, navunits.go), and the palette keeps this entry for the Tasks anchor.
// The anchor of each module:
//   - Birthdays: today's divider, with the past above
//   - Tasks: the first task due today or later, else the top
//   - Home, People, Follow-up: the top of the list
//
// Modules without an anchor (Organizations, Deals, Merge, Archived, Sync) leave it out.
func todayActions() []action {
	return []action{
		{id: "go-today", title: "Go to today", group: "Views",
			desc: "jump to today in this view: today's day in Birthdays (past birthdays are above, ctrl+shift+up and ctrl+shift+down do the same), the first task dated today or later in Tasks, the top in Home, People and Follow-up",
			when: hasTodayAnchor,
			run:  func(m *Model) tea.Cmd { m.goToday(); return nil }},
	}
}

func hasTodayAnchor(m *Model) bool {
	if m.focus != focusList {
		return false
	}
	switch m.view {
	case viewHome, viewPeople, viewDue, viewTasks, viewBirthdays:
		return true
	}
	return false
}

func (m *Model) goToday() {
	switch m.view {
	case viewBirthdays:
		m.snapBirthdays()
	case viewTasks:
		today := m.now().Format("2006-01-02")
		m.step(-1 << 20)
		for i, r := range m.rows {
			if !r.skip && len(r.meta) >= 10 && r.meta[:10] >= today {
				m.cursor = i
				break
			}
		}
	default:
		m.step(-1 << 20)
	}
	m.clamp()
	m.refreshDetail(false)
}
