package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

var personShowColumns = []format.ColumnDef{
	{Header: "ID", Field: "id"},
	{Header: "Name", Field: "name"},
	{Header: "Emails", Field: "emails_text"},
	{Header: "Phones", Field: "phones_text"},
	{Header: "Handles", Field: "handles_text"},
	{Header: "Company", Field: "company"},
	{Header: "Location", Field: "location"},
	{Header: "Birthday", Field: "birthday"},
	{Header: "Keep in touch", Field: "keep_in_touch"},
	{Header: "Last contacted", Field: "last_contacted_at"},
}

var dueColumns = []format.ColumnDef{
	{Header: "ID", Field: "person_id"},
	{Header: "Name", Field: "name"},
	{Header: "Cadence", Field: "keep_in_touch"},
	{Header: "Last seen", Field: "last_seen"},
	{Header: "Due", Field: "due_at"},
	{Header: "Days overdue", Field: "days_overdue"},
}

var birthdayColumns = []format.ColumnDef{
	{Header: "ID", Field: "person_id"},
	{Header: "Name", Field: "name"},
	{Header: "Date", Field: "date"},
	{Header: "In (days)", Field: "days_until"},
	{Header: "Turning", Field: "turning"},
}

func formatBirthday(p *model.Person) string {
	if p.BirthdayMonth == nil || p.BirthdayDay == nil {
		return ""
	}
	if p.BirthdayYear != nil {
		return fmt.Sprintf("%04d-%02d-%02d", *p.BirthdayYear, *p.BirthdayMonth, *p.BirthdayDay)
	}
	return fmt.Sprintf("--%02d-%02d", *p.BirthdayMonth, *p.BirthdayDay)
}

// addDetailFields adds the keep-in-touch, birthday and contact-method fields of p to m.
func addDetailFields(m map[string]any, p *model.Person) {
	if b := formatBirthday(p); b != "" {
		m["birthday"] = b
	}
	if p.KeepInTouch != nil {
		m["keep_in_touch"] = *p.KeepInTouch
	}
	if p.KeepInTouchSnoozedUntil != nil {
		m["keep_in_touch_snoozed_until"] = *p.KeepInTouchSnoozedUntil
	}
	if p.LastContactedAt != nil {
		m["last_contacted_at"] = *p.LastContactedAt
	}
	if p.IsFavorite {
		m["is_favorite"] = true
	}
	if p.PriorityTier != nil {
		m["priority_tier"] = *p.PriorityTier
	}
	if len(p.Emails) > 0 {
		m["emails"] = p.Emails
		parts := make([]string, len(p.Emails))
		for i, e := range p.Emails {
			parts[i] = e.Email
		}
		m["emails_text"] = strings.Join(parts, ", ")
	}
	if len(p.Phones) > 0 {
		m["phones"] = p.Phones
		parts := make([]string, len(p.Phones))
		for i, ph := range p.Phones {
			parts[i] = ph.PhoneNumber
		}
		m["phones_text"] = strings.Join(parts, ", ")
	}
	if len(p.Handles) > 0 {
		m["handles"] = p.Handles
		parts := make([]string, len(p.Handles))
		for i, h := range p.Handles {
			parts[i] = h.Kind + ":" + h.Value
		}
		m["handles_text"] = strings.Join(parts, ", ")
	}
}

// outputPersonDetail loads a person with details and prints them.
func outputPersonDetail(cmd *cobra.Command, r *repo.PersonRepo, id int64) error {
	person, err := r.FindByID(cmd.Context(), id)
	if err != nil {
		return err
	}
	if err := r.LoadDetails(cmd.Context(), person); err != nil {
		return err
	}
	m := personToMap(person)
	if resolveFormat() == format.FormatJSON {
		for _, k := range []string{"emails_text", "phones_text", "handles_text"} {
			delete(m, k)
		}
	}
	return format.Output(os.Stdout, resolveFormat(), []map[string]any{m}, personShowColumns, flagQuiet)
}

func parsePersonID(arg string) (int64, error) {
	id, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return 0, model.NewExitError(model.ErrValidation, "invalid person ID: %s", arg)
	}
	return id, nil
}

// withPersonRepo opens the database, runs fn, and closes it.
func withPersonRepo(cmd *cobra.Command, fn func(r *repo.PersonRepo) error) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	return fn(repo.NewPersonRepo(db))
}

func registerPersonDetailCommands(personCmd *cobra.Command) {
	personCmd.AddCommand(personEmailCmd())
	personCmd.AddCommand(personPhoneCmd())
	personCmd.AddCommand(personHandleCmd())
	personCmd.AddCommand(personContactedCmd())
}

func personEmailCmd() *cobra.Command {
	emailCmd := &cobra.Command{Use: "email", Short: "Manage a person's email addresses"}

	var label string
	var primary bool
	add := &cobra.Command{
		Use:   "add <id> <email>",
		Short: "Add an email address",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePersonID(args[0])
			if err != nil {
				return err
			}
			return withPersonRepo(cmd, func(r *repo.PersonRepo) error {
				if err := r.AddEmail(cmd.Context(), id, args[1], nilIfEmpty(label), primary); err != nil {
					return err
				}
				return outputPersonDetail(cmd, r, id)
			})
		},
	}
	add.Flags().StringVar(&label, "label", "", "label, e.g. Work or Home")
	add.Flags().BoolVar(&primary, "primary", false, "make this the primary email")

	rm := &cobra.Command{
		Use:   "rm <id> <email>",
		Short: "Remove an email address",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePersonID(args[0])
			if err != nil {
				return err
			}
			return withPersonRepo(cmd, func(r *repo.PersonRepo) error {
				if err := r.RemoveEmail(cmd.Context(), id, args[1]); err != nil {
					return err
				}
				return outputPersonDetail(cmd, r, id)
			})
		},
	}

	emailCmd.AddCommand(add, rm)
	return emailCmd
}

func personPhoneCmd() *cobra.Command {
	phoneCmd := &cobra.Command{Use: "phone", Short: "Manage a person's phone numbers"}

	var label string
	var primary bool
	add := &cobra.Command{
		Use:   "add <id> <phone>",
		Short: "Add a phone number",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePersonID(args[0])
			if err != nil {
				return err
			}
			return withPersonRepo(cmd, func(r *repo.PersonRepo) error {
				if err := r.AddPhone(cmd.Context(), id, args[1], nilIfEmpty(label), primary); err != nil {
					return err
				}
				return outputPersonDetail(cmd, r, id)
			})
		},
	}
	add.Flags().StringVar(&label, "label", "", "label, e.g. Mobile or Work")
	add.Flags().BoolVar(&primary, "primary", false, "make this the primary phone")

	rm := &cobra.Command{
		Use:   "rm <id> <phone>",
		Short: "Remove a phone number",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePersonID(args[0])
			if err != nil {
				return err
			}
			return withPersonRepo(cmd, func(r *repo.PersonRepo) error {
				if err := r.RemovePhone(cmd.Context(), id, args[1]); err != nil {
					return err
				}
				return outputPersonDetail(cmd, r, id)
			})
		},
	}

	phoneCmd.AddCommand(add, rm)
	return phoneCmd
}

func personHandleCmd() *cobra.Command {
	handleCmd := &cobra.Command{Use: "handle", Short: "Manage a person's social profiles and website"}

	set := &cobra.Command{
		Use:   "set <id> <kind> <value>",
		Short: "Set a handle. Kinds: " + strings.Join(model.HandleKinds, ", "),
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePersonID(args[0])
			if err != nil {
				return err
			}
			return withPersonRepo(cmd, func(r *repo.PersonRepo) error {
				if err := r.SetHandle(cmd.Context(), id, args[1], args[2]); err != nil {
					return err
				}
				return outputPersonDetail(cmd, r, id)
			})
		},
	}

	rm := &cobra.Command{
		Use:   "rm <id> <kind>",
		Short: "Remove a handle",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePersonID(args[0])
			if err != nil {
				return err
			}
			return withPersonRepo(cmd, func(r *repo.PersonRepo) error {
				if err := r.RemoveHandle(cmd.Context(), id, args[1]); err != nil {
					return err
				}
				return outputPersonDetail(cmd, r, id)
			})
		},
	}

	handleCmd.AddCommand(set, rm)
	return handleCmd
}

func personContactedCmd() *cobra.Command {
	var at string
	cmd := &cobra.Command{
		Use:   "contacted <id>",
		Short: "Record that you just reached out, which resets the keep-in-touch clock",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePersonID(args[0])
			if err != nil {
				return err
			}
			when := at
			if when == "" {
				when = time.Now().UTC().Format("2006-01-02 15:04:05")
			}
			empty := ""
			return withPersonRepo(cmd, func(r *repo.PersonRepo) error {
				if _, err := r.Update(cmd.Context(), id, model.UpdatePersonInput{LastContactedAt: &when, SnoozedUntil: &empty}); err != nil {
					return err
				}
				return outputPersonDetail(cmd, r, id)
			})
		},
	}
	cmd.Flags().StringVar(&at, "at", "", "when, as YYYY-MM-DD or YYYY-MM-DD HH:MM:SS (default: now, UTC)")
	return cmd
}

func registerReminderCommands(rootCmd *cobra.Command) {
	var within, limit int
	due := &cobra.Command{
		Use:   "due",
		Short: "List people you are due to reach out to (keep-in-touch)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withPersonRepo(cmd, func(r *repo.PersonRepo) error {
				list, err := r.Due(cmd.Context(), within, limit)
				if err != nil {
					return err
				}
				data := make([]map[string]any, len(list))
				for i, d := range list {
					data[i] = map[string]any{
						"person_id": d.PersonID, "id": d.PersonID, "name": d.Name,
						"keep_in_touch": d.KeepInTouch, "due_at": d.DueAt, "days_overdue": d.DaysOverdue,
					}
					if d.LastSeen != nil {
						data[i]["last_seen"] = *d.LastSeen
					}
				}
				return format.Output(os.Stdout, resolveFormat(), data, dueColumns, flagQuiet)
			})
		},
	}
	due.Flags().IntVar(&within, "within", 0, "also include people due in the next N days")
	due.Flags().IntVar(&limit, "limit", 50, "max results")

	var days int
	birthdays := &cobra.Command{
		Use:   "birthdays",
		Short: "List upcoming birthdays",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withPersonRepo(cmd, func(r *repo.PersonRepo) error {
				list, err := r.Birthdays(cmd.Context(), days, time.Now())
				if err != nil {
					return err
				}
				data := make([]map[string]any, len(list))
				for i, b := range list {
					data[i] = map[string]any{
						"person_id": b.PersonID, "id": b.PersonID, "name": b.Name,
						"date": b.Date, "days_until": b.DaysUntil,
					}
					if b.Turning != nil {
						data[i]["turning"] = *b.Turning
					}
				}
				return format.Output(os.Stdout, resolveFormat(), data, birthdayColumns, flagQuiet)
			})
		},
	}
	birthdays.Flags().IntVar(&days, "days", 30, "look ahead this many days")

	rootCmd.AddCommand(due, birthdays)
}
