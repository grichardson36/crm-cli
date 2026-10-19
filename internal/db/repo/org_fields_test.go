package repo_test

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrgNativeFieldsRoundTripAndMergeFill(t *testing.T) {
	f := omSetup(t)
	ctx := context.Background()
	keep := f.org(t, "Acme", "")
	web, size, city, country, desc, logo := "https://acme.com", "51-200", "Austin", "US", "Rockets", "logos/acme.png"
	o, err := f.orgs.Create(ctx, model.CreateOrgInput{Name: "ACME", AllowDuplicate: true,
		OrgNativeFields: model.OrgNativeFields{Website: &web, Size: &size, City: &city, Country: &country, Description: &desc, LogoPath: &logo}})
	require.NoError(t, err)
	assert.Equal(t, "Austin, US", o.Location())

	got, err := f.orgs.FindByID(ctx, o.ID)
	require.NoError(t, err)
	assert.Equal(t, web, *got.Website)
	assert.Equal(t, "51-200", *got.Size)
	assert.Nil(t, got.Region)

	li := "https://linkedin.com/company/acme"
	region := "TX"
	upd, err := f.orgs.Update(ctx, o.ID, model.UpdateOrgInput{OrgNativeFields: model.OrgNativeFields{LinkedinURL: &li, Region: &region}})
	require.NoError(t, err)
	assert.Equal(t, "Austin, TX, US", upd.Location())
	assert.Equal(t, web, *upd.Website, "a field left out is unchanged")

	// Overwrite sets all, including back to empty.
	before := *upd
	cleared := *upd
	cleared.Website, cleared.Size = nil, nil
	require.NoError(t, f.orgs.Overwrite(ctx, o.ID, &cleared))
	got, _ = f.orgs.FindByID(ctx, o.ID)
	assert.Nil(t, got.Website)
	require.NoError(t, f.orgs.Overwrite(ctx, o.ID, &before))
	got, _ = f.orgs.FindByID(ctx, o.ID)
	assert.Equal(t, web, *got.Website)

	// A merge fills the survivor's empty native fields from the duplicate.
	res, err := f.orgs.MergeOrgs(ctx, keep, []int64{o.ID})
	require.NoError(t, err)
	names := map[string]bool{}
	for _, c := range res.Fields {
		names[c.Field] = true
	}
	for _, want := range []string{"website", "size", "city", "region", "country", "linkedin_url", "description", "logo_path"} {
		assert.True(t, names[want], want)
	}
	k, _ := f.orgs.FindByID(ctx, keep)
	assert.Equal(t, "Austin, TX, US", k.Location())
	require.NoError(t, f.orgs.UndoMerge(ctx, res))
	k, _ = f.orgs.FindByID(ctx, keep)
	assert.Nil(t, k.Website)
}

func TestSplitLocation(t *testing.T) {
	s := func(p *string) string {
		if p == nil {
			return "-"
		}
		return *p
	}
	for in, want := range map[string]string{
		"Austin":                           "Austin|-|-",
		"Austin, US":                       "Austin|-|US",
		"Austin, TX, US":                   "Austin|TX|US",
		", TX, US":                         "-|TX|US",
		"Paris, Ile-de-France, FR, Europe": "Paris|Ile-de-France|FR, Europe",
	} {
		c, r, co := model.SplitLocation(in)
		assert.Equal(t, want, s(c)+"|"+s(r)+"|"+s(co), in)
	}
}
