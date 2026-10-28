package cli_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stageRow struct {
	Name    string `json:"name"`
	Outcome string `json:"outcome"`
	InUse   int    `json:"in_use"`
}

func stageRowsOf(t *testing.T, db string, extra ...string) []stageRow {
	t.Helper()
	stdout, stderr, code := crm(t, db, append([]string{"deal", "stages", "list", "-f", "json"}, extra...)...)
	require.Equal(t, 0, code, stderr)
	var rows []stageRow
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	return rows
}

func stageNames(t *testing.T, db string) []string {
	var out []string
	for _, r := range stageRowsOf(t, db) {
		out = append(out, r.Name)
	}
	return out
}

func TestDealStagesList_SeededWithOutcomes(t *testing.T) {
	db := filepath.Join(t.TempDir(), "crm.db")
	rows := stageRowsOf(t, db)
	require.Len(t, rows, 6)
	var got []string
	for _, r := range rows {
		got = append(got, r.Name+":"+r.Outcome)
	}
	assert.Equal(t, []string{"lead:open", "prospect:open", "proposal:open", "negotiation:open", "won:won", "lost:lost"}, got)
}

func TestDealStagesManage(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantErr  string
		wantHead []string
	}{
		{"add", []string{"add", "qualified"}, 0, "", nil},
		{"add duplicate", []string{"add", "LEAD"}, 2, "E_VOCAB_DUPLICATE", nil},
		{"add empty", []string{"add", " "}, 2, "E_VOCAB_INVALID", nil},
		{"rename", []string{"rename", "proposal", "quote sent"}, 0, "", []string{"lead", "prospect", "quote sent"}},
		{"rename onto another", []string{"rename", "lead", "won"}, 2, "E_VOCAB_DUPLICATE", nil},
		{"rename unknown", []string{"rename", "ghost", "x"}, 3, "", nil},
		{"remove unused", []string{"remove", "prospect"}, 0, "", []string{"lead", "proposal"}},
		{"remove unknown", []string{"remove", "ghost"}, 3, "", nil},
		{"remove the only won stage", []string{"remove", "won"}, 2, "E_STAGE_NO_WON", nil},
		{"remove the only lost stage", []string{"remove", "lost"}, 2, "E_STAGE_NO_LOST", nil},
		{"reorder names", []string{"reorder", "lost", "won"}, 0, "", []string{"lost", "won", "lead"}},
		{"reorder one to a position", []string{"reorder", "lost", "--position", "2"}, 0, "", []string{"lead", "lost", "prospect"}},
		{"reorder unknown", []string{"reorder", "ghost"}, 3, "", nil},
		{"position with two stages", []string{"reorder", "lost", "won", "--position", "1"}, 2, "", nil},
		{"outcome", []string{"outcome", "negotiation", "won"}, 0, "", nil},
		{"outcome invalid", []string{"outcome", "lead", "closed"}, 2, "E_STAGE_OUTCOME_INVALID", nil},
		{"outcome would leave no won stage", []string{"outcome", "won", "open"}, 2, "E_STAGE_NO_WON", nil},
		{"outcome would leave no lost stage", []string{"outcome", "lost", "won"}, 2, "E_STAGE_NO_LOST", nil},
		{"outcome unknown", []string{"outcome", "ghost", "open"}, 3, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := filepath.Join(t.TempDir(), "crm.db")
			_, stderr, code := crm(t, db, append([]string{"deal", "stages"}, tt.args...)...)
			assert.Equal(t, tt.wantCode, code, stderr)
			if tt.wantErr != "" {
				assert.Contains(t, stderr, tt.wantErr)
			}
			if tt.wantHead != nil {
				assert.Equal(t, tt.wantHead, stageNames(t, db)[:len(tt.wantHead)])
			}
		})
	}
	t.Run("every open stage cannot go", func(t *testing.T) {
		db := filepath.Join(t.TempDir(), "crm.db")
		for _, n := range []string{"lead", "prospect", "proposal"} {
			_, stderr, code := crm(t, db, "deal", "stages", "remove", n)
			require.Equal(t, 0, code, stderr)
		}
		_, stderr, code := crm(t, db, "deal", "stages", "remove", "negotiation")
		assert.Equal(t, 2, code)
		assert.Contains(t, stderr, "E_STAGE_NO_OPEN")
	})
}

func TestDealStagesUsedByDeals(t *testing.T) {
	db := filepath.Join(t.TempDir(), "crm.db")
	for _, a := range [][]string{
		{"deal", "add", "A", "--stage", "proposal"},
		{"deal", "add", "B", "--stage", "proposal"},
		{"deal", "add", "C"},
	} {
		_, stderr, code := crm(t, db, a...)
		require.Equal(t, 0, code, stderr)
	}
	dealStage := func(id string) string {
		stdout, _, code := crm(t, db, "deal", "show", id, "-f", "json")
		require.Equal(t, 0, code)
		var data []map[string]any
		require.NoError(t, json.Unmarshal([]byte(stdout), &data))
		return data[0]["stage"].(string)
	}
	assert.Equal(t, "lead", dealStage("3"), "no --stage is the first open stage")

	t.Run("a stage name that is not listed is refused with a fix", func(t *testing.T) {
		_, stderr, code := crm(t, db, "deal", "add", "D", "--stage", "qualified")
		assert.Equal(t, 2, code)
		assert.Contains(t, stderr, "E_VOCAB_UNKNOWN")
		assert.Contains(t, stderr, "crm deal stages add")
	})
	t.Run("in use shows in the list", func(t *testing.T) {
		for _, r := range stageRowsOf(t, db) {
			if r.Name == "proposal" {
				assert.Equal(t, 2, r.InUse)
			}
		}
	})
	t.Run("remove of a used stage needs --replace-with", func(t *testing.T) {
		_, stderr, code := crm(t, db, "deal", "stages", "remove", "proposal")
		assert.Equal(t, 2, code)
		assert.Contains(t, stderr, "E_VOCAB_IN_USE")
		assert.Contains(t, stderr, "--replace-with")
		assert.NotContains(t, stderr, "--keep")
		assert.Contains(t, stageNames(t, db), "proposal")
	})
	t.Run("rename propagates to the deals", func(t *testing.T) {
		_, stderr, code := crm(t, db, "deal", "stages", "rename", "proposal", "quote sent")
		require.Equal(t, 0, code, stderr)
		assert.Equal(t, "quote sent", dealStage("1"))
		assert.Equal(t, "quote sent", dealStage("2"))
		_, _, code = crm(t, db, "deal", "list", "--stage", "quote sent", "-f", "json")
		assert.Equal(t, 0, code)
	})
	t.Run("remove with a replacement moves the deals", func(t *testing.T) {
		stdout, stderr, code := crm(t, db, "deal", "stages", "remove", "quote sent", "--replace-with", "prospect", "-f", "json")
		require.Equal(t, 0, code, stderr)
		var res []map[string]any
		require.NoError(t, json.Unmarshal([]byte(stdout), &res))
		assert.EqualValues(t, 2, res[0]["moved"])
		assert.Equal(t, "prospect", dealStage("1"))
		assert.NotContains(t, stageNames(t, db), "quote sent")
	})
	t.Run("the pipeline follows the list order and shows the outcome", func(t *testing.T) {
		_, _, code := crm(t, db, "deal", "add", "Closed", "--stage", "won")
		require.Equal(t, 0, code)
		_, _, code = crm(t, db, "deal", "stages", "reorder", "won", "prospect")
		require.Equal(t, 0, code)
		stdout, _, code := crm(t, db, "deal", "pipeline", "-f", "json")
		require.Equal(t, 0, code)
		var rows []map[string]any
		require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
		require.Len(t, rows, 3)
		assert.Equal(t, "won", rows[0]["stage"])
		assert.Equal(t, "won", rows[0]["outcome"])
		assert.Equal(t, "prospect", rows[1]["stage"])
		assert.Equal(t, "lead", rows[2]["stage"])
	})
}

func TestDealStagesOutcomeDrivesClosedAt(t *testing.T) {
	db := filepath.Join(t.TempDir(), "crm.db")
	closedAt := func(id string) any {
		stdout, _, code := crm(t, db, "deal", "show", id, "-f", "json")
		require.Equal(t, 0, code)
		var data []map[string]any
		require.NoError(t, json.Unmarshal([]byte(stdout), &data))
		return data[0]["closed_at"]
	}
	_, _, code := crm(t, db, "deal", "add", "A")
	require.Equal(t, 0, code)
	_, stderr, code := crm(t, db, "deal", "stages", "rename", "won", "signed")
	require.Equal(t, 0, code, stderr)
	_, stderr, code = crm(t, db, "deal", "edit", "1", "--stage", "signed")
	require.Equal(t, 0, code, stderr)
	assert.NotNil(t, closedAt("1"), "a renamed won stage still closes")
	_, _, code = crm(t, db, "deal", "edit", "1", "--stage", "negotiation")
	require.Equal(t, 0, code)
	assert.Nil(t, closedAt("1"))
	_, _, code = crm(t, db, "deal", "stages", "outcome", "negotiation", "lost")
	require.Equal(t, 0, code)
	_, _, code = crm(t, db, "deal", "edit", "1", "--stage", "negotiation")
	require.Equal(t, 0, code)
	assert.NotNil(t, closedAt("1"), "the outcome closes, not the name")
}

func TestDealStageFlagsPointAtTheList(t *testing.T) {
	db := filepath.Join(t.TempDir(), "crm.db")
	for _, c := range [][]string{{"deal", "add", "--help"}, {"deal", "edit", "--help"}} {
		stdout, _, code := crm(t, db, c...)
		require.Equal(t, 0, code)
		assert.Contains(t, stdout, "crm deal stages list")
		assert.NotContains(t, stdout, "negotiation, won, lost")
	}
}
