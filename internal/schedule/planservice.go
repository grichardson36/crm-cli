package schedule

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// PlanOptions selects what to plan.
type PlanOptions struct {
	Now time.Time
	// HorizonDays overrides the stored horizon. Zero keeps it.
	HorizonDays int
	// Only limits the plan to these task ids. Empty means every auto-scheduled task.
	Only []int64
}

func priorityOf(p string) int {
	switch p {
	case "high":
		return 3
	case "low":
		return 1
	}
	return 2
}

// taskTime reads a stored task date or instant. A date is the start of that day in loc, or with
// endOfDay the start of the next one.
func taskTime(s string, loc *time.Location, endOfDay bool) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if len(s) == 10 {
		d, err := tz.ParseDate(s)
		if err != nil {
			return time.Time{}, false
		}
		if endOfDay {
			d = d.AddDate(0, 0, 1)
		}
		return tz.Instant(d, loc), true
	}
	t, err := tz.ParseInstant(s)
	return t, err == nil
}

// PlanTasks proposes placements for auto-scheduled tasks. It reads and never writes.
func PlanTasks(ctx context.Context, db *sql.DB, opt PlanOptions) (*PlanResult, error) {
	ar := repo.NewAvailabilityRepo(db)
	cfg, err := ar.Config(ctx)
	if err != nil {
		return nil, err
	}
	days := cfg.HorizonDays
	if opt.HorizonDays > 0 {
		days = opt.HorizonDays
	}
	tasks, err := repo.NewTaskRepo(db).FindAll(ctx, model.TaskFilters{})
	if err != nil {
		return nil, err
	}
	only := map[int64]bool{}
	for _, id := range opt.Only {
		only[id] = true
	}
	hours := map[string]*model.WorkingHours{}
	loadHours := func(name string) *model.WorkingHours {
		if h, ok := hours[name]; ok {
			return h
		}
		h, err := ar.Get(ctx, name)
		if err != nil {
			h = nil
		}
		hours[name] = h
		return h
	}
	in := PlanInput{Now: opt.Now, End: opt.Now.AddDate(0, 0, days), Hours: hours,
		Buffer: time.Duration(cfg.BufferMin) * time.Minute, Step: time.Duration(cfg.StepMin) * time.Minute}
	for _, t := range tasks {
		if !t.AutoSchedule || t.ScheduledLocked || t.Completed {
			continue
		}
		if len(only) > 0 && !only[t.ID] {
			continue
		}
		h := loadHours(t.ScheduleName)
		loc := time.UTC
		if h != nil {
			if l, err := tz.Load(h.TZ); err == nil {
				loc = l
			}
		}
		pt := PlanTask{ID: t.ID, UUID: t.UUID, Title: t.Title, Priority: priorityOf(t.Priority), Schedule: t.ScheduleName, Hard: t.DeadlineHard}
		if t.DurationMin != nil {
			pt.Duration = time.Duration(*t.DurationMin) * time.Minute
		}
		if t.MinChunkMin != nil {
			pt.MinChunk = time.Duration(*t.MinChunkMin) * time.Minute
		}
		if t.EarliestStart != nil {
			if v, ok := taskTime(*t.EarliestStart, loc, false); ok {
				pt.Earliest = v
			}
		}
		switch {
		case t.DeadlineAt != nil:
			if v, ok := taskTime(*t.DeadlineAt, loc, true); ok {
				pt.Deadline = v
			}
		case t.DueAt != nil:
			// The due date is the soft deadline when no deadline is set (Q2).
			if v, ok := taskTime(*t.DueAt, loc, true); ok {
				pt.Deadline, pt.SoftDeadline = v, true
			}
		}
		if v, err := tz.ParseInstant(t.CreatedAt); err == nil {
			pt.Created = v
		}
		if t.ScheduledStart != nil {
			if v, err := tz.ParseInstant(*t.ScheduledStart); err == nil {
				pt.Prev = &v
			}
		}
		in.Tasks = append(in.Tasks, pt)
	}
	busyLoc := time.UTC
	if h := loadHours(model.DefaultScheduleConfig().DefaultRule); h != nil {
		if l, err := tz.Load(h.TZ); err == nil {
			busyLoc = l
		}
	}
	in.Busy, err = Busy(ctx, db, in.Now.Add(-in.Buffer), in.End.Add(in.Buffer), busyLoc, false)
	if err != nil {
		return nil, err
	}
	res := Plan(in)
	return &res, nil
}

// ApplyOptions refines which decisions are written.
type ApplyOptions struct {
	// Reject leaves these task ids as they are.
	Reject map[int64]bool
	// Lock locks these task ids at their proposed time.
	Lock map[int64]bool
	Note string
}

// Apply writes a proposal: placed and late tasks get their time, and tasks the planner placed
// earlier but could not place now are unplaced. Locked tasks are never touched. It returns the
// journal entry that undoes it.
func Apply(ctx context.Context, db *sql.DB, res *PlanResult, opt ApplyOptions) (*repo.PlanJournal, error) {
	var entries []repo.PlanEntry
	for _, d := range res.Decisions {
		if opt.Reject[d.TaskID] {
			continue
		}
		switch d.Decision {
		case Placed, Late:
			if len(d.Blocks) == 0 {
				continue
			}
			start := d.Blocks[0].Start
			e := repo.PlanEntry{TaskID: d.TaskID, Start: &start, Lock: opt.Lock[d.TaskID]}
			if len(d.Blocks) > 1 {
				for _, b := range d.Blocks {
					e.Blocks = append(e.Blocks, repo.Chunk{Start: b.Start, End: b.End})
				}
			}
			entries = append(entries, e)
		case Unfit, "skipped":
			entries = append(entries, repo.PlanEntry{TaskID: d.TaskID})
		}
	}
	// Leave out entries that would change nothing.
	var keep []repo.PlanEntry
	tr := repo.NewTaskRepo(db)
	chunks, err := repo.NewPlanRepo(db).Chunks(ctx)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		t, err := tr.FindByID(ctx, e.TaskID)
		if err != nil {
			continue
		}
		if e.Start == nil && t.ScheduledStart == nil {
			continue
		}
		if e.Start != nil && t.ScheduledStart != nil && *t.ScheduledStart == *e.Start && sameChunks(e.Blocks, chunks[e.TaskID]) && e.Lock == t.ScheduledLocked {
			continue
		}
		keep = append(keep, e)
	}
	if len(keep) == 0 {
		return nil, fmt.Errorf("the plan changes nothing: %w", model.ErrConflict)
	}
	note := opt.Note
	if note == "" {
		note = fmt.Sprintf("%d placed, %d late, %d unfit", res.Summary.Placed, res.Summary.Late, res.Summary.Unfit)
	}
	return repo.NewPlanRepo(db).Apply(ctx, keep, note)
}

func sameChunks(a, b []repo.Chunk) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
