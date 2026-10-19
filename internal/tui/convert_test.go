package tui

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The standard test data has Zed (company Acme), Bob (Globex) and Cara (Initech).
func convertModel(t *testing.T) *Model {
	t.Helper()
	m, _ := newTestModel(t)
	runPalette(m, "company review")
	require.Equal(t, viewConvert, m.view)
	return m
}

func convKeys(m *Model) []string {
	var out []string
	for _, r := range m.rows {
		out = append(out, r.convKey)
	}
	return out
}

func TestCompanyReviewListsGroupsWithCountsAndCreatesNothing(t *testing.T) {
	m := convertModel(t)
	assert.Equal(t, []string{"acme", "globex", "initech"}, convKeys(m))
	assert.Contains(t, m.View(), "3 people, 3 groups, 0 matched, 0 staged")
	assert.Contains(t, m.detail.View(), "no organization yet")
	assert.Empty(t, openOrgs(t, m), "looking creates nothing")

	// Reachable from the Organizations view with C, and the keys show in the footer.
	runPalette(m, "organizations view")
	press(m, "C")
	assert.Equal(t, viewConvert, m.view)
	foot := m.View()
	for _, want := range []string{"create", "link", "rename", "skip"} {
		assert.Contains(t, foot, want)
	}
}

func TestCompanyReviewStagesThenAppliesOneBatchWithOneUndo(t *testing.T) {
	m := convertModel(t)
	ctx := context.Background()
	exist, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Globex"})
	require.NoError(t, err)
	m.afterWrite()

	// acme: create. globex: link the existing organization. initech: skip.
	press(m, "c")
	assert.Equal(t, 1, len(m.convStaged))
	assert.Contains(t, m.rows[0].meta, "create", "the decision shows on the row")
	assert.Equal(t, "globex", m.rows[m.cursor].convKey, "staging moves down")
	press(m, "l")
	require.NotNil(t, m.ov)
	assert.Contains(t, m.ov.title, "Globex")
	press(m, "enter") // the suggestion is the first row
	assert.Equal(t, repo.ConvertLink, m.convStaged["globex"].Action)
	assert.Equal(t, exist.ID, m.convStaged["globex"].OrgID)
	press(m, "s")
	assert.Empty(t, openOrgsNamed(t, m, "Initech"), "nothing is made before the batch is applied")
	assert.Contains(t, m.View(), "3 staged")

	press(m, "enter")
	require.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, "apply 3 decisions: 2 people, 1 new organizations")
	press(m, "n")
	assert.Equal(t, 3, len(m.convStaged), "no keeps the staged decisions")
	press(m, "enter", "y")

	assert.Empty(t, m.convStaged)
	assert.Equal(t, []string{}, append([]string{}, convKeys(m)...), "all three are converted or skipped")
	assert.Len(t, openOrgs(t, m), 2, "Acme created, Globex reused")
	assert.Equal(t, exist.ID, *personOrg(t, m, 2))
	assert.Equal(t, int64(0), personOrgOr0(t, m, 3), "the skipped group's person has no organization")
	p, err := m.pr.FindByID(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "Acme", *p.Company, "people.company stays")

	// One undo step puts everything back, one redo repeats it.
	press(m, "ctrl+z")
	assert.Len(t, openOrgs(t, m), 1, "the created organization is gone")
	assert.Nil(t, personOrg(t, m, 1))
	assert.Nil(t, personOrg(t, m, 2))
	assert.Equal(t, []string{"acme", "globex", "initech"}, convKeys(m), "the groups are back, the skip is undone too")
	press(m, "ctrl+y")
	assert.Len(t, openOrgs(t, m), 2)
	assert.NotNil(t, personOrg(t, m, 1))
	assert.Empty(t, m.rows)
}

func openOrgsNamed(t *testing.T, m *Model, name string) []*model.Organization {
	t.Helper()
	var out []*model.Organization
	for _, o := range openOrgs(t, m) {
		if o.Name == name {
			out = append(out, o)
		}
	}
	return out
}

func personOrgOr0(t *testing.T, m *Model, id int64) int64 {
	t.Helper()
	if o := personOrg(t, m, id); o != nil {
		return *o
	}
	return 0
}

func TestCompanyReviewRenameCreateAndTypedNewOrganization(t *testing.T) {
	m := convertModel(t)
	press(m, "e")
	require.Equal(t, modePrompt, m.mode)
	assert.Equal(t, "Acme", m.input.Value())
	submit(m, "Acme Corporation")
	assert.Equal(t, "Acme Corporation", m.convStaged["acme"].Name)

	// l with a typed name that matches nobody offers to create it.
	press(m, "l")
	typeText(m, "Brand New Co")
	press(m, "enter")
	assert.Equal(t, repo.ConvertCreate, m.convStaged["globex"].Action)
	assert.Equal(t, "Brand New Co", m.convStaged["globex"].Name)

	press(m, "x") // cursor is on initech now, nothing staged there
	m.cursorOn(func(r row) bool { return r.convKey == "globex" })
	press(m, "x")
	_, staged := m.convStaged["globex"]
	assert.False(t, staged, "x clears a decision")

	m.cursorOn(func(r row) bool { return r.convKey == "acme" })
	press(m, "enter", "y")
	names := []string{}
	for _, o := range openOrgs(t, m) {
		names = append(names, o.Name)
	}
	assert.Equal(t, []string{"Acme Corporation"}, names)
}

func TestCompanyReviewAcceptsExactMatchesInOneConfirmedStep(t *testing.T) {
	m := convertModel(t)
	ctx := context.Background()
	_, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme Inc"})
	require.NoError(t, err)
	_, err = m.orr.Create(ctx, model.CreateOrgInput{Name: "Initech LLC"})
	require.NoError(t, err)
	m.afterWrite()
	assert.Contains(t, m.View(), "3 people, 3 groups, 2 matched")

	press(m, "A")
	require.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, "link 2 exact matches (2 people)")
	press(m, "y")
	assert.Equal(t, []string{"globex"}, convKeys(m), "only the group without a match is left")
	assert.NotNil(t, personOrg(t, m, 1))
	assert.NotNil(t, personOrg(t, m, 3))
	assert.Len(t, openOrgs(t, m), 2, "nothing was created")
	press(m, "ctrl+z")
	assert.Equal(t, 3, len(m.rows))
	assert.Nil(t, personOrg(t, m, 1))
}

func TestCompanyReviewIsBlockedInReadOnlyMode(t *testing.T) {
	m := convertModel(t)
	m.readOnly = true
	press(m, "c")
	assert.Empty(t, m.convStaged)
	assert.False(t, m.actionByID("convert-create").enabled(m) && !m.readOnly)
}
