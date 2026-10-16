package recur

import (
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func chicago(t *testing.T) *time.Location {
	t.Helper()
	loc, err := tz.Load("America/Chicago")
	require.NoError(t, err)
	return loc
}

func utc(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, time.UTC)
}

func starts(occs []Occurrence) []string {
	out := make([]string, len(occs))
	for i, o := range occs {
		out[i] = tz.FormatInstant(o.Start)
	}
	return out
}

// localStarts renders instances in a zone as "2006-01-02 15:04".
func localStarts(occs []Occurrence, loc *time.Location) []string {
	out := make([]string, len(occs))
	for i, o := range occs {
		out[i] = o.Start.In(loc).Format("2006-01-02 15:04")
	}
	return out
}

// A weekly 09:00 meeting in America/Chicago keeps 09:00 local across both 2026 daylight saving
// changes (spring forward 2026-03-08, fall back 2026-11-01).
func TestWeeklyKeepsLocalTimeAcrossDST(t *testing.T) {
	loc := chicago(t)
	// First Monday, 2026-03-02 09:00 CST (15:00Z).
	spec := Spec{
		Start: utc(2026, 3, 2, 15, 0), End: utc(2026, 3, 2, 16, 0), TZ: loc,
		RRule: "FREQ=WEEKLY;BYDAY=MO",
	}

	t.Run("spring", func(t *testing.T) {
		occs, truncated, err := Expand(spec, utc(2026, 3, 1, 0, 0), utc(2026, 3, 24, 0, 0))
		require.NoError(t, err)
		assert.False(t, truncated)
		assert.Equal(t, []string{
			"2026-03-02T15:00:00Z", // CST
			"2026-03-09T14:00:00Z", // CDT: the instant moves, the clock does not
			"2026-03-16T14:00:00Z",
			"2026-03-23T14:00:00Z",
		}, starts(occs))
		assert.Equal(t, []string{"2026-03-02 09:00", "2026-03-09 09:00", "2026-03-16 09:00", "2026-03-23 09:00"}, localStarts(occs, loc))
	})

	t.Run("autumn", func(t *testing.T) {
		occs, _, err := Expand(spec, utc(2026, 10, 25, 0, 0), utc(2026, 11, 10, 0, 0))
		require.NoError(t, err)
		assert.Equal(t, []string{
			"2026-10-26T14:00:00Z", // CDT
			"2026-11-02T15:00:00Z", // CST
			"2026-11-09T15:00:00Z",
		}, starts(occs))
		assert.Equal(t, []string{"2026-10-26 09:00", "2026-11-02 09:00", "2026-11-09 09:00"}, localStarts(occs, loc))
	})

	t.Run("every instance of the year reads 09:00", func(t *testing.T) {
		occs, _, err := Expand(spec, utc(2026, 1, 1, 0, 0), utc(2027, 1, 1, 0, 0))
		require.NoError(t, err)
		require.Len(t, occs, 44)
		for _, o := range occs {
			assert.Equal(t, "09:00", o.Start.In(loc).Format("15:04"), o.Start)
			assert.Equal(t, time.Hour, o.End.Sub(o.Start))
		}
	})
}

// RFC 5545 3.3.5: a nonexistent local time uses the offset before the gap, a repeated local time
// is the first occurrence.
func TestNonexistentAndRepeatedLocalTimes(t *testing.T) {
	loc := chicago(t)

	t.Run("nonexistent 02:30 on the spring forward day", func(t *testing.T) {
		// Sundays 02:30 local, anchored 2026-03-01 (CST, 08:30Z).
		spec := Spec{Start: utc(2026, 3, 1, 8, 30), End: utc(2026, 3, 1, 9, 0), TZ: loc, RRule: "FREQ=WEEKLY;BYDAY=SU"}
		occs, _, err := Expand(spec, utc(2026, 3, 1, 0, 0), utc(2026, 3, 16, 0, 0))
		require.NoError(t, err)
		assert.Equal(t, []string{
			"2026-03-01T08:30:00Z",
			"2026-03-08T08:30:00Z", // 02:30 CST, which is 03:30 CDT
			"2026-03-15T07:30:00Z", // 02:30 CDT
		}, starts(occs))
		assert.Equal(t, "03:30", occs[1].Start.In(loc).Format("15:04"))
	})

	t.Run("repeated 01:30 on the fall back day", func(t *testing.T) {
		// Daily 01:30 local, anchored 2026-10-31 (CDT, 06:30Z).
		spec := Spec{Start: utc(2026, 10, 31, 6, 30), End: utc(2026, 10, 31, 7, 0), TZ: loc, RRule: "FREQ=DAILY;COUNT=3"}
		occs, _, err := Expand(spec, utc(2026, 10, 30, 0, 0), utc(2026, 11, 5, 0, 0))
		require.NoError(t, err)
		assert.Equal(t, []string{
			"2026-10-31T06:30:00Z",
			"2026-11-01T06:30:00Z", // first 01:30 (CDT), not 07:30Z (CST)
			"2026-11-02T07:30:00Z", // 01:30 CST
		}, starts(occs))
	})
}

func TestRulesInWallClock(t *testing.T) {
	loc := chicago(t)
	mondayNine := Spec{Start: utc(2026, 1, 5, 15, 0), End: utc(2026, 1, 5, 16, 0), TZ: loc} // Mon 2026-01-05 09:00 CST

	tests := []struct {
		name string
		rule string
		from time.Time
		to   time.Time
		want []string // local, "2006-01-02 15:04"
	}{
		{"first monday of the month", "FREQ=MONTHLY;BYDAY=1MO", utc(2026, 9, 1, 0, 0), utc(2026, 12, 31, 0, 0),
			[]string{"2026-09-07 09:00", "2026-10-05 09:00", "2026-11-02 09:00", "2026-12-07 09:00"}},
		{"last monday of the month", "FREQ=MONTHLY;BYDAY=-1MO", utc(2026, 9, 1, 0, 0), utc(2026, 12, 31, 0, 0),
			[]string{"2026-09-28 09:00", "2026-10-26 09:00", "2026-11-30 09:00", "2026-12-28 09:00"}},
		{"count of three", "FREQ=WEEKLY;COUNT=3", utc(2026, 1, 1, 0, 0), utc(2027, 1, 1, 0, 0),
			[]string{"2026-01-05 09:00", "2026-01-12 09:00", "2026-01-19 09:00"}},
		{"every other week", "FREQ=WEEKLY;INTERVAL=2", utc(2026, 1, 1, 0, 0), utc(2026, 2, 10, 0, 0),
			[]string{"2026-01-05 09:00", "2026-01-19 09:00", "2026-02-02 09:00"}},
		{"weekdays", "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", utc(2026, 1, 5, 0, 0), utc(2026, 1, 13, 0, 0),
			[]string{"2026-01-05 09:00", "2026-01-06 09:00", "2026-01-07 09:00", "2026-01-08 09:00", "2026-01-09 09:00", "2026-01-12 09:00"}},
		{"until in UTC includes the final instant", "FREQ=WEEKLY;UNTIL=20260119T150000Z", utc(2026, 1, 1, 0, 0), utc(2027, 1, 1, 0, 0),
			[]string{"2026-01-05 09:00", "2026-01-12 09:00", "2026-01-19 09:00"}},
		{"until in UTC one second early excludes it", "FREQ=WEEKLY;UNTIL=20260119T145959Z", utc(2026, 1, 1, 0, 0), utc(2027, 1, 1, 0, 0),
			[]string{"2026-01-05 09:00", "2026-01-12 09:00"}},
		{"until as a date is inclusive", "FREQ=WEEKLY;UNTIL=20260119", utc(2026, 1, 1, 0, 0), utc(2027, 1, 1, 0, 0),
			[]string{"2026-01-05 09:00", "2026-01-12 09:00", "2026-01-19 09:00"}},
		{"monthly on the 31st skips short months", "FREQ=MONTHLY;BYMONTHDAY=31", utc(2026, 1, 1, 0, 0), utc(2026, 6, 1, 0, 0),
			[]string{"2026-01-31 09:00", "2026-03-31 09:00", "2026-05-31 09:00"}},
		{"yearly", "FREQ=YEARLY;COUNT=3", utc(2026, 1, 1, 0, 0), utc(2040, 1, 1, 0, 0),
			[]string{"2026-01-05 09:00", "2027-01-05 09:00", "2028-01-05 09:00"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := mondayNine
			spec.RRule = tt.rule
			occs, _, err := Expand(spec, tt.from, tt.to)
			require.NoError(t, err)
			assert.Equal(t, tt.want, localStarts(occs, loc))
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		rule    string
		wantErr string
	}{
		{"FREQ=WEEKLY;BYDAY=MO,WE", ""},
		{"RRULE:FREQ=DAILY;COUNT=5", ""},
		{"freq=monthly;byday=1mo", ""},
		{"FREQ=DAILY;COUNT=5;UNTIL=20261231", "both COUNT and UNTIL"},
		{"FREQ=DAILY;COUNT=5;UNTIL=20261231T000000Z", "both COUNT and UNTIL"},
		{"COUNT=5", "needs FREQ"},
		{"", "empty"},
		{"FREQ=HOURLY", "not supported"},
		{"FREQ=SECONDLY", "not supported"},
		{"FREQ=DAILY;COUNT=0", "positive"},
		{"FREQ=DAILY;INTERVAL=-1", "positive"},
		{"FREQ=DAILY;FREQ=WEEKLY", "twice"},
		{"FREQ=DAILY;BOGUS=1", "invalid recurrence"},
		{"FREQ=DAILY;BYDAY", "bad recurrence part"},
		{"FREQ=MONTHLY;BYMONTHDAY=40", "invalid recurrence"},
	}
	for _, tt := range tests {
		t.Run(tt.rule, func(t *testing.T) {
			err := Validate(tt.rule)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestNormalize(t *testing.T) {
	got, err := Normalize("rrule:freq=weekly;byday=mo,we")
	require.NoError(t, err)
	assert.Equal(t, "FREQ=WEEKLY;BYDAY=MO,WE", got)
}

func TestExpansionCap(t *testing.T) {
	spec := Spec{Start: utc(2026, 1, 1, 9, 0), End: utc(2026, 1, 1, 10, 0), TZ: time.UTC, RRule: "FREQ=DAILY"}
	occs, truncated, err := Expand(spec, utc(2026, 1, 1, 0, 0), utc(2032, 1, 1, 0, 0))
	require.NoError(t, err)
	assert.True(t, truncated)
	assert.Len(t, occs, MaxInstances)

	occs, truncated, err = Expand(spec, utc(2026, 1, 1, 0, 0), utc(2026, 2, 1, 0, 0))
	require.NoError(t, err)
	assert.False(t, truncated)
	assert.Len(t, occs, 31)
}

func TestAllDayIgnoresTimeZone(t *testing.T) {
	// A birthday style all-day event is on the same date everywhere, whatever zone is set.
	for _, zone := range []string{"America/Chicago", "Pacific/Auckland", "UTC"} {
		t.Run(zone, func(t *testing.T) {
			loc, err := tz.Load(zone)
			require.NoError(t, err)
			spec := Spec{
				Start: utc(2026, 3, 8, 0, 0), End: utc(2026, 3, 9, 0, 0), AllDay: true, TZ: loc,
				RRule: "FREQ=YEARLY;COUNT=3",
			}
			occs, _, err := Expand(spec, utc(2026, 1, 1, 0, 0), utc(2030, 1, 1, 0, 0))
			require.NoError(t, err)
			require.Len(t, occs, 3)
			for i, o := range occs {
				assert.True(t, o.AllDay)
				assert.Equal(t, time.Date(2026+i, 3, 8, 0, 0, 0, 0, time.UTC), o.Start)
				assert.Equal(t, 24*time.Hour, o.End.Sub(o.Start))
			}
		})
	}
}

func TestAllDayWeeklyAndUntil(t *testing.T) {
	spec := Spec{Start: utc(2026, 10, 12, 0, 0), End: utc(2026, 10, 13, 0, 0), AllDay: true, RRule: "FREQ=WEEKLY;UNTIL=20261026"}
	occs, _, err := Expand(spec, utc(2026, 10, 1, 0, 0), utc(2026, 12, 1, 0, 0))
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-10-12T00:00:00Z", "2026-10-19T00:00:00Z", "2026-10-26T00:00:00Z"}, starts(occs))
}

func TestExdatesAndRdates(t *testing.T) {
	loc := chicago(t)
	spec := Spec{
		Start: utc(2026, 1, 5, 15, 0), End: utc(2026, 1, 5, 16, 0), TZ: loc, RRule: "FREQ=WEEKLY;COUNT=4",
		Exdates: []time.Time{utc(2026, 1, 12, 15, 0)},
		Rdates:  []time.Time{utc(2026, 1, 14, 18, 0)},
	}
	occs, _, err := Expand(spec, utc(2026, 1, 1, 0, 0), utc(2026, 2, 1, 0, 0))
	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-01-05T15:00:00Z",
		"2026-01-14T18:00:00Z",
		"2026-01-19T15:00:00Z",
		"2026-01-26T15:00:00Z",
	}, starts(occs), "the excluded Monday is skipped, COUNT still counts it, the extra date is added")
}

func TestWindowOverlap(t *testing.T) {
	spec := Spec{Start: utc(2026, 5, 1, 22, 0), End: utc(2026, 5, 2, 2, 0), TZ: time.UTC}
	tests := []struct {
		name     string
		from, to time.Time
		want     int
	}{
		{"window inside the event", utc(2026, 5, 2, 0, 0), utc(2026, 5, 2, 1, 0), 1},
		{"window starts when the event ends", utc(2026, 5, 2, 2, 0), utc(2026, 5, 3, 0, 0), 0},
		{"window ends when the event starts", utc(2026, 5, 1, 0, 0), utc(2026, 5, 1, 22, 0), 0},
		{"window before", utc(2026, 4, 1, 0, 0), utc(2026, 4, 2, 0, 0), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			occs, _, err := Expand(spec, tt.from, tt.to)
			require.NoError(t, err)
			assert.Len(t, occs, tt.want)
		})
	}
}

func TestSingleEventHasOneInstance(t *testing.T) {
	spec := Spec{Start: utc(2026, 5, 1, 22, 0), End: utc(2026, 5, 1, 23, 0)}
	occs, _, err := Expand(spec, utc(2026, 1, 1, 0, 0), utc(2027, 1, 1, 0, 0))
	require.NoError(t, err)
	require.Len(t, occs, 1)
	assert.True(t, occs[0].RecurrenceID.Equal(spec.Start))
}

func TestBadRuleSurfacesInExpand(t *testing.T) {
	spec := Spec{Start: utc(2026, 5, 1, 22, 0), End: utc(2026, 5, 1, 23, 0), RRule: "FREQ=WEEKLY;COUNT=2;UNTIL=20270101"}
	_, _, err := Expand(spec, utc(2026, 1, 1, 0, 0), utc(2027, 1, 1, 0, 0))
	require.Error(t, err)
}

func TestApplyOverrides(t *testing.T) {
	spec := Spec{Start: utc(2026, 1, 5, 15, 0), End: utc(2026, 1, 5, 16, 0), TZ: time.UTC, RRule: "FREQ=WEEKLY;COUNT=4"}
	from, to := utc(2026, 1, 1, 0, 0), utc(2026, 2, 1, 0, 0)
	base, _, err := Expand(spec, from, to)
	require.NoError(t, err)
	require.Len(t, base, 4)

	t.Run("move one instance", func(t *testing.T) {
		got := ApplyOverrides(base, []Override{{
			RecurrenceID: utc(2026, 1, 12, 15, 0), Start: utc(2026, 1, 13, 17, 0), End: utc(2026, 1, 13, 18, 0),
		}}, from, to)
		require.Len(t, got, 4)
		assert.Equal(t, "2026-01-13T17:00:00Z", tz.FormatInstant(got[1].Start))
		assert.Equal(t, "2026-01-12T15:00:00Z", tz.FormatInstant(got[1].RecurrenceID), "keeps its original id")
		assert.Equal(t, 1, got[1].Override)
		assert.Equal(t, 0, got[0].Override)
	})
	t.Run("cancel one instance", func(t *testing.T) {
		got := ApplyOverrides(base, []Override{{RecurrenceID: utc(2026, 1, 19, 15, 0), Cancelled: true}}, from, to)
		assert.Equal(t, []string{"2026-01-05T15:00:00Z", "2026-01-12T15:00:00Z", "2026-01-26T15:00:00Z"}, starts(got))
	})
	t.Run("moved out of the window disappears", func(t *testing.T) {
		got := ApplyOverrides(base, []Override{{
			RecurrenceID: utc(2026, 1, 26, 15, 0), Start: utc(2026, 3, 1, 15, 0), End: utc(2026, 3, 1, 16, 0),
		}}, from, to)
		assert.Len(t, got, 3)
	})
	t.Run("moved into the window from outside shows", func(t *testing.T) {
		narrow, _, err := Expand(spec, utc(2026, 1, 10, 0, 0), utc(2026, 1, 11, 0, 0))
		require.NoError(t, err)
		assert.Empty(t, narrow)
		got := ApplyOverrides(narrow, []Override{{
			RecurrenceID: utc(2026, 1, 26, 15, 0), Start: utc(2026, 1, 10, 9, 0), End: utc(2026, 1, 10, 10, 0),
		}}, utc(2026, 1, 10, 0, 0), utc(2026, 1, 11, 0, 0))
		require.Len(t, got, 1)
		assert.Equal(t, "2026-01-10T09:00:00Z", tz.FormatInstant(got[0].Start))
	})
}

func TestSplit(t *testing.T) {
	loc := chicago(t)
	base := Spec{Start: utc(2026, 1, 5, 15, 0), End: utc(2026, 1, 5, 16, 0), TZ: loc}

	tests := []struct {
		name    string
		rule    string
		at      time.Time
		wantOld string
		wantNew string
		wantErr string
	}{
		{"count is divided", "FREQ=WEEKLY;COUNT=6", utc(2026, 1, 19, 15, 0), "FREQ=WEEKLY;COUNT=2", "FREQ=WEEKLY;COUNT=4", ""},
		{"open ended gets UNTIL in UTC", "FREQ=WEEKLY;BYDAY=MO", utc(2026, 1, 19, 15, 0), "FREQ=WEEKLY;BYDAY=MO;UNTIL=20260119T145959Z", "FREQ=WEEKLY;BYDAY=MO", ""},
		{"until moves to the new series", "FREQ=WEEKLY;UNTIL=20261231", utc(2026, 1, 19, 15, 0), "FREQ=WEEKLY;UNTIL=20260119T145959Z", "FREQ=WEEKLY;UNTIL=20261231", ""},
		{"splitting at the first instance moves the whole series", "FREQ=WEEKLY;COUNT=3", utc(2026, 1, 5, 15, 0), "", "FREQ=WEEKLY;COUNT=3", ""},
		{"nothing left", "FREQ=WEEKLY;COUNT=2", utc(2026, 2, 2, 15, 0), "", "", "no instances left"},
		{"not a series", "", utc(2026, 1, 19, 15, 0), "", "", "does not repeat"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := base
			spec.RRule = tt.rule
			oldRule, newRule, err := Split(spec, tt.at)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOld, oldRule)
			assert.Equal(t, tt.wantNew, newRule)
		})
	}

	t.Run("the two halves do not overlap or lose instances", func(t *testing.T) {
		spec := base
		spec.RRule = "FREQ=WEEKLY;COUNT=6"
		at := utc(2026, 1, 19, 15, 0)
		oldRule, newRule, err := Split(spec, at)
		require.NoError(t, err)

		first := spec
		first.RRule = oldRule
		second := spec
		second.Start, second.End, second.RRule = at, at.Add(time.Hour), newRule

		from, to := utc(2026, 1, 1, 0, 0), utc(2026, 6, 1, 0, 0)
		a, _, err := Expand(first, from, to)
		require.NoError(t, err)
		b, _, err := Expand(second, from, to)
		require.NoError(t, err)
		all, _, err := Expand(spec, from, to)
		require.NoError(t, err)
		assert.Equal(t, starts(all), append(starts(a), starts(b)...))
	})

	t.Run("an UNTIL half ends before the split instance", func(t *testing.T) {
		spec := base
		spec.RRule = "FREQ=WEEKLY;BYDAY=MO"
		at := utc(2026, 1, 19, 15, 0)
		oldRule, _, err := Split(spec, at)
		require.NoError(t, err)
		spec.RRule = oldRule
		occs, _, err := Expand(spec, utc(2026, 1, 1, 0, 0), utc(2026, 3, 1, 0, 0))
		require.NoError(t, err)
		assert.Equal(t, []string{"2026-01-05T15:00:00Z", "2026-01-12T15:00:00Z"}, starts(occs))
	})

	t.Run("all-day UNTIL is a date", func(t *testing.T) {
		spec := Spec{Start: utc(2026, 10, 12, 0, 0), End: utc(2026, 10, 13, 0, 0), AllDay: true, RRule: "FREQ=WEEKLY"}
		oldRule, _, err := Split(spec, utc(2026, 10, 26, 0, 0))
		require.NoError(t, err)
		assert.Equal(t, "FREQ=WEEKLY;UNTIL=20261025", oldRule)
	})
}

func TestICSRule(t *testing.T) {
	loc := chicago(t)
	timed := Spec{Start: utc(2026, 1, 5, 15, 0), End: utc(2026, 1, 5, 16, 0), TZ: loc}
	allDay := Spec{Start: utc(2026, 1, 5, 0, 0), End: utc(2026, 1, 6, 0, 0), AllDay: true}
	tests := []struct {
		name string
		spec Spec
		rule string
		want string
	}{
		{"no until", timed, "FREQ=WEEKLY;BYDAY=MO", "FREQ=WEEKLY;BYDAY=MO"},
		{"a date until becomes the end of that local day in UTC", timed, "FREQ=WEEKLY;UNTIL=20261231", "FREQ=WEEKLY;UNTIL=20270101T055959Z"},
		{"a UTC until stays", timed, "FREQ=WEEKLY;UNTIL=20260119T150000Z", "FREQ=WEEKLY;UNTIL=20260119T150000Z"},
		{"a floating until is read as local", timed, "FREQ=WEEKLY;UNTIL=20260119T090000", "FREQ=WEEKLY;UNTIL=20260119T150000Z"},
		{"all-day until is a date", allDay, "FREQ=WEEKLY;UNTIL=20261231", "FREQ=WEEKLY;UNTIL=20261231"},
		{"all-day UTC until becomes a date", allDay, "FREQ=WEEKLY;UNTIL=20261231T000000Z", "FREQ=WEEKLY;UNTIL=20261231"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := tt.spec
			spec.RRule = tt.rule
			got, err := ICSRule(spec)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
	_, err := ICSRule(Spec{RRule: "nonsense"})
	assert.Error(t, err)
}
