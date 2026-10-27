package tui

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func primaryOrgOf(t *testing.T, m *Model, personID int64) string {
	t.Helper()
	name, err := repo.PrimaryOrgName(context.Background(), m.db, personID)
	require.NoError(t, err)
	return name
}

func liveOrgCount(t *testing.T, m *Model) int {
	t.Helper()
	var n int
	require.NoError(t, m.db.QueryRow(`SELECT COUNT(*) FROM organizations WHERE archived = 0`).Scan(&n))
	return n
}

// Editing the company in the person form creates the organization, and ctrl+z undoes the edit and the
// creation as one step. ctrl+y repeats both.
func TestFormCompanyEditCreatesOrganizationAsOneUndoStep(t *testing.T) {
	m, pr := newTestModel(t)
	before := len(m.hist)
	press(m, "home", "tab")
	goTo(t, m, "company")
	press(m, "enter")
	replaceText(m, "Initrode")
	press(m, "enter")
	assert.Equal(t, "Initrode", *personByID(t, pr, 1).Company)
	assert.Equal(t, "Initrode", primaryOrgOf(t, m, 1))
	assert.Equal(t, 1, liveOrgCount(t, m))
	assert.Equal(t, before+1, len(m.hist), "one undo step")

	press(m, "ctrl+z")
	assert.Equal(t, "Acme", *personByID(t, pr, 1).Company)
	assert.Equal(t, "", primaryOrgOf(t, m, 1))
	assert.Equal(t, 0, liveOrgCount(t, m))

	press(m, "ctrl+y")
	assert.Equal(t, "Initrode", *personByID(t, pr, 1).Company)
	assert.Equal(t, "Initrode", primaryOrgOf(t, m, 1))
	assert.Equal(t, 1, liveOrgCount(t, m))
}

// A second person with the same company text reuses the organization and a junk string makes none.
func TestFormCompanyEditReusesAndIgnores(t *testing.T) {
	m, _ := newTestModel(t)
	edit := func(text string) {
		press(m, "home", "tab")
		goTo(t, m, "company")
		press(m, "enter")
		replaceText(m, text)
		press(m, "enter")
		press(m, "esc")
	}
	edit("Initrode Inc")
	assert.Equal(t, 1, liveOrgCount(t, m))
	press(m, "down")
	edit("INITRODE")
	assert.Equal(t, 1, liveOrgCount(t, m), "reused")
	press(m, "down")
	edit("N/A")
	assert.Equal(t, 1, liveOrgCount(t, m), "junk makes none")
}

// The Organizations view shows a near-duplicate name beside the row and on the page, and no key for the
// removed company review is left in the registry.
func TestOrgsViewShowsSimilarNamesAndTheReviewIsGone(t *testing.T) {
	m, _ := newTestModel(t)
	oa := repo.NewOrgAutoRepo(m.db)
	for _, c := range []string{"Acme Corp", "Acme Corpp"} {
		_, err := oa.ApplyCompany(m.ctx, 1, c, repo.ModePrimary)
		require.NoError(t, err)
	}
	m.switchView(viewOrgs)
	out := plain(m.View())
	assert.Contains(t, out, "~ Acme Corp")
	for _, a := range buildActions() {
		assert.NotContains(t, a.group, "Company review")
		assert.NotEqual(t, "convert-open", a.id)
	}
}

// The palette backfill confirms with the counts, writes one undo step and is safe to repeat.
func TestPaletteBackfillIsOneUndoStep(t *testing.T) {
	m, _ := newTestModel(t) // Acme, Globex and Initech are company text only
	before := len(m.hist)
	m.askBackfill()
	require.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, "create 3 organizations")
	press(m, "y")
	assert.Equal(t, 3, liveOrgCount(t, m))
	assert.Equal(t, "Acme", primaryOrgOf(t, m, 1))
	assert.Equal(t, before+1, len(m.hist))

	press(m, "ctrl+z")
	assert.Equal(t, 0, liveOrgCount(t, m))
	assert.Equal(t, "", primaryOrgOf(t, m, 1))
	press(m, "ctrl+y")
	assert.Equal(t, 3, liveOrgCount(t, m))

	m.askBackfill()
	assert.NotEqual(t, modeConfirm, m.mode, "nothing left to do")
}

func TestPaletteIgnoreListAddsAndTurnsOff(t *testing.T) {
	m, _ := newTestModel(t)
	m.setIgnore("Gig Work", true)
	ig := repo.NewIgnoreRepo(m.db)
	ok, err := ig.Ignored(m.ctx, "gig work")
	require.NoError(t, err)
	assert.True(t, ok)
	press(m, "ctrl+z")
	ok, _ = ig.Ignored(m.ctx, "gig work")
	assert.False(t, ok, "undo takes it off the list")
	m.setIgnore("retired", false)
	ok, _ = ig.Ignored(m.ctx, "retired")
	assert.False(t, ok)
	press(m, "ctrl+z")
	ok, _ = ig.Ignored(m.ctx, "retired")
	assert.True(t, ok)
}
