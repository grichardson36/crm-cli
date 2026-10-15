package tui

import (
	"context"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// In the tests the clock is Tuesday 2026-10-13 12:00 UTC, so fri is Fri 16 Oct.

func openBar(m *Model, line string) {
	press(m, "ctrl+k")
	typeText(m, line)
}

func allNotes(t *testing.T, m *Model) []*model.Interaction {
	t.Helper()
	its, err := m.ir.FindAll(context.Background(), model.InteractionFilters{})
	require.NoError(t, err)
	return its
}

func TestCommandBarShowsALivePreviewAndRunsANote(t *testing.T) {
	m, pr := newTestModel(t)
	openBar(m, "n called about the deal #deal remind fri")
	require.Equal(t, modeOverlay, m.mode)
	require.NotNil(t, m.ov.cmd, "a verb and a space switch the palette to a command line")
	require.True(t, m.ov.cmd.plan.Ready())

	out := plain(m.View())
	assert.Contains(t, out, "Command bar")
	assert.Contains(t, out, `note on Zed Adams: "called about the deal"`)
	assert.Contains(t, out, "tag #deal (new)")
	assert.Contains(t, out, "remind Fri 16 Oct")
	assert.Contains(t, out, "n called about the deal #deal remind fri", "the typed line is echoed")

	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode)
	assert.Contains(t, m.status, "done: note on Zed Adams")
	assert.False(t, m.statusIsErr)
	its := allNotes(t, m)
	require.Len(t, its, 1)
	assert.Equal(t, "called about the deal", *its[0].Content)
	tags, err := m.tagr.GetForEntity(context.Background(), "interaction", its[0].ID)
	require.NoError(t, err)
	require.Len(t, tags, 1)
	assert.Equal(t, "deal", tags[0].Name)
	tasks, _ := m.tr.FindAll(context.Background(), model.TaskFilters{})
	require.Len(t, tasks, 1)
	assert.Equal(t, "2026-10-16", *tasks[0].DueAt)
	_ = pr
}

func TestCommandBarMarksChips(t *testing.T) {
	m, _ := newTestModel(t)
	openBar(m, "n hello #friend #brandnew remind fri")
	marks := m.ov.cmd.plan.Marks
	var kinds []string
	for _, mk := range marks {
		kinds = append(kinds, mk.Kind)
		if mk.Kind == "tag" && strings.Contains(m.ov.filter.Value()[mk.Start:mk.End], "brandnew") {
			assert.True(t, mk.New, "an unknown tag is flagged new")
		}
		if mk.Kind == "tag" && strings.Contains(m.ov.filter.Value()[mk.Start:mk.End], "friend") {
			assert.False(t, mk.New)
		}
	}
	assert.Equal(t, []string{"verb", "tag", "tag", "due"}, kinds)
	assert.Contains(t, plain(m.View()), "tag #brandnew (new)")
}

func TestCommandBarNameWorksWithoutASelectedPerson(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "6") // the Merge view has no selected person
	openBar(m, "n Bob called back")
	require.NotNil(t, m.ov.cmd)
	assert.Contains(t, plain(m.View()), `note on Bob Baker: "called back"`)
	press(m, "enter")
	its := allNotes(t, m)
	require.Len(t, its, 1)
	assert.Equal(t, "called back", *its[0].Content)
}

func TestCommandBarNeedsAPersonOrAName(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "6")
	openBar(m, "n called back")
	assert.Contains(t, plain(m.View()), "who is this about?")
	press(m, "enter")
	assert.Equal(t, modeOverlay, m.mode, "the bar stays open")
	assert.Contains(t, plain(m.View()), "who is this about?")
	assert.Empty(t, allNotes(t, m))
}

func TestCommandBarAmbiguousNameBlocksEnter(t *testing.T) {
	m, pr := newTestModel(t)
	_, err := pr.Create(context.Background(), model.CreatePersonInput{FirstName: "Bob", LastName: sp("Brown")})
	require.NoError(t, err)
	require.NoError(t, m.reload())
	press(m, "6")
	openBar(m, "n Bob called back")
	out := plain(m.View())
	assert.Contains(t, out, "2 people match")
	assert.Contains(t, out, "Bob Baker")
	assert.Contains(t, out, "Bob Brown")
	press(m, "enter")
	assert.Equal(t, modeOverlay, m.mode)
	assert.Empty(t, allNotes(t, m))
	// A full name is exact.
	openBarAgain := func(line string) {
		press(m, "ctrl+u")
		typeText(m, line)
	}
	openBarAgain("n Bob Brown called back")
	require.True(t, m.ov.cmd.plan.Ready())
	press(m, "enter")
	assert.Len(t, allNotes(t, m), 1)
}

func TestCommandBarEscClosesWithoutWriting(t *testing.T) {
	m, _ := newTestModel(t)
	openBar(m, "n something")
	press(m, "esc")
	assert.Equal(t, modeNormal, m.mode)
	assert.Empty(t, allNotes(t, m))
}

func TestCommandBarTabCompletesVerbsNamesTagsDatesAndCadences(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "6")

	// Verb.
	press(m, "ctrl+k")
	typeText(m, "ta")
	press(m, "tab")
	assert.Equal(t, "task ", m.ov.filter.Value())
	require.NotNil(t, m.ov.cmd)

	// Name.
	press(m, "ctrl+u")
	typeText(m, "n Ba")
	assert.Equal(t, []string{"Bob Baker "}, m.ov.cmd.comp.Candidates)
	assert.Contains(t, plain(m.View()), "Bob Baker")
	press(m, "tab")
	assert.Equal(t, "n Bob Baker ", m.ov.filter.Value())

	// Tag.
	typeText(m, "met #fr")
	press(m, "tab")
	assert.Equal(t, "n Bob Baker met #friend", m.ov.filter.Value())

	// Date presets cycle with repeated tab.
	press(m, "ctrl+u")
	typeText(m, "n Bob hi by t")
	assert.Equal(t, []string{"today", "tomorrow", "tue", "thu"}, m.ov.cmd.comp.Candidates)
	press(m, "tab")
	assert.Equal(t, "n Bob hi by today", m.ov.filter.Value())
	press(m, "tab")
	assert.Equal(t, "n Bob hi by tomorrow", m.ov.filter.Value())
	press(m, "shift+tab")
	assert.Equal(t, "n Bob hi by today", m.ov.filter.Value())

	// Cadence.
	press(m, "ctrl+u")
	typeText(m, "n Bob hi every 3")
	press(m, "tab")
	assert.Equal(t, "n Bob hi every 3m", m.ov.filter.Value())

	// Up and down choose which candidate tab inserts.
	press(m, "ctrl+u")
	typeText(m, "t Bob hi !")
	press(m, "down", "tab")
	assert.Equal(t, "t Bob hi !high", m.ov.filter.Value())

	// The plan follows the completed text.
	require.NotNil(t, m.ov.cmd)
	assert.Contains(t, m.ov.cmd.plan.Preview, `task for Bob Baker: "hi"`)
	assert.Contains(t, m.ov.cmd.plan.Preview, "high priority")
}

func TestCommandBarTabWithNothingToCompleteSaysSo(t *testing.T) {
	m, _ := newTestModel(t)
	openBar(m, "n hello world")
	press(m, "tab")
	assert.Equal(t, "n hello world", m.ov.filter.Value())
	assert.Contains(t, plain(m.View()), "nothing to complete")
}

func TestCommandBarSingleFieldSetAppliesAtOnce(t *testing.T) {
	m, pr := newTestModel(t)
	openBar(m, "set company=Zorg")
	assert.Contains(t, plain(m.View()), "set Zed Adams: company Zorg")
	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode, "one field needs no confirmation")
	assert.Equal(t, "Zorg", *personByID(t, pr, 1).Company)
}

func TestCommandBarMultiFieldSetShowsOldAndNewAndNeedsConfirm(t *testing.T) {
	m, pr := newTestModel(t)
	openBar(m, "set company=Zorg title=CTO birthday=Mar 4")
	require.True(t, m.ov.cmd.plan.NeedsConfirm)
	press(m, "enter")
	require.Equal(t, modeOverlay, m.mode, "the first enter asks")
	require.True(t, m.ov.cmd.confirming)
	out := plain(m.View())
	assert.Contains(t, out, "Confirm Zed Adams")
	assert.Contains(t, out, "company")
	assert.Contains(t, out, "Acme -> Zorg")
	assert.Contains(t, out, "(empty) -> CTO")
	assert.Contains(t, out, "(empty) -> --03-04")
	assert.Equal(t, "Acme", *personByID(t, pr, 1).Company, "nothing is written before the second enter")

	// Esc goes back to editing.
	press(m, "esc")
	assert.False(t, m.ov.cmd.confirming)
	assert.Equal(t, modeOverlay, m.mode)

	press(m, "enter", "enter")
	assert.Equal(t, modeNormal, m.mode)
	p := personByID(t, pr, 1)
	assert.Equal(t, "Zorg", *p.Company)
	assert.Equal(t, "CTO", *p.Title)
	assert.Equal(t, 3, *p.BirthdayMonth)
}

func TestCommandBarUndoAndRedo(t *testing.T) {
	m, pr := newTestModel(t)
	openBar(m, "n called #deal remind fri")
	press(m, "enter")
	require.Len(t, allNotes(t, m), 1)
	assert.Contains(t, plain(m.View()), "Done: note on Zed Adams", "a toast confirms the command")
	openBar(m, "set company=Zorg")
	press(m, "enter")
	assert.Equal(t, "Zorg", *personByID(t, pr, 1).Company)
	require.Len(t, m.hist, 2)

	press(m, "ctrl+z")
	assert.Contains(t, plain(m.View()), "Undone: set company on Zed Adams")
	assert.Equal(t, "Acme", *personByID(t, pr, 1).Company)
	assert.Len(t, allNotes(t, m), 1)

	press(m, "ctrl+z")
	assert.Empty(t, allNotes(t, m), "the note is archived again")
	tasks, _ := m.tr.FindAll(context.Background(), model.TaskFilters{})
	assert.Empty(t, tasks)
	tags, _ := m.tagr.GetForEntity(context.Background(), "interaction", 1)
	assert.Empty(t, tags)

	press(m, "ctrl+z")
	assert.Contains(t, plain(m.View()), "Nothing to undo")

	press(m, "ctrl+y")
	assert.Contains(t, plain(m.View()), "Redone: add note on Zed Adams")
	assert.Len(t, allNotes(t, m), 1)
	tasks, _ = m.tr.FindAll(context.Background(), model.TaskFilters{})
	assert.Len(t, tasks, 1)
	press(m, "ctrl+y")
	assert.Equal(t, "Zorg", *personByID(t, pr, 1).Company, "ctrl+y is redo too")
	press(m, "ctrl+y")
	assert.Contains(t, plain(m.View()), "Nothing to redo")

	// A new change clears what could be redone.
	press(m, "ctrl+z")
	openBar(m, "set title=CTO")
	press(m, "enter")
	assert.Empty(t, m.redoStack)
	press(m, "ctrl+y")
	assert.Contains(t, plain(m.View()), "Nothing to redo")
}

func TestUndoIsRefusedWhenTheTargetChangedSince(t *testing.T) {
	m, pr := newTestModel(t)
	press(m, "enter") // nothing: stay in the list
	openBar(m, "set company=Zorg")
	press(m, "enter")
	// Someone else (a sync, the CLI) changes the same field.
	_, err := pr.Update(context.Background(), 1, model.UpdatePersonInput{Company: sp("Elsewhere")})
	require.NoError(t, err)
	press(m, "ctrl+z")
	assert.True(t, m.toast.isErr)
	assert.Contains(t, m.toast.text, "Cannot undo set company on Zed Adams")
	assert.Contains(t, m.toast.text, "company of Zed Adams changed since")
	assert.Equal(t, "Elsewhere", *personByID(t, pr, 1).Company, "nothing was written")
	assert.Len(t, m.hist, 1, "the entry stays, so it can be retried")

	// The same guard protects field edits made with f, p and the detail pane.
	press(m, "f")
	_, err = pr.Update(context.Background(), 1, model.UpdatePersonInput{IsFavorite: func() *bool { b := false; return &b }()})
	require.NoError(t, err)
	press(m, "ctrl+z")
	assert.Contains(t, m.toast.text, "favorite of Zed Adams changed since")
}

func TestUndoCoversEditsTagsBulkAndFavorites(t *testing.T) {
	m, pr := newTestModel(t)
	// Favorite, then undo.
	press(m, "f")
	assert.True(t, personByID(t, pr, 1).IsFavorite)
	press(m, "ctrl+z")
	assert.False(t, personByID(t, pr, 1).IsFavorite)
	assert.Contains(t, m.toast.text, "Undone: set favorite on Zed Adams")
	press(m, "ctrl+y")
	assert.True(t, personByID(t, pr, 1).IsFavorite)

	// Tag from the prompt, undo removes it.
	press(m, "T")
	typeText(m, "vip")
	press(m, "enter")
	tags, _ := m.tagr.GetForEntity(context.Background(), "person", 1)
	require.Len(t, tags, 1)
	press(m, "ctrl+z")
	tags, _ = m.tagr.GetForEntity(context.Background(), "person", 1)
	assert.Empty(t, tags)

	// One bulk action over marked people undoes in one step.
	press(m, " ", " ")
	press(m, "T")
	typeText(m, "batch")
	press(m, "enter")
	for _, id := range []int64{1, 2} {
		assert.True(t, m.hasTag(context.Background(), "person", id, "batch"), "person %d", id)
	}
	press(m, "ctrl+z")
	for _, id := range []int64{1, 2} {
		assert.False(t, m.hasTag(context.Background(), "person", id, "batch"), "person %d", id)
	}

	// An inline edit in the detail pane.
	press(m, "g", "tab")
	goTo(t, m, "company")
	press(m, "enter")
	replaceText(m, "Initrode")
	press(m, "enter")
	assert.Equal(t, "Initrode", *personByID(t, pr, 1).Company)
	press(m, "ctrl+z")
	assert.Equal(t, "Acme", *personByID(t, pr, 1).Company)
	assert.Contains(t, m.toast.text, "Undone: set company on Zed Adams")
}

func TestUndoNoteDeleteAndTaskComplete(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "n")
	typeText(m, "a note")
	press(m, "enter")
	press(m, "t")
	typeText(m, "a task")
	press(m, "enter")
	require.Len(t, m.hist, 2)
	press(m, "ctrl+z", "ctrl+z")
	assert.Empty(t, allNotes(t, m))
	tasks, _ := m.tr.FindAll(context.Background(), model.TaskFilters{})
	assert.Empty(t, tasks)
	press(m, "ctrl+y", "ctrl+y")
	assert.Len(t, allNotes(t, m), 1)
	tasks, _ = m.tr.FindAll(context.Background(), model.TaskFilters{})
	assert.Len(t, tasks, 1)

	// Completing a task, and undoing it, reopens it.
	press(m, "4", "x")
	tasks, _ = m.tr.FindAll(context.Background(), model.TaskFilters{})
	assert.Empty(t, tasks)
	press(m, "ctrl+z")
	tasks, _ = m.tr.FindAll(context.Background(), model.TaskFilters{})
	assert.Len(t, tasks, 1)
	assert.Contains(t, m.toast.text, "Undone: complete task a task")
}

func TestToastExpiresAndFitsNarrowTerminals(t *testing.T) {
	m, _ := newTestModel(t)
	cmd := m.showToast("a very long message that will not fit in a narrow terminal at all, really", false)
	require.NotNil(t, cmd)
	for _, size := range [][2]int{{100, 30}, {40, 10}, {20, 8}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		lines := strings.Split(plain(m.View()), "\n")
		assert.LessOrEqual(t, len(lines), size[1])
		for _, l := range lines {
			assert.LessOrEqual(t, lipgloss.Width(l), size[0])
		}
		assert.Contains(t, plain(m.View()), "a ")
	}
	// A stale timer does not clear a newer toast.
	old := m.toast.seq
	m.showToast("newer", false)
	m.Update(toastExpireMsg{seq: old})
	require.NotNil(t, m.toast)
	m.Update(toastExpireMsg{seq: m.toast.seq})
	assert.Nil(t, m.toast)
	assert.NotContains(t, plain(m.View()), "newer")
}

func TestToastShowsOverOtherScreensAndInReadOnly(t *testing.T) {
	m, _ := newTestModel(t)
	m.readOnly = true
	press(m, "ctrl+z")
	assert.True(t, m.toast.isErr)
	assert.Contains(t, plain(m.View()), "read-only: undo is disabled")
	press(m, "?")
	assert.Contains(t, plain(m.View()), "read-only: undo is disabled", "the toast shows on the help screen too")
}

// paletteItem finds an action in the open palette by id.
func paletteItem(t *testing.T, m *Model, id string) overlayItem {
	t.Helper()
	for _, idx := range m.ov.shown {
		if m.ov.items[idx].id == id {
			return m.ov.items[idx]
		}
	}
	t.Fatalf("action %s is not in the palette", id)
	return overlayItem{}
}

func TestUndoAndRedoArePaletteActions(t *testing.T) {
	m, pr := newTestModel(t)
	press(m, "f")
	press(m, "ctrl+k")
	typeText(m, "undo")
	assert.Empty(t, paletteItem(t, m, "undo").disabled)
	press(m, "enter")
	assert.False(t, personByID(t, pr, 1).IsFavorite)
	press(m, "ctrl+k")
	typeText(m, "redo")
	press(m, "enter")
	assert.True(t, personByID(t, pr, 1).IsFavorite)
	press(m, "esc")
	m.readOnly = true
	press(m, "ctrl+k")
	typeText(m, "undo")
	assert.Equal(t, "read-only", paletteItem(t, m, "undo").disabled)
}

func TestCommandBarReadOnlyOnlyLooks(t *testing.T) {
	m, _ := newTestModel(t)
	m.readOnly = true
	openBar(m, "n hello")
	require.NotNil(t, m.ov.cmd)
	assert.Contains(t, plain(m.View()), "read-only")
	press(m, "enter")
	assert.Equal(t, modeOverlay, m.mode)
	assert.Contains(t, plain(m.View()), "read-only: this line would change data")
	assert.Empty(t, allNotes(t, m))
}

func TestCommandBarLogTaskTagAndSnooze(t *testing.T) {
	m, pr := newTestModel(t)
	openBar(m, "log meeting kickoff yesterday")
	press(m, "enter")
	its := allNotes(t, m)
	require.Len(t, its, 1)
	assert.Equal(t, "meeting", its[0].Type)
	assert.Equal(t, "2026-10-12 12:00:00", its[0].OccurredAt)

	openBar(m, "t send deck fri 3pm !high")
	assert.Contains(t, plain(m.View()), `task for Zed Adams: "send deck"`)
	press(m, "enter")
	tasks, _ := m.tr.FindAll(context.Background(), model.TaskFilters{})
	require.Len(t, tasks, 1)
	assert.Equal(t, "2026-10-16 15:00", *tasks[0].DueAt)
	assert.Equal(t, "high", tasks[0].Priority)

	openBar(m, "tag vip -friend")
	press(m, "enter")
	tags, _ := m.tagr.GetForEntity(context.Background(), "person", 1)
	require.Len(t, tags, 1)
	assert.Equal(t, "vip", tags[0].Name)

	openBar(m, "snooze 7")
	press(m, "enter")
	assert.Equal(t, "2026-10-20 12:00:00", *personByID(t, pr, 1).KeepInTouchSnoozedUntil)
}

func TestCommandBarKeepsActionSearchForActionTitles(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+k")
	typeText(m, "set keep")
	assert.Nil(t, m.ov.cmd, "set keep is the start of an action title")
	typeText(m, "x")
	assert.NotNil(t, m.ov.cmd, "set keepx is no action, so it is read as a command")
	press(m, "ctrl+u")
	assert.Nil(t, m.ov.cmd)
	assert.NotEmpty(t, m.ov.shown)
}

func TestCommandBarFitsTheTerminal(t *testing.T) {
	m, _ := newTestModel(t)
	for _, size := range [][2]int{{100, 30}, {60, 12}, {40, 10}, {120, 50}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, line := range []string{
			"n a rather long note with many words that goes on and on and on and on #deal remind fri 3pm every 3m !high ~30m",
			"set company=Zorg title=CTO birthday=Mar 4",
			"n Ba",
			"n ",
		} {
			press(m, "ctrl+k")
			typeText(m, line)
			if strings.HasPrefix(line, "set") {
				press(m, "enter")
			}
			out := plain(m.View())
			lines := strings.Split(out, "\n")
			assert.LessOrEqual(t, len(lines), size[1], "height at %v for %q", size, line)
			for _, l := range lines {
				assert.LessOrEqual(t, lipgloss.Width(l), size[0], "width at %v for %q: %q", size, line, l)
			}
			press(m, "esc", "esc")
		}
	}
}

func TestCommandBarMouseIsIgnoredInCommandMode(t *testing.T) {
	m, _ := newTestModel(t)
	openBar(m, "n hi")
	m.Update(tea.MouseMsg{X: 3, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m.Update(tea.MouseMsg{X: 3, Y: 4, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	assert.Equal(t, modeOverlay, m.mode)
	assert.Empty(t, allNotes(t, m))
}

func TestNotePromptAppliesTokens(t *testing.T) {
	m, pr := newTestModel(t)
	press(m, "2", "n")
	typeText(m, "met at the conference #vip @Cara_Cole remind fri every 3m")
	press(m, "enter")
	assert.Equal(t, "note added, #vip, reminder 2026-10-16, cadence 3 mons", m.status)
	its := allNotes(t, m)
	require.Len(t, its, 1)
	assert.Equal(t, "met at the conference", *its[0].Content)
	assert.Len(t, its[0].PersonIDs, 2, "@Name links a second person")
	tags, _ := m.tagr.GetForEntity(context.Background(), "interaction", its[0].ID)
	require.Len(t, tags, 1)
	assert.Equal(t, "vip", tags[0].Name)
	tasks, _ := m.tr.FindAll(context.Background(), model.TaskFilters{})
	require.Len(t, tasks, 1)
	assert.Equal(t, "2026-10-16", *tasks[0].DueAt)
	assert.Equal(t, "3 mons", *personByID(t, pr, 1).KeepInTouch)

	// A colon in the text is not a name.
	press(m, "n")
	typeText(m, "Update: all good")
	press(m, "enter")
	assert.Equal(t, "note added", m.status)
	assert.False(t, m.statusIsErr)

	// An unknown +Name is an error and writes nothing.
	press(m, "n")
	typeText(m, "hello @Nobody")
	press(m, "enter")
	assert.True(t, m.statusIsErr)
	assert.Len(t, allNotes(t, m), 2)
}

func TestNotePromptKeepsTheChosenType(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "2", "n", "tab")
	typeText(m, "call went well remind fri")
	press(m, "enter")
	assert.Contains(t, m.status, "added")
	its := allNotes(t, m)
	require.Len(t, its, 1)
	assert.NotEqual(t, "note", its[0].Type)
	tasks, _ := m.tr.FindAll(context.Background(), model.TaskFilters{})
	assert.Len(t, tasks, 1, "@ is a reminder for every note type")
}

func TestTaskPromptAppliesTokensAndKeepsThePipe(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "t")
	typeText(m, "send deck fri !high ~30m")
	press(m, "enter")
	assert.Equal(t, "task added", m.status)
	tasks, _ := m.tr.FindAll(context.Background(), model.TaskFilters{})
	require.Len(t, tasks, 1)
	assert.Equal(t, "send deck", tasks[0].Title)
	assert.Equal(t, "2026-10-16", *tasks[0].DueAt)
	assert.Equal(t, "high", tasks[0].Priority)

	// The old "title | due" form still wins over a token, and a natural date works after the pipe.
	press(m, "t")
	typeText(m, "second tue | fri")
	press(m, "enter")
	tasks, _ = m.tr.FindAll(context.Background(), model.TaskFilters{})
	require.Len(t, tasks, 2)
	var second *model.Task
	for _, tk := range tasks {
		if tk.Title == "second" {
			second = tk
		}
	}
	require.NotNil(t, second)
	assert.Equal(t, "2026-10-16", *second.DueAt)
}

func TestShortSyntaxToggle(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+k")
	typeText(m, "toggle short")
	press(m, "enter")
	assert.True(t, m.noSyntax)
	assert.Contains(t, m.status, "short syntax off")

	press(m, "n")
	typeText(m, "keep #this remind fri")
	press(m, "enter")
	its := allNotes(t, m)
	require.Len(t, its, 1)
	assert.Equal(t, "keep #this remind fri", *its[0].Content)

	openBar(m, "n and #this too")
	assert.Contains(t, plain(m.View()), `"and #this too"`)
	press(m, "esc")

	press(m, "ctrl+k")
	typeText(m, "toggle short")
	press(m, "enter")
	assert.False(t, m.noSyntax)
}

func TestPeopleSearchUnderstandsPriorityAndCadenceTokens(t *testing.T) {
	m, pr := newTestModel(t)
	ctx := context.Background()
	tier := 2
	_, err := pr.Update(ctx, 3, model.UpdatePersonInput{PriorityTier: &tier}) // Cara Cole: high
	require.NoError(t, err)
	require.NoError(t, m.reload())

	press(m, "/")
	typeText(m, "!high")
	assert.Equal(t, []string{"Cara Cole"}, names(m))
	press(m, "ctrl+u")
	typeText(m, "!low")
	assert.Empty(t, names(m))
	press(m, "ctrl+u")
	typeText(m, "every 1w")
	assert.Equal(t, []string{"Bob Baker"}, names(m), "Bob is on 7 days")
	press(m, "ctrl+u")
	typeText(m, "#friend every 1w")
	assert.Equal(t, []string{"Bob Baker"}, names(m))
	press(m, "ctrl+u")
	typeText(m, "#friend !high")
	assert.Empty(t, names(m))
	press(m, "ctrl+u")
	typeText(m, "!top cole")
	assert.Empty(t, names(m))
	press(m, "ctrl+u")
	typeText(m, "!high cole")
	assert.Equal(t, []string{"Cara Cole"}, names(m))
}

func TestCommandBarActionsAreInTheRegistry(t *testing.T) {
	m, _ := newTestModel(t)
	for _, id := range []string{"undo", "syntax-toggle"} {
		a := m.actionByID(id)
		require.NotNil(t, a, id)
		assert.NotEmpty(t, a.desc)
	}
	assert.True(t, m.actionByID("undo").write, "undo changes data, so read-only blocks it")
	assert.False(t, m.actionByID("syntax-toggle").write)
	assert.Contains(t, m.helpText(), "undo the last data change")
}

var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

// plain drops style codes, so tests read the text the user sees whatever the theme.
func plain(s string) string { return ansiRE.ReplaceAllString(s, "") }

func TestTasksCarryTagsInTheTui(t *testing.T) {
	m, _ := newTestModel(t)
	// A task made from the prompt reads #tag.
	press(m, "t")
	typeText(m, "send deck #work")
	press(m, "enter")
	tasks, _ := m.tr.FindAll(context.Background(), model.TaskFilters{})
	require.Len(t, tasks, 1)
	assert.Equal(t, "send deck", tasks[0].Title)
	assert.Equal(t, []string{"work"}, tasks[0].Tags)

	// Tags show in the Tasks view rows.
	press(m, "4")
	require.Len(t, m.rows, 1)
	assert.Contains(t, m.rows[0].sub, "#work")
	assert.Contains(t, m.View(), "#work")

	// T tags the task, not its person, and -name removes.
	press(m, "T")
	typeText(m, "urgent")
	press(m, "enter")
	tasks, _ = m.tr.FindAll(context.Background(), model.TaskFilters{})
	assert.Equal(t, []string{"urgent", "work"}, tasks[0].Tags)
	people, _ := m.tagr.GetEntities(context.Background(), "urgent", "person")
	assert.Empty(t, people)
	assert.Contains(t, m.rows[0].sub, "#urgent")
	press(m, "T")
	typeText(m, "-work")
	press(m, "enter")
	tasks, _ = m.tr.FindAll(context.Background(), model.TaskFilters{})
	assert.Equal(t, []string{"urgent"}, tasks[0].Tags)

	// Undo covers task tags.
	press(m, "ctrl+z")
	tasks, _ = m.tr.FindAll(context.Background(), model.TaskFilters{})
	assert.Equal(t, []string{"urgent", "work"}, tasks[0].Tags)
	assert.Contains(t, m.toast.text, "Undone: remove tag #work from task")

	// In the detail pane a task row shows its tags and T tags it too.
	press(m, "2", "tab")
	goTo(t, m, "task")
	assert.Contains(t, m.View(), "#urgent")
	press(m, "T")
	typeText(m, "fromdetail")
	press(m, "enter")
	tasks, _ = m.tr.FindAll(context.Background(), model.TaskFilters{})
	assert.Contains(t, tasks[0].Tags, "fromdetail")

	// The command bar tags a task from its text as well.
	openBar(m, "t call plumber #home")
	assert.NotContains(t, m.View(), "stays in the title")
	assert.Contains(t, m.View(), "tag #home (new)")
	press(m, "enter")
	home := "home"
	tasks, _ = m.tr.FindAll(context.Background(), model.TaskFilters{Tag: &home})
	require.Len(t, tasks, 1)
	assert.Equal(t, "call plumber", tasks[0].Title)
}
func TestCommandBarDismissTurnsAChipBackIntoText(t *testing.T) {
	m, _ := newTestModel(t)
	openBar(m, "t see the doc fri #work")
	require.Contains(t, m.View(), "due Fri 16 Oct")
	// The cursor is after #work: alt+d turns that last chip back into text.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}, Alt: true})
	assert.Contains(t, m.toast.text, "Kept as plain text: #work")
	assert.NotContains(t, m.View(), "tag #work")
	assert.Contains(t, m.View(), "due Fri 16 Oct", "other chips stay")
	// Move the cursor onto fri and dismiss it too.
	press(m, "left", "left", "left", "left", "left", "left", "left")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}, Alt: true})
	assert.NotContains(t, m.ov.cmd.plan.Preview, "due")
	assert.Contains(t, m.ov.cmd.plan.Preview, `"see the doc fri #work"`)
	press(m, "enter")
	tasks, _ := m.tr.FindAll(context.Background(), model.TaskFilters{})
	require.Len(t, tasks, 1)
	assert.Equal(t, "see the doc fri #work", tasks[0].Title)
	assert.Nil(t, tasks[0].DueAt)
}

func TestNotesDoNotReadBareDatesInTheBar(t *testing.T) {
	m, _ := newTestModel(t)
	openBar(m, "n said fri is fine")
	assert.Contains(t, m.View(), `"said fri is fine"`)
	assert.NotContains(t, m.ov.cmd.plan.Preview, "remind")
	press(m, "ctrl+u")
	typeText(m, "n said fri is fine remind next week")
	assert.Contains(t, m.View(), "remind Mon 19 Oct")
}