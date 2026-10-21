package model_test

import (
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestRelativeDaysBoundaries(t *testing.T) {
	today := day("2026-10-20")
	for days, want := range map[int]string{
		0:   "today",
		1:   "in 1d",
		6:   "in 6d",
		7:   "in 1w",
		8:   "in 1w 1d",
		13:  "in 1w 6d",
		14:  "in 2w",
		17:  "in 2w 3d",
		29:  "in 4w 1d",
		30:  "in 4w 2d",
		31:  "in 1mo", // Nov 20
		38:  "in 1mo 1w",
		40:  "in 1mo 1w 2d",
		61:  "in 2mo", // Dec 20
		364: "in 11mo 4w 1d",
		365: "in 1y",
		366: "in 1y 1d",
		-1:  "1d ago",
		-6:  "6d ago",
		-7:  "1w ago",
		-9:  "1w 2d ago",
		-14: "2w ago",
		-29: "4w 1d ago",
		-30: "1mo ago", // Sep 20
		-31: "1mo 1d ago",
	} {
		assert.Equal(t, want, model.RelativeDays(today, days), "%d days", days)
	}
}

func TestRelativeDateMonthsAreCalendarMonths(t *testing.T) {
	for _, tc := range []struct{ today, target, want string }{
		{"2026-01-31", "2026-02-28", "in 1mo"},    // month end clamps
		{"2026-01-31", "2026-03-01", "in 1mo 1d"}, // Feb 28 plus one day
		{"2026-01-31", "2026-03-31", "in 2mo"},
		{"2026-02-28", "2026-03-28", "in 1mo"},
		{"2028-02-28", "2028-02-29", "in 1d"}, // leap day
		{"2028-02-29", "2028-03-29", "in 1mo"},
		{"2027-02-28", "2027-03-01", "in 1d"},    // no leap day
		{"2028-02-29", "2029-02-28", "in 1y"},    // a year from a leap day lands on Feb 28
		{"2026-12-20", "2027-01-01", "in 1w 5d"}, // year boundary
		{"2026-12-31", "2027-01-31", "in 1mo"},
		{"2026-10-20", "2027-12-20", "in 1y 2mo"},
		{"2026-10-20", "2028-01-21", "in 1y 3mo 1d"},
		{"2026-10-20", "2026-10-20", "today"},
		// the past mirrors it
		{"2026-03-31", "2026-02-28", "1mo ago"}, // month end clamps back too
		{"2026-03-31", "2026-02-27", "1mo 1d ago"},
		{"2026-03-28", "2026-02-28", "1mo ago"},
		{"2027-01-01", "2026-12-20", "1w 5d ago"},
		{"2028-03-01", "2028-02-29", "1d ago"},
		{"2027-12-20", "2026-10-20", "1y 2mo ago"},
	} {
		assert.Equal(t, tc.want, model.RelativeDate(day(tc.today), day(tc.target)), "%s -> %s", tc.today, tc.target)
	}
}

func TestRelativeDateShowsAtMostThreeParts(t *testing.T) {
	got := model.RelativeDate(day("2026-10-20"), day("2028-01-28")) // 1y 3mo 1w 1d
	assert.Equal(t, "in 1y 3mo 1w", got)
	got = model.RelativeDate(day("2028-01-28"), day("2026-10-20"))
	assert.Equal(t, "1y 3mo 1w ago", got)
}

func TestRelativeDateIgnoresTimeOfDay(t *testing.T) {
	now := time.Date(2026, 10, 20, 23, 59, 0, 0, time.UTC)
	assert.Equal(t, "today", model.RelativeDate(now, day("2026-10-20")))
	assert.Equal(t, "in 1d", model.RelativeDate(now, day("2026-10-21")))
}
