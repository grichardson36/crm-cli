package schedule

import (
	"fmt"
	"sort"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Decisions of the planner.
const (
	Placed = "placed"
	Late   = "late"
	Unfit  = "unfit"
)

// Unfit reasons.
const (
	ReasonNoFreeTime  = "no_free_time_before_horizon"
	ReasonLongerThanG = "longer_than_any_gap"
	ReasonNoHours     = "unknown_working_hours"
)

// PlanTask is a task the planner may place.
type PlanTask struct {
	ID       int64
	UUID     string
	Title    string
	Duration time.Duration
	// MinChunk lets the task split into blocks of at least this length. Zero or >= Duration means
	// one block.
	MinChunk time.Duration
	// Earliest delays the start. Zero means now.
	Earliest time.Time
	// Deadline is the planning deadline; zero means none, and the horizon end stands in for it.
	Deadline time.Time
	// SoftDeadline says the deadline came from the due date.
	SoftDeadline bool
	// Hard lets the planner use time outside working hours to meet the deadline.
	Hard bool
	// Priority is 1 (low), 2 (medium) or 3 (high).
	Priority int
	Created  time.Time
	Schedule string
	// Prev is where the task was placed before, if it was.
	Prev *time.Time
}

// PlanInput is everything the planner reads. Same input, same output.
type PlanInput struct {
	Now, End time.Time
	Tasks    []PlanTask
	Hours    map[string]*model.WorkingHours
	Busy     []Span
	Buffer   time.Duration
	Step     time.Duration
}

// Decision is the planner's answer for one task.
type Decision struct {
	TaskID    int64    `json:"task_id"`
	UUID      string   `json:"uuid"`
	Title     string   `json:"task"`
	Decision  string   `json:"decision"`
	Reason    string   `json:"reason,omitempty"`
	Blocks    []Block  `json:"blocks"`
	ETA       string   `json:"eta,omitempty"`
	Deadline  string   `json:"deadline,omitempty"`
	SlackMin  *int     `json:"slack_min,omitempty"`
	Why       []string `json:"why"`
	MovedFrom string   `json:"moved_from,omitempty"`
	// PrevStart is where the task is placed now, if it is.
	PrevStart string `json:"previous_start,omitempty"`
	Spilled   bool   `json:"spilled_outside_hours,omitempty"`
}

// Block is one placed span of a task.
type Block struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// Summary counts the decisions.
type Summary struct {
	Placed          int      `json:"placed"`
	Late            int      `json:"late"`
	Unfit           int      `json:"unfit"`
	Skipped         int      `json:"skipped_no_duration"`
	OverloadMinutes int      `json:"overload_minutes"`
	Suggestions     []string `json:"suggestions"`
}

// PlanResult is the proposal.
type PlanResult struct {
	Now       string     `json:"now"`
	Horizon   string     `json:"horizon_end"`
	Decisions []Decision `json:"tasks"`
	Summary   Summary    `json:"summary"`
}

func infinite() time.Time { return time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC) }

type ranked struct {
	t        PlanTask
	deadline time.Time // effective
	slack    time.Duration
}

func fmtLen(d time.Duration) string {
	if d%(24*time.Hour) == 0 && d >= 24*time.Hour {
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
	h, m := int(d/time.Hour), int(d%time.Hour/time.Minute)
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%dm", h, m)
}

// Plan places tasks into free time. It is a pure function of its input: tasks are ranked by hard
// deadline, then slack, priority, deadline, creation time and uuid, and each takes the earliest
// free time that fits. Nothing already placed is ever moved to make room.
func Plan(in PlanInput) PlanResult {
	step := in.Step
	if step <= 0 {
		step = 15 * time.Minute
	}
	res := PlanResult{Now: tz.FormatInstant(in.Now), Horizon: tz.FormatInstant(in.End), Decisions: []Decision{}, Summary: Summary{Suggestions: []string{}}}

	var list []ranked
	for _, t := range in.Tasks {
		if t.Duration <= 0 {
			res.Summary.Skipped++
			res.Decisions = append(res.Decisions, Decision{TaskID: t.ID, UUID: t.UUID, Title: t.Title, Decision: "skipped", Reason: "no_duration", Blocks: []Block{}, Why: []string{"no duration, so it cannot be placed"}})
			continue
		}
		r := ranked{t: t, deadline: t.Deadline}
		if r.deadline.IsZero() {
			r.deadline = infinite()
			r.slack = 1<<62 - 1
		} else {
			r.slack = r.deadline.Sub(in.Now) - t.Duration
		}
		list = append(list, r)
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.t.Hard != b.t.Hard {
			return a.t.Hard
		}
		if a.slack != b.slack {
			return a.slack < b.slack
		}
		if a.t.Priority != b.t.Priority {
			return a.t.Priority > b.t.Priority
		}
		if !a.deadline.Equal(b.deadline) {
			return a.deadline.Before(b.deadline)
		}
		if !a.t.Created.Equal(b.t.Created) {
			return a.t.Created.Before(b.t.Created)
		}
		return a.t.UUID < b.t.UUID
	})

	busy := append([]Span(nil), in.Busy...)
	var taken []Span
	skipped := res.Decisions
	res.Decisions = []Decision{}
	for rank, r := range list {
		t := r.t
		d := Decision{TaskID: t.ID, UUID: t.UUID, Title: t.Title, Blocks: []Block{}, Why: []string{}}
		hours := in.Hours[t.Schedule]
		if hours == nil {
			d.Decision, d.Reason = Unfit, ReasonNoHours
			d.Why = append(d.Why, fmt.Sprintf("no working hours named %q", t.Schedule))
			res.add(d, t.Duration)
			continue
		}
		loc, _ := tz.Load(hours.TZ)
		local := func(x time.Time) time.Time { return x.In(loc) }
		if !r.deadline.Equal(infinite()) {
			d.Deadline = tz.FormatInstant(r.deadline)
			mins := int(r.slack / time.Minute)
			d.SlackMin = &mins
		}
		d.Why = append(d.Why, fmt.Sprintf("rank %d of %d: %s", rank+1, len(list), deadlineWhy(r, in.Now)))
		d.Why = append(d.Why, fmt.Sprintf("priority %s, needs %s", map[int]string{1: "low", 2: "medium", 3: "high"}[t.Priority], fmtLen(t.Duration)))

		from := in.Now
		if t.Earliest.After(from) {
			from = t.Earliest
			d.Why = append(d.Why, "not before "+local(from).Format("Mon 2 Jan 15:04"))
		}
		blocksOf := func(avail []Span) ([]Span, string, Span) {
			free := Subtract(avail, Pad(Merge(append(append([]Span(nil), busy...), taken...)), in.Buffer))
			return placeInto(free, t, step, loc)
		}
		avail, err := Intervals(hours, from, in.End)
		if err != nil {
			d.Decision, d.Reason = Unfit, ReasonNoHours
			d.Why = append(d.Why, err.Error())
			res.add(d, t.Duration)
			continue
		}
		blocks, fail, gap := blocksOf(avail)
		if fail == "" && !r.deadline.Equal(infinite()) && blocks[len(blocks)-1].End.After(r.deadline) && t.Hard {
			// Hard deadline: try again with any hour of the day before the deadline.
			if spill, f2, gap2 := blocksOf([]Span{{from, minTime(r.deadline, in.End)}}); f2 == "" && !spill[len(spill)-1].End.After(r.deadline) {
				blocks, gap = spill, gap2
				d.Spilled = true
				d.Why = append(d.Why, "used time outside working hours to meet the hard deadline")
			}
		}
		if fail != "" {
			d.Decision, d.Reason = Unfit, fail
			if fail == ReasonLongerThanG {
				d.Why = append(d.Why, fmt.Sprintf("no gap of %s inside working hours before %s", fmtLen(t.Duration), local(in.End).Format("Mon 2 Jan")))
			} else {
				d.Why = append(d.Why, fmt.Sprintf("not enough free time before %s", local(in.End).Format("Mon 2 Jan")))
			}
			res.add(d, t.Duration)
			continue
		}
		for _, b := range blocks {
			d.Blocks = append(d.Blocks, Block{Start: tz.FormatInstant(b.Start), End: tz.FormatInstant(b.End)})
		}
		taken = append(taken, blocks...)
		last := blocks[len(blocks)-1].End
		d.ETA = tz.FormatInstant(last)
		d.Decision = Placed
		if len(blocks) == 1 {
			d.Why = append(d.Why, fmt.Sprintf("fit in gap %s-%s (%s free)", local(gap.Start).Format("Mon 15:04"), local(gap.End).Format("15:04"), fmtLen(gap.Len())))
		} else {
			d.Why = append(d.Why, fmt.Sprintf("split into %d blocks of at least %s, ends %s", len(blocks), fmtLen(t.MinChunk), local(last).Format("Mon 15:04")))
		}
		if in.Buffer > 0 {
			d.Why = append(d.Why, fmt.Sprintf("%s buffers kept around busy time", fmtLen(in.Buffer)))
		}
		if !r.deadline.Equal(infinite()) && last.After(r.deadline) {
			d.Decision = Late
			d.Why = append(d.Why, fmt.Sprintf("ends %s after the deadline", fmtLen(last.Sub(r.deadline))))
		}
		if t.Prev != nil {
			d.PrevStart = tz.FormatInstant(*t.Prev)
		}
		if t.Prev != nil && !t.Prev.Equal(blocks[0].Start) {
			d.MovedFrom = tz.FormatInstant(*t.Prev)
		}
		res.add(d, t.Duration)
	}
	res.Decisions = append(res.Decisions, skipped...)
	res.suggest()
	return res
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func deadlineWhy(r ranked, now time.Time) string {
	if r.deadline.Equal(infinite()) {
		return "no deadline, placed after the dated work"
	}
	kind := "deadline"
	if r.t.Hard {
		kind = "hard deadline"
	} else if r.t.SoftDeadline {
		kind = "due date"
	}
	if r.deadline.Before(now) {
		return fmt.Sprintf("%s passed %s ago", kind, fmtLen(now.Sub(r.deadline).Truncate(time.Minute)))
	}
	return fmt.Sprintf("%s in %s, slack %dm", kind, fmtLen(r.deadline.Sub(now).Truncate(time.Minute)), int(r.slack/time.Minute))
}

func (r *PlanResult) add(d Decision, dur time.Duration) {
	switch d.Decision {
	case Placed:
		r.Summary.Placed++
	case Late:
		r.Summary.Late++
		r.Summary.OverloadMinutes += int(dur / time.Minute)
	case Unfit:
		r.Summary.Unfit++
		r.Summary.OverloadMinutes += int(dur / time.Minute)
	}
	r.Decisions = append(r.Decisions, d)
}

func (r *PlanResult) suggest() {
	for _, d := range r.Decisions {
		switch d.Decision {
		case Late:
			r.Summary.Suggestions = append(r.Summary.Suggestions, fmt.Sprintf("%q ends after its deadline: extend working hours, shorten it or move the deadline", d.Title))
		case Unfit:
			if d.Reason == ReasonLongerThanG {
				r.Summary.Suggestions = append(r.Summary.Suggestions, fmt.Sprintf("%q needs a longer gap: allow splitting it (--min-chunk) or free a block", d.Title))
			} else if d.Reason == ReasonNoFreeTime {
				r.Summary.Suggestions = append(r.Summary.Suggestions, fmt.Sprintf("%q does not fit the horizon: extend hours, reduce durations or lower other priorities", d.Title))
			}
		}
	}
}

// placeInto finds the blocks for a task in free time. It returns the blocks, a failure reason and
// the gap the first block sits in.
func placeInto(free []Span, t PlanTask, step time.Duration, loc *time.Location) ([]Span, string, Span) {
	if t.MinChunk <= 0 || t.MinChunk >= t.Duration {
		for _, f := range free {
			s := alignUp(f.Start, step, loc)
			if !s.Add(t.Duration).After(f.End) {
				return []Span{{s, s.Add(t.Duration)}}, "", f
			}
		}
		for _, f := range free {
			if f.Len() >= t.Duration {
				return nil, ReasonNoFreeTime, Span{}
			}
		}
		return nil, ReasonLongerThanG, Span{}
	}
	remaining := t.Duration
	var out []Span
	var first Span
	for _, f := range free {
		s := alignUp(f.Start, step, loc)
		room := f.End.Sub(s)
		if room < t.MinChunk {
			continue
		}
		take := room
		if take > remaining {
			take = remaining
		}
		if take < remaining && remaining-take < t.MinChunk {
			take = remaining - t.MinChunk
			if take < t.MinChunk {
				continue
			}
		}
		if len(out) == 0 {
			first = f
		}
		out = append(out, Span{s, s.Add(take)})
		remaining -= take
		if remaining == 0 {
			return out, "", first
		}
	}
	return nil, ReasonNoFreeTime, Span{}
}
