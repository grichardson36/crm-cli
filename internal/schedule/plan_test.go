package schedule

import (
	"math/rand"
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// planNow is Monday 2026-10-12 08:00 in Chicago. Hours are Mon-Fri 09:00-17:00.
var planNow = utc(2026, 10, 12, 13, 0)

func baseInput(tasks ...PlanTask) PlanInput {
	return PlanInput{
		Now: planNow, End: planNow.AddDate(0, 0, 14), Tasks: tasks,
		Hours:  map[string]*model.WorkingHours{"work": work("America/Chicago")},
		Buffer: 10 * time.Minute, Step: 15 * time.Minute,
	}
}

func ptask(id int64, title string, dur time.Duration) PlanTask {
	return PlanTask{ID: id, UUID: title, Title: title, Duration: dur, Priority: 2, Schedule: "work", Created: utc(2026, 10, 1, 0, 0).Add(time.Duration(id) * time.Minute)}
}

func local(t *testing.T, s string) string {
	t.Helper()
	v, err := tz.ParseInstant(s)
	require.NoError(t, err)
	return v.In(chicagoLoc(t)).Format("Mon 15:04")
}

func chicagoLoc(t *testing.T) *time.Location {
	t.Helper()
	l, err := tz.Load("America/Chicago")
	require.NoError(t, err)
	return l
}

func byTitle(res PlanResult) map[string]Decision {
	m := map[string]Decision{}
	for _, d := range res.Decisions {
		m[d.Title] = d
	}
	return m
}

func span(t *testing.T, d Decision) string {
	t.Helper()
	require.NotEmpty(t, d.Blocks, d.Title)
	var out string
	for i, b := range d.Blocks {
		if i > 0 {
			out += ", "
		}
		out += local(t, b.Start) + "-" + local(t, b.End)[4:]
	}
	return out
}

func TestPlanDeadlineOrderThenPriorityThenCreated(t *testing.T) {
	late := ptask(1, "later deadline", time.Hour)
	late.Deadline = utc(2026, 10, 16, 22, 0)
	soon := ptask(2, "soon deadline", time.Hour)
	soon.Deadline = utc(2026, 10, 12, 22, 0)
	none := ptask(3, "no deadline", time.Hour)
	high := ptask(4, "undated high", time.Hour)
	high.Priority = 3
	older := ptask(5, "undated older", time.Hour)
	older.Created = utc(2026, 9, 1, 0, 0)

	res := Plan(baseInput(late, soon, none, high, older))
	d := byTitle(res)
	assert.Equal(t, "Mon 09:00-10:00", span(t, d["soon deadline"]), "the tightest slack goes first")
	order := []string{"soon deadline", "later deadline", "undated high", "undated older", "no deadline"}
	var got []string
	for _, x := range res.Decisions {
		got = append(got, x.Title)
	}
	assert.Equal(t, order, got, "decisions come back in planning order")
	// Buffers of 10 minutes between tasks, steps of 15 minutes: 09:00-10:00, then 10:15.
	assert.Equal(t, "Mon 10:15-11:15", span(t, d["later deadline"]))
	assert.Equal(t, "Mon 11:30-12:30", span(t, d["undated high"]))
	for _, x := range res.Decisions {
		assert.Equal(t, Placed, x.Decision, x.Title)
	}
	assert.Equal(t, 5, res.Summary.Placed)
}

func TestPlanHardDeadlineBeatsSlack(t *testing.T) {
	loose := ptask(1, "loose", time.Hour)
	loose.Deadline = utc(2026, 10, 12, 20, 0) // tight slack
	hard := ptask(2, "hard", time.Hour)
	hard.Deadline = utc(2026, 10, 16, 22, 0)
	hard.Hard = true
	d := byTitle(Plan(baseInput(loose, hard)))
	assert.Equal(t, "Mon 09:00-10:00", span(t, d["hard"]))
	assert.Equal(t, "Mon 10:15-11:15", span(t, d["loose"]))
}

func TestPlanRespectsBusyEarliestAndHours(t *testing.T) {
	in := baseInput()
	in.Busy = []Span{{utc(2026, 10, 12, 14, 0), utc(2026, 10, 12, 16, 0)}} // 09:00-11:00 CDT
	a := ptask(1, "a", 90*time.Minute)
	b := ptask(2, "late start", time.Hour)
	b.Earliest = utc(2026, 10, 13, 18, 0) // Tue 13:00
	c := ptask(3, "long", 9*time.Hour)    // longer than any working day
	in.Tasks = []PlanTask{a, b, c}
	res := Plan(in)
	d := byTitle(res)
	assert.Equal(t, "Mon 11:15-12:45", span(t, d["a"]), "after the meeting and its buffer")
	assert.Equal(t, "Tue 13:00-14:00", span(t, d["late start"]))
	assert.Equal(t, Unfit, d["long"].Decision)
	assert.Equal(t, ReasonLongerThanG, d["long"].Reason)
	assert.Empty(t, d["long"].Blocks)
	assert.Equal(t, 540, res.Summary.OverloadMinutes)
	assert.Equal(t, 1, res.Summary.Unfit)
	assert.NotEmpty(t, res.Summary.Suggestions)
}

func TestPlanSplitsIntoChunks(t *testing.T) {
	in := baseInput()
	in.Busy = []Span{{utc(2026, 10, 12, 15, 0), utc(2026, 10, 12, 20, 0)}} // 10:00-15:00 CDT
	big := ptask(1, "report", 5*time.Hour)
	big.MinChunk = time.Hour
	in.Tasks = []PlanTask{big}
	d := byTitle(Plan(in))["report"]
	assert.Equal(t, Placed, d.Decision)
	// Free: 09:00-09:50 (50m, too short for a 1h chunk), 15:10-17:00 (110m), then Tuesday, where the rest fits in one block.
	require.Len(t, d.Blocks, 2)
	assert.Equal(t, "Mon 15:15-17:00", local(t, d.Blocks[0].Start)+"-"+local(t, d.Blocks[0].End)[4:])
	var total time.Duration
	for _, b := range d.Blocks {
		s, _ := tz.ParseInstant(b.Start)
		e, _ := tz.ParseInstant(b.End)
		assert.GreaterOrEqual(t, e.Sub(s), time.Hour, "no chunk below the minimum")
		total += e.Sub(s)
	}
	assert.Equal(t, 5*time.Hour, total)
	assert.Contains(t, d.Why[len(d.Why)-2], "split into 2 blocks")

	// Without a minimum chunk it must be contiguous and does not fit before the busy time.
	big.MinChunk = 0
	in.Tasks = []PlanTask{big}
	d = byTitle(Plan(in))["report"]
	assert.Equal(t, "Tue 09:00-14:00", span(t, d))

	// The last chunk is never left under the minimum.
	odd := ptask(2, "odd", 3*time.Hour+20*time.Minute)
	odd.MinChunk = time.Hour
	in2 := baseInput(odd)
	in2.Busy = []Span{{utc(2026, 10, 12, 16, 0), utc(2026, 10, 12, 20, 0)}} // 11:00-15:00
	d = byTitle(Plan(in2))["odd"]
	for _, b := range d.Blocks {
		s, _ := tz.ParseInstant(b.Start)
		e, _ := tz.ParseInstant(b.End)
		assert.GreaterOrEqual(t, e.Sub(s), time.Hour)
	}
}

func TestPlanLateAndHardDeadlineSpill(t *testing.T) {
	// Working hours end at 17:00. A 3 hour task due Monday 19:00 cannot finish inside them after a busy day.
	in := baseInput()
	in.Busy = []Span{{utc(2026, 10, 12, 13, 0), utc(2026, 10, 12, 20, 0)}} // 08:00-15:00 CDT busy
	task := ptask(1, "crunch", 3*time.Hour)
	task.Deadline = utc(2026, 10, 13, 0, 0) // 19:00 CDT
	in.Tasks = []PlanTask{task}
	d := byTitle(Plan(in))["crunch"]
	assert.Equal(t, Late, d.Decision, "soft deadline: placed on Tuesday, flagged late")
	assert.Equal(t, "Tue 09:00-12:00", span(t, d))
	assert.Contains(t, d.Why[len(d.Why)-1], "after the deadline")

	task.Hard = true
	in.Tasks = []PlanTask{task}
	d = byTitle(Plan(in))["crunch"]
	assert.Equal(t, Placed, d.Decision)
	assert.True(t, d.Spilled)
	assert.Equal(t, "Mon 15:15-18:00"[:3], local(t, d.Blocks[0].Start)[:3])
	end, _ := tz.ParseInstant(d.Blocks[len(d.Blocks)-1].End)
	assert.False(t, end.After(task.Deadline), "the spill still meets the deadline")
	assert.Contains(t, d.Why, "used time outside working hours to meet the hard deadline")
}

func TestPlanSkipsTasksWithoutDurationAndUnknownHours(t *testing.T) {
	noLen := ptask(1, "no length", 0)
	other := ptask(2, "elsewhere", time.Hour)
	other.Schedule = "nonexistent"
	ok := ptask(3, "fine", time.Hour)
	res := Plan(baseInput(noLen, other, ok))
	d := byTitle(res)
	assert.Equal(t, "skipped", d["no length"].Decision)
	assert.Equal(t, "no_duration", d["no length"].Reason)
	assert.Equal(t, 1, res.Summary.Skipped)
	assert.Equal(t, Unfit, d["elsewhere"].Decision)
	assert.Equal(t, ReasonNoHours, d["elsewhere"].Reason)
	assert.Equal(t, Placed, d["fine"].Decision)
}

func TestPlanWhyExplainsAndReportsMoves(t *testing.T) {
	task := ptask(1, "write", time.Hour)
	task.Deadline = utc(2026, 10, 14, 22, 0)
	prev := utc(2026, 10, 13, 15, 0)
	task.Prev = &prev
	d := byTitle(Plan(baseInput(task)))["write"]
	require.Len(t, d.Why, 4)
	assert.Contains(t, d.Why[0], "rank 1 of 1: deadline in")
	assert.Equal(t, "priority medium, needs 1h", d.Why[1])
	assert.Equal(t, "fit in gap Mon 09:00-17:00 (8h free)", d.Why[2])
	assert.Equal(t, "10m buffers kept around busy time", d.Why[3])
	assert.Equal(t, tz.FormatInstant(prev), d.MovedFrom)
	require.NotNil(t, d.SlackMin)
	assert.Equal(t, tz.FormatInstant(task.Deadline), d.Deadline)
	assert.Equal(t, d.Blocks[0].End, d.ETA)
}

func TestPlanAcrossDaylightSaving(t *testing.T) {
	now := utc(2026, 10, 30, 13, 0) // Friday 08:00 CDT
	in := baseInput(ptask(1, "a", 6*time.Hour), ptask(2, "b", 6*time.Hour))
	in.Now, in.End = now, now.AddDate(0, 0, 7)
	d := byTitle(Plan(in))
	assert.Equal(t, "2026-10-30T14:00:00Z", d["a"].Blocks[0].Start, "Friday 09:00 CDT")
	assert.Equal(t, "Mon 09:00", local(t, d["b"].Blocks[0].Start), "Monday 09:00 CST")
	assert.Equal(t, "2026-11-02T15:00:00Z", d["b"].Blocks[0].Start)
}

func TestPlanIsDeterministicUnderShuffle(t *testing.T) {
	var tasks []PlanTask
	for i := int64(1); i <= 12; i++ {
		tk := ptask(i, string(rune('a'+i)), time.Duration(30+int(i)*15)*time.Minute)
		tk.Priority = int(i%3) + 1
		if i%2 == 0 {
			tk.Deadline = utc(2026, 10, 13+int(i%4), 22, 0)
		}
		if i%5 == 0 {
			tk.Created = utc(2026, 10, 1, 0, 0) // equal created times: the uuid decides
		}
		tasks = append(tasks, tk)
	}
	want := Plan(baseInput(tasks...))
	r := rand.New(rand.NewSource(7))
	for i := 0; i < 20; i++ {
		shuffled := append([]PlanTask(nil), tasks...)
		r.Shuffle(len(shuffled), func(a, b int) { shuffled[a], shuffled[b] = shuffled[b], shuffled[a] })
		assert.Equal(t, want, Plan(baseInput(shuffled...)))
	}
}

func TestPlanNeverOverlapsAndStaysInsideFreeTime(t *testing.T) {
	var tasks []PlanTask
	for i := int64(1); i <= 15; i++ {
		tk := ptask(i, string(rune('A'+i)), time.Duration(45+int(i%4)*30)*time.Minute)
		tk.MinChunk = 30 * time.Minute
		tasks = append(tasks, tk)
	}
	in := baseInput(tasks...)
	in.Busy = []Span{{utc(2026, 10, 13, 14, 0), utc(2026, 10, 13, 16, 0)}}
	res := Plan(in)
	var all []Span
	for _, d := range res.Decisions {
		for _, b := range d.Blocks {
			s, _ := tz.ParseInstant(b.Start)
			e, _ := tz.ParseInstant(b.End)
			all = append(all, Span{s, e})
		}
	}
	require.NotEmpty(t, all)
	hours, err := Intervals(work("America/Chicago"), planNow, in.End)
	require.NoError(t, err)
	for i, a := range all {
		inside := false
		for _, h := range hours {
			if !a.Start.Before(h.Start) && !a.End.After(h.End) {
				inside = true
			}
		}
		assert.True(t, inside, "block %d is inside working hours", i)
		assert.False(t, a.Start.Before(planNow))
		for j, b := range all {
			if i != j {
				assert.False(t, a.Start.Before(b.End) && b.Start.Before(a.End), "blocks %d and %d overlap", i, j)
			}
		}
		assert.False(t, a.Start.Before(in.Busy[0].End) && in.Busy[0].Start.Before(a.End), "block %d hits the busy time", i)
	}
}

func TestPlanHorizonLimit(t *testing.T) {
	in := baseInput(ptask(1, "one", 5*time.Hour), ptask(2, "two", 5*time.Hour))
	in.End = planNow.AddDate(0, 0, 1) // only Monday
	res := Plan(in)
	d := byTitle(res)
	assert.Equal(t, Placed, d["one"].Decision)
	assert.Equal(t, Unfit, d["two"].Decision)
	assert.Equal(t, ReasonLongerThanG, d["two"].Reason, "no 5 hour gap is left on Monday")
}
