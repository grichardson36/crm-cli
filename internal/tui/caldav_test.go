package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// With CRM_CALDAV_URL set the CalDAV source shows up in the Sync view and in the palette, like
// any other sync source, so it needs no calendar-specific TUI of its own.
func TestCalDAVSourceIsInTheSyncViewAndPalette(t *testing.T) {
	t.Setenv("CRM_CALDAV_URL", "http://127.0.0.1:1/grant/main/")
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	m, err := New(context.Background(), d)
	require.NoError(t, err)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	press(m, "8")
	var titles []string
	for _, r := range m.rows {
		titles = append(titles, r.title)
	}
	assert.Contains(t, titles, "caldav")

	press(m, "ctrl+k")
	var ids []string
	for _, it := range m.ov.items {
		ids = append(ids, it.id)
	}
	assert.Contains(t, ids, "sync-run:caldav")
	assert.Contains(t, ids, "sync-dry:caldav")
}
