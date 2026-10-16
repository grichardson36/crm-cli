package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/book"
	"github.com/jdanielnd/crm-cli/internal/cal/ics"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/cal/when"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
)

// Booking links (Tier 0) from the palette. Every prompt is one line of parts split by "|".

func (m *Model) startBookPrompt(kind, prompt, initial string) tea.Cmd {
	m.startInput(kind, prompt, initial)
	return m.input.Focus()
}

func parts(value string) []string {
	raw := strings.Split(value, "|")
	out := make([]string, len(raw))
	for i, p := range raw {
		out[i] = strings.TrimSpace(p)
	}
	return out
}

func part(p []string, i int) string {
	if i < len(p) {
		return p[i]
	}
	return ""
}

func (m *Model) runBookPrompt(kind, value string) {
	p := parts(value)
	var err error
	switch kind {
	case "book-add":
		err = m.bookAdd(p)
	case "book-offer":
		err = m.bookOffer(p)
	case "book-export":
		err = m.bookExport(p)
	case "book-confirm":
		err = m.bookConfirm(p)
	case "book-cancel":
		err = m.bookCancel(p)
	case "book-pause", "book-resume", "book-rm":
		err = m.bookLinkChange(kind, part(p, 0))
	}
	if err != nil {
		m.setStatus(err.Error(), true)
	}
}

// bookShowLinks lists links and upcoming bookings in a page.
func (m *Model) bookShowLinks() {
	br := repo.NewBookingRepo(m.db)
	links, err := br.ListLinks(m.ctx)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	var b strings.Builder
	if len(links) == 0 {
		b.WriteString("No booking links yet. Palette: \"Booking: add link\".\n")
	}
	for _, l := range links {
		state := ""
		if !l.Active {
			state = "  (paused)"
		}
		fmt.Fprintf(&b, "%-14s %s  %v min  %d days ahead  notice %d min%s\n", l.Slug, l.Title, l.Durations, l.HorizonDays, l.MinNoticeMin, state)
	}
	list, err := br.ListBookings(m.ctx, repo.BookingFilters{From: tz.FormatInstant(m.now().Add(-24 * time.Hour))})
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	if len(list) > 0 {
		b.WriteString("\nbooked meetings\n")
	}
	for _, bk := range list {
		s, _ := tz.ParseInstant(bk.Start)
		fmt.Fprintf(&b, "#%d  %s  %s  %s  (%s)\n", bk.ID, s.In(m.loc).Format("Mon 2 Jan 15:04"), bk.Link, bk.Email, bk.Status)
	}
	b.WriteString("\nsame as crm book list. Offer, export, confirm and cancel are in the palette (crm help book).\n")
	m.openText("Booking links", b.String(), nil)
}

func (m *Model) bookAdd(p []string) error {
	sp := book.LinkSpec{Name: part(p, 0), Lens: part(p, 1), Email: part(p, 2), Notice: "4h"}
	in, err := sp.Input(m.now(), m.loc)
	if err != nil {
		return err
	}
	l, err := repo.NewBookingRepo(m.db).CreateLink(m.ctx, in)
	if err != nil {
		return err
	}
	m.setStatus("link "+l.Slug+" created", false)
	return nil
}

func (m *Model) bookSlots(link, length string, count int) (*book.Result, error) {
	l, err := repo.NewBookingRepo(m.db).FindLink(m.ctx, link)
	if err != nil {
		return nil, err
	}
	n := 0
	if length != "" {
		d, err := when.ParseLength(length)
		if err != nil {
			return nil, err
		}
		n = int(d / time.Minute)
	}
	res, err := book.Slots(m.ctx, m.db, l, n, m.now())
	if err != nil {
		return nil, err
	}
	res.Limit(count)
	return res, nil
}

func (m *Model) bookOffer(p []string) error {
	count := 10
	if v := part(p, 2); v != "" {
		if _, err := fmt.Sscanf(v, "%d", &count); err != nil {
			return fmt.Errorf("count must be a number")
		}
	}
	res, err := m.bookSlots(part(p, 0), part(p, 1), count)
	if err != nil {
		return err
	}
	var inv *time.Location
	if name := part(p, 3); name != "" {
		if inv, err = tz.Load(name); err != nil {
			return err
		}
	}
	text, err := book.Offer(res, inv)
	if err != nil {
		return err
	}
	if m.clip != nil {
		m.clip(text)
	}
	m.openText("Times to send (copied)", text, nil)
	return nil
}

func (m *Model) bookExport(p []string) error {
	res, err := m.bookSlots(part(p, 0), "", 60)
	if err != nil {
		return err
	}
	if err := book.NeedsHost(res.Link); err != nil {
		return err
	}
	page, err := book.HTML(res, m.now())
	if err != nil {
		return err
	}
	out := part(p, 1)
	if out == "" {
		return fmt.Errorf("say where to write: link | folder")
	}
	path := out
	if !strings.HasSuffix(strings.ToLower(out), ".html") {
		if err := os.MkdirAll(out, 0o755); err != nil {
			return err
		}
		path = filepath.Join(out, "index.html")
	}
	if err := os.WriteFile(path, page, 0o644); err != nil {
		return err
	}
	m.setStatus(fmt.Sprintf("wrote %s with %d times", path, len(res.Slots)), false)
	return nil
}

func (m *Model) bookConfirm(p []string) error {
	ref := part(p, 0)
	var start time.Time
	lenMin := 0
	if slug, t, n, ok := book.SplitToken(ref); ok {
		ref, start, lenMin = slug, t, n
	}
	l, err := repo.NewBookingRepo(m.db).FindLink(m.ctx, ref)
	if err != nil {
		return err
	}
	if v := part(p, 1); v != "" {
		if start, err = book.ResolveStart(m.ctx, m.db, l, v, m.now()); err != nil {
			return err
		}
	}
	if start.IsZero() {
		return fmt.Errorf("say which time: link | tue 10:00 | name | email")
	}
	res, err := book.Confirm(m.ctx, m.db, book.ConfirmInput{
		Link: l, Start: start, LengthMin: lenMin, Name: part(p, 2), Email: part(p, 3), Now: m.now(),
	})
	if err != nil {
		return err
	}
	msg := fmt.Sprintf("booked %s with %s", start.In(m.loc).Format("Mon 2 Jan 15:04"), part(p, 3))
	if file := part(p, 4); file != "" {
		if err := os.WriteFile(file, res.Invite, 0o600); err != nil {
			return err
		}
		msg += ", invitation in " + file
	}
	if res.Note != "" {
		msg += " (" + res.Note + ")"
	}
	m.afterWriteCal(m.calToday(), res.Event.ID)
	m.setStatus(msg, false)
	return nil
}

func (m *Model) bookCancel(p []string) error {
	var id int64
	if _, err := fmt.Sscanf(part(p, 0), "%d", &id); err != nil {
		return fmt.Errorf("booking id must be a number")
	}
	b, cancel, err := book.Cancel(m.ctx, m.db, id, false)
	if err != nil {
		return err
	}
	msg := fmt.Sprintf("booking %d cancelled", b.ID)
	if file := part(p, 1); file != "" && cancel != nil {
		if err := os.WriteFile(file, cancel, 0o600); err != nil {
			return err
		}
		msg += ", cancellation in " + file
	}
	m.afterWriteCal(m.calToday(), 0)
	m.setStatus(msg, false)
	return nil
}

func (m *Model) bookLinkChange(kind, ref string) error {
	br := repo.NewBookingRepo(m.db)
	l, err := br.FindLink(m.ctx, ref)
	if err != nil {
		return err
	}
	switch kind {
	case "book-pause":
		err = br.SetLinkActive(m.ctx, l.ID, false)
	case "book-resume":
		err = br.SetLinkActive(m.ctx, l.ID, true)
	default:
		err = br.ArchiveLink(m.ctx, l.ID)
	}
	if err != nil {
		return err
	}
	m.setStatus(strings.TrimPrefix(kind, "book-")+": "+l.Slug, false)
	return nil
}

// calInviteRun writes an invitation for the event picked in the Calendar view.
func (m *Model) calInviteRun(value string) {
	p := parts(value)
	pend := m.cal.pending
	if pend == nil {
		return
	}
	var to []ics.Person
	for _, a := range strings.FieldsFunc(part(p, 0), func(r rune) bool { return r == ',' || r == ' ' }) {
		to = append(to, ics.Person{Email: a})
	}
	if len(to) == 0 || part(p, 2) == "" {
		m.setStatus("say who and where: to | from | file", true)
		return
	}
	data, err := book.Invite(m.ctx, m.db, pend.eventID, part(p, 1), "", to, "REQUEST")
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	if err := os.WriteFile(part(p, 2), data, 0o600); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.setStatus("invitation written to "+part(p, 2), false)
}

func (m *Model) startCalInvite() tea.Cmd {
	in := m.calEventTarget()
	if in == nil {
		return nil
	}
	m.cal.pending = &calPending{eventID: in.SeriesID}
	return m.startBookPrompt("cal-invite", "invite (to | your address | file.ics): ", " |  | invite.ics")
}
