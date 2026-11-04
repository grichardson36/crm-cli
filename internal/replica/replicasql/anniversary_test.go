package replicasql_test

import (
	"context"
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CRM-022, device sync door: anniversary is a plain person_relation column, so the generated capture
// triggers carry it with the last-writer-wins default. No registry line and no contract change are
// needed, the same as middle_name. The built-in relation kinds exist on every device.
func TestAnniversaryReplicatesSetAndUnset(t *testing.T) {
	a := enable(t, newDB(t))
	b := enable(t, newDB(t))
	mustExec(t, a.DB(), `INSERT INTO people (uuid, first_name, last_name) VALUES ('p1', 'Joey', 'Osgood'), ('p2', 'Hannah', 'Osgood')`)
	mustExec(t, a.DB(), `INSERT INTO person_relation (uuid, a_id, b_id, kind_id, anniversary)
		VALUES ('l1', (SELECT id FROM people WHERE uuid='p1'), (SELECT id FROM people WHERE uuid='p2'),
		        (SELECT id FROM relation_kind WHERE object_type = 'person' AND name = 'spouse'), '2014-06-20')`)
	syncOnce(t, a, b)
	assert.Equal(t, "2014-06-20", str(t, b.DB(), `SELECT anniversary FROM person_relation WHERE uuid='l1'`))

	mustExec(t, a.DB(), `UPDATE person_relation SET anniversary = '--06-21' WHERE uuid='l1'`)
	syncOnce(t, a, b)
	assert.Equal(t, "--06-21", str(t, b.DB(), `SELECT anniversary FROM person_relation WHERE uuid='l1'`))

	mustExec(t, a.DB(), `UPDATE person_relation SET anniversary = NULL WHERE uuid='l1'`)
	syncOnce(t, a, b)
	assert.Equal(t, 1, count(t, b.DB(), `SELECT COUNT(*) FROM person_relation WHERE uuid='l1' AND anniversary IS NULL`))
}

// Turning sync on gives every relation_kind row a uuid derived from its natural key, so the built-in
// uuid of the spouse kind is gone on a syncing database. The anniversary rules still find the kind.
func TestAnniversaryRulesWorkOnASyncingDatabase(t *testing.T) {
	ctx := context.Background()
	s := enable(t, newDB(t))
	assert.Equal(t, 0, count(t, s.DB(), `SELECT COUNT(*) FROM relation_kind WHERE uuid = 'a0000001-0000-4000-8000-000000000002'`), "sync renamed the row's uuid")

	links, people := repo.NewLinkRepo(s.DB()), repo.NewPersonRepo(s.DB())
	last := "Osgood"
	a, err := people.Create(ctx, model.CreatePersonInput{FirstName: "Joey", LastName: &last})
	require.NoError(t, err)
	b, err := people.Create(ctx, model.CreatePersonInput{FirstName: "Hannah", LastName: &last})
	require.NoError(t, err)
	spouse, friend := "spouse", "friend"
	ann := "2014-06-20"
	_, err = links.Link(ctx, repo.Ref{Kind: "person", ID: a.ID}, repo.Ref{Kind: "person", ID: b.ID}, model.LinkAttrs{Kind: &friend, Anniversary: &ann})
	assert.ErrorIs(t, err, model.ErrValidation, "another kind still refuses it")
	_, err = links.Link(ctx, repo.Ref{Kind: "person", ID: a.ID}, repo.Ref{Kind: "person", ID: b.ID}, model.LinkAttrs{Kind: &spouse, Anniversary: &ann})
	require.NoError(t, err)

	got, err := people.AnniversariesWindow(ctx, 90, 100, time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Joey-Hannah Osgood Anniversary", got[0].Title)
}
