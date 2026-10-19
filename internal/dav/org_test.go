package dav_test

import (
	"context"
	"testing"
	"time"

	"github.com/emersion/go-vcard"
	"github.com/jdanielnd/crm-cli/internal/dav"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// orgFixture builds a person with a primary membership at Acme Corp.
func orgFixture(t *testing.T) (*env, *model.Person, *repo.OrgRepo, *repo.LinkRepo, *model.Organization) {
	t.Helper()
	e := newEnv(t, false)
	ctx := context.Background()
	p, err := e.people.Create(ctx, model.CreatePersonInput{FirstName: "Jane", Company: ptr("Old Text Co"), Title: ptr("CTO")})
	require.NoError(t, err)
	orgs := repo.NewOrgRepo(e.db)
	acme, err := orgs.Create(ctx, model.CreateOrgInput{Name: "Acme Corp"})
	require.NoError(t, err)
	links := repo.NewLinkRepo(e.db)
	role := "CTO"
	_, err = links.Link(ctx, repo.Ref{Kind: "person", ID: p.ID}, repo.Ref{Kind: "organization", ID: acme.ID}, model.LinkAttrs{Role: &role, StartedOn: ptr("2019")})
	require.NoError(t, err)
	e.bump(p.ID)
	return e, p, orgs, links, acme
}

func primaryOf(t *testing.T, e *env, personID int64) (orgID int64, role string) {
	t.Helper()
	var r *string
	require.NoError(t, e.db.QueryRow(`SELECT org_id, role FROM membership WHERE person_id = ? AND is_primary = 1 AND deleted_at IS NULL`, personID).Scan(&orgID, &r))
	if r != nil {
		role = *r
	}
	return
}

// E4: the primary membership fills the vCard ORG, TITLE stays the person's title.
func TestPrimaryOrganizationFillsOrg(t *testing.T) {
	e, p, orgs, _, acme := orgFixture(t)
	ctx := context.Background()
	card, _ := e.pull(e.reload(p.ID))
	assert.Equal(t, "Acme Corp", card.PreferredValue(vcard.FieldOrganization))
	assert.Equal(t, "CTO", card.PreferredValue(vcard.FieldTitle))

	// Renaming the organization changes the contact's version and its ORG.
	_, err := e.db.Exec(`UPDATE people SET updated_at = datetime('now', '-1 day') WHERE id = ?`, p.ID)
	require.NoError(t, err)
	_, etagOld := e.pull(e.reload(p.ID))
	name := "Acme International"
	_, err = orgs.Update(ctx, acme.ID, model.UpdateOrgInput{Name: &name})
	require.NoError(t, err)
	card, etagNew := e.pull(e.reload(p.ID))
	assert.NotEqual(t, etagOld, etagNew)
	assert.Equal(t, "Acme International", card.PreferredValue(vcard.FieldOrganization))
}

func TestPhoneOrgEditMatchingAnOrganizationMovesThePrimaryMembership(t *testing.T) {
	e, p, orgs, _, _ := orgFixture(t)
	ctx := context.Background()
	globex, err := orgs.Create(ctx, model.CreateOrgInput{Name: "Globex"})
	require.NoError(t, err)
	old, err := orgs.Create(ctx, model.CreateOrgInput{Name: "Initech Old"})
	require.NoError(t, err)
	merged, err := orgs.Create(ctx, model.CreateOrgInput{Name: "Initech"})
	require.NoError(t, err)
	require.NoError(t, orgs.Archive(ctx, old.ID))
	_, err = e.db.Exec(`INSERT INTO merge_alias (uuid, kind, old_id, old_uuid, survivor_id) VALUES ('al1', 'organization', ?, ?, ?)`, old.ID, old.UUID, merged.ID)
	require.NoError(t, err)

	card, etag := e.pull(e.reload(p.ID))
	card.SetValue(vcard.FieldOrganization, "GLOBEX") // case-insensitive
	res, err := e.push(e.davID(p), card, etagHeader(etag))
	require.NoError(t, err)
	assert.Empty(t, res.ETag, "the phone typed GLOBEX, the CRM says Globex, so it fetches the card again")
	org, role := primaryOf(t, e, p.ID)
	assert.Equal(t, globex.ID, org, "the membership was repointed in place")
	assert.Equal(t, "CTO", role, "role and dates are kept")
	var n int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM membership WHERE person_id = ? AND deleted_at IS NULL`, p.ID).Scan(&n))
	assert.Equal(t, 1, n)
	assert.Equal(t, "Old Text Co", *e.reload(p.ID).Company, "people.company is untouched")

	// A name that only an archived, merged organization had resolves to its survivor.
	e.bump(p.ID)
	card, etag = e.pull(e.reload(p.ID))
	card.SetValue(vcard.FieldOrganization, "Initech Old")
	_, err = e.push(e.davID(p), card, etagHeader(etag))
	require.NoError(t, err)
	org, _ = primaryOf(t, e, p.ID)
	assert.Equal(t, merged.ID, org)
}

func TestPhoneOrgEditMatchingNothingKeepsTheMembershipAndQueuesReview(t *testing.T) {
	e, p, orgs, _, acme := orgFixture(t)
	card, etag := e.pull(e.reload(p.ID))
	card.SetValue(vcard.FieldOrganization, "Brand New Startup")
	_, err := e.push(e.davID(p), card, etagHeader(etag))
	require.NoError(t, err)

	org, role := primaryOf(t, e, p.ID)
	assert.Equal(t, acme.ID, org, "the primary membership is unchanged")
	assert.Equal(t, "CTO", role)
	list, err := orgs.FindAll(context.Background(), 0)
	require.NoError(t, err)
	assert.Len(t, list, 1, "no organization is created from phone text (Q21)")
	assert.Equal(t, "Brand New Startup", *e.reload(p.ID).Company, "the text is kept on the person")
	var n int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM taggings tg JOIN tags t ON t.id = tg.tag_id WHERE t.name = ? AND tg.entity_type = 'person' AND tg.entity_id = ?`, dav.ReviewTag, p.ID).Scan(&n))
	assert.Equal(t, 1, n, "the person is queued for the company conversion screen")
}

func TestPhoneOrgEditForAPersonWithoutAPrimaryMembership(t *testing.T) {
	e := newEnv(t, false)
	ctx := context.Background()
	p, err := e.people.Create(ctx, model.CreatePersonInput{FirstName: "Sam", Company: ptr("Old Text Co")})
	require.NoError(t, err)
	orgs := repo.NewOrgRepo(e.db)
	globex, err := orgs.Create(ctx, model.CreateOrgInput{Name: "Globex"})
	require.NoError(t, err)

	card, etag := e.pull(e.reload(p.ID))
	assert.Equal(t, "Old Text Co", card.PreferredValue(vcard.FieldOrganization), "no membership: the company text is the ORG")
	card.SetValue(vcard.FieldOrganization, "Globex")
	_, err = e.push(e.davID(p), card, etagHeader(etag))
	require.NoError(t, err)
	org, _ := primaryOf(t, e, p.ID)
	assert.Equal(t, globex.ID, org, "a primary membership was created")

	// No match and no membership: the text is stored and the person is queued.
	q, err := e.people.Create(ctx, model.CreatePersonInput{FirstName: "Quinn"})
	require.NoError(t, err)
	card, etag = e.pull(e.reload(q.ID))
	card.SetValue(vcard.FieldOrganization, "Nowhere Ltd")
	_, err = e.push(e.davID(q), card, etagHeader(etag))
	require.NoError(t, err)
	assert.Equal(t, "Nowhere Ltd", *e.reload(q.ID).Company)
	var n int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM membership WHERE person_id = ?`, q.ID).Scan(&n))
	assert.Equal(t, 0, n)
}

func TestOrgChangedOnBothSidesIsAConflictDecidedByLastWrite(t *testing.T) {
	for _, deviceNewer := range []bool{true, false} {
		e, p, orgs, links, _ := orgFixture(t)
		ctx := context.Background()
		globex, err := orgs.Create(ctx, model.CreateOrgInput{Name: "Globex"})
		require.NoError(t, err)
		initech, err := orgs.Create(ctx, model.CreateOrgInput{Name: "Initech"})
		require.NoError(t, err)

		card, etag := e.pull(e.reload(p.ID)) // the phone holds Acme
		// The CRM moves the primary to Globex after that.
		yes := true
		_, err = links.Link(ctx, repo.Ref{Kind: "person", ID: p.ID}, repo.Ref{Kind: "organization", ID: globex.ID}, model.LinkAttrs{Primary: &yes})
		require.NoError(t, err)
		if !deviceNewer {
			e.clock = e.clock.Add(-48 * time.Hour) // the phone edit counts as older than the CRM change
		}
		// The phone also changed ORG, to Initech.
		card.SetValue(vcard.FieldOrganization, "Initech")
		_, err = e.push(e.davID(p), card, etagHeader(etag))
		require.NoError(t, err)

		org, _ := primaryOf(t, e, p.ID)
		conflicts := e.conflicts()
		require.Len(t, conflicts, 1, "both sides changed ORG since the last sync")
		assert.Equal(t, "company", conflicts[0].Field)
		if deviceNewer {
			assert.Equal(t, initech.ID, org, "the newer phone edit wins")
			assert.Equal(t, "device", conflicts[0].Winner)
		} else {
			assert.Equal(t, globex.ID, org, "the newer CRM change wins")
			assert.Equal(t, "crm", conflicts[0].Winner)
		}
	}
}
