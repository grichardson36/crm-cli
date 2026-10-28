package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newObjectOf makes a live object of a kind for the tests of the pipe link segments.
func newObjectOf(t *testing.T, m *Model, kind string, seq *int) repo.Ref {
	t.Helper()
	ctx := context.Background()
	*seq++
	switch kind {
	case "person":
		p, err := m.hCreatePerson(ctx, model.CreatePersonInput{FirstName: "Pia", LastName: sp(fmt.Sprintf("Lane%d", *seq))})
		require.NoError(t, err)
		return repo.Ref{Kind: kind, ID: p.ID}
	case "organization":
		o, err := m.hCreateOrg(model.CreateOrgInput{Name: fmt.Sprintf("Orbit %d", *seq)})
		require.NoError(t, err)
		return repo.Ref{Kind: kind, ID: o.ID}
	case "deal":
		d, err := m.hCreateDeal(model.CreateDealInput{Title: fmt.Sprintf("Deal %d", *seq), Stage: "lead"})
		require.NoError(t, err)
		return repo.Ref{Kind: kind, ID: d.ID}
	case "task":
		tk, err := m.hCreateTask(model.CreateTaskInput{Title: fmt.Sprintf("Task %d", *seq), Priority: "medium"})
		require.NoError(t, err)
		return repo.Ref{Kind: kind, ID: tk.ID}
	}
	txt := fmt.Sprintf("a note %d", *seq)
	n, err := m.hCreateNote(model.CreateInteractionInput{Type: "note", Content: &txt})
	require.NoError(t, err)
	return repo.Ref{Kind: kind, ID: n.ID}
}

// startAddOf opens the add prompt of a kind the way the add picker does and returns its pipe line.
func startAddOf(t *testing.T, m *Model, kind string) *pipeLine {
	t.Helper()
	switch kind {
	case "person":
		m.pendingCmd = m.startCreate()
		require.True(t, m.creating)
		return personAddLine()
	case model.NoteKind:
		m.startInput("note-free", "note: ", "")
		_ = m.input.Focus()
		return m.pipeFor("note-free")
	}
	m.pendingCmd = objectByKind(kind).add(m)
	require.Equal(t, modePrompt, m.mode, kind)
	return m.pipeFor(m.promptKind)
}

// findCreated finds the object a test line made, by its text "X".
func findCreated(t *testing.T, m *Model, kind string) repo.Ref {
	t.Helper()
	ctx := context.Background()
	switch kind {
	case "person":
		list, _ := m.pr.FindAll(ctx, model.PersonFilters{})
		for _, p := range list {
			if p.FirstName == "X" {
				return repo.Ref{Kind: kind, ID: p.ID}
			}
		}
	case "organization":
		list, _ := m.orr.FindAll(ctx, 0)
		for _, o := range list {
			if o.Name == "X" {
				return repo.Ref{Kind: kind, ID: o.ID}
			}
		}
	case "deal":
		list, _ := m.dlr.FindAll(ctx, model.DealFilters{})
		for _, d := range list {
			if d.Title == "X" {
				return repo.Ref{Kind: kind, ID: d.ID}
			}
		}
	case "task":
		list, _ := m.tr.FindAll(ctx, model.TaskFilters{})
		for _, tk := range list {
			if tk.Title == "X" {
				return repo.Ref{Kind: kind, ID: tk.ID}
			}
		}
	default:
		list, _ := m.ir.FindAll(ctx, model.InteractionFilters{Limit: 100})
		for _, n := range list {
			if n.Content != nil && *n.Content == "X" {
				return repo.Ref{Kind: kind, ID: n.ID}
			}
		}
	}
	t.Fatalf("no %s named X was created", kind)
	return repo.Ref{}
}

func segFor(pl *pipeLine, kind string) int {
	for i, s := range pl.segs {
		for _, k := range s.Kinds {
			if s.Link && k == kind {
				return i
			}
		}
	}
	return -1
}

// Every pair of object kinds (owner decision 2026-10-29): the highlighted object is the first suggestion
// of the segment that takes its kind, tab accepts it, and enter links it. A kind the relation registry
// does not join to the new object has no such segment.
func TestHighlightedObjectIsTheFirstSuggestionOfTheMatchingSegmentForEveryPairOfKinds(t *testing.T) {
	ctx := context.Background()
	kinds := []string{"person", "organization", "deal", "task", model.NoteKind}
	for _, newKind := range kinds {
		for _, hl := range kinds {
			t.Run(newKind+" new, "+hl+" highlighted", func(t *testing.T) {
				m, _ := newTestModel(t)
				seq := 0
				target := newObjectOf(t, m, hl, &seq)
				_, linkable := model.RelationBetween(newKind, hl)
				m.addSugg = &addSuggestion{ref: target, label: m.refName(target)}
				pl := startAddOf(t, m, newKind)
				idx := segFor(pl, hl)
				if !linkable {
					assert.Equal(t, -1, idx, "the registry joins no %s to a %s, so the line has no segment for it", newKind, hl)
					return
				}
				require.GreaterOrEqual(t, idx, 0, "%s has a segment for %s", newKind, hl)
				typeText(m, "X"+strings.Repeat("|", idx))
				s := m.pipeRefresh()
				require.NotNil(t, s)
				rows := s.ac.Rows()
				require.NotEmpty(t, rows)
				assert.Equal(t, target, rows[0].Ref, "the highlighted object leads")
				press(m, "tab")
				in, _ := m.pipeInput()
				assert.True(t, strings.HasSuffix(in.Value(), ", "), "a comma and a space follow the accepted name: %q", in.Value())
				histBefore := len(m.hist)
				press(m, "enter")
				created := findCreated(t, m, newKind)
				assert.Equal(t, histBefore+1, len(m.hist), "create and link are one undo step")
				linked, err := m.links.Linked(ctx, created, hl)
				require.NoError(t, err)
				found := false
				for _, li := range linked {
					if li.ID == target.ID {
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

// A highlighted tag is the first suggestion of the tags segment of every kind that carries tags, and the
// new object gets the tag when the line is entered.
func TestHighlightedTagIsTheFirstSuggestionOfTheTagsSegment(t *testing.T) {
	for _, kind := range []string{"person", "organization", "deal", "task", model.NoteKind} {
		t.Run(kind, func(t *testing.T) {
			m, _ := newTestModel(t)
			m.addSugg = &addSuggestion{tag: "vip", label: "#vip"}
			pl := startAddOf(t, m, kind)
			idx := segFor(pl, tagKind)
			require.GreaterOrEqual(t, idx, 0)
			typeText(m, "X"+strings.Repeat("|", idx))
			rows := m.pipeRefresh().ac.Rows()
			require.NotEmpty(t, rows)
			assert.Equal(t, "vip", rows[0].Tag)
			press(m, "tab", "enter")
			created := findCreated(t, m, kind)
			tags, err := m.tagr.GetForEntity(context.Background(), created.Kind, created.ID)
			require.NoError(t, err)
			require.Len(t, tags, 1)
			assert.Equal(t, "vip", tags[0].Name)
		})
	}
}

// End to end: a with a person highlighted, pick Organization, type the name, the people segment offers
// the person, tab accepts it, enter links it.
func TestAddOrganizationWithAPersonHighlightedLinksOnTab(t *testing.T) {
	m, _ := newTestModel(t)
	person := m.curPerson()
	press(m, "a", "o")
	require.Equal(t, modePrompt, m.mode)
	typeText(m, "Acme Co"+strings.Repeat("|", 9))
	assert.Contains(t, m.screen(), "Zed Adams", "the highlighted person is the first row of the popup")
	assert.Contains(t, m.screen(), "highlighted")
	press(m, "tab", "enter")
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
	typeText(m, "Nina Park|")
	assert.Contains(t, m.screen(), "highlighted", "the organizations segment offers the highlighted organization")
	assert.NotContains(t, m.screen(), "tab: link")
	press(m, "tab")
	press(m, "enter")
	members, err := m.links.Linked(context.Background(), repo.Ref{Kind: "organization", ID: o.ID}, "person")
	require.NoError(t, err)
	require.Len(t, members, 1)
	assert.Equal(t, "Nina Park", members[0].Name)
}
