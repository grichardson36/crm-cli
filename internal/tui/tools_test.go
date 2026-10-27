package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// toolsModel is a test model whose Env points at a real database file in a temp directory,
// next to a second CRM database and a file that is not a database.
func toolsModel(t *testing.T) *Model {
	t.Helper()
	m, _ := newTestModel(t)
	dir := t.TempDir()
	for _, name := range []string{"crm.db", "work.db"} {
		d, err := db.Open(filepath.Join(dir, name))
		require.NoError(t, err)
		require.NoError(t, d.Close())
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.db"), []byte("not sqlite"), 0o600))
	m.env = Env{DBPath: filepath.Join(dir, "crm.db"), Source: "--db", DataDir: dir, Version: "test", MCPTools: 3}
	return m
}

func runPalette(m *Model, query string) {
	press(m, "ctrl+k")
	typeText(m, query)
	press(m, "enter")
}

func TestPaletteListsHelpDoctorWhereDomains(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		m := toolsModel(t)
		m.readOnly = readOnly
		press(m, "ctrl+k")
		require.NotNil(t, m.ov)
		var titles []string
		for _, it := range m.ov.items {
			if it.disabled == "" {
				titles = append(titles, it.label)
			}
		}
		for _, want := range []string{"Help: topics", "Help: search", "Doctor", "Where", "Domains"} {
			assert.Contains(t, titles, want, "read-only=%v", readOnly)
		}
	}
}

func TestHelpTopicsFilterOpenScrollAndBack(t *testing.T) {
	m := toolsModel(t)
	runPalette(m, "help: topics")
	require.NotNil(t, m.ov)
	assert.Equal(t, "Help topics", m.ov.title)
	all := len(m.ov.shown)
	assert.Greater(t, all, 5)

	typeText(m, "keep")
	require.NotEmpty(t, m.ov.shown)
	assert.Less(t, len(m.ov.shown), all)
	assert.Equal(t, "keep-in-touch", m.ov.items[m.ov.shown[0]].id)

	press(m, "enter")
	require.NotNil(t, m.tv)
	assert.Nil(t, m.ov)
	assert.Contains(t, m.View(), "Help: keep-in-touch")
	assert.Contains(t, m.View(), "ctrl+k for every action")

	// Scrolling: j moves down, G goes to the end, g to the top.
	require.Greater(t, m.tv.vp.TotalLineCount(), m.tv.vp.Height, "topic is longer than the screen")
	press(m, "down")
	assert.Equal(t, 1, m.tv.vp.YOffset)
	press(m, "end")
	assert.True(t, m.tv.vp.AtBottom())
	press(m, "up")
	assert.False(t, m.tv.vp.AtBottom())
	press(m, "home")
	assert.Equal(t, 0, m.tv.vp.YOffset)

	// esc returns to the topic list with the filter kept.
	press(m, "esc")
	assert.Nil(t, m.tv)
	require.NotNil(t, m.ov)
	assert.Equal(t, "keep", m.ov.filter.Value())
	press(m, "esc")
	assert.Nil(t, m.ov)
	assert.Equal(t, modeNormal, m.mode)
}

func TestHelpSearchListsMatchesAndOpensTopic(t *testing.T) {
	m := toolsModel(t)
	runPalette(m, "help: search")
	assert.Equal(t, modePrompt, m.mode)
	typeText(m, "snooze")
	press(m, "enter")
	require.NotNil(t, m.ov)
	assert.Contains(t, m.ov.title, "Help search: snooze")
	require.NotEmpty(t, m.ov.shown)

	press(m, "enter")
	require.NotNil(t, m.tv)
	assert.Contains(t, m.tv.title, "Help: ")
	press(m, "esc")
	require.NotNil(t, m.ov, "esc returns to the matches")
	assert.Contains(t, m.ov.title, "Help search: snooze")
}

func TestHelpSearchWithoutMatchesSetsStatus(t *testing.T) {
	m := toolsModel(t)
	runPalette(m, "help: search")
	typeText(m, "zzqqxxnothing")
	press(m, "enter")
	assert.Nil(t, m.ov)
	assert.True(t, m.statusIsErr)
	assert.Contains(t, m.status, "no help topic mentions")
}

func TestKeysHelpShowsPaletteHint(t *testing.T) {
	m := toolsModel(t)
	press(m, "?")
	assert.Contains(t, m.View(), "ctrl+k for every action")
}

func TestDoctorOverlayShowsChecksAndFixes(t *testing.T) {
	m := toolsModel(t)
	runPalette(m, "doctor")
	require.NotNil(t, m.tv)
	v := m.View()
	assert.Contains(t, v, "Doctor")
	assert.Contains(t, v, "binary")
	assert.Contains(t, v, "database")
	assert.Contains(t, m.tv.vp.View()+strings.Join(strings.Split(v, "\n"), "\n"), "ok")
	assert.Contains(t, v, "ctrl+k for every action")
	press(m, "esc")
	assert.Nil(t, m.tv)
}

func TestDoctorReportsFixForMissingDatabase(t *testing.T) {
	m := toolsModel(t)
	m.env.DBPath = filepath.Join(t.TempDir(), "missing.db")
	runPalette(m, "doctor")
	require.NotNil(t, m.tv)
	v := m.View()
	assert.Contains(t, v, "fail")
	assert.Contains(t, v, "fix: run: crm init")
}

func TestDoctorWorksReadOnly(t *testing.T) {
	m := toolsModel(t)
	m.readOnly = true
	runPalette(m, "doctor")
	require.NotNil(t, m.tv)
	assert.False(t, m.statusIsErr, m.status)
}

func TestWhereShowsActiveDatabase(t *testing.T) {
	m := toolsModel(t)
	m.readOnly = true
	runPalette(m, "where")
	require.NotNil(t, m.tv)
	body := m.tv.vp.View()
	assert.Contains(t, body, m.env.DBPath)
	assert.Contains(t, body, "domain:")
	assert.Contains(t, body, "crm")
	assert.Contains(t, body, "schema version:")
	assert.Contains(t, body, "device id:")
	assert.Contains(t, body, "size:")
	assert.Contains(t, body, "--db")
	press(m, "esc")
	assert.Nil(t, m.tv)
}

func TestDomainsListsCRMDatabasesOnly(t *testing.T) {
	m := toolsModel(t)
	runPalette(m, "domains")
	require.NotNil(t, m.tv)
	body := m.tv.vp.View()
	assert.Contains(t, body, "crm")
	assert.Contains(t, body, "work")
	assert.NotContains(t, body, "notes", "a file that is not a CRM database is skipped")
	assert.Contains(t, body, "people")
	var active string
	for _, l := range strings.Split(body, "\n") {
		if strings.HasPrefix(l, "*") && strings.Contains(l, "crm ") {
			active = l
		}
	}
	assert.Contains(t, active, "crm", "the open database is marked")
}

func TestToolsWithoutDatabasePathSayWhy(t *testing.T) {
	m, _ := newTestModel(t)
	for _, q := range []string{"doctor", "where", "domains"} {
		runPalette(m, q)
		assert.Nil(t, m.tv, q)
		assert.True(t, m.statusIsErr, q)
		assert.Contains(t, m.status, "not known", q)
	}
}
