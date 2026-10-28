package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The test clock is 2026-10-13. Bob Baker (--10-20, no year) is in the fixture.
func birthdayModel(t *testing.T) *Model {
	t.Helper()
	m, pr := newTestModel(t)
	ctx := context.Background()
	add := func(name, birthday string) {
		p, err := pr.Create(ctx, model.CreatePersonInput{FirstName: name})
		require.NoError(t, err)
		_, err = pr.Update(ctx, p.ID, model.UpdatePersonInput{Birthday: sp(birthday)})
		require.NoError(t, err)
	}
	add("Ann", "1990-10-13")   // today, turns 36
	add("Cy", "--10-13")       // today, no year
	add("Past", "1980-10-05")  // 8 days ago, turned 46
	add("Older", "2001-07-20") // 85 days ago
	add("Next", "2000-11-02")  // in 2w 6d
	add("Far", "1975-10-13")   // today, turns 51
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	m.switchView(viewBirthdays)
	return m
}

func TestBirthdayViewGroupsByMonthAndDayWithDividersAndAges(t *testing.T) {
	m := birthdayModel(t)
	var kinds []string
	for _, r := range m.rows {
		switch {
		case r.rule:
			kinds = append(kinds, "day:"+r.title+"|"+r.meta)
		case r.skip:
			kinds = append(kinds, "month:"+r.title)
		default:
			kinds = append(kinds, r.title+"|"+r.meta)
		}
	}
	assert.Equal(t, []string{
		"month:JULY",
		"day:2026-07-20|2mo 3w 3d ago",
		"Older|25 yrs",
		"month:OCTOBER",
		"day:2026-10-05|1w 1d ago",
		"Past|46 yrs",
		"day:2026-10-13|today",
		"Ann|36 yrs",
		"Cy|",
		"Far|51 yrs",
		"day:2026-10-20|in 1w",
		"Bob Baker|",
		"month:NOVEMBER",
		"day:2026-11-02|in 2w 6d",
		"Next|26 yrs",
		"month:JULY 2027",
		"day:2027-07-20|in 9mo 1w",
		"Older|26 yrs",
		"month:OCTOBER 2027",
		"day:2027-10-05|in 11mo 3w 1d",
		"Past|47 yrs",
	}, kinds)
	for _, r := range m.rows {
		if r.rule || (r.skip && !r.rule) {
			assert.True(t, r.skip, "headings and dividers are not selectable")
		}
	}
}

func TestBirthdayViewOpensAtTodayWithThePastAbove(t *testing.T) {
	m := birthdayModel(t)
	assert.Equal(t, "Ann", m.rows[m.cursor].title, "the cursor starts on the first birthday from today on")
	require.True(t, m.rows[m.offset].rule)
	assert.Equal(t, "2026-10-13", m.rows[m.offset].title, "today's divider is at the top of the list")
	first := strings.Split(m.listView(m.listWidth(), 6), "\n")[0]
	assert.Contains(t, first, "2026-10-13")
	assert.Contains(t, first, "today")

	// Scroll up: j/k skip the dividers, past birthdays come into view.
	for i := 0; i < 20; i++ {
		press(m, "up")
		assert.False(t, m.rows[m.cursor].skip, "the cursor never rests on a divider or heading")
	}
	assert.Equal(t, "Older", m.rows[m.cursor].title, "the oldest birthday is the first row to select")
	assert.Equal(t, 0, m.offset, "the list scrolled to the top to show the heading above it")
	assert.Contains(t, m.screen(), "JULY")
	assert.Contains(t, m.screen(), "2mo 3w 3d ago")

	for i := 0; i < 20; i++ {
		press(m, "down")
		assert.False(t, m.rows[m.cursor].skip)
	}
	assert.Equal(t, "Past", m.rows[m.cursor].title, "the last row is next year's birthday of the person 8 days ago")
	assert.Equal(t, "47 yrs", m.rows[m.cursor].meta)
}

func TestBirthdayHomeDestinationIsTodayOnCtrlShiftArrows(t *testing.T) {
	for _, key := range []string{"ctrl+shift+up", "ctrl+shift+down"} {
		m := birthdayModel(t)
		press(m, "home")
		assert.Equal(t, "Older", m.rows[m.cursor].title)
		press(m, key)
		assert.Equal(t, "Ann", m.rows[m.cursor].title, "%s goes to today in Birthdays", key)
		assert.Equal(t, "2026-10-13", m.rows[m.offset].title)
	}
}

func TestHomeAndEndAlwaysGoToTheFirstAndLastRow(t *testing.T) {
	m := birthdayModel(t)
	press(m, "home")
	assert.Equal(t, "Older", m.rows[m.cursor].title, "home is the first row even where ctrl+shift+up has a home destination")
	press(m, "end")
	assert.Equal(t, "Past", m.rows[m.cursor].title)
	assert.Equal(t, "47 yrs", m.rows[m.cursor].meta)
}

func TestCtrlUpAndDownJumpADayInBirthdays(t *testing.T) {
	m := birthdayModel(t)
	assert.Equal(t, "Ann", m.rows[m.cursor].title)
	press(m, "ctrl+down")
	assert.Equal(t, "Bob Baker", m.rows[m.cursor].title, "the next day: 2026-10-20")
	press(m, "ctrl+down")
	assert.Equal(t, "Next", m.rows[m.cursor].title)
	press(m, "ctrl+up")
	assert.Equal(t, "Bob Baker", m.rows[m.cursor].title, "and back a day")
	press(m, "ctrl+up")
	assert.Equal(t, "Ann", m.rows[m.cursor].title, "the first row of the day before")

	// From a later row of today, ctrl+up goes to the first row of today, then to the day before.
	m = birthdayModel(t)
	press(m, "down", "down") // Far
	assert.Equal(t, "Far", m.rows[m.cursor].title)
	press(m, "ctrl+up")
	assert.Equal(t, "Ann", m.rows[m.cursor].title)
	press(m, "ctrl+up")
	assert.Equal(t, "Past", m.rows[m.cursor].title)
}

func TestCtrlUpAndDownJumpAFirstLetterGroupInPeople(t *testing.T) {
	m, pr := newTestModel(t)
	for _, n := range []string{"Ana", "Alma", "Barb", "Bea", "Cleo"} {
		_, err := pr.Create(m.ctx, model.CreatePersonInput{FirstName: n})
		require.NoError(t, err)
	}
	m.afterWrite()
	press(m, "home")
	letters := func() string { return groupLetter(m.rows[m.cursor].title) }
	start := letters()
	press(m, "ctrl+down")
	assert.NotEqual(t, start, letters(), "ctrl+down lands in the next first-letter group")
	first := m.cursor
	press(m, "down")
	press(m, "ctrl+up")
	assert.Equal(t, first, m.cursor, "ctrl+up from inside a group goes to the start of that group")
	press(m, "ctrl+up")
	assert.Equal(t, start, letters(), "and once more to the group before")
}

func TestCtrlUpAndDownJumpAPageInViewsWithoutAUnit(t *testing.T) {
	m, pr := newTestModel(t)
	for i := 0; i < 40; i++ {
		_, err := pr.Create(m.ctx, model.CreatePersonInput{FirstName: "P" + strings.Repeat("q", i+1), LastName: sp("Due")})
		require.NoError(t, err)
	}
	m.afterWrite()
	m.switchView(viewDue)
	m.settle()
	if len(m.rows) < 5 {
		t.Skip("no follow-up rows to page through")
	}
	press(m, "ctrl+down")
	assert.Greater(t, m.cursor, 1, "a page down in a view without a unit")
}

func TestPageKeysMoveAPageAndCtrlKeysHalfAPage(t *testing.T) {
	m, pr := newTestModel(t)
	for i := 0; i < 60; i++ {
		_, err := pr.Create(m.ctx, model.CreatePersonInput{FirstName: "Pg", LastName: sp(string(rune('a'+i%26)) + strings.Repeat("z", i/26))})
		require.NoError(t, err)
	}
	m.afterWrite()
	press(m, "home")
	press(m, "ctrl+d")
	half := m.cursor
	press(m, "home")
	press(m, "pgdown")
	full := m.cursor
	assert.Greater(t, half, 0)
	assert.Greater(t, full, half, "pgdown moves a full page and ctrl+d half of one")
	press(m, "pgup")
	assert.Less(t, m.cursor, full)
	press(m, "end")
	press(m, "ctrl+u")
	assert.Less(t, m.cursor, len(m.rows)-1)
}

func TestTheOldNavigationAndAddKeysAreGone(t *testing.T) {
	for _, k := range []string{"j", "k", "h", "g", "G", "ctrl+t"} {
		m, _ := newTestModel(t)
		press(m, "down")
		before := m.cursor
		press(m, k)
		assert.Equal(t, before, m.cursor, "%s no longer moves", k)
		assert.Equal(t, viewPeople, m.view, k)
		assert.Equal(t, modeNormal, m.mode, k)
		assert.Equal(t, focusList, m.focus, k)
	}
	m, _ := newTestModel(t)
	press(m, "l") // l is Link now, not "move right"
	assert.Equal(t, focusList, m.focus)
	require.NotNil(t, m.ov)
	assert.Contains(t, m.ov.title, "Link")
}

func TestGoToTodayIsAPaletteEntryInEveryModuleWithAnAnchor(t *testing.T) {
	m, pr := newTestModel(t)
	// Tasks: the first task due today or later.
	for _, in := range []model.CreateTaskInput{
		{Title: "old", Priority: "medium", DueAt: sp("2026-10-01")},
		{Title: "later", Priority: "medium", DueAt: sp("2026-10-20")},
		{Title: "today", Priority: "medium", DueAt: sp("2026-10-13")},
	} {
		_, err := m.tr.Create(context.Background(), in)
		require.NoError(t, err)
	}
	_ = pr
	m.afterWrite()
	m.switchView(viewTasks)
	goToday := m.actionByID("go-today")
	require.NotNil(t, goToday)
	assert.Empty(t, goToday.keys, "ctrl+t and ctrl+up are retired")
	press(m, "end")
	m.runAction(goToday)
	assert.Equal(t, "today", m.rows[m.cursor].title, "Tasks: the first task due today or later")

	// Home, People and Follow-up: the top of the list.
	for _, v := range []viewKind{viewHome, viewPeople, viewDue} {
		m.switchView(v)
		if len(m.rows) == 0 {
			continue
		}
		press(m, "end")
		m.runAction(goToday)
		first := 0
		for i, r := range m.rows {
			if !r.skip {
				first = i
				break
			}
		}
		assert.Equal(t, first, m.cursor, "%s: Go to today goes to the top", viewNames[v])
	}

	// A module with no anchor leaves the entry out.
	for _, v := range []viewKind{viewOrgs, viewDeals, viewArchived, viewSync} {
		m.switchView(v)
		assert.False(t, m.actionByID("go-today").enabled(m), "%s has no anchor", viewNames[v])
	}
}

func TestBirthdayViewFitsEightyColumns(t *testing.T) {
	m := birthdayModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m.snapBirthdays()
	for _, l := range strings.Split(m.screen(), "\n") {
		assert.LessOrEqual(t, lipgloss.Width(l), 80, "%q", l)
	}
	list := m.listView(38, 20)
	assert.Contains(t, list, "2026-10-20 ")
	assert.Contains(t, list, "─ in 1w")
	assert.Contains(t, list, "36 yrs")
}

func TestRuleLine(t *testing.T) {
	line := ruleLine("2026-10-20", "in 1w", 30)
	assert.Equal(t, 30, lipgloss.Width(line))
	assert.True(t, strings.HasPrefix(line, "2026-10-20 ─"))
	assert.True(t, strings.HasSuffix(line, "─ in 1w"))
	assert.LessOrEqual(t, lipgloss.Width(ruleLine("2026-10-20", "in 1y 11mo 3w", 14)), 14, "too narrow still fits")
}
