package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/modules"
	crmsync "github.com/jdanielnd/crm-cli/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sp(s string) *string { return &s }

// newTestModel builds a model over an in-memory database with a few people.
func newTestModel(t *testing.T) (*Model, *repo.PersonRepo) {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	pr := repo.NewPersonRepo(d)
	ctx := context.Background()

	add := func(first, last, company, email string) *model.Person {
		p, err := pr.Create(ctx, model.CreatePersonInput{FirstName: first, LastName: sp(last), Company: sp(company), Email: sp(email)})
		require.NoError(t, err)
		return p
	}
	add("Zed", "Adams", "Acme", "zed@acme.com")
	bob := add("Bob", "Baker", "Globex", "bob@globex.com")
	add("Cara", "Cole", "Initech", "cara@initech.com")

	require.NoError(t, pr.AddEmail(ctx, bob.ID, "bob@home.org", nil, false))
	_, err = pr.Update(ctx, bob.ID, model.UpdatePersonInput{KeepInTouch: sp("7 days"), LastContactedAt: sp("2020-01-01"), Birthday: sp("--10-20")})
	require.NoError(t, err)
	tag := repo.NewTagRepo(d)
	require.NoError(t, tag.Apply(ctx, "person", bob.ID, "friend"))

	m, err := New(ctx, d)
	require.NoError(t, err)
	// Most tests reach a view by its number key, so they keep the order the number keys had before the Tags
	// module: Tags is last and off here (tests of the Tags view switch to it). The default order has its own tests (modules_test.go).
	m.mods = modules.Arrangement{Order: []string{"home", "people", "follow-up", "tasks", "organizations", "deals", "birthdays", "merge", "archived", "sync", "tags"}, Hidden: []string{"tags"}}.Normalize()
	m.patchModuleActions()
	m.now = func() time.Time { return time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC) }
	m.clip = func(string) {}
	m.open = func(string) error { return nil }
	m.registry = crmsync.NewRegistry() // no adapters unless a test installs the fake
	m.runner = crmsync.NewRunner(d, m.registry)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.switchView(viewPeople) // most tests start on the People list; Home has its own tests
	return m, pr
}

func press(m *Model, keys ...string) {
	for _, k := range keys {
		m.Update(keyPress(k))
	}
}

func typeText(m *Model, s string) {
	for _, r := range s {
		m.Update(keyPress(string(r)))
	}
}

func names(m *Model) []string {
	var out []string
	for _, r := range m.rows {
		out = append(out, strings.TrimPrefix(r.title, "★ "))
	}
	return out
}

// goTo moves the detail cursor down to the first row with the given label.
func goTo(t *testing.T, m *Model, label string) {
	t.Helper()
	for i := 0; i < 80 && m.form[m.fcur].label != label; i++ {
		press(m, "down")
	}
	require.Equal(t, label, m.form[m.fcur].label)
}

// replaceText clears the inline input and types s.
func replaceText(m *Model, s string) {
	press(m, "ctrl+u")
	typeText(m, s)
}

func personByID(t *testing.T, pr *repo.PersonRepo, id int64) *model.Person {
	t.Helper()
	p, err := pr.FindByID(context.Background(), id)
	require.NoError(t, err)
	require.NoError(t, pr.LoadDetails(context.Background(), p))
	return p
}

func TestInitialRender(t *testing.T) {
	m, _ := newTestModel(t)
	assert.Equal(t, []string{"Zed Adams", "Bob Baker", "Cara Cole"}, names(m), "sorted by last name")

	out := m.screen()
	assert.Contains(t, out, "2 People")
	assert.Contains(t, out, "Zed Adams")
	assert.Contains(t, out, "zed@acme.com", "detail pane shows the first person")
	assert.Contains(t, out, "3 shown / 3 total")
}

func TestViewFitsTerminal(t *testing.T) {
	m, _ := newTestModel(t)
	check := func(size [2]int, what string) {
		out := m.screen()
		lines := strings.Split(out, "\n")
		assert.LessOrEqual(t, len(lines), size[1], "height at %v %s", size, what)
		for _, l := range lines {
			assert.LessOrEqual(t, lipgloss.Width(l), size[0], "width at %v %s: %q", size, what, l)
		}
	}
	for _, size := range [][2]int{{100, 30}, {60, 12}, {40, 10}, {120, 50}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, v := range []string{"1", "2", "3", "4", "5", "6", "7", "8"} {
			press(m, v)
			check(size, "view "+v)
		}
		press(m, "2", "tab")
		check(size, "detail focus")
		press(m, "a")
		check(size, "inline add")
		press(m, "esc", "esc", "ctrl+k")
		check(size, "palette")
		typeText(m, "zz")
		check(size, "palette no match")
		press(m, "esc", "?")
		check(size, "help")
		press(m, "x")
		press(m, "+")
		check(size, "create")
		press(m, "esc")
	}
}

func TestNavigation(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "down")
	assert.Equal(t, 1, m.cursor)
	assert.Contains(t, m.screen(), "bob@globex.com")
	assert.Contains(t, m.screen(), "bob@home.org", "all emails appear in the detail pane")
	press(m, "end")
	assert.Equal(t, 2, m.cursor)
	press(m, "down")
	assert.Equal(t, 2, m.cursor, "stops at the end")
	press(m, "home")
	assert.Equal(t, 0, m.cursor)
	press(m, "up")
	assert.Equal(t, 0, m.cursor, "stops at the top")
}

// barNames lists the labels of the bar rows of one kind.
func barNames(m *Model, kind string) []string {
	var out []string
	for _, i := range m.ov.shown {
		if it := m.ov.items[i]; it.kind == kind {
			out = append(out, it.label)
		}
	}
	return out
}

func TestSlashOpensTheBarAndFindsPeople(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "/")
	require.Equal(t, modeOverlay, m.mode)
	require.True(t, m.ov.palette, "/ opens the same bar as ctrl+k")
	typeText(m, "globex")
	assert.Equal(t, []string{"Bob Baker"}, barNames(m, kindPerson))
	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode)
	assert.Equal(t, int64(2), m.curPerson(), "enter opens the person")
	assert.Len(t, m.rows, 3, "no filter is left behind")

	// A secondary email is found too.
	press(m, "/")
	typeText(m, "home.org")
	assert.Equal(t, []string{"Bob Baker"}, barNames(m, kindPerson))
	press(m, "esc")

	// #tag filters the list, esc clears the filter.
	press(m, "/")
	typeText(m, "#friend")
	press(m, "enter")
	assert.Equal(t, []string{"Bob Baker"}, names(m))
	assert.Contains(t, m.screen(), "filter: #friend")
	press(m, "esc")
	assert.Len(t, m.rows, 3, "esc clears the filter")

	// Nothing found offers to create.
	press(m, "/")
	typeText(m, "Qwerty Zxcvb")
	assert.Empty(t, barNames(m, kindPerson))
	assert.Equal(t, []string{`Create person "Qwerty Zxcvb"`, `Add note "Qwerty Zxcvb"`}, barNames(m, kindCreate))
	press(m, "esc")
	assert.Len(t, m.rows, 3)
}

func TestViewsDueBirthdaysTasks(t *testing.T) {
	m, _ := newTestModel(t)

	press(m, "3")
	assert.Equal(t, viewDue, m.view)
	assert.Contains(t, m.screen(), "Follow-up")
	assert.Equal(t, []string{"Bob Baker"}, names(m))
	assert.Contains(t, m.screen(), "overdue")

	press(m, "7")
	var birthdays []row
	var days []row
	for _, r := range m.rows {
		if r.rule {
			days = append(days, r)
		} else if !r.skip {
			birthdays = append(birthdays, r)
		}
	}
	require.Len(t, birthdays, 1)
	assert.Equal(t, "Bob Baker", birthdays[0].title)
	assert.Equal(t, "", birthdays[0].meta, "no birth year, no age")
	require.Len(t, days, 1)
	assert.Equal(t, "2026-10-20", days[0].title)
	assert.Equal(t, "in 1w", days[0].meta)

	press(m, "4")
	assert.Empty(t, m.rows)
	assert.Contains(t, m.screen(), "No open tasks")

	press(m, "]")
	assert.Equal(t, viewOrgs, m.view, "Organizations follows Tasks")
	press(m, "]")
	assert.Equal(t, viewDeals, m.view, "Deals follows Organizations")
	press(m, "]")
	assert.Equal(t, viewBirthdays, m.view)
	press(m, "]", "]", "]")
	assert.Equal(t, viewSync, m.view)
	press(m, "]")
	assert.Equal(t, viewHome, m.view, "] wraps around")
	press(m, "[")
	assert.Equal(t, viewSync, m.view, "[ wraps back")
}

func TestMarkContactedRemovesFromDue(t *testing.T) {
	m, pr := newTestModel(t)
	m.now = time.Now // the due query uses the database clock, so the app clock must be the real one
	press(m, "3")
	require.Len(t, m.rows, 1)
	press(m, "x")
	assert.Empty(t, m.rows, "contacted people are no longer due")
	assert.Equal(t, "marked as contacted", m.status)

	p, err := pr.FindByID(context.Background(), 2)
	require.NoError(t, err)
	require.NotNil(t, p.LastContactedAt)
	assert.Equal(t, time.Now().UTC().Format("2006-01-02"), (*p.LastContactedAt)[:10])
}

func TestSnoozeFavoriteNoteAndTask(t *testing.T) {
	m, pr := newTestModel(t)
	m.now = time.Now // the due query uses the database clock, so the app clock must be the real one
	ctx := context.Background()

	// Snooze Bob out of the due list for 7 days.
	press(m, "3", "s")
	assert.Equal(t, modePrompt, m.mode)
	press(m, "enter") // accepts the default of 7
	assert.Empty(t, m.rows)

	// Favorite Zed, add a note and a task.
	press(m, "2", "f")
	assert.Contains(t, m.rows[0].title, "★")
	press(m, "a", "n")
	typeLinked(m, "met at the conference")
	press(m, "enter")
	assert.Equal(t, "note added", m.status)
	assert.Contains(t, m.screen(), "met at the conference")

	press(m, "a", "t")
	typeLinked(m, "send deck | 2026-12-01")
	press(m, "enter")
	assert.Equal(t, "task added", m.status)

	press(m, "4")
	require.Len(t, m.rows, 1)
	assert.Equal(t, "send deck", m.rows[0].title)
	assert.Equal(t, "2026-12-01", m.rows[0].meta)
	assert.Contains(t, m.screen(), "send deck")
	press(m, "x")
	assert.Empty(t, m.rows)

	p, err := pr.FindByID(ctx, 2)
	require.NoError(t, err)
	assert.NotNil(t, p.KeepInTouchSnoozedUntil)

	// An empty prompt or esc changes nothing.
	press(m, "2", "a", "n", "esc")
	assert.Equal(t, modeNormal, m.mode)
	press(m, "s")
	typeText(m, "abc")
	press(m, "enter")
	assert.True(t, m.statusIsErr)
}

func TestCompleteOutsideTasksViewIsRejected(t *testing.T) {
	m, _ := newTestModel(t)
	runID(m, "done") // x marks a person contacted there, Complete task is a palette entry
	assert.True(t, m.statusIsErr)
}

func TestMouse(t *testing.T) {
	m, _ := newTestModel(t)
	// Rows start on screen line 1, under the tab bar.
	m.Update(tea.MouseClickMsg{X: 3, Y: 3, Button: tea.MouseLeft})
	assert.Equal(t, 2, m.cursor)
	m.Update(tea.MouseWheelMsg{X: 3, Y: 3, Button: tea.MouseWheelUp})
	assert.Equal(t, 0, m.cursor)

	// Click the "3 Due" tab.
	x := m.tabLayout()[2].x0 + 1
	m.Update(tea.MouseClickMsg{X: x, Y: 0, Button: tea.MouseLeft})
	assert.Equal(t, viewDue, m.view)
}

func click(m *Model, x, y int) {
	m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
}

func TestMouseSelectsDetailRowAndSecondClickEdits(t *testing.T) {
	m, _ := newTestModel(t)
	idx := -1
	for i, r := range m.form {
		if r.label == "company" {
			idx = i
		}
	}
	require.GreaterOrEqual(t, idx, 0)
	x, y := m.listWidth()+3, 1+m.rowStart[idx]-m.detail.YOffset()
	click(m, x, y)
	assert.Equal(t, focusDetail, m.focus)
	assert.Equal(t, idx, m.fcur)
	assert.Equal(t, modeNormal, m.mode)
	click(m, x, y)
	assert.Equal(t, modeEdit, m.mode, "second click on the selected row edits it")
	press(m, "esc")

	// Clicking the list moves focus back.
	click(m, 3, 2)
	assert.Equal(t, focusList, m.focus)
	assert.Equal(t, 1, m.cursor)
}

func TestFooterButtonsAndStatusTap(t *testing.T) {
	m, _ := newTestModel(t)
	var help *footSeg
	for _, s := range m.footerSegs() {
		if s.id == "help" {
			s := s
			help = &s
		}
	}
	require.NotNil(t, help, "help button is in the footer")
	click(m, help.x0+1, m.height-1)
	assert.True(t, m.help)
	press(m, "x")
	assert.False(t, m.help)

	click(m, 5, m.height-2)
	assert.Equal(t, modeOverlay, m.mode, "tapping the status line opens the palette")
	press(m, "esc")
	assert.Equal(t, modeNormal, m.mode)

	// A footer button runs its action: "a add" opens the add picker.
	for _, s := range m.footerSegs() {
		if s.id == "add-any" {
			click(m, s.x0, m.height-1)
		}
	}
	assert.Equal(t, modeOverlay, m.mode)
	assert.Equal(t, "Add", m.ov.title)
}

func TestHelpOverlay(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "?")
	assert.Contains(t, m.screen(), "Keys in People")
	assert.Contains(t, m.screen(), "Framework")
	press(m, "x") // any other key closes it and is swallowed
	assert.False(t, m.help)
	assert.NotContains(t, m.screen(), "press any key")

	// The help text lists the keys that work in this view and mode, framework first, then the app.
	text := m.helpText()
	assert.Less(t, strings.Index(text, "Framework"), strings.Index(text, "App: specific"))
	chain := m.scopeChain()
	for _, a := range m.actions {
		if a.barOnly || a.mode != keyNormal || len(a.keys) == 0 {
			continue
		}
		if inScopes(&a, chain) {
			assert.Contains(t, text, a.desc, "help lists %s", a.id)
		} else {
			assert.NotContains(t, text, a.desc, "help leaves out %s: its scope %s is not live in People", a.id, a.scopeOf())
		}
	}
}

func TestSortToggleAndReload(t *testing.T) {
	m, pr := newTestModel(t)
	_, err := pr.Update(context.Background(), 3, model.UpdatePersonInput{Title: sp("CEO")})
	require.NoError(t, err)
	press(m, "ctrl+r")
	assert.Equal(t, "reloaded", m.status)

	// updated_at only has one-second resolution, so set it directly to get a stable order.
	for i := range m.people {
		if m.people[i].Name == "Cara Cole" {
			m.people[i].UpdatedAt = "2099-01-01 00:00:00"
		}
	}
	press(m, "ctrl+o")
	require.Equal(t, modeOverlay, m.mode, "ctrl+o opens the view options")
	press(m, "right")
	press(m, "esc")
	assert.True(t, m.sortedRecent(viewPeople))
	assert.Equal(t, "Cara Cole", names(m)[0], "most recently updated first")
	press(m, "ctrl+o", "right", "esc")
	assert.Equal(t, "Zed Adams", names(m)[0])
}

func TestReadOnlyBlocksWrites(t *testing.T) {
	m, pr := newTestModel(t)
	m.readOnly = true
	for _, k := range []string{"x", "f", "a", "s", "d"} {
		press(m, k)
		assert.Equal(t, modeNormal, m.mode, "key %s must not open a prompt", k)
		assert.True(t, m.statusIsErr, "key %s", k)
		m.setStatus("", false)
	}
	assert.Contains(t, m.screen(), "READ-ONLY")
	p, err := pr.FindByID(context.Background(), 1)
	require.NoError(t, err)
	assert.False(t, p.IsFavorite)
	assert.Nil(t, p.LastContactedAt)
	// Reading still works.
	press(m, "/")
	assert.Equal(t, modeOverlay, m.mode)
	press(m, "esc", "down")

	// The detail pane is read-only too: enter, a and d are blocked, copy still works.
	press(m, "tab")
	assert.Equal(t, focusDetail, m.focus)
	goTo(t, m, "email")
	for _, k := range []string{"enter", "a", "d", "o"} {
		press(m, k)
		assert.Equal(t, modeNormal, m.mode, "key %s", k)
		assert.True(t, m.statusIsErr, "key %s", k)
		m.setStatus("", false)
	}
	var copied string
	m.clip = func(s string) { copied = s }
	press(m, "ctrl+c")
	assert.Equal(t, "bob@globex.com", copied)
	assert.False(t, m.statusIsErr)
	assert.Len(t, personByID(t, pr, 2).Emails, 2)
}

func TestFocusSwitch(t *testing.T) {
	m, _ := newTestModel(t)
	assert.Equal(t, focusList, m.focus)
	for _, into := range []string{"tab", "right", "shift+tab"} {
		for _, back := range []string{"esc", "left", "tab", "shift+tab"} {
			press(m, into)
			assert.Equal(t, focusDetail, m.focus, into)
			press(m, back)
			assert.Equal(t, focusList, m.focus, "%s then %s", into, back)
		}
	}
	// up and down move the form cursor, not the list, while the detail has focus.
	press(m, "tab")
	cur := m.cursor
	press(m, "down")
	assert.Equal(t, cur, m.cursor)
	assert.Equal(t, 1, m.fcur)
	press(m, "up")
	assert.Equal(t, 0, m.fcur)
	press(m, "end")
	assert.Equal(t, len(m.form)-1, m.fcur)
	press(m, "esc")
	press(m, "down")
	assert.Equal(t, cur+1, m.cursor, "list moves again after leaving the pane")
}

func TestInlineEditCommitAndCancel(t *testing.T) {
	m, pr := newTestModel(t)
	press(m, "tab")
	goTo(t, m, "company")
	press(m, "enter")
	require.Equal(t, modeEdit, m.mode)
	assert.Contains(t, m.screen(), "Acme", "inline box starts with the current value")

	// Esc cancels and writes nothing.
	replaceText(m, "Nope")
	press(m, "esc")
	assert.Equal(t, modeNormal, m.mode)
	assert.Equal(t, "Acme", *personByID(t, pr, 1).Company)

	// Enter saves, the list reloads and the cursor stays on the row.
	press(m, "enter")
	replaceText(m, "Acme Corp")
	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode)
	assert.Equal(t, "Acme Corp", *personByID(t, pr, 1).Company)
	assert.Equal(t, "company", m.form[m.fcur].label)
	assert.Contains(t, m.screen(), "Acme Corp")

	// Birthday accepts MM-DD and a bad value shows an error and keeps the box open.
	goTo(t, m, "birthday")
	press(m, "enter")
	replaceText(m, "13-45")
	press(m, "enter")
	assert.Equal(t, modeEdit, m.mode)
	assert.True(t, m.statusIsErr)
	replaceText(m, "03-09")
	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode)
	assert.Equal(t, "03-09", formatBirthdayParts(personByID(t, pr, 1)))

	// First name cannot be emptied.
	press(m, "home", "enter")
	replaceText(m, "")
	press(m, "enter")
	assert.Equal(t, modeEdit, m.mode)
	assert.True(t, m.statusIsErr)
	press(m, "esc")
	assert.Equal(t, "Zed", personByID(t, pr, 1).FirstName)
}

func TestAddAndDeleteEmail(t *testing.T) {
	m, pr := newTestModel(t)
	press(m, "down", "tab") // Bob, two emails
	goTo(t, m, "email")
	press(m, "a")
	require.Equal(t, modeEdit, m.mode)

	// An invalid email shows an error and keeps the box open.
	typeText(m, "not-an-email")
	press(m, "enter")
	assert.Equal(t, modeEdit, m.mode)
	assert.True(t, m.statusIsErr)

	replaceText(m, "bob@work.io")
	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode)
	emails := personByID(t, pr, 2).Emails
	require.Len(t, emails, 3)
	assert.Equal(t, "bob@work.io", m.form[m.fcur].value, "cursor lands on the new row")

	// Edit it in place (replaces the address).
	press(m, "enter")
	replaceText(m, "bob@work.org")
	press(m, "enter")
	emails = personByID(t, pr, 2).Emails
	require.Len(t, emails, 3)
	assert.Equal(t, "bob@work.org", emails[2].Email)

	// d asks first, n keeps it, y deletes it.
	press(m, "d")
	assert.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.screen(), "delete email bob@work.org")
	press(m, "n")
	assert.Len(t, personByID(t, pr, 2).Emails, 3)
	press(m, "d", "y")
	assert.Equal(t, modeNormal, m.mode)
	assert.Len(t, personByID(t, pr, 2).Emails, 2)

	// Esc on a pending add removes the empty row.
	press(m, "a", "esc")
	assert.Equal(t, modeNormal, m.mode)
	assert.Len(t, personByID(t, pr, 2).Emails, 2)
	for _, r := range m.form {
		if r.kind == rkEmail {
			assert.NotEmpty(t, r.value)
		}
	}
}

func TestAddPhoneHandleTagTaskNote(t *testing.T) {
	m, pr := newTestModel(t)
	press(m, "tab")

	goTo(t, m, "phone")
	press(m, "enter") // placeholder row: enter adds
	typeText(m, "+1 555 0100")
	press(m, "enter")
	require.Len(t, personByID(t, pr, 1).Phones, 1)

	goTo(t, m, "link")
	press(m, "a")
	typeText(m, "https://www.linkedin.com/in/zed")
	press(m, "enter")
	h := personByID(t, pr, 1).Handles
	require.Len(t, h, 1)
	assert.Equal(t, "linkedin", h[0].Kind, "kind is inferred from the URL")

	// Tab in the box changes the kind of a handle.
	press(m, "enter", "tab")
	assert.Contains(t, m.screen(), "twitter")
	press(m, "enter")
	h = personByID(t, pr, 1).Handles
	require.Len(t, h, 1)
	assert.Equal(t, "twitter", h[0].Kind)

	goTo(t, m, "tag")
	press(m, "a")
	typeText(m, "#vip")
	press(m, "enter")
	tags, err := m.tagr.GetForEntity(context.Background(), "person", 1)
	require.NoError(t, err)
	require.Len(t, tags, 1)
	assert.Equal(t, "vip", tags[0].Name)

	goTo(t, m, "task")
	press(m, "a")
	typeText(m, "send deck | 2026-12-01")
	press(m, "enter")
	tasks, err := m.tr.FindAll(context.Background(), model.TaskFilters{PersonID: ptr64(1)})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "send deck", tasks[0].Title)
	press(m, "d", "y")
	tasks, _ = m.tr.FindAll(context.Background(), model.TaskFilters{PersonID: ptr64(1)})
	assert.Empty(t, tasks)

	// Notes: a opens the note prompt.
	goTo(t, m, "note")
	press(m, "a")
	assert.Equal(t, modePrompt, m.mode)
	typeText(m, "hello there")
	press(m, "enter")
	assert.Contains(t, m.screen(), "hello there")
	for i, r := range m.form {
		if r.kind == rkNote && !r.empty {
			m.fcur = i
		}
	}
	press(m, "enter")
	assert.Equal(t, modeEdit, m.mode, "enter edits a note")
	press(m, "esc")
	press(m, "d")
	assert.Equal(t, modeConfirm, m.mode, "d asks before deleting a note")
	press(m, "n")
	var copied string
	m.clip = func(s string) { copied = s }
	press(m, "ctrl+c") // copies the focused field in the detail pane
	assert.Equal(t, "hello there", copied)
	assert.NotNil(t, m, "ctrl+c in the detail pane did not quit")
}

func ptr64(i int64) *int64 { return &i }

func TestCopyAndOpen(t *testing.T) {
	m, _ := newTestModel(t)
	var copied, opened string
	m.clip = func(s string) { copied = s }
	m.open = func(u string) error { opened = u; return nil }
	press(m, "down", "tab")
	goTo(t, m, "email")
	press(m, "ctrl+c")
	assert.Equal(t, "bob@globex.com", copied)
	press(m, "o")
	assert.Equal(t, "mailto:bob@globex.com", opened)
	press(m, "home")
	goTo(t, m, "company")
	opened = ""
	press(m, "o")
	assert.Equal(t, "https://www.google.com/search?q=Globex", opened, "text that is no link opens a web search for it")
	assert.False(t, m.statusIsErr)

	var sb strings.Builder
	osc52Copy(&sb, "hi")
	assert.Equal(t, "\x1b]52;c;aGk=\x07", sb.String())
}

func TestCreateContact(t *testing.T) {
	m, pr := newTestModel(t)

	// Esc abandons it, nothing is inserted.
	press(m, "a", "enter")
	assert.Equal(t, modeEdit, m.mode)
	assert.Equal(t, focusDetail, m.focus)
	assert.Contains(t, m.screen(), "New contact")
	press(m, "esc")
	assert.Equal(t, modeNormal, m.mode)
	assert.False(t, m.creating)
	all, err := pr.FindAll(context.Background(), model.PersonFilters{})
	require.NoError(t, err)
	assert.Len(t, all, 3)

	// An empty name is rejected and the box stays open.
	press(m, "a", "enter", "enter")
	assert.Equal(t, modeEdit, m.mode)
	assert.True(t, m.statusIsErr)

	// A name inserts the person on the first save.
	typeText(m, "Dana Mary Dunn")
	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode)
	assert.False(t, m.creating)
	all, err = pr.FindAll(context.Background(), model.PersonFilters{})
	require.NoError(t, err)
	require.Len(t, all, 4)
	var dana *model.Person
	for _, p := range all {
		if p.FirstName == "Dana Mary" {
			dana = p
		}
	}
	require.NotNil(t, dana)
	assert.Equal(t, "Dunn", *dana.LastName)
	assert.Equal(t, "Dana Mary Dunn", names(m)[3], "the list shows the new person")
	assert.Equal(t, dana.ID, m.rows[m.cursor].personID)
	assert.Equal(t, focusDetail, m.focus)
	assert.Equal(t, "title", m.form[m.fcur].field, "cursor moves on to fill in the rest")

	// Continue filling in: add an email on the new person.
	goTo(t, m, "email")
	press(m, "a")
	typeText(m, "dana@dunn.io")
	press(m, "enter")
	require.Len(t, personByID(t, pr, dana.ID).Emails, 1)
}

func TestCreateContactFromOtherViewAndPalette(t *testing.T) {
	m, pr := newTestModel(t)
	press(m, "3", "ctrl+k")
	typeText(m, "new contact")
	press(m, "enter")
	require.Equal(t, modeEdit, m.mode)
	typeText(m, "Eve")
	press(m, "enter")
	assert.Equal(t, viewPeople, m.view)
	all, err := pr.FindAll(context.Background(), model.PersonFilters{})
	require.NoError(t, err)
	assert.Len(t, all, 4)
}

func TestMarksAndBulkActions(t *testing.T) {
	m, pr := newTestModel(t)
	ctx := context.Background()

	press(m, " ", " ") // marks Zed and Bob, cursor moves down each time
	assert.Equal(t, 2, m.cursor)
	assert.Len(t, m.markedIDs(), 2)
	assert.Contains(t, m.screen(), "2 marked")
	assert.Contains(t, m.screen(), "✓ Zed Adams")

	// Tag all marked (the palette's Tag), marks clear afterwards.
	runID(m, "tag")
	assert.Equal(t, modePrompt, m.mode)
	typeText(m, "#vip")
	press(m, "enter")
	assert.Empty(t, m.marked)
	assert.NotContains(t, m.screen(), "marked")
	for _, id := range []int64{1, 2} {
		tags, err := m.tagr.GetForEntity(ctx, "person", id)
		require.NoError(t, err)
		var found bool
		for _, tg := range tags {
			found = found || tg.Name == "vip"
		}
		assert.True(t, found, "person %d tagged", id)
	}
	tags, _ := m.tagr.GetForEntity(ctx, "person", 3)
	assert.Empty(t, tags)

	// ctrl+a selects all, a second ctrl+a clears them.
	press(m, "ctrl+a")
	assert.Len(t, m.markedIDs(), 3)
	press(m, "ctrl+a")
	assert.Empty(t, m.markedIDs())

	// Cadence (palette) and priority through pickers.
	press(m, "ctrl+a")
	runID(m, "cadence")
	assert.Equal(t, modeOverlay, m.mode)
	typeText(m, "6 mons")
	press(m, "enter")
	assert.Empty(t, m.marked)
	for id := int64(1); id <= 3; id++ {
		assert.Equal(t, "6 mons", str(personByID(t, pr, id).KeepInTouch))
	}
	press(m, "ctrl+a")
	// Priority is a choice field of the person page now (no palette entry), edited in place.
	press(m, "tab")
	setChoice(t, m, "priority", "High")
	press(m, "esc")
	cur := m.curPerson()
	require.NotNil(t, personByID(t, pr, cur).PriorityTier)
	assert.Equal(t, 2, *personByID(t, pr, cur).PriorityTier)
	m.marked = map[int64]bool{}

	// Favorite on a selection toggles all of them; marks only cover the marked people.
	press(m, "home", " ", " ", "f")
	assert.True(t, personByID(t, pr, 1).IsFavorite)
	assert.True(t, personByID(t, pr, 2).IsFavorite)
	assert.False(t, personByID(t, pr, 3).IsFavorite)
	press(m, "ctrl+a", "f") // not all favorite: everyone becomes favorite
	assert.True(t, personByID(t, pr, 3).IsFavorite)
	press(m, "ctrl+a", "f") // all favorite: everyone is cleared
	assert.False(t, personByID(t, pr, 1).IsFavorite)

	// Marks clear when the view changes.
	press(m, " ")
	assert.Len(t, m.marked, 1)
	press(m, "3")
	assert.Empty(t, m.marked)
}

func TestPaletteFilterAndRun(t *testing.T) {
	m, pr := newTestModel(t)
	press(m, "ctrl+k")
	require.Equal(t, modeOverlay, m.mode)
	on := 0
	for i := range m.actions {
		if m.moduleOn(&m.actions[i]) && !m.actions[i].barOnly && m.actions[i].mode != keyText {
			on++
		}
	}
	assert.Len(t, m.ov.items, on, "palette lists every action of a module that is on")
	out := m.screen()
	assert.Contains(t, out, "Command bar")
	assert.Contains(t, out, "Toggle favorite")

	typeText(m, "toggle favor")
	require.NotEmpty(t, m.ov.shown)
	assert.Equal(t, "favorite", m.ov.items[m.ov.shown[0]].id)
	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode)
	assert.True(t, personByID(t, pr, 1).IsFavorite)

	// "/" opens the bar again, esc closes without running anything. ":" is no longer a key.
	press(m, ":")
	assert.Equal(t, modeNormal, m.mode)
	press(m, "/")
	assert.Equal(t, modeOverlay, m.mode)
	typeText(m, "mark contacted")
	press(m, "esc")
	assert.Equal(t, modeNormal, m.mode)
	assert.Nil(t, personByID(t, pr, 1).LastContactedAt)

	// Out-of-context actions are shown greyed out and do not run.
	press(m, "ctrl+k")
	typeText(m, "edit row")
	for i, idx := range m.ov.shown {
		if m.ov.items[idx].id == "edit" {
			assert.Equal(t, "not here", m.ov.items[idx].disabled)
			assert.Greater(t, i, 0, "disabled actions sort after enabled ones")
			m.ov.cursor = i
		}
	}
	press(m, "enter")
	assert.True(t, m.statusIsErr)

	// Up/down choose, and running a picker action from the palette works.
	press(m, "ctrl+k")
	typeText(m, "set keep")
	press(m, "down", "up", "enter")
	assert.Equal(t, modeOverlay, m.mode, "set keep-in-touch opens its own picker")
	press(m, "esc")
	assert.Equal(t, modeNormal, m.mode)
}

func TestPaletteReadOnlyIsGreyedOut(t *testing.T) {
	m, pr := newTestModel(t)
	m.readOnly = true
	press(m, "ctrl+k")
	typeText(m, "favorite")
	var it overlayItem
	for pos, idx := range m.ov.shown { // a disabled action sorts after enabled ones that merely look like the text
		if m.ov.items[idx].id == "favorite" {
			it, m.ov.cursor = m.ov.items[idx], pos
		}
	}
	assert.Equal(t, "favorite", it.id)
	assert.Equal(t, "read-only", it.disabled)
	assert.Contains(t, m.screen(), "(read-only)")
	press(m, "enter")
	assert.True(t, m.statusIsErr)
	assert.False(t, personByID(t, pr, 1).IsFavorite)

	// Read actions still run from the palette.
	press(m, "ctrl+k")
	typeText(m, "due")
	press(m, "enter")
	assert.Equal(t, viewDue, m.view)
}

func TestRegistryHasUniqueIDsAndEveryActionIsInThePalette(t *testing.T) {
	m, _ := newTestModel(t)
	seen := map[string]bool{}
	for _, a := range m.actions {
		assert.False(t, seen[a.id], "duplicate id %s", a.id)
		if !m.moduleOn(&a) || a.barOnly || a.mode == keyText {
			continue // an off module and the verbs of a tag row are not in the palette
		}
		seen[a.id] = true
		assert.NotEmpty(t, a.title, a.id)
		assert.NotNil(t, a.run, a.id)
	}
	press(m, "/")
	ids := map[string]bool{}
	for _, it := range m.ov.items {
		ids[it.id] = true
	}
	assert.Equal(t, seen, ids)
}

func TestCleanCell(t *testing.T) {
	assert.Equal(t, "Ann Lee", cleanCell("Ann 🌟 Lee"))
	assert.Equal(t, "Acme", cleanCell("Acme 🚀"))
	assert.Equal(t, "Café ★", cleanCell("Café ★"))
}

func TestCleanCellStripsEmoji(t *testing.T) {
	assert.Equal(t, "Jane Smith", cleanCell("Jane ❤️ Smith"))
	assert.Equal(t, "Ann", cleanCell("Ann 👩‍💻"))
	assert.Equal(t, "Zoë Müller", cleanCell("Zoë Müller"))
	assert.Equal(t, "田中 太郎", cleanCell("田中 太郎"))
}
