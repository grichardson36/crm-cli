package repo_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/validate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stageEnv struct {
	db *sql.DB
	dr *repo.DealRepo
	vr *repo.VocabRepo
}

func newStageEnv(t *testing.T) stageEnv {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return stageEnv{db: d, dr: repo.NewDealRepo(d), vr: repo.NewVocabRepo(d)}
}

func (e stageEnv) deal(t *testing.T, title, stage string) *model.Deal {
	t.Helper()
	d, err := e.dr.Create(context.Background(), model.CreateDealInput{Title: title, Stage: stage})
	require.NoError(t, err)
	return d
}

func (e stageEnv) stages(t *testing.T) []string {
	t.Helper()
	n, err := e.vr.Names(context.Background(), model.DealStageKind)
	require.NoError(t, err)
	return n
}

func issueCode(t *testing.T, err error) string {
	t.Helper()
	require.Error(t, err)
	e, ok := validate.As(err)
	require.True(t, ok, err)
	return e.Issues[0].Code
}

func TestDealStages_SeededOrderAndOutcomes(t *testing.T) {
	e := newStageEnv(t)
	assert.Equal(t, []string{"lead", "prospect", "proposal", "negotiation", "won", "lost"}, e.stages(t))
	list, err := e.vr.List(context.Background(), model.DealStageKind, false)
	require.NoError(t, err)
	got := map[string]string{}
	for _, s := range list {
		got[s.Name] = s.Outcome
	}
	assert.Equal(t, map[string]string{"lead": "open", "prospect": "open", "proposal": "open", "negotiation": "open", "won": "won", "lost": "lost"}, got)
}

func TestDealStages_DealsUseTheList(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		prepare func(t *testing.T, e stageEnv)
		stage   string
		want    string
		code    string
	}{
		{"empty stage is the first open stage", nil, "", "lead", ""},
		{"case is corrected to the listed spelling", nil, "PROPOSAL", "proposal", ""},
		{"unknown stage", nil, "qualified", "", validate.CodeVocabUnknown},
		{"a stage added to the list is accepted", func(t *testing.T, e stageEnv) {
			_, err := e.vr.Add(ctx, model.DealStageKind, "qualified")
			require.NoError(t, err)
		}, "Qualified", "qualified", ""},
		{"the first open stage follows the order", func(t *testing.T, e stageEnv) {
			require.NoError(t, e.vr.Move(ctx, model.DealStageKind, "negotiation", 1))
		}, "", "negotiation", ""},
		{"a renamed stage replaces the old name", func(t *testing.T, e stageEnv) {
			_, err := e.vr.Rename(ctx, model.DealStageKind, "lead", "inbound")
			require.NoError(t, err)
		}, "lead", "", validate.CodeVocabUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newStageEnv(t)
			if tt.prepare != nil {
				tt.prepare(t, e)
			}
			d, err := e.dr.Create(ctx, model.CreateDealInput{Title: "D", Stage: tt.stage})
			if tt.code != "" {
				assert.Equal(t, tt.code, issueCode(t, err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, d.Stage)
		})
	}
	t.Run("update checks the stage and an empty stage is required", func(t *testing.T) {
		e := newStageEnv(t)
		d := e.deal(t, "D", "")
		bad, empty := "zzz", ""
		_, err := e.dr.Update(ctx, d.ID, model.UpdateDealInput{Stage: &bad})
		assert.Equal(t, validate.CodeVocabUnknown, issueCode(t, err))
		_, err = e.dr.Update(ctx, d.ID, model.UpdateDealInput{Stage: &empty})
		assert.Equal(t, validate.CodeFieldRequired, issueCode(t, err))
	})
}

func TestDealStages_RenamePropagatesToDeals(t *testing.T) {
	ctx := context.Background()
	e := newStageEnv(t)
	a := e.deal(t, "A", "proposal")
	b := e.deal(t, "B", "proposal")
	c := e.deal(t, "C", "lead")
	res, err := e.vr.Rename(ctx, model.DealStageKind, "proposal", "quote sent")
	require.NoError(t, err)
	assert.Equal(t, 2, res.Rows)
	for id, want := range map[int64]string{a.ID: "quote sent", b.ID: "quote sent", c.ID: "lead"} {
		d, err := e.dr.FindByID(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, want, d.Stage)
	}
	assert.Equal(t, []string{"lead", "prospect", "quote sent", "negotiation", "won", "lost"}, e.stages(t), "the position stays")
	_, err = e.vr.Rename(ctx, model.DealStageKind, "lead", "WON")
	assert.Equal(t, validate.CodeVocabDuplicate, issueCode(t, err))
}

func TestDealStages_RemoveNeedsReplacementWhenUsed(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name        string
		remove      string
		replaceWith string
		keep        bool
		code        string
		moved       int
	}{
		{"unused stage goes without a replacement", "prospect", "", false, "", 0},
		{"used stage without a replacement", "lead", "", false, validate.CodeVocabInUse, 0},
		{"keep is not available for stages", "lead", "", true, validate.CodeVocabInUse, 0},
		{"used stage with a replacement", "lead", "prospect", false, "", 2},
		{"the replacement must be listed", "lead", "ghost", false, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newStageEnv(t)
			e.deal(t, "A", "lead")
			e.deal(t, "B", "lead")
			res, err := e.vr.Remove(ctx, model.DealStageKind, tt.remove, tt.replaceWith, tt.keep)
			switch {
			case tt.code != "":
				assert.Equal(t, tt.code, issueCode(t, err))
				assert.Contains(t, e.stages(t), tt.remove, "nothing changed")
			case tt.replaceWith == "ghost":
				assert.ErrorIs(t, err, model.ErrNotFound)
			default:
				require.NoError(t, err)
				assert.Equal(t, tt.moved, res.Moved)
				assert.NotContains(t, e.stages(t), tt.remove)
				if tt.moved > 0 {
					deals, err := e.dr.FindAll(ctx, model.DealFilters{})
					require.NoError(t, err)
					for _, d := range deals {
						assert.Equal(t, tt.replaceWith, d.Stage)
					}
				}
			}
		})
	}
}

func TestDealStages_ClosedAtFollowsTheOutcome(t *testing.T) {
	ctx := context.Background()
	move := func(t *testing.T, e stageEnv, id int64, stage string) *model.Deal {
		t.Helper()
		d, err := e.dr.Update(ctx, id, model.UpdateDealInput{Stage: &stage})
		require.NoError(t, err)
		return d
	}
	t.Run("default stages", func(t *testing.T) {
		e := newStageEnv(t)
		d := e.deal(t, "D", "lead")
		assert.Nil(t, d.ClosedAt)
		assert.NotNil(t, move(t, e, d.ID, "won").ClosedAt)
		assert.NotNil(t, move(t, e, d.ID, "lost").ClosedAt, "won to lost stamps again")
		assert.Nil(t, move(t, e, d.ID, "proposal").ClosedAt, "reopening clears it")
		assert.NotNil(t, e.deal(t, "starts won", "won").ClosedAt)
	})
	t.Run("a renamed won stage still closes", func(t *testing.T) {
		e := newStageEnv(t)
		_, err := e.vr.Rename(ctx, model.DealStageKind, "won", "signed")
		require.NoError(t, err)
		d := e.deal(t, "D", "lead")
		assert.NotNil(t, move(t, e, d.ID, "signed").ClosedAt)
		assert.NotNil(t, e.deal(t, "starts signed", "signed").ClosedAt)
	})
	t.Run("a stage called won that is open does not close", func(t *testing.T) {
		e := newStageEnv(t)
		_, err := e.vr.Add(ctx, model.DealStageKind, "closed won")
		require.NoError(t, err)
		_, err = e.vr.SetOutcome(ctx, model.DealStageKind, "closed won", "won")
		require.NoError(t, err)
		_, err = e.vr.SetOutcome(ctx, model.DealStageKind, "won", "open")
		require.NoError(t, err)
		d := e.deal(t, "D", "lead")
		assert.Nil(t, move(t, e, d.ID, "won").ClosedAt, "the name won means nothing, the outcome does")
		assert.NotNil(t, move(t, e, d.ID, "closed won").ClosedAt)
	})
	t.Run("a new lost stage closes and an explicit closed_at wins", func(t *testing.T) {
		e := newStageEnv(t)
		_, err := e.vr.Add(ctx, model.DealStageKind, "no budget")
		require.NoError(t, err)
		_, err = e.vr.SetOutcome(ctx, model.DealStageKind, "no budget", "lost")
		require.NoError(t, err)
		d := e.deal(t, "D", "lead")
		assert.NotNil(t, move(t, e, d.ID, "no budget").ClosedAt)
		stage, at := "negotiation", "2026-01-05"
		up, err := e.dr.Update(ctx, d.ID, model.UpdateDealInput{Stage: &stage, ClosedAt: &at})
		require.NoError(t, err)
		require.NotNil(t, up.ClosedAt)
		assert.Equal(t, at, *up.ClosedAt)
	})
	t.Run("removing a stage with a replacement follows the outcomes", func(t *testing.T) {
		e := newStageEnv(t)
		_, err := e.vr.Add(ctx, model.DealStageKind, "won again")
		require.NoError(t, err)
		_, err = e.vr.SetOutcome(ctx, model.DealStageKind, "won again", "won")
		require.NoError(t, err)
		open := e.deal(t, "open", "lead")
		won := e.deal(t, "won", "won")
		_, err = e.db.Exec(`UPDATE deals SET closed_at = '2026-01-02 03:04:05' WHERE id = ?`, won.ID)
		require.NoError(t, err)
		_, err = e.vr.Remove(ctx, model.DealStageKind, "won", "won again", false)
		require.NoError(t, err)
		got, _ := e.dr.FindByID(ctx, won.ID)
		assert.Equal(t, "won again", got.Stage)
		require.NotNil(t, got.ClosedAt)
		assert.Equal(t, "2026-01-02 03:04:05", *got.ClosedAt, "same outcome keeps the date")
		_, err = e.vr.Remove(ctx, model.DealStageKind, "lead", "won again", false)
		require.NoError(t, err)
		got, _ = e.dr.FindByID(ctx, open.ID)
		assert.NotNil(t, got.ClosedAt, "an open deal moved into a won stage is stamped")
	})
}

func TestDealStages_Rules(t *testing.T) {
	ctx := context.Background()
	kind := model.DealStageKind
	tests := []struct {
		name string
		run  func(e stageEnv) error
		code string
	}{
		{"remove the only won stage", func(e stageEnv) error { _, err := e.vr.Remove(ctx, kind, "won", "", false); return err }, validate.CodeStageNoWon},
		{"remove the only lost stage", func(e stageEnv) error { _, err := e.vr.Remove(ctx, kind, "lost", "", false); return err }, validate.CodeStageNoLost},
		{"mark the only won stage open", func(e stageEnv) error { _, err := e.vr.SetOutcome(ctx, kind, "won", "open"); return err }, validate.CodeStageNoWon},
		{"mark the only lost stage won", func(e stageEnv) error { _, err := e.vr.SetOutcome(ctx, kind, "lost", "won"); return err }, validate.CodeStageNoLost},
		{"outcome must be open, won or lost", func(e stageEnv) error { _, err := e.vr.SetOutcome(ctx, kind, "lead", "closed"); return err }, validate.CodeStageOutcome},
		{"remove every open stage", func(e stageEnv) error {
			for _, n := range []string{"lead", "prospect", "proposal"} {
				if _, err := e.vr.Remove(ctx, kind, n, "", false); err != nil {
					return err
				}
			}
			_, err := e.vr.Remove(ctx, kind, "negotiation", "", false)
			return err
		}, validate.CodeStageNoOpen},
		{"mark every open stage won", func(e stageEnv) error {
			for _, n := range []string{"lead", "prospect", "proposal", "negotiation"} {
				if _, err := e.vr.SetOutcome(ctx, kind, n, "won"); err != nil {
					return err
				}
			}
			return nil
		}, validate.CodeStageNoOpen},
		{"a second won stage lets the first become open", func(e stageEnv) error {
			if _, err := e.vr.Add(ctx, kind, "signed"); err != nil {
				return err
			}
			if _, err := e.vr.SetOutcome(ctx, kind, "signed", "won"); err != nil {
				return err
			}
			_, err := e.vr.SetOutcome(ctx, kind, "won", "open")
			return err
		}, ""},
		{"setting the same outcome is a no-op", func(e stageEnv) error { _, err := e.vr.SetOutcome(ctx, kind, "won", "WON"); return err }, ""},
		{"unknown stage", func(e stageEnv) error { _, err := e.vr.SetOutcome(ctx, kind, "ghost", "open"); return err }, "notfound"},
		{"deal roles have no outcome", func(e stageEnv) error { _, err := e.vr.SetOutcome(ctx, model.DealRoleKind, "champion", "won"); return err }, "validation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newStageEnv(t)
			err := tt.run(e)
			switch tt.code {
			case "":
				assert.NoError(t, err)
			case "notfound":
				assert.ErrorIs(t, err, model.ErrNotFound)
			case "validation":
				assert.ErrorIs(t, err, model.ErrValidation)
			default:
				assert.Equal(t, tt.code, issueCode(t, err))
				assert.ErrorIs(t, err, model.ErrValidation)
			}
		})
	}
	t.Run("a rule failure changes nothing", func(t *testing.T) {
		e := newStageEnv(t)
		_, err := e.vr.Remove(ctx, kind, "won", "", false)
		require.Error(t, err)
		assert.Contains(t, e.stages(t), "won")
	})
}

func TestDealStages_OutcomeStampsCloseDateOfDealsAlreadyThere(t *testing.T) {
	ctx := context.Background()
	e := newStageEnv(t)
	d := e.deal(t, "D", "negotiation")
	res, err := e.vr.SetOutcome(ctx, model.DealStageKind, "negotiation", "lost")
	require.NoError(t, err)
	assert.Equal(t, 1, res.Closed)
	got, _ := e.dr.FindByID(ctx, d.ID)
	assert.NotNil(t, got.ClosedAt)
}

func TestDealStages_PipelineFollowsTheListOrderAndOutcome(t *testing.T) {
	ctx := context.Background()
	e := newStageEnv(t)
	e.deal(t, "A", "won")
	e.deal(t, "B", "lead")
	e.deal(t, "C", "negotiation")
	e.deal(t, "D", "lead")
	stagesOf := func() []string {
		p, err := e.dr.Pipeline(ctx)
		require.NoError(t, err)
		var out []string
		for _, s := range p {
			out = append(out, s.Stage+":"+s.Outcome)
		}
		return out
	}
	assert.Equal(t, []string{"lead:open", "negotiation:open", "won:won"}, stagesOf())
	require.NoError(t, e.vr.Reorder(ctx, model.DealStageKind, []string{"won", "negotiation"}))
	assert.Equal(t, []string{"won:won", "negotiation:open", "lead:open"}, stagesOf())
	_, err := e.vr.Rename(ctx, model.DealStageKind, "negotiation", "talks")
	require.NoError(t, err)
	assert.Equal(t, []string{"won:won", "talks:open", "lead:open"}, stagesOf())
	p, _ := e.dr.Pipeline(ctx)
	assert.Equal(t, 2, p[2].Count)
}

func TestDealStages_OpenCountsUseOutcomes(t *testing.T) {
	ctx := context.Background()
	e := newStageEnv(t)
	_, err := e.vr.Rename(ctx, model.DealStageKind, "won", "signed")
	require.NoError(t, err)
	e.deal(t, "open", "lead")
	e.deal(t, "signed", "signed")
	e.deal(t, "lost", "lost")
	open, err := e.dr.FindAll(ctx, model.DealFilters{ExcludeClosed: true})
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, "open", open[0].Title)
	var n int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM deals WHERE archived = 0 AND `+repo.OpenDealCond("stage")).Scan(&n))
	assert.Equal(t, 1, n)
}
