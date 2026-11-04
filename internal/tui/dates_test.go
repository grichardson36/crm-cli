package tui

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/keys"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/modules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CRM-022 Anniversaries: the Dates view lists birthdays and the anniversaries of spouse links. The test
// clock is 2026-06-17. Bob Baker (--10-20) is in the base fixture.

func datesModel(t *testing.T, anniversary string) (*Model, int64, int64) {
	t.Helper()
	m, pr := newTestModel(t)
	m.now = func() time.Time { return time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC) }
	ctx := context.Background()
	joey, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Joey", LastName: sp("Osgood")})
	require.NoError(t, err)
	hannah, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Hannah", LastName: sp("Osgood")})
	require.NoError(t, err)
	_, err = pr.Update(ctx, hannah.ID, model.UpdatePersonInput{Birthday: sp("1988-06-20")})
	require.NoError(t, err)
	at := model.LinkAttrs{Kind: sp("spouse")}
	if anniversary != "" {
		at.Anniversary = sp(anniversary)
	}
	_, err = m.links.Link(ctx, repo.Ref{Kind: "person", ID: joey.ID}, repo.Ref{Kind: "person", ID: hannah.ID}, at)
	require.NoError(t, err)
	m.switchView(viewDates)
	return m, joey.ID, hannah.ID
}

// dateRowsSummary is the list as text: "month:", "day:date|relative" and "title|meta".
func dateRowsSummary(m *Model) []string {
	var out []string
	for _, r := range m.rows {
		switch {
		case r.rule:
			out = append(out, "day:"+r.title+"|"+r.meta)
		case r.skip:
			out = append(out, "month:"+r.title)
		default:
			out = append(out, r.title+"|"+r.meta)
		}
	}
	return out
}

func TestDatesViewS1AnniversaryRowUnderItsDayWithYears(t *testing.T) {
	m, joey, _ := datesModel(t, "2014-06-20")
	assert.Equal(t, "Dates", viewNames[viewDates])
	assert.Equal(t, []string{
		"month:JUNE",
		"day:2026-06-20|in 3d",
		"Hannah Osgood|38 yrs",
		"Joey-Hannah Osgood Anniversary|12 yrs",
		"month:OCTOBER",
		"day:2026-10-20|in 4mo 3d",
		"Bob Baker|",
	}, dateRowsSummary(m), "birthdays come before anniversaries on the same day")

	for _, r := range m.rows {
		if r.title == "Joey-Hannah Osgood Anniversary" {
			assert.Equal(t, joey, r.personID, "the row opens the first partner")
			assert.False(t, r.skip)
		}
	}
	assert.Contains(t, m.screen(), "Joey-Hannah Osgood Anniversary")
	assert.Contains(t, m.screen(), "12 yrs")
}

func TestDatesViewS1OneYearReadsOneYr(t *testing.T) {
	m, _, _ := datesModel(t, "2025-06-20")
	assert.Contains(t, dateRowsSummary(m), "Joey-Hannah Osgood Anniversary|1 yr")
}

func TestDatesViewS2YearlessAnniversaryHasNoMeta(t *testing.T) {
	m, _, _ := datesModel(t, "--06-20")
	assert.Contains(t, dateRowsSummary(m), "Joey-Hannah Osgood Anniversary|")
}

func TestDatesViewS3ShowOptionFiltersByKind(t *testing.T) {
	m, _, _ := datesModel(t, "2014-06-20")
	opts := viewOptionsFor(viewDates)
	require.Len(t, opts, 1)
	assert.Equal(t, "Show", opts[0].label)
	assert.Equal(t, "all", opts[0].get(m), "the default shows both")

	press(m, "ctrl+o")
	require.NotNil(t, m.ov)
	assert.Contains(t, m.ov.title, "View options, Dates")
	press(m, "right") // birthdays
	assert.Equal(t, "birthdays", m.datesShow)
	assert.NotContains(t, dateRowsSummary(m), "Joey-Hannah Osgood Anniversary|12 yrs")
	assert.Contains(t, dateRowsSummary(m), "Hannah Osgood|38 yrs")
	assert.Contains(t, dateRowsSummary(m), "Bob Baker|")

	press(m, "right") // anniversaries
	assert.Equal(t, "anniversaries", m.datesShow)
	assert.Equal(t, []string{"month:JUNE", "day:2026-06-20|in 3d", "Joey-Hannah Osgood Anniversary|12 yrs"}, dateRowsSummary(m))

	press(m, "right") // all again
	assert.Equal(t, "all", opts[0].get(m))
	assert.Len(t, dateRowsSummary(m), 7)
	press(m, "esc")
	assert.Equal(t, modeNormal, m.mode)
}

func TestDatesViewS4ExclusionsAndOtherKinds(t *testing.T) {
	ctx := context.Background()
	t.Run("different last names", func(t *testing.T) {
		m, pr := newTestModel(t)
		m.now = func() time.Time { return time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC) }
		a, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Joey", LastName: sp("Osgood")})
		b, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Hannah", LastName: sp("Smith")})
		_, err := m.links.Link(ctx, repo.Ref{Kind: "person", ID: a.ID}, repo.Ref{Kind: "person", ID: b.ID}, model.LinkAttrs{Kind: sp("spouse"), Anniversary: sp("2014-06-20")})
		require.NoError(t, err)
		m.switchView(viewDates)
		assert.Contains(t, dateRowsSummary(m), "Joey Osgood-Hannah Smith Anniversary|12 yrs")
	})
	t.Run("an archived partner and a deleted link", func(t *testing.T) {
		m, joey, hannah := datesModel(t, "2014-06-20")
		require.NoError(t, m.pr.Archive(ctx, hannah))
		m.rebuild(false)
		assert.NotContains(t, dateRowsSummary(m), "Joey-Hannah Osgood Anniversary|12 yrs")

		m, joey, hannah = datesModel(t, "2014-06-20")
		_, err := m.links.Unlink(ctx, repo.Ref{Kind: "person", ID: joey}, repo.Ref{Kind: "person", ID: hannah})
		require.NoError(t, err)
		m.rebuild(false)
		assert.NotContains(t, dateRowsSummary(m), "Joey-Hannah Osgood Anniversary|12 yrs")
	})
	t.Run("other kinds never appear", func(t *testing.T) {
		m, pr := newTestModel(t)
		m.now = func() time.Time { return time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC) }
		a, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Joey", LastName: sp("Osgood")})
		b, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Jude", LastName: sp("Osgood")})
		for _, kind := range []string{"parent", "friend", "colleague"} {
			_, err := m.links.Link(ctx, repo.Ref{Kind: "person", ID: a.ID}, repo.Ref{Kind: "person", ID: b.ID}, model.LinkAttrs{Kind: sp(kind)})
			require.NoError(t, err)
		}
		m.switchView(viewDates)
		for _, s := range dateRowsSummary(m) {
			assert.NotContains(t, s, "Anniversary")
		}
	})
}

func TestDatesViewEnterOnAnAnniversaryOpensTheFirstPartner(t *testing.T) {
	m, joey, _ := datesModel(t, "2014-06-20")
	for m.rows[m.cursor].title != "Joey-Hannah Osgood Anniversary" {
		press(m, "down")
	}
	press(m, "enter")
	assert.Equal(t, focusDetail, m.focus)
	assert.Equal(t, joey, m.formPerson)
}

// linkPersonAs runs the Link form from Zed Adams to Bob Baker and picks a kind.
func linkPersonAs(t *testing.T, m *Model, kind string) {
	t.Helper()
	runPalette(m, "link person")
	typeText(m, "bob")
	press(m, "enter")
	require.NotNil(t, m.ov, "the kind picker follows the person picker")
	typeText(m, kind)
	press(m, "enter")
}

func zedBobLinks(t *testing.T, m *Model) []model.LinkedObject {
	t.Helper()
	items, err := m.links.Linked(context.Background(), repo.Ref{Kind: "person", ID: 1}, "person")
	require.NoError(t, err)
	return items
}

func TestLinkFlowS5SpouseAsksForTheAnniversary(t *testing.T) {
	t.Run("blank skips", func(t *testing.T) {
		m, _ := newTestModel(t)
		linkPersonAs(t, m, "spouse")
		require.Equal(t, modePrompt, m.mode)
		assert.Equal(t, "link-anniversary", m.promptKind)
		assert.Contains(t, m.input.Prompt, "blank skips")
		press(m, "enter")
		assert.Equal(t, modeNormal, m.mode)
		items := zedBobLinks(t, m)
		require.Len(t, items, 1)
		assert.Equal(t, "spouse", items[0].Label)
		assert.Nil(t, items[0].Anniversary)
	})
	t.Run("a date is stored", func(t *testing.T) {
		m, _ := newTestModel(t)
		linkPersonAs(t, m, "spouse")
		typeText(m, "2014-06-20")
		press(m, "enter")
		items := zedBobLinks(t, m)
		require.Len(t, items, 1)
		require.NotNil(t, items[0].Anniversary)
		assert.Equal(t, "2014-06-20", *items[0].Anniversary)
	})
	t.Run("invalid text is refused with a message and asked again", func(t *testing.T) {
		m, _ := newTestModel(t)
		linkPersonAs(t, m, "spouse")
		typeText(m, "soon")
		press(m, "enter")
		require.Equal(t, modePrompt, m.mode, "the prompt stays open")
		assert.Equal(t, "link-anniversary", m.promptKind)
		assert.Contains(t, m.status, "invalid anniversary")
		assert.Empty(t, zedBobLinks(t, m), "nothing is written yet")
		press(m, "ctrl+u")
		typeText(m, "--06-20")
		press(m, "enter")
		items := zedBobLinks(t, m)
		require.Len(t, items, 1)
		assert.Equal(t, "--06-20", *items[0].Anniversary)
	})
	t.Run("esc leaves no trace", func(t *testing.T) {
		m, _ := newTestModel(t)
		linkPersonAs(t, m, "spouse")
		press(m, "esc")
		assert.Empty(t, zedBobLinks(t, m))
	})
	t.Run("parent asks nothing", func(t *testing.T) {
		m, _ := newTestModel(t)
		linkPersonAs(t, m, "parent")
		assert.Equal(t, modeNormal, m.mode)
		items := zedBobLinks(t, m)
		require.Len(t, items, 1)
		assert.Nil(t, items[0].Anniversary)
	})
	t.Run("a user kind asks nothing even when it is called spouse-like", func(t *testing.T) {
		m, _ := newTestModel(t)
		_, err := repo.NewKindRepo(m.db).Add(context.Background(), "person", "life partner", "")
		require.NoError(t, err)
		linkPersonAs(t, m, "life partner")
		assert.Equal(t, modeNormal, m.mode)
		assert.Len(t, zedBobLinks(t, m), 1)
	})
}

func TestLinkFlowS6EditTheDateOnTheLinkRowAndUndoIt(t *testing.T) {
	m, _ := newTestModel(t)
	m.now = func() time.Time { return time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC) }
	linkPersonAs(t, m, "spouse")
	press(m, "enter") // no anniversary yet

	press(m, "tab") // into the detail form of Zed Adams
	idx := -1
	for i, r := range m.form {
		if r.kind == rkLinked && r.other == "person" && !r.empty {
			idx = i
		}
	}
	require.GreaterOrEqual(t, idx, 0, "the link row is on the person page")
	m.fcur = idx
	m.syncDetail()
	row := m.form[m.fcur]
	var keys []string
	for _, c := range row.cells {
		keys = append(keys, c.key)
	}
	assert.Equal(t, []string{"kind", "anniversary", "notes"}, keys, "a spouse link row has an anniversary cell")

	setCell(t, m, 2, "2014-06-20")
	assert.Contains(t, m.status, "saved anniversary")
	items := zedBobLinks(t, m)
	require.NotNil(t, items[0].Anniversary)
	assert.Equal(t, "2014-06-20", *items[0].Anniversary)
	assert.Contains(t, linkSummary(items[0]), "anniversary 2014-06-20")

	m.switchView(viewDates)
	assert.Contains(t, dateRowsSummary(m), "Zed Adams-Bob Baker Anniversary|12 yrs", "the Dates view follows the edit (the partners have different last names)")

	m.switchView(viewPeople)
	press(m, "ctrl+z")
	items = zedBobLinks(t, m)
	assert.Nil(t, items[0].Anniversary, "ctrl+z undoes the edit")
	m.switchView(viewDates)
	for _, s := range dateRowsSummary(m) {
		assert.NotContains(t, s, "Anniversary")
	}
}

func TestLinkRowOfAnotherKindHasNoAnniversaryCell(t *testing.T) {
	m, _ := newTestModel(t)
	linkPersonAs(t, m, "parent")
	items := zedBobLinks(t, m)
	require.Len(t, items, 1)
	for _, c := range m.linkCells(items[0], nil) {
		assert.NotEqual(t, "anniversary", c.key)
	}
	assert.False(t, items[0].CanAnniversary)
}

// S8: a device that stored the arrangement and a key binding under the old Birthdays key keeps them,
// moved to Dates, and the old key is not reported as unknown.
func TestStartupMovesTheStoredBirthdaysArrangementAndKeyToDates(t *testing.T) {
	m, _ := newTestModel(t)
	m.env.DataDir = t.TempDir()
	cfg := "[modules]\norder = [\"home\", \"people\", \"birthdays\", \"tasks\"]\nhidden = [\"birthdays\"]\n\n[keys]\nview-birthdays = \"f9\"\n"
	require.NoError(t, os.WriteFile(modules.ConfigFile(m.env.DataDir), []byte(cfg), 0o644))
	m.applyStartupModules()
	assert.False(t, m.statusIsErr, "no problem is reported: %s", m.status)
	assert.Zero(t, m.mods.Position("dates"), "the module stays hidden under its new key")
	assert.True(t, m.mods.IsHidden("dates"))
	assert.Equal(t, "dates", m.mods.Normalize().Order[2], "and keeps its place in the order")
	o, problems := keys.Load(m.env.DataDir)
	assert.Empty(t, problems)
	assert.Equal(t, []string{"f9"}, o["view-dates"])
	assert.NotNil(t, m.actionByID("view-dates"), "the action id the binding names exists")
	assert.Nil(t, m.actionByID("view-birthdays"))
}

func TestDatesViewKeepsTheCursorOnTodayAfterAnOptionChange(t *testing.T) {
	m, _, _ := datesModel(t, "2014-06-20")
	press(m, "ctrl+o", "right", "esc")
	require.False(t, m.rows[m.cursor].skip)
	assert.Equal(t, "Hannah Osgood", m.rows[m.cursor].title, "the first row from today on")
}
