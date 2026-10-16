package book

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Offer renders the slots as plain text that can be pasted into a message. Times are in the
// host's zone. With an invitee zone each time also shows there, so nobody does the sum.
// The same input always gives the same text.
func Offer(r *Result, invitee *time.Location) (string, error) {
	loc, err := tz.Load(r.TZ)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%d min)\n", r.Link.Title, r.LengthMin)
	if r.Link.Description != "" {
		fmt.Fprintf(&b, "%s\n", r.Link.Description)
	}
	if r.Link.Location != "" {
		fmt.Fprintf(&b, "Where: %s\n", r.Link.Location)
	}
	if len(r.Slots) == 0 {
		b.WriteString("\nNo times are open right now.\n")
		return b.String(), nil
	}
	fmt.Fprintf(&b, "Times are in %s.\n", r.TZ)
	if invitee != nil && invitee.String() != loc.String() {
		fmt.Fprintf(&b, "Your time (%s) is in brackets.\n", invitee.String())
	}
	b.WriteString("\n")
	day := ""
	var times []string
	flush := func() {
		if day != "" {
			fmt.Fprintf(&b, "%s: %s\n", day, strings.Join(times, ", "))
		}
	}
	for _, s := range r.Slots {
		st := s.Start.In(loc)
		d := st.Format("Mon 2 Jan")
		if d != day {
			flush()
			day, times = d, nil
		}
		t := st.Format("15:04")
		if invitee != nil && invitee.String() != loc.String() {
			t += " [" + s.Start.In(invitee).Format("15:04") + "]"
		}
		times = append(times, t)
	}
	flush()
	b.WriteString("\nReply with the time you want.\n")
	return b.String(), nil
}

// Mailto builds the mail link that asks for one slot. The body carries a line the host can
// read back with `crm book confirm`.
func Mailto(l *model.BookingLink, s Slot, lengthMin int, loc *time.Location) string {
	when := s.Start.In(loc).Format("Mon 2 Jan 15:04 MST")
	subject := fmt.Sprintf("Book %s, %s", l.Title, when)
	var body strings.Builder
	fmt.Fprintf(&body, "Hi,\r\n\r\nI would like to book \"%s\" (%d min) on %s.\r\n\r\n", l.Title, lengthMin, when)
	fmt.Fprintf(&body, "Slot: %s/%s/%d\r\n", l.Slug, tz.FormatInstant(s.Start), lengthMin)
	body.WriteString("Name:\r\n")
	for _, q := range l.Questions {
		fmt.Fprintf(&body, "%s:\r\n", q.Label)
	}
	return "mailto:" + url.PathEscape(l.HostEmail) + "?subject=" + escape(subject) + "&body=" + escape(body.String())
}

// escape percent-encodes with %20 for spaces, as mail clients expect in mailto links.
func escape(s string) string { return strings.ReplaceAll(url.QueryEscape(s), "+", "%20") }
