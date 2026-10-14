package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/importer"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var domainNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// initResult is the output of `crm init`.
type initResult struct {
	Domain        string              `json:"domain"`
	Path          string              `json:"path"`
	Source        string              `json:"source"`
	Created       bool                `json:"created"`
	DryRun        bool                `json:"dry_run"`
	SchemaVersion int                 `json:"schema_version,omitempty"`
	Imported      *importer.DexReport `json:"imported,omitempty"`
	NextSteps     []string            `json:"next_steps"`
}

func registerInitCommand(rootCmd *cobra.Command) {
	var domain, path, dexContacts, dexNotes string
	var yes, dryRun bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create the data directory and a new database (domain)",
		Long: "Create the data directory and a new, empty database. Refuses to overwrite an existing file.\n\n" +
			"Where the database goes, in priority order: --path, then ~/.crm/<domain>.db when --domain is given, then the active\n" +
			"database (--db, CRM_DB, or ~/.crm/crm.db). Without any of these it creates the default ~/.crm/crm.db, which every other\n" +
			"command finds without flags. The domain name is always the file name without .db.\n\n" +
			"Without --yes on a terminal, init asks first. Without --yes and without a terminal it fails.\n" +
			"--import-dex-contacts and --import-dex-notes import a Dex CSV export right after creating the database.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := planInit(domain, path, dexContacts, dexNotes, cmd.Flags().Changed("domain"))
			if err != nil {
				return err
			}
			res.DryRun = dryRun
			if dryRun {
				return printInit(res)
			}
			if !yes {
				if err := confirmInit(res.Path); err != nil {
					return err
				}
			}

			if err := os.MkdirAll(filepath.Dir(res.Path), 0o700); err != nil {
				return fmt.Errorf("create data directory %s: %w", filepath.Dir(res.Path), err)
			}
			d, err := db.Open(res.Path)
			if err != nil {
				return fmt.Errorf("create database: %w: %w", err, model.ErrDatabase)
			}
			defer d.Close()
			res.Created = true
			if v, err := db.SchemaVersion(d); err == nil {
				res.SchemaVersion = v
			}
			if dexContacts != "" || dexNotes != "" {
				rep, err := importer.ImportDex(cmd.Context(), d, dexContacts, dexNotes, importer.DexOptions{})
				if err != nil {
					return fmt.Errorf("database created at %s, but the import failed: %w", res.Path, err)
				}
				res.Imported = rep
			}
			return printInit(res)
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "domain name: creates ~/.crm/<domain>.db (omit for the default database ~/.crm/crm.db)")
	cmd.Flags().StringVar(&path, "path", "", "database file to create (overrides --domain)")
	cmd.Flags().StringVar(&dexContacts, "import-dex-contacts", "", "import this Dex contacts CSV after creating the database")
	cmd.Flags().StringVar(&dexNotes, "import-dex-notes", "", "import this Dex notes CSV after creating the database")
	cmd.Flags().BoolVar(&yes, "yes", false, "do not ask for confirmation")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be created and write nothing")
	rootCmd.AddCommand(cmd)
}

// planInit resolves the target path and validates the inputs. It writes nothing.
func planInit(domain, path, dexContacts, dexNotes string, domainGiven bool) (*initResult, error) {
	res := &initResult{}
	switch {
	case path != "":
		res.Path, res.Source = path, "path"
	case domainGiven:
		if !domainNameRe.MatchString(domain) {
			return nil, model.NewExitError(model.ErrValidation, "invalid domain name %q: use letters, digits, '.', '_' or '-'", domain)
		}
		dir, err := defaultDataDir()
		if err != nil {
			return nil, err
		}
		res.Path, res.Source = filepath.Join(dir, domain+".db"), "domain"
	default:
		loc, err := resolveDBLocation()
		if err != nil {
			return nil, err
		}
		res.Path, res.Source = loc.Path, loc.Source
	}
	if abs, err := filepath.Abs(res.Path); err == nil {
		res.Path = abs
	}
	// The domain is the file name without .db, the same rule `crm where` uses.
	res.Domain = domainName(res.Path)

	if _, err := os.Stat(res.Path); err == nil {
		return nil, model.NewExitError(model.ErrConflict, "a database already exists at %s: init never overwrites. Use it as is (crm where), or choose another --domain or --path", res.Path)
	}
	for _, f := range []string{dexContacts, dexNotes} {
		if f == "" {
			continue
		}
		if _, err := os.Stat(f); err != nil {
			return nil, model.NewExitError(model.ErrNotFound, "import file %s: %v", f, err)
		}
	}
	if dexContacts == "" && dexNotes != "" {
		return nil, model.NewExitError(model.ErrValidation, "--import-dex-notes needs --import-dex-contacts: notes attach to the imported contacts")
	}

	res.NextSteps = []string{"crm help overview", "crm doctor"}
	if res.Source == "path" || res.Source == "domain" {
		res.NextSteps = []string{
			"export CRM_DB=" + res.Path + "   (or pass --db " + res.Path + " to every command)",
			"crm help overview", "crm doctor",
		}
	}
	return res, nil
}

// confirmInit asks on a terminal and fails without one.
func confirmInit(path string) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return model.NewExitError(model.ErrValidation, "init would create %s but needs confirmation: pass --yes (not running in a terminal)", path)
	}
	fmt.Fprintf(os.Stderr, "Create a new database at %s? [y/N] ", path)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
		return model.NewExitError(model.ErrValidation, "cancelled: nothing was created")
	}
	return nil
}

func printInit(res *initResult) error {
	if resolveFormat() == format.FormatJSON {
		return json.NewEncoder(os.Stdout).Encode(res)
	}
	w := os.Stdout
	if res.DryRun {
		fmt.Fprintf(w, "dry run: would create domain %q at %s\n", res.Domain, res.Path)
	} else {
		fmt.Fprintf(w, "Created domain %q at %s (schema version %d)\n", res.Domain, res.Path, res.SchemaVersion)
	}
	if res.Imported != nil {
		fmt.Fprintf(w, "Imported %d contacts and %d notes from Dex\n", res.Imported.ContactsImported, res.Imported.NotesImported)
	}
	fmt.Fprintln(w, "\nNext steps:")
	for i, s := range res.NextSteps {
		fmt.Fprintf(w, "  %d. %s\n", i+1, s)
	}
	return nil
}
