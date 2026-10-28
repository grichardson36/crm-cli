package repo_test

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/validate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const roleKind = model.DealRoleKind

func names(t *testing.T, vr *repo.VocabRepo) []string {
	t.Helper()
	n, err := vr.Names(context.Background(), roleKind)
	require.NoError(t, err)
	return n
}

// codeOf is the validation code of an error, "" when it is not a validation error.
func codeOf(err error) string {
	if e, ok := validate.As(err); ok {
		return e.First().Code
	}
	return ""
}

func TestVocab_SeededDealRoles(t *testing.T) {
	f := setupLinks(t)
	vr := repo.NewVocabRepo(f.links.DB())
	vs, _ := model.VocabularyNamed(roleKind)
	assert.Equal(t, vs.Seeds, names(t, vr))
	for _, want := range []string{"champion", "decision maker", "economic buyer", "influencer", "blocker", "end user", "signer", "billing contact"} {
		assert.Contains(t, names(t, vr), want)
	}
}

func TestVocab_Add(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		input    string
		wantCode string
		want     string
	}{
		{"new role goes last", "board observer", "", "board observer"},
		{"spaces are folded", "  legal   reviewer ", "", "legal reviewer"},
		{"duplicate ignoring case", "CHAMPION", validate.CodeVocabDuplicate, ""},
		{"empty", "   ", validate.CodeVocabInvalid, ""},
		{"too long", string(make([]byte, 0)) + repeat("x", 81), validate.CodeVocabInvalid, ""},
		{"pipe is not allowed", "a|b", validate.CodeVocabInvalid, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vr := repo.NewVocabRepo(setupLinks(t).links.DB())
			e, err := vr.Add(ctx, roleKind, tt.input)
			if tt.wantCode != "" {
				assert.Equal(t, tt.wantCode, codeOf(err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, e.Name)
			n := names(t, vr)
			assert.Equal(t, tt.want, n[len(n)-1])
		})
	}
	t.Run("unknown vocabulary", func(t *testing.T) {
		vr := repo.NewVocabRepo(setupLinks(t).links.DB())
		_, err := vr.Add(ctx, "nonsense", "x")
		assert.ErrorIs(t, err, model.ErrValidation)
	})
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}

func TestVocab_RenamePropagatesToRows(t *testing.T) {
	ctx := context.Background()
	f := setupLinks(t)
	vr := repo.NewVocabRepo(f.links.DB())
	deal := repo.Ref{Kind: "deal", ID: f.deal}
	_, err := f.links.Link(ctx, deal, repo.Ref{Kind: "person", ID: f.person}, model.LinkAttrs{Role: sp("champion")})
	require.NoError(t, err)
	_, err = f.links.Link(ctx, deal, repo.Ref{Kind: "person", ID: f.other}, model.LinkAttrs{Role: sp("Champion")})
	require.NoError(t, err) // stored in the listed spelling

	tests := []struct {
		name, from, to string
		wantCode       string
		wantRows       int
		wantNames      []string
	}{
		{"rename rewrites both rows", "champion", "internal champion", "", 2, []string{"internal champion", "decision maker"}},
		{"rename to an existing name", "blocker", "signer", validate.CodeVocabDuplicate, 0, nil},
		{"rename a name that is not listed", "ghost", "x", "", 0, nil},
		{"rename to empty", "signer", " ", validate.CodeVocabInvalid, 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := vr.Rename(ctx, roleKind, tt.from, tt.to)
			switch {
			case tt.wantCode != "":
				assert.Equal(t, tt.wantCode, codeOf(err))
			case tt.from == "ghost":
				assert.ErrorIs(t, err, model.ErrNotFound)
			default:
				require.NoError(t, err)
				assert.Equal(t, tt.wantRows, res.Rows)
				assert.Equal(t, tt.wantNames, names(t, vr)[:2], "position is kept")
			}
		})
	}
	linked, err := f.links.Linked(ctx, deal, "person")
	require.NoError(t, err)
	for _, l := range linked {
		assert.Equal(t, "internal champion", *l.Role)
	}
	t.Run("case only change", func(t *testing.T) {
		_, err := vr.Rename(ctx, roleKind, "signer", "Signer")
		require.NoError(t, err)
		assert.Contains(t, names(t, vr), "Signer")
	})
}

func TestVocab_Remove(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name         string
		role         string // used by one link before the removal
		remove       string
		replace      string
		keep         bool
		wantCode     string
		wantMoved    int
		wantKept     int
		wantRowRole  string
		wantListed   bool
		wantNotFound bool
	}{
		{name: "unused role is removed", remove: "blocker", wantListed: false},
		{name: "used role needs a decision", role: "blocker", remove: "blocker", wantCode: validate.CodeVocabInUse, wantRowRole: "blocker", wantListed: true},
		{name: "replacement moves the rows", role: "blocker", remove: "blocker", replace: "influencer", wantMoved: 1, wantRowRole: "influencer"},
		{name: "keep leaves plain text", role: "blocker", remove: "blocker", keep: true, wantKept: 1, wantRowRole: "blocker"},
		{name: "replacement must be listed", role: "blocker", remove: "blocker", replace: "ghost", wantNotFound: true, wantRowRole: "blocker", wantListed: true},
		{name: "replacement cannot be itself", role: "blocker", remove: "blocker", replace: "BLOCKER", wantRowRole: "blocker", wantListed: true, wantCode: "validation"},
		{name: "not listed", remove: "ghost", wantNotFound: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := setupLinks(t)
			vr := repo.NewVocabRepo(f.links.DB())
			deal := repo.Ref{Kind: "deal", ID: f.deal}
			if tt.role != "" {
				_, err := f.links.Link(ctx, deal, repo.Ref{Kind: "person", ID: f.person}, model.LinkAttrs{Role: sp(tt.role)})
				require.NoError(t, err)
			}
			res, err := vr.Remove(ctx, roleKind, tt.remove, tt.replace, tt.keep)
			switch {
			case tt.wantNotFound:
				assert.ErrorIs(t, err, model.ErrNotFound)
			case tt.wantCode == "validation":
				assert.ErrorIs(t, err, model.ErrValidation)
			case tt.wantCode != "":
				assert.Equal(t, tt.wantCode, codeOf(err))
			default:
				require.NoError(t, err)
				assert.Equal(t, tt.wantMoved, res.Moved)
				assert.Equal(t, tt.wantKept, res.Kept)
			}
			assert.Equal(t, tt.wantListed, contains(names(t, vr), tt.remove))
			if tt.wantRowRole != "" {
				linked, err := f.links.Linked(ctx, deal, "person")
				require.NoError(t, err)
				require.Len(t, linked, 1)
				assert.Equal(t, tt.wantRowRole, *linked[0].Role)
			}
		})
	}
	t.Run("a kept value is plain text, not a listed role", func(t *testing.T) {
		f := setupLinks(t)
		vr := repo.NewVocabRepo(f.links.DB())
		deal := repo.Ref{Kind: "deal", ID: f.deal}
		_, err := f.links.Link(ctx, deal, repo.Ref{Kind: "person", ID: f.person}, model.LinkAttrs{Role: sp("blocker")})
		require.NoError(t, err)
		_, err = vr.Remove(ctx, roleKind, "blocker", "", true)
		require.NoError(t, err)
		_, err = f.links.Link(ctx, deal, repo.Ref{Kind: "person", ID: f.other}, model.LinkAttrs{Role: sp("blocker")})
		assert.Equal(t, validate.CodeVocabUnknown, codeOf(err))
	})
	t.Run("a removed name comes back with Add", func(t *testing.T) {
		vr := repo.NewVocabRepo(setupLinks(t).links.DB())
		_, err := vr.Remove(ctx, roleKind, "blocker", "", false)
		require.NoError(t, err)
		_, err = vr.Add(ctx, roleKind, "Blocker")
		require.NoError(t, err)
		n := names(t, vr)
		assert.Equal(t, "Blocker", n[len(n)-1])
	})
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestVocab_ReorderAndMove(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		do       func(vr *repo.VocabRepo) error
		wantHead []string
		wantErr  bool
	}{
		{"named first, rest keep order", func(vr *repo.VocabRepo) error {
			return vr.Reorder(ctx, roleKind, []string{"signer", "blocker"})
		}, []string{"signer", "blocker", "champion", "decision maker"}, false},
		{"case is ignored and repeats collapse", func(vr *repo.VocabRepo) error {
			return vr.Reorder(ctx, roleKind, []string{"BLOCKER", "blocker"})
		}, []string{"blocker", "champion", "decision maker", "economic buyer"}, false},
		{"unknown name changes nothing", func(vr *repo.VocabRepo) error {
			return vr.Reorder(ctx, roleKind, []string{"signer", "ghost"})
		}, []string{"champion", "decision maker", "economic buyer", "influencer"}, true},
		{"move to position 2", func(vr *repo.VocabRepo) error { return vr.Move(ctx, roleKind, "signer", 2) },
			[]string{"champion", "signer", "decision maker", "economic buyer"}, false},
		{"move past the end goes last", func(vr *repo.VocabRepo) error { return vr.Move(ctx, roleKind, "champion", 99) },
			[]string{"decision maker", "economic buyer", "influencer", "blocker"}, false},
		{"position 0 is refused", func(vr *repo.VocabRepo) error { return vr.Move(ctx, roleKind, "champion", 0) },
			[]string{"champion", "decision maker", "economic buyer", "influencer"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vr := repo.NewVocabRepo(setupLinks(t).links.DB())
			err := tt.do(vr)
			assert.Equal(t, tt.wantErr, err != nil)
			assert.Equal(t, tt.wantHead, names(t, vr)[:4])
		})
	}
}

func TestVocab_ListShowsUseAndRemoved(t *testing.T) {
	ctx := context.Background()
	f := setupLinks(t)
	vr := repo.NewVocabRepo(f.links.DB())
	_, err := f.links.Link(ctx, repo.Ref{Kind: "deal", ID: f.deal}, repo.Ref{Kind: "person", ID: f.person}, model.LinkAttrs{Role: sp("signer")})
	require.NoError(t, err)
	_, err = vr.Remove(ctx, roleKind, "blocker", "", false)
	require.NoError(t, err)
	live, err := vr.List(ctx, roleKind, false)
	require.NoError(t, err)
	all, err := vr.List(ctx, roleKind, true)
	require.NoError(t, err)
	assert.Len(t, all, len(live)+1)
	assert.True(t, all[len(all)-1].Removed, "removed values come last")
	for _, e := range live {
		want := 0
		if e.Name == "signer" {
			want = 1
		}
		assert.Equal(t, want, e.InUse, e.Name)
	}
}

func TestVocab_BackfillPlanApplyUndo(t *testing.T) {
	ctx := context.Background()
	f := setupLinks(t)
	d := f.links.DB()
	vr := repo.NewVocabRepo(d)
	// Free-text roles from before the vocabulary, written the way old data was: straight into the table.
	p3, err := f.people.Create(ctx, model.CreatePersonInput{FirstName: "Kim"})
	require.NoError(t, err)
	p4, err := f.people.Create(ctx, model.CreatePersonInput{FirstName: "Lee"})
	require.NoError(t, err)
	for i, role := range map[int64]string{f.person: "Board Observer", f.other: "board observer ", p3.ID: "Champion", p4.ID: "legacy|pipe"} {
		_, err := d.Exec(`INSERT INTO deal_person (uuid, deal_id, person_id, role) VALUES (?, ?, ?, ?)`, "u"+role, f.deal, i, role)
		require.NoError(t, err)
	}

	before := names(t, vr)
	plan, err := vr.BackfillPlan(ctx, roleKind)
	require.NoError(t, err)
	assert.Equal(t, 4, plan.Rows)
	assert.Equal(t, 3, plan.Distinct)
	assert.Equal(t, 1, plan.Listed, "Champion is already listed")
	assert.Equal(t, 1, plan.Invalid, "a pipe cannot be a list value")
	require.Len(t, plan.ToAdd, 1)
	assert.Equal(t, repo.VocabCandidate{Name: "Board Observer", Rows: 2}, plan.ToAdd[0], "counts both spellings, shows the commonest (ties alphabetical)")
	assert.Equal(t, before, names(t, vr), "the plan writes nothing")

	rep, ch, err := vr.BackfillApply(ctx, roleKind)
	require.NoError(t, err)
	assert.Len(t, rep.ToAdd, 1)
	assert.Equal(t, append(append([]string{}, before...), "Board Observer"), names(t, vr))

	again, _, err := vr.BackfillApply(ctx, roleKind)
	require.NoError(t, err)
	assert.Empty(t, again.ToAdd, "running it again adds nothing")

	require.NoError(t, vr.UndoBackfill(ctx, ch))
	assert.Equal(t, before, names(t, vr), "one undo step takes every added value off")
	require.NoError(t, vr.RedoBackfill(ctx, ch))
	assert.Contains(t, names(t, vr), "Board Observer")
	var kept int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM deal_person WHERE role IS NOT NULL`).Scan(&kept))
	assert.Equal(t, 4, kept, "no record lost its role")
}

func TestVocab_BackfillSkipsRemovedOnPurpose(t *testing.T) {
	ctx := context.Background()
	f := setupLinks(t)
	d := f.links.DB()
	vr := repo.NewVocabRepo(d)
	_, err := d.Exec(`INSERT INTO deal_person (uuid, deal_id, person_id, role) VALUES ('u1', ?, ?, 'blocker')`, f.deal, f.person)
	require.NoError(t, err)
	_, err = vr.Remove(ctx, roleKind, "blocker", "", true)
	require.NoError(t, err)
	plan, err := vr.BackfillPlan(ctx, roleKind)
	require.NoError(t, err)
	assert.Equal(t, 1, plan.Removed)
	assert.Empty(t, plan.ToAdd)
}

func TestVocab_FieldAndResolve(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name        string
		raw         string
		create      bool
		wantValue   string
		wantCreated bool
		wantCode    string
	}{
		{"listed value", "decision maker", false, "decision maker", false, ""},
		{"stored spelling wins", "Decision Maker", false, "decision maker", false, ""},
		{"empty clears", "", false, "", false, ""},
		{"unknown is refused", "kingmaker", false, "", false, validate.CodeVocabUnknown},
		{"unknown with create is added", "kingmaker", true, "kingmaker", true, ""},
		{"create still checks the name", "a|b", true, "", false, validate.CodeVocabInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vr := repo.NewVocabRepo(setupLinks(t).links.DB())
			got, created, err := vr.Resolve(ctx, model.DealPersonKind, "role", tt.raw, tt.create)
			if tt.wantCode != "" {
				assert.Equal(t, tt.wantCode, codeOf(err))
				assert.NotContains(t, names(t, vr), tt.raw)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantValue, got)
			assert.Equal(t, tt.wantCreated, created)
			if tt.wantCreated {
				assert.Contains(t, names(t, vr), tt.wantValue)
			}
		})
	}
	t.Run("not a vocabulary field", func(t *testing.T) {
		vr := repo.NewVocabRepo(setupLinks(t).links.DB())
		_, _, err := vr.Resolve(ctx, "person", "first", "lead", false)
		assert.ErrorIs(t, err, model.ErrValidation)
	})
}

func TestDealLinkRole_ValidatedThroughVocabulary(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		attrs    model.LinkAttrs
		wantRole string
		wantCode string
		created  string
	}{
		{"listed role", model.LinkAttrs{Role: sp("champion")}, "champion", "", ""},
		{"listed role other case", model.LinkAttrs{Role: sp("ECONOMIC BUYER")}, "economic buyer", "", ""},
		{"no role", model.LinkAttrs{}, "", "", ""},
		{"empty role clears", model.LinkAttrs{Role: sp("")}, "", "", ""},
		{"unknown role refused", model.LinkAttrs{Role: sp("kingmaker")}, "", validate.CodeVocabUnknown, ""},
		{"unknown role with create", model.LinkAttrs{Role: sp("kingmaker"), CreateRole: true}, "kingmaker", "", "kingmaker"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := setupLinks(t)
			vr := repo.NewVocabRepo(f.links.DB())
			deal := repo.Ref{Kind: "deal", ID: f.deal}
			_, err := f.links.Link(ctx, deal, repo.Ref{Kind: "person", ID: f.person}, tt.attrs)
			if tt.wantCode != "" {
				assert.Equal(t, tt.wantCode, codeOf(err))
				e, _ := validate.As(err)
				assert.Contains(t, e.First().Fix, "crm deal roles add")
				assert.Contains(t, e.First().Fix, "--create")
				return
			}
			require.NoError(t, err)
			linked, err := f.links.Linked(ctx, deal, "person")
			require.NoError(t, err)
			require.Len(t, linked, 1)
			if tt.wantRole == "" {
				assert.True(t, linked[0].Role == nil || *linked[0].Role == "")
			} else {
				assert.Equal(t, tt.wantRole, *linked[0].Role)
			}
			if tt.created != "" {
				assert.Contains(t, names(t, vr), tt.created)
			}
		})
	}
	t.Run("membership roles stay free text", func(t *testing.T) {
		f := setupLinks(t)
		_, err := f.links.Link(ctx, repo.Ref{Kind: "person", ID: f.person}, repo.Ref{Kind: "organization", ID: f.org}, model.LinkAttrs{Role: sp("Chief Whatever Officer")})
		assert.NoError(t, err)
	})
	t.Run("task and note links stay free text", func(t *testing.T) {
		f := setupLinks(t)
		_, err := f.links.Link(ctx, repo.Ref{Kind: "task", ID: f.task}, repo.Ref{Kind: "person", ID: f.person}, model.LinkAttrs{Role: sp("whoever")})
		assert.NoError(t, err)
	})
}

func TestDealLinkRole_LegacyTextSurvivesOtherEdits(t *testing.T) {
	ctx := context.Background()
	f := setupLinks(t)
	d := f.links.DB()
	_, err := d.Exec(`INSERT INTO deal_person (uuid, deal_id, person_id, role) VALUES ('u1', ?, ?, 'Board Observer')`, f.deal, f.person)
	require.NoError(t, err)
	deal := repo.Ref{Kind: "deal", ID: f.deal}
	linked, err := f.links.Linked(ctx, deal, "person")
	require.NoError(t, err)
	require.Len(t, linked, 1)
	assert.Equal(t, "Board Observer", *linked[0].Role, "shown as-is")

	_, err = f.links.Update(ctx, linked[0].Relation, linked[0].LinkID, model.LinkAttrs{Role: sp("board observer")})
	assert.NoError(t, err, "the role it already holds is accepted")
	_, err = f.links.Update(ctx, linked[0].Relation, linked[0].LinkID, model.LinkAttrs{Role: sp("someone new")})
	assert.Equal(t, validate.CodeVocabUnknown, codeOf(err), "a different unlisted role is refused")
	_, err = f.links.Update(ctx, linked[0].Relation, linked[0].LinkID, model.LinkAttrs{Role: sp("signer")})
	assert.NoError(t, err)
}
