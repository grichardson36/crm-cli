package repo_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func listStrings(t *testing.T, d *sql.DB, q string, args ...any) []string {
	t.Helper()
	rows, err := d.Query(q, args...)
	require.NoError(t, err, q)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s sql.NullString
		require.NoError(t, rows.Scan(&s))
		out = append(out, s.String)
	}
	require.NoError(t, rows.Err())
	return out
}

func TestMergeWith_ExcludesEmailsPhonesTagsHandlesAndFields(t *testing.T) {
	d, pr := openMergeDB(t)
	ctx := context.Background()
	keep, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Jane", LastName: ptr("Smith"), Email: ptr("a@x.com"), Phone: ptr("+1 555 000 1111")})
	require.NoError(t, err)
	drop, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Janet", LastName: ptr("Smith"), Email: ptr("c@x.com"), Phone: ptr("+1 555 000 2222")})
	require.NoError(t, err)
	require.NoError(t, pr.AddEmail(ctx, keep.ID, "b@x.com", nil, false))
	require.NoError(t, pr.AddEmail(ctx, drop.ID, "d@x.com", nil, false))
	require.NoError(t, pr.SetHandle(ctx, keep.ID, "linkedin", "li/keep"))
	require.NoError(t, pr.SetHandle(ctx, keep.ID, "twitter", "keeptw"))
	require.NoError(t, pr.SetHandle(ctx, drop.ID, "linkedin", "li/drop"))
	require.NoError(t, pr.SetHandle(ctx, drop.ID, "website", "dropsite"))
	tags := repo.NewTagRepo(d)
	require.NoError(t, tags.Apply(ctx, "person", keep.ID, "vip"))
	require.NoError(t, tags.Apply(ctx, "person", drop.ID, "friend"))
	require.NoError(t, tags.Apply(ctx, "person", drop.ID, "lead"))
	mustExec(t, d, `INSERT INTO custom_fields (entity_type, entity_id, field_name, field_value) VALUES
		('person', ?, 'pet', 'cat'), ('person', ?, 'pet', 'dog'), ('person', ?, 'car', 'vw'), ('person', ?, 'boat', 'ib31'), ('person', ?, 'bike', 'bmx')`,
		keep.ID, drop.ID, drop.ID, drop.ID, keep.ID)

	opts := model.MergeOptions{
		Exclude: map[string][]string{
			"email":        {"B@x.com", "c@x.com"}, // the survivor's b and the dropped person's c
			"phone":        {"+15550001111"},       // the survivor's phone, written differently
			"tag":          {"VIP", "lead"},
			"handle":       {"twitter"},
			"custom_field": {"boat", "bike"},
		},
		CustomFrom: map[string]int64{"pet": drop.ID},
	}
	choices := map[string]int64{"linkedin": drop.ID}

	// A dry run reports the same and changes nothing.
	dry, err := pr.MergeDryRunWith(ctx, keep.ID, []int64{drop.ID}, choices, opts)
	require.NoError(t, err)
	assert.True(t, dry.DryRun)
	assert.Equal(t, 1, dry.Removed["emails"])
	assert.Equal(t, 2, count(t, d, `SELECT COUNT(*) FROM person_emails WHERE person_id = ?`, keep.ID))

	res, err := pr.MergeWith(ctx, keep.ID, []int64{drop.ID}, choices, opts)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Removed["emails"])
	assert.Equal(t, 1, res.Removed["phones"])
	assert.Equal(t, 1, res.Removed["tags"])
	assert.Equal(t, 1, res.Removed["handles"])
	assert.Equal(t, 1, res.Removed["custom_fields"])
	assert.Equal(t, 1, res.Added["emails"], "only d came over")
	assert.Equal(t, 1, res.Added["tags"], "friend came over, lead did not")

	assert.ElementsMatch(t, []string{"a@x.com", "d@x.com"}, listStrings(t, d, `SELECT email FROM person_emails WHERE person_id = ?`, keep.ID))
	assert.ElementsMatch(t, []string{"+1 555 000 2222"}, listStrings(t, d, `SELECT phone_number FROM person_phones WHERE person_id = ?`, keep.ID))
	assert.Equal(t, []string{"+1 555 000 2222"}, listStrings(t, d, `SELECT phone FROM people WHERE id = ?`, keep.ID), "the mirror column follows")
	assert.Equal(t, []string{"a@x.com"}, listStrings(t, d, `SELECT email FROM people WHERE id = ?`, keep.ID))
	assert.Equal(t, []string{"friend"}, listStrings(t, d, `SELECT t.name FROM taggings g JOIN tags t ON t.id = g.tag_id WHERE g.entity_type = 'person' AND g.entity_id = ? AND g.deleted_at IS NULL`, keep.ID))
	assert.ElementsMatch(t, []string{"website=dropsite", "linkedin=li/drop"},
		listStrings(t, d, `SELECT kind || '=' || value FROM person_handles WHERE person_id = ?`, keep.ID))
	assert.ElementsMatch(t, []string{"pet=dog", "car=vw"},
		listStrings(t, d, `SELECT field_name || '=' || field_value FROM custom_fields WHERE entity_type = 'person' AND entity_id = ?`, keep.ID))
	assert.Equal(t, 1, count(t, d, `SELECT archived FROM people WHERE id = ?`, drop.ID))
	assert.Equal(t, int(keep.ID), count(t, d, `SELECT merged_into FROM people WHERE id = ?`, drop.ID))
}

func TestMergeWith_ExcludingThePrimaryPromotesAnotherEmail(t *testing.T) {
	d, pr := openMergeDB(t)
	ctx := context.Background()
	keep, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Jane", Email: ptr("a@x.com")})
	require.NoError(t, err)
	drop, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Janet", Email: ptr("c@x.com")})
	require.NoError(t, err)
	require.NoError(t, pr.AddEmail(ctx, keep.ID, "b@x.com", nil, false))

	_, err = pr.MergeWith(ctx, keep.ID, []int64{drop.ID}, nil, model.MergeOptions{Exclude: map[string][]string{"email": {"a@x.com"}}})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"b@x.com", "c@x.com"}, listStrings(t, d, `SELECT email FROM person_emails WHERE person_id = ?`, keep.ID))
	assert.Equal(t, 1, count(t, d, `SELECT COUNT(*) FROM person_emails WHERE person_id = ? AND is_primary = 1`, keep.ID))
	assert.Equal(t, []string{"b@x.com"}, listStrings(t, d, `SELECT email FROM people WHERE id = ?`, keep.ID), "the survivor's next email is primary")
}

func TestMergeWith_ExcludingEveryEmailLeavesNone(t *testing.T) {
	d, pr := openMergeDB(t)
	ctx := context.Background()
	keep, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Jane", Email: ptr("a@x.com")})
	require.NoError(t, err)
	drop, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Janet", Email: ptr("c@x.com")})
	require.NoError(t, err)
	_, err = pr.MergeWith(ctx, keep.ID, []int64{drop.ID}, nil, model.MergeOptions{Exclude: map[string][]string{"email": {"a@x.com", "c@x.com"}}})
	require.NoError(t, err)
	assert.Equal(t, 0, count(t, d, `SELECT COUNT(*) FROM person_emails WHERE person_id = ?`, keep.ID))
	assert.Equal(t, []string{""}, listStrings(t, d, `SELECT email FROM people WHERE id = ?`, keep.ID))
}

func TestMergeWith_RejectsCustomFieldSourceOutsideTheMerge(t *testing.T) {
	_, pr := openMergeDB(t)
	ctx := context.Background()
	keep, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Jane"})
	require.NoError(t, err)
	drop, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Janet"})
	require.NoError(t, err)
	_, err = pr.MergeWith(ctx, keep.ID, []int64{drop.ID}, nil, model.MergeOptions{CustomFrom: map[string]int64{"pet": 999}})
	require.Error(t, err)
	assert.ErrorIs(t, err, model.ErrValidation)
}

func TestMerge_WithoutOptionsIsUnchangedAndHasNoRemoved(t *testing.T) {
	_, pr := openMergeDB(t)
	ctx := context.Background()
	keep, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Jane", Email: ptr("a@x.com")})
	require.NoError(t, err)
	drop, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Janet", Email: ptr("c@x.com")})
	require.NoError(t, err)
	res, err := pr.Merge(ctx, keep.ID, []int64{drop.ID}, nil)
	require.NoError(t, err)
	assert.Nil(t, res.Removed)
	assert.Equal(t, 1, res.Added["emails"])
}
