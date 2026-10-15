package tz

import "time"

// Transition is one change of UTC offset in a zone.
type Transition struct {
	At         time.Time // the instant the new offset starts
	From, To   int       // UTC offsets in seconds before and after
	Name       string    // abbreviation after the change, such as CDT
	Daylight   bool      // the new offset is daylight saving time
	WallBefore time.Time // the change as a floating wall time on the old offset
}

// Initial is the offset in effect at an instant.
type Initial struct {
	Offset   int
	Name     string
	Daylight bool
}

// StateAt returns the offset in effect at an instant.
func StateAt(loc *time.Location, at time.Time) Initial {
	l := at.In(loc)
	name, off := l.Zone()
	return Initial{Offset: off, Name: name, Daylight: l.IsDST()}
}

func offsetAt(loc *time.Location, t time.Time) int {
	_, off := t.In(loc).Zone()
	return off
}

// Transitions lists the offset changes of loc after from and up to to, in order. It is what a
// VTIMEZONE needs: iCalendar consumers read explicit onsets instead of the tz database.
//
// It finds changes by sampling the offset once a day and then bisecting to the second, because
// Time.ZoneBounds is not reliable for years that follow the tz database's final rule.
func Transitions(loc *time.Location, from, to time.Time) []Transition {
	var out []Transition
	prev := offsetAt(loc, from)
	t := from
	for t.Before(to) {
		next := t.Add(24 * time.Hour)
		if next.After(to) {
			next = to
		}
		if off := offsetAt(loc, next); off != prev {
			lo, hi := t, next
			for hi.Sub(lo) > time.Second {
				mid := lo.Add(hi.Sub(lo) / 2)
				if offsetAt(loc, mid) == prev {
					lo = mid
				} else {
					hi = mid
				}
			}
			at := hi.UTC().Truncate(time.Second)
			l := at.In(loc)
			name, newOff := l.Zone()
			out = append(out, Transition{
				At: at, From: prev, To: newOff, Name: name, Daylight: l.IsDST(),
				WallBefore: at.Add(time.Duration(prev) * time.Second),
			})
			prev = newOff
		}
		t = next
	}
	return out
}
