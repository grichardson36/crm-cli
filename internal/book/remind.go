package book

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Draft kinds.
const (
	DraftReminder = "reminder"
	DraftFollowup = "followup"
)

// Draft is a message to the invitee, ready to send. Nothing is sent from here: delivery is up to
// the owner or an agent (the imap-smtp skill sends mail).
type Draft struct {
	BookingID int64  `json:"booking_id"`
	Link      string `json:"link"`
	Kind      string `json:"kind"`
	Minutes   int    `json:"minutes_before,omitempty"`
	SendAt    string `json:"send_at"`
	To        string `json:"to"`
	ReplyTo   string `json:"reply_to,omitempty"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
}

// Drafts renders the reminders and follow-ups that are due for confirmed bookings: a reminder when
// its time before the start has come (and the meeting has not started), a follow-up once its delay
// after the end has passed. Each is handed out once: Mark records it.
func Drafts(ctx context.Context, db *sql.DB, now time.Time) ([]Draft, error) {
	br := repo.NewBookingRepo(db)
	list, err := br.ListBookings(ctx, repo.BookingFilters{Status: model.BookingConfirmed})
	if err != nil {
		return nil, err
	}
	links := map[int64]*model.BookingLink{}
	out := []Draft{}
	for _, b := range list {
		l, ok := links[b.LinkID]
		if !ok {
			if l, err = br.FindLinkByID(ctx, b.LinkID); err != nil {
				return nil, err
			}
			links[b.LinkID] = l
		}
		start, err1 := tz.ParseInstant(b.Start)
		end, err2 := tz.ParseInstant(b.End)
		if err1 != nil || err2 != nil {
			continue
		}
		loc, err := HostZone(ctx, db, l)
		if err != nil {
			return nil, err
		}
		for _, m := range l.RemindersMin {
			if has(b.Reminded, m) {
				continue
			}
			fire := start.Add(-time.Duration(m) * time.Minute)
			if fire.After(now) || !start.After(now) {
				continue
			}
			out = append(out, reminderDraft(b, l, start, fire, m, loc, now))
		}
		if l.FollowupAfterMin != nil && !b.FollowupSent {
			send := end.Add(time.Duration(*l.FollowupAfterMin) * time.Minute)
			if !send.After(now) {
				out = append(out, followupDraft(b, l, start, send, loc))
			}
		}
	}
	return out, nil
}

func has(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func greeting(b *model.Booking) string {
	if n := strings.Fields(b.InviteeName); len(n) > 0 {
		return "Hi " + n[0] + ","
	}
	return "Hi,"
}

// whenText writes the start in the invitee's zone when known, and always in the host's.
func whenText(b *model.Booking, start time.Time, host *time.Location) string {
	text := start.In(host).Format("Monday 2 January at 15:04 MST")
	if b.TZ != "" && b.TZ != host.String() {
		if inv, err := tz.Load(b.TZ); err == nil {
			text += " (" + start.In(inv).Format("15:04 MST") + " in your zone)"
		}
	}
	return text
}

// cancelLine says how to cancel. Tier 0 has no cancel link, so the invitee writes back.
func cancelLine(l *model.BookingLink) string {
	if l.HostEmail != "" {
		return "To cancel or move it, reply to this email or write to " + l.HostEmail + "."
	}
	return "To cancel or move it, reply to this email."
}

func reminderDraft(b *model.Booking, l *model.BookingLink, start, fire time.Time, minutes int, host *time.Location, now time.Time) Draft {
	var body strings.Builder
	fmt.Fprintf(&body, "%s\n\nA reminder: %s is %s, on %s.\n", greeting(b), l.Title, humanIn(start.Sub(now)), whenText(b, start, host))
	if l.Location != "" {
		fmt.Fprintf(&body, "Where: %s\n", l.Location)
	}
	fmt.Fprintf(&body, "\n%s\n", cancelLine(l))
	return Draft{
		BookingID: b.ID, Link: l.Slug, Kind: DraftReminder, Minutes: minutes, SendAt: tz.FormatInstant(fire), To: b.Email, ReplyTo: l.HostEmail,
		Subject: fmt.Sprintf("Reminder: %s, %s", l.Title, start.In(host).Format("Mon 2 Jan 15:04 MST")), Body: body.String(),
	}
}

func followupDraft(b *model.Booking, l *model.BookingLink, start, send time.Time, host *time.Location) Draft {
	body := fmt.Sprintf("%s\n\nThank you for meeting on %s. Reply if there is anything I can follow up on.\n", greeting(b), start.In(host).Format("Monday 2 January"))
	return Draft{
		BookingID: b.ID, Link: l.Slug, Kind: DraftFollowup, SendAt: tz.FormatInstant(send), To: b.Email, ReplyTo: l.HostEmail,
		Subject: "Following up: " + l.Title, Body: body,
	}
}

// humanIn says "in 25 minutes", "in 2 hours", "in 1 day" for a message body.
func humanIn(d time.Duration) string {
	m := int(d / time.Minute)
	plural := func(n int, unit string) string {
		if n == 1 {
			return fmt.Sprintf("in 1 %s", unit)
		}
		return fmt.Sprintf("in %d %ss", n, unit)
	}
	switch {
	case m < 2:
		return "starting now"
	case m < 60:
		return plural(m, "minute")
	case m < 48*60:
		return plural((m+30)/60, "hour")
	}
	return plural(m/(24*60), "day")
}

// Mark records drafts as handed out.
func Mark(ctx context.Context, db *sql.DB, drafts []Draft) error {
	br := repo.NewBookingRepo(db)
	byID := map[int64]*model.Booking{}
	for _, d := range drafts {
		b, ok := byID[d.BookingID]
		if !ok {
			var err error
			if b, err = br.FindBooking(ctx, d.BookingID); err != nil {
				return err
			}
			byID[d.BookingID] = b
		}
		switch d.Kind {
		case DraftReminder:
			if !has(b.Reminded, d.Minutes) {
				b.Reminded = append(b.Reminded, d.Minutes)
			}
			if err := br.MarkReminded(ctx, b.ID, b.Reminded); err != nil {
				return err
			}
		case DraftFollowup:
			if err := br.MarkFollowup(ctx, b.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
