package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/modules"
	"github.com/jdanielnd/crm-cli/internal/phone"
	"github.com/spf13/cobra"
)

var phoneChangeColumns = []format.ColumnDef{
	{Header: "Person", Field: "person_id"},
	{Header: "Typed", Field: "typed"},
	{Header: "E.164", Field: "e164"},
	{Header: "Valid", Field: "valid"},
}

// registerPhoneCommands adds `crm phone normalize|region|parse` (patterns/phone-numbers-are-e164).
func registerPhoneCommands(rootCmd *cobra.Command) {
	group := &cobra.Command{
		Use:   "phone",
		Short: "Phone number normalization to E.164 and the default region",
		Long: "Phone numbers are stored as typed and also in E.164 (+14155550100). Matching and duplicate detection use E.164, " +
			"numbers show in national format in your default region and international format elsewhere. The default region is the [phone] region " +
			"setting in ~/.crm/config.toml (US when unset). See 'crm help phone-numbers'.",
	}
	group.AddCommand(phoneNormalizeCmd(), phoneRegionCmd(), phoneParseCmd())
	rootCmd.AddCommand(group)
}

func phoneNormalizeCmd() *cobra.Command {
	var apply, undo bool
	var limit int
	cmd := &cobra.Command{
		Use:   "normalize",
		Short: "Fill the E.164 form of existing phone numbers (dry run unless --apply)",
		Long: "Reads every stored phone number and says how many get an E.164 form, how many are already current, how many cannot be parsed (they keep matching on their last 10 digits) and how many parse but look invalid for the region.\n" +
			"Without --apply nothing is written. With --apply every change is made in one transaction and only the E.164 column is written: the text you typed never changes. Running it again does nothing.\n" +
			"--undo clears the E.164 column of every number, which is the one step back. Typed text is untouched, so nothing is lost.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			people := repo.NewPersonRepo(db)
			if undo {
				n, err := people.ClearPhoneE164(cmd.Context())
				if err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "Cleared the E.164 form of %d phone numbers\n", n)
				return nil
			}
			if limit <= 0 {
				limit = 20
			}
			rep, err := people.NormalizePhones(cmd.Context(), apply, limit)
			if err != nil {
				return err
			}
			if apply {
				fmt.Fprintln(os.Stderr, "Applied")
			} else {
				fmt.Fprintln(os.Stderr, "Dry run: add --apply to write this")
			}
			fmt.Fprintln(os.Stderr, rep.Line())
			if resolveFormat() == format.FormatJSON {
				return json.NewEncoder(os.Stdout).Encode(rep)
			}
			rows := make([]map[string]any, len(rep.Examples))
			for i, c := range rep.Examples {
				rows[i] = map[string]any{"person_id": c.PersonID, "typed": c.Typed, "e164": c.E164, "valid": c.Valid}
			}
			return format.Output(os.Stdout, resolveFormat(), rows, phoneChangeColumns, flagQuiet)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "write the E.164 forms (without it the run is only previewed)")
	cmd.Flags().BoolVar(&undo, "undo", false, "clear every stored E.164 form again")
	cmd.Flags().IntVar(&limit, "limit", 20, "list the first N changes")
	return cmd
}

func phoneRegionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "region [code]",
		Short: "Show or set the default region used for numbers without a country code",
		Long: "Without an argument prints the default region. With a two-letter code (US, GB, DE ...) saves it as [phone] region in ~/.crm/config.toml, per device. " +
			"Run 'crm phone normalize --apply' afterwards so stored E.164 forms follow the new region.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := defaultDataDir()
			if err != nil {
				return err
			}
			if len(args) == 1 {
				if !phone.ValidRegion(args[0]) {
					return model.NewExitError(model.ErrUsage, "%q is not a region code (use two letters such as US, GB, DE)", args[0])
				}
				if err := modules.SetValue(dir, "phone", "region", phone.NormalizeRegion(args[0])); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "Default phone region set to %s\n", phone.NormalizeRegion(args[0]))
				return nil
			}
			applyPhoneConfig()
			fmt.Fprintln(os.Stdout, phone.DefaultRegion())
			return nil
		},
	}
}

func phoneParseCmd() *cobra.Command {
	var region string
	cmd := &cobra.Command{
		Use:   "parse <number>",
		Short: "Show how a number is read: E.164, national and international format, validity",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			applyPhoneConfig()
			n := phone.ParseRegion(args[0], region)
			out := map[string]any{"typed": args[0], "region": phone.DefaultRegionOr(region), "e164": n.E164, "key": n.Key(),
				"national": n.National(), "international": n.International(), "number_region": n.Region,
				"parsed": n.Parsed, "possible": n.Possible, "valid": n.Valid}
			return json.NewEncoder(os.Stdout).Encode(out)
		},
	}
	cmd.Flags().StringVar(&region, "region", "", "region for a number without a country code (default: the configured region)")
	return cmd
}
