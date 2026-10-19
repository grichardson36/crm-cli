package repo_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeCompany(t *testing.T) {
	for in, want := range map[string]string{
		"Acme Inc":              "acme",
		"  ACME,  Inc.  ":       "acme",
		"acme L.L.C.":           "acme",
		"The Acme Corp":         "acme",
		"Acme Co.":              "acme",
		"Smith & Sons Ltd":      "smith and sons",
		"Smith and Sons":        "smith and sons",
		"Bob's Pizza":           "bobs pizza",
		"Inc":                   "inc",
		"Acme Holdings Limited": "acme holdings",
		"Acme Corp Inc":         "acme",
		"IBM":                   "ibm",
	} {
		assert.Equal(t, want, repo.NormalizeCompany(in), in)
	}
}

type convFx struct {
	d      *sql.DB
	people *repo.PersonRepo
	orgs   *repo.OrgRepo
	links  *repo.LinkRepo
	conv   *repo.ConvertRepo
	tags   *repo.TagRepo
	add    func(first, company, title string) int64
}

func convSetup(t *testing.T) *convFx {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	f := &convFx{d: d, people: repo.NewPersonRepo(d), orgs: repo.NewOrgRepo(d), links: repo.NewLinkRepo(d), conv: repo.NewConvertRepo(d), tags: repo.NewTagRepo(d)}
	f.add = func(first, company, title string) int64 {
		in := model.CreatePersonInput{FirstName: first}
		if company != "" {
			in.Company = &company
		}
		if title != "" {
			in.Title = &title
		}
		p, err := f.people.Create(context.Background(), in)
		require.NoError(t, err)
		return p.ID
	}
	return f
}

func (f *convFx) groups(t *testing.T) (map[string]repo.ConvertGroup, []repo.ConvertGroup, repo.ConvertStats) {
	t.Helper()
	list, stats, err := f.conv.Groups(context.Background(), false)
	require.NoError(t, err)
	by := map[string]repo.ConvertGroup{}
	for _, g := range list {
		by[g.Key] = g
	}
	return by, list, stats
}

func TestGroupsNormalizeCountAndMatch(t *testing.T) {
	f := convSetup(t)
	ctx := context.Background()
	f.add("A", "Acme Inc", "CTO")
	f.add("B", "ACME", "")
	f.add("C", "acme, inc.", "")
	f.add("D", "Globex", "")
	f.add("E", "Initech Old", "")
	f.add("F", "", "")
	f.add("G", "Initec Corporation", "")
	acme, err := f.orgs.Create(ctx, model.CreateOrgInput{Name: "Acme Corp"})
	require.NoError(t, err)
	old, err := f.orgs.Create(ctx, model.CreateOrgInput{Name: "Initech Old"})
	require.NoError(t, err)
	merged, err := f.orgs.Create(ctx, model.CreateOrgInput{Name: "Initech"})
	require.NoError(t, err)
	_, err = f.orgs.Create(ctx, model.CreateOrgInput{Name: "Globex Holdings"})
	require.NoError(t, err)
	require.NoError(t, f.orgs.Archive(ctx, old.ID))
	_, err = f.d.Exec(`INSERT INTO merge_alias (uuid, kind, old_id, old_uuid, survivor_id) VALUES ('x', 'organization', ?, ?, ?)`, old.ID, old.UUID, merged.ID)
	require.NoError(t, err)

	by, list, stats := f.groups(t)
	a := by["acme"]
	assert.Equal(t, 3, a.Count)
	assert.Len(t, a.Variants, 3, "three spellings")
	require.NotNil(t, a.Match)
	assert.Equal(t, "exact", a.Match.Kind)
	assert.Equal(t, acme.ID, a.Match.OrgID)
	assert.Equal(t, "acme", list[0].Key, "biggest group first")
	require.NotNil(t, by["initech old"].Match)
	assert.Equal(t, "alias", by["initech old"].Match.Kind)
	assert.Equal(t, merged.ID, by["initech old"].Match.OrgID)
	assert.Nil(t, by["globex"].Match, "Globex Holdings is not close enough")
	require.NotNil(t, by["initec"].Match)
	assert.Equal(t, "fuzzy", by["initec"].Match.Kind)
	assert.Equal(t, merged.ID, by["initec"].Match.OrgID)
	assert.False(t, by["initec"].Exact(), "a fuzzy match is a suggestion only")

	assert.Equal(t, 6, stats.People, "people with company text")
	assert.Equal(t, 4, stats.Groups)
	assert.Equal(t, 2, stats.Matched, "acme exact and the alias")
	assert.Equal(t, 1, stats.Fuzzy)
	assert.Equal(t, "6 people with company text, 4 groups, 2 matched", stats.Line())
}

func TestApplyCreatesLinksWithRoleAndRemovesReviewTag(t *testing.T) {
	f := convSetup(t)
	ctx := context.Background()
	a := f.add("A", "Acme Inc", "CTO")
	b := f.add("B", "ACME", "")
	require.NoError(t, f.tags.Apply(ctx, "person", b, repo.ReviewTag))
	by, _, _ := f.groups(t)
	assert.Equal(t, 1, by["acme"].Review, "the DAV-tagged person is flagged")

	// Nothing is created by looking.
	list, err := f.orgs.FindAll(ctx, 0)
	require.NoError(t, err)
	assert.Empty(t, list)

	ch, err := f.conv.Apply(ctx, []repo.ConvertDecision{{Key: "acme", Action: repo.ConvertCreate, Name: "Acme Corporation"}})
	require.NoError(t, err)
	assert.Equal(t, 1, ch.GroupsDone)
	assert.Equal(t, 2, ch.PeopleDone)
	require.Len(t, ch.OrgsCreated, 1)
	assert.Equal(t, []string{"Acme Corporation"}, ch.Created)

	items, err := f.links.Linked(ctx, repo.Ref{Kind: "person", ID: a}, "organization")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "Acme Corporation", items[0].Name, "the renamed organization")
	assert.True(t, items[0].Primary)
	require.NotNil(t, items[0].Role)
	assert.Equal(t, "CTO", *items[0].Role, "the title becomes the free-text role")
	assert.Nil(t, items[0].StartedOn)
	itemsB, _ := f.links.Linked(ctx, repo.Ref{Kind: "person", ID: b}, "organization")
	require.Len(t, itemsB, 1)
	assert.Nil(t, itemsB[0].Role)

	// people.company stays, the review tag goes.
	p, err := f.people.FindByID(ctx, a)
	require.NoError(t, err)
	assert.Equal(t, "Acme Inc", *p.Company)
	tags, err := f.tags.GetForEntity(ctx, "person", b)
	require.NoError(t, err)
	assert.Empty(t, tags)

	// Idempotent: the pass shows only what is left.
	by, _, stats := f.groups(t)
	assert.Empty(t, by)
	assert.Equal(t, 0, stats.People)
	ch2, err := f.conv.Apply(ctx, []repo.ConvertDecision{{Key: "acme", Action: repo.ConvertCreate}})
	require.NoError(t, err)
	assert.True(t, ch2.Empty(), "a group that is gone is skipped quietly, nothing is created twice")
	list, _ = f.orgs.FindAll(ctx, 0)
	assert.Len(t, list, 1)
}

func TestApplyOneUndoStepAndRedo(t *testing.T) {
	f := convSetup(t)
	ctx := context.Background()
	a := f.add("A", "Acme", "CTO")
	b := f.add("B", "Globex", "")
	exist, err := f.orgs.Create(ctx, model.CreateOrgInput{Name: "Globex"})
	require.NoError(t, err)
	require.NoError(t, f.tags.Apply(ctx, "person", b, repo.ReviewTag))

	ch, err := f.conv.Apply(ctx, []repo.ConvertDecision{
		{Key: "acme", Action: repo.ConvertCreate},
		{Key: "globex", Action: repo.ConvertLink, OrgID: exist.ID},
		{Key: "zzz", Action: repo.ConvertSkip},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, ch.GroupsDone)
	orgsNow, _ := f.orgs.FindAll(ctx, 0)
	assert.Len(t, orgsNow, 2)

	require.NoError(t, f.conv.Undo(ctx, ch))
	orgsNow, _ = f.orgs.FindAll(ctx, 0)
	require.Len(t, orgsNow, 1, "the created organization is gone")
	for _, id := range []int64{a, b} {
		items, err := f.links.Linked(ctx, repo.Ref{Kind: "person", ID: id}, "organization")
		require.NoError(t, err)
		assert.Empty(t, items)
	}
	tags, _ := f.tags.GetForEntity(ctx, "person", b)
	require.Len(t, tags, 1, "the review tag is back")
	by, _, _ := f.groups(t)
	assert.Len(t, by, 2, "both groups are back in the review")

	require.NoError(t, f.conv.Redo(ctx, ch))
	by, _, _ = f.groups(t)
	assert.Empty(t, by)
	items, _ := f.links.Linked(ctx, repo.Ref{Kind: "person", ID: a}, "organization")
	require.Len(t, items, 1)
	assert.True(t, items[0].Primary)
}

func TestSkipIsRememberedUntilTheTextChanges(t *testing.T) {
	f := convSetup(t)
	ctx := context.Background()
	p := f.add("A", "Mystery Ltd", "")
	_, err := f.conv.Apply(ctx, []repo.ConvertDecision{{Key: "mystery", Action: repo.ConvertSkip}})
	require.NoError(t, err)
	by, _, stats := f.groups(t)
	assert.Empty(t, by)
	assert.Equal(t, 1, stats.Skipped)
	all, _, err := f.conv.Groups(ctx, true)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.True(t, all[0].Skipped)

	// The text changes: a new key, so the group shows again.
	_, err = f.people.Update(ctx, p, model.UpdatePersonInput{Company: ptrS("Mystery Labs")})
	require.NoError(t, err)
	by, _, _ = f.groups(t)
	assert.Contains(t, by, "mystery labs")

	// A same-key spelling stays skipped, and unskip brings it back.
	f.add("B", "MYSTERY", "")
	_, err = f.conv.Apply(ctx, []repo.ConvertDecision{{Key: "mystery", Action: repo.ConvertSkip}})
	require.NoError(t, err)
	by, _, _ = f.groups(t)
	assert.NotContains(t, by, "mystery")
	_, err = f.conv.Apply(ctx, []repo.ConvertDecision{{Key: "mystery", Action: repo.ConvertUnskip}})
	require.NoError(t, err)
	by, _, _ = f.groups(t)
	assert.Contains(t, by, "mystery")
}

func TestPeopleWithAPrimaryOrganizationAreLeftOutUnlessTheTextDiffers(t *testing.T) {
	f := convSetup(t)
	ctx := context.Background()
	same := f.add("Same", "Acme Inc", "")
	differs := f.add("Differs", "Globex", "")
	acme, err := f.orgs.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	for _, pid := range []int64{same, differs} {
		_, err = f.links.Link(ctx, repo.Ref{Kind: "person", ID: pid}, repo.Ref{Kind: "organization", ID: acme.ID}, model.LinkAttrs{})
		require.NoError(t, err)
	}
	by, _, stats := f.groups(t)
	assert.NotContains(t, by, "acme", "already converted")
	require.Contains(t, by, "globex")
	assert.True(t, by["globex"].People[0].HasPrimary)
	assert.Equal(t, 1, stats.People)

	// Linking that person to another organization keeps the existing primary.
	globex, err := f.orgs.Create(ctx, model.CreateOrgInput{Name: "Globex"})
	require.NoError(t, err)
	_, err = f.conv.Apply(ctx, []repo.ConvertDecision{{Key: "globex", Action: repo.ConvertLink, OrgID: globex.ID}})
	require.NoError(t, err)
	items, _ := f.links.Linked(ctx, repo.Ref{Kind: "person", ID: differs}, "organization")
	require.Len(t, items, 2)
	assert.True(t, items[0].Primary)
	assert.Equal(t, acme.ID, items[0].ID, "the primary did not move")
	by, _, _ = f.groups(t)
	assert.Empty(t, by, "converted, even though the organization name differs from the text")
}

func TestBulkAcceptTakesOnlyExactMatches(t *testing.T) {
	f := convSetup(t)
	ctx := context.Background()
	f.add("A", "Acme Inc", "")
	f.add("B", "Globexs", "")
	_, err := f.orgs.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	_, err = f.orgs.Create(ctx, model.CreateOrgInput{Name: "Globex"})
	require.NoError(t, err)
	_, list, _ := f.groups(t)
	dec := repo.ExactDecisions(list)
	require.Len(t, dec, 1)
	assert.Equal(t, "acme", dec[0].Key)
	ch, err := f.conv.Apply(ctx, dec)
	require.NoError(t, err)
	assert.Equal(t, 1, ch.PeopleDone)
	by, _, _ := f.groups(t)
	assert.Contains(t, by, "globexs", "the fuzzy suggestion waits for a decision")
}

func TestApplyRejectsAnUnknownAction(t *testing.T) {
	f := convSetup(t)
	f.add("A", "Acme", "")
	_, err := f.conv.Apply(context.Background(), []repo.ConvertDecision{{Key: "acme", Action: "merge"}})
	assert.ErrorIs(t, err, model.ErrValidation)
	_, err = f.conv.Apply(context.Background(), []repo.ConvertDecision{{Key: "acme", Action: repo.ConvertLink, OrgID: 99}})
	assert.ErrorIs(t, err, model.ErrNotFound)
}

func ptrS(s string) *string { return &s }
