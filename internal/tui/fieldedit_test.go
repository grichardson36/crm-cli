package tui

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func goOrgs(m *Model) {
	m.mods, _ = m.mods.With("organizations", true)
	m.patchModuleActions()
	runPalette(m, "organizations view")
}

// editField opens the typed row with the label, replaces its text and presses the commit key.
func editField(t *testing.T, m *Model, label, text, commit string) {
	t.Helper()
	if m.focus != focusDetail {
		press(m, "tab")
	}
	focusLabel(t, m, label)
	press(m, "enter")
	require.NotNil(t, m.edit, "enter opens the editor")
	replaceText(m, text)
	press(m, commit)
}

func TestTypedFieldsOnTheOrganizationPage(t *testing.T) {
	m, _ := newTestModel(t)
	o, err := m.orr.Create(context.Background(), model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	goOrgs(m)
	require.Equal(t, viewOrgs, m.view)

	tests := []struct {
		name, label, text, commit string
		check                     func(o *model.Organization)
		wantErr                   string
	}{
		{"text commits with enter", "domain", "acme.com", "enter", func(o *model.Organization) { assert.Equal(t, "acme.com", str(o.Domain)) }, ""},
		{"text commits with tab", "industry", "Software", "tab", func(o *model.Organization) { assert.Equal(t, "Software", str(o.Industry)) }, ""},
		{"location splits into parts", "location", "Austin, TX, US", "enter", func(o *model.Organization) {
			assert.Equal(t, "Austin", str(o.City))
			assert.Equal(t, "TX", str(o.Region))
		}, ""},
		{"longtext", "about", "# Acme", "enter", func(o *model.Organization) { assert.Equal(t, "# Acme", str(o.Description)) }, ""},
		{"required name cannot be emptied", "name", "", "enter", nil, "required"},
		{"name renames", "name", "Acme Corp", "enter", func(o *model.Organization) { assert.Equal(t, "Acme Corp", o.Name) }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editField(t, m, tt.label, tt.text, tt.commit)
			if tt.wantErr != "" {
				assert.True(t, m.statusIsErr, m.status)
				assert.Contains(t, m.status, tt.wantErr)
				assert.Equal(t, modeEdit, m.mode, "a failed check keeps the editor open")
				press(m, "esc")
				return
			}
			assert.False(t, m.statusIsErr, m.status)
			got, err := m.orr.FindByID(context.Background(), o.ID)
			require.NoError(t, err)
			tt.check(got)
		})
	}

	t.Run("longtext keeps the new line", func(t *testing.T) {
		editField(t, m, "notes", "line one", "alt+enter")
		typeText(m, "line two")
		press(m, "enter")
		got, _ := m.orr.FindByID(context.Background(), o.ID)
		assert.Equal(t, "line one\nline two", str(got.Notes))
	})

	t.Run("esc cancels and undo restores", func(t *testing.T) {
		before, _ := m.orr.FindByID(context.Background(), o.ID)
		editField(t, m, "size", "999", "esc")
		got, _ := m.orr.FindByID(context.Background(), o.ID)
		assert.Equal(t, str(before.Size), str(got.Size))
		press(m, "ctrl+z")
		got, _ = m.orr.FindByID(context.Background(), o.ID)
		assert.NotEqual(t, "line one\nline two", str(got.Notes), "ctrl+z undid the last field edit")
	})

	t.Run("d clears an optional field after y/n", func(t *testing.T) {
		focusLabel(t, m, "domain")
		press(m, "d")
		require.Equal(t, modeConfirm, m.mode)
		press(m, "y")
		got, _ := m.orr.FindByID(context.Background(), o.ID)
		assert.Empty(t, str(got.Domain))
		focusLabel(t, m, "name")
		press(m, "d")
		assert.Contains(t, m.status, "cannot be cleared")
	})
}

func TestTypedFieldsOnTheDealPage(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	d, err := m.dlr.Create(ctx, model.CreateDealInput{Title: "Site", Stage: "lead"})
	require.NoError(t, err)
	goDeals(m)

	editField(t, m, "value", "lots", "enter")
	assert.True(t, m.statusIsErr)
	assert.Contains(t, m.status, "a number")
	replaceText(m, "$12,500")
	press(m, "enter")
	got, _ := m.dlr.FindByID(ctx, d.ID)
	require.NotNil(t, got.Value)
	assert.Equal(t, 12500.0, *got.Value)

	editField(t, m, "value", "-3", "enter")
	assert.Contains(t, m.status, "at least")
	press(m, "esc")

	editField(t, m, "title", "Site v2", "enter")
	got, _ = m.dlr.FindByID(ctx, d.ID)
	assert.Equal(t, "Site v2", got.Title)

	// Choice: letter jump, esc keeps the old value.
	focusLabel(t, m, "stage")
	press(m, "enter")
	press(m, "w")
	assert.Equal(t, "won", m.edit.typed.spec.Choices[m.edit.typed.choice].Value)
	press(m, "esc")
	got, _ = m.dlr.FindByID(ctx, d.ID)
	assert.Equal(t, "lead", got.Stage)
	setChoice(t, m, "stage", "lost")
	got, _ = m.dlr.FindByID(ctx, d.ID)
	assert.Equal(t, "lost", got.Stage)
	require.NotNil(t, got.ClosedAt)
	focusLabel(t, m, "stage")
	press(m, "d")
	assert.Contains(t, m.status, "cannot be cleared", "stage has no empty option")
}

func TestPersonPriorityBirthdayAndDateHints(t *testing.T) {
	m, pr := newTestModel(t)
	press(m, "tab")
	cur := m.curPerson()

	setChoice(t, m, "priority", "Top")
	assert.Equal(t, 1, *personByID(t, pr, cur).PriorityTier)
	focusLabel(t, m, "priority")
	press(m, "d")
	require.Equal(t, modeConfirm, m.mode)
	press(m, "y")
	if tier := personByID(t, pr, cur).PriorityTier; tier != nil {
		assert.Equal(t, 0, *tier)
	}

	// A birthday keeps its forms and now reads a phrase through the date type.
	editField(t, m, "birthday", "03-14", "enter")
	p := personByID(t, pr, cur)
	require.NotNil(t, p.BirthdayMonth)
	assert.Equal(t, 3, *p.BirthdayMonth)
	editField(t, m, "birthday", "someday", "enter")
	assert.True(t, m.statusIsErr, "a bad birthday is refused")
	assert.Equal(t, modeEdit, m.mode)
	replaceText(m, "")
	typeText(m, "next friday")
	assert.Contains(t, m.View(), "-> 2026-10-23", "the hint previews the date the phrase means")
	press(m, "esc")
}

func TestRelationTypeahead(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	for _, n := range []string{"Acme", "Globex", "Zachary Foods"} {
		_, err := m.orr.Create(ctx, model.CreateOrgInput{Name: n})
		require.NoError(t, err)
	}
	m.afterWrite()
	press(m, "tab")
	focusLabel(t, m, "organization")
	press(m, "enter")
	require.NotNil(t, m.edit)
	require.NotNil(t, m.edit.ta)

	labels := func() []string {
		var out []string
		for _, it := range m.edit.ta.shown {
			out = append(out, it.label)
		}
		return out
	}
	tests := []struct {
		name, typed string
		want        []string
		create      bool
	}{
		{"empty lists candidates", "", []string{"Acme", "Globex", "Zachary Foods"}, false},
		{"prefix", "glo", []string{"Globex"}, true},
		{"alias: Zach finds Zachary", "zach", []string{"Zachary Foods"}, true},
		{"typo: Globx finds Globex", "globx", []string{"Globex"}, true},
		{"exact match offers no create", "acme", []string{"Acme"}, false},
		{"nothing matches", "qqqq", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			replaceText(m, tt.typed)
			assert.Equal(t, tt.want, labels())
			assert.Equal(t, tt.create, m.edit.ta.canCreate)
			if tt.create && tt.typed != "" {
				assert.Contains(t, m.View(), "Create '"+tt.typed+"'", "the last row reads Create 'X'")
			}
		})
	}

	t.Run("tab completes the highlighted match", func(t *testing.T) {
		replaceText(m, "glo")
		press(m, "tab")
		assert.Equal(t, "Globex", m.rowInput.Value())
		assert.False(t, m.edit.ta.canCreate)
	})

	t.Run("enter on the Create row makes and links the object, u unlinks the chip", func(t *testing.T) {
		replaceText(m, "Initrode")
		press(m, "enter")
		confirmLink(m)
		orgs, err := m.links.Linked(ctx, repo.Ref{Kind: "person", ID: m.curPerson()}, "organization")
		require.NoError(t, err)
		require.Len(t, orgs, 1)
		assert.Equal(t, "Initrode", orgs[0].Name)

		focusLabel(t, m, "organization")
		press(m, "u")
		require.Equal(t, modeConfirm, m.mode)
		press(m, "y")
		orgs, _ = m.links.Linked(ctx, repo.Ref{Kind: "person", ID: m.curPerson()}, "organization")
		assert.Empty(t, orgs)
	})

	t.Run("enter picks the highlighted existing match, esc closes", func(t *testing.T) {
		focusLabel(t, m, "organization")
		press(m, "enter")
		require.NotNil(t, m.edit.ta)
		replaceText(m, "glo")
		press(m, "enter")
		confirmLink(m)
		orgs, _ := m.links.Linked(ctx, repo.Ref{Kind: "person", ID: m.curPerson()}, "organization")
		require.Len(t, orgs, 1)
		assert.Equal(t, "Globex", orgs[0].Name)
		focusLabel(t, m, "organization")
		press(m, "a")
		require.NotNil(t, m.edit.ta, "a on a linked row adds another")
		press(m, "esc")
		assert.Nil(t, m.edit)
	})
}
