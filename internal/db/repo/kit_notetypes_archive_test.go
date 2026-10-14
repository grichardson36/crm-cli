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

func openRaw(t *testing.T) *sql.DB {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

func setAtOf(t *testing.T, d *sql.DB, id int64) sql.NullString {
	t.Helper()
	var s sql.NullString
	require.NoError(t, d.QueryRow(`SELECT keep_in_touch_set_at FROM people WHERE id = ?`, id).Scan(&s))
	return s
}

func TestKeepInTouch_SetAtTracksCadenceChanges(t *testing.T) {
	d := openRaw(t)
	r := repo.NewPersonRepo(d)
	ctx := context.Background()
	p, err := r.Create(ctx, model.CreatePersonInput{FirstName: "Jane"})
	require.NoError(t, err)
	assert.False(t, setAtOf(t, d, p.ID).Valid, "no cadence, no stamp")

	_, err = r.Update(ctx, p.ID, model.UpdatePersonInput{KeepInTouch: ptr("1 mon")})
	require.NoError(t, err)
	assert.True(t, setAtOf(t, d, p.ID).Valid, "setting a cadence stamps it")

	// Backdate the stamp, then re-set the same cadence: the stamp must not move.
	_, err = d.Exec(`UPDATE people SET keep_in_touch_set_at = '2020-01-01 00:00:00' WHERE id = ?`, p.ID)
	require.NoError(t, err)
	_, err = r.Update(ctx, p.ID, model.UpdatePersonInput{KeepInTouch: ptr("1 mon")})
	require.NoError(t, err)
	assert.Equal(t, "2020-01-01 00:00:00", setAtOf(t, d, p.ID).String)

	// Changing the cadence restamps.
	_, err = r.Update(ctx, p.ID, model.UpdatePersonInput{KeepInTouch: ptr("3 mons")})
	require.NoError(t, err)
	assert.NotEqual(t, "2020-01-01 00:00:00", setAtOf(t, d, p.ID).String)

	// Clearing removes it.
	_, err = r.Update(ctx, p.ID, model.UpdatePersonInput{KeepInTouch: ptr("")})
	require.NoError(t, err)
	assert.False(t, setAtOf(t, d, p.ID).Valid)
}

func TestDue_NeverContactedStartsFromSetAt(t *testing.T) {
	d := openRaw(t)
	r := repo.NewPersonRepo(d)
	ctx := context.Background()
	// Created long ago, cadence set just now: not due.
	p, err := r.Create(ctx, model.CreatePersonInput{FirstName: "Old"})
	require.NoError(t, err)
	_, err = d.Exec(`UPDATE people SET created_at = '2015-01-01 00:00:00' WHERE id = ?`, p.ID)
	require.NoError(t, err)
	_, err = r.Update(ctx, p.ID, model.UpdatePersonInput{KeepInTouch: ptr("7 days")})
	require.NoError(t, err)

	due, err := r.Due(ctx, 0, 50)
	require.NoError(t, err)
	assert.Empty(t, due, "first due is cadence after it was set, not after created_at")

	due, err = r.Due(ctx, 8, 50)
	require.NoError(t, err)
	assert.Len(t, due, 1, "it comes due a week after the cadence was set")

	// An older stamp makes the person overdue.
	_, err = d.Exec(`UPDATE people SET keep_in_touch_set_at = '2020-01-01 00:00:00' WHERE id = ?`, p.ID)
	require.NoError(t, err)
	due, err = r.Due(ctx, 0, 50)
	require.NoError(t, err)
	assert.Len(t, due, 1)
}

func TestNoteTypes_SeedAddArchive(t *testing.T) {
	d := openRaw(t)
	nr := repo.NewNoteTypeRepo(d)
	ctx := context.Background()

	types, err := nr.List(ctx, false)
	require.NoError(t, err)
	names := map[string]bool{}
	for _, nt := range types {
		names[nt.Name] = nt.CountsAsContact
	}
	assert.Equal(t, map[string]bool{"call": true, "email": true, "meeting": true, "message": true, "note": false}, names)

	nt, err := nr.Add(ctx, "standup", true)
	require.NoError(t, err)
	assert.NotEmpty(t, nt.UUID)
	assert.True(t, nt.CountsAsContact)

	_, err = nr.Add(ctx, "STANDUP", true)
	assert.ErrorIs(t, err, model.ErrConflict, "names are unique case-insensitively")
	for _, bad := range []string{"", "has space", "-lead", "a/b"} {
		_, err = nr.Add(ctx, bad, true)
		assert.ErrorIs(t, err, model.ErrValidation, "name %q", bad)
	}

	require.NoError(t, nr.Archive(ctx, "Standup"))
	assert.ErrorIs(t, nr.Archive(ctx, "standup"), model.ErrNotFound, "already archived")
	active, _ := nr.List(ctx, false)
	all, _ := nr.List(ctx, true)
	assert.Len(t, all, len(active)+1)

	// Adding an archived name restores it with the new setting.
	back, err := nr.Add(ctx, "standup", false)
	require.NoError(t, err)
	assert.False(t, back.Archived)
	assert.False(t, back.CountsAsContact)
	assert.Equal(t, nt.ID, back.ID)
}

func TestInteractionCreate_ValidatesAgainstNoteTypes(t *testing.T) {
	d := openRaw(t)
	pr := repo.NewPersonRepo(d)
	ir := repo.NewInteractionRepo(d)
	nr := repo.NewNoteTypeRepo(d)
	ctx := context.Background()
	p, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Jane"})
	require.NoError(t, err)

	// Old values keep working, case-insensitively, stored in canonical form.
	i, err := ir.Create(ctx, model.CreateInteractionInput{Type: "CALL", PersonIDs: []int64{p.ID}})
	require.NoError(t, err)
	assert.Equal(t, "call", i.Type)

	_, err = ir.Create(ctx, model.CreateInteractionInput{Type: "standup", PersonIDs: []int64{p.ID}})
	assert.ErrorIs(t, err, model.ErrValidation, "unknown type")

	_, err = nr.Add(ctx, "Standup", true)
	require.NoError(t, err)
	i, err = ir.Create(ctx, model.CreateInteractionInput{Type: "standup", PersonIDs: []int64{p.ID}})
	require.NoError(t, err)
	assert.Equal(t, "Standup", i.Type, "stored as defined")

	// Archiving a type blocks new interactions but keeps old ones.
	require.NoError(t, nr.Archive(ctx, "standup"))
	_, err = ir.Create(ctx, model.CreateInteractionInput{Type: "standup", PersonIDs: []int64{p.ID}})
	assert.ErrorIs(t, err, model.ErrValidation)
	got, err := ir.FindByID(ctx, i.ID)
	require.NoError(t, err)
	assert.Equal(t, "Standup", got.Type)
}

func TestDue_OnlyContactTypesResetTimer(t *testing.T) {
	d := openRaw(t)
	pr := repo.NewPersonRepo(d)
	ir := repo.NewInteractionRepo(d)
	nr := repo.NewNoteTypeRepo(d)
	ctx := context.Background()
	p, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Jane"})
	require.NoError(t, err)
	_, err = pr.Update(ctx, p.ID, model.UpdatePersonInput{KeepInTouch: ptr("7 days")})
	require.NoError(t, err)
	_, err = d.Exec(`UPDATE people SET keep_in_touch_set_at = '2020-01-01 00:00:00' WHERE id = ?`, p.ID)
	require.NoError(t, err)
	_, err = nr.Add(ctx, "journal", false)
	require.NoError(t, err)

	for _, typ := range []string{"note", "journal"} {
		_, err = ir.Create(ctx, model.CreateInteractionInput{Type: typ, PersonIDs: []int64{p.ID}})
		require.NoError(t, err)
	}
	due, err := pr.Due(ctx, 0, 50)
	require.NoError(t, err)
	assert.Len(t, due, 1, "note and a custom non-contact type leave the person overdue")

	_, err = ir.Create(ctx, model.CreateInteractionInput{Type: "meeting", PersonIDs: []int64{p.ID}})
	require.NoError(t, err)
	due, err = pr.Due(ctx, 0, 50)
	require.NoError(t, err)
	assert.Empty(t, due, "a meeting counts")
}

func TestPersonArchiveRestorePurge(t *testing.T) {
	d := openRaw(t)
	pr := repo.NewPersonRepo(d)
	ir := repo.NewInteractionRepo(d)
	tr := repo.NewTaskRepo(d)
	dr := repo.NewDealRepo(d)
	tagr := repo.NewTagRepo(d)
	rr := repo.NewRelationshipRepo(d)
	ctx := context.Background()

	keep, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Keep", Email: ptr("keep@x.com")})
	gone, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Gone", Email: ptr("gone@x.com"), Phone: ptr("+15550001")})
	require.NoError(t, err)
	require.NoError(t, pr.SetHandle(ctx, gone.ID, "linkedin", "gone"))
	_, err = pr.Update(ctx, gone.ID, model.UpdatePersonInput{Birthday: ptr("1990-01-02"), KeepInTouch: ptr("1 mon")})
	require.NoError(t, err)

	solo, err := ir.Create(ctx, model.CreateInteractionInput{Type: "call", Subject: ptr("solo"), PersonIDs: []int64{gone.ID}})
	require.NoError(t, err)
	shared, err := ir.Create(ctx, model.CreateInteractionInput{Type: "meeting", Subject: ptr("shared"), PersonIDs: []int64{gone.ID, keep.ID}})
	require.NoError(t, err)
	soloTask, err := tr.Create(ctx, model.CreateTaskInput{Title: "solo task", Priority: "medium", PersonID: &gone.ID})
	require.NoError(t, err)
	sharedTask, err := tr.Create(ctx, model.CreateTaskInput{Title: "shared task", Priority: "medium", PersonID: &gone.ID, PersonIDs: []int64{keep.ID}})
	require.NoError(t, err)
	deal, err := dr.Create(ctx, model.CreateDealInput{Title: "Deal", Stage: "lead", PersonID: &gone.ID})
	require.NoError(t, err)
	require.NoError(t, tagr.Apply(ctx, "person", gone.ID, "vip"))
	_, err = rr.Create(ctx, gone.ID, keep.ID, "friend", nil)
	require.NoError(t, err)

	t.Run("purge refuses an active person", func(t *testing.T) {
		assert.ErrorIs(t, pr.Purge(ctx, gone.ID), model.ErrConflict)
		assert.ErrorIs(t, pr.Purge(ctx, 999), model.ErrNotFound)
	})

	t.Run("archive hides and restore brings back", func(t *testing.T) {
		require.NoError(t, pr.Archive(ctx, gone.ID))
		_, err := pr.FindByID(ctx, gone.ID)
		assert.ErrorIs(t, err, model.ErrNotFound)

		active, _ := pr.FindAll(ctx, model.PersonFilters{})
		archived, _ := pr.FindAll(ctx, model.PersonFilters{Archived: true})
		require.Len(t, active, 1)
		require.Len(t, archived, 1)
		assert.Equal(t, gone.ID, archived[0].ID)

		found, _ := pr.Search(ctx, "gone", 10)
		assert.Empty(t, found, "archived people are not searchable")
		due, _ := pr.Due(ctx, 100000, 50)
		assert.Empty(t, due, "archived people are not due")

		assert.ErrorIs(t, pr.Restore(ctx, keep.ID), model.ErrNotFound, "restoring an active person")
		require.NoError(t, pr.Restore(ctx, gone.ID))
		_, err = pr.FindByID(ctx, gone.ID)
		require.NoError(t, err)
		require.NoError(t, pr.Archive(ctx, gone.ID))
	})

	t.Run("purge deletes dependents and writes tombstones", func(t *testing.T) {
		require.NoError(t, pr.Purge(ctx, gone.ID))

		count := func(q string, args ...any) int {
			var n int
			require.NoError(t, d.QueryRow(q, args...).Scan(&n))
			return n
		}
		assert.Equal(t, 0, count(`SELECT COUNT(*) FROM people WHERE id = ?`, gone.ID))
		assert.Equal(t, 0, count(`SELECT COUNT(*) FROM person_emails WHERE person_id = ?`, gone.ID))
		assert.Equal(t, 0, count(`SELECT COUNT(*) FROM person_phones WHERE person_id = ?`, gone.ID))
		assert.Equal(t, 0, count(`SELECT COUNT(*) FROM person_handles WHERE person_id = ?`, gone.ID))
		assert.Equal(t, 0, count(`SELECT COUNT(*) FROM relationships`))
		assert.Equal(t, 0, count(`SELECT COUNT(*) FROM taggings`))
		assert.Equal(t, 0, count(`SELECT COUNT(*) FROM interactions WHERE id = ?`, solo.ID), "solo interaction removed")
		assert.Equal(t, 1, count(`SELECT COUNT(*) FROM interactions WHERE id = ?`, shared.ID), "shared interaction kept")
		assert.Equal(t, 0, count(`SELECT COUNT(*) FROM tasks WHERE id = ?`, soloTask.ID))
		assert.Equal(t, 1, count(`SELECT COUNT(*) FROM tasks WHERE id = ? AND person_id IS NULL`, sharedTask.ID))
		assert.Equal(t, 1, count(`SELECT COUNT(*) FROM deals WHERE id = ? AND person_id IS NULL`, deal.ID))
		assert.Equal(t, 1, count(`SELECT COUNT(*) FROM people WHERE id = ?`, keep.ID))
		assert.Equal(t, 0, count(`SELECT COUNT(*) FROM interactions_fts WHERE interactions_fts MATCH 'solo'`))

		var device string
		require.NoError(t, d.QueryRow(`SELECT value FROM meta WHERE key = 'device_id'`).Scan(&device))
		var n int
		require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM tombstones WHERE table_name = 'people' AND uuid = ? AND device_id = ? AND deleted_at IS NOT NULL`, gone.UUID, device).Scan(&n))
		assert.Equal(t, 1, n)
		assert.Equal(t, 2, count(`SELECT COUNT(*) FROM tombstones WHERE table_name IN ('interactions', 'tasks')`))
	})
}

// TestEveryWriteBumpsUpdatedAt fails when a repo write forgets to bump updated_at on the
// row it changes. Every row is backdated first, then each write runs on its own fixture.
func TestEveryWriteBumpsUpdatedAt(t *testing.T) {
	const old = "2000-01-01 00:00:00"
	type fx struct {
		d                                    *sql.DB
		pr                                   *repo.PersonRepo
		or                                   *repo.OrgRepo
		dr                                   *repo.DealRepo
		tr                                   *repo.TaskRepo
		tagr                                 *repo.TagRepo
		rr                                   *repo.RelationshipRepo
		nr                                   *repo.NoteTypeRepo
		person, other, org, deal, task, note int64
	}
	ctx := context.Background()

	tests := []struct {
		name  string
		table string
		id    func(f *fx) int64
		do    func(f *fx) error
	}{
		{"person update", "people", func(f *fx) int64 { return f.person }, func(f *fx) error {
			_, err := f.pr.Update(ctx, f.person, model.UpdatePersonInput{Title: ptr("CEO")})
			return err
		}},
		{"person keep in touch", "people", func(f *fx) int64 { return f.person }, func(f *fx) error {
			_, err := f.pr.Update(ctx, f.person, model.UpdatePersonInput{KeepInTouch: ptr("1 mon")})
			return err
		}},
		{"person archive", "people", func(f *fx) int64 { return f.person }, func(f *fx) error { return f.pr.Archive(ctx, f.person) }},
		{"person restore", "people", func(f *fx) int64 { return f.person }, func(f *fx) error {
			if err := f.pr.Archive(ctx, f.person); err != nil {
				return err
			}
			if _, err := f.d.Exec(`UPDATE people SET updated_at = ?`, old); err != nil {
				return err
			}
			return f.pr.Restore(ctx, f.person)
		}},
		{"add secondary email", "people", func(f *fx) int64 { return f.person }, func(f *fx) error {
			return f.pr.AddEmail(ctx, f.person, "second@x.com", ptr("Home"), false)
		}},
		{"relabel email", "people", func(f *fx) int64 { return f.person }, func(f *fx) error {
			return f.pr.AddEmail(ctx, f.person, "jane@x.com", ptr("Work"), false)
		}},
		{"remove email", "people", func(f *fx) int64 { return f.person }, func(f *fx) error {
			return f.pr.RemoveEmail(ctx, f.person, "jane@x.com")
		}},
		{"add secondary phone", "people", func(f *fx) int64 { return f.person }, func(f *fx) error {
			return f.pr.AddPhone(ctx, f.person, "+15550002", nil, false)
		}},
		{"remove phone", "people", func(f *fx) int64 { return f.person }, func(f *fx) error {
			return f.pr.RemovePhone(ctx, f.person, "+15550001")
		}},
		{"set handle", "people", func(f *fx) int64 { return f.person }, func(f *fx) error {
			return f.pr.SetHandle(ctx, f.person, "linkedin", "jane")
		}},
		{"remove handle", "people", func(f *fx) int64 { return f.person }, func(f *fx) error {
			if err := f.pr.SetHandle(ctx, f.person, "twitter", "jane"); err != nil {
				return err
			}
			if _, err := f.d.Exec(`UPDATE people SET updated_at = ?`, old); err != nil {
				return err
			}
			return f.pr.RemoveHandle(ctx, f.person, "twitter")
		}},
		{"tag person", "people", func(f *fx) int64 { return f.person }, func(f *fx) error {
			return f.tagr.Apply(ctx, "person", f.person, "new")
		}},
		{"untag person", "people", func(f *fx) int64 { return f.person }, func(f *fx) error {
			return f.tagr.Remove(ctx, "person", f.person, "vip")
		}},
		{"delete tag touches tagged people", "people", func(f *fx) int64 { return f.person }, func(f *fx) error {
			return f.tagr.Delete(ctx, "vip")
		}},
		{"relationship create", "people", func(f *fx) int64 { return f.other }, func(f *fx) error {
			_, err := f.rr.Create(ctx, f.person, f.other, "friend", nil)
			return err
		}},
		{"relationship delete", "people", func(f *fx) int64 { return f.other }, func(f *fx) error {
			rel, err := f.rr.Create(ctx, f.person, f.other, "friend", nil)
			if err != nil {
				return err
			}
			if _, err := f.d.Exec(`UPDATE people SET updated_at = ?`, old); err != nil {
				return err
			}
			return f.rr.Delete(ctx, rel.ID)
		}},
		{"org update", "organizations", func(f *fx) int64 { return f.org }, func(f *fx) error {
			_, err := f.or.Update(ctx, f.org, model.UpdateOrgInput{Industry: ptr("Tech")})
			return err
		}},
		{"org archive", "organizations", func(f *fx) int64 { return f.org }, func(f *fx) error { return f.or.Archive(ctx, f.org) }},
		{"tag org", "organizations", func(f *fx) int64 { return f.org }, func(f *fx) error {
			return f.tagr.Apply(ctx, "organization", f.org, "new")
		}},
		{"deal update", "deals", func(f *fx) int64 { return f.deal }, func(f *fx) error {
			_, err := f.dr.Update(ctx, f.deal, model.UpdateDealInput{Notes: ptr("n")})
			return err
		}},
		{"deal archive", "deals", func(f *fx) int64 { return f.deal }, func(f *fx) error { return f.dr.Archive(ctx, f.deal) }},
		{"tag deal", "deals", func(f *fx) int64 { return f.deal }, func(f *fx) error {
			return f.tagr.Apply(ctx, "deal", f.deal, "new")
		}},
		{"task update", "tasks", func(f *fx) int64 { return f.task }, func(f *fx) error {
			_, err := f.tr.Update(ctx, f.task, model.UpdateTaskInput{Title: ptr("renamed")})
			return err
		}},
		{"task complete", "tasks", func(f *fx) int64 { return f.task }, func(f *fx) error {
			_, err := f.tr.Complete(ctx, f.task)
			return err
		}},
		{"task archive", "tasks", func(f *fx) int64 { return f.task }, func(f *fx) error { return f.tr.Archive(ctx, f.task) }},
		{"note type archive", "note_types", func(f *fx) int64 { return f.note }, func(f *fx) error { return f.nr.Archive(ctx, "standup") }},
		{"note type restore", "note_types", func(f *fx) int64 { return f.note }, func(f *fx) error {
			if err := f.nr.Archive(ctx, "standup"); err != nil {
				return err
			}
			if _, err := f.d.Exec(`UPDATE note_types SET updated_at = ?`, old); err != nil {
				return err
			}
			_, err := f.nr.Add(ctx, "standup", false)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := openRaw(t)
			f := &fx{
				d: d, pr: repo.NewPersonRepo(d), or: repo.NewOrgRepo(d), dr: repo.NewDealRepo(d),
				tr: repo.NewTaskRepo(d), tagr: repo.NewTagRepo(d), rr: repo.NewRelationshipRepo(d),
				nr: repo.NewNoteTypeRepo(d),
			}
			p, err := f.pr.Create(ctx, model.CreatePersonInput{FirstName: "Jane", Email: ptr("jane@x.com"), Phone: ptr("+15550001")})
			require.NoError(t, err)
			o, err := f.pr.Create(ctx, model.CreatePersonInput{FirstName: "Other"})
			require.NoError(t, err)
			org, err := f.or.Create(ctx, model.CreateOrgInput{Name: "Acme"})
			require.NoError(t, err)
			deal, err := f.dr.Create(ctx, model.CreateDealInput{Title: "Deal", Stage: "lead"})
			require.NoError(t, err)
			task, err := f.tr.Create(ctx, model.CreateTaskInput{Title: "Task", Priority: "medium", PersonID: &p.ID})
			require.NoError(t, err)
			nt, err := f.nr.Add(ctx, "standup", true)
			require.NoError(t, err)
			for _, e := range []struct {
				typ string
				id  int64
			}{{"person", p.ID}, {"organization", org.ID}, {"deal", deal.ID}} {
				require.NoError(t, f.tagr.Apply(ctx, e.typ, e.id, "vip"))
			}
			f.person, f.other, f.org, f.deal, f.task, f.note = p.ID, o.ID, org.ID, deal.ID, task.ID, nt.ID

			for _, table := range []string{"people", "organizations", "deals", "tasks", "note_types", "tags"} {
				_, err := d.Exec(`UPDATE `+table+` SET updated_at = ?`, old)
				require.NoError(t, err)
			}

			require.NoError(t, tt.do(f))

			var updated string
			require.NoError(t, d.QueryRow(`SELECT updated_at FROM `+tt.table+` WHERE id = ?`, tt.id(f)).Scan(&updated))
			assert.NotEqual(t, old, updated, "%s did not bump updated_at on %s", tt.name, tt.table)
		})
	}
}

func TestSyncColumnsPresent(t *testing.T) {
	d := openRaw(t)
	for _, table := range []string{"people", "organizations", "interactions", "tasks", "tags", "deals", "note_types"} {
		var uuidCol, updatedCol int
		require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = 'uuid'`, table).Scan(&uuidCol))
		require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = 'updated_at'`, table).Scan(&updatedCol))
		assert.Equal(t, 1, uuidCol, "%s.uuid", table)
		assert.Equal(t, 1, updatedCol, "%s.updated_at", table)
	}

	tagr := repo.NewTagRepo(d)
	tag, err := tagr.FindOrCreate(context.Background(), "fresh")
	require.NoError(t, err)
	var u, c, up sql.NullString
	require.NoError(t, d.QueryRow(`SELECT uuid, created_at, updated_at FROM tags WHERE id = ?`, tag.ID).Scan(&u, &c, &up))
	assert.True(t, u.Valid && c.Valid && up.Valid, "new tags get uuid and timestamps")
}
