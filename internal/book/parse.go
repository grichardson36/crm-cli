package book

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/cal/when"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Lengths reads "30m,45m,1h" as minutes.
func Lengths(text string) ([]int, error) {
	var out []int
	for _, part := range strings.Split(text, ",") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		d, err := when.ParseLength(part)
		if err != nil {
			return nil, err
		}
		out = append(out, int(d/time.Minute))
	}
	return out, nil
}

// Reminders reads "1d", "1h" as minutes before the start.
func Reminders(list []string) ([]int, error) {
	var out []int
	for _, s := range list {
		m, err := when.ParseReminder(s)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// Questions reads "Agenda" (optional) and "Agenda!" (required).
func Questions(list []string) []model.BookingQuestion {
	var out []model.BookingQuestion
	for _, s := range list {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		q := model.BookingQuestion{Label: strings.TrimSuffix(s, "!")}
		q.Required = strings.HasSuffix(s, "!")
		out = append(out, q)
	}
	return out
}

// Answers reads "Agenda=pricing" pairs.
func Answers(list []string) (map[string]string, error) {
	if len(list) == 0 {
		return nil, nil
	}
	out := map[string]string{}
	for _, s := range list {
		k, v, ok := strings.Cut(s, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("answer %q must look like Question=answer: %w", s, model.ErrValidation)
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out, nil
}

// HostZone is the zone of the link's working hours: the zone slots are shown and typed in.
func HostZone(ctx context.Context, db *sql.DB, l *model.BookingLink) (*time.Location, error) {
	h, err := repo.NewAvailabilityRepo(db).Get(ctx, l.Schedule)
	if err != nil {
		return nil, err
	}
	loc, err := tz.Load(h.TZ)
	if err != nil {
		return nil, fmt.Errorf("%v: %w", err, model.ErrValidation)
	}
	return loc, nil
}

// SplitToken reads the slot line the mail link puts in the reply: slug/2026-10-12T15:00:00Z/30.
func SplitToken(s string) (slug string, start time.Time, lengthMin int, ok bool) {
	parts := strings.Split(strings.TrimSpace(s), "/")
	if len(parts) != 3 {
		return "", time.Time{}, 0, false
	}
	t, err := tz.ParseInstant(parts[1])
	if err != nil {
		return "", time.Time{}, 0, false
	}
	n, err := strconv.Atoi(parts[2])
	if err != nil || n <= 0 {
		return "", time.Time{}, 0, false
	}
	return parts[0], t, n, true
}

// ResolveStart reads a time typed by the host: an instant, or a phrase such as "tue 10:00" in the
// zone of the link.
func ResolveStart(ctx context.Context, db *sql.DB, l *model.BookingLink, text string, now time.Time) (time.Time, error) {
	loc, err := HostZone(ctx, db, l)
	if err != nil {
		return time.Time{}, err
	}
	r, err := when.Parse(text, now, loc)
	if err != nil {
		return time.Time{}, err
	}
	if !r.HasTime {
		return time.Time{}, fmt.Errorf("%q needs a time of day: %w", text, model.ErrValidation)
	}
	return r.Start, nil
}
