package mcp

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/input"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/cal/when"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/schedule"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func minutesArg(args map[string]any, key string) (*int, error) {
	v, ok := argString(args, key)
	if !ok {
		return nil, nil
	}
	if s := strings.ToLower(strings.TrimSpace(v)); s == "" || s == "none" || s == "0" {
		z := 0
		return &z, nil
	}
	d, err := when.ParseLength(v)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	n := int(d.Minutes())
	return &n, nil
}

func timeArg(args map[string]any, key string) (*string, error) {
	v, ok := argString(args, key)
	if !ok {
		return nil, nil
	}
	if strings.TrimSpace(v) == "" {
		return &v, nil
	}
	loc := tz.Device()
	r, err := when.Parse(v, calNow(), loc)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	out := tz.FormatInstant(r.Start)
	if r.AllDay {
		out = r.Start.Format(tz.DateLayout)
	}
	return &out, nil
}

// scheduleFromArgs reads the task scheduling arguments shared by crm_task_create and crm_task_update.
func scheduleFromArgs(args map[string]any) (model.TaskSchedule, error) {
	var s model.TaskSchedule
	var err error
	if s.DurationMin, err = minutesArg(args, "duration"); err != nil {
		return s, err
	}
	if s.MinChunkMin, err = minutesArg(args, "min_chunk"); err != nil {
		return s, err
	}
	if s.EarliestStart, err = timeArg(args, "earliest_start"); err != nil {
		return s, err
	}
	if s.DeadlineAt, err = timeArg(args, "deadline"); err != nil {
		return s, err
	}
	if v, ok := argString(args, "scheduled_start"); ok {
		out := ""
		if strings.TrimSpace(v) != "" {
			r, perr := when.Parse(v, calNow(), tz.Device())
			if perr != nil || r.AllDay {
				return s, fmt.Errorf("scheduled_start needs a date and time, such as \"fri 14:00\": %w", model.ErrValidation)
			}
			out = tz.FormatInstant(r.Start)
		}
		s.ScheduledStart = &out
	}
	if v, ok := args["hard_deadline"].(bool); ok {
		s.DeadlineHard = &v
	}
	if v, ok := args["auto_schedule"].(bool); ok {
		s.AutoSchedule = &v
	}
	if v, ok := args["locked"].(bool); ok {
		s.ScheduledLocked = &v
	}
	if v, ok := argString(args, "schedule_name"); ok {
		s.ScheduleName = &v
	}
	return s, nil
}

func parseRangesArg(args map[string]any, key string) ([]model.TimeRange, error) {
	v, _ := argString(args, key)
	return model.ParseRanges(v)
}

// registerScheduleTools adds task updates with scheduling fields and the working hours tools.
func registerScheduleTools(s *server.MCPServer, db *sql.DB, tr *repo.TaskRepo) {
	ar := repo.NewAvailabilityRepo(db)

	s.AddTool(
		gomcp.NewTool("crm_task_update",
			gomcp.WithDescription("Change a task. Only the fields you pass change. Scheduling fields: duration, earliest_start, deadline, hard_deadline, min_chunk, auto_schedule, schedule_name, scheduled_start (locks), locked. An empty string clears a text field. An auto-scheduled task without a duration is listed with schedule_issue no_duration."),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Task ID")),
			gomcp.WithString("title", gomcp.Description("Title")),
			gomcp.WithString("description", gomcp.Description("Description")),
			gomcp.WithString("due", gomcp.Description("Soft due date (ISO 8601)")),
			gomcp.WithString("priority", gomcp.Description("low, medium or high")),
			gomcp.WithString("duration", gomcp.Description("How long it takes: 30m, 2h (empty clears)")),
			gomcp.WithString("earliest_start", gomcp.Description("Do not schedule before this date or time")),
			gomcp.WithString("deadline", gomcp.Description("The scheduler plans against this date or time")),
			gomcp.WithBoolean("hard_deadline", gomcp.Description("The deadline may use hours outside working hours")),
			gomcp.WithString("min_chunk", gomcp.Description("Smallest block the task may be split into")),
			gomcp.WithBoolean("auto_schedule", gomcp.Description("Let the planner place it (needs a duration)")),
			gomcp.WithString("schedule_name", gomcp.Description("Working hours rule, default work")),
			gomcp.WithString("scheduled_start", gomcp.Description("Place it at this date and time by hand; this locks it (empty unplaces)")),
			gomcp.WithBoolean("locked", gomcp.Description("Keep the planner away from the placed time")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			id, errResult := requireID(req, "id")
			if errResult != nil {
				return errResult, nil
			}
			args := req.GetArguments()
			var in model.UpdateTaskInput
			if v, ok := argString(args, "title"); ok {
				in.Title = &v
			}
			if v, ok := argString(args, "description"); ok {
				in.Description = &v
			}
			if v, ok := argString(args, "due"); ok {
				in.DueAt = &v
			}
			if v, ok := argString(args, "priority"); ok {
				in.Priority = &v
			}
			sched, err := scheduleFromArgs(args)
			if err != nil {
				return validationError(err)
			}
			in.Schedule = sched
			t, err := tr.Update(ctx, id, in)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(taskView{Task: t, ScheduleIssue: repo.ScheduleIssue(t)})
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_cal_hours_get",
			gomcp.WithDescription("Working hours (weekly hours per day, date overrides, zone) and the scheduling defaults (buffer, step, horizon, default rule). With expand_from, also the working intervals for the horizon starting at that date."),
			gomcp.WithString("name", gomcp.Description("Rule name (default: the default rule, work)")),
			gomcp.WithString("expand_from", gomcp.Description("List the working intervals for the horizon from this date, such as today")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			h, err := ar.Get(ctx, req.GetString("name", ""))
			if err != nil {
				return mcpError(err)
			}
			cfg, err := ar.Config(ctx)
			if err != nil {
				return mcpError(err)
			}
			out := map[string]any{"hours": h, "config": cfg}
			if from := req.GetString("expand_from", ""); from != "" {
				loc, err := tz.Load(h.TZ)
				if err != nil {
					return validationError(err)
				}
				r, err := when.Parse(from, calNow(), loc)
				if err != nil {
					return validationError(err)
				}
				w := r.Start
				if !r.AllDay {
					w = tz.Wall(r.Start, loc)
				}
				start := tz.Instant(time.Date(w.Year(), w.Month(), w.Day(), 0, 0, 0, 0, time.UTC), loc)
				spans, err := schedule.Intervals(h, start, start.AddDate(0, 0, cfg.HorizonDays))
				if err != nil {
					return validationError(err)
				}
				list := make([]map[string]string, len(spans))
				for i, sp := range spans {
					list[i] = map[string]string{"start": tz.FormatInstant(sp.Start), "end": tz.FormatInstant(sp.End)}
				}
				out["intervals"] = list
			}
			return jsonResult(out)
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_cal_hours_set",
			gomcp.WithDescription("Change working hours. With days and hours: set those weekdays (days like mon-fri, weekdays, mon,wed; hours like 09:00-17:00 or 09:00-12:00,13:00-17:00 or off). With date and hours: set one date, off is a day off. With remove_date: delete that date's override. buffer, step, horizon and default_rule change the scheduling defaults."),
			gomcp.WithString("name", gomcp.Description("Rule name (default: the default rule, work)")),
			gomcp.WithString("days", gomcp.Description("Weekdays to set")),
			gomcp.WithString("hours", gomcp.Description("Hours for the days or the date, or off")),
			gomcp.WithString("tz", gomcp.Description("IANA zone of the rule")),
			gomcp.WithString("date", gomcp.Description("A date (2026-12-24) to override")),
			gomcp.WithString("remove_date", gomcp.Description("A date whose override is removed")),
			gomcp.WithNumber("buffer", gomcp.Description("Minutes kept free around busy time")),
			gomcp.WithNumber("step", gomcp.Description("Slots start every this many minutes")),
			gomcp.WithNumber("horizon", gomcp.Description("Days ahead to plan and offer")),
			gomcp.WithString("default_rule", gomcp.Description("Name of the default rule")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			args := req.GetArguments()
			name := req.GetString("name", "")
			var buffer, step, horizon *int
			for k, dst := range map[string]**int{"buffer": &buffer, "step": &step, "horizon": &horizon} {
				if v, ok := argInt64(args, k); ok {
					n := int(v)
					*dst = &n
				}
			}
			var rule *string
			if v, ok := argString(args, "default_rule"); ok {
				rule = &v
			}
			if buffer != nil || step != nil || horizon != nil || rule != nil {
				if _, err := ar.SetConfig(ctx, rule, buffer, step, horizon); err != nil {
					return mcpError(err)
				}
			}
			if d := req.GetString("remove_date", ""); d != "" {
				if _, err := ar.RemoveOverride(ctx, name, d); err != nil {
					return mcpError(err)
				}
			}
			if d := req.GetString("date", ""); d != "" {
				ranges, err := parseRangesArg(args, "hours")
				if err != nil {
					return validationError(err)
				}
				if _, err := ar.SetOverride(ctx, name, d, ranges); err != nil {
					return mcpError(err)
				}
			}
			if ds := req.GetString("days", ""); ds != "" {
				days, err := model.ParseDays(ds)
				if err != nil {
					return validationError(err)
				}
				ranges, err := parseRangesArg(args, "hours")
				if err != nil {
					return validationError(err)
				}
				if _, err := ar.SetDays(ctx, name, req.GetString("tz", ""), days, ranges); err != nil {
					return mcpError(err)
				}
			}
			h, err := ar.Get(ctx, name)
			if err != nil {
				return mcpError(err)
			}
			cfg, err := ar.Config(ctx)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(map[string]any{"hours": h, "config": cfg})
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_cal_free",
			gomcp.WithDescription("Find a time: meeting slots of a given length inside working hours, clear of busy events (repeats expanded), locked task blocks and buffers. Slots start on the step grid (default every 15 minutes), after the minimum notice, within the horizon (default 14 days). Only your own calendar is checked. Times are UTC; zone says the wall clock of the working hours."),
			gomcp.WithString("length", gomcp.Required(), gomcp.Description("Meeting length: 30m, 45m, 1h30m")),
			gomcp.WithString("from", gomcp.Description("Search from this date or phrase (default: now)")),
			gomcp.WithNumber("within_days", gomcp.Description("Days to search (default: the horizon)")),
			gomcp.WithString("rule", gomcp.Description("Working hours rule (default: the default rule)")),
			gomcp.WithNumber("buffer", gomcp.Description("Minutes kept free around busy time (default: stored setting)")),
			gomcp.WithNumber("step", gomcp.Description("Slots start every this many minutes (default: stored setting)")),
			gomcp.WithString("notice", gomcp.Description("Minimum notice from now, such as 4h")),
			gomcp.WithNumber("limit", gomcp.Description("Max slots (default 20)")),
			gomcp.WithBoolean("busy_tasks", gomcp.Description("Also treat placed tasks that are not locked as busy")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			args := req.GetArguments()
			length, err := when.ParseLength(req.GetString("length", ""))
			if err != nil {
				return validationError(err)
			}
			now := calNow()
			p := schedule.FindParams{Length: length, Now: now, Rule: req.GetString("rule", ""), Limit: req.GetInt("limit", 20), BusyTasks: req.GetBool("busy_tasks", false)}
			if v := req.GetString("notice", ""); v != "" {
				if p.MinNotice, err = when.ParseLength(v); err != nil {
					return validationError(err)
				}
			}
			if v, ok := argInt64(args, "buffer"); ok {
				b := time.Duration(v) * time.Minute
				p.Buffer = &b
			}
			if v, ok := argInt64(args, "step"); ok {
				st := time.Duration(v) * time.Minute
				p.Step = &st
			}
			from := req.GetString("from", "")
			days, _ := argInt64(args, "within_days")
			if from != "" || days > 0 {
				if days <= 0 {
					days = 14
				}
				f, t, err := input.Window(from, "", int(days), tz.Device(), now)
				if err != nil {
					return validationError(err)
				}
				if from != "" {
					p.From = f
				}
				p.To = t
			}
			res, err := schedule.FindTime(ctx, db, p)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(res)
		},
	)
}

// taskView is a task with its scheduling issue, if any.
type taskView struct {
	*model.Task
	ScheduleIssue string `json:"schedule_issue,omitempty"`
}
