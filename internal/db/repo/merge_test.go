package repo_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/dedupe"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openMergeDB(t *testing.T) (*sql.DB, *repo.PersonRepo) {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })
	return d, repo.NewPersonRepo(d)
}

func mustExec(t *testing.T, d *sql.DB, q string, args ...any) {
	t.Helper()
	_, err := d.Exec(q, args...)
	require.NoError(t, err, q)
}

func count(t *testing.T, d *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, d.QueryRow(q, args...).Scan(&n), q)
	return n
}

// addTask inserts a task about a person through activity_target.
func addTask(t *testing.T, d *sql.DB, uuid string, personID int64) {
	t.Helper()
	res, err := d.Exec(`INSERT INTO tasks (uuid, title) VALUES (?, 'Call')`, uuid)
	require.NoError(t, err)
	id, _ := res.LastInsertId()
	mustExec(t, d, `INSERT INTO activity_target (uuid, source_kind, source_id, target_kind, target_id) VALUES (?, 'task', ?, 'person', ?)`, "at-"+uuid, id, personID)
}

// addDeal inserts a deal with a person link.
func addDeal(t *testing.T, d *sql.DB, uuid string, personID int64, updatedAt string) {
	t.Helper()
	res, err := d.Exec(`INSERT INTO deals (uuid, title, updated_at) VALUES (?, 'Deal', ?)`, uuid, updatedAt)
	require.NoError(t, err)
	id, _ := res.LastInsertId()
	mustExec(t, d, `INSERT INTO deal_person (uuid, deal_id, person_id) VALUES (?, ?, ?)`, "dp-"+uuid, id, personID)
}

func TestMerge_CombinesListsAndMovesRecords(t *testing.T) {
	d, pr := openMergeDB(t)
	ctx := context.Background()

	keep, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Jane", LastName: ptr("Smith"), Email: ptr("jane@work.com"), Title: ptr("CEO")})
	require.NoError(t, err)
	drop, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Janet", LastName: ptr("Smith"), Email: ptr("JANE@work.com"), Phone: ptr("+1 555 123 4567"), Title: ptr("CTO"), Company: ptr("Acme"), Location: ptr("Paris")})
	require.NoError(t, err)
	require.NoError(t, pr.AddEmail(ctx, drop.ID, "jane@home.com", nil, false))
	require.NoError(t, pr.SetHandle(ctx, keep.ID, "linkedin", "linkedin.com/in/keep"))
	require.NoError(t, pr.SetHandle(ctx, drop.ID, "linkedin", "linkedin.com/in/drop"))
	require.NoError(t, pr.SetHandle(ctx, drop.ID, "twitter", "jane"))

	// Tags, custom fields, interactions, tasks, deals, relationships.
	tags := repo.NewTagRepo(d)
	require.NoError(t, tags.Apply(ctx, "person", keep.ID, "vip"))
	require.NoError(t, tags.Apply(ctx, "person", drop.ID, "vip"))
	require.NoError(t, tags.Apply(ctx, "person", drop.ID, "friend"))
	mustExec(t, d, `INSERT INTO custom_fields (entity_type, entity_id, field_name, field_value) VALUES ('person', ?, 'pet', 'cat'), ('person', ?, 'pet', 'dog'), ('person', ?, 'car', 'vw')`, keep.ID, drop.ID, drop.ID)
	mustExec(t, d, `INSERT INTO interactions (uuid, type) VALUES ('i1', 'call'), ('i2', 'call')`)
	mustExec(t, d, `INSERT INTO activity_target (uuid, source_kind, source_id, target_kind, target_id) VALUES
		('a1', 'interaction', 1, 'person', ?), ('a2', 'interaction', 1, 'person', ?), ('a3', 'interaction', 2, 'person', ?)`, keep.ID, drop.ID, drop.ID)
	addTask(t, d, "t1", drop.ID)
	addDeal(t, d, "d1", drop.ID, "2000-01-01 00:00:00")
	other, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Other"})
	require.NoError(t, err)
	mustExec(t, d, `INSERT INTO person_relation (uuid, a_id, b_id, kind_id) VALUES
		('r1', ?, ?, (SELECT id FROM relation_kind WHERE name = 'friend')),
		('r2', ?, ?, (SELECT id FROM relation_kind WHERE name = 'friend')),
		('r3', ?, ?, (SELECT id FROM relation_kind WHERE name = 'colleague'))`,
		keep.ID, other.ID, // the survivor already has this one
		drop.ID, other.ID, // duplicate of it once moved
		keep.ID, drop.ID, // becomes a self link
	)
	before := count(t, d, `SELECT COUNT(*) FROM person_relation`)

	res, err := pr.Merge(ctx, keep.ID, []int64{drop.ID}, nil)
	require.NoError(t, err)
	assert.False(t, res.DryRun)

	// Scalars: survivor wins, empty fields fill from the drop.
	got, err := pr.FindByID(ctx, keep.ID)
	require.NoError(t, err)
	assert.Equal(t, "CEO", *got.Title)
	assert.Equal(t, "Acme", *got.Company)
	assert.Equal(t, "Paris", *got.Location)
	assert.Equal(t, "jane@work.com", *got.Email)
	assert.Equal(t, "+1 555 123 4567", *got.Phone, "empty primary phone takes the dropped person's")

	// Lists are combined without case-insensitive duplicates.
	emails, _ := pr.Emails(ctx, keep.ID)
	require.Len(t, emails, 2)
	assert.Equal(t, "jane@work.com", emails[0].Email)
	assert.True(t, emails[0].IsPrimary)
	assert.Equal(t, "jane@home.com", emails[1].Email)
	phones, _ := pr.Phones(ctx, keep.ID)
	require.Len(t, phones, 1)
	assert.True(t, phones[0].IsPrimary)
	handles, _ := pr.Handles(ctx, keep.ID)
	require.Len(t, handles, 2)
	assert.Equal(t, "linkedin.com/in/keep", handles[0].Value, "survivor handle wins")
	assert.Equal(t, "twitter", handles[1].Kind)
	assert.Equal(t, 2, count(t, d, `SELECT COUNT(*) FROM taggings WHERE entity_type='person' AND entity_id=?`, keep.ID))
	assert.Equal(t, 2, count(t, d, `SELECT COUNT(*) FROM custom_fields WHERE entity_type='person' AND entity_id=?`, keep.ID))
	assert.Equal(t, "cat", func() string {
		var v string
		_ = d.QueryRow(`SELECT field_value FROM custom_fields WHERE entity_type='person' AND entity_id=? AND field_name='pet'`, keep.ID).Scan(&v)
		return v
	}())

	// Records move.
	live := func(table, where string, args ...any) int {
		return count(t, d, `SELECT COUNT(*) FROM `+table+` WHERE deleted_at IS NULL AND `+where, args...)
	}
	assert.Equal(t, 2, live("activity_target", "source_kind = 'interaction' AND target_kind = 'person' AND target_id = ?", keep.ID))
	assert.Equal(t, 1, live("activity_target", "source_kind = 'task' AND target_kind = 'person' AND target_id = ?", keep.ID))
	assert.Equal(t, 1, live("deal_person", "person_id = ?", keep.ID))
	assert.Equal(t, before, count(t, d, `SELECT COUNT(*) FROM person_relation`), "link rows are kept, duplicates are soft deleted")
	assert.Equal(t, 1, live("person_relation", "1 = 1"), "one friend link is left: the duplicate and the self link are dropped")
	assert.Equal(t, 1, live("person_relation", "a_id = ? AND b_id = ?", keep.ID, other.ID))
	assert.Equal(t, 1, count(t, d, `SELECT COUNT(*) FROM merge_alias WHERE kind = 'person' AND old_id = ? AND survivor_id = ?`, drop.ID, keep.ID))

	// The dropped person is archived, points at the survivor and keeps its original fields.
	var archived int
	var mergedInto sql.NullInt64
	var title string
	require.NoError(t, d.QueryRow(`SELECT archived, merged_into, title FROM people WHERE id=?`, drop.ID).Scan(&archived, &mergedInto, &title))
	assert.Equal(t, 1, archived)
	assert.Equal(t, keep.ID, mergedInto.Int64)
	assert.Equal(t, "CTO", title)
	_, err = pr.FindByID(ctx, drop.ID)
	assert.ErrorIs(t, err, model.ErrNotFound)

	assert.Equal(t, 1, res.Added["emails"])
	assert.Equal(t, 1, res.Added["phones"])
	assert.Equal(t, 1, res.Added["handles"])
	assert.Equal(t, 1, res.Added["tags"])
	assert.Equal(t, 1, res.Added["custom_fields"])
	assert.Equal(t, 1, res.Moved["tasks"])
	assert.Equal(t, 1, res.Moved["deals"])
}

func TestMerge_FieldChoices(t *testing.T) {
	d, pr := openMergeDB(t)
	ctx := context.Background()
	keep, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Jane", LastName: ptr("Smith"), Title: ptr("CEO"), Email: ptr("a@x.com"), Phone: ptr("111 111 1111"), Notes: ptr("keep notes")})
	drop, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Janet", LastName: ptr("Smith"), Title: ptr("CTO"), Email: ptr("b@x.com"), Phone: ptr("222 222 2222"), Notes: ptr("drop notes")})
	require.NoError(t, pr.SetHandle(ctx, keep.ID, "linkedin", "keep"))
	require.NoError(t, pr.SetHandle(ctx, drop.ID, "linkedin", "drop"))
	mustExec(t, d, `UPDATE people SET birthday_month=3, birthday_day=14, is_favorite=1, priority_tier=2 WHERE id=?`, drop.ID)

	res, err := pr.Merge(ctx, keep.ID, []int64{drop.ID}, map[string]int64{
		"title": drop.ID, "first_name": drop.ID, "email": drop.ID, "phone": drop.ID, "linkedin": drop.ID, "notes": keep.ID,
	})
	require.NoError(t, err)

	got, _ := pr.FindByID(ctx, keep.ID)
	assert.Equal(t, "CTO", *got.Title)
	assert.Equal(t, "Janet", got.FirstName)
	assert.Equal(t, "keep notes", *got.Notes)
	assert.Equal(t, "b@x.com", *got.Email)
	assert.Equal(t, "222 222 2222", *got.Phone)
	emails, _ := pr.Emails(ctx, keep.ID)
	require.Len(t, emails, 2)
	assert.Equal(t, "b@x.com", emails[0].Email, "chosen email is the primary")
	assert.True(t, emails[0].IsPrimary)
	assert.False(t, emails[1].IsPrimary)
	handles, _ := pr.Handles(ctx, keep.ID)
	require.Len(t, handles, 1)
	assert.Equal(t, "drop", handles[0].Value)

	// Empty survivor fields fall back to the drop without a choice. Birthday moves as a unit.
	require.NotNil(t, got.BirthdayMonth)
	assert.Equal(t, 3, *got.BirthdayMonth)
	assert.Equal(t, 14, *got.BirthdayDay)
	assert.True(t, got.IsFavorite)
	require.NotNil(t, got.PriorityTier)

	fields := map[string]model.MergeFieldChange{}
	for _, f := range res.Fields {
		fields[f.Field] = f
	}
	assert.Equal(t, drop.ID, fields["title"].From)
	assert.Equal(t, "CEO", fields["title"].Old)
	assert.Equal(t, "CTO", fields["title"].New)
	assert.Contains(t, fields, "birthday")
	assert.NotContains(t, fields, "notes", "choosing the survivor is not a change")
}

func TestMerge_Validation(t *testing.T) {
	d, pr := openMergeDB(t)
	ctx := context.Background()
	a, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "A"})
	b, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "B"})
	c, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "C"})
	require.NoError(t, pr.Archive(ctx, c.ID))

	tests := []struct {
		name    string
		keep    int64
		drops   []int64
		choices map[string]int64
		want    error
	}{
		{"no drops", a.ID, nil, nil, model.ErrValidation},
		{"drop equals keep", a.ID, []int64{a.ID}, nil, model.ErrValidation},
		{"duplicate drop", a.ID, []int64{b.ID, b.ID}, nil, model.ErrValidation},
		{"unknown field", a.ID, []int64{b.ID}, map[string]int64{"shoe_size": a.ID}, model.ErrValidation},
		{"choice from outside", a.ID, []int64{b.ID}, map[string]int64{"title": 99}, model.ErrValidation},
		{"missing person", a.ID, []int64{99}, nil, model.ErrNotFound},
		{"archived drop", a.ID, []int64{c.ID}, nil, model.ErrValidation},
		{"handle missing on chosen", a.ID, []int64{b.ID}, map[string]int64{"linkedin": b.ID}, model.ErrValidation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pr.Merge(ctx, tt.keep, tt.drops, tt.choices)
			assert.ErrorIs(t, err, tt.want)
		})
	}
	assert.Equal(t, 0, count(t, d, `SELECT COUNT(*) FROM people WHERE merged_into IS NOT NULL`))
}

func TestMerge_RollsBackOnFailure(t *testing.T) {
	d, pr := openMergeDB(t)
	ctx := context.Background()
	keep, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Keep", Title: ptr("T")})
	drop, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Drop", Company: ptr("Acme"), Email: ptr("d@x.com")})
	addTask(t, d, "t1", drop.ID)

	// Make the final archive step fail after earlier steps have written.
	mustExec(t, d, `CREATE TRIGGER fail_archive BEFORE UPDATE OF archived ON people WHEN NEW.archived = 1 BEGIN SELECT RAISE(ABORT, 'boom'); END`)

	_, err := pr.Merge(ctx, keep.ID, []int64{drop.ID}, nil)
	require.Error(t, err)

	got, _ := pr.FindByID(ctx, keep.ID)
	assert.Nil(t, got.Company, "scalar update rolled back")
	assert.Nil(t, got.Email, "primary email rolled back")
	assert.Equal(t, 0, count(t, d, `SELECT COUNT(*) FROM person_emails WHERE person_id=?`, keep.ID))
	assert.Equal(t, 1, count(t, d, `SELECT COUNT(*) FROM activity_target WHERE source_kind = 'task' AND target_id = ?`, drop.ID), "task move rolled back")
	assert.Equal(t, 0, count(t, d, `SELECT COUNT(*) FROM merge_alias`), "alias rolled back")
	assert.Equal(t, 0, count(t, d, `SELECT COUNT(*) FROM people WHERE archived=1`))
}

func TestMerge_DryRunChangesNothing(t *testing.T) {
	d, pr := openMergeDB(t)
	ctx := context.Background()
	keep, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Keep"})
	drop, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Drop", Company: ptr("Acme"), Email: ptr("d@x.com")})
	addTask(t, d, "t1", drop.ID)

	res, err := pr.MergeDryRun(ctx, keep.ID, []int64{drop.ID}, nil)
	require.NoError(t, err)
	assert.True(t, res.DryRun)
	assert.Equal(t, 1, res.Moved["tasks"])
	assert.Equal(t, 1, res.Added["emails"])

	got, _ := pr.FindByID(ctx, keep.ID)
	assert.Nil(t, got.Company)
	assert.Equal(t, 1, count(t, d, `SELECT COUNT(*) FROM activity_target WHERE source_kind = 'task' AND target_id = ?`, drop.ID))
	assert.Equal(t, 0, count(t, d, `SELECT COUNT(*) FROM merge_alias`))
	_, err = pr.FindByID(ctx, drop.ID)
	assert.NoError(t, err, "dropped person is still active")
}

func TestMerge_ClearsDismissalsAndChainsMergedInto(t *testing.T) {
	d, pr := openMergeDB(t)
	ctx := context.Background()
	dr := repo.NewDupeRepo(d)
	a, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "A"})
	b, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "B"})
	c, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "C"})
	require.NoError(t, dr.Dismiss(ctx, a.ID, b.ID))
	require.NoError(t, dr.Dismiss(ctx, a.ID, c.ID))

	_, err := pr.Merge(ctx, b.ID, []int64{a.ID}, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, count(t, d, `SELECT COUNT(*) FROM dupe_dismissals WHERE archived = 0`), "only the merged pair is cleared")

	// Merging b away repoints people already merged into b.
	_, err = pr.Merge(ctx, c.ID, []int64{b.ID}, nil)
	require.NoError(t, err)
	var into int64
	require.NoError(t, d.QueryRow(`SELECT merged_into FROM people WHERE id=?`, a.ID).Scan(&into))
	assert.Equal(t, c.ID, into)
}

func TestMerge_UpdatesTimestamps(t *testing.T) {
	d, pr := openMergeDB(t)
	ctx := context.Background()
	keep, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Keep"})
	drop, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Drop"})
	mustExec(t, d, `UPDATE people SET updated_at = '2000-01-01 00:00:00'`)
	addTask(t, d, "t1", drop.ID)
	addDeal(t, d, "d1", drop.ID, "2000-01-01 00:00:00")
	mustExec(t, d, `UPDATE tasks SET updated_at = '2000-01-01 00:00:00'`)

	_, err := pr.Merge(ctx, keep.ID, []int64{drop.ID}, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, count(t, d, `SELECT COUNT(*) FROM people WHERE updated_at = '2000-01-01 00:00:00'`))
	assert.Equal(t, 0, count(t, d, `SELECT COUNT(*) FROM tasks WHERE updated_at = '2000-01-01 00:00:00'`))
	assert.Equal(t, 0, count(t, d, `SELECT COUNT(*) FROM deals WHERE updated_at = '2000-01-01 00:00:00'`))
}

func TestDupeRepo_FindAndDismiss(t *testing.T) {
	d, pr := openMergeDB(t)
	ctx := context.Background()
	dr := repo.NewDupeRepo(d)

	a, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Jane", LastName: ptr("Smith"), Email: ptr("j1@x.com")})
	mustExec(t, d, `INSERT INTO people (uuid, first_name, last_name) VALUES ('dup-b', 'Jane', 'Smith')`)
	b, err := pr.FindByID(ctx, 2)
	require.NoError(t, err)
	require.NoError(t, pr.AddEmail(ctx, b.ID, "j2@x.com", nil, false))
	require.NoError(t, pr.AddEmail(ctx, b.ID, "J1@X.com", nil, false))
	cc, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Zed", LastName: ptr("Quux")})

	groups, err := dr.Find(ctx, dedupe.Medium)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, dedupe.High, groups[0].Level, "person_emails feed the shared email check")
	assert.Equal(t, []int64{a.ID, b.ID}, groups[0].IDs())

	// Dismissal hides the pair, in either argument order, and is idempotent.
	require.NoError(t, dr.Dismiss(ctx, b.ID, a.ID))
	require.NoError(t, dr.Dismiss(ctx, a.ID, b.ID))
	groups, _ = dr.Find(ctx, dedupe.Fuzzy)
	assert.Empty(t, groups)
	assert.Equal(t, 1, count(t, d, `SELECT COUNT(*) FROM dupe_dismissals`))

	// Undismiss is a soft delete and brings the group back.
	require.NoError(t, dr.Undismiss(ctx, a.ID, b.ID))
	assert.Equal(t, 1, count(t, d, `SELECT COUNT(*) FROM dupe_dismissals WHERE archived = 1`))
	groups, _ = dr.Find(ctx, dedupe.Medium)
	assert.Len(t, groups, 1)
	assert.ErrorIs(t, dr.Undismiss(ctx, a.ID, b.ID), model.ErrNotFound)

	// Dismissing again revives the same row.
	require.NoError(t, dr.Dismiss(ctx, a.ID, b.ID))
	assert.Equal(t, 1, count(t, d, `SELECT COUNT(*) FROM dupe_dismissals WHERE archived = 0`))

	assert.ErrorIs(t, dr.Dismiss(ctx, a.ID, a.ID), model.ErrValidation)
	assert.ErrorIs(t, dr.Dismiss(ctx, a.ID, 999), model.ErrNotFound)
	assert.NoError(t, dr.Dismiss(ctx, a.ID, cc.ID))
}
