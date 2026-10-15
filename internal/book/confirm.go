package book

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/ics"
	"github.com/jdanielnd/crm-cli/internal/cal/icsio"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// ConfirmInput is a time the invitee asked for.
type ConfirmInput struct {
	Link      *model.BookingLink
	Start     time.Time
	LengthMin int // 0 takes the link's first length
	Name      string
	Email     string
	TZ        string // the invitee's zone, if known
	Answers   map[string]string
	PersonID  int64 // 0 matches the contact by email
	FollowUp  bool  // add a follow-up task
	Now       time.Time
	DryRun    bool
}

// ConfirmResult is what a confirmation made.
type ConfirmResult struct {
	DryRun  bool           `json:"dry_run,omitempty"`
	Booking *model.Booking `json:"booking"`
	Event   *model.Event   `json:"event,omitempty"`
	// Person is the matched contact, or empty when the address matched nobody (or several people).
	Person   string `json:"person,omitempty"`
	PersonID int64  `json:"person_id,omitempty"`
	Note     string `json:"note,omitempty"`
	TaskID   int64  `json:"task_id,omitempty"`
	// Invite is the iCalendar invitation (METHOD:REQUEST) to attach to the reply.
	Invite []byte `json:"-"`
}

// Confirm books an offered time: it checks the time is still open, takes it (two confirmations
// cannot both get the same time), creates the event, links the contact, logs a meeting and
// builds the invitation.
func Confirm(ctx context.Context, db *sql.DB, in ConfirmInput) (*ConfirmResult, error) {
	l := in.Link
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !strings.Contains(email, "@") {
		return nil, fmt.Errorf("the invitee needs an email address: %w", model.ErrValidation)
	}
	for _, q := range l.Questions {
		if q.Required && strings.TrimSpace(in.Answers[q.Label]) == "" {
			return nil, fmt.Errorf("question %q needs an answer: %w", q.Label, model.ErrValidation)
		}
	}
	length, err := PickLength(l, in.LengthMin)
	if err != nil {
		return nil, err
	}
	offered, err := Slots(ctx, db, l, length, in.Now)
	if err != nil {
		return nil, err
	}
	if offered.Reason != "" {
		return nil, fmt.Errorf("link %q is closed (%s): %w", l.Slug, offered.Reason, model.ErrConflict)
	}
	if !offered.Offers(in.Start) {
		return nil, fmt.Errorf("%s is not an open time on link %q: %w", tz.FormatInstant(in.Start), l.Slug, model.ErrConflict)
	}
	loc, err := tz.Load(offered.TZ)
	if err != nil {
		return nil, err
	}
	end := in.Start.Add(time.Duration(length) * time.Minute)

	res := &ConfirmResult{DryRun: in.DryRun}
	var personID int64
	switch {
	case in.PersonID != 0:
		personID = in.PersonID
	default:
		ids, err := repo.NewEventRepo(db).MatchPersonByEmail(ctx, email)
		if err != nil {
			return nil, err
		}
		switch len(ids) {
		case 1:
			personID = ids[0]
		case 0:
			res.Note = "no contact has this address, add one and attach it with --person"
		default:
			res.Note = fmt.Sprintf("%d contacts share this address, none attached (use --person)", len(ids))
		}
	}
	if personID != 0 {
		p, err := repo.NewPersonRepo(db).FindByID(ctx, personID)
		if err != nil {
			return nil, err
		}
		res.PersonID = personID
		res.Person = strings.TrimSpace(p.FirstName + " " + derefStr(p.LastName))
	}

	b := model.Booking{
		LinkID: l.ID, InviteeName: strings.TrimSpace(in.Name), Email: email, TZ: in.TZ, Answers: in.Answers,
		Start: tz.FormatInstant(in.Start), End: tz.FormatInstant(end), Status: model.BookingConfirmed,
	}
	if personID != 0 {
		b.PersonID = &personID
	}
	res.Booking = &b
	if in.DryRun {
		return res, nil
	}

	holdStart := tz.FormatInstant(in.Start.Add(-time.Duration(l.BufferBeforeMin) * time.Minute))
	holdEnd := tz.FormatInstant(end.Add(time.Duration(l.BufferAfterMin) * time.Minute))
	bk := repo.NewBookingRepo(db)
	saved, err := bk.InsertBooking(ctx, b, holdStart, holdEnd)
	if err != nil {
		return nil, err
	}
	res.Booking = saved

	name := b.InviteeName
	if name == "" {
		name = email
	}
	title := l.Title + " with " + name
	ei := model.CreateEventInput{
		Title: title, Start: in.Start, End: end, TZ: offered.TZ, Source: "booking",
	}
	if l.Location != "" {
		loc := l.Location
		ei.Location = &loc
	}
	if notes := notesOf(l, in.Answers); notes != "" {
		ei.Notes = &notes
	}
	if personID != 0 {
		ei.Attendees = []model.EventAttendee{{PersonID: personID, Role: "attendee", RSVP: "accepted"}}
	} else {
		ei.Guests = []model.EventGuest{{Email: email, Name: b.InviteeName, Role: "attendee", RSVP: "accepted"}}
	}
	for _, m := range l.RemindersMin {
		ei.Reminders = append(ei.Reminders, model.EventReminder{MinutesBefore: m, Method: "display"})
	}
	ev, err := repo.NewEventRepo(db).Create(ctx, ei)
	if err != nil {
		_ = bk.DropBooking(ctx, saved.ID)
		return nil, err
	}
	if err := bk.AttachEvent(ctx, saved.ID, ev.ID); err != nil {
		return nil, err
	}
	saved.EventID = &ev.ID
	res.Event = ev

	if personID != 0 {
		subject := title
		content := notesOf(l, in.Answers)
		dir := "inbound"
		occurred := in.Start.UTC().Format("2006-01-02 15:04:05")
		if _, err := repo.NewInteractionRepo(db).Create(ctx, model.CreateInteractionInput{
			Type: "meeting", Subject: &subject, Content: &content, Direction: &dir, OccurredAt: &occurred, PersonIDs: []int64{personID},
		}); err != nil {
			return nil, err
		}
	}
	if in.FollowUp {
		after := 24 * 60
		if l.FollowupAfterMin != nil {
			after = *l.FollowupAfterMin
		}
		due := tz.Wall(end.Add(time.Duration(after)*time.Minute), loc).Format(tz.DateLayout)
		ti := model.CreateTaskInput{Title: "Follow up: " + title, DueAt: &due, Priority: "medium"}
		if personID != 0 {
			ti.PersonID = &personID
		}
		t, err := repo.NewTaskRepo(db).Create(ctx, ti)
		if err != nil {
			return nil, err
		}
		res.TaskID = t.ID
	}
	inv, err := Invite(ctx, db, ev.ID, l.HostEmail, "", []ics.Person{{Email: email, Name: b.InviteeName}}, "REQUEST")
	if err != nil {
		return nil, err
	}
	res.Invite = inv
	return res, nil
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// notesOf joins the invitee's answers in question order.
func notesOf(l *model.BookingLink, answers map[string]string) string {
	if len(answers) == 0 {
		return ""
	}
	seen := map[string]bool{}
	var lines []string
	for _, q := range l.Questions {
		if a, ok := answers[q.Label]; ok {
			lines = append(lines, q.Label+": "+a)
			seen[q.Label] = true
		}
	}
	var rest []string
	for k := range answers {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		lines = append(lines, k+": "+answers[k])
	}
	return strings.Join(lines, "\n")
}

// Invite writes an iCalendar invitation for an event. method is REQUEST or CANCEL. The host is the
// organizer when hostEmail is set. Nothing is stored or sent.
func Invite(ctx context.Context, db *sql.DB, eventID int64, hostEmail, hostName string, to []ics.Person, method string) ([]byte, error) {
	opt := icsio.ExportOptions{EventIDs: []int64{eventID}, Name: "crm", Method: method, Invitees: to}
	if hostEmail != "" {
		opt.Organizer = &ics.Person{Email: hostEmail, Name: hostName, PartStat: "ACCEPTED"}
	}
	res, err := icsio.Export(ctx, db, opt)
	if err != nil {
		return nil, err
	}
	if res.Events == 0 {
		return nil, fmt.Errorf("event %d: %w", eventID, model.ErrNotFound)
	}
	return res.Data, nil
}

// Cancel cancels a booking and deletes its event. The invitation to send back is a CANCEL.
func Cancel(ctx context.Context, db *sql.DB, id int64, dryRun bool) (*model.Booking, []byte, error) {
	bk := repo.NewBookingRepo(db)
	b, err := bk.FindBooking(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if b.Status == model.BookingCancelled {
		return nil, nil, fmt.Errorf("booking %d is already cancelled: %w", id, model.ErrConflict)
	}
	var cancel []byte
	if b.EventID != nil {
		l, err := bk.FindLinkByID(ctx, b.LinkID)
		if err != nil {
			return nil, nil, err
		}
		cancel, err = Invite(ctx, db, *b.EventID, l.HostEmail, "", []ics.Person{{Email: b.Email, Name: b.InviteeName}}, "CANCEL")
		if err != nil {
			return nil, nil, err
		}
	}
	if dryRun {
		return b, cancel, nil
	}
	if err := bk.CancelBooking(ctx, id); err != nil {
		return nil, nil, err
	}
	if b.EventID != nil {
		if err := repo.NewEventRepo(db).Archive(ctx, *b.EventID, model.ScopeAll, ""); err != nil {
			return nil, nil, err
		}
	}
	b.Status = model.BookingCancelled
	return b, cancel, nil
}
