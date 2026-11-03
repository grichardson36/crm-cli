package quick

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CRM-019, command bar and crm do door: "set Bob: middle=Lee" sets the middle name, applies once and undoes.
func TestSetMiddleName(t *testing.T) {
	f := newFixture(t)
	p := Parse("set Bob: middle=Lee", f.env(""))
	require.Empty(t, p.Problems, p.Preview)
	require.Len(t, p.Changes, 1)
	assert.Equal(t, "middle", p.Changes[0].Field)
	assert.Equal(t, "Lee", p.Changes[0].New)

	res, err := Apply(context.Background(), f.repos, p)
	require.NoError(t, err)
	require.NotNil(t, f.person("Bob Baker").MiddleName)
	assert.Equal(t, "Lee", *f.person("Bob Baker").MiddleName)
	assert.Equal(t, "Bob Baker", p.Changes[0].Label, "the label stays First Last")

	// The aliases a person types.
	p = Parse("set Bob: middle_name=Quill", f.env(""))
	require.Empty(t, p.Problems, p.Preview)
	require.Len(t, p.Changes, 1)
	assert.Equal(t, "middle", p.Changes[0].Field)

	// Undo puts the old value back.
	require.NotNil(t, res)
	err = Undo(context.Background(), f.repos, res.Changes)
	require.NoError(t, err)
	got := f.person("Bob Baker").MiddleName
	assert.True(t, got == nil || *got == "", "undo restores the empty value")
}

func TestInverseUpdateRestoresMiddleName(t *testing.T) {
	before := &model.Person{FirstName: "Bob", MiddleName: sp("Old")}
	inv := InverseUpdate(before, model.UpdatePersonInput{MiddleName: sp("New")})
	require.NotNil(t, inv.MiddleName)
	assert.Equal(t, "Old", *inv.MiddleName)
	assert.Nil(t, InverseUpdate(before, model.UpdatePersonInput{}).MiddleName, "an update that leaves it alone has nothing to restore")
}
