package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dealWithPerson makes a deal with person 1 on it and opens the deal page with the cursor on the
// people row.
func dealWithPerson(t *testing.T, m *Model, role string) (dealID int64) {
	t.Helper()
	ctx := context.Background()
	d, err := m.dlr.Create(ctx, model.CreateDealInput{Title: "Site", Stage: "lead"})
	require.NoError(t, err)
	var attrs []model.LinkAttrs
	raw := strings.HasPrefix(role, "raw:")
	role = strings.TrimPrefix(role, "raw:")
	if role != "" && !raw {
		attrs = append(attrs, model.LinkAttrs{Role: &role, CreateRole: true})
	}
	_, err = m.links.Link(ctx, refOf("deal", d.ID), personRef(1), attrs...)
	require.NoError(t, err)
	if raw {
		// A role written before the list existed: straight into the table.
		_, err = m.db.Exec(`UPDATE deal_person SET role = ? WHERE deal_id = ?`, role, d.ID)
		require.NoError(t, err)
	}
	goDeals(m)
	m.cursorOn(func(r row) bool { return r.dealID == d.ID })
	press(m, "tab")
	focusLabel(t, m, "person")
	return d.ID
}

func dealRole(t *testing.T, m *Model, dealID int64) string {
	t.Helper()
	items, err := m.links.Linked(context.Background(), refOf("deal", dealID), "person")
	require.NoError(t, err)
	require.Len(t, items, 1)
	if items[0].Role == nil {
		return ""
	}
	return *items[0].Role
}

func roleList(t *testing.T, m *Model) []string {
	t.Helper()
	n, err := m.vr.Names(context.Background(), model.DealRoleKind)
	require.NoError(t, err)
	return n
}

// openRoleCell moves to the deal role cell of the focused people row and opens the editor.
func openRoleCell(t *testing.T, m *Model) {
	t.Helper()
	press(m, "right")
	require.Equal(t, 1, m.fcol)
	press(m, "enter")
	require.NotNil(t, m.edit, "enter opens the editor of the cell")
	require.NotNil(t, m.edit.typed)
	require.NotNil(t, m.edit.typed.vocab, "the deal role is a vocabulary choice field")
}

func itemLabels(m *Model) []string {
	var out []string
	for _, it := range m.edit.typed.vocab.items {
		out = append(out, it.label)
	}
	return out
}

func TestDealRoleCellIsAVocabularyChoiceWithPickOrCreate(t *testing.T) {
	tests := []struct {
		name     string
		start    string // role on the link before the edit
		keys     func(m *Model)
		wantRole string
		wantList func(t *testing.T, got []string)
	}{
		{"typing filters and enter picks", "", func(m *Model) { typeText(m, "econ"); press(m, "enter") }, "economic buyer", nil},
		{"up and down pick", "", func(m *Model) { press(m, "down", "down"); press(m, "enter") }, "economic buyer", nil},
		{"tab completes the highlighted name", "", func(m *Model) { typeText(m, "sig"); press(m, "tab"); press(m, "enter") }, "signer", nil},
		{"substring matches too", "", func(m *Model) { typeText(m, "buyer"); press(m, "enter") }, "economic buyer", nil},
		{"unknown text offers Create role X and enter creates it", "", func(m *Model) { typeText(m, "king  maker"); press(m, "enter") }, "king maker",
			func(t *testing.T, got []string) {
				assert.Equal(t, "king maker", got[len(got)-1], "added at the end of the list")
			}},
		{"a match does not offer create", "", func(m *Model) { typeText(m, "champion"); press(m, "enter") }, "champion",
			func(t *testing.T, got []string) { assert.Len(t, got, 8) }},
		{"esc cancels", "champion", func(m *Model) { typeText(m, "blocker"); press(m, "esc") }, "champion", nil},
		{"the clear row empties the role", "champion", func(m *Model) {
			press(m, "down", "down", "down", "down", "down", "down", "down", "down")
			press(m, "enter")
		}, "", nil},
		{"a pipe is not a valid role name", "", func(m *Model) { typeText(m, "a|b"); press(m, "enter") }, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestModel(t)
			id := dealWithPerson(t, m, tt.start)
			openRoleCell(t, m)
			tt.keys(m)
			assert.Equal(t, tt.wantRole, dealRole(t, m, id))
			if tt.wantList != nil {
				tt.wantList(t, roleList(t, m))
			}
		})
	}
}

func TestDealRoleCellShowsTheOffer(t *testing.T) {
	m, _ := newTestModel(t)
	dealWithPerson(t, m, "")
	openRoleCell(t, m)
	assert.Len(t, itemLabels(m), 8, "an empty input lists every role in pick order")
	assert.Equal(t, "champion", itemLabels(m)[0])
	typeText(m, "kingmaker")
	assert.Equal(t, []string{"Create role kingmaker"}, itemLabels(m))
	view := plain(m.detail.View())
	assert.Contains(t, view, "Create role kingmaker")
	assert.Contains(t, plain(m.hintsLine()), "type to filter")
	typeText(m, "")
	press(m, "ctrl+u")
	typeText(m, "buyer")
	assert.Equal(t, []string{"economic buyer", "Create role buyer"}, itemLabels(m), "a partial match still offers to create the typed name")
	press(m, "ctrl+u")
	typeText(m, "ZZZ")
	assert.Equal(t, []string{"Create role ZZZ"}, itemLabels(m))
}

func TestCreatingARoleInTheCellIsOneUndoStep(t *testing.T) {
	m, _ := newTestModel(t)
	id := dealWithPerson(t, m, "")
	before := roleList(t, m)
	openRoleCell(t, m)
	typeText(m, "kingmaker")
	press(m, "enter")
	assert.False(t, m.statusIsErr, m.status)
	assert.Equal(t, "kingmaker", dealRole(t, m, id))
	assert.Contains(t, roleList(t, m), "kingmaker")

	press(m, "ctrl+z")
	assert.Equal(t, "", dealRole(t, m, id), "undo takes the role off the link")
	assert.Equal(t, before, roleList(t, m), "and off the list, in the same step")
	press(m, "ctrl+y")
	assert.Equal(t, "kingmaker", dealRole(t, m, id))
	assert.Contains(t, roleList(t, m), "kingmaker")
}

func TestLegacyFreeTextRoleShowsAsWrittenAndStaysUntilChanged(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	id := dealWithPerson(t, m, "raw:Board Observer")
	var err error
	_ = err
	assert.Contains(t, plain(m.detail.View()), "Board Observer", "a role not on the list shows as written")
	assert.Equal(t, "Board Observer", dealRole(t, m, id))

	openRoleCell(t, m)
	press(m, "esc")
	assert.Equal(t, "Board Observer", dealRole(t, m, id), "opening and cancelling keeps it")
	names, err := m.vr.Names(ctx, model.DealRoleKind)
	require.NoError(t, err)
	assert.NotContains(t, names, "Board Observer", "the list is not touched without a backfill")

	openRoleCell(t, m)
	typeText(m, "champ")
	press(m, "enter")
	assert.Equal(t, "champion", dealRole(t, m, id))
	press(m, "ctrl+z")
	assert.Equal(t, "Board Observer", dealRole(t, m, id), "undo restores the free text exactly")
}

func TestLinkPromptOffersToCreateAnUnlistedDealRole(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	d, err := m.dlr.Create(ctx, model.CreateDealInput{Title: "Site", Stage: "lead"})
	require.NoError(t, err)
	_ = d
	runPalette(m, "link deal")
	typeText(m, "site")
	press(m, "enter")
	require.Equal(t, modePrompt, m.mode)
	submit(m, "kingmaker")
	require.Equal(t, modePrompt, m.mode, "the first enter offers the create and asks again")
	assert.Contains(t, m.status, "Create role kingmaker")
	assert.NotContains(t, roleList(t, m), "kingmaker")
	press(m, "enter")
	items, err := m.links.Linked(ctx, personRef(1), "deal")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "kingmaker", *items[0].Role)
	assert.Contains(t, roleList(t, m), "kingmaker")

	press(m, "ctrl+z")
	items, _ = m.links.Linked(ctx, personRef(1), "deal")
	assert.Empty(t, items, "one undo step unlinks")
	assert.NotContains(t, roleList(t, m), "kingmaker", "and removes the new role")
}

func TestLinkPromptKeepsMembershipRolesFreeText(t *testing.T) {
	m, _ := newTestModel(t)
	_, err := m.orr.Create(context.Background(), model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	runPalette(m, "link organization")
	typeText(m, "acme")
	press(m, "enter")
	submit(m, "Chief Whatever Officer")
	assert.Equal(t, modeNormal, m.mode, "no create offer for a company role")
	assert.NotContains(t, strings.Join(roleList(t, m), ","), "Chief")
}

func TestSetFieldOnSelectedSetsTheDealRoleOfPeople(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	d, err := m.dlr.Create(ctx, model.CreateDealInput{Title: "Site", Stage: "lead"})
	require.NoError(t, err)
	for _, pid := range []int64{1, 2, 3} {
		_, err = m.links.Link(ctx, refOf("deal", d.ID), personRef(pid))
		require.NoError(t, err)
	}
	roles := func() []string {
		items, err := m.links.Linked(ctx, refOf("deal", d.ID), "person")
		require.NoError(t, err)
		out := make([]string, len(items))
		for i, it := range items {
			out[i] = str(it.Role)
		}
		return out
	}

	press(m, "ctrl+a")
	runID(m, "set-field")
	require.NotNil(t, m.ov)
	var labels []string
	for _, it := range m.ov.items {
		labels = append(labels, it.label)
	}
	assert.Contains(t, labels, "deal role", "the deal role is a settable field like every choice field")
	typeText(m, "deal role")
	press(m, "enter")
	require.NotNil(t, m.ov, "asks which deal first")
	assert.Contains(t, m.ov.title, "which deal")
	press(m, "enter")
	require.NotNil(t, m.ov)
	assert.Contains(t, m.ov.title, "Set deal role")
	typeText(m, "champ")
	press(m, "enter")
	assert.Equal(t, []string{"champion", "champion", "champion"}, roles())
	assert.Empty(t, m.marked)

	press(m, "ctrl+z")
	assert.Equal(t, []string{"", "", ""}, roles(), "one undo step for the whole set")

	// Pick or create: a name that is not listed gets the last row.
	press(m, "ctrl+a")
	runID(m, "set-field")
	typeText(m, "deal role")
	press(m, "enter")
	press(m, "enter")
	typeText(m, "kingmaker")
	var last overlayItem
	for _, i := range m.ov.shown {
		last = m.ov.items[i]
	}
	assert.Equal(t, "Create role kingmaker", last.label)
	press(m, "enter")
	assert.Equal(t, []string{"kingmaker", "kingmaker", "kingmaker"}, roles())
	assert.Contains(t, roleList(t, m), "kingmaker")
	press(m, "ctrl+z")
	assert.NotContains(t, roleList(t, m), "kingmaker", "creating the role and setting it undo together")
}

func TestSetFieldDealRoleNeedsADeal(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+a")
	runID(m, "set-field")
	typeText(m, "deal role")
	press(m, "enter")
	assert.True(t, m.statusIsErr)
	assert.Contains(t, m.status, "no deal")
}

func refOf(kind string, id int64) repo.Ref { return repo.Ref{Kind: kind, ID: id} }
