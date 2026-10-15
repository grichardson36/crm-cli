package tui

import (
	"fmt"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/book"
	"github.com/jdanielnd/crm-cli/internal/remind"
)

// showReminders lists what `crm cal due-reminders` and `crm book remind` would hand out, and marks
// nothing. The copy is the booking messages, ready to paste into a mail.
func (m *Model) showReminders() {
	due, err := remind.List(m.ctx, m.db, m.now(), 0, m.loc)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	drafts, err := book.Drafts(m.ctx, m.db, m.now())
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	var b strings.Builder
	if len(due) == 0 && len(drafts) == 0 {
		b.WriteString("Nothing is due.\n")
	}
	if len(due) > 0 {
		b.WriteString("Event reminders\n")
		for _, d := range due {
			fmt.Fprintf(&b, "%s  %s  %s  (%s)\n", d.LocalStart, d.Title, d.StartsIn, d.Method)
		}
	}
	if len(drafts) > 0 {
		b.WriteString("\nBooking messages\n")
		for _, d := range drafts {
			fmt.Fprintf(&b, "\nTo: %s\nSubject: %s\n\n%s", d.To, d.Subject, d.Body)
		}
	}
	b.WriteString("\nPalette \"Reminders: mark due as sent\" marks all of these, same as running the commands without --dry-run.\n")
	m.openText("Reminders due", b.String(), nil)
}

func (m *Model) markReminders() {
	due, err := remind.List(m.ctx, m.db, m.now(), 0, m.loc)
	if err == nil {
		err = remind.Mark(m.ctx, m.db, due)
	}
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	drafts, err := book.Drafts(m.ctx, m.db, m.now())
	if err == nil {
		err = book.Mark(m.ctx, m.db, drafts)
	}
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.afterWrite()
	m.setStatus(fmt.Sprintf("marked %d reminders and %d booking messages as sent", len(due), len(drafts)), false)
}
