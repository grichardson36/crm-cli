package tui

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pageFixture opens the record page of a person (Bob), an organization or a deal with the cursor in
// the list. It returns the object the page is about.
func pageFixture(t *testing.T, kind string) (*Model, repo.Ref) {
	t.Helper()
	m, _ := newTestModel(t)
	ctx := context.Background()
	switch kind {
	case "person":
		press(m, "down") // Bob
		require.Equal(t, int64(2), m.curPerson())
		return m, repo.Ref{Kind: "person", ID: 2}
	case "organization":
		o, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme Test"})
		require.NoError(t, err)
		m.switchView(viewOrgs)
		m.cursorOn(func(r row) bool { return r.orgID == o.ID })
		return m, repo.Ref{Kind: "organization", ID: o.ID}
	default:
		d, err := m.dlr.Create(ctx, model.CreateDealInput{Title: "Site Test", Stage: "lead"})
		require.NoError(t, err)
		m.switchView(viewDeals)
		m.cursorOn(func(r row) bool { return r.dealID == d.ID })
		return m, repo.Ref{Kind: "deal", ID: d.ID}
	}
}

func tasksOf(t *testing.T, m *Model, ref repo.Ref) []*model.Task {
	t.Helper()
	f := model.TaskFilters{}
	switch ref.Kind {
	case "person":
		f.PersonID = &ref.ID
	case "organization":
		f.OrgID = &ref.ID
	case "deal":
		f.DealID = &ref.ID
	}
	list, err := m.tr.FindAll(context.Background(), f)
	require.NoError(t, err)
	return list
}

func notesAbout(t *testing.T, m *Model, ref repo.Ref) []*model.Interaction {
	t.Helper()
	f := model.InteractionFilters{}
	switch ref.Kind {
	case "person":
		f.PersonID = &ref.ID
	case "organization":
		f.OrgID = &ref.ID
	case "deal":
		f.DealID = &ref.ID
	}
	list, err := m.ir.FindAll(context.Background(), f)
	require.NoError(t, err)
	return list
}

func focusRow(t *testing.T, m *Model, kind rowKind) {
	t.Helper()
	for i, r := range m.form {
		if r.kind == kind && !r.empty {
			m.fcur = i
			m.syncDetail()
			return
		}
	}
	t.Fatalf("no row of kind %v", kind)
}

// Notes are registered like every other object, so every verb of the one verb set exists for them
// in the palette under the same "Verb object" title, and the special note actions are gone.
func TestNoteIsRegisteredWithTheGenericVerbs(t *testing.T) {
	m, _ := newTestModel(t)
	o := objectByKind(model.NoteKind)
	require.NotNil(t, o)
	for _, v := range []verb{vAdd, vEdit, vDelete, vRestore, vLink, vUnlink} {
		assert.True(t, o.has(v), "note supports %s", v)
	}
	assert.False(t, o.has(vMark), "a verb a note cannot do is absent")

	titles := map[string]string{}
	for _, a := range m.actions {
		titles[a.title] = a.id
	}
	tests := []struct{ title, id string }{
		{"Add note", "note"},
		{"Edit note", "note-edit"},
		{"Delete note", "note-delete"},
		{"Restore note", "note-restore"},
		{"Link note", "link-note"},
		{"Unlink note", "unlink-note"},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			assert.Equal(t, tt.id, titles[tt.title])
		})
	}
	for _, gone := range []string{"Add note to organization or deal", "Restore last deleted note"} {
		assert.NotContains(t, titles, gone, "the special-cased name is replaced by the generic one")
	}
	assert.Nil(t, m.actionByID("note-object"))
}

// Org and deal pages behave like a person page: t adds a task, n adds a note, and task and note rows
// edit, delete and restore through the same keys. The table runs the same steps on the three pages
// and expects the same messages.
func TestRecordPagesShareTaskAndNoteBehavior(t *testing.T) {
	for _, kind := range []string{"person", "organization", "deal"} {
		t.Run(kind, func(t *testing.T) {
			m, ref := pageFixture(t, kind)

			// t adds a task for the page object, with the same prompt and the due date after the pipe.
			press(m, "a", "t", "tab")
			require.Equal(t, modePrompt, m.mode, "t opens the task prompt on a %s page", kind)
			assert.Equal(t, "task: ", m.input.Prompt)
			submit(m, "Call back | 2026-12-01")
			tasks := tasksOf(t, m, ref)
			require.Len(t, tasks, 1)
			assert.Equal(t, "Call back", tasks[0].Title)
			assert.Equal(t, "task added", m.status)

			// n adds a note to the page object.
			press(m, "a", "n", "tab")
			require.Equal(t, modePrompt, m.mode, "n opens the note prompt on a %s page", kind)
			submit(m, "first draft")
			notes := notesAbout(t, m, ref)
			require.Len(t, notes, 1)
			assert.Equal(t, "first draft", str(notes[0].Content))
			assert.Contains(t, m.status, "added")

			// The rows show on the page and Enter edits them inline.
			press(m, "tab")
			require.Equal(t, focusDetail, m.focus)
			focusRow(t, m, rkTask)
			press(m, "enter")
			require.Equal(t, modeEdit, m.mode, "enter edits the task row inline")
			replaceText(m, "Call back today")
			press(m, "enter")
			assert.Equal(t, "task saved", m.status)
			assert.Equal(t, "Call back today", tasksOf(t, m, ref)[0].Title)

			focusRow(t, m, rkNote)
			press(m, "enter")
			require.Equal(t, modeEdit, m.mode, "enter edits the note row inline")
			replaceText(m, "second draft")
			press(m, "enter")
			assert.Equal(t, "note saved", m.status)
			assert.Equal(t, "second draft", str(notesAbout(t, m, ref)[0].Content))

			// d deletes the note after y/n and Restore note brings it back.
			focusRow(t, m, rkNote)
			press(m, "d")
			require.Equal(t, modeConfirm, m.mode)
			press(m, "y")
			assert.Empty(t, notesAbout(t, m, ref))
			assert.Equal(t, "note deleted (ctrl+z undoes)", m.status)
			runPalette(m, "restore note")
			assert.Equal(t, "note restored", m.status)
			assert.Len(t, notesAbout(t, m, ref), 1)

			// The task row deletes the same way.
			focusRow(t, m, rkTask)
			press(m, "d", "y")
			assert.Empty(t, tasksOf(t, m, ref))
		})
	}
}

// An empty page shows one placeholder row for tasks and for notes on every record page, and Enter on
// it adds the first one.
func TestRecordPagesShowAddRowsAndEnterAdds(t *testing.T) {
	for _, kind := range []string{"person", "organization", "deal"} {
		t.Run(kind, func(t *testing.T) {
			m, ref := pageFixture(t, kind)
			if kind == "person" {
				m, ref = pageFixtureFresh(t)
			}
			var task, note int
			for _, r := range m.form {
				if r.kind == rkTask && r.empty {
					task++
				}
				if r.kind == rkNote && r.empty {
					note++
				}
			}
			assert.Equal(t, 1, task, "one empty task row")
			assert.Equal(t, 1, note, "one empty note row")

			press(m, "tab")
			for i, r := range m.form {
				if r.kind == rkTask && r.empty {
					m.fcur = i
					m.syncDetail()
				}
			}
			press(m, "enter")
			if kind != "person" { // a person's empty row opens an inline row, the others open the task prompt
				require.Equal(t, modePrompt, m.mode)
				submit(m, "Send deck")
				assert.Len(t, tasksOf(t, m, ref), 1)
			}
		})
	}
}

// pageFixtureFresh is a person with no tasks and no notes (Cara).
func pageFixtureFresh(t *testing.T) (*Model, repo.Ref) {
	t.Helper()
	m, _ := newTestModel(t)
	press(m, "down", "down")
	require.Equal(t, int64(3), m.curPerson())
	return m, repo.Ref{Kind: "person", ID: 3}
}

// Attaching an existing note to an object goes through the generic Link verb: Link note on the
// organization lists the notes, picking one links it, and Unlink note removes the link.
func TestExistingNoteAttachesThroughLink(t *testing.T) {
	for _, kind := range []string{"organization", "deal", "person"} {
		t.Run(kind, func(t *testing.T) {
			m, ref := pageFixture(t, kind)
			n, err := m.ir.Create(context.Background(), model.CreateInteractionInput{Type: "meeting", Content: sp("loose note")})
			require.NoError(t, err)
			m.afterWrite()

			a := m.actionByID("link-note")
			require.NotNil(t, a)
			require.True(t, a.enabled(m), "Link note is offered on a %s", kind)
			m.pendingCmd = a.run(m)
			require.Equal(t, modeOverlay, m.mode, "the note picker opens")
			require.NotEmpty(t, m.ov.items)
			assert.Equal(t, "loose note", m.ov.items[0].label)
			press(m, "enter")
			require.Len(t, notesAbout(t, m, ref), 1, "the note is on the %s", kind)
			assert.Equal(t, n.ID, notesAbout(t, m, ref)[0].ID)

			// ctrl+z undoes the link.
			press(m, "ctrl+z")
			assert.Empty(t, notesAbout(t, m, ref))
		})
	}
}

// Link works the other way too: with the cursor on a note row, L links that note to another object.
func TestLinkFromANoteRow(t *testing.T) {
	m, id := noteModel(t)
	o, err := m.orr.Create(context.Background(), model.CreateOrgInput{Name: "Globex Test"})
	require.NoError(t, err)
	m.afterWrite()
	focusNote(t, m)
	src, ok := m.cursorObject()
	require.True(t, ok)
	assert.Equal(t, repo.Ref{Kind: model.NoteKind, ID: id}, src)
	assert.Equal(t, []string{"person", "organization", "deal", tagKind}, relationsOf(model.NoteKind))

	m.pendingCmd = m.startLink(src, "organization", false)
	require.Equal(t, modeOverlay, m.mode)
	typeText(m, "Globex Test")
	press(m, "enter")
	f := model.InteractionFilters{OrgID: &o.ID}
	list, err := m.ir.FindAll(context.Background(), f)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, id, list[0].ID)
}

// Note data and note types keep working: the types of existing notes show on the rows and the
// type key still changes them on an organization page.
func TestNoteTypesStillWorkOnAnOrganizationPage(t *testing.T) {
	m, ref := pageFixture(t, "organization")
	_, err := m.ir.Create(context.Background(), model.CreateInteractionInput{Type: "call", Content: sp("rang them"), OrgIDs: []int64{ref.ID}})
	require.NoError(t, err)
	m.afterWrite()
	press(m, "tab")
	focusRow(t, m, rkNote)
	press(m, "E")
	require.Equal(t, modeOverlay, m.mode)
	assert.Equal(t, "Note type", m.ov.title)
	assert.Equal(t, "call", m.ov.items[0].label, "the current type first")
	typeText(m, "meeting")
	press(m, "enter")
	assert.Equal(t, "meeting", notesAbout(t, m, ref)[0].Type)
}
