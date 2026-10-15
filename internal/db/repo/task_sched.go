package repo

import (
	"fmt"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// checkTaskTime accepts a date or an RFC 3339 instant and returns it in the stored form.
func checkTaskTime(field, v string) (string, error) {
	v = strings.TrimSpace(v)
	if len(v) == 10 {
		if _, err := tz.ParseDate(v); err != nil {
			return "", fmt.Errorf("%s must be a date (2026-11-01) or an instant (2026-11-01T15:00:00Z): %w", field, model.ErrValidation)
		}
		return v, nil
	}
	t, err := tz.ParseInstant(v)
	if err != nil {
		return "", fmt.Errorf("%s must be a date (2026-11-01) or an instant (2026-11-01T15:00:00Z): %w", field, model.ErrValidation)
	}
	return tz.FormatInstant(t), nil
}

// scheduleColumns turns the scheduling fields into column assignments, validating each. The
// returned columns and values are parallel.
func scheduleColumns(s model.TaskSchedule) (cols []string, vals []any, err error) {
	add := func(col string, v any) {
		cols = append(cols, col)
		vals = append(vals, v)
	}
	positive := func(field string, p *int, col string) error {
		if p == nil {
			return nil
		}
		switch {
		case *p < 0:
			return fmt.Errorf("%s cannot be negative: %w", field, model.ErrValidation)
		case *p == 0:
			add(col, nil)
		default:
			add(col, *p)
		}
		return nil
	}
	if err = positive("duration", s.DurationMin, "duration_min"); err != nil {
		return
	}
	if err = positive("minimum chunk", s.MinChunkMin, "min_chunk_min"); err != nil {
		return
	}
	for _, f := range []struct {
		field, col string
		p          *string
	}{{"earliest start", "earliest_start", s.EarliestStart}, {"deadline", "deadline_at", s.DeadlineAt}} {
		if f.p == nil {
			continue
		}
		if strings.TrimSpace(*f.p) == "" {
			add(f.col, nil)
			continue
		}
		v, cerr := checkTaskTime(f.field, *f.p)
		if cerr != nil {
			return nil, nil, cerr
		}
		add(f.col, v)
	}
	if s.DeadlineHard != nil {
		add("deadline_hard", *s.DeadlineHard)
	}
	if s.AutoSchedule != nil {
		add("auto_schedule", *s.AutoSchedule)
	}
	if s.ScheduledStart != nil {
		if strings.TrimSpace(*s.ScheduledStart) == "" {
			add("scheduled_start", nil)
			if s.ScheduledLocked == nil {
				add("scheduled_locked", false)
			}
		} else {
			t, perr := tz.ParseInstant(*s.ScheduledStart)
			if perr != nil {
				return nil, nil, fmt.Errorf("scheduled start must be an instant such as 2026-11-01T15:00:00Z: %w", model.ErrValidation)
			}
			add("scheduled_start", tz.FormatInstant(t))
			if s.ScheduledLocked == nil {
				add("scheduled_locked", true)
			}
		}
	}
	if s.ScheduledLocked != nil {
		add("scheduled_locked", *s.ScheduledLocked)
	}
	if s.ScheduleName != nil {
		name := strings.TrimSpace(*s.ScheduleName)
		if name == "" {
			name = "work"
		}
		add("schedule_name", name)
	}
	return cols, vals, nil
}
