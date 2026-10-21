package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(i int) *int { return &i }

func TestPersonEmails_PrimaryTracking(t *testing.T) {
	r := setupTestDB(t)
	ctx := context.Background()
	p, err := r.Create(ctx, model.CreatePersonInput{FirstName: "Jane", Email: ptr("jane@work.com")})
	require.NoError(t, err)

	// Create mirrors the email into person_emails as primary.
	emails, err := r.Emails(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, emails, 1)
	assert.True(t, emails[0].IsPrimary)

	require.NoError(t, r.AddEmail(ctx, p.ID, "jane@home.com", ptr("Home"), false))
	got, _ := r.FindByID(ctx, p.ID)
	assert.Equal(t, "jane@work.com", *got.Email, "a non-primary add leaves the primary alone")

	require.NoError(t, r.AddEmail(ctx, p.ID, "jane@home.com", nil, true))
	got, _ = r.FindByID(ctx, p.ID)
	assert.Equal(t, "jane@home.com", *got.Email)
	emails, _ = r.Emails(ctx, p.ID)
	assert.Equal(t, "jane@home.com", emails[0].Email, "primary sorts first")
	assert.Equal(t, "Home", *emails[0].Label, "a later add without a label keeps the old label")

	// Removing the primary promotes the remaining address.
	require.NoError(t, r.RemoveEmail(ctx, p.ID, "jane@home.com"))
	got, _ = r.FindByID(ctx, p.ID)
	assert.Equal(t, "jane@work.com", *got.Email)

	// Removing the last address clears people.email.
	require.NoError(t, r.RemoveEmail(ctx, p.ID, "jane@work.com"))
	got, _ = r.FindByID(ctx, p.ID)
	assert.Nil(t, got.Email)

	assert.ErrorIs(t, r.RemoveEmail(ctx, p.ID, "nobody@x.com"), model.ErrNotFound)
	assert.ErrorIs(t, r.AddEmail(ctx, p.ID, "not-an-email", nil, false), model.ErrValidation)
	assert.ErrorIs(t, r.AddEmail(ctx, 999, "a@b.com", nil, false), model.ErrNotFound)
}

func TestPersonPhones_AddRemove(t *testing.T) {
	r := setupTestDB(t)
	ctx := context.Background()
	p, _ := r.Create(ctx, model.CreatePersonInput{FirstName: "Jane"})

	require.NoError(t, r.AddPhone(ctx, p.ID, "+15551112222", nil, false))
	require.NoError(t, r.AddPhone(ctx, p.ID, "+15553334444", ptr("Work"), false))
	got, _ := r.FindByID(ctx, p.ID)
	assert.Equal(t, "+15551112222", *got.Phone, "first phone becomes primary")

	require.NoError(t, r.RemovePhone(ctx, p.ID, "+15551112222"))
	got, _ = r.FindByID(ctx, p.ID)
	assert.Equal(t, "+15553334444", *got.Phone)
}

func TestPersonHandles(t *testing.T) {
	r := setupTestDB(t)
	ctx := context.Background()
	p, _ := r.Create(ctx, model.CreatePersonInput{FirstName: "Jane"})

	require.NoError(t, r.SetHandle(ctx, p.ID, "linkedin", "https://linkedin.com/in/jane"))
	require.NoError(t, r.SetHandle(ctx, p.ID, "linkedin", "https://linkedin.com/in/jane2"))
	handles, err := r.Handles(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, handles, 1)
	assert.Equal(t, "https://linkedin.com/in/jane2", handles[0].Value)

	assert.ErrorIs(t, r.SetHandle(ctx, p.ID, "myspace", "x"), model.ErrValidation)
	require.NoError(t, r.RemoveHandle(ctx, p.ID, "linkedin"))
	assert.ErrorIs(t, r.RemoveHandle(ctx, p.ID, "linkedin"), model.ErrNotFound)
}

func TestPersonUpdate_NewFields(t *testing.T) {
	r := setupTestDB(t)
	ctx := context.Background()
	p, _ := r.Create(ctx, model.CreatePersonInput{FirstName: "Jane"})

	fav := true
	got, err := r.Update(ctx, p.ID, model.UpdatePersonInput{
		Birthday:     ptr("1990-03-14"),
		KeepInTouch:  ptr("3 mons"),
		IsFavorite:   &fav,
		PriorityTier: intPtr(2),
	})
	require.NoError(t, err)
	assert.Equal(t, 3, *got.BirthdayMonth)
	assert.Equal(t, 14, *got.BirthdayDay)
	assert.Equal(t, 1990, *got.BirthdayYear)
	assert.Equal(t, "3 mons", *got.KeepInTouch)
	assert.True(t, got.IsFavorite)
	assert.Equal(t, 2, *got.PriorityTier)

	// A year-less birthday clears the year. Empty values clear the field.
	got, err = r.Update(ctx, p.ID, model.UpdatePersonInput{Birthday: ptr("--02-29"), KeepInTouch: ptr(""), PriorityTier: intPtr(0)})
	require.NoError(t, err)
	assert.Equal(t, 2, *got.BirthdayMonth)
	assert.Nil(t, got.BirthdayYear)
	assert.Nil(t, got.KeepInTouch)
	assert.Nil(t, got.PriorityTier)

	got, err = r.Update(ctx, p.ID, model.UpdatePersonInput{Birthday: ptr("")})
	require.NoError(t, err)
	assert.Nil(t, got.BirthdayMonth)

	_, err = r.Update(ctx, p.ID, model.UpdatePersonInput{KeepInTouch: ptr("weekly")})
	assert.ErrorIs(t, err, model.ErrValidation)
	_, err = r.Update(ctx, p.ID, model.UpdatePersonInput{Birthday: ptr("1990-02-31")})
	assert.ErrorIs(t, err, model.ErrValidation)
}

func TestPersonUpdate_EmailSyncsRows(t *testing.T) {
	r := setupTestDB(t)
	ctx := context.Background()
	p, _ := r.Create(ctx, model.CreatePersonInput{FirstName: "Jane", Email: ptr("a@x.com")})

	_, err := r.Update(ctx, p.ID, model.UpdatePersonInput{Email: ptr("b@x.com")})
	require.NoError(t, err)
	emails, _ := r.Emails(ctx, p.ID)
	require.Len(t, emails, 2)
	assert.Equal(t, "b@x.com", emails[0].Email)
	assert.True(t, emails[0].IsPrimary)
}

func TestPersonSearch_FindsSecondaryEmailAndPhone(t *testing.T) {
	r := setupTestDB(t)
	ctx := context.Background()
	p, _ := r.Create(ctx, model.CreatePersonInput{FirstName: "Jane", Email: ptr("jane@work.com")})
	require.NoError(t, r.AddEmail(ctx, p.ID, "jane@gmail.com", nil, false))
	require.NoError(t, r.AddPhone(ctx, p.ID, "+15559998888", nil, false))

	found, err := r.Search(ctx, "jane@gmail.com", 10)
	require.NoError(t, err)
	require.Len(t, found, 1)
	found, err = r.Search(ctx, "5559998888", 10)
	require.NoError(t, err)
	require.Len(t, found, 1)
}

func TestPersonCreate_DuplicateSecondaryEmail(t *testing.T) {
	r := setupTestDB(t)
	ctx := context.Background()
	p, _ := r.Create(ctx, model.CreatePersonInput{FirstName: "Jane"})
	require.NoError(t, r.AddEmail(ctx, p.ID, "jane@gmail.com", nil, false))

	_, err := r.Create(ctx, model.CreatePersonInput{FirstName: "Other", Email: ptr("jane@gmail.com")})
	assert.ErrorIs(t, err, model.ErrConflict)
}

func TestPersonDue(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	r := repo.NewPersonRepo(d)
	ctx := context.Background()

	old, _ := r.Create(ctx, model.CreatePersonInput{FirstName: "Old"})
	fresh, _ := r.Create(ctx, model.CreatePersonInput{FirstName: "Fresh"})
	snoozed, _ := r.Create(ctx, model.CreatePersonInput{FirstName: "Snoozed"})
	never, _ := r.Create(ctx, model.CreatePersonInput{FirstName: "Never"})
	for _, p := range []*model.Person{old, fresh, snoozed} {
		_, err := r.Update(ctx, p.ID, model.UpdatePersonInput{KeepInTouch: ptr("7 days")})
		require.NoError(t, err)
	}
	_, err = r.Update(ctx, never.ID, model.UpdatePersonInput{KeepInTouch: ptr("never")})
	require.NoError(t, err)

	_, err = r.Update(ctx, old.ID, model.UpdatePersonInput{LastContactedAt: ptr("2020-01-01 00:00:00")})
	require.NoError(t, err)
	_, err = r.Update(ctx, fresh.ID, model.UpdatePersonInput{LastContactedAt: ptr(time.Now().UTC().Format("2006-01-02 15:04:05"))})
	require.NoError(t, err)
	_, err = r.Update(ctx, snoozed.ID, model.UpdatePersonInput{
		LastContactedAt: ptr("2020-01-01 00:00:00"),
		SnoozedUntil:    ptr(time.Now().UTC().Add(48 * time.Hour).Format("2006-01-02 15:04:05")),
	})
	require.NoError(t, err)

	due, err := r.Due(ctx, 0, 50)
	require.NoError(t, err)
	require.Len(t, due, 1, "fresh is not due, snoozed is hidden, never has no cadence")
	assert.Equal(t, old.ID, due[0].PersonID)
	assert.Equal(t, "Old", due[0].Name)
	assert.Greater(t, due[0].DaysOverdue, 1000)

	// Looking ahead 10 days also picks up the fresh contact (due in 7 days).
	due, err = r.Due(ctx, 10, 50)
	require.NoError(t, err)
	assert.Len(t, due, 2)
}

func TestPersonBirthdays(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	r := repo.NewPersonRepo(d)
	ctx := context.Background()

	mk := func(name, birthday string) {
		p, err := r.Create(ctx, model.CreatePersonInput{FirstName: name})
		require.NoError(t, err)
		_, err = r.Update(ctx, p.ID, model.UpdatePersonInput{Birthday: ptr(birthday)})
		require.NoError(t, err)
	}
	mk("Today", "1990-10-13")
	mk("Soon", "--10-20")
	mk("Wrap", "2000-01-05")
	mk("Leap", "--02-29")
	mk("Far", "--06-01")

	now := time.Date(2026, 10, 13, 15, 0, 0, 0, time.UTC)
	got, err := r.Birthdays(ctx, 365, now)
	require.NoError(t, err)
	var names []string
	for _, b := range got {
		names = append(names, b.Name)
	}
	assert.Equal(t, []string{"Today", "Soon", "Wrap", "Leap", "Far"}, names)
	assert.Equal(t, 0, got[0].DaysUntil)
	assert.Equal(t, 36, *got[0].Turning)
	assert.Equal(t, "2026-10-20", got[1].Date)
	assert.Nil(t, got[1].Turning)
	assert.Equal(t, "2027-01-05", got[2].Date)
	assert.Equal(t, 27, *got[2].Turning)
	assert.Equal(t, "2027-02-28", got[3].Date, "Feb 29 falls on Feb 28 in a non-leap year")

	got, _ = r.Birthdays(ctx, 10, now)
	assert.Len(t, got, 2)
}

func TestTaskRecurrenceAndPeople(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	pr := repo.NewPersonRepo(d)
	tr := repo.NewTaskRepo(d)
	ctx := context.Background()
	a, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "A"})
	b, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "B"})

	_, err = tr.Create(ctx, model.CreateTaskInput{Title: "no due", Priority: "medium", Recurrence: ptr("1 mon")})
	assert.ErrorIs(t, err, model.ErrValidation, "recurring tasks need a due date")
	_, err = tr.Create(ctx, model.CreateTaskInput{Title: "bad", Priority: "medium", DueAt: ptr("2026-01-01"), Recurrence: ptr("sometimes")})
	assert.ErrorIs(t, err, model.ErrValidation)

	task, err := tr.Create(ctx, model.CreateTaskInput{
		Title: "Quarterly check-in", Priority: "high", DueAt: ptr("2026-01-15"),
		PersonID: &a.ID, PersonIDs: []int64{b.ID}, Recurrence: ptr("3 mons"),
	})
	require.NoError(t, err)
	assert.Equal(t, []int64{a.ID, b.ID}, task.PersonIDs)

	// The task shows up under either person.
	for _, pid := range []int64{a.ID, b.ID} {
		pid := pid
		list, err := tr.FindAll(ctx, model.TaskFilters{PersonID: &pid})
		require.NoError(t, err)
		require.Len(t, list, 1)
	}

	// Completing a recurring task creates the next one, dated after now and sharing people.
	done, err := tr.Complete(ctx, task.ID)
	require.NoError(t, err)
	require.NotNil(t, done.NextTaskID)
	next, err := tr.FindByID(ctx, *done.NextTaskID)
	require.NoError(t, err)
	assert.Equal(t, "Quarterly check-in", next.Title)
	assert.Equal(t, "high", next.Priority)
	assert.Equal(t, "3 mons", *next.Recurrence)
	assert.Equal(t, []int64{a.ID, b.ID}, next.PersonIDs)
	nextDue, err := time.Parse("2006-01-02", *next.DueAt)
	require.NoError(t, err)
	assert.True(t, nextDue.After(time.Now()))
}

func TestNextDue(t *testing.T) {
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	tests := []struct{ due, rec, want string }{
		{"2026-10-01", "7 days", "2026-10-15"},
		{"2026-10-20", "7 days", "2026-10-27"},
		{"2026-01-31", "1 mon", "2026-11-03"},
		{"2026-10-01 09:30:00", "weekly", "2026-10-15 09:30:00"},
		{"2020-02-29", "1 year", "2027-03-01"},
	}
	for _, tt := range tests {
		got, err := model.NextDue(tt.due, tt.rec, now)
		require.NoError(t, err, tt.due)
		assert.Equal(t, tt.want, got, "%s + %s", tt.due, tt.rec)
	}
}

func TestPersonBirthdaysWindowLooksBack(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	r := repo.NewPersonRepo(d)
	ctx := context.Background()
	mk := func(name, birthday string) {
		p, err := r.Create(ctx, model.CreatePersonInput{FirstName: name})
		require.NoError(t, err)
		_, err = r.Update(ctx, p.ID, model.UpdatePersonInput{Birthday: ptr(birthday)})
		require.NoError(t, err)
	}
	mk("Yesterday", "1990-10-12")
	mk("Today", "--10-13")
	mk("LastWeek", "2000-10-06")
	mk("LongAgo", "--06-01")    // 134 days back: outside a 90 day window
	mk("YearEnd", "1985-01-02") // 284 days back, but in 81 days ahead
	now := time.Date(2026, 10, 13, 15, 0, 0, 0, time.UTC)

	got, err := r.BirthdaysWindow(ctx, 90, 365, now)
	require.NoError(t, err)
	type item struct {
		name  string
		until int
		rel   string
	}
	var items []item
	for _, b := range got {
		items = append(items, item{b.Name, b.DaysUntil, b.Relative})
	}
	assert.Equal(t, []item{
		{"LastWeek", -7, "1w ago"},
		{"Yesterday", -1, "1d ago"},
		{"Today", 0, "today"},
		{"YearEnd", 81, "in 2mo 2w 6d"},
		{"LongAgo", 231, "in 7mo 2w 5d"},
		{"LastWeek", 358, "in 11mo 3w 2d"},
		{"Yesterday", 364, "in 11mo 4w 1d"},
	}, items)
	assert.Equal(t, "2026-10-06", got[0].Date)
	assert.Equal(t, 26, *got[0].Turning, "the age they turned on that date")
	assert.Equal(t, 36, *got[1].Turning)
	assert.Nil(t, got[2].Turning)

	// The look-ahead only form is unchanged: nothing in the past.
	ahead, _ := r.Birthdays(ctx, 365, now)
	for _, b := range ahead {
		assert.GreaterOrEqual(t, b.DaysUntil, 0)
		assert.NotEmpty(t, b.Relative)
	}
}
