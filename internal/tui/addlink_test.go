package tui

import (
	"context"
	"fmt"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every pair of object kinds (owner decision 2026-10-28): the highlighted object is offered as the
// link of the new one when the relation registry joins the two kinds, and the prompt says so inline
// when it does not.
func TestAddSuggestionForEveryPairOfKinds(t *testing.T) {
	ctx := context.Background()
	kinds := []string{"person", "organization", "deal", "task", model.NoteKind}
	orgSeq := 0

	newObject := func(t *testing.T, m *Model, kind string) repo.Ref {
		t.Helper()
		switch kind {
		case "person":
			p, err := m.hCreatePerson(ctx, model.CreatePersonInput{FirstName: "Pia"})
			require.NoError(t, err)
			return repo.Ref{Kind: kind, ID: p.ID}
		case "organization":
			orgSeq++
			o, err := m.hCreateOrg(model.CreateOrgInput{Name: fmt.Sprintf("Orbit %d", orgSeq)})
			require.NoError(t, err)
			return repo.Ref{Kind: kind, ID: o.ID}
		case "deal":
			d, err := m.hCreateDeal(model.CreateDealInput{Title: "Deal", Stage: "lead"})
			require.NoError(t, err)
			return repo.Ref{Kind: kind, ID: d.ID}
		case "task":
			tk, err := m.hCreateTask(model.CreateTaskInput{Title: "Task", Priority: "medium"})
			require.NoError(t, err)
			return repo.Ref{Kind: kind, ID: tk.ID}
		}
		txt := "a note"
		n, err := m.hCreateNote(model.CreateInteractionInput{Type: "note", Content: &txt})
		require.NoError(t, err)
		return repo.Ref{Kind: kind, ID: n.ID}
	}

	for _, newKind := range kinds {
		for _, hl := range kinds {
			name := newKind + " new, " + hl + " highlighted"
			t.Run(name, func(t *testing.T) {
				m, _ := newTestModel(t)
				target := newObject(t, m, hl)
				_, linkable := model.RelationBetween(newKind, hl)

				m.addSugg = &addSuggestion{ref: target, label: m.refName(target)}
				s := m.suggestionFor(newKind)
				if !linkable {
					assert.Nil(t, s)
					require.NotNil(t, m.addSugg)
					assert.Contains(t, m.addSugg.reason, "no link between")
					assert.Contains(t, m.suggestionHint(), "no link between", "said inline in the prompt hint")
					return
				}
				require.NotNil(t, s, "the registry joins %s and %s", newKind, hl)
				assert.Contains(t, m.suggestionHint(), "tab: link")
				legacy := (newKind == "task" || newKind == model.NoteKind) && (hl == "person" || hl == "organization" || hl == "deal")
				if legacy {
					assert.False(t, s.generic, "a task or note offered a person, organization or deal keeps its own flow (globaladd_test)")
					return
				}
				// Tab accepts, the create links it, one undo step.
				m.pendingCmd = m.acceptSuggestion()
				require.True(t, m.addSugg.accepted)
				histBefore := len(m.hist)
				created := newObject(t, m, newKind)
				assert.Equal(t, histBefore+1, len(m.hist), "create and link are one undo step")
				linked, err := m.links.Linked(ctx, created, hl)
				require.NoError(t, err)
				found := false
				for _, li := range linked {
					if li.ID == target.ID && !(newKind == hl && li.ID == created.ID) {
						found = true
					}
				}
				assert.True(t, found, "%s is linked to the highlighted %s", newKind, hl)
				m.undoLast()
				linked, _ = m.links.Linked(ctx, created, hl)
				for _, li := range linked {
					assert.NotEqual(t, target.ID, li.ID, "undo removes the link with the object")
				}
			})
		}
	}
}

func TestAddSuggestionNothingHighlightedAndTags(t *testing.T) {
	ctx := context.Background()
	m, _ := newTestModel(t)

	// Nothing highlighted: the prompt says so.
	m.addSugg = nil
	assert.Nil(t, m.suggestionFor("deal"))
	assert.Equal(t, "nothing highlighted to link", m.suggestionHint())

	// A highlighted tag is offered to every kind that carries tags.
	for _, kind := range []string{"person", "organization", "deal", "task", model.NoteKind} {
		m.addSugg = &addSuggestion{tag: "vip", label: "#vip"}
		s := m.suggestionFor(kind)
		require.NotNil(t, s, kind)
		assert.True(t, s.generic)
	}
	m.addSugg = &addSuggestion{tag: "vip", label: "#vip"}
	require.NotNil(t, m.suggestionFor("deal"))
	m.pendingCmd = m.acceptSuggestion()
	d, err := m.hCreateDeal(model.CreateDealInput{Title: "Tagged", Stage: "lead"})
	require.NoError(t, err)
	tags, err := m.tagr.GetForEntity(ctx, "deal", d.ID)
	require.NoError(t, err)
	require.Len(t, tags, 1)
	assert.Equal(t, "vip", tags[0].Name)
}

// End to end: a with a person highlighted, pick Organization, tab, type the name, enter.
func TestAddOrganizationWithAPersonHighlightedLinksOnTab(t *testing.T) {
	m, _ := newTestModel(t)
	person := m.curPerson()
	press(m, "a", "o")
	require.Equal(t, modePrompt, m.mode)
	assert.Contains(t, m.View(), "tab: link", "the highlighted person is offered in grayscale")
	press(m, "tab")
	assert.Contains(t, m.View(), "linked to")
	submit(m, "Acme Co | acme.com | | | | | | | ")
	orgs, err := m.links.Linked(context.Background(), repo.Ref{Kind: "person", ID: person}, "organization")
	require.NoError(t, err)
	require.Len(t, orgs, 1)
	assert.Equal(t, "Acme Co", orgs[0].Name)

	// Without tab nothing is linked.
	press(m, "a", "o")
	submit(m, "Globex Co | globex.com | | | | | | | ")
	orgs, _ = m.links.Linked(context.Background(), repo.Ref{Kind: "person", ID: person}, "organization")
	assert.Len(t, orgs, 1)
}

// A new person added from an organization page links to it on tab, in the inline name row.
func TestAddPersonWithAnOrganizationHighlightedLinksOnTab(t *testing.T) {
	m, _ := newTestModel(t)
	o, err := m.orr.Create(context.Background(), model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	goOrgs(m)
	m.cursorOn(func(r row) bool { return r.orgID == o.ID })
	press(m, "a", "p")
	require.True(t, m.creating, "the person form opens")
	assert.Contains(t, m.View(), "tab: link Acme")
	press(m, "tab")
	typeText(m, "Nina Park")
	press(m, "enter")
	members, err := m.links.Linked(context.Background(), repo.Ref{Kind: "organization", ID: o.ID}, "person")
	require.NoError(t, err)
	require.Len(t, members, 1)
	assert.Equal(t, "Nina Park", members[0].Name)
}
