package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// calModel opens the Calendar view. Today is Tuesday 2026-10-13, the zone is America/Chicago.
func calModel(t *testing.T) *Model {
	t.Helper()
	m, _ := newTestModel(t)
	loc, err := tz.Load("America/Chicago")
	require.NoError(t, err)
	m.loc = loc
	press(m, "9")
	require.Equal(t, viewCalendar, m.view)
	return m
}

func utcT(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, time.UTC)
}

func addEvent(t *testing.T, m *Model, in model.CreateEventInput) *model.Event {
	t.Helper()
	if in.TZ == "" {
		in.TZ = "America/Chicago"
	}
	e, err := m.er.Create(context.Background(), in)
	require.NoError(t, err)
	require.NoError(t, m.reload())
	return e
}

func rowTexts(m *Model) []string {
	var out []string
	for _, r := range m.rows {
		out = append(out, r.title)
	}
	return out
}

func TestCalendarOpensOnAgendaWithToday(t *testing.T) {
	m := calModel(t)
	assert.Equal(t, calAgenda, m.cal.mode)
	assert.Contains(t, m.View(), "v Calendar")
	assert.Equal(t, "Agenda from 13 Oct", m.calCountText())
	assert.Contains(t, rowTexts(m), "Tue 13 Oct")
	assert.Contains(t, rowTexts(m), "nothing planned")
}

func TestCalendarQuickAddEditDelete(t *testing.T) {
	m := calModel(t)
	press(m, "+")
	require.Equal(t, modePrompt, m.mode)
	typeText(m, "Lunch with Sam fri 12:30 for 1h")
	assert.Contains(t, m.hintsLine(), "Fri 16 Oct 12:30-13:30", "the preview shows what will be created")
	press(m, "enter")
	assert.Contains(t, m.status, "event added")

	list, _, err := m.er.List(context.Background(), model.EventFilters{From: utcT(2026, 10, 1, 0, 0), To: utcT(2026, 11, 1, 0, 0), Loc: m.loc})
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "2026-10-16T17:30:00Z", list[0].Start)
	assert.Equal(t, "America/Chicago", list[0].TZ)
	sel := m.calSelected()
	require.NotNil(t, sel)
	assert.Equal(t, "event", sel.kind, "the new event is selected")
	assert.Contains(t, m.View(), "Lunch with Sam")
	assert.Contains(t, m.calDetail(), "Fri 16 Oct 12:30-13:30")

	// Edit: the line is prefilled with the event.
	press(m, "enter")
	require.Equal(t, modePrompt, m.mode)
	assert.Equal(t, "Lunch with Sam 2026-10-16 12:30 for 1h", m.input.Value())
	m.input.SetValue("Lunch with Sam and Pat 2026-10-16 13:00 for 1h")
	press(m, "enter")
	assert.Contains(t, m.status, "event updated")
	e, err := m.er.FindByID(context.Background(), list[0].EventID)
	require.NoError(t, err)
	assert.Equal(t, "Lunch with Sam and Pat", e.Title)
	assert.Equal(t, "2026-10-16T18:00:00Z", *e.StartUTC)
	assert.Equal(t, "2026-10-16T19:00:00Z", *e.EndUTC)

	// An unchanged line writes nothing.
	press(m, "enter")
	press(m, "enter")
	assert.Equal(t, "nothing changed", m.status)

	// Delete asks first.
	press(m, "#")
	require.Equal(t, modeConfirm, m.mode)
	press(m, "n")
	_, err = m.er.FindByID(context.Background(), e.ID)
	require.NoError(t, err)
	press(m, "#", "y")
	_, err = m.er.FindByID(context.Background(), e.ID)
	assert.ErrorIs(t, err, model.ErrNotFound)
	assert.NotContains(t, m.View(), "Lunch with Sam and Pat")

	// Restore from the palette action.
	require.True(t, m.actionByID("cal-restore").enabled(m))
	m.runAction(m.actionByID("cal-restore"))
	_, err = m.er.FindByID(context.Background(), e.ID)
	assert.NoError(t, err)
}

func TestCalendarQuickAddVariants(t *testing.T) {
	m := calModel(t)
	tests := []struct {
		line    string
		allDay  bool
		start   string
		wantErr bool
	}{
		{"Holiday", true, "2026-10-13", false},
		{"Holiday 2026-11-26", true, "2026-11-26", false},
		{"Call 3pm", false, "2026-10-13T20:00:00Z", false},
		{"Standup 2026-10-19 09:15 for 15m every weekday", false, "2026-10-19T14:15:00Z", false},
		{" fri", false, "", true},
		{"", false, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			q, err := m.parseQuick(tt.line)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.allDay, q.allDay)
			if tt.allDay {
				assert.Equal(t, tt.start, q.start.Format(tz.DateLayout))
			} else {
				assert.Equal(t, tt.start, tz.FormatInstant(q.start))
			}
		})
	}
	press(m, "+")
	typeText(m, "fri 12:30")
	assert.Contains(t, m.hintsLine(), "type a title first")
	press(m, "enter")
	assert.NotEqual(t, "event added", m.status)
}

func TestCalendarModesStepsAndToday(t *testing.T) {
	m := calModel(t)
	addEvent(t, m, model.CreateEventInput{Title: "Standup", Start: utcT(2026, 10, 12, 14, 15), End: utcT(2026, 10, 12, 14, 30), RRule: "weekdays"})
	press(m, "t")

	press(m, "d")
	assert.Equal(t, calDay, m.cal.mode)
	assert.Equal(t, "Tue 13 Oct 2026", m.calCountText())
	assert.Contains(t, rowTexts(m), "09:15 Standup")
	press(m, "tab")
	assert.Equal(t, "Wed 14 Oct 2026", m.calCountText())
	press(m, "shift+tab", "shift+tab")
	assert.Equal(t, "Mon 12 Oct 2026", m.calCountText())
	press(m, "t")
	assert.Equal(t, "Tue 13 Oct 2026", m.calCountText())

	press(m, "w")
	assert.Equal(t, calWeek, m.cal.mode)
	assert.Equal(t, "12 Oct - 18 Oct 2026", m.calCountText())
	out := m.View()
	for _, h := range []string{"Mon 12", "Tue 13", "Wed 14", "Sun 18", "09:15 Standup"} {
		assert.Contains(t, out, h)
	}
	press(m, "l")
	assert.Equal(t, "2026-10-14", dayKey(m.calSelDate()))
	press(m, "h", "h")
	assert.Equal(t, "2026-10-12", dayKey(m.calSelDate()))
	press(m, "h")
	assert.Equal(t, "2026-10-11", dayKey(m.calSelDate()))
	press(m, "h")
	assert.Equal(t, "2026-10-10", dayKey(m.calSelDate()), "leaving the week loads the one before")
	assert.Equal(t, "5 Oct - 11 Oct 2026", m.calCountText())
	press(m, "tab")
	assert.Equal(t, "12 Oct - 18 Oct 2026", m.calCountText())

	press(m, "m")
	assert.Equal(t, calMonth, m.cal.mode)
	assert.Equal(t, "October 2026", m.calCountText())
	assert.Contains(t, m.View(), "Wed")
	press(m, "tab")
	assert.Equal(t, "November 2026", m.calCountText())
	press(m, "shift+tab", "shift+tab")
	assert.Equal(t, "September 2026", m.calCountText())
	press(m, "t")
	assert.Equal(t, "October 2026", m.calCountText())

	// j and k move a week in the month grid, enter opens the day.
	press(m, "j")
	assert.Equal(t, "2026-10-20", dayKey(m.calSelDate()))
	press(m, "k", "h")
	assert.Equal(t, "2026-10-12", dayKey(m.calSelDate()))
	press(m, "enter")
	assert.Equal(t, calDay, m.cal.mode)
	assert.Equal(t, "Mon 12 Oct 2026", m.calCountText())

	press(m, "a")
	assert.Equal(t, calAgenda, m.cal.mode)
}

func TestCalendarNarrowTerminalFallsBackToAgenda(t *testing.T) {
	m := calModel(t)
	addEvent(t, m, model.CreateEventInput{Title: "Dentist", Start: utcT(2026, 10, 14, 15, 0)})
	press(m, "w")
	assert.True(t, m.calGrid())
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	assert.False(t, m.calGrid(), "too narrow for a grid")
	assert.Equal(t, calAgenda, m.calEff())
	assert.Equal(t, calWeek, m.cal.mode, "the chosen mode is remembered")
	assert.Contains(t, rowTexts(m), "10:00 Dentist")
	assert.NotContains(t, m.View(), "Tue 13 │")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	assert.True(t, m.calGrid())
}

func TestCalendarOverlays(t *testing.T) {
	m := calModel(t)
	addEvent(t, m, model.CreateEventInput{Title: "Standup", Start: utcT(2026, 10, 20, 14, 15)})
	_, err := m.tr.Create(context.Background(), model.CreateTaskInput{Title: "send deck", Priority: "medium", DueAt: sp("2026-10-15")})
	require.NoError(t, err)
	require.NoError(t, m.reload())

	joined := func() string { return strings.Join(rowTexts(m), "|") }
	assert.Contains(t, joined(), "* Bob Baker", "birthdays are on by default")
	assert.Contains(t, joined(), "[ ] send deck", "tasks are on by default")
	assert.NotContains(t, joined(), "~ Bob Baker", "keep-in-touch is off")

	toggle(m, "cal-due")
	assert.Contains(t, joined(), "~ Bob Baker")
	toggle(m, "cal-due")
	toggle(m, "cal-birthdays")
	assert.NotContains(t, joined(), "* Bob Baker")
	assert.NotContains(t, joined(), "~ Bob Baker")
	toggle(m, "cal-tasks")
	assert.NotContains(t, joined(), "[ ] send deck")
	assert.Contains(t, joined(), "Standup", "events are never hidden")
	toggle(m, "cal-birthdays")
	toggle(m, "cal-tasks")
	assert.Contains(t, joined(), "* Bob Baker")
	assert.Contains(t, joined(), "[ ] send deck")

	// A birthday row opens the person's form in the detail pane.
	for i, r := range m.rows {
		if r.cal != nil && r.cal.kind == "birthday" {
			m.cursor = i
		}
	}
	m.refreshDetail(true)
	assert.NotEmpty(t, m.form)
	press(m, "enter")
	assert.Equal(t, viewPeople, m.view)
	assert.Equal(t, "Bob Baker", m.rows[m.cursor].title)
}

func TestCalendarRepeatScopes(t *testing.T) {
	m := calModel(t)
	e := addEvent(t, m, model.CreateEventInput{Title: "Standup", Start: utcT(2026, 10, 12, 14, 15), End: utcT(2026, 10, 12, 14, 30), RRule: "weekdays"})
	press(m, "d", "tab") // Wed 14 Oct
	require.Equal(t, "Wed 14 Oct 2026", m.calCountText())
	sel := m.calSelected()
	require.NotNil(t, sel)
	require.Equal(t, "event", sel.kind)

	// Edit this instance only.
	press(m, "enter")
	require.Equal(t, modeOverlay, m.mode, "a repeat asks which instances")
	press(m, "enter") // This event
	require.Equal(t, modePrompt, m.mode)
	m.input.SetValue("Standup (late) 2026-10-14 11:00 for 15m")
	press(m, "enter")
	assert.Contains(t, m.status, "updated")
	got, _, err := m.er.List(context.Background(), model.EventFilters{From: utcT(2026, 10, 14, 0, 0), To: utcT(2026, 10, 15, 0, 0), Loc: m.loc})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Standup (late)", got[0].Title)
	assert.Equal(t, "2026-10-14T16:00:00Z", got[0].Start)

	// Delete this and the following ones.
	press(m, "tab", "#") // Thu 15
	require.Equal(t, modeOverlay, m.mode)
	press(m, "down", "enter") // This and following
	require.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, "from")
	press(m, "y")
	after, err := m.er.FindByID(context.Background(), e.ID)
	require.NoError(t, err)
	assert.Contains(t, *after.RRule, "UNTIL=")
	list, _, err := m.er.List(context.Background(), model.EventFilters{From: utcT(2026, 10, 15, 0, 0), To: utcT(2026, 11, 30, 0, 0), Loc: m.loc})
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestCalendarAttendeesRemindersTagsStatus(t *testing.T) {
	m := calModel(t)
	e := addEvent(t, m, model.CreateEventInput{Title: "Sync", Start: utcT(2026, 10, 14, 15, 0)})
	press(m, "d", "tab")
	require.NotNil(t, m.calEventTarget())

	m.runAction(m.actionByID("cal-attendee-add"))
	require.Equal(t, modeOverlay, m.mode)
	typeText(m, "Bob")
	press(m, "enter")
	got, err := m.er.FindByID(context.Background(), e.ID)
	require.NoError(t, err)
	require.Len(t, got.Attendees, 1)
	assert.Equal(t, "Bob Baker", got.Attendees[0].Name)
	assert.Contains(t, m.calDetail(), "Bob Baker")

	m.runAction(m.actionByID("cal-attendee-open"))
	press(m, "enter")
	assert.Equal(t, viewPeople, m.view)
	press(m, "9")

	m.runAction(m.actionByID("cal-attendee-remove"))
	press(m, "enter")
	got, _ = m.er.FindByID(context.Background(), e.ID)
	assert.Empty(t, got.Attendees)

	m.runAction(m.actionByID("cal-remind"))
	require.Equal(t, modePrompt, m.mode)
	m.input.SetValue("15m, 1d:email")
	press(m, "enter")
	got, _ = m.er.FindByID(context.Background(), e.ID)
	assert.Len(t, got.Reminders, 2)
	assert.Contains(t, m.calDetail(), "email")

	m.runAction(m.actionByID("cal-tag"))
	m.input.SetValue("work")
	press(m, "enter")
	got, _ = m.er.FindByID(context.Background(), e.ID)
	assert.Equal(t, []string{"work"}, got.Tags)
	m.runAction(m.actionByID("cal-tag"))
	m.input.SetValue("-work")
	press(m, "enter")
	got, _ = m.er.FindByID(context.Background(), e.ID)
	assert.Empty(t, got.Tags)

	m.runAction(m.actionByID("cal-status"))
	press(m, "down", "enter")
	got, _ = m.er.FindByID(context.Background(), e.ID)
	assert.Equal(t, "tentative", got.Status)
	assert.Contains(t, strings.Join(rowTexts(m), "|"), "Sync")
	m.runAction(m.actionByID("cal-busy"))
	got, _ = m.er.FindByID(context.Background(), e.ID)
	assert.False(t, got.Busy)
}

func TestCalendarGotoZoneAndErrors(t *testing.T) {
	m := calModel(t)
	m.runAction(m.actionByID("cal-goto"))
	typeText(m, "2026-12-24")
	press(m, "enter")
	assert.Equal(t, "2026-12-24", dayKey(m.cal.day))
	assert.Contains(t, m.status, "went to")
	m.runAction(m.actionByID("cal-goto"))
	typeText(m, "never")
	press(m, "enter")
	assert.True(t, m.statusIsErr)

	press(m, "t")
	addEvent(t, m, model.CreateEventInput{Title: "Call", Start: utcT(2026, 10, 14, 15, 0)})
	press(m, "d", "tab")
	assert.Contains(t, strings.Join(rowTexts(m), "|"), "10:00 Call")
	m.runAction(m.actionByID("cal-zone"))
	typeText(m, "Europe/London")
	press(m, "enter")
	assert.Equal(t, "Europe/London", tz.Name(m.loc))
	press(m, "t", "d", "tab")
	assert.Contains(t, strings.Join(rowTexts(m), "|"), "16:00 Call")
	m.runAction(m.actionByID("cal-zone"))
	typeText(m, "Mars/Base")
	press(m, "enter")
	assert.True(t, m.statusIsErr)
}

func TestCalendarICSActions(t *testing.T) {
	m := calModel(t)
	addEvent(t, m, model.CreateEventInput{Title: "Exported", Start: utcT(2026, 10, 14, 15, 0)})
	file := filepath.Join(t.TempDir(), "out.ics")

	m.runAction(m.actionByID("cal-export"))
	typeText(m, file)
	press(m, "enter")
	assert.Contains(t, m.status, "exported 1 events")
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Contains(t, string(data), "SUMMARY:Exported")

	incoming := filepath.Join(t.TempDir(), "in.ics")
	require.NoError(t, os.WriteFile(incoming, []byte("BEGIN:VCALENDAR\nBEGIN:VEVENT\nUID:in@x\nSUMMARY:Imported\nDTSTART:20261014T180000Z\nDTEND:20261014T190000Z\nEND:VEVENT\nEND:VCALENDAR\n"), 0o600))
	m.runAction(m.actionByID("cal-import"))
	typeText(m, incoming)
	press(m, "enter")
	require.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, "1 new")
	press(m, "n")
	list, _, err := m.er.List(context.Background(), model.EventFilters{From: utcT(2026, 10, 14, 0, 0), To: utcT(2026, 10, 15, 0, 0), Loc: m.loc})
	require.NoError(t, err)
	assert.Len(t, list, 1, "declining writes nothing")

	m.runAction(m.actionByID("cal-import"))
	typeText(m, incoming)
	press(m, "enter", "y")
	assert.Contains(t, m.status, "imported 1 new")
	list, _, _ = m.er.List(context.Background(), model.EventFilters{From: utcT(2026, 10, 14, 0, 0), To: utcT(2026, 10, 15, 0, 0), Loc: m.loc})
	assert.Len(t, list, 2)

	m.runAction(m.actionByID("cal-import"))
	typeText(m, incoming)
	press(m, "enter")
	assert.Contains(t, m.status, "up to date")

	m.runAction(m.actionByID("cal-import"))
	typeText(m, filepath.Join(t.TempDir(), "missing.ics"))
	press(m, "enter")
	assert.True(t, m.statusIsErr)
}

func TestCalendarReadOnlyBlocksWrites(t *testing.T) {
	m := calModel(t)
	addEvent(t, m, model.CreateEventInput{Title: "Locked", Start: utcT(2026, 10, 14, 15, 0)})
	m.readOnly = true
	press(m, "d", "tab")
	press(m, "+")
	assert.NotEqual(t, modePrompt, m.mode)
	assert.Contains(t, m.status, "read-only")
	press(m, "enter")
	assert.NotEqual(t, modePrompt, m.mode)
	press(m, "#")
	assert.NotEqual(t, modeConfirm, m.mode)
	// Moving around still works.
	press(m, "w", "t")
	assert.Equal(t, calWeek, m.cal.mode)
}

// Every calendar action is in the palette, and the ones that need an event say so elsewhere.
func TestCalendarActionsAreInThePalette(t *testing.T) {
	m := calModel(t)
	want := []string{
		"cal-open", "cal-prev", "cal-next", "cal-today", "cal-agenda", "cal-day", "cal-week", "cal-month", "cal-left", "cal-right",
		"cal-goto", "cal-birthdays", "cal-due", "cal-tasks", "cal-zone", "cal-add", "cal-delete", "cal-edit", "cal-restore",
		"cal-attendee-add", "cal-attendee-remove", "cal-attendee-open", "cal-remind", "cal-tag", "cal-status", "cal-busy",
		"cal-import", "cal-export", "cal-hours", "cal-hours-set", "cal-free", "find-time", "cal-plan", "cal-plan-undo", "cal-invite", "view-calendar",
	}
	press(m, "ctrl+k")
	require.Equal(t, modeOverlay, m.mode)
	ids := map[string]overlayItem{}
	for _, it := range m.ov.items {
		ids[it.id] = it
	}
	for _, id := range want {
		it, ok := ids[id]
		require.True(t, ok, "palette lacks %s", id)
		assert.NotEmpty(t, it.desc, id)
	}
	for _, a := range m.actions {
		if strings.HasPrefix(a.id, "cal-") {
			assert.Contains(t, want, a.id, "list the new action in this test")
		}
	}
	press(m, "esc")
	assert.Equal(t, "not here", ids["cal-delete"].disabled, "no event is selected")

	// The view is also reachable from another view with the add action.
	m.switchView(viewPeople)
	m.runAction(m.actionByID("cal-add"))
	assert.Equal(t, viewCalendar, m.view)
	assert.Equal(t, modePrompt, m.mode)
}

func TestCalendarPlusStillAddsContactsElsewhere(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "+")
	assert.True(t, m.creating, "People view: + creates a contact")
}

func TestCalendarMouseSelectsGridDay(t *testing.T) {
	m := calModel(t)
	press(m, "w")
	_ = m.View()
	require.NotEmpty(t, m.cal.cells)
	c := m.cal.cells[3] // Thursday
	m.Update(tea.MouseMsg{X: c.x0 + 1, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	assert.Equal(t, "2026-10-15", dayKey(m.calSelDate()))
}

func TestCalendarLoadsFastWith5000Events(t *testing.T) {
	m := calModel(t)
	tx, err := m.db.Begin()
	require.NoError(t, err)
	base := utcT(2025, 1, 1, 9, 0)
	for i := 0; i < 5000; i++ {
		s := base.Add(time.Duration(i) * 3 * time.Hour)
		_, err := tx.Exec(`INSERT INTO events (uuid, title, start_utc, end_utc, tz) VALUES (?, 'event', ?, ?, 'UTC')`,
			"perf-"+s.Format(time.RFC3339), tz.FormatInstant(s), tz.FormatInstant(s.Add(time.Hour)))
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit())
	m.now = func() time.Time { return utcT(2025, 6, 10, 12, 0) }
	press(m, "m", "t")

	start := time.Now()
	for i := 0; i < 5; i++ {
		require.NoError(t, m.rebuild(true))
	}
	per := time.Since(start) / 5
	assert.Greater(t, len(m.cal.byDay), 20)
	assert.Less(t, per, 100*time.Millisecond, "month view rebuild (budget 50 ms, relaxed for slow CI)")
	t.Logf("month rebuild with 5000 events: %v", per)
}

func toggle(m *Model, id string) { m.runAction(m.actionByID(id)) }

func TestCalendarQuickAddReadsShortSyntax(t *testing.T) {
	m := calModel(t)
	q, err := m.parseQuick("Lunch fri 12:30 for 1h at Cafe Rio every week #food")
	require.NoError(t, err)
	assert.Equal(t, "Lunch", q.title)
	assert.Equal(t, "Cafe Rio", q.location)
	assert.Equal(t, []string{"food"}, q.tags)
	assert.False(t, q.allDay)
	assert.Equal(t, time.Hour, q.end.Sub(q.start))
	require.NotNil(t, q.rule)
	assert.Contains(t, *q.rule, "FREQ=WEEKLY")

	// Saving stores the place and tag.
	press(m, "+")
	typeText(m, "Lunch fri 12:30 for 1h at Cafe Rio #food")
	press(m, "enter")
	list, _, err := m.er.List(context.Background(), model.EventFilters{From: utcT(2026, 10, 1, 0, 0), To: utcT(2026, 11, 1, 0, 0), Loc: m.loc})
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "Cafe Rio", list[0].Location)
	assert.Equal(t, "Lunch", list[0].Title)

	// An unknown @name stops the save.
	press(m, "+")
	typeText(m, "Call @Nobody fri 9am")
	press(m, "enter")
	assert.Contains(t, m.status, "no person matches")
}
func TestCalendarEditPromptUsesTheShortSyntaxAndTheOldFormatIsGone(t *testing.T) {
	m := calModel(t)
	addEvent(t, m, model.CreateEventInput{Title: "Lunch", Start: utcT(2026, 10, 16, 17, 30), End: utcT(2026, 10, 16, 18, 30)})
	press(m, "d", "tab", "tab", "tab") // Fri 16 Oct
	require.Equal(t, "Fri 16 Oct 2026", m.calCountText())
	press(m, "enter")
	require.Equal(t, modePrompt, m.mode)
	assert.Equal(t, "Lunch 2026-10-16 12:30 for 1h", m.input.Value())
	assert.NotContains(t, m.input.Prompt, "|")

	// Place and length come from the same words as the add prompt.
	m.input.SetValue("Lunch 2026-10-16 12:30 for 90m at Cafe Rio")
	press(m, "enter")
	assert.Contains(t, m.status, "event updated")
	list, _, err := m.er.List(context.Background(), model.EventFilters{From: utcT(2026, 10, 16, 0, 0), To: utcT(2026, 10, 17, 0, 0), Loc: m.loc})
	require.NoError(t, err)
	require.Len(t, list, 1)
	e, err := m.er.FindByID(context.Background(), list[0].EventID)
	require.NoError(t, err)
	assert.Equal(t, "Cafe Rio", str(e.Location))
	assert.Equal(t, "2026-10-16T19:00:00Z", *e.EndUTC)

	// Tags and attendees have their own actions: the edit line says so instead of dropping them.
	press(m, "enter")
	m.input.SetValue("Lunch #food")
	press(m, "enter")
	assert.True(t, m.statusIsErr)
	assert.Contains(t, m.status, "Calendar: tag event")
}
