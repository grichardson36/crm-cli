package repo

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/phone"
	"github.com/jdanielnd/crm-cli/internal/validate"
)

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// syncPrimary keeps person_emails and person_phones in step with people.email and people.phone.
func (r *PersonRepo) syncPrimary(ctx context.Context, id int64, email, phone *string) error {
	if email != nil && *email != "" {
		if err := r.AddEmail(ctx, id, *email, nil, true); err != nil {
			return err
		}
	}
	if phone != nil && *phone != "" {
		if err := r.AddPhone(ctx, id, *phone, nil, true); err != nil {
			return err
		}
	}
	return nil
}

// LoadDetails fills p.Emails, p.Phones and p.Handles.
func (r *PersonRepo) LoadDetails(ctx context.Context, p *model.Person) error {
	var err error
	if p.Emails, err = r.Emails(ctx, p.ID); err != nil {
		return err
	}
	if p.Phones, err = r.Phones(ctx, p.ID); err != nil {
		return err
	}
	if p.Handles, err = r.Handles(ctx, p.ID); err != nil {
		return err
	}
	p.Links, err = NewLinkRepo(r.db).All(ctx, Ref{Kind: "person", ID: p.ID})
	return err
}

// Emails lists a person's email addresses, primary first.
func (r *PersonRepo) Emails(ctx context.Context, id int64) ([]model.PersonEmail, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT email, label, is_primary FROM person_emails WHERE person_id = ? ORDER BY is_primary DESC, id`, id)
	if err != nil {
		return nil, fmt.Errorf("list emails: %w", err)
	}
	defer rows.Close()
	var out []model.PersonEmail
	for rows.Next() {
		var e model.PersonEmail
		if err := rows.Scan(&e.Email, &e.Label, &e.IsPrimary); err != nil {
			return nil, fmt.Errorf("scan email: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Phones lists a person's phone numbers, primary first.
func (r *PersonRepo) Phones(ctx context.Context, id int64) ([]model.PersonPhone, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT phone_number, label, country_code, is_primary, e164 FROM person_phones WHERE person_id = ? ORDER BY is_primary DESC, id`, id)
	if err != nil {
		return nil, fmt.Errorf("list phones: %w", err)
	}
	defer rows.Close()
	var out []model.PersonPhone
	for rows.Next() {
		var p model.PersonPhone
		var e164 sql.NullString
		if err := rows.Scan(&p.PhoneNumber, &p.Label, &p.CountryCode, &p.IsPrimary, &e164); err != nil {
			return nil, fmt.Errorf("scan phone: %w", err)
		}
		p.E164 = e164.String
		p.Display = phone.Display(p.PhoneNumber, p.E164)
		out = append(out, p)
	}
	return out, rows.Err()
}

// Handles lists a person's social profiles and websites.
func (r *PersonRepo) Handles(ctx context.Context, id int64) ([]model.PersonHandle, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT kind, value FROM person_handles WHERE person_id = ? ORDER BY kind`, id)
	if err != nil {
		return nil, fmt.Errorf("list handles: %w", err)
	}
	defer rows.Close()
	var out []model.PersonHandle
	for rows.Next() {
		var h model.PersonHandle
		if err := rows.Scan(&h.Kind, &h.Value); err != nil {
			return nil, fmt.Errorf("scan handle: %w", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (r *PersonRepo) mustExist(ctx context.Context, id int64) error {
	_, err := r.FindByID(ctx, id)
	return err
}

// AddEmail adds an email address. The first address, or one added with primary, becomes people.email.
func (r *PersonRepo) AddEmail(ctx context.Context, id int64, email string, label *string, primary bool) error {
	spec, _ := model.FieldNamed("person", "email")
	fr := validate.Field(spec, email, validate.Options{Kind: "person"})
	if err := fr.Err(); err != nil {
		return err
	}
	email = fr.Value
	if email == "" {
		return validate.Fail(validate.Issue{Code: validate.CodeFieldRequired, Field: spec.Label, Expected: "a value is required", Fix: "give an email address"})
	}
	if err := r.mustExist(ctx, id); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO person_emails (person_id, email, label) VALUES (?, ?, ?)
		 ON CONFLICT(person_id, email) DO UPDATE SET label = COALESCE(excluded.label, label)`, id, email, label); err != nil {
		return fmt.Errorf("add email: %w", err)
	}
	if err := touchPerson(ctx, tx, id); err != nil {
		return err
	}
	var hasPrimary int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM person_emails WHERE person_id = ? AND is_primary = 1`, id).Scan(&hasPrimary); err != nil {
		return err
	}
	if primary || hasPrimary == 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE person_emails SET is_primary = (email = ?) WHERE person_id = ?`, email, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE people SET email = ?, updated_at = datetime('now') WHERE id = ?`, email, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RemoveEmail removes an email address and promotes another if it was the primary.
func (r *PersonRepo) RemoveEmail(ctx context.Context, id int64, email string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `DELETE FROM person_emails WHERE person_id = ? AND email = ?`, id, email)
	if err != nil {
		return fmt.Errorf("remove email: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("email %q on person %d: %w", email, id, model.ErrNotFound)
	}
	var next sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT email FROM person_emails WHERE person_id = ? ORDER BY is_primary DESC, id LIMIT 1`, id).Scan(&next)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE person_emails SET is_primary = (email = ?) WHERE person_id = ?`, next, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE people SET email = ?, updated_at = datetime('now') WHERE id = ?`, next, id); err != nil {
		return err
	}
	return tx.Commit()
}

// AddPhone adds a phone number. The first number, or one added with primary, becomes people.phone.
func (r *PersonRepo) AddPhone(ctx context.Context, id int64, number string, label *string, primary bool) error {
	spec, _ := model.FieldNamed("person", "phone")
	fr := validate.Field(spec, number, validate.Options{Kind: "person"})
	if err := fr.Err(); err != nil {
		return err
	}
	phoneText := fr.Value
	if phoneText == "" {
		return validate.Fail(validate.Issue{Code: validate.CodeFieldRequired, Field: spec.Label, Expected: "a value is required", Fix: "give a phone number"})
	}
	if err := r.mustExist(ctx, id); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// The typed text is stored as typed and the E.164 form next to it. A number the person already has
	// in another format is the same number: its row keeps the text it was typed with.
	num := phone.Parse(phoneText)
	stored := phoneText
	if key := num.Key(); key != "" {
		have, err := samePhone(ctx, tx, id, key)
		if err != nil {
			return err
		}
		if have != "" {
			stored = have
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO person_phones (person_id, phone_number, label, e164) VALUES (?, ?, ?, ?)
		 ON CONFLICT(person_id, phone_number) DO UPDATE SET label = COALESCE(excluded.label, label), e164 = excluded.e164`,
		id, stored, label, nullIfEmpty(num.E164)); err != nil {
		return fmt.Errorf("add phone: %w", err)
	}
	if err := touchPerson(ctx, tx, id); err != nil {
		return err
	}
	var hasPrimary int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM person_phones WHERE person_id = ? AND is_primary = 1`, id).Scan(&hasPrimary); err != nil {
		return err
	}
	if primary || hasPrimary == 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE person_phones SET is_primary = (phone_number = ?) WHERE person_id = ?`, stored, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE people SET phone = ?, updated_at = datetime('now') WHERE id = ?`, stored, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// samePhone returns the stored text of the person's number with the given matching key, "" if none.
func samePhone(ctx context.Context, q DBTX, id int64, key string) (string, error) {
	rows, err := q.QueryContext(ctx, `SELECT phone_number FROM person_phones WHERE person_id = ? ORDER BY id`, id)
	if err != nil {
		return "", fmt.Errorf("list phones: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var have string
		if err := rows.Scan(&have); err != nil {
			return "", fmt.Errorf("scan phone: %w", err)
		}
		if phone.Key(have) == key {
			return have, nil
		}
	}
	return "", rows.Err()
}

// RemovePhone removes a phone number and promotes another if it was the primary.
func (r *PersonRepo) RemovePhone(ctx context.Context, id int64, number string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `DELETE FROM person_phones WHERE person_id = ? AND phone_number = ?`, id, number)
	if err != nil {
		return fmt.Errorf("remove phone: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// The same number in another format counts as the same number.
		if key := phone.Key(number); key != "" {
			match, err := samePhone(ctx, tx, id, key)
			if err != nil {
				return err
			}
			if match != "" {
				res, err = tx.ExecContext(ctx, `DELETE FROM person_phones WHERE person_id = ? AND phone_number = ?`, id, match)
				if err != nil {
					return fmt.Errorf("remove phone: %w", err)
				}
				n, _ = res.RowsAffected()
			}
		}
	}
	if n == 0 {
		return fmt.Errorf("phone %q on person %d: %w", number, id, model.ErrNotFound)
	}
	var next sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT phone_number FROM person_phones WHERE person_id = ? ORDER BY is_primary DESC, id LIMIT 1`, id).Scan(&next)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE person_phones SET is_primary = (phone_number = ?) WHERE person_id = ?`, next, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE people SET phone = ?, updated_at = datetime('now') WHERE id = ?`, next, id); err != nil {
		return err
	}
	return tx.Commit()
}

// SetHandle sets (or replaces) a person's handle of the given kind.
func (r *PersonRepo) SetHandle(ctx context.Context, id int64, kind, value string) error {
	if !model.ValidHandleKind(kind) {
		return fmt.Errorf("invalid handle kind %q (must be one of: %s): %w", kind, strings.Join(model.HandleKinds, ", "), model.ErrValidation)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("handle value is empty: %w", model.ErrValidation)
	}
	if err := r.mustExist(ctx, id); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO person_handles (person_id, kind, value) VALUES (?, ?, ?)
		 ON CONFLICT(person_id, kind) DO UPDATE SET value = excluded.value`, id, kind, value)
	if err != nil {
		return fmt.Errorf("set handle: %w", err)
	}
	return touchPerson(ctx, r.db, id)
}

// touchPerson bumps people.updated_at after a change to a row owned by the person.
func touchPerson(ctx context.Context, q interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, id int64) error {
	if _, err := q.ExecContext(ctx, `UPDATE people SET updated_at = datetime('now') WHERE id = ?`, id); err != nil {
		return fmt.Errorf("touch person %d: %w", id, err)
	}
	return nil
}

// RemoveHandle removes a person's handle of the given kind.
func (r *PersonRepo) RemoveHandle(ctx context.Context, id int64, kind string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM person_handles WHERE person_id = ? AND kind = ?`, id, kind)
	if err != nil {
		return fmt.Errorf("remove handle: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("%s handle on person %d: %w", kind, id, model.ErrNotFound)
	}
	return touchPerson(ctx, r.db, id)
}

func displayName(first string, last sql.NullString) string {
	if last.Valid && last.String != "" {
		return first + " " + last.String
	}
	return first
}

// Due lists people whose keep-in-touch date falls within the next withinDays days (0 = already due).
// People snoozed past now are left out. The most overdue come first.
func (r *PersonRepo) Due(ctx context.Context, withinDays, limit int) ([]model.DueContact, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT k.person_id, k.first_name, k.last_name, k.keep_in_touch, k.last_seen, k.due_at,
		        CAST(julianday('now') - julianday(k.due_at) AS INTEGER)
		 FROM keep_in_touch_status k
		 WHERE k.due_at <= datetime('now', '+' || ? || ' days')
		   AND (k.keep_in_touch_snoozed_until IS NULL OR k.keep_in_touch_snoozed_until <= datetime('now'))
		 ORDER BY k.due_at ASC, k.person_id ASC LIMIT ?`, withinDays, limit)
	if err != nil {
		return nil, fmt.Errorf("list due contacts: %w", err)
	}
	defer rows.Close()
	var out []model.DueContact
	for rows.Next() {
		var d model.DueContact
		var first string
		var last sql.NullString
		if err := rows.Scan(&d.PersonID, &first, &last, &d.KeepInTouch, &d.LastSeen, &d.DueAt, &d.DaysOverdue); err != nil {
			return nil, fmt.Errorf("scan due contact: %w", err)
		}
		d.Name = displayName(first, last)
		out = append(out, d)
	}
	return out, rows.Err()
}

// Birthdays lists birthdays within the next days days, soonest first. A Feb 29 birthday
// falls on Feb 28 in non-leap years.
func (r *PersonRepo) Birthdays(ctx context.Context, days int, now time.Time) ([]model.UpcomingBirthday, error) {
	return r.BirthdaysWindow(ctx, 0, days, now)
}

// BirthdaysWindow lists birthdays from daysBack days before today to daysAhead days after, oldest
// first. A person shows once for each of their birthdays inside the window: the most recent one
// before today (negative DaysUntil) and the next one from today on.
func (r *PersonRepo) BirthdaysWindow(ctx context.Context, daysBack, daysAhead int, now time.Time) ([]model.UpcomingBirthday, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, first_name, last_name, birthday_month, birthday_day, birthday_year
		 FROM people WHERE archived = 0 AND birthday_month IS NOT NULL AND birthday_day IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("list birthdays: %w", err)
	}
	defer rows.Close()

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	var out []model.UpcomingBirthday
	for rows.Next() {
		var id int64
		var first string
		var last sql.NullString
		var month, day int
		var year sql.NullInt64
		if err := rows.Scan(&id, &first, &last, &month, &day, &year); err != nil {
			return nil, fmt.Errorf("scan birthday: %w", err)
		}
		add := func(when time.Time) {
			until := int(when.Sub(today).Hours() / 24)
			if until > daysAhead || until < -daysBack {
				return
			}
			b := model.UpcomingBirthday{PersonID: id, Name: displayName(first, last), Date: when.Format("2006-01-02"), DaysUntil: until,
				Relative: model.RelativeDate(today, when)}
			if year.Valid {
				age := when.Year() - int(year.Int64)
				b.Turning = &age
			}
			out = append(out, b)
		}
		if daysBack > 0 {
			add(previousOccurrence(today, month, day))
		}
		add(nextOccurrence(today, month, day))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DaysUntil != out[j].DaysUntil {
			return out[i].DaysUntil < out[j].DaysUntil
		}
		return out[i].PersonID < out[j].PersonID
	})
	return out, nil
}

// previousOccurrence is the latest birthday strictly before today.
func previousOccurrence(today time.Time, month, day int) time.Time {
	for year := today.Year(); ; year-- {
		d := day
		if last := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day(); d > last {
			d = last
		}
		t := time.Date(year, time.Month(month), d, 0, 0, 0, 0, time.UTC)
		if t.Before(today) {
			return t
		}
	}
}

func nextOccurrence(today time.Time, month, day int) time.Time {
	for year := today.Year(); ; year++ {
		d := day
		if last := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day(); d > last {
			d = last
		}
		t := time.Date(year, time.Month(month), d, 0, 0, 0, 0, time.UTC)
		if !t.Before(today) {
			return t
		}
	}
}

// ListSummaries returns every active person as a lightweight row, ordered by last then first name.
func (r *PersonRepo) ListSummaries(ctx context.Context) ([]model.PersonSummary, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT p.id, p.first_name, p.last_name, p.company, p.title, p.email, p.location, p.is_favorite, p.keep_in_touch, p.updated_at, COALESCE(p.priority_tier, 0),
		        (SELECT group_concat(t.name, ', ') FROM taggings tg JOIN tags t ON t.id = tg.tag_id
		          WHERE tg.entity_type = 'person' AND tg.entity_id = p.id AND tg.deleted_at IS NULL),
		        (SELECT group_concat(e.email, ' ') FROM person_emails e WHERE e.person_id = p.id)
		 FROM people p WHERE p.archived = 0
		 ORDER BY lower(COALESCE(p.last_name, p.first_name)), lower(p.first_name)`)
	if err != nil {
		return nil, fmt.Errorf("list people: %w", err)
	}
	defer rows.Close()
	var out []model.PersonSummary
	for rows.Next() {
		var s model.PersonSummary
		var first string
		var last, company, title, email, location, cadence, tags, emails sql.NullString
		if err := rows.Scan(&s.ID, &first, &last, &company, &title, &email, &location, &s.IsFavorite, &cadence, &s.UpdatedAt, &s.PriorityTier, &tags, &emails); err != nil {
			return nil, fmt.Errorf("scan person summary: %w", err)
		}
		s.Name = displayName(first, last)
		s.Company, s.Title, s.Email, s.Location = company.String, title.String, email.String, location.String
		s.KeepInTouch, s.Tags, s.AllEmails = cadence.String, tags.String, emails.String
		out = append(out, s)
	}
	return out, rows.Err()
}
