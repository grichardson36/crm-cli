package tui

import (
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func countRows(t *testing.T, m *Model, table string) int {
	t.Helper()
	var n int
	require.NoError(t, m.db.QueryRowContext(m.ctx, "SELECT COUNT(*) FROM "+table).Scan(&n))
	return n
}

func TestPaletteListsDavViewsInReadOnlyMode(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		m, _ := newTestModel(t)
		m.readOnly = readOnly
		press(m, "ctrl+k")
		require.NotNil(t, m.ov)
		var titles []string
		for _, it := range m.ov.items {
			if it.disabled == "" {
				titles = append(titles, it.label)
			}
		}
		for _, want := range []string{"DAV: info", "DAV: conflicts", "DAV: status"} {
			assert.Contains(t, titles, want, "read-only=%v", readOnly)
		}
	}
}

func TestDavInfoShowsConnectionDetailsReadOnly(t *testing.T) {
	t.Setenv("CRM_DAV_USER", "grant")
	t.Setenv("CRM_DAV_BASE_URL", "https://contacts.example.com")
	m := toolsModel(t)
	m.readOnly = true
	runPalette(m, "dav: info")
	require.NotNil(t, m.tv)
	assert.False(t, m.statusIsErr, m.status)
	body := m.tv.vp.View()
	assert.Contains(t, body, "https://contacts.example.com")
	assert.Contains(t, body, "grant")
	assert.Contains(t, body, "CRM contacts", "book is named after crm.db")
	assert.Contains(t, m.View(), "DAV info")
	press(m, "end")
	assert.Contains(t, m.tv.vp.View(), "does not start the server")
	press(m, "esc")
	assert.Nil(t, m.tv)
}

func TestDavInfoDefaultsWithoutEnv(t *testing.T) {
	t.Setenv("CRM_DAV_USER", "")
	t.Setenv("CRM_DAV_BASE_URL", "")
	m, _ := newTestModel(t)
	runPalette(m, "dav: info")
	require.NotNil(t, m.tv)
	assert.Contains(t, m.tv.vp.View(), "http://127.0.0.1:5232")
	assert.Contains(t, m.tv.vp.View(), "user:")
}

func TestDavStatusShowsCountsReadOnly(t *testing.T) {
	m, _ := newTestModel(t)
	m.readOnly = true
	runPalette(m, "dav: status")
	require.NotNil(t, m.tv)
	assert.False(t, m.statusIsErr, m.status)
	body := m.tv.vp.View()
	assert.Contains(t, body, "contacts exposed:")
	assert.Contains(t, body, "3", "three active people")
	assert.Contains(t, body, "last request:")
	assert.Contains(t, body, "never")
	assert.Contains(t, body, "conflicts:")
	assert.Contains(t, body, "0 (0 open)")
	press(m, "esc")
	assert.Nil(t, m.tv)
}

func TestDavConflictsEmptySetsStatus(t *testing.T) {
	m, _ := newTestModel(t)
	runPalette(m, "dav: conflicts")
	assert.Nil(t, m.ov)
	assert.Nil(t, m.tv)
	assert.False(t, m.statusIsErr)
	assert.Contains(t, m.status, "no DAV conflicts")
}

func TestDavConflictsListShowAndBackReadOnly(t *testing.T) {
	m, pr := newTestModel(t)
	m.readOnly = true
	people, err := pr.ListSummaries(m.ctx)
	require.NoError(t, err)
	require.NotEmpty(t, people)
	dr := repo.NewDavRepo(m.db)
	crmV, devV := "Acme", "Acme Corp"
	c1, err := dr.AddConflict(m.ctx, people[0].ID, "company", &crmV, &devV, model.ConflictWinnerDevice)
	require.NoError(t, err)
	_, err = dr.AddConflict(m.ctx, people[1].ID, "title", &devV, &crmV, model.ConflictWinnerCRM)
	require.NoError(t, err)
	require.NoError(t, dr.MarkConflictUndone(m.ctx, c1.ID))
	before := countRows(t, m, "sync_conflicts")

	runPalette(m, "dav: conflicts")
	require.NotNil(t, m.ov)
	assert.Contains(t, m.ov.title, "DAV conflicts (2)")
	require.Len(t, m.ov.items, 2)
	assert.Contains(t, m.ov.items[0].label, "title", "newest first")
	assert.Contains(t, m.ov.items[1].label, "(undone)")

	press(m, "down")
	press(m, "enter")
	require.NotNil(t, m.tv)
	assert.Contains(t, m.tv.title, "DAV conflict #")
	body := m.tv.vp.View()
	assert.Contains(t, body, "Acme Corp")
	assert.Contains(t, body, "company")
	assert.Contains(t, body, "undone:    true")
	assert.NotContains(t, body, "crm dav conflicts undo", "an undone conflict offers no undo")

	press(m, "esc")
	require.NotNil(t, m.ov, "esc returns to the list")
	press(m, "enter")
	require.NotNil(t, m.tv)
	assert.Contains(t, m.tv.vp.View(), "crm dav conflicts undo", "an open conflict names the CLI undo")

	assert.Equal(t, before, countRows(t, m, "sync_conflicts"), "viewing changes nothing")
	open, err := dr.Conflicts(m.ctx, false, 0)
	require.NoError(t, err)
	assert.Len(t, open, 1, "the open conflict is still open")
}
