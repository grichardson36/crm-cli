package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
		press(m, "k")
		assert.False(t, m.rows[m.cursor].skip, "the cursor never rests on a divider or heading")
	}
	assert.Equal(t, "Older", m.rows[m.cursor].title, "the oldest birthday is the first row to select")
	assert.Equal(t, 0, m.offset, "the list scrolled to the top to show the heading above it")
	assert.Contains(t, m.View(), "JULY")
	assert.Contains(t, m.View(), "2mo 3w 3d ago")

	for i := 0; i < 20; i++ {
		press(m, "j")
		assert.False(t, m.rows[m.cursor].skip)
	}
	assert.Equal(t, "Past", m.rows[m.cursor].title, "the last row is next year's birthday of the person 8 days ago")
	assert.Equal(t, "47 yrs", m.rows[m.cursor].meta)
}

func TestBirthdayTodayKeyAndCtrlUpSnapBack(t *testing.T) {
	for _, key := range []string{"t", "ctrl+up"} {
		m := birthdayModel(t)
		press(m, "g")
		assert.Equal(t, "Older", m.rows[m.cursor].title)
		press(m, key)
		assert.Equal(t, "Ann", m.rows[m.cursor].title, "%s snaps back to today", key)
		assert.Equal(t, "2026-10-13", m.rows[m.offset].title)
	}
	m := birthdayModel(t)
	assert.Contains(t, m.footerView(), "today", "the key shows in the footer")

	// The key is a registry action, rebindable.
	m.applyKeys(map[string][]string{"birthdays-today": {"ctrl+b"}})
	press(m, "g")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	assert.Equal(t, "Ann", m.rows[m.cursor].title)
	assert.Equal(t, []string{"ctrl+b"}, m.actionByID("birthdays-today").keys)
}

func TestTodayKeyOutsideBirthdaysStillMeansTask(t *testing.T) {
	m, _ := newTestModel(t) // People view, a person selected
	press(m, "t")
	assert.Equal(t, modePrompt, m.mode, "t adds a task on a person outside the Birthdays view")
}

func TestBirthdayViewFitsEightyColumns(t *testing.T) {
	m := birthdayModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m.snapBirthdays()
	for _, l := range strings.Split(m.View(), "\n") {
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
