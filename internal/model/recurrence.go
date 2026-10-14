package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseRecurrence parses a recurrence like "7 days", "2 weeks", "1 mon", "3 mons", "1 year",
// or one of daily, weekly, monthly, quarterly, yearly. It returns the step in years, months and days.
func ParseRecurrence(s string) (years, months, days int, err error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "daily":
		return 0, 0, 1, nil
	case "weekly":
		return 0, 0, 7, nil
	case "monthly":
		return 0, 1, 0, nil
	case "quarterly":
		return 0, 3, 0, nil
	case "yearly", "annually":
		return 1, 0, 0, nil
	}
	parts := strings.Fields(s)
	if len(parts) == 2 {
		n, nerr := strconv.Atoi(parts[0])
		if nerr == nil && n > 0 {
			switch strings.TrimSuffix(parts[1], "s") {
			case "day":
				return 0, 0, n, nil
			case "week":
				return 0, 0, 7 * n, nil
			case "mon", "month":
				return 0, n, 0, nil
			case "year":
				return n, 0, 0, nil
			}
		}
	}
	return 0, 0, 0, fmt.Errorf("invalid recurrence %q (use e.g. \"7 days\", \"2 weeks\", \"1 mon\", \"1 year\", or daily/weekly/monthly/yearly): %w", s, ErrValidation)
}

var dueLayouts = []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02"}

// NextDue advances a due date by the recurrence until it is after now.
// The result uses the same layout as due.
func NextDue(due, recurrence string, now time.Time) (string, error) {
	y, m, d, err := ParseRecurrence(recurrence)
	if err != nil {
		return "", err
	}
	for _, layout := range dueLayouts {
		t, perr := time.Parse(layout, due)
		if perr != nil {
			continue
		}
		for i := 0; i < 10000; i++ {
			t = t.AddDate(y, m, d)
			if t.After(now) {
				break
			}
		}
		return t.Format(layout), nil
	}
	return "", fmt.Errorf("cannot parse due date %q: %w", due, ErrValidation)
}
