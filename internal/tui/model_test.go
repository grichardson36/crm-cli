package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
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
	m.now = func() time.Time { return time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC) }
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m, pr
}

func press(m *Model, keys ...string) {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case "ctrl+d":
			msg = tea.KeyMsg{Type: tea.KeyCtrlD}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		m.Update(msg)
	}
}

func typeText(m *Model, s string) {
	for _, r := range s {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func names(m *Model) []string {
	var out []string
	for _, r := range m.rows {
		out = append(out, strings.TrimPrefix(r.title, "★ "))
	}
	return out
}

func TestInitialRender(t *testing.T) {
	m, _ := newTestModel(t)
	assert.Equal(t, []string{"Zed Adams", "Bob Baker", "Cara Cole"}, names(m), "sorted by last name")

	out := m.View()
	assert.Contains(t, out, "1 People")
	assert.Contains(t, out, "Zed Adams")
	assert.Contains(t, out, "zed@acme.com", "detail pane shows the first person")
	assert.Contains(t, out, "3 shown / 3 total")
}

func TestViewFitsTerminal(t *testing.T) {
	m, _ := newTestModel(t)
	for _, size := range [][2]int{{100, 30}, {60, 12}, {40, 10}, {120, 50}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, v := range []string{"1", "2", "3", "4"} {
			press(m, v)
			out := m.View()
			lines := strings.Split(out, "\n")
			assert.LessOrEqual(t, len(lines), size[1], "height at %v view %s", size, v)
			for _, l := range lines {
				assert.LessOrEqual(t, lipgloss.Width(l), size[0], "width at %v view %s: %q", size, v, l)
			}
		}
		press(m, "1")
	}
}

func TestNavigation(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "j")
	assert.Equal(t, 1, m.cursor)
	assert.Contains(t, m.View(), "bob@globex.com")
	assert.Contains(t, m.View(), "bob@home.org", "all emails appear in the detail pane")
	press(m, "G")
	assert.Equal(t, 2, m.cursor)
	press(m, "j")
	assert.Equal(t, 2, m.cursor, "stops at the end")
	press(m, "g")
	assert.Equal(t, 0, m.cursor)
	press(m, "k")
	assert.Equal(t, 0, m.cursor, "stops at the top")
}

func TestSearchAndTagFilter(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "/")
	assert.Equal(t, modeSearch, m.mode)
	typeText(m, "globex")
	assert.Equal(t, []string{"Bob Baker"}, names(m))
	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode)
	assert.Equal(t, []string{"Bob Baker"}, names(m), "filter stays after enter")
	press(m, "esc")
	assert.Len(t, m.rows, 3, "esc clears the filter")

	// Secondary email and tag searches.
	press(m, "/")
	typeText(m, "home.org")
	assert.Equal(t, []string{"Bob Baker"}, names(m))
	press(m, "esc")
	press(m, "/")
	typeText(m, "#friend")
	assert.Equal(t, []string{"Bob Baker"}, names(m))
	press(m, "esc")
	press(m, "/")
	typeText(m, "zzz")
	assert.Empty(t, m.rows)
	assert.Contains(t, m.View(), "No matches.")
	press(m, "esc")
	assert.Len(t, m.rows, 3)
}

func TestViewsDueBirthdaysTasks(t *testing.T) {
	m, _ := newTestModel(t)

	press(m, "2")
	assert.Equal(t, viewDue, m.view)
	assert.Equal(t, []string{"Bob Baker"}, names(m))
	assert.Contains(t, m.View(), "overdue")

	press(m, "3")
	assert.Equal(t, []string{"Bob Baker"}, names(m))
	assert.Equal(t, "in 7d", m.rows[0].meta)

	press(m, "4")
	assert.Empty(t, m.rows)
	assert.Contains(t, m.View(), "No open tasks")

	press(m, "tab")
	assert.Equal(t, viewPeople, m.view, "tab wraps around")
}

func TestMarkContactedRemovesFromDue(t *testing.T) {
	m, pr := newTestModel(t)
	press(m, "2")
	require.Len(t, m.rows, 1)
	press(m, "c")
	assert.Empty(t, m.rows, "contacted people are no longer due")
	assert.Equal(t, "marked as contacted", m.status)

	p, err := pr.FindByID(context.Background(), 2)
	require.NoError(t, err)
	require.NotNil(t, p.LastContactedAt)
	assert.Equal(t, "2026-10-13 12:00:00", *p.LastContactedAt)
}

func TestSnoozeFavoriteNoteAndTask(t *testing.T) {
	m, pr := newTestModel(t)
	ctx := context.Background()

	// Snooze Bob out of the due list for 7 days.
	press(m, "2", "s")
	assert.Equal(t, modePrompt, m.mode)
	press(m, "enter") // accepts the default of 7
	assert.Empty(t, m.rows)

	// Favorite Zed, add a note and a task.
	press(m, "1", "f")
	assert.Contains(t, m.rows[0].title, "★")
	press(m, "n")
	typeText(m, "met at the conference")
	press(m, "enter")
	assert.Equal(t, "note added", m.status)
	assert.Contains(t, m.View(), "met at the conference")

	press(m, "t")
	typeText(m, "send deck | 2026-12-01")
	press(m, "enter")
	assert.Equal(t, "task added", m.status)

	press(m, "4")
	require.Len(t, m.rows, 1)
	assert.Equal(t, "send deck", m.rows[0].title)
	assert.Equal(t, "2026-12-01", m.rows[0].meta)
	assert.Contains(t, m.View(), "send deck")
	press(m, "x")
	assert.Empty(t, m.rows)

	p, err := pr.FindByID(ctx, 2)
	require.NoError(t, err)
	assert.NotNil(t, p.KeepInTouchSnoozedUntil)

	// An empty prompt or esc changes nothing.
	press(m, "1", "n", "esc")
	assert.Equal(t, modeNormal, m.mode)
	press(m, "s")
	typeText(m, "abc")
	press(m, "enter")
	assert.True(t, m.statusIsErr)
}

func TestCompleteOutsideTasksViewIsRejected(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "x")
	assert.True(t, m.statusIsErr)
}

func TestMouse(t *testing.T) {
	m, _ := newTestModel(t)
	// Rows start on screen line 1, under the tab bar.
	m.Update(tea.MouseMsg{X: 3, Y: 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	assert.Equal(t, 2, m.cursor)
	m.Update(tea.MouseMsg{X: 3, Y: 3, Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	assert.Equal(t, 0, m.cursor)

	// Click the "2 Due" tab.
	x := len(tabLabel(0, viewNames[0])) + 2 + 2
	m.Update(tea.MouseMsg{X: x, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	assert.Equal(t, viewDue, m.view)
}

func TestHelpOverlay(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "?")
	assert.Contains(t, m.View(), "mark contacted")
	press(m, "x") // any key closes it and is swallowed
	assert.False(t, m.help)
	assert.NotContains(t, m.View(), "press any key")
}

func TestSortToggleAndReload(t *testing.T) {
	m, pr := newTestModel(t)
	_, err := pr.Update(context.Background(), 3, model.UpdatePersonInput{Title: sp("CEO")})
	require.NoError(t, err)
	press(m, "r")
	assert.Equal(t, "reloaded", m.status)

	// updated_at only has one-second resolution, so set it directly to get a stable order.
	for i := range m.people {
		if m.people[i].Name == "Cara Cole" {
			m.people[i].UpdatedAt = "2099-01-01 00:00:00"
		}
	}
	press(m, "o")
	assert.True(t, m.sortRecent)
	assert.Equal(t, "Cara Cole", names(m)[0], "most recently updated first")
	press(m, "o")
	assert.Equal(t, "Zed Adams", names(m)[0])
}

func TestReadOnlyBlocksWrites(t *testing.T) {
	m, pr := newTestModel(t)
	m.readOnly = true
	for _, k := range []string{"c", "f", "n", "t", "s", "x"} {
		press(m, k)
		assert.Equal(t, modeNormal, m.mode, "key %s must not open a prompt", k)
		assert.True(t, m.statusIsErr, "key %s", k)
	}
	assert.Contains(t, m.View(), "READ-ONLY")
	p, err := pr.FindByID(context.Background(), 1)
	require.NoError(t, err)
	assert.False(t, p.IsFavorite)
	assert.Nil(t, p.LastContactedAt)
	// Reading still works.
	press(m, "j", "/")
	assert.Equal(t, modeSearch, m.mode)
}
