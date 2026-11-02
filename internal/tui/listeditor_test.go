package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The list editor (patterns/list-editor-standard), tested through the keys, for each of the four
// lists with the same steps.

func openLE(t *testing.T, m *Model, id string) *listEditor {
	t.Helper()
	runCmds(m, m.openListEditor(id))
	require.NotNil(t, m.ov, id)
	require.NotNil(t, m.ov.le, "%s opens the list editor", id)
	return m.ov.le
}

func leNames(le *listEditor, group string) []string {
	var out []string
	for _, it := range le.items {
		if it.group == group {
			out = append(out, it.name)
		}
	}
	return out
}

// leTo moves the cursor onto the item with the name.
func leTo(t *testing.T, m *Model, le *listEditor, name string) {
	t.Helper()
	press(m, "home")
	for range 80 {
		if it, ok := le.cur(); ok && it.name == name {
			return
		}
		press(m, "down")
	}
	t.Fatalf("no item %q in %v", name, le.items)
}

func leCur(t *testing.T, le *listEditor) listItem {
	t.Helper()
	it, ok := le.cur()
	require.True(t, ok)
	return it
}

func leItem(le *listEditor, name string) (listItem, bool) {
	for _, it := range le.items {
		if strings.EqualFold(it.name, name) {
			return it, true
		}
	}
	return listItem{}, false
}

func backspaces(m *Model, n int) {
	for range n {
		press(m, "backspace")
	}
}

func TestListEditor_SameKeysForEveryList(t *testing.T) {
	tests := []struct {
		id    string
		title string
		group string
		first string // an existing item that is not locked
	}{
		{"deal-stages", "Deal stages", "", "prospect"},
		{"deal-roles", "Deal roles", "", "champion"},
		{"note-types", "Note types", "", "call"},
		{"relation-kinds", "Relation kinds", "person", "friend"},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			m, _ := newTestModel(t)
			le := openLE(t, m, tt.id)
			assert.Equal(t, tt.title, m.ov.title)
			start := leNames(le, tt.group)
			require.Contains(t, start, tt.first)

			// a adds inline, enter saves, the new item is last in its group and under the cursor.
			leTo(t, m, le, tt.first)
			press(m, "a")
			require.Equal(t, lpAdd, le.phase)
			typeText(m, "zeta")
			press(m, "enter")
			require.Equal(t, lpBrowse, le.phase, le.msg)
			assert.Equal(t, append(append([]string{}, start...), "zeta"), leNames(le, tt.group))
			assert.Equal(t, "zeta", leCur(t, le).name)

			// enter renames inline.
			press(m, "enter")
			require.Equal(t, lpRename, le.phase)
			backspaces(m, 4)
			typeText(m, "omega")
			press(m, "enter")
			require.Equal(t, lpBrowse, le.phase, le.msg)
			assert.Equal(t, "omega", leCur(t, le).name)

			// shift+up moves it up, shift+down back.
			press(m, "shift+up")
			names := leNames(le, tt.group)
			assert.Equal(t, "omega", names[len(names)-2])
			assert.Equal(t, "omega", leCur(t, le).name, "the cursor follows the item")
			press(m, "shift+down")
			names = leNames(le, tt.group)
			assert.Equal(t, "omega", names[len(names)-1])
			press(m, "shift+down")
			assert.Equal(t, "omega", leNames(le, tt.group)[len(names)-1], "the end of the group stops it")

			// d on an unused item removes it at once, ctrl+z brings it back.
			press(m, "d")
			require.Equal(t, lpBrowse, le.phase, le.msg)
			_, there := leItem(le, "omega")
			assert.False(t, there)
			press(m, "ctrl+z")
			_, there = leItem(le, "omega")
			assert.True(t, there, "undo of the removal")
			press(m, "ctrl+y")
			_, there = leItem(le, "omega")
			assert.False(t, there, "redo of the removal")

			// Undo everything above: removal, moves, rename, add.
			for range 5 {
				press(m, "ctrl+z")
			}
			assert.Equal(t, start, leNames(le, tt.group))
			press(m, "esc")
			assert.Equal(t, modeNormal, m.mode, "esc closes")
		})
	}
}

func TestListEditor_EscCancelsInlineInput(t *testing.T) {
	m, _ := newTestModel(t)
	le := openLE(t, m, "deal-roles")
	before := leNames(le, "")
	press(m, "a")
	typeText(m, "nope")
	press(m, "esc")
	assert.Equal(t, lpBrowse, le.phase)
	assert.Equal(t, modeOverlay, m.mode, "esc leaves the input, not the editor")
	assert.Equal(t, before, leNames(le, ""))
	leTo(t, m, le, "champion")
	press(m, "e")
	require.Equal(t, lpRename, le.phase, "e renames like enter")
	typeText(m, "x")
	press(m, "esc")
	assert.Equal(t, before, leNames(le, ""))
}

func TestListEditor_AddRefusesADuplicateAndKeepsTheInput(t *testing.T) {
	m, _ := newTestModel(t)
	le := openLE(t, m, "deal-roles")
	press(m, "a")
	typeText(m, "CHAMPION")
	press(m, "enter")
	assert.Equal(t, lpAdd, le.phase, "the input stays open to fix the name")
	assert.True(t, le.msgErr)
	assert.Contains(t, le.msg, "champion")
}

func TestListEditor_RenamePropagatesToRecords(t *testing.T) {
	ctx := context.Background()
	t.Run("deal stage", func(t *testing.T) {
		m, _ := newTestModel(t)
		d, err := m.dlr.Create(ctx, model.CreateDealInput{Title: "Site", Stage: "lead"})
		require.NoError(t, err)
		le := openLE(t, m, "deal-stages")
		leTo(t, m, le, "lead")
		press(m, "enter")
		backspaces(m, 4)
		typeText(m, "new lead")
		press(m, "enter")
		got, _ := m.dlr.FindByID(ctx, d.ID)
		assert.Equal(t, "new lead", got.Stage)
		press(m, "ctrl+z")
		got, _ = m.dlr.FindByID(ctx, d.ID)
		assert.Equal(t, "lead", got.Stage)
	})
	t.Run("note type", func(t *testing.T) {
		m, pr := newTestModel(t)
		p, _ := pr.FindByID(ctx, 1)
		_, err := m.ir.Create(ctx, model.CreateInteractionInput{Type: "call", PersonIDs: []int64{p.ID}})
		require.NoError(t, err)
		le := openLE(t, m, "note-types")
		leTo(t, m, le, "call")
		assert.Equal(t, 1, leCur(t, le).inUse)
		press(m, "enter")
		backspaces(m, 4)
		typeText(m, "phone")
		press(m, "enter")
		c, _ := m.ntr.Counts(ctx)
		assert.Equal(t, 1, c["phone"])
		assert.Equal(t, 0, c["call"])
	})
	t.Run("the default note type keeps its name", func(t *testing.T) {
		m, _ := newTestModel(t)
		le := openLE(t, m, "note-types")
		leTo(t, m, le, "note")
		press(m, "enter")
		assert.Equal(t, lpBrowse, le.phase)
		assert.True(t, le.msgErr)
		assert.Contains(t, le.msg, "default type")
	})
}

// removal fixtures

type removalCase struct {
	name string
	id   string
	// setup creates records that use the item "victim" and returns a function that counts them where they should be.
	victim string
	setup  func(t *testing.T, m *Model)
	// the picker rows expected, in order
	picks []string
	// choose is the picker row to take
	choose int
	// check runs after the removal, undo checks the state before again.
	after  func(t *testing.T, m *Model)
	before func(t *testing.T, m *Model)
	// message part expected in the status line
	say string
}

func dealCount(t *testing.T, m *Model, stage string) int {
	t.Helper()
	var n int
	require.NoError(t, m.db.QueryRow(`SELECT COUNT(*) FROM deals WHERE stage = ?`, stage).Scan(&n))
	return n
}

func roleOf(t *testing.T, m *Model, uuid string) any {
	t.Helper()
	var v any
	require.NoError(t, m.db.QueryRow(`SELECT role FROM deal_person WHERE uuid = ?`, uuid).Scan(&v))
	return v
}

func TestListEditor_RemoveInUse(t *testing.T) {
	ctx := context.Background()
	tests := []removalCase{
		{
			name: "stage: move deals to another stage", id: "deal-stages", victim: "lead",
			setup: func(t *testing.T, m *Model) {
				for _, title := range []string{"a", "b"} {
					_, err := m.dlr.Create(ctx, model.CreateDealInput{Title: title, Stage: "lead"})
					require.NoError(t, err)
				}
			},
			picks:  []string{"Move them to prospect", "Move them to proposal", "Move them to negotiation", "Move them to won", "Move them to lost"},
			choose: 0,
			before: func(t *testing.T, m *Model) { assert.Equal(t, 2, dealCount(t, m, "lead")) },
			after: func(t *testing.T, m *Model) {
				assert.Equal(t, 0, dealCount(t, m, "lead"))
				assert.Equal(t, 2, dealCount(t, m, "prospect"))
			},
			say: "2 deals moved to prospect",
		},
		{
			name: "role: leave blank", id: "deal-roles", victim: "champion",
			setup: func(t *testing.T, m *Model) {
				d, _ := m.dlr.Create(ctx, model.CreateDealInput{Title: "d", Stage: "lead"})
				_, err := m.db.Exec(`INSERT INTO deal_person (uuid, deal_id, person_id, role) VALUES ('r1', ?, 1, 'champion')`, d.ID)
				require.NoError(t, err)
			},
			picks:  nil, // checked by prefix below
			choose: -1,  // the blank row, last
			before: func(t *testing.T, m *Model) { assert.Equal(t, "champion", roleOf(t, m, "r1")) },
			after:  func(t *testing.T, m *Model) { assert.Nil(t, roleOf(t, m, "r1")) },
			say:    "1 links now blank",
		},
		{
			name: "role: move to another role", id: "deal-roles", victim: "champion",
			setup: func(t *testing.T, m *Model) {
				d, _ := m.dlr.Create(ctx, model.CreateDealInput{Title: "d", Stage: "lead"})
				_, err := m.db.Exec(`INSERT INTO deal_person (uuid, deal_id, person_id, role) VALUES ('r1', ?, 1, 'champion')`, d.ID)
				require.NoError(t, err)
			},
			picks:  nil,
			choose: 0,
			before: func(t *testing.T, m *Model) { assert.Equal(t, "champion", roleOf(t, m, "r1")) },
			after:  func(t *testing.T, m *Model) { assert.Equal(t, "decision maker", roleOf(t, m, "r1")) },
			say:    "moved to decision maker",
		},
		{
			name: "note type: move notes", id: "note-types", victim: "call",
			setup: func(t *testing.T, m *Model) {
				_, err := m.ir.Create(ctx, model.CreateInteractionInput{Type: "call", PersonIDs: []int64{1}})
				require.NoError(t, err)
			},
			picks:  nil,
			choose: 0, // email
			before: func(t *testing.T, m *Model) {
				c, _ := m.ntr.Counts(ctx)
				assert.Equal(t, 1, c["call"])
			},
			after: func(t *testing.T, m *Model) {
				c, _ := m.ntr.Counts(ctx)
				assert.Equal(t, 0, c["call"])
				assert.Equal(t, 1, c["email"])
			},
			say: "1 notes moved to email",
		},
		{
			name: "note type: keep the type on the old notes", id: "note-types", victim: "call",
			setup: func(t *testing.T, m *Model) {
				_, err := m.ir.Create(ctx, model.CreateInteractionInput{Type: "call", PersonIDs: []int64{1}})
				require.NoError(t, err)
			},
			choose: -1,
			before: func(t *testing.T, m *Model) { assert.Contains(t, m.noteTypeNames(), "call") },
			after: func(t *testing.T, m *Model) {
				assert.NotContains(t, m.noteTypeNames(), "call")
				c, _ := m.ntr.Counts(ctx)
				assert.Equal(t, 1, c["call"], "old notes keep the type")
			},
			say: "1 notes keep the type",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestModel(t)
			tt.setup(t, m)
			le := openLE(t, m, tt.id)
			leTo(t, m, le, tt.victim)
			require.Positive(t, leCur(t, le).inUse)
			press(m, "d")
			require.Equal(t, lpPick, le.phase, "items in use ask where their records go: %s", le.msg)
			if tt.picks != nil {
				var got []string
				for _, p := range le.pick {
					got = append(got, p.label)
				}
				assert.Equal(t, tt.picks, got, "the picker is built from the same list")
			}
			screen := m.screen()
			assert.Contains(t, screen, "Where do they go?")
			assert.Regexp(t, `\d+ \w+ use `+tt.victim, screen, "the count of affected records is shown")
			if tt.choose >= 0 {
				press(m, strings.Repeat("down,", tt.choose)[:0])
				for range tt.choose {
					press(m, "down")
				}
			} else {
				press(m, "up") // wraps to the last row
			}
			press(m, "enter")
			require.Equal(t, lpConfirm, le.phase)
			assert.Contains(t, m.screen(), "y/n")
			press(m, "y")
			require.Equal(t, lpBrowse, le.phase, le.msg)
			assert.False(t, le.msgErr, le.msg)
			assert.Contains(t, le.msg, tt.say)
			_, there := leItem(le, tt.victim)
			assert.False(t, there)
			tt.after(t, m)
			// ONE undo step puts the item and every record back.
			press(m, "ctrl+z")
			_, there = leItem(le, tt.victim)
			assert.True(t, there)
			tt.before(t, m)
			press(m, "ctrl+y")
			tt.after(t, m)
		})
	}
}

func TestListEditor_RemovalOptionsPerList(t *testing.T) {
	ctx := context.Background()
	picksOf := func(t *testing.T, id, victim string, setup func(m *Model)) []string {
		m, _ := newTestModel(t)
		setup(m)
		le := openLE(t, m, id)
		leTo(t, m, le, victim)
		press(m, "d")
		require.Equal(t, lpPick, le.phase, le.msg)
		var out []string
		for _, p := range le.pick {
			out = append(out, p.label)
		}
		return out
	}
	t.Run("stages never offer blank", func(t *testing.T) {
		got := picksOf(t, "deal-stages", "lead", func(m *Model) { m.dlr.Create(ctx, model.CreateDealInput{Title: "a", Stage: "lead"}) })
		for _, g := range got {
			assert.True(t, strings.HasPrefix(g, "Move them to "), g)
		}
	})
	t.Run("roles offer blank", func(t *testing.T) {
		got := picksOf(t, "deal-roles", "champion", func(m *Model) {
			d, _ := m.dlr.Create(ctx, model.CreateDealInput{Title: "a", Stage: "lead"})
			m.db.Exec(`INSERT INTO deal_person (uuid, deal_id, person_id, role) VALUES ('r1', ?, 1, 'champion')`, d.ID)
		})
		assert.Equal(t, "Leave the role blank on those links", got[len(got)-1])
		assert.NotContains(t, got, "Move them to champion")
	})
	t.Run("note types offer keep", func(t *testing.T) {
		got := picksOf(t, "note-types", "call", func(m *Model) {
			m.ir.Create(ctx, model.CreateInteractionInput{Type: "call", PersonIDs: []int64{1}})
		})
		assert.Equal(t, "Keep the type on those notes (the type is archived)", got[len(got)-1])
	})
	t.Run("relation kinds offer unlink", func(t *testing.T) {
		got := picksOf(t, "relation-kinds", "client", func(m *Model) { kindWithLink(t, m, "client", "vendor") })
		assert.Equal(t, "Unlink them (the links are deleted, ctrl+z brings them back)", got[len(got)-1])
		assert.Contains(t, got, "Move them to friend")
	})
}

func kindWithLink(t *testing.T, m *Model, name, inverse string) {
	t.Helper()
	k, err := repoKinds(m).AddKind(m.ctx, "person", name, inverse, false)
	require.NoError(t, err)
	_, err = m.db.Exec(`INSERT INTO person_relation (uuid, a_id, b_id, kind_id) VALUES ('pr1', 1, 2, ?)`, k.ID)
	require.NoError(t, err)
}

func TestListEditor_RelationKindRemoval(t *testing.T) {
	linkKind := func(t *testing.T, m *Model) (kind string, live bool) {
		var k string
		err := m.db.QueryRow(`SELECT rk.name FROM person_relation pr JOIN relation_kind rk ON rk.id = pr.kind_id WHERE pr.uuid = 'pr1' AND pr.deleted_at IS NULL`).Scan(&k)
		return k, err == nil
	}
	t.Run("move the links to another kind", func(t *testing.T) {
		m, _ := newTestModel(t)
		kindWithLink(t, m, "client", "vendor")
		le := openLE(t, m, "relation-kinds")
		leTo(t, m, le, "client")
		press(m, "d")
		i := 0
		for ; i < len(le.pick); i++ {
			if le.pick[i].label == "Move them to colleague" {
				break
			}
			press(m, "down")
		}
		press(m, "enter", "y")
		k, live := linkKind(t, m)
		assert.True(t, live)
		assert.Equal(t, "colleague", k)
		press(m, "ctrl+z")
		k, _ = linkKind(t, m)
		assert.Equal(t, "client", k)
	})
	t.Run("unlink them, one undo step", func(t *testing.T) {
		m, _ := newTestModel(t)
		kindWithLink(t, m, "client", "vendor")
		le := openLE(t, m, "relation-kinds")
		leTo(t, m, le, "client")
		press(m, "d", "up", "enter")
		assert.Contains(t, m.screen(), "Unlink them", "the confirmation says what happens")
		assert.Contains(t, m.screen(), "1 links")
		press(m, "y")
		_, live := linkKind(t, m)
		assert.False(t, live)
		press(m, "ctrl+z")
		_, live = linkKind(t, m)
		assert.True(t, live)
	})
	t.Run("a built-in kind stays", func(t *testing.T) {
		m, _ := newTestModel(t)
		le := openLE(t, m, "relation-kinds")
		leTo(t, m, le, "friend")
		press(m, "d")
		assert.True(t, le.msgErr)
		assert.Contains(t, le.msg, "built-in")
		press(m, "enter")
		assert.Equal(t, lpBrowse, le.phase, "a built-in kind is not renamed")
	})
	t.Run("kinds are grouped by object type", func(t *testing.T) {
		m, _ := newTestModel(t)
		le := openLE(t, m, "relation-kinds")
		screen := m.screen()
		assert.Contains(t, screen, "Person links")
		assert.Contains(t, screen, "Organization links")
		press(m, "end")
		require.Equal(t, "organization", leCur(t, le).group)
		press(m, "a")
		typeText(m, "partner")
		press(m, "enter")
		assert.Contains(t, leNames(le, "organization"), "partner", "the add goes to the group under the cursor")
	})
}

func TestListEditor_StageRulesBlockRemoval(t *testing.T) {
	tests := []struct {
		name   string
		remove []string
		want   string
	}{
		{"the only lost stage", []string{"lost"}, "no lost stage"},
		{"the only won stage", []string{"won"}, "no won stage"},
		{"both closing stages selected together", []string{"won", "lost"}, "one open, one won and one lost stage must remain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestModel(t)
			le := openLE(t, m, "deal-stages")
			before := leNames(le, "")
			for _, n := range tt.remove {
				leTo(t, m, le, n)
				if len(tt.remove) > 1 {
					press(m, " ")
				}
			}
			if len(tt.remove) > 1 {
				leTo(t, m, le, tt.remove[0])
			}
			press(m, "d")
			assert.Equal(t, lpBrowse, le.phase, "the removal does not start")
			assert.True(t, le.msgErr)
			assert.Contains(t, le.msg, tt.want)
			assert.Equal(t, before, leNames(le, ""))
		})
	}
	t.Run("a second won stage makes the first removable", func(t *testing.T) {
		m, _ := newTestModel(t)
		le := openLE(t, m, "deal-stages")
		press(m, "a")
		typeText(m, "signed")
		press(m, "enter")
		press(m, "tab", "enter", "right", "enter") // outcome open -> won
		it, _ := leItem(le, "signed")
		assert.Equal(t, "won", it.cells[0])
		leTo(t, m, le, "won")
		press(m, "d")
		assert.False(t, le.msgErr, le.msg)
		_, there := leItem(le, "won")
		assert.False(t, there)
	})
}

func TestListEditor_BulkSelectionRemoves(t *testing.T) {
	m, _ := newTestModel(t)
	le := openLE(t, m, "deal-roles")
	before := leNames(le, "")
	leTo(t, m, le, "signer")
	press(m, " ", " ") // signer, billing contact
	assert.Len(t, le.marked, 2)
	assert.Contains(t, m.screen(), "2 selected")
	leTo(t, m, le, "signer")
	press(m, "d")
	require.Equal(t, lpConfirm, le.phase, "several items ask once")
	assert.Contains(t, m.screen(), "Remove 2 deal roles")
	press(m, "n")
	assert.Equal(t, before, leNames(le, ""))
	press(m, "d", "y")
	assert.Equal(t, before[:len(before)-2], leNames(le, ""))
	assert.Empty(t, le.marked)
	press(m, "ctrl+z")
	assert.Equal(t, before, leNames(le, ""), "the whole bulk removal is one undo step")
}

func TestListEditor_BulkRemovalOfItemsInUse(t *testing.T) {
	ctx := context.Background()
	m, _ := newTestModel(t)
	for _, st := range []string{"lead", "prospect"} {
		_, err := m.dlr.Create(ctx, model.CreateDealInput{Title: st, Stage: st})
		require.NoError(t, err)
	}
	le := openLE(t, m, "deal-stages")
	leTo(t, m, le, "lead")
	press(m, " ", " ") // lead, prospect
	leTo(t, m, le, "lead")
	press(m, "d")
	require.Equal(t, lpPick, le.phase)
	assert.Contains(t, m.screen(), "2 deals use lead, prospect")
	assert.Equal(t, "Move them to proposal", le.pick[0].label, "the selected stages are not offered as destinations")
	press(m, "enter", "y")
	assert.Equal(t, 2, dealCount(t, m, "proposal"))
	press(m, "ctrl+z")
	assert.Equal(t, 1, dealCount(t, m, "lead"))
	assert.Equal(t, 1, dealCount(t, m, "prospect"))
}

func TestListEditor_NoOtherItemToMoveTo(t *testing.T) {
	ctx := context.Background()
	m, _ := newTestModel(t)
	for _, st := range []string{"lead"} {
		_, err := m.dlr.Create(ctx, model.CreateDealInput{Title: st, Stage: st})
		require.NoError(t, err)
	}
	le := openLE(t, m, "deal-stages")
	leTo(t, m, le, "lead")
	press(m, "ctrl+a") // every stage selected
	press(m, "d")
	assert.True(t, le.msgErr)
	assert.Equal(t, lpBrowse, le.phase)
}

func TestListEditor_CellsAcrossTheRow(t *testing.T) {
	t.Run("stage outcome with tab and enter", func(t *testing.T) {
		m, _ := newTestModel(t)
		le := openLE(t, m, "deal-stages")
		leTo(t, m, le, "negotiation")
		press(m, "tab")
		assert.Equal(t, 1, le.col)
		assert.Contains(t, m.screen(), "[open]", "the active cell is marked")
		press(m, "enter")
		require.Equal(t, lpCell, le.phase)
		press(m, "right", "right")
		press(m, "enter")
		require.Equal(t, lpBrowse, le.phase, le.msg)
		out, err := m.vr.List(m.ctx, model.DealStageKind, false)
		require.NoError(t, err)
		for _, e := range out {
			if e.Name == "negotiation" {
				assert.Equal(t, "lost", e.Outcome)
			}
		}
		press(m, "ctrl+z")
		it, _ := leItem(le, "negotiation")
		assert.Equal(t, "open", it.cells[0])
		press(m, "shift+tab")
		assert.Equal(t, 0, le.col, "shift+tab moves back to the name")
	})
	t.Run("an outcome change that breaks the rule is refused", func(t *testing.T) {
		m, _ := newTestModel(t)
		le := openLE(t, m, "deal-stages")
		leTo(t, m, le, "won")
		press(m, "tab", "enter", "left", "enter") // won -> open leaves no won stage
		assert.True(t, le.msgErr)
		assert.Contains(t, le.msg, "no won stage")
	})
	t.Run("note type counts as contact", func(t *testing.T) {
		m, _ := newTestModel(t)
		le := openLE(t, m, "note-types")
		leTo(t, m, le, "call")
		it := leCur(t, le)
		require.Equal(t, "yes", it.cells[0])
		press(m, "tab", "enter", "right", "enter")
		nt, err := m.ntr.FindByName(m.ctx, "call")
		require.NoError(t, err)
		assert.False(t, nt.CountsAsContact)
		press(m, "ctrl+z")
		nt, _ = m.ntr.FindByName(m.ctx, "call")
		assert.True(t, nt.CountsAsContact)
	})
	t.Run("relation kind inverse name and mutual", func(t *testing.T) {
		m, _ := newTestModel(t)
		le := openLE(t, m, "relation-kinds")
		press(m, "a")
		typeText(m, "client")
		press(m, "enter")
		it := leCur(t, le)
		require.Equal(t, "client", it.name)
		assert.Equal(t, "yes", it.cells[1], "a new kind is mutual until it has an inverse name")
		press(m, "tab", "enter")
		require.Equal(t, lpCell, le.phase)
		backspaces(m, 10)
		typeText(m, "vendor")
		press(m, "enter")
		require.Equal(t, lpBrowse, le.phase, le.msg)
		it = leCur(t, le)
		assert.Equal(t, "vendor", it.cells[0])
		assert.Equal(t, "no", it.cells[1], "an inverse name makes it directional")
		k, _, err := repoKinds(m).Resolve(m.ctx, "person", "vendor")
		require.NoError(t, err)
		assert.Equal(t, "client", k.Name)
		// The mutual cell toggles back to mutual.
		press(m, "tab", "enter", "left", "enter")
		it = leCur(t, le)
		assert.Equal(t, "yes", it.cells[1])
		assert.Equal(t, "client", it.cells[0])
		press(m, "ctrl+z")
		it = leCur(t, le)
		assert.Equal(t, "vendor", it.cells[0])
	})
	t.Run("a built-in kind refuses cell edits", func(t *testing.T) {
		m, _ := newTestModel(t)
		le := openLE(t, m, "relation-kinds")
		leTo(t, m, le, "parent")
		press(m, "tab", "enter")
		if le.phase == lpCell {
			backspaces(m, 10)
			typeText(m, "x")
			press(m, "enter")
		}
		assert.True(t, le.msgErr)
		assert.Contains(t, le.msg, "built-in")
	})
	t.Run("lists without cells ignore tab", func(t *testing.T) {
		m, _ := newTestModel(t)
		le := openLE(t, m, "deal-roles")
		press(m, "tab")
		assert.Equal(t, 0, le.col)
	})
}

func TestListEditor_ReadOnlyBlocksWrites(t *testing.T) {
	m, _ := newTestModel(t)
	m.readOnly = true
	le := openLE(t, m, "deal-roles")
	before := leNames(le, "")
	for _, k := range []string{"a", "enter", "d", "shift+down"} {
		press(m, k)
		assert.Equal(t, lpBrowse, le.phase, k)
	}
	assert.True(t, le.msgErr)
	assert.Equal(t, before, leNames(le, ""))
}

func TestListEditor_PaletteOpensEachList(t *testing.T) {
	for _, id := range listEditorIDs {
		t.Run(id, func(t *testing.T) {
			m, _ := newTestModel(t)
			runCmds(m, runID(m, id))
			require.NotNil(t, m.ov)
			require.NotNil(t, m.ov.le, "the palette action opens the list editor")
			assert.Equal(t, id, m.ov.le.d.id)
		})
	}
	t.Run("the separate Manage actions are gone", func(t *testing.T) {
		m, _ := newTestModel(t)
		for _, a := range m.actions {
			assert.False(t, strings.HasPrefix(a.title, "Manage deal"), a.title)
			assert.NotEqual(t, "Manage note types", a.title)
		}
	})
}

func TestListEditor_DrawsRowsWithCells(t *testing.T) {
	m, _ := newTestModel(t)
	openLE(t, m, "deal-stages")
	screen := m.screen()
	for _, want := range []string{"Deal stages", "outcome", "lead", "negotiation", "won", "not used"} {
		assert.Contains(t, screen, want)
	}
}

func TestListEditor_MouseClickMovesTheCursor(t *testing.T) {
	m, _ := newTestModel(t)
	le := openLE(t, m, "deal-roles")
	m.handleMouse(mouseEvent{X: 5, Y: overlayTop + 1 + 2, Button: tea.MouseLeft})
	require.NotNil(t, m.ov, "a click keeps the editor open")
	assert.Equal(t, 2, m.ov.cursor)
	assert.Same(t, le, m.ov.le, "the state of the editor is kept")
}

func repoKinds(m *Model) *repo.KindRepo { return repo.NewKindRepo(m.db) }
