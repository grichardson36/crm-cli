package book

import (
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDraftsRemindersAndFollowUpAreHandedOutOnce(t *testing.T) {
	d := openDB(t)
	l := newLink(t, d, func(in *model.CreateBookingLinkInput) {
		in.RemindersMin = []int{24 * 60, 60}
		in.FollowupAfterMin = ip(120)
		in.Location = "https://meet.example.com/room"
	})
	start := at(t, 2026, 10, 13, 10, 0) // Tuesday 10:00 CDT
	res, err := Confirm(ctx, d, ConfirmInput{Link: l, Start: start, Name: "Jane Doe", Email: "jane@example.com", TZ: "America/New_York", Now: mon(t)})
	require.NoError(t, err)

	tests := []struct {
		name string
		now  time.Time
		want []string // kind:minutes
	}{
		{"long before", at(t, 2026, 10, 12, 9, 0), nil},
		{"a day before", at(t, 2026, 10, 12, 10, 0), []string{"reminder:1440"}},
		{"an hour before", at(t, 2026, 10, 13, 9, 0), []string{"reminder:1440", "reminder:60"}},
		{"after the start no reminder", at(t, 2026, 10, 13, 10, 1), nil},
		{"follow-up not yet", at(t, 2026, 10, 13, 11, 59), nil},
		{"follow-up due", at(t, 2026, 10, 13, 12, 30), []string{"followup:0"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Drafts(ctx, d, tt.now)
			require.NoError(t, err)
			var kinds []string
			for _, dr := range got {
				kinds = append(kinds, dr.Kind+":"+itoa(dr.Minutes))
			}
			assert.Equal(t, tt.want, kinds)
		})
	}

	got, err := Drafts(ctx, d, at(t, 2026, 10, 13, 9, 0))
	require.NoError(t, err)
	require.Len(t, got, 2)
	hour := got[1]
	assert.Equal(t, res.Booking.ID, hour.BookingID)
	assert.Equal(t, "jane@example.com", hour.To)
	assert.Equal(t, "me@example.com", hour.ReplyTo)
	assert.Equal(t, "Reminder: Intro call, Tue 13 Oct 10:00 CDT", hour.Subject)
	assert.Equal(t, "2026-10-13T14:00:00Z", hour.SendAt)
	assert.Equal(t, "Hi Jane,\n\nA reminder: Intro call is in 1 hour, on Tuesday 13 October at 10:00 CDT (11:00 EDT in your zone).\n"+
		"Where: https://meet.example.com/room\n\nTo cancel or move it, reply to this email or write to me@example.com.\n", hour.Body)

	require.NoError(t, Mark(ctx, d, got))
	again, err := Drafts(ctx, d, at(t, 2026, 10, 13, 9, 30))
	require.NoError(t, err)
	assert.Empty(t, again, "handed out once")

	late, err := Drafts(ctx, d, at(t, 2026, 10, 13, 12, 30))
	require.NoError(t, err)
	require.Len(t, late, 1)
	assert.Equal(t, "Following up: Intro call", late[0].Subject)
	assert.Contains(t, late[0].Body, "Thank you for meeting on Tuesday 13 October")
	require.NoError(t, Mark(ctx, d, late))
	none, _ := Drafts(ctx, d, at(t, 2026, 10, 14, 12, 30))
	assert.Empty(t, none)
}

func TestDraftsSkipCancelledBookingsAndWorkWithoutAnyAddress(t *testing.T) {
	d := openDB(t)
	l := newLink(t, d, func(in *model.CreateBookingLinkInput) { in.RemindersMin = []int{60}; in.HostEmail = "" })
	res, err := Confirm(ctx, d, ConfirmInput{Link: l, Start: at(t, 2026, 10, 13, 10, 0), Email: "bob@example.com", Now: mon(t)})
	require.NoError(t, err)
	got, err := Drafts(ctx, d, at(t, 2026, 10, 13, 9, 30))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Hi,\n\nA reminder: Intro call is in 30 minutes, on Tuesday 13 October at 10:00 CDT.\n\nTo cancel or move it, reply to this email.\n", got[0].Body)
	assert.Empty(t, got[0].ReplyTo)

	_, _, err = Cancel(ctx, d, res.Booking.ID, false)
	require.NoError(t, err)
	got, err = Drafts(ctx, d, at(t, 2026, 10, 13, 9, 30))
	require.NoError(t, err)
	assert.Empty(t, got)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for ; n > 0; n /= 10 {
		s = string(rune('0'+n%10)) + s
	}
	return s
}
