package cli

import (
	"os"
	"strconv"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/input"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/cal/when"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/schedule"
	"github.com/spf13/cobra"
)

var freeColumns = []format.ColumnDef{
	{Header: "Date", Field: "date"},
	{Header: "Time", Field: "time"},
	{Header: "Start", Field: "start"},
}

// withinDays reads --within as days: 14, 7d or 2w.
func withinDays(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n, nil
	}
	d, err := when.ParseLength(s)
	if err != nil || d%(24*time.Hour) != 0 {
		return 0, model.NewExitError(model.ErrValidation, "--within needs whole days, such as 7d or 14")
	}
	return int(d / (24 * time.Hour)), nil
}

func registerCalFreeCommand(calCmd *cobra.Command) {
	var length, from, within, rule, notice, tzName string
	var buffer, step, limit int
	var busyTasks bool

	cmd := &cobra.Command{
		Use:   "free",
		Short: "Find a time: free slots inside working hours, clear of events and buffers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := when.ParseLength(length)
			if err != nil {
				return model.NewExitError(model.ErrValidation, "--len: %v", err)
			}
			disp, err := displayZone(tzName)
			if err != nil {
				return err
			}
			now := clockNow()
			p := schedule.FindParams{Length: l, Now: now, Rule: rule, Limit: limit, BusyTasks: busyTasks}
			if notice != "" {
				if p.MinNotice, err = when.ParseLength(notice); err != nil {
					return model.NewExitError(model.ErrValidation, "--notice: %v", err)
				}
			}
			if cmd.Flags().Changed("buffer") {
				b := time.Duration(buffer) * time.Minute
				p.Buffer = &b
			}
			if cmd.Flags().Changed("step") {
				s := time.Duration(step) * time.Minute
				p.Step = &s
			}
			days, err := withinDays(within)
			if err != nil {
				return err
			}
			if from != "" || days > 0 {
				d := days
				if d == 0 {
					d = 14
				}
				f, t, err := input.Window(from, "", d, disp, now)
				if err != nil {
					return err
				}
				if from != "" {
					p.From = f
				}
				p.To = t
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			res, err := schedule.FindTime(cmd.Context(), db, p)
			if err != nil {
				return err
			}
			rows := make([]map[string]any, len(res.Slots))
			for i, s := range res.Slots {
				st, _ := tz.ParseInstant(s.Start)
				en, _ := tz.ParseInstant(s.End)
				ls, le := st.In(disp), en.In(disp)
				rows[i] = map[string]any{
					"start": s.Start, "end": s.End, "local_start": ls.Format(time.RFC3339), "local_end": le.Format(time.RFC3339),
					"display_tz": tz.Name(disp), "date": ls.Format("Mon 2 Jan"), "time": ls.Format("15:04") + "-" + le.Format("15:04"),
				}
			}
			return format.Output(os.Stdout, resolveFormat(), rows, freeColumns, flagQuiet)
		},
	}
	cmd.Flags().StringVar(&length, "len", "30m", "meeting length, e.g. 30m, 45m, 1h30m")
	cmd.Flags().StringVar(&from, "from", "", "search from this date or phrase (default: now)")
	cmd.Flags().StringVar(&within, "within", "", "search this many days (7d, 14; default: the horizon, 14 days)")
	cmd.Flags().StringVar(&rule, "rule", "", "working hours rule (default: the default rule, work)")
	cmd.Flags().IntVar(&buffer, "buffer", 0, "minutes kept free around busy time (default: stored setting, 10)")
	cmd.Flags().IntVar(&step, "step", 0, "slots start every this many minutes (default: stored setting, 15)")
	cmd.Flags().StringVar(&notice, "notice", "", "do not offer anything sooner than this from now, e.g. 4h")
	cmd.Flags().IntVar(&limit, "limit", 20, "max slots (0 for all)")
	cmd.Flags().BoolVar(&busyTasks, "busy-tasks", false, "also treat placed tasks that are not locked as busy")
	cmd.Flags().StringVar(&tzName, "tz", "", "zone to show slots in (default: this device)")
	calCmd.AddCommand(cmd)
}
