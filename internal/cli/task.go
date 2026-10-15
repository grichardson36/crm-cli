package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/quick"
	"github.com/jdanielnd/crm-cli/internal/shortsyntax"
	"github.com/spf13/cobra"
)

var taskColumns = []format.ColumnDef{
	{Header: "ID", Field: "id"},
	{Header: "Title", Field: "title"},
	{Header: "Due", Field: "due_at"},
	{Header: "Priority", Field: "priority"},
	{Header: "Done", Field: "completed"},
	{Header: "Person", Field: "person_id"},
	{Header: "Repeats", Field: "recurrence"},
	{Header: "Tags", Field: "tags"},
	{Header: "Length", Field: "duration_min"},
	{Header: "Placed", Field: "scheduled_start"},
	{Header: "Issue", Field: "schedule_issue"},
}

func taskToMap(t *model.Task) map[string]any {
	m := map[string]any{
		"id":         t.ID,
		"uuid":       t.UUID,
		"title":      t.Title,
		"priority":   t.Priority,
		"completed":  t.Completed,
		"created_at": t.CreatedAt,
		"updated_at": t.UpdatedAt,
	}
	if t.Description != nil {
		m["description"] = *t.Description
	}
	if t.PersonID != nil {
		m["person_id"] = *t.PersonID
	}
	if len(t.PersonIDs) > 0 {
		m["person_ids"] = t.PersonIDs
	}
	if t.Recurrence != nil {
		m["recurrence"] = *t.Recurrence
	}
	if len(t.Tags) > 0 {
		m["tags"] = t.Tags
	}
	if t.DealID != nil {
		m["deal_id"] = *t.DealID
	}
	if t.DueAt != nil {
		m["due_at"] = *t.DueAt
	}
	if t.CompletedAt != nil {
		m["completed_at"] = *t.CompletedAt
	}
	if t.DurationMin != nil {
		m["duration_min"] = *t.DurationMin
	}
	if t.EarliestStart != nil {
		m["earliest_start"] = *t.EarliestStart
	}
	if t.DeadlineAt != nil {
		m["deadline_at"] = *t.DeadlineAt
	}
	if t.MinChunkMin != nil {
		m["min_chunk_min"] = *t.MinChunkMin
	}
	if t.ScheduledStart != nil {
		m["scheduled_start"] = *t.ScheduledStart
	}
	m["deadline_hard"] = t.DeadlineHard
	m["auto_schedule"] = t.AutoSchedule
	m["scheduled_locked"] = t.ScheduledLocked
	m["schedule_name"] = t.ScheduleName
	return m
}

func tasksToMaps(tasks []*model.Task) []map[string]any {
	result := make([]map[string]any, len(tasks))
	for i, t := range tasks {
		result[i] = taskToMap(t)
	}
	return result
}

func registerTaskCommands(rootCmd *cobra.Command) {
	taskCmd := &cobra.Command{
		Use:   "task",
		Short: "Manage tasks and follow-ups",
	}

	taskCmd.AddCommand(taskAddCmd())
	taskCmd.AddCommand(taskListCmd())
	taskCmd.AddCommand(taskShowCmd())
	taskCmd.AddCommand(taskEditCmd())
	taskCmd.AddCommand(taskDoneCmd())
	taskCmd.AddCommand(taskDeleteCmd())
	taskCmd.AddCommand(taskTagCmd(false))
	taskCmd.AddCommand(taskTagCmd(true))

	rootCmd.AddCommand(taskCmd)
}

func taskAddCmd() *cobra.Command {
	var description, due, priority, recurrence string
	var personIDs []int64
	var dealID int64
	var dryRun, noSyntax bool
	var tagFlags []string
	var sf *schedFlags

	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Add a new task",
		Long: "Add a new task. The title can carry short syntax: #tag, fri 3pm (due), by mon (deadline), ~30m or for 30m (duration), !high or !!! (priority), every 2 weeks (repeat), @Name (link a person).\n" +
			"Flags win over the same value in the title. Use --dry-run to see what was read, --no-syntax to keep the title as typed. See 'crm help short-syntax'.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			if !model.ValidPriority(priority) {
				return model.NewExitError(model.ErrValidation, "invalid priority: %s (must be one of: %s)", priority, strings.Join(model.Priorities, ", "))
			}

			in, err := parseInline(cmd.Context(), db, args[0], noSyntax, true, shortsyntax.KindCadence)
			if err != nil {
				return err
			}
			if in.Res.Title == "" {
				return model.NewExitError(model.ErrValidation, "the title is empty once the short syntax is removed")
			}
			tf := quick.TaskFieldsOf(in.Res, clockNow().Location())
			input := model.CreateTaskInput{
				Tags:        append(append([]string(nil), tagFlags...), in.Res.Tags...),
				Title:       in.Res.Title,
				Description: nilIfEmpty(description),
				Priority:    priority,
				DueAt:       nilIfEmpty(due),
			}
			if !cmd.Flags().Changed("description") {
				input.Description = nilIfEmpty(tf.Description)
			}
			if !cmd.Flags().Changed("due") {
				input.DueAt = nilIfEmpty(tf.Due)
			}
			if !cmd.Flags().Changed("priority") {
				input.Priority = tf.Priority
			}
			ids := personIDs
			if !cmd.Flags().Changed("person") {
				if ids, err = in.people(); err != nil {
					return err
				}
			}
			if len(ids) > 0 {
				input.PersonID = &ids[0]
				input.PersonIDs = ids[1:]
			}
			if cmd.Flags().Changed("deal") {
				input.DealID = &dealID
			}
			input.Recurrence = nilIfEmpty(recurrence)
			if !cmd.Flags().Changed("recurrence") {
				input.Recurrence = nilIfEmpty(tf.Recurrence)
			}
			if input.Schedule, err = sf.build(cmd); err != nil {
				return err
			}
			// ~30m and by fri fill the duration and the deadline unless a flag already did.
			if input.Schedule.DurationMin == nil {
				input.Schedule.DurationMin = tf.Schedule.DurationMin
			}
			if input.Schedule.DeadlineAt == nil {
				input.Schedule.DeadlineAt = tf.Schedule.DeadlineAt
			}
			if in.Res.Recurrence != "" && tf.Recurrence == "" && !cmd.Flags().Changed("recurrence") {
				fmt.Fprintf(os.Stderr, "crm: %q is not an interval, so it is kept in the description\n", in.Res.Recurrence)
			}

			if dryRun {
				return printInlineDryRun(map[string]any{
					"dry_run": true, "syntax": in.Res,
					"task": map[string]any{
						"title": input.Title, "description": input.Description, "due_at": input.DueAt, "priority": input.Priority,
						"person_ids": ids, "deal_id": input.DealID, "recurrence": input.Recurrence, "tags": input.Tags,
						"duration_min": input.Schedule.DurationMin, "deadline_at": input.Schedule.DeadlineAt,
					},
				})
			}

			r := repo.NewTaskRepo(db)
			task, err := r.Create(cmd.Context(), input)
			if err != nil {
				return err
			}

			data := []map[string]any{taskToMap(task)}
			return format.Output(os.Stdout, resolveFormat(), data, taskColumns, flagQuiet)
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "task description")
	cmd.Flags().StringVar(&due, "due", "", "due date (ISO 8601)")
	cmd.Flags().StringVar(&priority, "priority", "medium", "priority: "+strings.Join(model.Priorities, ", "))
	cmd.Flags().Int64SliceVar(&personIDs, "person", nil, "associated person ID (repeatable)")
	cmd.Flags().Int64Var(&dealID, "deal", 0, "associated deal ID")
	cmd.Flags().StringVar(&recurrence, "recurrence", "", "repeat after completion, e.g. \"7 days\", \"1 mon\", weekly (needs --due)")
	cmd.Flags().StringSliceVar(&tagFlags, "tag", nil, "tag the task (repeatable); #tag in the title does the same")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what was read from the title and what would be created, and write nothing")
	cmd.Flags().BoolVar(&noSyntax, "no-syntax", false, "keep the title as typed: do not read #, @, !, ~ or date words")

	sf = bindScheduleFlags(cmd)
	return cmd
}

func taskListCmd() *cobra.Command {
	var personID, dealID int64
	var overdue, all bool
	var tag string
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			filters := model.TaskFilters{
				Limit:            limit,
				Tag:              nilIfEmpty(tag),
				Overdue:          overdue,
				IncludeCompleted: all,
			}
			if cmd.Flags().Changed("person") {
				filters.PersonID = &personID
			}
			if cmd.Flags().Changed("deal") {
				filters.DealID = &dealID
			}

			r := repo.NewTaskRepo(db)
			tasks, err := r.FindAll(cmd.Context(), filters)
			if err != nil {
				return err
			}

			return format.Output(os.Stdout, resolveFormat(), tasksToMaps(tasks), taskColumns, flagQuiet)
		},
	}

	cmd.Flags().Int64Var(&personID, "person", 0, "filter by person ID")
	cmd.Flags().Int64Var(&dealID, "deal", 0, "filter by deal ID")
	cmd.Flags().StringVar(&tag, "tag", "", "only tasks with this tag")
	cmd.Flags().BoolVar(&overdue, "overdue", false, "show only overdue tasks")
	cmd.Flags().BoolVar(&all, "all", false, "include completed tasks")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results")

	return cmd
}

func taskShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show task details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return model.NewExitError(model.ErrValidation, "invalid task ID: %s", args[0])
			}

			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			r := repo.NewTaskRepo(db)
			task, err := r.FindByID(cmd.Context(), id)
			if err != nil {
				return err
			}

			data := []map[string]any{taskToMap(task)}
			return format.Output(os.Stdout, resolveFormat(), data, taskColumns, flagQuiet)
		},
	}
}

func taskEditCmd() *cobra.Command {
	var title, description, due, priority, recurrence string
	var personID, dealID int64
	var sf *schedFlags

	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Edit a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return model.NewExitError(model.ErrValidation, "invalid task ID: %s", args[0])
			}

			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			input := model.UpdateTaskInput{}
			if cmd.Flags().Changed("title") {
				input.Title = &title
			}
			if cmd.Flags().Changed("description") {
				input.Description = &description
			}
			if cmd.Flags().Changed("due") {
				input.DueAt = &due
			}
			if cmd.Flags().Changed("priority") {
				if !model.ValidPriority(priority) {
					return model.NewExitError(model.ErrValidation, "invalid priority: %s (must be one of: %s)", priority, strings.Join(model.Priorities, ", "))
				}
				input.Priority = &priority
			}
			if cmd.Flags().Changed("person") {
				input.PersonID = &personID
			}
			if cmd.Flags().Changed("deal") {
				input.DealID = &dealID
			}
			if cmd.Flags().Changed("recurrence") {
				input.Recurrence = &recurrence
			}
			if input.Schedule, err = sf.build(cmd); err != nil {
				return err
			}

			r := repo.NewTaskRepo(db)
			task, err := r.Update(cmd.Context(), id, input)
			if err != nil {
				return err
			}

			data := []map[string]any{taskToMap(task)}
			return format.Output(os.Stdout, resolveFormat(), data, taskColumns, flagQuiet)
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "task title")
	cmd.Flags().StringVar(&description, "description", "", "task description")
	cmd.Flags().StringVar(&due, "due", "", "due date (ISO 8601)")
	cmd.Flags().StringVar(&priority, "priority", "", "priority: "+strings.Join(model.Priorities, ", "))
	cmd.Flags().Int64Var(&personID, "person", 0, "add an associated person ID")
	cmd.Flags().Int64Var(&dealID, "deal", 0, "associated deal ID")
	cmd.Flags().StringVar(&recurrence, "recurrence", "", "repeat after completion (empty clears)")
	sf = bindScheduleFlags(cmd)

	return cmd
}

func taskDoneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "done <id>",
		Short: "Mark a task as completed",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return model.NewExitError(model.ErrValidation, "invalid task ID: %s", args[0])
			}

			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			r := repo.NewTaskRepo(db)
			task, err := r.Complete(cmd.Context(), id)
			if err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Completed task #%d: %s\n", task.ID, task.Title)
			if task.NextTaskID != nil {
				fmt.Fprintf(os.Stderr, "Next occurrence: task #%d\n", *task.NextTaskID)
			}
			return nil
		},
	}
}

func taskDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a task (soft-delete)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return model.NewExitError(model.ErrValidation, "invalid task ID: %s", args[0])
			}

			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			r := repo.NewTaskRepo(db)
			if err := r.Archive(cmd.Context(), id); err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Deleted task #%d\n", id)
			return nil
		},
	}
}

// taskTagCmd is "task tag" (add tags) or "task untag" (remove tags).
func taskTagCmd(remove bool) *cobra.Command {
	use, short := "tag <id> <tag>...", "Add tags to a task"
	if remove {
		use, short = "untag <id> <tag>...", "Remove tags from a task"
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return model.NewExitError(model.ErrValidation, "invalid task ID: %s", args[0])
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			tr := repo.NewTagRepo(db)
			for _, name := range args[1:] {
				name = strings.TrimLeft(name, "#")
				if name == "" {
					continue
				}
				if remove {
					err = tr.Remove(cmd.Context(), "task", id, name)
				} else {
					err = tr.Apply(cmd.Context(), "task", id, name)
				}
				if err != nil {
					return err
				}
			}
			task, err := repo.NewTaskRepo(db).FindByID(cmd.Context(), id)
			if err != nil {
				return err
			}
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{taskToMap(task)}, taskColumns, flagQuiet)
		},
	}
}