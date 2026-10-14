package sync_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	crmsync "github.com/jdanielnd/crm-cli/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T { return &v }

type env struct {
	t      *testing.T
	db     *sql.DB
	people *repo.PersonRepo
	fake   *crmsync.FakeAdapter
	runner *crmsync.Runner
}

func newEnv(t *testing.T, records ...crmsync.FakeRecord) *env {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	fake := &crmsync.FakeAdapter{Records: records}
	return &env{t: t, db: d, people: repo.NewPersonRepo(d), fake: fake,
		runner: crmsync.NewRunner(d, crmsync.NewRegistry(fake))}
}

func (e *env) count(q string, args ...any) int {
	e.t.Helper()
	var n int
	require.NoError(e.t, e.db.QueryRow(q, args...).Scan(&n))
	return n
}

func (e *env) person(first, email, phone string) *model.Person {
	e.t.Helper()
	in := model.CreatePersonInput{FirstName: first}
	if email != "" {
		in.Email = &email
	}
	if phone != "" {
		in.Phone = &phone
	}
	p, err := e.people.Create(context.Background(), in)
	require.NoError(e.t, err)
	return p
}

func (e *env) run(opts crmsync.RunOptions) *crmsync.RunResult {
	e.t.Helper()
	res, err := e.runner.Run(context.Background(), "fake", opts)
	require.NoError(e.t, err)
	return res
}

func personRec(seq int, ext string, p crmsync.PersonPatch) crmsync.FakeRecord {
	return crmsync.FakeRecord{Seq: seq, Kind: "person", ExternalID: ext, Person: &p}
}

func TestRunMatchesByEmailAndIsIdempotent(t *testing.T) {
	e := newEnv(t,
		personRec(1, "g1", crmsync.PersonPatch{Emails: []string{" JANE@Acme.com "}, Title: ptr("CTO"), Company: ptr("Other"), ETag: "v1"}),
	)
	jane := e.person("Jane", "jane@acme.com", "")
	_, err := e.people.Update(context.Background(), jane.ID, model.UpdatePersonInput{Company: ptr("Acme")})
	require.NoError(t, err)

	res := e.run(crmsync.RunOptions{})
	assert.Equal(t, model.SyncOK, res.Status)
	assert.Equal(t, 1, res.Updated)
	assert.Equal(t, "1", res.NextCursor)

	got, err := e.people.FindByID(context.Background(), jane.ID)
	require.NoError(t, err)
	assert.Equal(t, "CTO", *got.Title, "blank field is filled")
	assert.Equal(t, "Acme", *got.Company, "first match fills blanks only")

	link, err := repo.NewExternalIDRepo(e.db).Lookup(context.Background(), "fake", "person", "g1")
	require.NoError(t, err)
	assert.Equal(t, jane.ID, link.LocalID)
	assert.Equal(t, "v1", *link.ETag)

	// Second run: cursor is past the record, nothing happens.
	res = e.run(crmsync.RunOptions{})
	assert.Equal(t, "1", res.Since)
	assert.Equal(t, 0, res.Updated+res.Created+res.Skipped)

	// Reset the cursor so the record is delivered again. It is already linked, so the patch
	// now overwrites, and an unchanged patch is skipped.
	require.NoError(t, repo.NewSyncRepo(e.db).SetCursor(context.Background(), "fake", ""))
	res = e.run(crmsync.RunOptions{})
	assert.Equal(t, 1, res.Updated, "linked record overwrites company")
	got, _ = e.people.FindByID(context.Background(), jane.ID)
	assert.Equal(t, "Other", *got.Company)
	require.NoError(t, repo.NewSyncRepo(e.db).SetCursor(context.Background(), "fake", ""))
	res = e.run(crmsync.RunOptions{})
	assert.Equal(t, 1, res.Skipped)
	assert.Equal(t, 0, res.Updated)
	assert.Equal(t, 1, e.count(`SELECT COUNT(*) FROM external_ids WHERE source = 'fake'`))
	assert.Equal(t, 1, e.count(`SELECT COUNT(*) FROM people`))
}

func TestRunMatchesByPhoneAndHandleAddsMissingData(t *testing.T) {
	e := newEnv(t,
		personRec(1, "p1", crmsync.PersonPatch{Phones: []string{"(555) 123-4567"}, Emails: []string{"new@home.com"}, Handles: map[string]string{"linkedin": "https://www.linkedin.com/in/ann"}}),
		personRec(2, "p2", crmsync.PersonPatch{Handles: map[string]string{"twitter": "@ben"}}),
	)
	ann := e.person("Ann", "", "+1 555-123-4567")
	ben := e.person("Ben", "", "")
	require.NoError(t, e.people.SetHandle(context.Background(), ben.ID, "twitter", "https://twitter.com/ben"))

	res := e.run(crmsync.RunOptions{})
	assert.Equal(t, 2, res.Updated)

	emails, err := e.people.Emails(context.Background(), ann.ID)
	require.NoError(t, err)
	require.Len(t, emails, 1)
	assert.Equal(t, "new@home.com", emails[0].Email)
	handles, err := e.people.Handles(context.Background(), ann.ID)
	require.NoError(t, err)
	require.Len(t, handles, 1)
	assert.Equal(t, "linkedin", handles[0].Kind)

	link, err := repo.NewExternalIDRepo(e.db).Lookup(context.Background(), "fake", "person", "p2")
	require.NoError(t, err)
	assert.Equal(t, ben.ID, link.LocalID, "handle match")
}

func TestRunUnmatchedGoesToInboxUnlessAllowCreate(t *testing.T) {
	recs := []crmsync.FakeRecord{
		personRec(1, "z1", crmsync.PersonPatch{FirstName: ptr("Zed"), LastName: ptr("Zane"), Emails: []string{"zed@x.com", "zed@y.com"}, Phones: []string{"555-987-6543"}}),
	}
	t.Run("inbox", func(t *testing.T) {
		e := newEnv(t, recs...)
		res := e.run(crmsync.RunOptions{})
		assert.Equal(t, 1, res.Inbox)
		assert.Equal(t, 0, res.Created)
		assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM people`))
		assert.Equal(t, 1, e.count(`SELECT COUNT(*) FROM sync_inbox WHERE status = 'pending'`))

		// Redelivery never duplicates the inbox row.
		require.NoError(t, repo.NewSyncRepo(e.db).SetCursor(context.Background(), "fake", ""))
		e.run(crmsync.RunOptions{})
		assert.Equal(t, 1, e.count(`SELECT COUNT(*) FROM sync_inbox`))
	})
	t.Run("allow create", func(t *testing.T) {
		e := newEnv(t, recs...)
		res := e.run(crmsync.RunOptions{AllowCreate: true})
		assert.Equal(t, 1, res.Created)
		assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM sync_inbox`))
		p, err := e.people.FindByID(context.Background(), 1)
		require.NoError(t, err)
		assert.Equal(t, "Zed", p.FirstName)
		assert.Equal(t, "zed@x.com", *p.Email)
		emails, _ := e.people.Emails(context.Background(), 1)
		assert.Len(t, emails, 2)
		phones, _ := e.people.Phones(context.Background(), 1)
		assert.Len(t, phones, 1)
		assert.Equal(t, 1, e.count(`SELECT COUNT(*) FROM external_ids WHERE source = 'fake' AND external_id = 'z1'`))
	})
	t.Run("name only duplicate is queued not failed", func(t *testing.T) {
		e := newEnv(t, personRec(1, "z2", crmsync.PersonPatch{FirstName: ptr("Zed"), LastName: ptr("Zane")}))
		_, err := e.people.Create(context.Background(), model.CreatePersonInput{FirstName: "Zed", LastName: ptr("Zane")})
		require.NoError(t, err)
		res := e.run(crmsync.RunOptions{AllowCreate: true})
		assert.Equal(t, 1, res.Inbox, "names never auto-match")
		assert.Equal(t, 0, res.Errors)
		assert.Equal(t, 1, e.count(`SELECT COUNT(*) FROM people`))
	})
	t.Run("nameless without email stays queued", func(t *testing.T) {
		e := newEnv(t, personRec(1, "z3", crmsync.PersonPatch{Phones: []string{"555-111-2222"}}))
		res := e.run(crmsync.RunOptions{AllowCreate: true})
		assert.Equal(t, 1, res.Inbox)
		assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM people`))
	})
}

func TestRunAmbiguousMatchGoesToInbox(t *testing.T) {
	e := newEnv(t, personRec(1, "a1", crmsync.PersonPatch{Emails: []string{"a@x.com"}, Phones: []string{"555-222-3333"}}))
	e.person("A", "a@x.com", "")
	e.person("B", "", "555-222-3333")
	res := e.run(crmsync.RunOptions{AllowCreate: true})
	assert.Equal(t, 1, res.Inbox, "two candidates are never guessed")
	assert.Equal(t, 0, res.Created+res.Updated)
	items, err := crmsync.NewInbox(e.db).List(context.Background(), "pending", 0)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "ambiguous", items[0].Reason)
	assert.Equal(t, []int64{1, 2}, items[0].Candidates)
}

func TestDryRunWritesNothing(t *testing.T) {
	e := newEnv(t,
		personRec(1, "m1", crmsync.PersonPatch{Emails: []string{"jane@acme.com"}, Title: ptr("CTO")}),
		personRec(2, "n1", crmsync.PersonPatch{FirstName: ptr("New"), Emails: []string{"new@x.com"}}),
		personRec(3, "n2", crmsync.PersonPatch{FirstName: ptr("New Again"), Emails: []string{"new@x.com"}}),
		personRec(4, "q1", crmsync.PersonPatch{Phones: []string{"555-000-1111"}}),
	)
	e.person("Jane", "jane@acme.com", "")
	before := e.count(`SELECT COUNT(*) FROM people`)

	res := e.run(crmsync.RunOptions{DryRun: true, AllowCreate: true})
	assert.True(t, res.DryRun)
	assert.Equal(t, 1, res.Created, "new person")
	assert.Equal(t, 1, res.Updated, "jane")
	assert.Equal(t, 1, res.Skipped, "repeated email matches the person this run would have created")
	assert.Equal(t, 1, res.Inbox)

	assert.Equal(t, before, e.count(`SELECT COUNT(*) FROM people`))
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM sync_runs`))
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM sync_cursors`))
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM external_ids`))
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM sync_inbox`))
	p, _ := e.people.FindByID(context.Background(), 1)
	assert.Nil(t, p.Title)
}

func TestRunNeverDeletes(t *testing.T) {
	e := newEnv(t,
		personRec(1, "g1", crmsync.PersonPatch{Emails: []string{"jane@acme.com"}}),
		personRec(2, "g2", crmsync.PersonPatch{Emails: []string{"bob@acme.com"}}),
	)
	e.person("Jane", "jane@acme.com", "")
	e.person("Bob", "bob@acme.com", "")
	e.run(crmsync.RunOptions{})

	// The other side now has only one of them. Nothing is removed here.
	e.fake.Records = e.fake.Records[:1]
	require.NoError(t, repo.NewSyncRepo(e.db).SetCursor(context.Background(), "fake", ""))
	e.run(crmsync.RunOptions{})
	assert.Equal(t, 2, e.count(`SELECT COUNT(*) FROM people WHERE archived = 0`))
	assert.Equal(t, 2, e.count(`SELECT COUNT(*) FROM external_ids`))
}

func TestRunFailureAndPartial(t *testing.T) {
	t.Run("pull failure keeps the cursor", func(t *testing.T) {
		e := newEnv(t,
			personRec(1, "g1", crmsync.PersonPatch{Emails: []string{"a@x.com"}}),
			personRec(2, "g2", crmsync.PersonPatch{Emails: []string{"b@x.com"}}),
		)
		e.fake.FailAfter = 1
		res := e.run(crmsync.RunOptions{})
		assert.Equal(t, model.SyncFailed, res.Status)
		assert.Contains(t, res.Note, "simulated failure")
		assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM sync_cursors`))
		var status string
		var finished sql.NullString
		require.NoError(t, e.db.QueryRow(`SELECT status, finished_at FROM sync_runs`).Scan(&status, &finished))
		assert.Equal(t, model.SyncFailed, status)
		assert.True(t, finished.Valid)

		e.fake.FailAfter = 0
		res = e.run(crmsync.RunOptions{})
		assert.Equal(t, model.SyncOK, res.Status)
		assert.Equal(t, "2", res.NextCursor)
	})
	t.Run("record errors are counted and the cursor waits", func(t *testing.T) {
		bad := crmsync.FakeRecord{Seq: 1, Kind: "interaction", ExternalID: "i1", Interaction: &crmsync.InteractionRecord{
			Type: "no-such-type", People: []crmsync.PersonRef{{Emails: []string{"jane@acme.com"}}}}}
		e := newEnv(t, bad)
		e.person("Jane", "jane@acme.com", "")
		res := e.run(crmsync.RunOptions{})
		assert.Equal(t, model.SyncPartial, res.Status)
		assert.Equal(t, 1, res.Errors)
		assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM sync_cursors`))
		assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM interactions`))
		assert.Equal(t, 1, e.count(`SELECT errors FROM sync_runs`))
	})
	t.Run("unknown source", func(t *testing.T) {
		e := newEnv(t)
		_, err := e.runner.Run(context.Background(), "nope", crmsync.RunOptions{})
		assert.ErrorIs(t, err, model.ErrNotFound)
	})
}

func TestArchivedLinkedPersonIsNotResurrectedAndMergeIsFollowed(t *testing.T) {
	e := newEnv(t, personRec(1, "g1", crmsync.PersonPatch{Emails: []string{"jane@acme.com"}, Title: ptr("CTO")}))
	jane := e.person("Jane", "jane@acme.com", "")
	e.run(crmsync.RunOptions{})

	require.NoError(t, e.people.Archive(context.Background(), jane.ID))
	e.fake.Records = []crmsync.FakeRecord{personRec(2, "g1", crmsync.PersonPatch{Title: ptr("CEO")})}
	res := e.run(crmsync.RunOptions{})
	assert.Equal(t, 1, res.Skipped)
	var archived int
	require.NoError(t, e.db.QueryRow(`SELECT archived FROM people WHERE id = ?`, jane.ID).Scan(&archived))
	assert.Equal(t, 1, archived)

	// Restore, then merge Jane into another person: the link follows the survivor.
	require.NoError(t, e.people.Restore(context.Background(), jane.ID))
	other := e.person("Janet", "", "")
	_, err := e.people.Merge(context.Background(), other.ID, []int64{jane.ID}, nil)
	require.NoError(t, err)
	e.fake.Records = []crmsync.FakeRecord{personRec(3, "g1", crmsync.PersonPatch{Title: ptr("Chair")})}
	res = e.run(crmsync.RunOptions{})
	assert.Equal(t, 1, res.Updated)
	survivor, err := e.people.FindByID(context.Background(), other.ID)
	require.NoError(t, err)
	assert.Equal(t, "Chair", *survivor.Title)
}

func TestInteractions(t *testing.T) {
	rec := func(seq int, ext string, ref crmsync.PersonRef) crmsync.FakeRecord {
		return crmsync.FakeRecord{Seq: seq, Kind: "interaction", ExternalID: ext, Interaction: &crmsync.InteractionRecord{
			Type: "message", Content: ptr("hello pelican"), OccurredAt: "2026-03-01 10:00:00", People: []crmsync.PersonRef{ref}}}
	}
	e := newEnv(t,
		rec(1, "m1", crmsync.PersonRef{Emails: []string{"jane@acme.com"}}),
		rec(2, "m2", crmsync.PersonRef{Emails: []string{"stranger@x.com"}}),
	)
	jane := e.person("Jane", "jane@acme.com", "")

	res := e.run(crmsync.RunOptions{})
	assert.Equal(t, 1, res.Created)
	assert.Equal(t, 1, res.Inbox)
	var pid int64
	require.NoError(t, e.db.QueryRow(`SELECT person_id FROM interaction_people`).Scan(&pid))
	assert.Equal(t, jane.ID, pid)

	// Redelivery is skipped by external id.
	require.NoError(t, repo.NewSyncRepo(e.db).SetCursor(context.Background(), "fake", ""))
	res = e.run(crmsync.RunOptions{})
	assert.Equal(t, 0, res.Created)
	assert.Equal(t, 1, e.count(`SELECT COUNT(*) FROM interactions`))

	// The unmatched one can be attached to a person, which logs it there.
	inbox := crmsync.NewInbox(e.db)
	items, err := inbox.List(context.Background(), "pending", 0)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "interaction", items[0].Kind)
	_, err = inbox.Accept(context.Background(), items[0].ID)
	assert.ErrorIs(t, err, model.ErrValidation, "interactions cannot be accepted as people")
	bob := e.person("Bob", "", "")
	require.NoError(t, inbox.Attach(context.Background(), items[0].ID, bob.ID))
	assert.Equal(t, 2, e.count(`SELECT COUNT(*) FROM interactions`))
	assert.Equal(t, 1, e.count(`SELECT COUNT(*) FROM external_ids WHERE entity = 'interaction' AND external_id = 'm2'`))
	assert.ErrorIs(t, inbox.Attach(context.Background(), items[0].ID, bob.ID), model.ErrConflict, "already decided")
}

func TestInboxAcceptAttachIgnore(t *testing.T) {
	e := newEnv(t,
		personRec(1, "a", crmsync.PersonPatch{FirstName: ptr("Acc"), Emails: []string{"acc@x.com"}}),
		personRec(2, "b", crmsync.PersonPatch{FirstName: ptr("Att"), Title: ptr("VP"), Emails: []string{"att@x.com"}}),
		personRec(3, "c", crmsync.PersonPatch{FirstName: ptr("Ign"), Emails: []string{"ign@x.com"}}),
	)
	res := e.run(crmsync.RunOptions{})
	assert.Equal(t, 3, res.Inbox)

	ctx := context.Background()
	inbox := crmsync.NewInbox(e.db)
	items, err := inbox.List(ctx, "pending", 0)
	require.NoError(t, err)
	require.Len(t, items, 3)
	assert.Equal(t, "Acc", items[0].Summary)

	pid, err := inbox.Accept(ctx, items[0].ID)
	require.NoError(t, err)
	acc, err := e.people.FindByID(ctx, pid)
	require.NoError(t, err)
	assert.Equal(t, "Acc", acc.FirstName)
	assert.Equal(t, 1, e.count(`SELECT COUNT(*) FROM external_ids WHERE external_id = 'a'`))
	_, err = inbox.Accept(ctx, items[0].ID)
	assert.ErrorIs(t, err, model.ErrConflict)

	target := e.person("Target", "", "")
	require.NoError(t, inbox.Attach(ctx, items[1].ID, target.ID))
	got, _ := e.people.FindByID(ctx, target.ID)
	assert.Equal(t, "VP", *got.Title)
	emails, _ := e.people.Emails(ctx, target.ID)
	require.Len(t, emails, 1)
	assert.Equal(t, "att@x.com", emails[0].Email)
	assert.ErrorIs(t, inbox.Attach(ctx, items[2].ID, 9999), model.ErrNotFound)

	require.NoError(t, inbox.Ignore(ctx, items[2].ID))
	pending, err := inbox.List(ctx, "pending", 0)
	require.NoError(t, err)
	assert.Empty(t, pending)
	all, err := inbox.List(ctx, "", 0)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	// An ignored record stays ignored when it comes back.
	require.NoError(t, repo.NewSyncRepo(e.db).SetCursor(ctx, "fake", ""))
	e.fake.Records = e.fake.Records[2:]
	res = e.run(crmsync.RunOptions{})
	assert.Equal(t, 1, res.Skipped)
	assert.Equal(t, 0, res.Inbox)
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM sync_inbox WHERE status = 'pending'`))
}

func TestStatusAndLastSync(t *testing.T) {
	e := newEnv(t, personRec(1, "z", crmsync.PersonPatch{FirstName: ptr("Z"), Emails: []string{"z@x.com"}}))
	ctx := context.Background()

	last, err := crmsync.LastSyncSummary(ctx, e.db)
	require.NoError(t, err)
	assert.Nil(t, last)

	e.run(crmsync.RunOptions{})
	last, err = crmsync.LastSyncSummary(ctx, e.db)
	require.NoError(t, err)
	require.NotNil(t, last)
	assert.Equal(t, "fake", last.Source)
	assert.Equal(t, model.SyncOK, last.Status)
	assert.Equal(t, 1, last.InboxPending)

	o, err := e.runner.Overview(ctx)
	require.NoError(t, err)
	require.Len(t, o.Sources, 1)
	assert.True(t, o.Sources[0].Registered)
	assert.Equal(t, "1", o.Sources[0].Cursor)
	assert.Equal(t, 1, o.InboxPending)
}
