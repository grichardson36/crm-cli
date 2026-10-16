package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// FieldChangedAt returns when a field of a row last changed on this device's history, or ""
// when no change was ever recorded. entity is "person". field is a people column name, or
// "birthday" for the three birthday columns.
func FieldChangedAt(ctx context.Context, q DBTX, entity string, localID int64, field string) (string, error) {
	var at string
	err := q.QueryRowContext(ctx,
		"SELECT changed_at FROM field_versions WHERE entity = ? AND local_id = ? AND field = ?",
		entity, localID, field).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("field changed at: %w", err)
	}
	return at, nil
}

func strVal(p *string) string {
	if p == nil {
		return "\x00"
	}
	return "s:" + *p
}

func intVal(p *int) string {
	if p == nil {
		return "\x00"
	}
	return "i:" + strconv.Itoa(*p)
}

// personFieldValues flattens the scalar fields that field_versions tracks into comparable strings.
func personFieldValues(p *model.Person) map[string]string {
	orgID := "\x00"
	if p.OrgID != nil {
		orgID = "i:" + strconv.FormatInt(*p.OrgID, 10)
	}
	return map[string]string{
		"first_name":                  "s:" + p.FirstName,
		"last_name":                   strVal(p.LastName),
		"email":                       strVal(p.Email),
		"phone":                       strVal(p.Phone),
		"title":                       strVal(p.Title),
		"company":                     strVal(p.Company),
		"location":                    strVal(p.Location),
		"notes":                       strVal(p.Notes),
		"summary":                     strVal(p.Summary),
		"org_id":                      orgID,
		"birthday":                    intVal(p.BirthdayMonth) + "/" + intVal(p.BirthdayDay) + "/" + intVal(p.BirthdayYear),
		"keep_in_touch":               strVal(p.KeepInTouch),
		"keep_in_touch_snoozed_until": strVal(p.KeepInTouchSnoozedUntil),
		"last_contacted_at":           strVal(p.LastContactedAt),
		"is_favorite":                 "b:" + strconv.FormatBool(p.IsFavorite),
		"priority_tier":               intVal(p.PriorityTier),
	}
}

// recordPersonFieldChanges writes a field_versions row for each field that differs between
// before and after. It runs in the caller's transaction.
func recordPersonFieldChanges(ctx context.Context, q DBTX, id int64, before, after *model.Person) error {
	return recordFieldChanges(ctx, q, "person", id, personFieldValues(before), personFieldValues(after))
}

// recordFieldChanges writes a field_versions row for each field whose value differs. It is how
// two-way sync tells which side changed a field last.
func recordFieldChanges(ctx context.Context, q DBTX, entity string, id int64, old, now map[string]string) error {
	for field, v := range now {
		if old[field] == v {
			continue
		}
		_, err := q.ExecContext(ctx,
			`INSERT INTO field_versions (entity, local_id, field, changed_at, device_id)
			 VALUES (?, ?, ?, datetime('now'), (SELECT value FROM meta WHERE key = 'device_id'))
			 ON CONFLICT(entity, local_id, field) DO UPDATE SET
			   changed_at = excluded.changed_at, device_id = excluded.device_id`,
			entity, id, field)
		if err != nil {
			return fmt.Errorf("record field version %s: %w", field, err)
		}
	}
	return nil
}

func deref0(p *string) string {
	if p == nil {
		return "\x00"
	}
	return "s:" + *p
}

// taskFieldValues flattens the fields of a task that sync tracks.
func taskFieldValues(t *model.Task) map[string]string {
	return map[string]string{
		"title": "s:" + t.Title, "description": deref0(t.Description), "due_at": deref0(t.DueAt), "priority": "s:" + t.Priority,
		"recurrence": deref0(t.Recurrence), "completed": "b:" + strconv.FormatBool(t.Completed),
	}
}
