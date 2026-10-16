package cli

import (
	"strings"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/cal/when"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

// schedFlags are the scheduling flags shared by task add and task edit.
type schedFlags struct {
	duration, minChunk, earliest, deadline, scheduledStart, scheduleName string
	hard, auto, lock                                                     bool
}

func bindScheduleFlags(cmd *cobra.Command) *schedFlags {
	f := &schedFlags{}
	fl := cmd.Flags()
	fl.StringVar(&f.duration, "duration", "", `how long the task takes, e.g. 30m, 2h (empty clears)`)
	fl.StringVar(&f.earliest, "earliest-start", "", "do not schedule before this date or time (empty clears)")
	fl.StringVar(&f.deadline, "deadline", "", "the scheduler plans against this date or time (empty clears; --due stays the soft target)")
	fl.BoolVar(&f.hard, "hard-deadline", false, "the deadline may use hours outside working hours")
	fl.StringVar(&f.minChunk, "min-chunk", "", "smallest block the task may be split into, e.g. 30m (empty clears)")
	fl.BoolVar(&f.auto, "auto-schedule", false, "let the planner place this task (30 min unless --duration is set)")
	fl.StringVar(&f.scheduleName, "schedule-name", "", "working hours rule the task is placed in (default work)")
	fl.StringVar(&f.scheduledStart, "scheduled-start", "", "place the task at this time by hand; it is locked (empty unplaces)")
	fl.BoolVar(&f.lock, "lock", false, "keep the planner away from the placed time (--lock=false unlocks)")
	return f
}

// timeText reads a date or time phrase as a stored task time: a date, or an RFC 3339 instant.
func timeText(flag, s string, loc string) (string, error) {
	if strings.TrimSpace(s) == "" {
		return "", nil
	}
	l, err := tz.Load(loc)
	if err != nil {
		return "", err
	}
	r, err := when.Parse(s, clockNow(), l)
	if err != nil {
		return "", model.NewExitError(model.ErrValidation, "--%s: %v", flag, err)
	}
	if r.AllDay {
		return r.Start.Format(tz.DateLayout), nil
	}
	return tz.FormatInstant(r.Start), nil
}

func minutesText(flag, s string) (*int, error) {
	if v := strings.ToLower(strings.TrimSpace(s)); v == "" || v == "none" || v == "0" {
		zero := 0
		return &zero, nil
	}
	d, err := when.ParseLength(s)
	if err != nil {
		return nil, model.NewExitError(model.ErrValidation, "--%s: %v", flag, err)
	}
	n := int(d.Minutes())
	return &n, nil
}

// build turns the flags the user actually passed into a schedule patch.
func (f *schedFlags) build(cmd *cobra.Command) (model.TaskSchedule, error) {
	var s model.TaskSchedule
	ch := cmd.Flags().Changed
	var err error
	if ch("duration") {
		if s.DurationMin, err = minutesText("duration", f.duration); err != nil {
			return s, err
		}
	}
	if ch("min-chunk") {
		if s.MinChunkMin, err = minutesText("min-chunk", f.minChunk); err != nil {
			return s, err
		}
	}
	if ch("earliest-start") {
		v, err := timeText("earliest-start", f.earliest, "")
		if err != nil {
			return s, err
		}
		s.EarliestStart = &v
	}
	if ch("deadline") {
		v, err := timeText("deadline", f.deadline, "")
		if err != nil {
			return s, err
		}
		s.DeadlineAt = &v
	}
	if ch("scheduled-start") {
		v := ""
		if strings.TrimSpace(f.scheduledStart) != "" {
			r, err := when.Parse(f.scheduledStart, clockNow(), tz.Device())
			if err != nil || r.AllDay {
				return s, model.NewExitError(model.ErrValidation, "--scheduled-start needs a date and time, such as \"fri 14:00\"")
			}
			v = tz.FormatInstant(r.Start)
		}
		s.ScheduledStart = &v
	}
	if ch("hard-deadline") {
		s.DeadlineHard = &f.hard
	}
	if ch("auto-schedule") {
		s.AutoSchedule = &f.auto
	}
	if ch("lock") {
		s.ScheduledLocked = &f.lock
	}
	if ch("schedule-name") {
		s.ScheduleName = &f.scheduleName
	}
	return s, nil
}
