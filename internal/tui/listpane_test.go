package tui

import (
	"context"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The lists live only in the right pane of Settings (listeditor.go, patterns/list-editor-standard).

func hintsText(m *Model) string { return ansi.Strip(m.hintsLine()) }

func paneText(m *Model) string { return ansi.Strip(m.screen()) }

func TestListPane_EnterEntersEscLeaves(t *testing.T) {
	for _, id := range listEditorIDs {
		t.Run(id, func(t *testing.T) {
			m := settingsModel(t)
			cursorToSetting(t, m, id)
			require.NotNil(t, m.paneList(), "the pane shows the list as soon as the row is under the cursor")
			assert.Equal(t, focusList, m.focus)
			press(m, "enter")
			assert.Equal(t, focusDetail, m.focus)
			assert.Nil(t, m.ov, "no popup")
			assert.Equal(t, modeNormal, m.mode)
			press(m, "down")
			assert.Equal(t, id, m.rows[m.cursor].setting, "down moves inside the pane, not in the list of settings")
			press(m, "esc")
			assert.Equal(t, focusList, m.focus)
			press(m, "tab")
			assert.Equal(t, focusDetail, m.focus, "tab enters the pane like right")
			press(m, "shift+tab")
			assert.Equal(t, focusList, m.focus, "shift+tab leaves it")
			press(m, "right")
			assert.Equal(t, focusDetail, m.focus)
			press(m, "left")
			assert.Equal(t, focusList, m.focus, "left leaves it")
		})
	}
}

func TestListPane_ColumnsPerList(t *testing.T) {
	tests := []struct {
		id      string
		heading string
		rows    []string
	}{
		{"relation-kinds", `name\s+inverse\s+used by`, []string{`People`, `related\]?\s+related\s+0`, `parent\s+child\s+0`, `referred-by\s+referred\s+0`, `Organizations`, `parent\s+child\s+0`}},
		{"note-types", `name\s+mark contacted\s+used by`, []string{`call\s+yes\s+0`, `note\s+no\s+0`}},
		{"deal-stages", `name\s+outcome\s+used by`, []string{`lead\s+open\s+0`, `won\s+won\s+0`, `lost\s+lost\s+0`}},
		{"deal-roles", `name\s+used by`, []string{`champion\s+0`, `billing contact\s+0`}},
		{"company-ignore", `text\s+state`, []string{`retired\s+ignored`}},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			m := settingsModel(t)
			openLE(t, m, tt.id)
			screen := paneText(m)
			assert.Regexp(t, tt.heading, screen)
			for _, r := range tt.rows {
				assert.Regexp(t, r, screen)
			}
		})
	}
	t.Run("the ignore list has no used by column", func(t *testing.T) {
		m := settingsModel(t)
		openLE(t, m, "company-ignore")
		assert.NotContains(t, paneText(m), "used by")
	})
	t.Run("the old word is gone", func(t *testing.T) {
		m := settingsModel(t)
		openLE(t, m, "note-types")
		assert.NotContains(t, paneText(m), "counts as contact")
	})
}

func TestListPane_UsedByCountsAreQueried(t *testing.T) {
	ctx := context.Background()
	t.Run("stages", func(t *testing.T) {
		m := settingsModel(t)
		for _, title := range []string{"a", "b"} {
			_, err := m.dlr.Create(ctx, model.CreateDealInput{Title: title, Stage: "lead"})
			require.NoError(t, err)
		}
		openLE(t, m, "deal-stages")
		assert.Regexp(t, `lead\s+open\s+2`, paneText(m))
		assert.Regexp(t, `prospect\s+open\s+0`, paneText(m))
	})
	t.Run("note types", func(t *testing.T) {
		m := settingsModel(t)
		_, err := m.ir.Create(ctx, model.CreateInteractionInput{Type: "call", PersonIDs: []int64{1}})
		require.NoError(t, err)
		openLE(t, m, "note-types")
		assert.Regexp(t, `call\s+yes\s+1`, paneText(m))
	})
	t.Run("relation kinds", func(t *testing.T) {
		m := settingsModel(t)
		kindWithLink(t, m, "client", "vendor")
		openLE(t, m, "relation-kinds")
		assert.Regexp(t, `client\s+vendor\s+1`, paneText(m))
	})
	t.Run("deal roles", func(t *testing.T) {
		m := settingsModel(t)
		d, _ := m.dlr.Create(ctx, model.CreateDealInput{Title: "d", Stage: "lead"})
		_, err := m.db.Exec(`INSERT INTO deal_person (uuid, deal_id, person_id, role) VALUES ('r1', ?, 1, 'champion')`, d.ID)
		require.NoError(t, err)
		openLE(t, m, "deal-roles")
		assert.Regexp(t, `champion\s+1`, paneText(m))
	})
}

func TestListPane_NoPaletteActionOrKeyOpensAList(t *testing.T) {
	m, _ := newTestModel(t)
	gone := map[string]bool{"deal-stages": true, "deal-roles": true, "relation-kinds": true, "org-ignore": true, "note-types-edit": true}
	for _, a := range m.actions {
		assert.False(t, gone[a.id], "%s opens a list", a.id)
		for _, w := range []string{"Edit deal stages", "Edit deal roles", "Edit note types", "Edit relation kinds", "Ignored company texts", "Manage deal stages", "Manage deal roles", "Manage note types"} {
			assert.NotEqual(t, w, a.title, "a standalone entry for a list")
		}
	}
	for _, q := range []string{"deal stages", "deal roles", "note types", "relation kinds", "ignored company"} {
		t.Run(q, func(t *testing.T) {
			m, _ := newTestModel(t)
			press(m, "ctrl+k")
			typeText(m, q)
			for _, idx := range m.ov.shown {
				it := m.ov.items[idx]
				assert.False(t, it.kind == kindAction && strings.Contains(strings.ToLower(it.label), q), "the palette lists an action for %q: %s", q, it.label)
			}
			press(m, "esc")
		})
	}
	t.Run("the pane is the only door", func(t *testing.T) {
		m, _ := newTestModel(t)
		for _, k := range []string{"ctrl+k", "v", "?"} {
			press(m, k, "esc")
			assert.Nil(t, m.lp)
		}
	})
}

func TestListPane_FooterShowsOnlyKeysThatWorkOnTheRow(t *testing.T) {
	has := func(footer, hint string) bool { return strings.Contains(footer, hint) }
	t.Run("a built-in link kind can only be moved", func(t *testing.T) {
		m, _ := newTestModel(t)
		le := openLE(t, m, "relation-kinds")
		leTo(t, m, le, "friend")
		f := hintsText(m)
		for _, want := range []string{"↑/↓ select", "a add", "shift+↑/↓ move", "shift+r reset", "esc back"} {
			assert.True(t, has(f, want), "%q in %q", want, f)
		}
		for _, no := range []string{"e rename", "d delete", "tab"} {
			assert.False(t, has(f, no), "%q in %q", no, f)
		}
	})
	t.Run("an added link kind has rename, delete and the inverse column", func(t *testing.T) {
		m, _ := newTestModel(t)
		le := openLE(t, m, "relation-kinds")
		press(m, "a")
		typeText(m, "client")
		press(m, "enter")
		f := hintsText(m)
		for _, want := range []string{"e rename", "tab column", "d delete"} {
			assert.True(t, has(f, want), "%q in %q", want, f)
		}
		press(m, "tab")
		assert.True(t, has(hintsText(m), "e inverse"), hintsText(m))
		_ = le
	})
	t.Run("the default note type has no rename and no delete but tab", func(t *testing.T) {
		m, _ := newTestModel(t)
		le := openLE(t, m, "note-types")
		leTo(t, m, le, "note")
		f := hintsText(m)
		assert.True(t, has(f, "tab mark contacted"), f)
		assert.False(t, has(f, "e rename"), f)
		assert.False(t, has(f, "d delete"), f)
		leTo(t, m, le, "call")
		f = hintsText(m)
		assert.True(t, has(f, "e rename") && has(f, "d delete") && has(f, "tab mark contacted"), f)
	})
	t.Run("stages cycle the outcome", func(t *testing.T) {
		m, _ := newTestModel(t)
		openLE(t, m, "deal-stages")
		f := hintsText(m)
		assert.True(t, has(f, "tab outcome"), f)
		assert.True(t, has(f, "shift+r reset"), f)
	})
	t.Run("roles have no tab", func(t *testing.T) {
		m, _ := newTestModel(t)
		openLE(t, m, "deal-roles")
		assert.False(t, has(hintsText(m), "tab"), hintsText(m))
	})
	t.Run("the ignore list has no move and no reset, a turned off built-in has no delete", func(t *testing.T) {
		m, _ := newTestModel(t)
		le := openLE(t, m, "company-ignore")
		leTo(t, m, le, "retired")
		f := hintsText(m)
		assert.False(t, has(f, "shift+↑/↓") || has(f, "shift+r"), f)
		assert.False(t, has(f, "e rename"), "a built-in text keeps its name: %s", f)
		assert.True(t, has(f, "d delete"), f)
		press(m, "d")
		assert.False(t, has(hintsText(m), "d delete"), hintsText(m))
	})
	t.Run("read-only shows no write key", func(t *testing.T) {
		m, _ := newTestModel(t)
		m.readOnly = true
		openLE(t, m, "deal-stages")
		f := hintsText(m)
		for _, no := range []string{"a add", "e rename", "d delete", "tab", "shift+r"} {
			assert.False(t, has(f, no), "%q in %q", no, f)
		}
		assert.True(t, has(f, "esc back"), f)
	})
	t.Run("the footer is one word per key, no sentences", func(t *testing.T) {
		m, _ := newTestModel(t)
		openLE(t, m, "deal-stages")
		assert.NotContains(t, hintsText(m), "[")
		assert.NotContains(t, hintsText(m), ".")
	})
}

func removeVocab(t *testing.T, m *Model, kind, name string) {
	t.Helper()
	_, err := m.vr.Remove(m.ctx, kind, name, "", false)
	require.NoError(t, err)
}

func stageNames(t *testing.T, m *Model) []string {
	t.Helper()
	names, err := m.vr.Names(m.ctx, model.DealStageKind)
	require.NoError(t, err)
	return names
}

func TestListPane_ResetStages(t *testing.T) {
	m, _ := newTestModel(t)
	defaults := stageNames(t, m)
	removeVocab(t, m, model.DealStageKind, "prospect")
	_, err := m.vr.Add(m.ctx, model.DealStageKind, "custom")
	require.NoError(t, err)
	_, err = m.vr.SetOutcome(m.ctx, model.DealStageKind, "negotiation", "won")
	require.NoError(t, err)
	require.NoError(t, m.vr.Reorder(m.ctx, model.DealStageKind, []string{"won", "lost", "lead"}))
	before := stageNames(t, m)
	require.NotEqual(t, defaults, before)

	le := openLE(t, m, "deal-stages")
	histBefore := len(m.hist)
	press(m, "R")
	require.Equal(t, lpConfirm, le.phase, "a confirm comes first")
	screen := paneText(m)
	assert.Contains(t, screen, "add prospect")
	assert.Contains(t, screen, "negotiation outcome open")
	assert.Contains(t, screen, "order")
	assert.Equal(t, before, stageNames(t, m), "nothing is written before the answer")
	assert.Contains(t, hintsText(m), "y yes")

	press(m, "n")
	assert.Equal(t, lpBrowse, le.phase)
	assert.Equal(t, before, stageNames(t, m))

	press(m, "R", "y")
	require.Equal(t, lpBrowse, le.phase, le.msg)
	assert.False(t, le.msgErr, le.msg)
	assert.Equal(t, append(append([]string{}, defaults...), "custom"), stageNames(t, m), "built-in stages first in the default order, the added one stays")
	it, _ := leItem(le, "negotiation")
	assert.Equal(t, "open", it.cells[0])
	assert.Equal(t, histBefore+1, len(m.hist), "the reset is one undo step")

	press(m, "ctrl+z")
	assert.Equal(t, before, stageNames(t, m), "one undo puts everything back")
	it, _ = leItem(le, "negotiation")
	assert.Equal(t, "won", it.cells[0])
	press(m, "ctrl+y")
	assert.Equal(t, append(append([]string{}, defaults...), "custom"), stageNames(t, m))

	press(m, "R")
	assert.Equal(t, lpBrowse, le.phase, "nothing left to reset")
	assert.Contains(t, le.msg, "defaults")
}

func TestListPane_ResetRolesNotesAndLinks(t *testing.T) {
	t.Run("deal roles", func(t *testing.T) {
		m, _ := newTestModel(t)
		defaults, _ := m.vr.Names(m.ctx, model.DealRoleKind)
		removeVocab(t, m, model.DealRoleKind, "signer")
		le := openLE(t, m, "deal-roles")
		press(m, "R", "y")
		got, _ := m.vr.Names(m.ctx, model.DealRoleKind)
		assert.Equal(t, defaults, got)
		press(m, "ctrl+z")
		got, _ = m.vr.Names(m.ctx, model.DealRoleKind)
		assert.NotContains(t, got, "signer")
		_ = le
	})
	t.Run("note types", func(t *testing.T) {
		m, _ := newTestModel(t)
		require.NoError(t, m.ntr.SetCountsAsContact(m.ctx, "call", false))
		require.NoError(t, m.ntr.Archive(m.ctx, "meeting"))
		le := openLE(t, m, "note-types")
		press(m, "R")
		require.Equal(t, lpConfirm, le.phase)
		assert.Contains(t, paneText(m), "add meeting")
		assert.Contains(t, paneText(m), "call mark contacted yes")
		press(m, "y")
		call, _ := m.ntr.FindByName(m.ctx, "call")
		assert.True(t, call.CountsAsContact)
		meeting, err := m.ntr.FindByName(m.ctx, "meeting")
		require.NoError(t, err)
		assert.False(t, meeting.Archived)
		assert.True(t, meeting.CountsAsContact, "the default of a built-in type comes back")
		assert.Equal(t, []string{"call", "email", "meeting", "message", "note"}, m.noteTypeNames())
	})
	t.Run("links put the built-in kinds back in order and keep added ones", func(t *testing.T) {
		m, _ := newTestModel(t)
		le := openLE(t, m, "relation-kinds")
		want := leNames(le, "person")
		press(m, "a")
		typeText(m, "client")
		press(m, "enter")
		leTo(t, m, le, "friend")
		press(m, "shift+up")
		require.NotEqual(t, append(append([]string{}, want...), "client"), leNames(le, "person"))
		press(m, "R")
		require.Equal(t, lpConfirm, le.phase)
		assert.Contains(t, paneText(m), "order people")
		press(m, "y")
		assert.Equal(t, append(append([]string{}, want...), "client"), leNames(le, "person"))
	})
	t.Run("the ignore list has no reset", func(t *testing.T) {
		m, _ := newTestModel(t)
		le := openLE(t, m, "company-ignore")
		press(m, "R")
		assert.Equal(t, lpBrowse, le.phase)
	})
}

func TestListPane_ResetKeepsRecordsOfItemsInUse(t *testing.T) {
	ctx := context.Background()
	m, _ := newTestModel(t)
	_, err := m.dlr.Create(ctx, model.CreateDealInput{Title: "a", Stage: "negotiation"})
	require.NoError(t, err)
	_, err = m.vr.Add(m.ctx, model.DealStageKind, "custom")
	require.NoError(t, err)
	_, err = m.dlr.Create(ctx, model.CreateDealInput{Title: "b", Stage: "custom"})
	require.NoError(t, err)
	openLE(t, m, "deal-stages")
	press(m, "R")
	assert.Equal(t, lpBrowse, m.lp.phase, "an added stage in use is not a change")
	assert.Equal(t, 1, dealCount(t, m, "custom"))
}

func TestListPane_NothingRunsOffThePane(t *testing.T) {
	m := settingsModel(t)
	le := openLE(t, m, "deal-roles")
	long := strings.Repeat("long name ", 8)
	press(m, "a")
	typeText(m, strings.TrimSpace(long))
	press(m, "enter")
	require.Equal(t, lpBrowse, le.phase, le.msg)
	for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 30}, {Width: 70, Height: 20}, {Width: 50, Height: 12}} {
		m.Update(size)
		w := m.detail.Width()
		for _, l := range le.paneLines(m, w, m.detail.Height()) {
			assert.LessOrEqual(t, ansi.StringWidth(l), w, "pane line %q at %dx%d", ansi.Strip(l), size.Width, size.Height)
		}
		for _, l := range strings.Split(m.screen(), "\n") {
			assert.LessOrEqual(t, ansi.StringWidth(l), size.Width, "screen line at %dx%d", size.Width, size.Height)
		}
	}
	assert.Contains(t, paneText(m), "…", "a long value is cut with an ellipsis")
	press(m, "d")
	for _, l := range le.paneLines(m, m.detail.Width(), m.detail.Height()) {
		assert.LessOrEqual(t, ansi.StringWidth(l), m.detail.Width())
	}
}

func TestListPane_ConfirmQuestionWrapsInThePane(t *testing.T) {
	m := settingsModel(t)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	ctx := context.Background()
	_, err := m.dlr.Create(ctx, model.CreateDealInput{Title: "a", Stage: "lead"})
	require.NoError(t, err)
	le := openLE(t, m, "deal-stages")
	leTo(t, m, le, "lead")
	press(m, "d", "enter")
	require.Equal(t, lpConfirm, le.phase)
	w := m.detail.Width()
	var words []string
	for _, l := range le.promptLines(m, w) {
		assert.LessOrEqual(t, ansi.StringWidth(l), w)
		words = append(words, strings.Fields(ansi.Strip(l))...)
	}
	assert.Regexp(t, regexp.MustCompile(`^Remove lead \(1 deals\)\. Move them to prospect\?$`), strings.Join(words, " "), "the question is whole, wrapped over lines")
	assert.Greater(t, len(le.promptLines(m, w)), 1)
}
