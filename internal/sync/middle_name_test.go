package sync_test

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	crmsync "github.com/jdanielnd/crm-cli/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CRM-019, source sync door: a source that knows the middle name fills a blank one on a match,
// creates people with it, and never overwrites a middle name the person already has on first match.
func TestSourceSyncCarriesMiddleName(t *testing.T) {
	e := newEnv(t,
		personRec(1, "g1", crmsync.PersonPatch{FirstName: ptr("Jane"), LastName: ptr("Smith"), MiddleName: ptr("Marie"), Emails: []string{"jane@acme.com"}}),
		personRec(2, "g2", crmsync.PersonPatch{FirstName: ptr("Bob"), LastName: ptr("Jones"), MiddleName: ptr("Lou"), Emails: []string{"bob@acme.com"}}),
	)
	jane := e.person("Jane", "jane@acme.com", "")
	bob := e.person("Bob", "bob@acme.com", "")
	_, err := e.people.Update(context.Background(), bob.ID, model.UpdatePersonInput{MiddleName: ptr("Keep")})
	require.NoError(t, err)

	res := e.run(crmsync.RunOptions{AllowCreate: true})
	assert.Equal(t, 2, res.Updated)

	got, err := e.people.FindByID(context.Background(), jane.ID)
	require.NoError(t, err)
	require.NotNil(t, got.MiddleName)
	assert.Equal(t, "Marie", *got.MiddleName, "a blank middle name is filled")
	got, err = e.people.FindByID(context.Background(), bob.ID)
	require.NoError(t, err)
	assert.Equal(t, "Keep", *got.MiddleName, "the first match fills blanks only")
}

func TestSourceSyncCreatesWithMiddleName(t *testing.T) {
	e := newEnv(t,
		personRec(1, "g1", crmsync.PersonPatch{FirstName: ptr("Cy"), LastName: ptr("Cole"), MiddleName: ptr("Dee"), Emails: []string{"cy@x.com"}}),
	)
	res := e.run(crmsync.RunOptions{AllowCreate: true})
	require.Equal(t, 1, res.Created)
	found, err := e.people.Search(context.Background(), "Dee", 10)
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "Cy", found[0].FirstName)
	assert.Equal(t, "Dee", *found[0].MiddleName)
}
