package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/schedule"
	"github.com/spf13/cobra"
)

var planColumns = []format.ColumnDef{
	{Header: "ID", Field: "task_id"},
	{Header: "Task", Field: "task"},
	{Header: "Decision", Field: "decision"},
	{Header: "Start", Field: "start"},
	{Header: "ETA", Field: "eta_local"},
	{Header: "Slack", Field: "slack_min"},
	{Header: "Why", Field: "why_text"},
}

// planDiff is one row of what --apply would change.
type planDiff struct {
	TaskID int64  `json:"task_id"`
	Task   string `json:"task"`
	Old    string `json:"old_start,omitempty"`
	New    string `json:"new_start,omitempty"`
}

func decisionRows(res *schedule.PlanResult, disp *time.Location) []map[string]any {
	rows := make([]map[string]any, len(res.Decisions))
	for i, d := range res.Decisions {
		m := toMap(d)
		if len(d.Blocks) > 0 {
			s, _ := tz.ParseInstant(d.Blocks[0].Start)
			m["start"] = s.In(disp).Format("Mon 2 Jan 15:04")
			if len(d.Blocks) > 1 {
				m["start"] = fmt.Sprintf("%s (+%d more)", m["start"], len(d.Blocks)-1)
			}
		}
		if d.ETA != "" {
			e, _ := tz.ParseInstant(d.ETA)
			m["eta_local"] = e.In(disp).Format("Mon 2 Jan 15:04")
		}
		if len(d.Why) > 0 {
			m["why_text"] = d.Why[len(d.Why)-1]
			if d.Reason != "" {
				m["why_text"] = d.Reason
			}
		}
		rows[i] = m
	}
	return rows
}

func diffOf(res *schedule.PlanResult) []planDiff {
	var out []planDiff
	for _, d := range res.Decisions {
		switch d.Decision {
		case schedule.Placed, schedule.Late:
			if len(d.Blocks) > 0 && d.PrevStart != d.Blocks[0].Start {
				out = append(out, planDiff{TaskID: d.TaskID, Task: d.Title, Old: d.PrevStart, New: d.Blocks[0].Start})
			}
		case schedule.Unfit, "skipped":
			if d.PrevStart != "" {
				out = append(out, planDiff{TaskID: d.TaskID, Task: d.Title, Old: d.PrevStart})
			}
		}
	}
	return out
}

func registerCalPlanCommand(calCmd *cobra.Command) {
	var apply, undo, history, dryRun bool
	var only, reject, lock []int64
	var horizon, nowText, tzName string

	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Propose where auto-scheduled tasks go; --apply writes the times, --undo restores the last plan",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			disp, err := displayZone(tzName)
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			pr := repo.NewPlanRepo(db)

			switch {
			case history:
				list, err := pr.History(cmd.Context(), 20)
				if err != nil {
					return err
				}
				rows := make([]map[string]any, len(list))
				for i, j := range list {
					rows[i] = map[string]any{"id": j.ID, "uuid": j.UUID, "note": j.Note, "tasks": len(j.Entries), "undone": j.Undone, "created_at": j.CreatedAt}
				}
				return format.Output(os.Stdout, resolveFormat(), rows, []format.ColumnDef{
					{Header: "ID", Field: "id"}, {Header: "When", Field: "created_at"}, {Header: "Note", Field: "note"}, {Header: "Tasks", Field: "tasks"}, {Header: "Undone", Field: "undone"}}, flagQuiet)
			case undo:
				if dryRun {
					list, err := pr.History(cmd.Context(), 20)
					if err != nil {
						return err
					}
					for _, j := range list {
						if !j.Undone {
							return printPlanJSON(map[string]any{"dry_run": true, "would_undo": j})
						}
					}
					return model.NewExitError(model.ErrNotFound, "no applied plan to undo")
				}
				j, err := pr.Undo(cmd.Context())
				if err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "Undid plan #%d (%d tasks)\n", j.ID, len(j.Entries))
				return format.Output(os.Stdout, resolveFormat(), []map[string]any{{"id": j.ID, "note": j.Note, "tasks": len(j.Entries), "undone": true}},
					[]format.ColumnDef{{Header: "ID", Field: "id"}, {Header: "Note", Field: "note"}, {Header: "Tasks", Field: "tasks"}}, flagQuiet)
			}

			now := clockNow()
			if nowText != "" {
				if now, err = time.Parse(time.RFC3339, nowText); err != nil {
					return model.NewExitError(model.ErrValidation, "--now must be an RFC 3339 time such as 2026-10-12T13:00:00Z")
				}
			}
			days, err := withinDays(horizon)
			if err != nil {
				return err
			}
			res, err := schedule.PlanTasks(cmd.Context(), db, schedule.PlanOptions{Now: now, HorizonDays: days, Only: only})
			if err != nil {
				return err
			}
			out := map[string]any{"mode": "proposal", "applied": false, "dry_run": dryRun, "now": res.Now, "horizon_end": res.Horizon, "summary": res.Summary, "tasks": res.Decisions}
			if apply {
				diff := diffOf(res)
				out["diff"] = diff
				if dryRun {
					out["mode"] = "apply preview"
				} else {
					rej, lk := map[int64]bool{}, map[int64]bool{}
					for _, id := range reject {
						rej[id] = true
					}
					for _, id := range lock {
						lk[id] = true
					}
					j, err := schedule.Apply(cmd.Context(), db, res, schedule.ApplyOptions{Reject: rej, Lock: lk})
					if err != nil {
						return err
					}
					out["mode"], out["applied"], out["journal_id"] = "applied", true, j.ID
				}
			}
			if resolveFormat() == format.FormatJSON {
				return printPlanJSON(out)
			}
			fmt.Fprintf(os.Stderr, "%d placed, %d late, %d unfit, %d without a duration; %d min overloaded\n",
				res.Summary.Placed, res.Summary.Late, res.Summary.Unfit, res.Summary.Skipped, res.Summary.OverloadMinutes)
			for _, s := range res.Summary.Suggestions {
				fmt.Fprintln(os.Stderr, "  suggestion:", s)
			}
			switch {
			case out["applied"] == true:
				fmt.Fprintf(os.Stderr, "applied as plan #%v (undo with: crm cal plan --undo)\n", out["journal_id"])
			case apply && dryRun:
				fmt.Fprintf(os.Stderr, "dry run: %d change(s) would be written\n", len(out["diff"].([]planDiff)))
			default:
				fmt.Fprintln(os.Stderr, "proposal only, nothing was written (add --apply)")
			}
			return format.Output(os.Stdout, resolveFormat(), decisionRows(res, disp), planColumns, flagQuiet)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "write the proposed times (scheduled_start, split blocks); the plan stays undoable")
	cmd.Flags().BoolVar(&undo, "undo", false, "restore what the last applied plan replaced")
	cmd.Flags().BoolVar(&history, "history", false, "list applied plans")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "with --apply: print the old and new times and write nothing")
	cmd.Flags().Int64SliceVar(&only, "only", nil, "plan only these task IDs (repeatable)")
	cmd.Flags().Int64SliceVar(&reject, "reject", nil, "with --apply: leave these task IDs as they are")
	cmd.Flags().Int64SliceVar(&lock, "lock", nil, "with --apply: lock these task IDs at their proposed time")
	cmd.Flags().StringVar(&horizon, "horizon", "", "days to plan ahead, 7d or 21 (default: the stored horizon, 14 days)")
	cmd.Flags().StringVar(&nowText, "now", "", "plan as of this RFC 3339 time (for tests and agents)")
	cmd.Flags().StringVar(&tzName, "tz", "", "zone to show times in (default: this device)")
	calCmd.AddCommand(cmd)
}

func printPlanJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	return enc.Encode([]any{v})
}
