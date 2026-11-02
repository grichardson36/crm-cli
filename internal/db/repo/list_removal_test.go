package repo_test

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveMany_DealStages(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		remove  []string
		act     repo.VocabRemoveAction
		wantErr string
		moved   int
		left    int
	}{
		{"unused stage goes", []string{"negotiation"}, repo.VocabRemoveAction{}, "", 0, 5},
		{"used stage moves", []string{"lead"}, repo.VocabRemoveAction{ReplaceWith: "prospect"}, "", 2, 5},
		{"used stage without a target is refused", []string{"lead"}, repo.VocabRemoveAction{}, "E_VOCAB_IN_USE", 0, 6},
		{"stages cannot be blank", []string{"lead"}, repo.VocabRemoveAction{Blank: true}, "cannot be blank", 0, 6},
		{"last won stage blocks", []string{"won"}, repo.VocabRemoveAction{ReplaceWith: "lead"}, "E_STAGE_NO_WON", 0, 6},
		{"two stages and the rule checked on what remains", []string{"won", "lost"}, repo.VocabRemoveAction{ReplaceWith: "lead"}, "E_STAGE_NO_WON", 0, 6},
		{"closed stage moved to an open one reopens the deal", []string{"lost"}, repo.VocabRemoveAction{ReplaceWith: "lead"}, "E_STAGE_NO_LOST", 0, 6},
		{"replacement cannot be removed too", []string{"lead", "prospect"}, repo.VocabRemoveAction{ReplaceWith: "lead"}, "must be a stage that stays", 0, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newStageEnv(t)
			e.deal(t, "a", "lead")
			e.deal(t, "b", "lead")
			rem, err := e.vr.RemoveMany(ctx, model.DealStageKind, tt.remove, tt.act)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Len(t, e.stages(t), tt.left, "a refused removal changes nothing")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.moved, rem.Moved)
			assert.Len(t, e.stages(t), tt.left)
			require.NoError(t, e.vr.UndoRemoval(ctx, rem))
			assert.Len(t, e.stages(t), 6, "undo brings every stage back")
		})
	}
}

func TestRemoveMany_StageUndoRestoresCloseDates(t *testing.T) {
	ctx := context.Background()
	e := newStageEnv(t)
	d := e.deal(t, "closed", "won")
	var closed string
	require.NoError(t, e.db.QueryRow(`SELECT COALESCE(closed_at, '') FROM deals WHERE id = ?`, d.ID).Scan(&closed))
	require.NotEmpty(t, closed)
	// Add a second won stage so "won" may go.
	_, err := e.vr.Add(ctx, model.DealStageKind, "signed")
	require.NoError(t, err)
	_, err = e.vr.SetOutcome(ctx, model.DealStageKind, "signed", model.OutcomeWon)
	require.NoError(t, err)
	rem, err := e.vr.RemoveMany(ctx, model.DealStageKind, []string{"won"}, repo.VocabRemoveAction{ReplaceWith: "lead"})
	require.NoError(t, err)
	var stage, closedAfter string
	require.NoError(t, e.db.QueryRow(`SELECT stage, COALESCE(closed_at, '') FROM deals WHERE id = ?`, d.ID).Scan(&stage, &closedAfter))
	assert.Equal(t, "lead", stage)
	assert.Empty(t, closedAfter, "an open stage reopens the deal")
	require.NoError(t, e.vr.UndoRemoval(ctx, rem))
	require.NoError(t, e.db.QueryRow(`SELECT stage, COALESCE(closed_at, '') FROM deals WHERE id = ?`, d.ID).Scan(&stage, &closedAfter))
	assert.Equal(t, "won", stage)
	assert.Equal(t, closed, closedAfter, "undo restores the close date")
	require.NoError(t, e.vr.RedoRemoval(ctx, rem))
	require.NoError(t, e.db.QueryRow(`SELECT stage FROM deals WHERE id = ?`, d.ID).Scan(&stage))
	assert.Equal(t, "lead", stage, "redo repeats it")
}

func TestRemoveMany_DealRoles(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		act     repo.VocabRemoveAction
		wantErr bool
		role    any
	}{
		{"blank", repo.VocabRemoveAction{Blank: true}, false, nil},
		{"move", repo.VocabRemoveAction{ReplaceWith: "signer"}, false, "signer"},
		{"nothing chosen", repo.VocabRemoveAction{}, true, "champion"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := setupLinks(t)
			d := f.links.DB()
			vr := repo.NewVocabRepo(d)
			_, err := d.Exec(`INSERT INTO deal_person (uuid, deal_id, person_id, role) VALUES ('u1', ?, ?, 'champion')`, f.deal, f.person)
			require.NoError(t, err)
			rem, err := vr.RemoveMany(ctx, roleKind, []string{"champion"}, tt.act)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			var role any
			require.NoError(t, d.QueryRow(`SELECT role FROM deal_person WHERE uuid = 'u1'`).Scan(&role))
			assert.Equal(t, tt.role, role)
			require.NoError(t, vr.UndoRemoval(ctx, rem))
			require.NoError(t, d.QueryRow(`SELECT role FROM deal_person WHERE uuid = 'u1'`).Scan(&role))
			assert.Equal(t, "champion", role)
			assert.Contains(t, names(t, vr), "champion")
		})
	}
}

func TestNoteTypes_ListEditorOps(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	nr := repo.NewNoteTypeRepo(d)
	ir := repo.NewInteractionRepo(d)
	pr := repo.NewPersonRepo(d)
	p, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Ann"})
	require.NoError(t, err)
	for _, ty := range []string{"call", "call", "email"} {
		_, err := ir.Create(ctx, model.CreateInteractionInput{Type: ty, PersonIDs: []int64{p.ID}})
		require.NoError(t, err)
	}
	typeNames := func() []string {
		l, err := nr.List(ctx, false)
		require.NoError(t, err)
		var out []string
		for _, x := range l {
			out = append(out, x.Name)
		}
		return out
	}
	assert.Equal(t, []string{"call", "email", "meeting", "message", "note"}, typeNames(), "today's order is kept by the migration")

	t.Run("counts", func(t *testing.T) {
		c, err := nr.Counts(ctx)
		require.NoError(t, err)
		assert.Equal(t, 2, c["call"])
		assert.Equal(t, 1, c["email"])
	})
	t.Run("reorder", func(t *testing.T) {
		require.NoError(t, nr.Reorder(ctx, []string{"note", "call"}))
		assert.Equal(t, []string{"note", "call", "email", "meeting", "message"}, typeNames())
	})
	t.Run("add goes last", func(t *testing.T) {
		_, err := nr.Add(ctx, "demo", false)
		require.NoError(t, err)
		l := typeNames()
		assert.Equal(t, "demo", l[len(l)-1])
	})
	t.Run("rename rewrites notes", func(t *testing.T) {
		res, err := nr.Rename(ctx, "call", "phone")
		require.NoError(t, err)
		assert.Equal(t, 2, res.Rows)
		_, err = nr.Rename(ctx, "note", "memo")
		assert.ErrorIs(t, err, model.ErrConflict)
		_, err = nr.Rename(ctx, "email", "phone")
		assert.ErrorIs(t, err, model.ErrConflict)
	})
	t.Run("remove with move and undo", func(t *testing.T) {
		rem, err := nr.RemoveMany(ctx, []string{"phone"}, "email")
		require.NoError(t, err)
		assert.Equal(t, 2, rem.Moved)
		c, _ := nr.Counts(ctx)
		assert.Equal(t, 3, c["email"])
		require.NoError(t, nr.UndoRemoval(ctx, rem))
		c, _ = nr.Counts(ctx)
		assert.Equal(t, 2, c["phone"])
		assert.Equal(t, 1, c["email"])
		require.NoError(t, nr.RedoRemoval(ctx, rem))
		c, _ = nr.Counts(ctx)
		assert.Equal(t, 3, c["email"])
		require.NoError(t, nr.UndoRemoval(ctx, rem))
	})
	t.Run("remove keeping the notes", func(t *testing.T) {
		rem, err := nr.RemoveMany(ctx, []string{"phone"}, "")
		require.NoError(t, err)
		assert.Equal(t, 2, rem.Kept)
		c, _ := nr.Counts(ctx)
		assert.Equal(t, 2, c["phone"], "old notes keep the type")
	})
	t.Run("the default type stays", func(t *testing.T) {
		_, err := nr.RemoveMany(ctx, []string{"note"}, "")
		assert.ErrorIs(t, err, model.ErrConflict)
	})
	t.Run("counts as contact", func(t *testing.T) {
		require.NoError(t, nr.SetCountsAsContact(ctx, "demo", true))
		l, _ := nr.List(ctx, false)
		for _, x := range l {
			if x.Name == "demo" {
				assert.True(t, x.CountsAsContact)
			}
		}
	})
}

func TestKinds_ListEditorOps(t *testing.T) {
	ctx := context.Background()
	f := setupLinks(t)
	d := f.links.DB()
	kr := repo.NewKindRepo(d)
	friend, _, err := kr.Resolve(ctx, "person", "friend")
	require.NoError(t, err)
	_, err = kr.Add(ctx, "person", "client", "vendor")
	require.NoError(t, err)
	_, err = kr.Add(ctx, "person", "pal", "")
	require.NoError(t, err)
	client, _, _ := kr.Resolve(ctx, "person", "client")
	pal, _, _ := kr.Resolve(ctx, "person", "pal")
	link := func(a, b, kind int64) {
		_, err := d.Exec(`INSERT INTO person_relation (uuid, a_id, b_id, kind_id) VALUES (lower(hex(randomblob(8))), ?, ?, ?)`, a, b, kind)
		require.NoError(t, err)
	}
	live := func(kind int64) int {
		var n int
		require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM person_relation WHERE kind_id = ? AND deleted_at IS NULL`, kind).Scan(&n))
		return n
	}
	names := func() []string {
		l, err := kr.List(ctx, "person")
		require.NoError(t, err)
		var out []string
		for _, k := range l {
			out = append(out, k.Name)
		}
		return out
	}

	t.Run("order and new kinds go last", func(t *testing.T) {
		n := names()
		assert.Equal(t, "related", n[0])
		assert.Equal(t, []string{"client", "pal"}, n[len(n)-2:])
		require.NoError(t, kr.Reorder(ctx, "person", []string{"pal", "client"}))
		n = names()
		assert.Equal(t, []string{"pal", "client", "related"}, n[:3])
	})
	t.Run("update name and inverse", func(t *testing.T) {
		inv := "supplier"
		k, err := kr.Update(ctx, "person", "client", repo.KindPatch{Inverse: &inv})
		require.NoError(t, err)
		assert.Equal(t, "supplier", k.InverseName)
		nm := "customer"
		k, err = kr.Update(ctx, "person", "client", repo.KindPatch{Name: &nm})
		require.NoError(t, err)
		assert.Equal(t, "customer", k.Name)
		_, err = kr.Update(ctx, "person", "customer", repo.KindPatch{Name: &nm, Inverse: &nm})
		require.NoError(t, err)
		back := "client"
		_, err = kr.Update(ctx, "person", "customer", repo.KindPatch{Name: &back, Inverse: &inv})
		require.NoError(t, err)
		_, err = kr.Update(ctx, "person", "friend", repo.KindPatch{Name: &back})
		assert.ErrorIs(t, err, model.ErrConflict, "built-in kinds stay")
		clash := "spouse"
		_, err = kr.Update(ctx, "person", "client", repo.KindPatch{Name: &clash})
		assert.ErrorIs(t, err, model.ErrConflict, "a name that is taken")
	})
	t.Run("direction cannot change while links use the kind", func(t *testing.T) {
		link(f.person, f.other, client.ID)
		none := ""
		_, err := kr.Update(ctx, "person", "client", repo.KindPatch{Inverse: &none})
		assert.ErrorIs(t, err, model.ErrConflict)
	})
	t.Run("remove in use needs a decision", func(t *testing.T) {
		_, err := kr.RemoveMany(ctx, "person", []string{"client"}, repo.KindRemoveAction{})
		assert.ErrorIs(t, err, model.ErrConflict)
		_, err = kr.RemoveMany(ctx, "person", []string{"friend"}, repo.KindRemoveAction{Unlink: true})
		assert.ErrorIs(t, err, model.ErrConflict, "built-in kinds stay")
	})
	t.Run("move to another kind and undo", func(t *testing.T) {
		counts, _ := kr.Counts(ctx, "person")
		assert.Equal(t, 1, counts[client.ID])
		rem, err := kr.RemoveMany(ctx, "person", []string{"client"}, repo.KindRemoveAction{MoveTo: "pal"})
		require.NoError(t, err)
		assert.Equal(t, 1, rem.Moved)
		assert.Equal(t, 1, live(pal.ID))
		assert.NotContains(t, names(), "client")
		require.NoError(t, kr.UndoRemoval(ctx, rem))
		assert.Equal(t, 1, live(client.ID))
		assert.Equal(t, 0, live(pal.ID))
		assert.Contains(t, names(), "client")
	})
	t.Run("move onto a kind that already links the pair is refused", func(t *testing.T) {
		link(f.person, f.other, friend.ID)
		_, err := kr.RemoveMany(ctx, "person", []string{"client"}, repo.KindRemoveAction{MoveTo: "friend"})
		assert.ErrorIs(t, err, model.ErrConflict)
		assert.Equal(t, 1, live(client.ID), "nothing changed")
	})
	t.Run("unlink and undo", func(t *testing.T) {
		rem, err := kr.RemoveMany(ctx, "person", []string{"client"}, repo.KindRemoveAction{Unlink: true})
		require.NoError(t, err)
		assert.Equal(t, 1, rem.Unlinked)
		assert.Equal(t, 0, live(client.ID))
		require.NoError(t, kr.UndoRemoval(ctx, rem))
		assert.Equal(t, 1, live(client.ID))
		require.NoError(t, kr.RedoRemoval(ctx, rem))
		assert.Equal(t, 0, live(client.ID))
	})
}
