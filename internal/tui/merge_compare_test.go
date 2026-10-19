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

// openBobs opens the compare screen for Bob Baker (#2) and bob baker (#4) with a fixed update
// order: #4 was changed last.
func openBobs(t *testing.T) (*Model, *compareState) {
	t.Helper()
	m, _ := dupeModel(t)
	_, err := m.db.Exec(`UPDATE people SET updated_at = '2026-01-01 10:00:00' WHERE id = 2`)
	require.NoError(t, err)
	_, err = m.db.Exec(`UPDATE people SET updated_at = '2026-02-01 10:00:00' WHERE id = 4`)
	require.NoError(t, err)
	press(m, "6", "j", "enter")
	require.NotNil(t, m.cmp)
	return m, m.cmp
}

// itemRow finds the list item row with the label and text.
func itemRow(c *compareState, label, text string) int {
	for i, r := range c.rows {
		if r.kind == rowItem && r.label == label && r.text == text {
			return i
		}
	}
	return -1
}

func rowIndex(c *compareState, label string) int {
	for i, r := range c.rows {
		if r.label == label {
			return i
		}
	}
	return -1
}

func TestCompareShowsEverythingAndDefaults(t *testing.T) {
	m, c := openBobs(t)
	require.Len(t, c.members, 2)
	assert.Equal(t, int64(2), c.pair(0).id)
	assert.Equal(t, int64(4), c.pair(1).id)
	assert.Equal(t, 0, c.s, "the lowest id is the default survivor")

	for _, label := range []string{"name", "title", "company", "location", "birthday", "email", "keep in touch", "last contact", "tag", "notes", "interactions", "tasks"} {
		assert.GreaterOrEqual(t, rowIndex(c, label), 0, label)
	}
	assert.Equal(t, -1, rowIndex(c, "phone"), "nobody has a phone")

	// Non-empty beats empty, then the newer record wins (#4), so its company is the default.
	assert.Equal(t, 1, c.resolved(rowIndex(c, "company")))
	assert.Equal(t, "Globex Inc", c.finalText(rowIndex(c, "company")))
	assert.Equal(t, 1, c.resolved(rowIndex(c, "title")), "only #4 has a title")
	assert.Equal(t, 0, c.resolved(rowIndex(c, "birthday")), "only #2 has a birthday")
	assert.Equal(t, "bob baker", c.finalText(rowIndex(c, "name")), "newer record wins a name conflict")
	assert.Equal(t, "7 days", c.finalText(rowIndex(c, "keep in touch")))
	assert.Equal(t, "2020-01-01", c.finalText(rowIndex(c, "last contact")))

	// Lists default to the union.
	for _, e := range []string{"bob@globex.com", "bob@home.org"} {
		i := itemRow(c, "email", e)
		require.GreaterOrEqual(t, i, 0, e)
		assert.True(t, c.included(i), e)
	}
	assert.True(t, c.included(itemRow(c, "tag", "friend")))

	// The cursor starts on the first row that differs. Differences are marked in the gutter.
	assert.Equal(t, "name", c.rows[c.cur].label)
	assert.True(t, c.conflict(rowIndex(c, "company")))
	assert.False(t, c.conflict(rowIndex(c, "title")), "one side empty is a difference, not a conflict")

	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	out := ansiStrip(m.View())
	for _, want := range []string{"Duplicates", "MED", "#2 Bob Baker", "#4 bob baker", "survivor", "archived after merge", "Final #2", "bob@home.org", "result:"} {
		assert.Contains(t, out, want)
	}
}

func TestCompareLayoutAtSeveralWidths(t *testing.T) {
	m, c := openBobs(t)
	cases := []struct {
		width int
		panes []int
	}{
		{160, []int{0, 1, 2}},
		{110, []int{0, 1, 2}},
		{109, []int{0, 1}},
		{80, []int{0, 1}},
		{64, []int{0, 1}},
		{63, []int{0}},
		{40, []int{0}},
	}
	for _, tc := range cases {
		c.page = 0
		assert.Equal(t, tc.panes, c.layout(tc.width).panes, "width %d", tc.width)
	}
	// r switches panes on narrow screens: candidates then result, or candidate 1, 2 and result.
	c.page = 1
	assert.Equal(t, []int{2}, c.layout(80).panes)
	assert.Equal(t, []int{1}, c.layout(40).panes)
	c.page = 2
	assert.Equal(t, []int{2}, c.layout(40).panes)
	c.page = 0

	for _, size := range [][2]int{{200, 40}, {160, 30}, {110, 24}, {100, 24}, {80, 24}, {64, 20}, {60, 12}, {40, 10}, {30, 8}, {20, 6}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for page := 0; page < 3; page++ {
			c.page = page
			c.confirm = false
			for _, step := range []string{"", "j", "j", "j", "j", "j", "j", "j"} {
				if step != "" {
					press(m, step)
				}
				lines := strings.Split(m.View(), "\n")
				assert.LessOrEqual(t, len(lines), size[1], "%v page %d height", size, page)
				for _, l := range lines {
					assert.LessOrEqual(t, lipgloss.Width(l), size[0], "%v page %d: %q", size, page, l)
				}
			}
			c.confirm = true
			for _, l := range strings.Split(m.View(), "\n") {
				assert.LessOrEqual(t, lipgloss.Width(l), size[0], "%v confirm: %q", size, l)
			}
		}
		c.confirm = false
	}
}

func TestCompareWidePanesAlignRows(t *testing.T) {
	m, c := openBobs(t)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	lines, rowAt := c.render(140, m.bodyHeight())
	// Each row is one line holding the label and all three panes.
	i := rowIndex(c, "company")
	var line string
	for n, r := range rowAt {
		if r == i {
			line = ansiStrip(lines[n])
		}
	}
	require.NotEmpty(t, line)
	assert.Contains(t, line, "company")
	assert.Equal(t, 2, strings.Count(line, "Globex Inc"), "candidate 2 and the result")
	assert.Equal(t, 3, strings.Count(line, "Globex"), "all three panes")
	assert.Equal(t, 3, strings.Count(line, "│"))
}

// ansiStrip removes styling so a test can read a rendered line.
func ansiStrip(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			in = true
		case in && r == 'm':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestCompareSinglePickKeys(t *testing.T) {
	m, c := openBobs(t)
	c.cur = rowIndex(c, "company")
	require.Equal(t, 1, c.resolved(c.cur))
	press(m, "left")
	assert.Equal(t, 0, c.resolved(c.cur))
	assert.Equal(t, "Globex", c.finalText(c.cur))
	press(m, "right")
	assert.Equal(t, 1, c.resolved(c.cur))
	press(m, "1")
	assert.Equal(t, 0, c.resolved(c.cur))
	press(m, "2")
	assert.Equal(t, 1, c.resolved(c.cur))
	press(m, "h")
	assert.Equal(t, 0, c.resolved(c.cur))
	press(m, "l")
	assert.Equal(t, 1, c.resolved(c.cur))
	press(m, " ")
	assert.Equal(t, 0, c.resolved(c.cur), "space flips")
	press(m, "tab")
	assert.Equal(t, 1, c.resolved(c.cur), "tab flips")

	// Picking the side that has no value clears the field.
	c.cur = rowIndex(c, "title")
	press(m, "left")
	assert.Equal(t, "", c.finalText(c.cur))

	// Up and down skip the count rows at the end.
	c.cur = len(c.rows) - 1
	for c.rows[c.cur].kind == rowInfo {
		c.cur--
	}
	last := c.cur
	press(m, "j")
	assert.Equal(t, last, c.cur)
	press(m, "k")
	assert.Less(t, c.cur, last)
}

func TestCompareListItemsToggleIntoResult(t *testing.T) {
	m, c := openBobs(t)
	home := itemRow(c, "email", "bob@home.org")
	c.cur = home
	assert.True(t, c.included(home))
	assert.Equal(t, "bob@home.org", c.finalText(home))
	press(m, " ")
	assert.False(t, c.included(home))
	assert.Equal(t, "", c.finalText(home), "out of the result pane")
	assert.Contains(t, ansiStrip(m.View()), "✗ bob@home.org", "the candidate pane still shows it, marked out")
	press(m, " ")
	assert.True(t, c.included(home))

	// Left takes the item only when candidate 1 has it. Candidate 2 has no such email.
	press(m, " ")
	press(m, "right")
	assert.True(t, m.statusIsErr)
	assert.False(t, c.included(home))
	press(m, "left")
	assert.True(t, c.included(home))

	_, _, _, opts := c.plan()
	assert.Empty(t, opts.Exclude["email"])
	press(m, " ")
	_, _, _, opts = c.plan()
	assert.Equal(t, []string{"bob@home.org"}, opts.Exclude["email"])
}

func TestCompareAlternativeLinksAreExclusive(t *testing.T) {
	m, pr := dupeModel(t)
	ctx := context.Background()
	require.NoError(t, pr.SetHandle(ctx, 2, "linkedin", "linkedin.com/in/old"))
	require.NoError(t, pr.SetHandle(ctx, 4, "linkedin", "linkedin.com/in/new"))
	require.NoError(t, pr.SetHandle(ctx, 4, "twitter", "bobby"))
	_, err := m.db.Exec(`UPDATE people SET updated_at = '2026-01-01 10:00:00' WHERE id = 2`)
	require.NoError(t, err)
	_, err = m.db.Exec(`UPDATE people SET updated_at = '2026-02-01 10:00:00' WHERE id = 4`)
	require.NoError(t, err)
	m.openCompare([]int64{2, 4}, 0, []string{"test"}, true)
	c := m.cmp
	require.NotNil(t, c)

	oldL, newL := itemRow(c, "linkedin", "linkedin.com/in/old"), itemRow(c, "linkedin", "linkedin.com/in/new")
	tw := itemRow(c, "twitter", "bobby")
	require.True(t, oldL >= 0 && newL >= 0 && tw >= 0)
	assert.True(t, c.conflict(oldL))
	assert.False(t, c.included(oldL), "the newer record's link wins by default")
	assert.True(t, c.included(newL))
	assert.True(t, c.included(tw), "a link only one side has comes along")

	c.cur = oldL
	press(m, "left")
	assert.True(t, c.included(oldL))
	assert.False(t, c.included(newL), "taking one link drops its alternative")

	keep, drops, choices, opts := c.plan()
	assert.Equal(t, int64(2), keep)
	assert.Equal(t, []int64{4}, drops)
	assert.Equal(t, int64(2), choices["linkedin"])
	assert.Equal(t, int64(4), choices["twitter"])

	// Dropping both links removes the kind from the result.
	press(m, " ")
	assert.False(t, c.included(oldL))
	_, _, choices, opts = c.plan()
	assert.NotContains(t, choices, "linkedin")
	assert.Equal(t, []string{"linkedin"}, opts.Exclude["handle"])

	press(m, "m")
	assert.Nil(t, m.cmp)
	keepP := personByID(t, pr, 2)
	kinds := map[string]string{}
	for _, h := range keepP.Handles {
		kinds[h.Kind] = h.Value
	}
	assert.Equal(t, map[string]string{"twitter": "bobby"}, kinds)
}

func TestCompareFlowSummaryThenApply(t *testing.T) {
	m, c := openBobs(t)
	pr := m.pr
	ctx := context.Background()
	d := m.db
	_, err := d.Exec(`INSERT INTO interactions (uuid, type) VALUES ('n1', 'note'), ('c1', 'call'), ('c2', 'call')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO activity_target (uuid, source_kind, source_id, target_kind, target_id) VALUES
		('a1', 'interaction', 1, 'person', 4), ('a2', 'interaction', 2, 'person', 4), ('a3', 'interaction', 3, 'person', 4)`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO tasks (uuid, title) VALUES ('t1', 'Call back')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO activity_target (uuid, source_kind, source_id, target_kind, target_id) VALUES ('a4', 'task', 1, 'person', 4)`)
	require.NoError(t, err)
	// Reopen so the counts are read.
	press(m, "esc", "enter")
	c = m.cmp
	require.NotNil(t, c)
	assert.Equal(t, 1, c.pair(1).notes)
	assert.Equal(t, 2, c.pair(1).interactions)
	assert.Equal(t, 1, c.pair(1).tasks)
	assert.Equal(t, "2", c.finalText(rowIndex(c, "interactions")), "count rows sum both sides")

	c.cur = rowIndex(c, "company")
	press(m, "left")
	press(m, "enter")
	require.NotNil(t, m.cmp)
	assert.True(t, c.confirm)
	out := ansiStrip(m.View())
	for _, want := range []string{"Merge #4 bob baker into #2 Bob Baker", "kept", "archived", "restorable", "merged into #2", "notes", "1 move to #2", "interactions", "2 move to #2", "tasks", "enter merges"} {
		assert.Contains(t, out, want)
	}
	assert.Contains(t, out, "title", "the changed fields are listed")

	// Esc goes back to the fields, nothing is merged.
	press(m, "esc")
	require.NotNil(t, m.cmp)
	assert.False(t, c.confirm)
	assert.Empty(t, archivedIDs(t, pr))

	// Navigation keys do not edit while the summary shows.
	press(m, "enter")
	before := c.resolved(rowIndex(c, "company"))
	press(m, "right", "left", " ", "s")
	assert.Equal(t, before, c.resolved(rowIndex(c, "company")))
	assert.Equal(t, 0, c.s)

	press(m, "enter")
	assert.Nil(t, m.cmp)
	assert.Contains(t, m.status, "merged 1 into #2")
	assert.False(t, m.statusIsErr)
	assert.True(t, archivedIDs(t, pr)[4])
	keep := personByID(t, pr, 2)
	assert.Equal(t, "Globex", str(keep.Company), "the pick on the screen was saved")
	assert.Equal(t, "CTO", str(keep.Title), "the default took the non-empty title")
	assert.Equal(t, "Austin", str(keep.Location))
	assert.Equal(t, "bob baker", keep.FirstName+" "+str(keep.LastName), "the newer name won the conflict")
	var moved int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM activity_target WHERE source_kind = 'interaction' AND target_kind = 'person' AND target_id = 2 AND deleted_at IS NULL`).Scan(&moved))
	assert.Equal(t, 3, moved)
	_ = ctx
}

func TestCompareMergeMatchesTheResultPane(t *testing.T) {
	m, c := openBobs(t)
	pr := m.pr
	// Drop an email and a tag, pick candidate 1's name, make candidate 2 the survivor.
	c.cur = itemRow(c, "email", "bob@home.org")
	press(m, " ")
	c.cur = itemRow(c, "tag", "friend")
	press(m, " ")
	c.cur = rowIndex(c, "name")
	press(m, "left")
	press(m, "s")
	assert.Equal(t, 1, c.s)

	var want []string
	for i := range c.rows {
		if c.rows[i].kind == rowItem && c.finalText(i) != "" {
			want = append(want, c.finalText(i))
		}
	}
	assert.Equal(t, []string{"bob@globex.com"}, want)

	press(m, "enter", "enter")
	assert.Nil(t, m.cmp)
	assert.True(t, archivedIDs(t, pr)[2], "the old survivor is archived")
	keep := personByID(t, pr, 4)
	assert.Equal(t, "Bob", keep.FirstName)
	assert.Equal(t, "Baker", str(keep.LastName))
	var emails []string
	for _, e := range keep.Emails {
		emails = append(emails, e.Email)
	}
	assert.Equal(t, []string{"bob@globex.com"}, emails)
	tags, err := m.tagr.GetForEntity(context.Background(), "person", 4)
	require.NoError(t, err)
	assert.Empty(t, tags)
	assert.Equal(t, "10-20", formatBirthdayParts(keep), "birthday only #2 had, default non-empty")
}

func TestCompareChangeSurvivorAndEsc(t *testing.T) {
	m, c := openBobs(t)
	press(m, "s")
	assert.Equal(t, 1, c.s)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	assert.Contains(t, ansiStrip(m.View()), "Final #4")
	for _, k := range []string{"esc", "q"} {
		press(m, k)
		assert.Nil(t, m.cmp, k)
		assert.Empty(t, archivedIDs(t, m.pr))
		press(m, "enter")
		require.NotNil(t, m.cmp)
		c = m.cmp
	}
}

func TestCompareSwallowsOtherKeys(t *testing.T) {
	m, _ := openBobs(t)
	for _, k := range []string{"c", "f", "n", "t", "#", "T", "x", "+", "u", "M", "3", "4"} {
		press(m, k)
		assert.NotNil(t, m.cmp, k)
		assert.Equal(t, modeNormal, m.mode, k)
		assert.Equal(t, viewMerge, m.view, k)
	}
	assert.Empty(t, archivedIDs(t, m.pr))
	assert.False(t, personByID(t, m.pr, 3).IsFavorite)
}

func TestCompareNarrowResultToggle(t *testing.T) {
	m, c := openBobs(t)
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	out := ansiStrip(m.View())
	assert.Contains(t, out, "#2 Bob Baker")
	assert.NotContains(t, out, "Final #2", "two panes show the candidates")
	press(m, "r")
	out = ansiStrip(m.View())
	assert.Contains(t, out, "Final #2")
	assert.NotContains(t, out, "#4 bob baker\n")
	press(m, "r")
	assert.Contains(t, ansiStrip(m.View()), "#4 bob baker")
	assert.Equal(t, 2, c.page)

	// The footer offers r only when panes are hidden.
	hints := func() string { return ansiStrip(m.hintsLine()) }
	assert.Contains(t, hints(), "r pane")
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	assert.NotContains(t, hints(), "r pane")
	assert.Contains(t, hints(), "enter merge")
	assert.Contains(t, hints(), "esc back")
}

func TestCompareClickPicksAndToggles(t *testing.T) {
	m, c := openBobs(t)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	l := c.layout(140)
	require.Equal(t, []int{0, 1, 2}, l.panes)
	_, rowAt := c.render(140, m.bodyHeight())
	lineOf := func(row int) int {
		for n, r := range rowAt {
			if r == row {
				return n + 1 // the title line is above the body
			}
		}
		return -1
	}
	company := rowIndex(c, "company")
	click(m, l.x0[0]+2, lineOf(company))
	assert.Equal(t, company, c.cur)
	assert.Equal(t, 0, c.resolved(company))
	click(m, l.x0[1]+2, lineOf(company))
	assert.Equal(t, 1, c.resolved(company))
	click(m, l.x0[2]+2, lineOf(company))
	assert.Equal(t, 1, c.resolved(company), "the result pane is not clickable")

	home := itemRow(c, "email", "bob@home.org")
	click(m, l.x0[0]+2, lineOf(home))
	assert.False(t, c.included(home))
	click(m, l.x0[0]+2, lineOf(home))
	assert.True(t, c.included(home))
	m.Update(tea.MouseMsg{X: 3, Y: 5, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	assert.Greater(t, c.cur, home)
}

func TestCompareGroupOfThreeMergesPairByPair(t *testing.T) {
	m, pr := dupeModel(t)
	ctx := context.Background()
	third, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Robert", LastName: sp("Baker"), Email: sp("bobby@elsewhere.net")})
	require.NoError(t, err)
	require.NoError(t, m.reload())
	m.openCompare([]int64{2, 4, third.ID}, 0, []string{"test group"}, true)
	c := m.cmp
	require.NotNil(t, c)
	require.Len(t, c.members, 3)
	assert.Equal(t, []int{0, 1}, []int{c.a, c.b})
	assert.Contains(t, ansiStrip(m.View()), "group of 3")
	press(m, "p")
	assert.Equal(t, []int{0, 2}, []int{c.a, c.b})
	assert.GreaterOrEqual(t, itemRow(c, "email", "bobby@elsewhere.net"), 0)
	press(m, "p")
	assert.Equal(t, []int{1, 2}, []int{c.a, c.b})
	press(m, "p")
	assert.Equal(t, []int{0, 1}, []int{c.a, c.b}, "wraps")

	press(m, "p", "m")
	assert.Nil(t, m.cmp)
	assert.True(t, archivedIDs(t, pr)[third.ID])
	assert.False(t, archivedIDs(t, pr)[4], "the third person is untouched")
}

func TestCompareDefaultsHelper(t *testing.T) {
	c := &compareState{members: []cmpMember{{id: 1, updated: "2026-01-01"}, {id: 2, updated: "2026-02-01"}}, b: 1}
	assert.Equal(t, 1, c.defaultSide([2]string{"a", "b"}, false), "newer record")
	assert.Equal(t, 0, c.defaultSide([2]string{"a", ""}, false), "non-empty")
	assert.Equal(t, 1, c.defaultSide([2]string{"", "b"}, false))
	assert.Equal(t, 0, c.defaultSide([2]string{"2026-05-01", "2026-04-01"}, true), "later date")
	c.members[1].updated = "2026-01-01"
	assert.Equal(t, 0, c.defaultSide([2]string{"a", "b"}, false), "tie goes to the survivor")
	c.s = 1
	assert.Equal(t, 1, c.defaultSide([2]string{"a", "b"}, false))
}

func TestCompareCustomFieldsPickAndSkip(t *testing.T) {
	m, pr := dupeModel(t)
	_, err := m.db.Exec(`INSERT INTO custom_fields (entity_type, entity_id, field_name, field_value) VALUES
		('person', 2, 'pet', 'cat'), ('person', 4, 'pet', 'dog'), ('person', 4, 'car', 'vw'), ('person', 4, 'boat', 'ib31')`)
	require.NoError(t, err)
	_, err = m.db.Exec(`UPDATE people SET updated_at = '2026-01-01 10:00:00' WHERE id = 2`)
	require.NoError(t, err)
	_, err = m.db.Exec(`UPDATE people SET updated_at = '2026-02-01 10:00:00' WHERE id = 4`)
	require.NoError(t, err)
	m.openCompare([]int64{2, 4}, 0, []string{"test"}, true)
	c := m.cmp
	require.NotNil(t, c)
	cat, dog := itemRow(c, "field", "pet: cat"), itemRow(c, "field", "pet: dog")
	car, boat := itemRow(c, "field", "car: vw"), itemRow(c, "field", "boat: ib31")
	require.True(t, cat >= 0 && dog >= 0 && car >= 0 && boat >= 0)
	assert.True(t, c.included(dog), "newer record's value wins the slot")
	assert.False(t, c.included(cat))
	c.cur = cat
	press(m, "left")
	c.cur = boat
	press(m, " ")
	press(m, "m")
	require.Nil(t, m.cmp)
	got := map[string]string{}
	rows, err := m.db.Query(`SELECT field_name, field_value FROM custom_fields WHERE entity_type = 'person' AND entity_id = 2`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var k, v string
		require.NoError(t, rows.Scan(&k, &v))
		got[k] = v
	}
	assert.Equal(t, map[string]string{"pet": "cat", "car": "vw"}, got)
	_ = pr
}
