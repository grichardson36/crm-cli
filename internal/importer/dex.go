// Package importer loads data exported from other CRMs.
package importer

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// DexOptions tunes an import.
type DexOptions struct {
	DryRun   bool
	SkipTags []string // group or tag names to leave out (case-insensitive)
}

// DexReport counts what an import did.
type DexReport struct {
	ContactsImported int `json:"contacts_imported"`
	ContactsSkipped  int `json:"contacts_skipped"`
	Emails           int `json:"emails"`
	Phones           int `json:"phones"`
	Handles          int `json:"handles"`
	Tags             int `json:"tags_applied"`
	CustomFields     int `json:"custom_fields"`
	NotesImported    int `json:"notes_imported"`
	NotesSkipped     int `json:"notes_skipped"`
	NoteLinks        int `json:"note_links"`
	NoteLinksMissing int `json:"note_links_missing"`
}

var (
	uuidRe = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

	handleColumns = []struct{ column, kind string }{
		{"linkedin", "linkedin"}, {"twitter", "twitter"}, {"facebook", "facebook"},
		{"instagram", "instagram"}, {"telegram", "telegram"}, {"tiktok", "tiktok"},
		{"youtube", "youtube"}, {"website", "website"},
	}

	// Dex columns that map to a custom field with the given name.
	fixedCustomColumns = []struct{ column, name string }{
		{"education", "education"}, {"business_card_url", "business_card_url"},
		{"image_url", "image_url"}, {"dex_address", "address"},
		{"dex_related_contacts", "related_contacts"},
	}

	// Every column Dex defines itself. Anything else is a user-created custom field.
	knownContactColumns = columnSet(`full_name first_name last_name description job_title company education
		image_url business_card_url website birthday birthday_year linkedin twitter facebook instagram telegram
		tiktok youtube starred is_archived frequency frequency_text last_seen_at last_reminder_at next_reminder_at
		linkedin_companies linkedin_education linkedin_enhance_date linkedin_last_message_at linkedin_message_link
		whatsapp_message_link whatsapp_last_message_at imessage_message_link imessage_last_message_at
		instagram_message_link instagram_last_message_at gmail_last_interaction_at gmail_last_interaction_provider
		gmail_last_interaction_subject gcal_last_interaction_at gcal_last_interaction_provider
		gcal_last_interaction_title phone_call_interaction_snippet phone_call_last_interaction_at
		web_search_summary created_at updated_at location dex_address dex_email dex_emails dex_phone dex_phones
		dex_groups dex_tags dex_related_contacts id user_id`)
)

func columnSet(names string) map[string]bool {
	set := map[string]bool{}
	for _, c := range strings.Fields(names) {
		set[c] = true
	}
	return set
}

type table struct {
	header map[string]int
	rows   [][]string
}

func (t *table) get(row []string, col string) string {
	i, ok := t.header[col]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func readCSV(path string) (*table, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("%s is empty", path)
	}
	t := &table{header: map[string]int{}, rows: records[1:]}
	for i, h := range records[0] {
		t.header[strings.TrimPrefix(strings.TrimSpace(h), "\ufeff")] = i
	}
	return t, nil
}

// ImportDex loads a Dex contacts export, and optionally a Dex notes export, in one
// transaction. With dryRun the transaction is rolled back after counting.
// Re-running is safe: contacts are matched by their Dex id, notes by "dex:<id>".
func ImportDex(ctx context.Context, db *sql.DB, contactsPath, notesPath string, opts DexOptions) (*DexReport, error) {
	contacts, err := readCSV(contactsPath)
	if err != nil {
		return nil, err
	}
	for _, col := range []string{"id", "first_name"} {
		if _, ok := contacts.header[col]; !ok {
			return nil, fmt.Errorf("%s does not look like a Dex contacts export (missing %q)", contactsPath, col)
		}
	}
	var notes *table
	if notesPath != "" {
		if notes, err = readCSV(notesPath); err != nil {
			return nil, err
		}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	rep := &DexReport{}
	skip := map[string]bool{}
	for _, name := range opts.SkipTags {
		skip[strings.ToLower(strings.TrimSpace(name))] = true
	}
	byDexID := map[string]int64{}

	for _, row := range contacts.rows {
		dexID := contacts.get(row, "id")
		if dexID == "" {
			continue
		}
		var existing int64
		err := tx.QueryRowContext(ctx,
			`SELECT entity_id FROM custom_fields WHERE entity_type='person' AND field_name='dex_id' AND field_value=?`, dexID).Scan(&existing)
		if err == nil {
			byDexID[dexID] = existing
			rep.ContactsSkipped++
			continue
		}
		if err != sql.ErrNoRows {
			return nil, err
		}
		pid, err := insertContact(ctx, tx, contacts, row, rep, skip)
		if err != nil {
			return nil, fmt.Errorf("contact %s: %w", dexID, err)
		}
		byDexID[dexID] = pid
		rep.ContactsImported++
	}

	if notes != nil {
		if err := insertNotes(ctx, tx, notes, byDexID, rep); err != nil {
			return nil, err
		}
	}

	if opts.DryRun {
		return rep, nil
	}
	return rep, tx.Commit()
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// sqliteTime converts "2024-01-22T12:00:00.000Z" to "2024-01-22 12:00:00".
func sqliteTime(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.Replace(s, "T", " ", 1)
	s = strings.TrimSuffix(s, "Z")
	if i := strings.Index(s, "."); i >= 0 {
		s = s[:i]
	}
	return s
}

// cleanCell drops the leading apostrophe Dex adds to values that look like formulas.
func cleanCell(s string) string { return strings.TrimPrefix(strings.TrimSpace(s), "'") }

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(cleanCell(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseBirthday(birthday, yearCol string) (month, day, year any) {
	b := cleanCell(birthday)
	if b == "" {
		return nil, nil, nil
	}
	var m, d int
	if strings.HasPrefix(b, "--") {
		parts := strings.Split(strings.TrimPrefix(b, "--"), "-")
		if len(parts) != 2 {
			return nil, nil, nil
		}
		m, _ = strconv.Atoi(parts[0])
		d, _ = strconv.Atoi(parts[1])
	} else {
		parts := strings.Split(b[:min(len(b), 10)], "-")
		if len(parts) != 3 {
			return nil, nil, nil
		}
		m, _ = strconv.Atoi(parts[1])
		d, _ = strconv.Atoi(parts[2])
	}
	if m < 1 || m > 12 || d < 1 || d > 31 {
		return nil, nil, nil
	}
	// Dex stores a placeholder year when the real one is unknown, so trust only the explicit column.
	if y, err := strconv.Atoi(strings.TrimSpace(yearCol)); err == nil && y > 0 {
		year = y
	}
	return m, d, year
}

func keepInTouch(frequencyText, frequency string) any {
	if t := strings.TrimSpace(frequencyText); t != "" {
		return t
	}
	if strings.Contains(strings.ToLower(frequency), "don't keep in touch") {
		return "never"
	}
	return nil
}

func insertContact(ctx context.Context, tx *sql.Tx, t *table, row []string, rep *DexReport, skip map[string]bool) (int64, error) {
	emails := splitList(t.get(row, "dex_emails"))
	if len(emails) == 0 {
		emails = splitList(t.get(row, "dex_email"))
	}
	phones := splitList(t.get(row, "dex_phones"))
	if len(phones) == 0 {
		phones = splitList(t.get(row, "dex_phone"))
	}

	first, last := t.get(row, "first_name"), t.get(row, "last_name")
	if first == "" {
		if full := t.get(row, "full_name"); full != "" {
			parts := strings.SplitN(full, " ", 2)
			first = parts[0]
			if len(parts) == 2 && last == "" {
				last = parts[1]
			}
		} else if len(emails) > 0 {
			first = emails[0]
		} else {
			first = "(unnamed)"
		}
	}

	var primaryEmail, primaryPhone any
	if len(emails) > 0 {
		primaryEmail = emails[0]
	}
	if len(phones) > 0 {
		primaryPhone = phones[0]
	}
	bm, bd, by := parseBirthday(t.get(row, "birthday"), t.get(row, "birthday_year"))
	fav := 0
	if strings.EqualFold(t.get(row, "starred"), "true") {
		fav = 1
	}
	created := sqliteTime(t.get(row, "created_at"))
	updated := sqliteTime(t.get(row, "updated_at"))
	if created == "" {
		created = "1970-01-01 00:00:00"
	}
	if updated == "" {
		updated = created
	}

	kit := keepInTouch(t.get(row, "frequency_text"), t.get(row, "frequency"))
	res, err := tx.ExecContext(ctx,
		`INSERT INTO people (uuid, first_name, last_name, email, phone, title, company, location, notes,
		   birthday_month, birthday_day, birthday_year, keep_in_touch, keep_in_touch_set_at, last_contacted_at, is_favorite, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CASE WHEN ? IS NULL THEN NULL ELSE datetime('now') END, ?, ?, ?, ?)`,
		uuid.NewString(), first, nullable(last), primaryEmail, primaryPhone,
		nullable(t.get(row, "job_title")), nullable(t.get(row, "company")), nullable(t.get(row, "location")),
		nullable(t.get(row, "description")), bm, bd, by,
		kit, kit,
		nullable(sqliteTime(t.get(row, "last_seen_at"))), fav, created, updated)
	if err != nil {
		return 0, err
	}
	pid, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	for i, e := range emails {
		r, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO person_emails (person_id, email, is_primary) VALUES (?, ?, ?)`, pid, e, b2i(i == 0))
		if err != nil {
			return 0, err
		}
		n, _ := r.RowsAffected()
		rep.Emails += int(n)
	}
	for i, p := range phones {
		r, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO person_phones (person_id, phone_number, is_primary) VALUES (?, ?, ?)`, pid, p, b2i(i == 0))
		if err != nil {
			return 0, err
		}
		n, _ := r.RowsAffected()
		rep.Phones += int(n)
	}
	for _, h := range handleColumns {
		if v := t.get(row, h.column); v != "" {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO person_handles (person_id, kind, value) VALUES (?, ?, ?)`, pid, h.kind, v); err != nil {
				return 0, err
			}
			rep.Handles++
		}
	}

	for _, col := range []string{"dex_groups", "dex_tags"} {
		for _, name := range splitList(t.get(row, col)) {
			if skip[strings.ToLower(name)] {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tags (name) VALUES (?)`, name); err != nil {
				return 0, err
			}
			r, err := tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO taggings (tag_id, entity_type, entity_id) SELECT id, 'person', ? FROM tags WHERE name = ?`, pid, name)
			if err != nil {
				return 0, err
			}
			n, _ := r.RowsAffected()
			rep.Tags += int(n)
		}
	}

	fields := [][2]string{{"dex_id", t.get(row, "id")}}
	for _, c := range fixedCustomColumns {
		if v := t.get(row, c.column); v != "" {
			fields = append(fields, [2]string{c.name, v})
		}
	}
	for name := range t.header {
		if !knownContactColumns[name] {
			if v := t.get(row, name); v != "" {
				fields = append(fields, [2]string{name, v})
			}
		}
	}
	for _, f := range fields {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO custom_fields (entity_type, entity_id, field_name, field_value) VALUES ('person', ?, ?, ?)`, pid, f[0], f[1]); err != nil {
			return 0, err
		}
		rep.CustomFields++
	}
	return pid, nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func insertNotes(ctx context.Context, tx *sql.Tx, t *table, byDexID map[string]int64, rep *DexReport) error {
	for _, row := range t.rows {
		dexID := t.get(row, "id")
		if dexID == "" {
			continue
		}
		key := "dex:" + dexID
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM interactions WHERE uuid = ?`, key).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			rep.NotesSkipped++
			continue
		}

		typ, subject := "note", ""
		switch kind := strings.ToLower(t.get(row, "note_type")); kind {
		case "email", "meeting", "call", "message", "note":
			typ = kind
		case "":
		default:
			subject = t.get(row, "note_type")
		}
		occurred := sqliteTime(t.get(row, "event_time"))
		if occurred == "" {
			occurred = "1970-01-01 00:00:00"
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO interactions (uuid, type, subject, content, occurred_at) VALUES (?, ?, ?, ?, ?)`,
			key, typ, nullable(subject), nullable(t.get(row, "basic_note")), occurred)
		if err != nil {
			return err
		}
		iid, _ := res.LastInsertId()
		rep.NotesImported++

		for _, id := range uuidRe.FindAllString(t.get(row, "contact_ids"), -1) {
			pid, ok := byDexID[id]
			if !ok {
				rep.NoteLinksMissing++
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO interaction_people (interaction_id, person_id) VALUES (?, ?)`, iid, pid); err != nil {
				return err
			}
			rep.NoteLinks++
		}
	}
	return nil
}
