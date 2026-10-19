package repo_test

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (f *convFx) email(t *testing.T, id int64, email string) {
	t.Helper()
	require.NoError(t, f.people.AddEmail(context.Background(), id, email, nil, false))
}

func TestDominantDomainIgnoresFreeMailAndNeedsTwoPeopleToAgree(t *testing.T) {
	f := convSetup(t)
	// Acme: two people share acme.com, one also has a gmail address.
	a := f.add("A", "Acme", "")
	b := f.add("B", "Acme", "")
	f.email(t, a, "a@acme.com")
	f.email(t, a, "a@gmail.com")
	f.email(t, b, "b@ACME.com")
	f.email(t, b, "b@gmail.com")
	// Globex: two people, different domains, nothing shared.
	c := f.add("C", "Globex", "")
	d := f.add("D", "Globex", "")
	f.email(t, c, "c@globex.com")
	f.email(t, d, "d@other.org")
	// Initech: one person with a work address counts, Hooli: one person with only free mail does not.
	e := f.add("E", "Initech", "")
	f.email(t, e, "e@initech.io")
	g := f.add("G", "Hooli", "")
	f.email(t, g, "g@outlook.com")
	f.add("H", "Pied Piper", "")

	by, _, stats := f.groups(t)
	assert.Equal(t, "acme.com", by["acme"].Domain)
	assert.Equal(t, "", by["globex"].Domain, "two people with different domains have no dominant one")
	assert.Equal(t, "initech.io", by["initech"].Domain)
	assert.Equal(t, "", by["hooli"].Domain)
	assert.Equal(t, "", by["pied piper"].Domain)
	assert.Equal(t, 2, stats.Domains)
	assert.True(t, repo.IsFreeMailDomain("Gmail.com"))
	assert.False(t, repo.IsFreeMailDomain("acme.com"))
}

func TestDomainMatchIsASeparateSuggestion(t *testing.T) {
	f := convSetup(t)
	ctx := context.Background()
	dom := "https://www.Acme.com/about"
	org, err := f.orgs.Create(ctx, model.CreateOrgInput{Name: "Acme Holdings", Domain: &dom})
	require.NoError(t, err)
	a := f.add("A", "ACME Worldwide", "")
	b := f.add("B", "Acme Worldwide", "")
	f.email(t, a, "a@acme.com")
	f.email(t, b, "b@acme.com")

	by, list, stats := f.groups(t)
	g := by["acme worldwide"]
	require.NotNil(t, g.DomainMatch)
	assert.Equal(t, org.ID, g.DomainMatch.OrgID)
	assert.Equal(t, "domain", g.DomainMatch.Kind)
	assert.Nil(t, g.Match, "no name match")
	assert.False(t, g.Exact())
	assert.Equal(t, 1, stats.DomainMatches)
	assert.Equal(t, 0, stats.Matched)
	assert.Empty(t, repo.ExactDecisions(list), "bulk accept never takes a domain match")
	rec := g.Record(false)
	assert.Equal(t, "acme.com", rec["domain"])
	assert.Contains(t, rec["domain_match_text"], "Acme Holdings")
}

func TestDomainMatchWithTheSameOrgAsTheNameMatchIsNotShownTwice(t *testing.T) {
	f := convSetup(t)
	dom := "acme.com"
	_, err := f.orgs.Create(context.Background(), model.CreateOrgInput{Name: "Acme", Domain: &dom})
	require.NoError(t, err)
	a := f.add("A", "Acme Inc", "")
	f.email(t, a, "a@acme.com")
	by, list, stats := f.groups(t)
	assert.True(t, by["acme"].Exact())
	assert.Nil(t, by["acme"].DomainMatch)
	assert.Len(t, repo.ExactDecisions(list), 1, "the name match is still accepted in bulk")
	assert.Equal(t, 0, stats.DomainMatches)
}

func TestCreateSetsTheDominantDomainUnlessGiven(t *testing.T) {
	f := convSetup(t)
	ctx := context.Background()
	a := f.add("A", "Acme", "")
	b := f.add("B", "Acme", "")
	f.email(t, a, "a@acme.com")
	f.email(t, b, "b@acme.com")
	c := f.add("C", "Globex", "")
	f.email(t, c, "c@globex.com")
	h := f.add("H", "Hooli", "")
	f.email(t, h, "h@gmail.com")
	i := f.add("I", "Initech", "")
	f.email(t, i, "i@initech.com")

	plan, err := f.conv.Plan(ctx, []repo.ConvertDecision{{Key: "acme", Action: repo.ConvertCreate}, {Key: "hooli", Action: repo.ConvertCreate}})
	require.NoError(t, err)
	assert.Equal(t, "acme.com", plan[0].Domain)
	assert.Equal(t, "", plan[1].Domain)

	ch, err := f.conv.Apply(ctx, []repo.ConvertDecision{
		{Key: "acme", Action: repo.ConvertCreate},
		{Key: "globex", Action: repo.ConvertCreate, Domain: "globex.net"},
		{Key: "hooli", Action: repo.ConvertCreate},
		{Key: "initech", Action: repo.ConvertLink, OrgID: 0},
	})
	require.Error(t, err, "link to a missing organization fails and undoes the batch")
	assert.True(t, ch.Empty())
	orgs, err := f.orgs.FindAll(ctx, 0)
	require.NoError(t, err)
	assert.Empty(t, orgs)

	_, err = f.conv.Apply(ctx, []repo.ConvertDecision{
		{Key: "acme", Action: repo.ConvertCreate},
		{Key: "globex", Action: repo.ConvertCreate, Domain: "globex.net"},
		{Key: "hooli", Action: repo.ConvertCreate},
	})
	require.NoError(t, err)
	got := map[string]string{}
	orgs, err = f.orgs.FindAll(ctx, 0)
	require.NoError(t, err)
	for _, o := range orgs {
		d := ""
		if o.Domain != nil {
			d = *o.Domain
		}
		got[o.Name] = d
	}
	assert.Equal(t, map[string]string{"Acme": "acme.com", "Globex": "globex.net", "Hooli": ""}, got)
}
