package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Today is Tuesday 2026-10-13 07:00 in Chicago.
func bookModel(t *testing.T) *Model {
	t.Helper()
	m := calModel(t)
	_, err := repo.NewAvailabilityRepo(m.db).SetDays(context.Background(), "", "America/Chicago", model.WeekdayKeys[:5], []model.TimeRange{{Start: "09:00", End: "17:00"}})
	require.NoError(t, err)
	return m
}

func runPrompt(t *testing.T, m *Model, id, value string) {
	t.Helper()
	m.runAction(m.actionByID(id))
	require.Equal(t, modePrompt, m.mode, id)
	m.input.SetValue(value)
	press(m, "enter")
}

func TestBookingActionsAreInThePalette(t *testing.T) {
	m := bookModel(t)
	want := []string{"book-links", "book-add", "book-offer", "book-export", "book-confirm", "book-cancel", "book-pause", "book-resume", "book-rm"}
	for _, id := range want {
		a := m.actionByID(id)
		require.NotNil(t, a, id)
		assert.NotEmpty(t, a.desc)
		assert.Empty(t, a.keys, "booking has no hard coded keys: %s", id)
	}
	for _, a := range m.actions {
		if strings.HasPrefix(a.id, "book-") {
			assert.Contains(t, want, a.id, "list the new action in this test")
		}
	}
}

func TestBookingFlowFromThePalette(t *testing.T) {
	m := bookModel(t)
	runPrompt(t, m, "book-add", "Intro call | 30m | me@example.com")
	assert.Contains(t, m.status, "link intro-call created")
	runPrompt(t, m, "book-add", "Intro call | 30m")
	assert.True(t, m.statusIsErr, "same slug is refused")

	copied := ""
	m.clip = func(s string) { copied = s }
	runPrompt(t, m, "book-offer", "intro-call | | 3 | America/New_York")
	require.NotNil(t, m.tv)
	assert.Contains(t, copied, "Intro call (30 min)")
	assert.Contains(t, copied, "[")
	m.tv = nil

	site := filepath.Join(t.TempDir(), "site")
	runPrompt(t, m, "book-export", "intro-call | "+site)
	data, err := os.ReadFile(filepath.Join(site, "index.html"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "mailto:me@example.com")

	invite := filepath.Join(t.TempDir(), "invite.ics")
	runPrompt(t, m, "book-confirm", "intro-call | wed 10:00 | Jane Doe | jane@example.com | "+invite)
	require.False(t, m.statusIsErr, m.status)
	assert.Contains(t, m.status, "booked Wed 14 Oct 10:00")
	ics, err := os.ReadFile(invite)
	require.NoError(t, err)
	assert.Contains(t, string(ics), "METHOD:REQUEST")

	runPrompt(t, m, "book-confirm", "intro-call | wed 10:00 | Bob | bob@example.com")
	assert.True(t, m.statusIsErr, "the time is taken: "+m.status)

	m.runAction(m.actionByID("book-links"))
	require.NotNil(t, m.tv)
	assert.Contains(t, m.tv.vp.View(), "intro-call")
	m.tv = nil

	runPrompt(t, m, "book-cancel", "1")
	assert.Contains(t, m.status, "booking 1 cancelled")
	runPrompt(t, m, "book-pause", "intro-call")
	runPrompt(t, m, "book-resume", "intro-call")
	runPrompt(t, m, "book-rm", "intro-call")
	links, err := repo.NewBookingRepo(m.db).ListLinks(context.Background())
	require.NoError(t, err)
	assert.Empty(t, links)
}

func TestBookingWritesAreBlockedInReadOnlyMode(t *testing.T) {
	m := bookModel(t)
	m.readOnly = true
	m.runAction(m.actionByID("book-add"))
	assert.NotEqual(t, modePrompt, m.mode)
	assert.Contains(t, m.status, "read-only")
}
