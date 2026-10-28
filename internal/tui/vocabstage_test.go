package tui

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stageList(t *testing.T, m *Model) []string {
	t.Helper()
	n, err := m.vr.Names(context.Background(), model.DealStageKind)
	require.NoError(t, err)
	return n
}

func stageOf(t *testing.T, m *Model, id int64) *model.Deal {
	t.Helper()
	d, err := m.dlr.FindByID(context.Background(), id)
	require.NoError(t, err)
	return d
}

func TestStageCellReadsTheVocabulary(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name      string
		prepare   func(t *testing.T, m *Model)
		keys      func(m *Model)
		wantStage string
		wantClose bool
	}{
		{"typing filters and enter picks", nil, func(m *Model) { typeText(m, "prop"); press(m, "enter") }, "proposal", false},
		{"a stage added to the list is offered", func(t *testing.T, m *Model) {
			_, err := m.vr.Add(ctx, model.DealStageKind, "qualified")
			require.NoError(t, err)
		}, func(m *Model) { typeText(m, "qualif"); press(m, "enter") }, "qualified", false},
		{"a renamed won stage closes the deal by its outcome", func(t *testing.T, m *Model) {
			_, err := m.vr.Rename(ctx, model.DealStageKind, "won", "signed")
			require.NoError(t, err)
		}, func(m *Model) { typeText(m, "signed"); press(m, "enter") }, "signed", true},
		{"a stage called won that is open does not close", func(t *testing.T, m *Model) {
			_, err := m.vr.Add(ctx, model.DealStageKind, "closed won")
			require.NoError(t, err)
			_, err = m.vr.SetOutcome(ctx, model.DealStageKind, "closed won", "won")
			require.NoError(t, err)
			_, err = m.vr.SetOutcome(ctx, model.DealStageKind, "won", "open")
			require.NoError(t, err)
		}, func(m *Model) { typeText(m, "won"); press(m, "enter") }, "won", false},
		{"unknown text creates an open stage in the list", nil, func(m *Model) { typeText(m, "intro call"); press(m, "enter") }, "intro call", false},
		{"esc keeps the stage", nil, func(m *Model) { typeText(m, "lost"); press(m, "esc") }, "lead", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestModel(t)
			if tt.prepare != nil {
				tt.prepare(t, m)
			}
			d, err := m.dlr.Create(ctx, model.CreateDealInput{Title: "Site", Stage: "lead"})
			require.NoError(t, err)
			goDeals(m)
			openStageEditor(t, m)
			require.NotNil(t, m.edit)
			require.NotNil(t, m.edit.typed.vocab, "the stage is edited with the vocabulary editor")
			tt.keys(m)
			got := stageOf(t, m, d.ID)
			if tt.wantStage != "" {
				assert.Equal(t, tt.wantStage, got.Stage)
			}
			assert.Equal(t, tt.wantClose, got.ClosedAt != nil)
		})
	}
	t.Run("creating a stage in the cell is one undo step with the move", func(t *testing.T) {
		m, _ := newTestModel(t)
		d, _ := m.dlr.Create(ctx, model.CreateDealInput{Title: "Site", Stage: "lead"})
		goDeals(m)
		openStageEditor(t, m)
		typeText(m, "intro call")
		press(m, "enter")
		assert.Contains(t, stageList(t, m), "intro call")
		press(m, "ctrl+z")
		assert.NotContains(t, stageList(t, m), "intro call")
		assert.Equal(t, "lead", stageOf(t, m, d.ID).Stage)
	})
	t.Run("a stage cannot be cleared", func(t *testing.T) {
		m, _ := newTestModel(t)
		_, _ = m.dlr.Create(ctx, model.CreateDealInput{Title: "Site", Stage: "lead"})
		goDeals(m)
		openStageEditor(t, m)
		for _, it := range m.edit.typed.vocab.items {
			assert.False(t, it.clear, "no clear row")
		}
	})
}

func TestSetFieldOnSelectedReadsTheStageVocabulary(t *testing.T) {
	ctx := context.Background()
	m, _ := newTestModel(t)
	_, err := m.vr.Add(ctx, model.DealStageKind, "qualified")
	require.NoError(t, err)
	_, err = m.vr.Rename(ctx, model.DealStageKind, "won", "signed")
	require.NoError(t, err)
	a, _ := m.dlr.Create(ctx, model.CreateDealInput{Title: "A", Stage: "lead"})
	b, _ := m.dlr.Create(ctx, model.CreateDealInput{Title: "B", Stage: "lead"})
	goDeals(m)
	press(m, "ctrl+a")
	runID(m, "set-field")
	require.NotNil(t, m.ov)
	typeText(m, "stage")
	press(m, "enter")
	require.NotNil(t, m.ov, "the stage options come from the vocabulary")
	var labels []string
	for _, it := range m.ov.items {
		labels = append(labels, it.label)
	}
	assert.Equal(t, []string{"lead", "prospect", "proposal", "negotiation", "signed", "lost", "qualified"}, labels)
	typeText(m, "signed")
	press(m, "enter")
	for _, id := range []int64{a.ID, b.ID} {
		d := stageOf(t, m, id)
		assert.Equal(t, "signed", d.Stage)
		assert.NotNil(t, d.ClosedAt, "the outcome of the stage closes the deals")
	}
}

func TestDealAddLineTakesAListedStage(t *testing.T) {
	ctx := context.Background()
	m, _ := newTestModel(t)
	_, err := m.vr.Add(ctx, model.DealStageKind, "qualified")
	require.NoError(t, err)
	got := func() []*model.Deal {
		l, err := m.dlr.FindAll(ctx, model.DealFilters{})
		require.NoError(t, err)
		return l
	}
	_, err = m.saveDealAdd("Big | 100 | Qualified")
	require.NoError(t, err)
	_, err = m.saveDealAdd("Plain")
	require.NoError(t, err)
	_, err = m.saveDealAdd("Bad | | nowhere")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "E_VOCAB_UNKNOWN")
	stages := map[string]string{}
	for _, d := range got() {
		stages[d.Title] = d.Stage
	}
	assert.Equal(t, map[string]string{"Big": "qualified", "Plain": "lead"}, stages, "an empty stage is the first open stage")
}

func TestManageDealStagesFromThePalette(t *testing.T) {
	ctx := context.Background()
	newM := func(t *testing.T) *Model {
		m, _ := newTestModel(t)
		runID(m, "deal-stages")
		require.NotNil(t, m.ov, "the palette action opens the deal stages")
		assert.Equal(t, "Deal stages", m.ov.title)
		return m
	}
	t.Run("add a stage is one undo step", func(t *testing.T) {
		m := newM(t)
		before := stageList(t, m)
		pickOverlay(t, m, "Add a stage")
		require.Equal(t, modePrompt, m.mode)
		submit(m, "  intro   call ")
		assert.Equal(t, append(append([]string{}, before...), "intro call"), stageList(t, m))
		press(m, "esc")
		press(m, "ctrl+z")
		assert.Equal(t, before, stageList(t, m))
	})
	t.Run("a duplicate name is refused with its code", func(t *testing.T) {
		m := newM(t)
		pickOverlay(t, m, "Add a stage")
		submit(m, "LEAD")
		assert.True(t, m.statusIsErr)
		assert.Contains(t, m.status, "E_VOCAB_DUPLICATE")
	})
	t.Run("rename rewrites the deals and undoes", func(t *testing.T) {
		m, _ := newTestModel(t)
		d, _ := m.dlr.Create(ctx, model.CreateDealInput{Title: "Site", Stage: "proposal"})
		runID(m, "deal-stages")
		pickOverlay(t, m, "proposal")
		pickOverlay(t, m, "Rename")
		require.Equal(t, modePrompt, m.mode)
		submit(m, "quote sent")
		assert.Equal(t, "quote sent", stageOf(t, m, d.ID).Stage)
		press(m, "esc")
		press(m, "ctrl+z")
		assert.Equal(t, "proposal", stageOf(t, m, d.ID).Stage)
	})
	t.Run("set outcome changes what closes a deal and undoes", func(t *testing.T) {
		m, _ := newTestModel(t)
		d, _ := m.dlr.Create(ctx, model.CreateDealInput{Title: "Site", Stage: "negotiation"})
		runID(m, "deal-stages")
		pickOverlay(t, m, "negotiation")
		pickOverlay(t, m, "Set outcome")
		pickOverlay(t, m, "lost")
		list, _ := m.vr.List(ctx, model.DealStageKind, false)
		outcomes := map[string]string{}
		for _, e := range list {
			outcomes[e.Name] = e.Outcome
		}
		assert.Equal(t, "lost", outcomes["negotiation"])
		assert.NotNil(t, stageOf(t, m, d.ID).ClosedAt)
		press(m, "esc")
		press(m, "ctrl+z")
		list, _ = m.vr.List(ctx, model.DealStageKind, false)
		for _, e := range list {
			if e.Name == "negotiation" {
				assert.Equal(t, "open", e.Outcome)
			}
		}
	})
	t.Run("a rule violation shows its code", func(t *testing.T) {
		m := newM(t)
		pickOverlay(t, m, "won")
		pickOverlay(t, m, "Set outcome")
		pickOverlay(t, m, "open")
		assert.True(t, m.statusIsErr)
		assert.Contains(t, m.status, "E_STAGE_NO_WON")
	})
	t.Run("move down and undo", func(t *testing.T) {
		m := newM(t)
		pickOverlay(t, m, "lead")
		pickOverlay(t, m, "Move down")
		assert.Equal(t, []string{"prospect", "lead"}, stageList(t, m)[:2])
		press(m, "esc")
		press(m, "ctrl+z")
		assert.Equal(t, []string{"lead", "prospect"}, stageList(t, m)[:2])
	})
	t.Run("remove an unused stage asks y or n", func(t *testing.T) {
		m := newM(t)
		pickOverlay(t, m, "prospect")
		pickOverlay(t, m, "Remove")
		require.Equal(t, modeConfirm, m.mode)
		press(m, "y")
		assert.NotContains(t, stageList(t, m), "prospect")
	})
	t.Run("remove a used stage moves its deals to the stage picked", func(t *testing.T) {
		m, _ := newTestModel(t)
		d, _ := m.dlr.Create(ctx, model.CreateDealInput{Title: "Site", Stage: "prospect"})
		runID(m, "deal-stages")
		pickOverlay(t, m, "prospect")
		pickOverlay(t, m, "Remove")
		require.NotNil(t, m.ov)
		assert.Contains(t, m.ov.title, "1 deal(s) are in prospect")
		pickOverlay(t, m, "lead")
		assert.Equal(t, "lead", stageOf(t, m, d.ID).Stage)
		assert.NotContains(t, stageList(t, m), "prospect")
	})
}

// openStageEditor opens the editor of the stage row of the deal under the cursor.
func openStageEditor(t *testing.T, m *Model) {
	t.Helper()
	if m.focus != focusDetail {
		press(m, "tab")
	}
	focusLabel(t, m, "stage")
	press(m, "enter")
}
