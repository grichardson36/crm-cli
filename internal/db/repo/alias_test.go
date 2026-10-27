package repo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/alias"
	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersonFTSQuery(t *testing.T) {
	tests := []struct {
		name, query, want string
	}{
		{"no alias keeps the old phrase query", "jane smith", `"jane smith"*`},
		{"nickname", "Zach", `("Zach"* OR "zachary"*)`},
		{"two words, one alias", "rich smith", `("rich"* OR "richard"*) AND ("smith"*)`},
		{"quote is escaped", `a"b`, `"a""b"*`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, repo.PersonFTSQuery(tt.query, alias.Default()))
		})
	}
}

func TestSearchExpandsAliases(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	ctx := context.Background()
	pr := repo.NewPersonRepo(d)
	_, err = pr.Create(ctx, model.CreatePersonInput{FirstName: "Zachary", LastName: ptr("Lee")})
	require.NoError(t, err)
	_, err = pr.Create(ctx, model.CreatePersonInput{FirstName: "Richard", LastName: ptr("Roe")})
	require.NoError(t, err)
	_, err = pr.Create(ctx, model.CreatePersonInput{FirstName: "Zed", LastName: ptr("Quinn")})
	require.NoError(t, err)

	names := func(q string) []string {
		got, err := pr.Search(ctx, q, 10)
		require.NoError(t, err)
		var out []string
		for _, p := range got {
			out = append(out, p.FirstName)
		}
		return out
	}
	assert.Equal(t, []string{"Zachary"}, names("Zach"))
	assert.Equal(t, []string{"Richard"}, names("Rich"))
	assert.Equal(t, []string{"Richard"}, names("Rick roe"))
	assert.Empty(t, names("Zachery"), "typo tolerance is for typeaheads, not plain search")

	ar := repo.NewAliasRepo(d)
	changed, err := ar.Add(ctx, "Zed", "Zachary")
	require.NoError(t, err)
	assert.True(t, changed)
	assert.ElementsMatch(t, []string{"Zachary", "Zed"}, names("zed"))
}

func TestAliasRepo(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	ctx := context.Background()
	ar := repo.NewAliasRepo(d)

	tests := []struct {
		name     string
		do       func() (bool, error)
		wantErr  error
		wantChng bool
	}{
		{"add new", func() (bool, error) { return ar.Add(ctx, "zeke", "zachary") }, nil, true},
		{"add again is a no-op", func() (bool, error) { return ar.Add(ctx, "zeke", "zachary") }, nil, false},
		{"builtin pair is a no-op", func() (bool, error) { return ar.Add(ctx, "zach", "zachary") }, nil, false},
		{"self alias", func() (bool, error) { return ar.Add(ctx, "bob", "bob") }, model.ErrValidation, false},
		{"two words", func() (bool, error) { return ar.Add(ctx, "big bob", "robert") }, model.ErrValidation, false},
		{"full name cannot become a nickname", func() (bool, error) { return ar.Add(ctx, "robert", "bob") }, model.ErrConflict, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed, err := tt.do()
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.wantErr), err.Error())
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantChng, changed)
		})
	}

	t.Run("remove addition and builtin", func(t *testing.T) {
		require.NoError(t, ar.Remove(ctx, "zeke"))
		assert.ErrorIs(t, ar.Remove(ctx, "zeke"), model.ErrNotFound)
		require.NoError(t, ar.Remove(ctx, "bobby"))
		tbl, err := ar.Table(ctx)
		require.NoError(t, err)
		assert.Equal(t, "bobby", tbl.Canonical("bobby"))
		assert.Equal(t, "robert", tbl.Canonical("bob"))
		list, err := ar.List(ctx)
		require.NoError(t, err)
		for _, e := range list {
			if e.Alias == "bobby" {
				assert.True(t, e.Builtin)
				assert.False(t, e.Active)
			}
		}
		changed, err := ar.Add(ctx, "bobby", "robert")
		require.NoError(t, err)
		assert.True(t, changed, "turns the builtin back on")
	})
}

// The duplicate finder reads the same table: the built-in names find what they always found and a
// user alias adds a pair.
func TestDupeFinderReadsAliases(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	ctx := context.Background()
	pr := repo.NewPersonRepo(d)
	for _, n := range [][2]string{{"Robert", "Stone"}, {"Bob", "Stone"}, {"Zeke", "Park"}, {"Zachary", "Park"}} {
		_, err := pr.Create(ctx, model.CreatePersonInput{FirstName: n[0], LastName: ptr(n[1])})
		require.NoError(t, err)
	}
	dr := repo.NewDupeRepo(d)
	groups, err := dr.Find(ctx, 1)
	require.NoError(t, err)
	require.Len(t, groups, 1, "only the built-in pair before the user adds an alias")
	_, err = repo.NewAliasRepo(d).Add(ctx, "zeke", "zachary")
	require.NoError(t, err)
	groups, err = dr.Find(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, groups, 2)
}
