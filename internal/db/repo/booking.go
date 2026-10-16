package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// BookingRepo stores booking links and the meetings booked through them.
type BookingRepo struct {
	db *sql.DB
}

// NewBookingRepo creates a new BookingRepo.
func NewBookingRepo(db *sql.DB) *BookingRepo { return &BookingRepo{db: db} }

const linkColumns = `id, uuid, name, title, slug, description, host_email, durations_json, schedule_name, horizon_days,
	start_after, min_notice_min, slot_step_min, buffer_before_min, buffer_after_min, max_per_day, max_per_week, max_per_month,
	busy_tasks, location_kind, location, questions_json, reminders_json, followup_after_min, max_uses, expires_at, active,
	archived, created_at, updated_at`

func scanLink(row interface{ Scan(...any) error }) (*model.BookingLink, error) {
	var l model.BookingLink
	var dur, qs, rem string
	var startAfter, expires sql.NullString
	var perDay, perWeek, perMonth, follow, maxUses sql.NullInt64
	var busy, active, archived int
	err := row.Scan(&l.ID, &l.UUID, &l.Name, &l.Title, &l.Slug, &l.Description, &l.HostEmail, &dur, &l.Schedule, &l.HorizonDays,
		&startAfter, &l.MinNoticeMin, &l.SlotStepMin, &l.BufferBeforeMin, &l.BufferAfterMin, &perDay, &perWeek, &perMonth,
		&busy, &l.LocationKind, &l.Location, &qs, &rem, &follow, &maxUses, &expires, &active, &archived, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(dur), &l.Durations)
	_ = json.Unmarshal([]byte(qs), &l.Questions)
	_ = json.Unmarshal([]byte(rem), &l.RemindersMin)
	if startAfter.Valid {
		l.StartAfter = &startAfter.String
	}
	if expires.Valid {
		l.ExpiresAt = &expires.String
	}
	l.MaxPerDay, l.MaxPerWeek, l.MaxPerMonth = nullInt(perDay), nullInt(perWeek), nullInt(perMonth)
	l.FollowupAfterMin, l.MaxUses = nullInt(follow), nullInt(maxUses)
	l.BusyTasks, l.Active, l.Archived = busy != 0, active != 0, archived != 0
	return &l, nil
}

func nullInt(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

func jsonText(v any, empty string) string {
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return empty
	}
	return string(b)
}

// CreateLink stores a new booking link.
func (r *BookingRepo) CreateLink(ctx context.Context, in model.CreateBookingLinkInput) (*model.BookingLink, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	notice := 240
	if in.MinNoticeMin != nil {
		notice = *in.MinNoticeMin
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO booking_links (uuid, name, title, slug, description, host_email, durations_json, schedule_name, horizon_days,
			start_after, min_notice_min, slot_step_min, buffer_before_min, buffer_after_min, max_per_day, max_per_week, max_per_month,
			busy_tasks, location_kind, location, questions_json, reminders_json, followup_after_min, max_uses, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), in.Name, in.Title, in.Slug, in.Description, in.HostEmail, jsonText(in.Durations, "[30]"), in.Schedule, in.HorizonDays,
		in.StartAfter, notice, in.SlotStepMin, in.BufferBeforeMin, in.BufferAfterMin, in.MaxPerDay, in.MaxPerWeek, in.MaxPerMonth,
		boolInt(in.BusyTasks), in.LocationKind, in.Location, jsonText(in.Questions, "[]"), jsonText(in.RemindersMin, "[]"),
		in.FollowupAfterMin, in.MaxUses, in.ExpiresAt)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, fmt.Errorf("a booking link with slug %q exists: %w", in.Slug, model.ErrConflict)
		}
		return nil, fmt.Errorf("create booking link: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.FindLinkByID(ctx, id)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// FindLinkByID returns one link, archived or not.
func (r *BookingRepo) FindLinkByID(ctx context.Context, id int64) (*model.BookingLink, error) {
	l, err := scanLink(r.db.QueryRowContext(ctx, "SELECT "+linkColumns+" FROM booking_links WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("booking link %d: %w", id, model.ErrNotFound)
	}
	return l, err
}

// FindLink returns a live link by slug or by numeric id.
func (r *BookingRepo) FindLink(ctx context.Context, ref string) (*model.BookingLink, error) {
	ref = strings.TrimSpace(ref)
	q := "SELECT " + linkColumns + " FROM booking_links WHERE archived = 0 AND slug = ?"
	args := []any{ref}
	if n, err := strconv.ParseInt(ref, 10, 64); err == nil {
		q += " OR (archived = 0 AND id = ?)"
		args = append(args, n)
	}
	l, err := scanLink(r.db.QueryRowContext(ctx, q, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("booking link %q: %w", ref, model.ErrNotFound)
	}
	return l, err
}

// ListLinks returns live links by name.
func (r *BookingRepo) ListLinks(ctx context.Context) ([]*model.BookingLink, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT "+linkColumns+" FROM booking_links WHERE archived = 0 ORDER BY name, id")
	if err != nil {
		return nil, fmt.Errorf("list booking links: %w", err)
	}
	defer rows.Close()
	out := []*model.BookingLink{}
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// SetLinkActive pauses or resumes a link. A paused link offers nothing.
func (r *BookingRepo) SetLinkActive(ctx context.Context, id int64, active bool) error {
	res, err := r.db.ExecContext(ctx, `UPDATE booking_links SET active = ?, updated_at = datetime('now') WHERE id = ? AND archived = 0`, boolInt(active), id)
	if err != nil {
		return fmt.Errorf("update booking link: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("booking link %d: %w", id, model.ErrNotFound)
	}
	return nil
}

// ArchiveLink removes a link from use. Its bookings stay.
func (r *BookingRepo) ArchiveLink(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `UPDATE booking_links SET archived = 1, updated_at = datetime('now') WHERE id = ? AND archived = 0`, id)
	if err != nil {
		return fmt.Errorf("archive booking link: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("booking link %d: %w", id, model.ErrNotFound)
	}
	return nil
}

const bookingColumns = `b.id, b.uuid, b.link_id, l.slug, b.event_id, b.person_id, b.invitee_name, b.invitee_email, b.invitee_tz,
	b.answers_json, b.start_utc, b.end_utc, b.status, b.reminded_json, b.followup_sent, b.created_at, b.updated_at`

func scanBooking(row interface{ Scan(...any) error }) (*model.Booking, error) {
	var b model.Booking
	var ev, pe sql.NullInt64
	var answers, reminded string
	var follow int
	err := row.Scan(&b.ID, &b.UUID, &b.LinkID, &b.Link, &ev, &pe, &b.InviteeName, &b.Email, &b.TZ, &answers, &b.Start, &b.End,
		&b.Status, &reminded, &follow, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if ev.Valid {
		b.EventID = &ev.Int64
	}
	if pe.Valid {
		b.PersonID = &pe.Int64
	}
	_ = json.Unmarshal([]byte(answers), &b.Answers)
	_ = json.Unmarshal([]byte(reminded), &b.Reminded)
	b.FollowupSent = follow != 0
	return &b, nil
}

// BookingFilters narrows ListBookings. Times are UTC instants.
type BookingFilters struct {
	LinkID int64
	From   string // start at or after
	To     string // start before
	Status string // empty means every status that holds time
	All    bool   // include cancelled
}

// ListBookings returns live bookings by start.
func (r *BookingRepo) ListBookings(ctx context.Context, f BookingFilters) ([]*model.Booking, error) {
	q := "SELECT " + bookingColumns + " FROM bookings b JOIN booking_links l ON l.id = b.link_id WHERE b.archived = 0"
	var args []any
	if f.LinkID != 0 {
		q += " AND b.link_id = ?"
		args = append(args, f.LinkID)
	}
	if f.From != "" {
		q += " AND b.start_utc >= ?"
		args = append(args, f.From)
	}
	if f.To != "" {
		q += " AND b.start_utc < ?"
		args = append(args, f.To)
	}
	switch {
	case f.Status != "":
		q += " AND b.status = ?"
		args = append(args, f.Status)
	case !f.All:
		q += " AND b.status IN ('requested', 'confirmed')"
	}
	rows, err := r.db.QueryContext(ctx, q+" ORDER BY b.start_utc, b.id", args...)
	if err != nil {
		return nil, fmt.Errorf("list bookings: %w", err)
	}
	defer rows.Close()
	out := []*model.Booking{}
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// FindBooking returns one booking.
func (r *BookingRepo) FindBooking(ctx context.Context, id int64) (*model.Booking, error) {
	b, err := scanBooking(r.db.QueryRowContext(ctx,
		"SELECT "+bookingColumns+" FROM bookings b JOIN booking_links l ON l.id = b.link_id WHERE b.id = ? AND b.archived = 0", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("booking %d: %w", id, model.ErrNotFound)
	}
	return b, err
}

// Uses counts the bookings that hold time on a link.
func (r *BookingRepo) Uses(ctx context.Context, linkID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM bookings WHERE link_id = ? AND archived = 0 AND status IN ('requested', 'confirmed')`, linkID).Scan(&n)
	return n, err
}

// InsertBooking stores a booking when no other booking holds the time. The check and the insert
// are one statement, so two callers cannot both take the same time. holdStart and holdEnd are the
// booked time grown by the link's buffers: a booking that overlaps them is a conflict.
func (r *BookingRepo) InsertBooking(ctx context.Context, b model.Booking, holdStart, holdEnd string) (*model.Booking, error) {
	answers := jsonText(b.Answers, "{}")
	if b.Status == "" {
		b.Status = model.BookingConfirmed
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO bookings (uuid, link_id, person_id, invitee_name, invitee_email, invitee_tz, answers_json, start_utc, end_utc, status)
		 SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		 WHERE NOT EXISTS (SELECT 1 FROM bookings o WHERE o.archived = 0 AND o.status IN ('requested', 'confirmed')
		                   AND o.start_utc < ? AND o.end_utc > ?)`,
		uuid.NewString(), b.LinkID, b.PersonID, b.InviteeName, strings.ToLower(b.Email), b.TZ, answers, b.Start, b.End, b.Status,
		holdEnd, holdStart)
	if err != nil {
		return nil, fmt.Errorf("insert booking: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, fmt.Errorf("that time is already booked: %w", model.ErrConflict)
	}
	id, _ := res.LastInsertId()
	return r.FindBooking(ctx, id)
}

// AttachEvent links a booking to the event created for it.
func (r *BookingRepo) AttachEvent(ctx context.Context, id, eventID int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE bookings SET event_id = ?, updated_at = datetime('now') WHERE id = ?`, eventID, id)
	return err
}

// CancelBooking marks a booking cancelled. The caller archives its event.
func (r *BookingRepo) CancelBooking(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE bookings SET status = 'cancelled', updated_at = datetime('now') WHERE id = ? AND archived = 0 AND status != 'cancelled'`, id)
	if err != nil {
		return fmt.Errorf("cancel booking: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("booking %d: %w", id, model.ErrNotFound)
	}
	return nil
}

// DropBooking removes a booking that never got its event.
func (r *BookingRepo) DropBooking(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE bookings SET archived = 1, status = 'cancelled', updated_at = datetime('now') WHERE id = ?`, id)
	return err
}

// MarkReminded records the reminder offsets (minutes before the start) already listed.
func (r *BookingRepo) MarkReminded(ctx context.Context, id int64, minutes []int) error {
	_, err := r.db.ExecContext(ctx, `UPDATE bookings SET reminded_json = ?, updated_at = datetime('now') WHERE id = ?`, jsonText(minutes, "[]"), id)
	return err
}

// MarkFollowup records that the follow-up was listed.
func (r *BookingRepo) MarkFollowup(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE bookings SET followup_sent = 1, updated_at = datetime('now') WHERE id = ?`, id)
	return err
}
