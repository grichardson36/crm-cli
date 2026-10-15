package book

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

func chicago(t *testing.T) *time.Location {
	t.Helper()
	loc, err := tz.Load("America/Chicago")
	require.NoError(t, err)
	return loc
}

// at builds an instant from a Chicago wall clock.
func at(t *testing.T, y int, mo time.Month, d, h, mi int) time.Time {
	t.Helper()
	return tz.Instant(time.Date(y, mo, d, h, mi, 0, 0, time.UTC), chicago(t))
}

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	_, err = repo.NewAvailabilityRepo(d).SetDays(ctx, "", "America/Chicago", model.WeekdayKeys[:5], []model.TimeRange{{Start: "09:00", End: "17:00"}})
	require.NoError(t, err)
	return d
}

func ip(n int) *int { return &n }

func newLink(t *testing.T, d *sql.DB, mod func(*model.CreateBookingLinkInput)) *model.BookingLink {
	t.Helper()
	in := model.CreateBookingLinkInput{Name: "Intro call", Slug: "intro", HostEmail: "me@example.com", MinNoticeMin: ip(0), HorizonDays: 3, SlotStepMin: 30}
	if mod != nil {
		mod(&in)
	}
	l, err := repo.NewBookingRepo(d).CreateLink(ctx, in)
	require.NoError(t, err)
	return l
}

// monday 09:00 Chicago.
func mon(t *testing.T) time.Time { return at(t, 2026, 10, 12, 9, 0) }

func hm(t *testing.T, r *Result) []string {
	t.Helper()
	loc := chicago(t)
	var out []string
	for _, s := range r.Slots {
		out = append(out, s.Start.In(loc).Format("Mon 15:04"))
	}
	return out
}

func slots(t *testing.T, d *sql.DB, l *model.BookingLink, now time.Time) *Result {
	t.Helper()
	r, err := Slots(ctx, d, l, 0, now)
	require.NoError(t, err)
	return r
}

func TestSlotsRespectHoursNoticeHorizonAndStep(t *testing.T) {
	d := openDB(t)
	tests := []struct {
		name  string
		mod   func(*model.CreateBookingLinkInput)
		now   time.Time
		first string
		last  string
		count int
	}{
		{"whole days", nil, mon(t), "Mon 09:00", "Wed 16:30", 48},
		{"notice pushes the first slot", func(in *model.CreateBookingLinkInput) { n := 240; in.MinNoticeMin = &n }, mon(t), "Mon 13:00", "Wed 16:30", 40},
		{"hourly step", func(in *model.CreateBookingLinkInput) { in.SlotStepMin = 60 }, mon(t), "Mon 09:00", "Wed 16:00", 24},
		{"60 minute meeting", func(in *model.CreateBookingLinkInput) { in.Durations = []int{60} }, mon(t), "Mon 09:00", "Wed 16:00", 45},
		{"weekend is empty", nil, at(t, 2026, 10, 17, 9, 0), "Mon 09:00", "Mon 16:30", 16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newLink(t, d, func(in *model.CreateBookingLinkInput) {
				if tt.mod != nil {
					tt.mod(in)
				}
				in.Slug = "s-" + strings.ReplaceAll(tt.name, " ", "-")
			})
			got := hm(t, slots(t, d, l, tt.now))
			require.NotEmpty(t, got)
			assert.Equal(t, tt.first, got[0])
			assert.Equal(t, tt.last, got[len(got)-1])
			assert.Len(t, got, tt.count)
		})
	}
}

func TestSlotsStartAfterAndHorizonEnd(t *testing.T) {
	d := openDB(t)
	after := "2026-10-14"
	l := newLink(t, d, func(in *model.CreateBookingLinkInput) { in.StartAfter = &after; in.HorizonDays = 14 })
	got := hm(t, slots(t, d, l, mon(t)))
	assert.Equal(t, "Wed 09:00", got[0], "nothing before the start date")
	assert.Equal(t, "Fri 16:30", got[len(got)-1], "the horizon ends Monday 09:00")
}

func TestSlotsBuffersKeepEventsClear(t *testing.T) {
	d := openDB(t)
	_, err := repo.NewEventRepo(d).Create(ctx, model.CreateEventInput{Title: "Secret", Start: at(t, 2026, 10, 12, 10, 0), End: at(t, 2026, 10, 12, 11, 0), TZ: "America/Chicago"})
	require.NoError(t, err)
	tests := []struct {
		name          string
		before, after int
		want          []string
	}{
		{"no buffer", 0, 0, []string{"Mon 09:00", "Mon 09:30", "Mon 11:00", "Mon 11:30"}},
		{"15 min each side", 15, 15, []string{"Mon 09:00", "Mon 11:30", "Mon 12:00"}},
		{"only after the meeting", 0, 30, []string{"Mon 09:00", "Mon 11:00", "Mon 11:30"}},
		{"only before the meeting", 30, 0, []string{"Mon 09:00", "Mon 09:30", "Mon 11:30"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newLink(t, d, func(in *model.CreateBookingLinkInput) {
				in.BufferBeforeMin, in.BufferAfterMin = tt.before, tt.after
				in.HorizonDays = 1
				in.Slug = "b" + strings.ReplaceAll(tt.name, " ", "")
			})
			got := hm(t, slots(t, d, l, mon(t)))
			assert.Equal(t, tt.want, got[:len(tt.want)])
		})
	}
}

func TestSlotsAcrossDaylightSavingKeepWallClock(t *testing.T) {
	d := openDB(t)
	l := newLink(t, d, func(in *model.CreateBookingLinkInput) { in.HorizonDays = 6 })
	r := slots(t, d, l, at(t, 2026, 10, 30, 9, 0)) // Friday before the clocks go back on Nov 1
	var friday, monday []time.Time
	for _, s := range r.Slots {
		switch s.Start.In(chicago(t)).Weekday() {
		case time.Friday:
			friday = append(friday, s.Start)
		case time.Monday:
			monday = append(monday, s.Start)
		}
	}
	require.NotEmpty(t, monday)
	assert.Equal(t, "2026-10-30T14:00:00Z", tz.FormatInstant(friday[0]), "09:00 CDT")
	assert.Equal(t, "2026-11-02T15:00:00Z", tz.FormatInstant(monday[0]), "09:00 CST, an hour later in UTC")
}

func TestSlotsCaps(t *testing.T) {
	tests := []struct {
		name     string
		mod      func(*model.CreateBookingLinkInput)
		full     func(day time.Time) bool // days that must have no slots
		wantOpen []string
	}{
		{"per day", func(in *model.CreateBookingLinkInput) { in.MaxPerDay = ip(1) },
			func(d time.Time) bool { return d.Day() == 12 }, []string{"2026-10-13", "2026-10-14"}},
		{"per week", func(in *model.CreateBookingLinkInput) { in.MaxPerWeek = ip(1); in.HorizonDays = 10 },
			func(d time.Time) bool { return d.Day() >= 12 && d.Day() <= 16 }, []string{"2026-10-19", "2026-10-20"}},
		{"per month", func(in *model.CreateBookingLinkInput) { in.MaxPerMonth = ip(1); in.HorizonDays = 30 },
			func(d time.Time) bool { return d.Month() == time.October }, []string{"2026-11-02", "2026-11-03"}},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := openDB(t)
			l := newLink(t, d, func(in *model.CreateBookingLinkInput) { tt.mod(in); in.Slug = "cap" + string(rune('a'+i)) })
			_, err := repo.NewBookingRepo(d).InsertBooking(ctx, model.Booking{LinkID: l.ID, Email: "a@x.com", Start: tz.FormatInstant(at(t, 2026, 10, 12, 9, 0)), End: tz.FormatInstant(at(t, 2026, 10, 12, 9, 30)), Status: "confirmed"},
				tz.FormatInstant(at(t, 2026, 10, 12, 9, 0)), tz.FormatInstant(at(t, 2026, 10, 12, 9, 30)))
			require.NoError(t, err)
			days := map[string]bool{}
			for _, s := range slots(t, d, l, mon(t)).Slots {
				w := s.Start.In(chicago(t))
				days[w.Format("2006-01-02")] = true
				assert.False(t, tt.full(w), "%s should be full", w.Format("2006-01-02"))
			}
			for _, day := range tt.wantOpen {
				assert.True(t, days[day], "%s should be open", day)
			}
		})
	}
}

func TestLinkStates(t *testing.T) {
	d := openDB(t)
	br := repo.NewBookingRepo(d)
	l := newLink(t, d, nil)
	require.NoError(t, br.SetLinkActive(ctx, l.ID, false))
	l, _ = br.FindLink(ctx, "intro")
	r := slots(t, d, l, mon(t))
	assert.Equal(t, ReasonPaused, r.Reason)
	assert.Empty(t, r.Slots)

	exp := "2026-10-12T20:00:00Z"
	l2 := newLink(t, d, func(in *model.CreateBookingLinkInput) { in.Slug = "exp"; in.ExpiresAt = &exp })
	assert.Empty(t, slots(t, d, l2, mon(t)).Reason, "before the expiry")
	assert.Equal(t, ReasonExpired, slots(t, d, l2, at(t, 2026, 10, 12, 15, 0)).Reason)

	once := newLink(t, d, func(in *model.CreateBookingLinkInput) { in.Slug = "once"; in.MaxUses = ip(1) })
	res, err := Confirm(ctx, d, ConfirmInput{Link: once, Start: at(t, 2026, 10, 12, 9, 0), Email: "a@x.com", Now: mon(t)})
	require.NoError(t, err)
	assert.NotNil(t, res.Booking)
	assert.Equal(t, ReasonUsedUp, slots(t, d, once, mon(t)).Reason, "a one-time link is spent after one booking")

	_, err = Slots(ctx, d, l2, 45, mon(t))
	assert.ErrorIs(t, err, model.ErrValidation, "a length the link does not offer")
}

func TestOfferTextIsDeterministicAndShowsTheInviteeZone(t *testing.T) {
	d := openDB(t)
	l := newLink(t, d, func(in *model.CreateBookingLinkInput) {
		in.Title = "Intro call"
		in.Location = "https://meet.example.com/room"
		in.HorizonDays = 1
	})
	r := slots(t, d, l, at(t, 2026, 10, 12, 15, 0))
	r.Limit(3)
	ny, err := tz.Load("America/New_York")
	require.NoError(t, err)
	got, err := Offer(r, ny)
	require.NoError(t, err)
	want := "Intro call (30 min)\nWhere: https://meet.example.com/room\nTimes are in America/Chicago.\nYour time (America/New_York) is in brackets.\n\n" +
		"Mon 12 Oct: 15:00 [16:00], 15:30 [16:30], 16:00 [17:00]\n\nReply with the time you want.\n"
	assert.Equal(t, want, got)
	again, _ := Offer(r, ny)
	assert.Equal(t, got, again)

	plain, _ := Offer(r, nil)
	assert.NotContains(t, plain, "[")
	empty := &Result{Link: l, TZ: "America/Chicago", LengthMin: 30}
	text, _ := Offer(empty, nil)
	assert.Contains(t, text, "No times are open")
}

func TestMailtoAndHTMLShowFreeTimesOnly(t *testing.T) {
	d := openDB(t)
	_, err := repo.NewEventRepo(d).Create(ctx, model.CreateEventInput{Title: "Private therapy session", Start: at(t, 2026, 10, 12, 10, 0), End: at(t, 2026, 10, 12, 11, 0), TZ: "America/Chicago"})
	require.NoError(t, err)
	l := newLink(t, d, func(in *model.CreateBookingLinkInput) {
		in.Title = "Intro <call>"
		in.Questions = []model.BookingQuestion{{Label: "Agenda", Required: true}}
	})
	r := slots(t, d, l, mon(t))
	m := Mailto(l, r.Slots[0], 30, chicago(t))
	assert.True(t, strings.HasPrefix(m, "mailto:me@example.com?subject=Book%20Intro%20%3Ccall%3E"), m)
	assert.Contains(t, m, "body=Hi")
	assert.Contains(t, m, "Slot%3A%20intro%2F2026-10-12T14%3A00%3A00Z%2F30")
	assert.Contains(t, m, "Agenda%3A")
	assert.NotContains(t, m, "+")

	page, err := HTML(r, mon(t))
	require.NoError(t, err)
	text := string(page)
	assert.NotContains(t, text, "therapy", "event titles never reach the page")
	assert.NotContains(t, text, "Private")
	assert.Contains(t, text, "Intro &lt;call&gt;", "titles are escaped")
	assert.Contains(t, text, `href="mailto:me@example.com?subject=`)
	assert.Contains(t, text, "Monday 12 October")
	assert.Contains(t, text, "09:00</a>")
	for _, s := range r.Slots {
		assert.NotEqual(t, "Mon 10:00", s.Start.In(chicago(t)).Format("Mon 15:04"), "the busy hour is not offered")
	}
	assert.NoError(t, NeedsHost(l))
	assert.ErrorIs(t, NeedsHost(&model.BookingLink{Slug: "x"}), model.ErrValidation)
}

func TestConfirmCreatesEverythingAndRejectsTheSameTime(t *testing.T) {
	d := openDB(t)
	pr := repo.NewPersonRepo(d)
	jane, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Jane", LastName: strPtr("Doe")})
	require.NoError(t, err)
	require.NoError(t, pr.AddEmail(ctx, jane.ID, "jane@example.com", nil, false))
	l := newLink(t, d, func(in *model.CreateBookingLinkInput) {
		in.RemindersMin = []int{60}
		in.Location = "Cafe Rio"
		in.Questions = []model.BookingQuestion{{Label: "Agenda", Required: true}}
		in.BufferAfterMin = 15
	})
	start := at(t, 2026, 10, 12, 10, 0)

	_, err = Confirm(ctx, d, ConfirmInput{Link: l, Start: start, Email: "jane@example.com", Now: mon(t)})
	assert.ErrorIs(t, err, model.ErrValidation, "required answers")
	_, err = Confirm(ctx, d, ConfirmInput{Link: l, Start: start, Email: "nope", Answers: map[string]string{"Agenda": "x"}, Now: mon(t)})
	assert.ErrorIs(t, err, model.ErrValidation)
	_, err = Confirm(ctx, d, ConfirmInput{Link: l, Start: at(t, 2026, 10, 12, 7, 0), Email: "jane@example.com", Answers: map[string]string{"Agenda": "x"}, Now: mon(t)})
	assert.ErrorIs(t, err, model.ErrConflict, "outside working hours")

	preview, err := Confirm(ctx, d, ConfirmInput{Link: l, Start: start, Email: "Jane@Example.com", Answers: map[string]string{"Agenda": "pricing"}, Now: mon(t), DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, "Jane Doe", preview.Person)
	var n int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM bookings`).Scan(&n))
	assert.Equal(t, 0, n, "a dry run writes nothing")

	res, err := Confirm(ctx, d, ConfirmInput{Link: l, Start: start, Name: "Jane Doe", Email: "Jane@Example.com", TZ: "America/New_York",
		Answers: map[string]string{"Agenda": "pricing"}, FollowUp: true, Now: mon(t)})
	require.NoError(t, err)
	assert.Equal(t, jane.ID, res.PersonID)
	assert.Equal(t, "Intro call with Jane Doe", res.Event.Title)
	assert.Equal(t, "2026-10-12T15:00:00Z", *res.Event.StartUTC)
	assert.Equal(t, "Cafe Rio", *res.Event.Location)
	assert.Equal(t, "Agenda: pricing", *res.Event.Notes)
	require.Len(t, res.Event.Attendees, 1)
	assert.Equal(t, jane.ID, res.Event.Attendees[0].PersonID)
	require.Len(t, res.Event.Reminders, 1)
	assert.Equal(t, 60, res.Event.Reminders[0].MinutesBefore)
	assert.Equal(t, "jane@example.com", res.Booking.Email)
	require.NotNil(t, res.Booking.EventID)
	assert.NotZero(t, res.TaskID)

	var subject, occurred string
	require.NoError(t, d.QueryRow(`SELECT subject, occurred_at FROM interactions WHERE type = 'meeting'`).Scan(&subject, &occurred))
	assert.Equal(t, "Intro call with Jane Doe", subject)
	assert.Equal(t, "2026-10-12 15:00:00", occurred)
	var due string
	require.NoError(t, d.QueryRow(`SELECT due_at FROM tasks WHERE id = ?`, res.TaskID).Scan(&due))
	assert.Equal(t, "2026-10-13", due, "one day after the meeting")

	inv := strings.ReplaceAll(string(res.Invite), "\r\n ", "") // unfold long lines
	for _, want := range []string{"METHOD:REQUEST", "BEGIN:VEVENT", "ORGANIZER", "me@example.com", "ATTENDEE", "jane@example.com", "SUMMARY:Intro call with Jane Doe"} {
		assert.Contains(t, inv, want)
	}

	// The same time, and a time that is too close because of the 15 minute buffer, are refused.
	for _, s := range []time.Time{start, at(t, 2026, 10, 12, 9, 30)} {
		_, err = Confirm(ctx, d, ConfirmInput{Link: l, Start: s, Email: "bob@example.com", Answers: map[string]string{"Agenda": "x"}, Now: mon(t)})
		assert.ErrorIs(t, err, model.ErrConflict, s.String())
	}
	ok, err := Confirm(ctx, d, ConfirmInput{Link: l, Start: at(t, 2026, 10, 12, 11, 30), Email: "bob@example.com", Answers: map[string]string{"Agenda": "x"}, Now: mon(t)})
	require.NoError(t, err)
	assert.Contains(t, ok.Note, "no contact has this address")
	assert.Len(t, ok.Event.Guests, 1, "an unknown invitee is kept as a guest, no contact is created")
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM people`).Scan(&n))
	assert.Equal(t, 1, n)
}

func strPtr(s string) *string { return &s }

func TestTwoConfirmationsOfTheSameTimeCannotBothWin(t *testing.T) {
	d := openDB(t)
	l := newLink(t, d, nil)
	start := at(t, 2026, 10, 13, 14, 0)
	var wg sync.WaitGroup
	results := make([]error, 8)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = Confirm(ctx, d, ConfirmInput{Link: l, Start: start, Email: "p" + string(rune('a'+i)) + "@x.com", Now: mon(t)})
		}()
	}
	wg.Wait()
	won := 0
	for _, err := range results {
		if err == nil {
			won++
		}
	}
	assert.Equal(t, 1, won)
	var n int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM bookings WHERE archived = 0 AND status = 'confirmed'`).Scan(&n))
	assert.Equal(t, 1, n)
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM events WHERE archived = 0 AND source = 'booking'`).Scan(&n))
	assert.Equal(t, 1, n, "no orphan events from the losers")
}

func TestCancelFreesTheTimeAndBuildsACancelInvite(t *testing.T) {
	d := openDB(t)
	l := newLink(t, d, nil)
	start := at(t, 2026, 10, 12, 10, 0)
	res, err := Confirm(ctx, d, ConfirmInput{Link: l, Start: start, Name: "Bob", Email: "bob@x.com", Now: mon(t)})
	require.NoError(t, err)
	assert.False(t, slots(t, d, l, mon(t)).Offers(start))

	b, cancel, err := Cancel(ctx, d, res.Booking.ID, true)
	require.NoError(t, err)
	assert.Equal(t, "confirmed", b.Status, "a dry run changes nothing")
	assert.Contains(t, string(cancel), "METHOD:CANCEL")
	assert.Contains(t, string(cancel), "STATUS:CANCELLED")

	b, _, err = Cancel(ctx, d, res.Booking.ID, false)
	require.NoError(t, err)
	assert.Equal(t, "cancelled", b.Status)
	assert.True(t, slots(t, d, l, mon(t)).Offers(start), "the time is open again")
	var n int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM events WHERE archived = 0`).Scan(&n))
	assert.Equal(t, 0, n)
	_, _, err = Cancel(ctx, d, res.Booking.ID, false)
	assert.ErrorIs(t, err, model.ErrConflict)
}

func TestLinkValidation(t *testing.T) {
	d := openDB(t)
	br := repo.NewBookingRepo(d)
	tests := []struct {
		name string
		in   model.CreateBookingLinkInput
	}{
		{"no name", model.CreateBookingLinkInput{}},
		{"bad slug", model.CreateBookingLinkInput{Name: "x", Slug: "Not A Slug"}},
		{"bad duration", model.CreateBookingLinkInput{Name: "x", Durations: []int{0}}},
		{"bad cap", model.CreateBookingLinkInput{Name: "x", MaxPerDay: ip(0)}},
		{"bad location kind", model.CreateBookingLinkInput{Name: "x", LocationKind: "teleport"}},
		{"bad email", model.CreateBookingLinkInput{Name: "x", HostEmail: "nope"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := br.CreateLink(ctx, tt.in)
			assert.ErrorIs(t, err, model.ErrValidation)
		})
	}
	l, err := br.CreateLink(ctx, model.CreateBookingLinkInput{Name: "Sales Intro!"})
	require.NoError(t, err)
	assert.Equal(t, "sales-intro", l.Slug)
	assert.Equal(t, 240, l.MinNoticeMin)
	assert.Equal(t, []int{30}, l.Durations)
	_, err = br.CreateLink(ctx, model.CreateBookingLinkInput{Name: "Sales Intro!"})
	assert.ErrorIs(t, err, model.ErrConflict)
	got, err := br.FindLink(ctx, "1")
	require.NoError(t, err)
	assert.Equal(t, l.ID, got.ID)
}
