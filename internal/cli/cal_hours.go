package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/input"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/schedule"
	"github.com/spf13/cobra"
)

var hoursColumns = []format.ColumnDef{
	{Header: "Rule", Field: "name"},
	{Header: "Zone", Field: "tz"},
	{Header: "Day", Field: "day"},
	{Header: "Hours", Field: "hours"},
}

func rangesText(rs []model.TimeRange) string {
	if len(rs) == 0 {
		return "off"
	}
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = r.Start + "-" + r.End
	}
	return strings.Join(parts, ", ")
}

// hoursMap is the JSON view of working hours with the scheduling defaults.
func hoursMap(h *model.WorkingHours, cfg model.ScheduleConfig) map[string]any {
	m := toMap(h)
	m["config"] = cfg
	return m
}

func hoursRows(h *model.WorkingHours) []map[string]any {
	var rows []map[string]any
	for _, d := range model.WeekdayKeys {
		rows = append(rows, map[string]any{"name": h.Name, "tz": h.TZ, "day": d, "hours": rangesText(h.Weekly[d])})
	}
	dates := make([]string, 0, len(h.Overrides))
	for d := range h.Overrides {
		dates = append(dates, d)
	}
	sortStrings(dates)
	for _, d := range dates {
		rows = append(rows, map[string]any{"name": h.Name, "tz": h.TZ, "day": d, "hours": rangesText(h.Overrides[d])})
	}
	return rows
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func printHours(h *model.WorkingHours, cfg model.ScheduleConfig) error {
	if resolveFormat() == format.FormatJSON {
		return format.Output(os.Stdout, format.FormatJSON, []map[string]any{hoursMap(h, cfg)}, nil, flagQuiet)
	}
	return format.Output(os.Stdout, resolveFormat(), hoursRows(h), hoursColumns, flagQuiet)
}

func registerCalHoursCommands(calCmd *cobra.Command) {
	hours := &cobra.Command{Use: "hours", Short: "Working hours and scheduling defaults"}

	var name string
	var expand string
	show := &cobra.Command{
		Use:   "show",
		Short: "Show working hours, date overrides and the scheduling defaults",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			ar := repo.NewAvailabilityRepo(db)
			h, err := ar.Get(cmd.Context(), name)
			if err != nil {
				return err
			}
			cfg, err := ar.Config(cmd.Context())
			if err != nil {
				return err
			}
			if expand == "" {
				return printHours(h, cfg)
			}
			loc, err := tz.Load(h.TZ)
			if err != nil {
				return err
			}
			from, to, err := input.Window(expand, "", cfg.HorizonDays, loc, clockNow())
			if err != nil {
				return err
			}
			spans, err := schedule.Intervals(h, from, to)
			if err != nil {
				return err
			}
			rows := make([]map[string]any, len(spans))
			for i, s := range spans {
				rows[i] = map[string]any{
					"start": tz.FormatInstant(s.Start), "end": tz.FormatInstant(s.End),
					"local_start": s.Start.In(loc).Format(time.RFC3339), "local_end": s.End.In(loc).Format(time.RFC3339),
					"date": s.Start.In(loc).Format("Mon 2 Jan"), "hours": s.Start.In(loc).Format("15:04") + "-" + s.End.In(loc).Format("15:04"),
				}
			}
			return format.Output(os.Stdout, resolveFormat(), rows, []format.ColumnDef{{Header: "Date", Field: "date"}, {Header: "Hours", Field: "hours"}}, flagQuiet)
		},
	}
	show.Flags().StringVar(&name, "name", "", "working hours rule (default: the default rule, work)")
	show.Flags().StringVar(&expand, "expand", "", "instead list the working intervals for the horizon starting at this date (use today)")

	var setName, setTZ string
	set := &cobra.Command{
		Use:   "set <days> <hours>",
		Short: "Set the hours of some weekdays, such as set mon-fri 09:00-17:00 or set sat off",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			days, err := model.ParseDays(args[0])
			if err != nil {
				return err
			}
			ranges, err := model.ParseRanges(args[1])
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			ar := repo.NewAvailabilityRepo(db)
			h, err := ar.SetDays(cmd.Context(), setName, setTZ, days, ranges)
			if err != nil {
				return err
			}
			cfg, err := ar.Config(cmd.Context())
			if err != nil {
				return err
			}
			return printHours(h, cfg)
		},
	}
	set.Flags().StringVar(&setName, "name", "", "working hours rule (default: the default rule, work)")
	set.Flags().StringVar(&setTZ, "tz", "", "IANA zone of the rule (default: this device)")

	var ovName string
	var ovRemove bool
	override := &cobra.Command{
		Use:   "override <date> [hours]",
		Short: "Set the hours of one date (off is a day off), or remove the override with --remove",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			ar := repo.NewAvailabilityRepo(db)
			var h *model.WorkingHours
			switch {
			case ovRemove:
				h, err = ar.RemoveOverride(cmd.Context(), ovName, args[0])
			case len(args) < 2:
				return model.NewExitError(model.ErrValidation, "give the hours for %s, such as 10:00-14:00, or off", args[0])
			default:
				var ranges []model.TimeRange
				if ranges, err = model.ParseRanges(args[1]); err != nil {
					return err
				}
				h, err = ar.SetOverride(cmd.Context(), ovName, args[0], ranges)
			}
			if err != nil {
				return err
			}
			cfg, err := ar.Config(cmd.Context())
			if err != nil {
				return err
			}
			return printHours(h, cfg)
		},
	}
	override.Flags().StringVar(&ovName, "name", "", "working hours rule (default: the default rule, work)")
	override.Flags().BoolVar(&ovRemove, "remove", false, "remove the override of this date")

	var rule string
	var buffer, step, horizon int
	config := &cobra.Command{
		Use:   "config",
		Short: "Show or change the scheduling defaults: buffer, step, horizon and default rule",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			ar := repo.NewAvailabilityRepo(db)
			var rp *string
			var bp, sp, hp *int
			ch := cmd.Flags().Changed
			if ch("rule") {
				rp = &rule
			}
			if ch("buffer") {
				bp = &buffer
			}
			if ch("step") {
				sp = &step
			}
			if ch("horizon") {
				hp = &horizon
			}
			cfg, err := ar.SetConfig(cmd.Context(), rp, bp, sp, hp)
			if err != nil {
				return err
			}
			if resolveFormat() == format.FormatJSON {
				return format.Output(os.Stdout, format.FormatJSON, []map[string]any{toMap(cfg)}, nil, flagQuiet)
			}
			rows := []map[string]any{
				{"setting": "default rule", "value": cfg.DefaultRule}, {"setting": "buffer (min)", "value": cfg.BufferMin},
				{"setting": "step (min)", "value": cfg.StepMin}, {"setting": "horizon (days)", "value": cfg.HorizonDays},
			}
			return format.Output(os.Stdout, resolveFormat(), rows, []format.ColumnDef{{Header: "Setting", Field: "setting"}, {Header: "Value", Field: "value"}}, flagQuiet)
		},
	}
	config.Flags().StringVar(&rule, "rule", "", "default working hours rule")
	config.Flags().IntVar(&buffer, "buffer", 0, "minutes kept free around busy time")
	config.Flags().IntVar(&step, "step", 0, "slots start every this many minutes")
	config.Flags().IntVar(&horizon, "horizon", 0, "how many days ahead to plan and offer")

	hours.AddCommand(show, set, override, config)
	calCmd.AddCommand(hours)
}

var _ = fmt.Sprint
