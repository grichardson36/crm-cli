package tui

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startBar opens the bar, types the query and puts the cursor on the row with the id.
func startBar(t *testing.T, m *Model, query, id string) {
	t.Helper()
	press(m, "ctrl+k")
	require.Equal(t, modeOverlay, m.mode)
	typeText(m, query)
	cursorOnBarRow(t, m, id)
}

func cursorOnBarRow(t *testing.T, m *Model, id string) {
	t.Helper()
	for pos, idx := range m.ov.shown {
		if m.ov.items[idx].id == id {
			m.ov.cursor = pos
			return
		}
	}
	require.Failf(t, "row not in the bar", "%s in %v", id, shownKinds(m))
}

func barFooter(m *Model) string {
	it, ok := m.ov.highlighted()
	if !ok {
		return ""
	}
	return m.barVerbLine(it)
}

func barVerbLabels(m *Model, id string) []string {
	var out []string
	for _, idx := range m.ov.shown {
		it := m.ov.items[idx]
		if it.id != id {
			continue
		}
		tg, ok := barTargetOf(it)
		if !ok {
			return nil
		}
		for _, v := range m.barVerbs(tg) {
			out = append(out, v.key+" "+v.label)
		}
	}
	return out
}

func TestBarTabMovesFocusToTheResultsAndBack(t *testing.T) {
	m, _ := newTestModel(t)
	startBar(t, m, "bob", "person:2")
	assert.False(t, m.ov.focusResults)
	press(m, "tab")
	assert.True(t, m.ov.focusResults, "Tab on an object row moves the focus to the results")
	assert.Contains(t, barFooter(m), "contacted", "the footer lists the object's verbs with their keys")
	assert.Contains(t, barFooter(m), "x contacted")
	press(m, "esc")
	assert.False(t, m.ov.focusResults, "Esc returns to the input")
	assert.Equal(t, modeOverlay, m.mode, "the bar stays open")
	press(m, "tab")
	assert.True(t, m.ov.focusResults)
	press(m, "tab")
	assert.False(t, m.ov.focusResults, "Tab returns to the input too")
	typeText(m, "x") // the input takes text again
	assert.Equal(t, "bobx", m.ov.filter.Value())

	// A row that is an action keeps Tab as "move down".
	m2, _ := newTestModel(t)
	press(m2, "ctrl+k")
	typeText(m2, "favorite")
	before := m2.ov.cursor
	press(m2, "tab")
	assert.False(t, m2.ov.focusResults)
	assert.NotEqual(t, before, m2.ov.cursor)
}

func TestBarTabStillCompletesTypedVerbs(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+k")
	typeText(m, "sno")
	press(m, "tab")
	assert.False(t, m.ov.focusResults)
}

func TestBarPersonHotkeysActLikeThePeopleView(t *testing.T) {
	m, pr := newTestModel(t)
	startBar(t, m, "bob", "person:2")
	press(m, "tab")

	press(m, "x") // mark contacted
	assert.Equal(t, modeOverlay, m.mode, "the verb ran without leaving the bar")
	assert.True(t, m.ov.focusResults, "and the focus stays on the results")
	assert.Equal(t, "Mark: Bob Baker", m.toast.text, "the toast names the action and the target")
	p := personByID(t, pr, 2)
	require.NotNil(t, p.LastContactedAt)
	assert.Equal(t, "2026-10-13 12:00:00", *p.LastContactedAt)
	before := "2020-01-01"

	press(m, "f") // favorite
	assert.True(t, personByID(t, pr, 2).IsFavorite)

	// Undo works from the bar, and so does redo.
	press(m, "ctrl+z")
	assert.False(t, personByID(t, pr, 2).IsFavorite)
	press(m, "ctrl+y")
	assert.True(t, personByID(t, pr, 2).IsFavorite)
	press(m, "ctrl+z", "ctrl+z")
	assert.Contains(t, *personByID(t, pr, 2).LastContactedAt, before, "undo walks back through the verbs run from the bar")

	// The screen under the bar is back as it was.
	press(m, "esc", "esc")
	assert.Equal(t, modeNormal, m.mode)
	assert.Equal(t, viewPeople, m.view)
	assert.Equal(t, int64(1), m.curPerson())
}

func TestBarNoteVerbTakesTheScreenAndTheBarComesBack(t *testing.T) {
	m, pr := newTestModel(t)
	startBar(t, m, "bob", "person:2")
	press(m, "tab", "a", "n") // a, the note kind, then tab accepts the highlighted person as the link
	require.Equal(t, modePrompt, m.mode, "a verb with a prompt shows the prompt")
	typeLinked(m, "met at the bar")
	press(m, "enter")
	assert.Equal(t, modeOverlay, m.mode, "the bar returns when the prompt is done")
	require.NotNil(t, m.ov)
	assert.True(t, m.ov.focusResults)
	p := personByID(t, pr, 2)
	require.NotNil(t, p)
	bob := int64(2)
	notes, err := m.ir.FindAll(m.ctx, model.InteractionFilters{PersonID: &bob, Limit: 10})
	require.NoError(t, err)
	require.NotEmpty(t, notes)
	assert.Equal(t, "met at the bar", *notes[0].Content)
	assert.Equal(t, viewPeople, m.view)

	// Esc in the prompt cancels and the bar still returns.
	press(m, "a", "n")
	require.Equal(t, modePrompt, m.mode)
	press(m, "esc")
	assert.Equal(t, modeOverlay, m.mode)
}

func TestBarVerbsDifferPerKindAndAreHiddenWhenNotApplicable(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := m.ctx
	o, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme Holdings"})
	require.NoError(t, err)
	d, err := m.dlr.Create(ctx, model.CreateDealInput{Title: "Acme rollout", Stage: "lead"})
	require.NoError(t, err)
	task, err := m.tr.Create(ctx, model.CreateTaskInput{Title: "Acme proposal", Priority: "medium"})
	require.NoError(t, err)
	m.afterWrite()

	press(m, "ctrl+k")
	typeText(m, "acme")
	person := barVerbLabels(m, "person:1")
	org := barVerbLabels(m, "org:"+itoa64(o.ID))
	deal := barVerbLabels(m, "deal:"+itoa64(d.ID))
	tk := barVerbLabels(m, "task:"+itoa64(task.ID))

	has := func(list []string, want string) bool {
		for _, v := range list {
			if strings.HasSuffix(v, want) {
				return true
			}
		}
		return false
	}
	assert.True(t, has(person, "contacted"), "%v", person)
	assert.False(t, has(person, "stage"))
	require.NotEmpty(t, org, "org row has verbs")
	assert.False(t, has(org, "contacted"), "a person verb is hidden on an organization: %v", org)
	assert.True(t, has(org, "edit"), "e edits an organization row: %v", org)
	assert.True(t, has(org, "delete"), "%v", org)
	require.NotEmpty(t, deal)
	assert.True(t, has(deal, "edit"), "%v", deal)
	assert.False(t, has(deal, "stage"), "the stage is a palette entry now: %v", deal)
	assert.False(t, has(deal, "contacted"))
	require.NotEmpty(t, tk)
	assert.True(t, has(tk, "delete"), "%v", tk)
	assert.False(t, has(tk, "contacted"))

	// A key that is not a verb of the kind does nothing but say so.
	cursorOnBarRow(t, m, "org:"+itoa64(o.ID))
	press(m, "tab", "s")
	assert.Contains(t, m.toast.text, "no verb")
	assert.Equal(t, modeOverlay, m.mode)
	assert.Nil(t, personByID(t, m.pr, 1).LastContactedAt)
}

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }

func TestBarOrganizationVerbsDeleteWithConfirmAndUndo(t *testing.T) {
	m, _ := newTestModel(t)
	o, err := m.orr.Create(m.ctx, model.CreateOrgInput{Name: "Zebra Works"})
	require.NoError(t, err)
	m.afterWrite()
	startBar(t, m, "zebra", "org:"+itoa64(o.ID))
	press(m, "tab", "d")
	require.Equal(t, modeConfirm, m.mode, "delete asks y/n like the view does")
	press(m, "y")
	assert.Equal(t, modeOverlay, m.mode, "the bar comes back")
	list, _ := m.orr.FindAll(m.ctx, 0)
	assert.Empty(t, list)
	for _, idx := range m.ov.shown {
		assert.NotEqual(t, "org:"+itoa64(o.ID), m.ov.items[idx].id, "results refresh")
	}
	press(m, "ctrl+z")
	list, _ = m.orr.FindAll(m.ctx, 0)
	assert.Len(t, list, 1, "undo from the bar restores it")
}

func TestBarTaskAndDealVerbs(t *testing.T) {
	m, _ := newTestModel(t)
	task, err := m.tr.Create(m.ctx, model.CreateTaskInput{Title: "Zebra proposal", Priority: "medium"})
	require.NoError(t, err)
	m.afterWrite()
	startBar(t, m, "zebra", "task:"+itoa64(task.ID))
	press(m, "tab", "x") // done
	got, err := m.tr.FindByID(m.ctx, task.ID)
	require.NoError(t, err)
	assert.True(t, got.Completed, "x completes the task as it does in the Tasks view")
	assert.Equal(t, modeOverlay, m.mode)

	d, err := m.dlr.Create(m.ctx, model.CreateDealInput{Title: "Quartz rollout", Stage: "lead"})
	require.NoError(t, err)
	m.afterWrite()
	press(m, "esc", "esc")
	startBar(t, m, "quartz", "deal:"+itoa64(d.ID))
	press(m, "tab", "e")
	require.Equal(t, modePrompt, m.mode, "e edits the deal as one line")
	press(m, "esc")
	assert.Equal(t, modeOverlay, m.mode, "the bar returns after the prompt")
	assert.Nil(t, m.barResume)
	// The stage is a choice field of the deal page, edited in place.
	press(m, "esc", "esc")
	m.switchView(viewDeals)
	m.settle()
	setChoice(t, m, "stage", "prospect")
	got2, err := m.dlr.FindByID(m.ctx, d.ID)
	require.NoError(t, err)
	assert.NotEqual(t, "lead", got2.Stage)
}

func TestBarTagRowMatchesWithoutHashAndRanksFirstAmongObjects(t *testing.T) {
	m, _ := newTestModel(t)
	wanda := "The Well Co"
	w, err := m.pr.Create(m.ctx, model.CreatePersonInput{FirstName: "Wanda", Company: &wanda})
	require.NoError(t, err)
	// A tag is a name on objects: it only lists while something carries it.
	require.NoError(t, m.tagr.Apply(m.ctx, "person", w.ID, "The Well"))
	require.NoError(t, m.tagr.Apply(m.ctx, "person", w.ID, "The Wellness Club"))
	m.afterWrite()

	press(m, "ctrl+k")
	typeText(m, "the well")
	var tags []string
	firstObject := ""
	for _, idx := range m.ov.shown {
		it := m.ov.items[idx]
		if it.kind == kindTag {
			tags = append(tags, it.label)
		}
		if firstObject == "" && (it.kind == kindPerson || it.kind == kindTag) {
			firstObject = it.label
		}
	}
	assert.Equal(t, []string{"#The Well", "#The Wellness Club"}, tags, "the exact name first, then the partial match: %v", shownKinds(m))
	assert.Equal(t, "#The Well", firstObject, "an exact tag leads the objects")

	// Case does not matter and a part of the name is enough.
	m.ov.filter.SetValue("WELLNESS")
	m.ov.refilterBar()
	assert.Contains(t, shownKinds(m), "tag:#The Wellness Club")
	assert.NotContains(t, shownKinds(m), "tag:#The Well")

	// # still shows only tags.
	m.ov.filter.SetValue("#the")
	m.ov.refilterBar()
	for _, idx := range m.ov.shown {
		assert.Equal(t, kindTag, m.ov.items[idx].kind)
	}
}

func TestBarTagNameDoesNotMixTaggedPeopleIntoTheResults(t *testing.T) {
	m, _ := newTestModel(t) // Bob has the tag "friend"
	press(m, "ctrl+k")
	typeText(m, "friend")
	var kinds []string
	for _, idx := range m.ov.shown {
		it := m.ov.items[idx]
		if it.kind == kindPerson || it.kind == kindTag {
			kinds = append(kinds, it.kind+":"+it.label)
		}
	}
	assert.Equal(t, []string{"tag:#friend", "person:Bob Baker"}, kinds, "the tag row first, then the person it matched")
	for _, idx := range m.ov.shown {
		if it := m.ov.items[idx]; it.kind == kindPerson {
			assert.Equal(t, "tagged #friend", it.desc, "a person who matched only through a tag says so")
		}
	}

	// A person who matches by their own text is not marked, and comes before the tag-only matches.
	_, err := m.pr.Create(m.ctx, model.CreatePersonInput{FirstName: "Friendly", LastName: sp("Fran")})
	require.NoError(t, err)
	m.afterWrite()
	m.ov.refilterBar()
	var people []string
	for _, idx := range m.ov.shown {
		if it := m.ov.items[idx]; it.kind == kindPerson {
			people = append(people, it.label+"|"+it.desc)
		}
	}
	assert.Equal(t, []string{"Friendly Fran|", "Bob Baker|tagged #friend"}, people)
}

func TestBarTagRowOpensItsTaggedPeopleAndHasItsOwnVerbs(t *testing.T) {
	m, _ := newTestModel(t)
	startBar(t, m, "friend", "tag:friend")
	assert.Equal(t, []string{"m merge", "d delete", "l link", "u unlink", "e rename", "ctrl+z undo", "ctrl+y redo"}, barVerbLabelsFor(m, "tag:friend"), "the verbs of the Tags view, not person verbs")

	// Enter opens the tag: the People view filtered by #friend, as # does.
	press(m, "enter")
	assert.Equal(t, viewPeople, m.view)
	assert.Equal(t, "#friend", m.query)
	assert.Equal(t, []string{"Bob Baker"}, names(m))

	// Link: pick Cara to give her the tag, from the bar, then undo it.
	m.query = ""
	startBar(t, m, "friend", "tag:friend")
	press(m, "tab", "l")
	require.NotNil(t, m.ov, "l opens a picker of objects that can take the tag")
	typeText(m, "cara")
	press(m, "enter")
	assert.Equal(t, modeOverlay, m.mode)
	ids, _ := m.tagr.GetEntities(m.ctx, "friend", "person")
	assert.ElementsMatch(t, []int64{2, 3}, ids)
	press(m, "ctrl+z")
	ids, _ = m.tagr.GetEntities(m.ctx, "friend", "person")
	assert.Equal(t, []int64{2}, ids)

	// Unlink Bob.
	press(m, "u")
	require.NotNil(t, m.ov)
	press(m, "enter")
	ids, _ = m.tagr.GetEntities(m.ctx, "friend", "person")
	assert.Empty(t, ids)

	// Delete asks first, undo puts the tag back on its records. A tag nothing carries leaves the bar
	// and comes back with undo, so the cursor is put back on it.
	press(m, "ctrl+z")
	cursorOnBarRow(t, m, "tag:friend")
	press(m, "d")
	require.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, "delete tag #friend")
	press(m, "y")
	tags, _ := m.tagr.FindAll(m.ctx)
	for _, tg := range tags {
		assert.NotEqual(t, "friend", tg.Name)
	}
	press(m, "ctrl+z")
	ids, _ = m.tagr.GetEntities(m.ctx, "friend", "person")
	assert.Equal(t, []int64{2}, ids)
}

func barVerbLabelsFor(m *Model, id string) []string { return barVerbLabels(m, id) }

func TestBarVerbsFollowReadOnlyMode(t *testing.T) {
	m, pr := newTestModel(t)
	m.readOnly = true
	startBar(t, m, "bob", "person:2")
	press(m, "tab")
	assert.NotContains(t, barFooter(m), "contacted", "write verbs are not offered in read-only mode")
	press(m, "x")
	assert.Contains(t, *personByID(t, pr, 2).LastContactedAt, "2020-01-01", "and do not run")
	assert.Contains(t, strings.ToLower(m.toast.text), "read-only")
	assert.Equal(t, modeOverlay, m.mode)
}

func TestBarVerbsFollowKeyRebinding(t *testing.T) {
	m, pr := newTestModel(t)
	m.applyKeys(map[string][]string{"mark-state": {"ctrl+g"}})
	startBar(t, m, "bob", "person:2")
	press(m, "tab")
	assert.Contains(t, barFooter(m), "ctrl+g contacted")
	press(m, "x")
	assert.Contains(t, *personByID(t, pr, 2).LastContactedAt, "2020-01-01", "the old key no longer runs the verb")
	m.Update(keyPress("ctrl+g"))
	assert.Contains(t, *personByID(t, pr, 2).LastContactedAt, "2026-10-13")
}

func TestBarResultsFocusFitsEightyColumns(t *testing.T) {
	m, _ := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	for _, tc := range []struct{ query, id string }{{"bob", "person:2"}, {"friend", "tag:friend"}} {
		press(m, "ctrl+k")
		typeText(m, tc.query)
		cursorOnBarRow(t, m, tc.id)
		press(m, "tab")
		for _, l := range strings.Split(m.screen(), "\n") {
			assert.LessOrEqual(t, lipgloss.Width(l), 80, "%q", l)
		}
		assert.Contains(t, plain(m.screen()), "tab or esc: input")
		press(m, "esc", "esc")
	}
}
