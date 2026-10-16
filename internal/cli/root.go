package cli

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

var (
	flagFormat  string
	flagQuiet   bool
	flagVerbose bool
	flagDB      string
	flagNoColor bool
)

// Version is set by ldflags at build time.
var Version = "dev"

// NewRootCmd creates the root cobra command.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "crm",
		Short:         "Local-first personal CRM for the terminal",
		Long:          "A local-first personal CRM for the terminal. Store contacts, organizations, interactions, deals, and tasks in a local SQLite database.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       Version,
	}

	rootCmd.PersistentFlags().StringVarP(&flagFormat, "format", "f", "", "output format: table, json, csv, tsv")
	rootCmd.PersistentFlags().BoolVarP(&flagQuiet, "quiet", "q", false, "minimal output (just IDs)")
	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().StringVar(&flagDB, "db", "", "alternate database path")
	rootCmd.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "disable colors")

	registerPersonCommands(rootCmd)
	registerOrgCommands(rootCmd)
	registerLogCommands(rootCmd)
	registerTagCommands(rootCmd)
	registerDealCommands(rootCmd)
	registerTaskCommands(rootCmd)
	registerDoCommand(rootCmd)
	registerSearchCommand(rootCmd)
	registerContextCommand(rootCmd)
	registerStatusCommand(rootCmd)
	registerMCPCommands(rootCmd)
	registerReminderCommands(rootCmd)
	registerTUICommand(rootCmd)
	registerBackupCommand(rootCmd)
	registerImportCommand(rootCmd)

	// dedupe (dupes, person merge)
	registerDupesCommands(rootCmd)

	// sync runner and inbox
	registerSyncCommands(rootCmd)
	registerDavCommands(rootCmd)

	// link, unlink and restore for every object, built from the object list
	addObjectVerbs(rootCmd)

	// agent surface: help topics, manifest, discovery, setup, health, docs
	help := helpCommand()
	rootCmd.SetHelpCommand(help)
	rootCmd.AddCommand(help)
	registerSchemaCommand(rootCmd)
	registerWhereCommand(rootCmd)
	registerDomainsCommand(rootCmd)
	registerThemeCommands(rootCmd)
	registerKeysCommands(rootCmd)
	registerInitCommand(rootCmd)
	registerDoctorCommand(rootCmd)
	registerAgentCommand(rootCmd)
	registerDocsCommand(rootCmd)

	rootCmd.DisableAutoGenTag = true
	rootCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return model.NewExitError(model.ErrUsage, "%s", err.Error())
	})

	// Must run last: it annotates every command registered above.
	applyMeta(rootCmd)

	return rootCmd
}

// openDB resolves the database path and opens it.
func openDB() (*sql.DB, error) {
	loc, err := resolveDBLocation()
	if err != nil {
		return nil, err
	}
	dbPath := loc.Path

	// Ensure parent directory exists
	dir := filepath.Dir(dbPath)
	if dbPath != ":memory:" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	return db.Open(dbPath)
}

// Sources of the active database path, in priority order.
const (
	dbSourceFlag    = "flag"
	dbSourceEnv     = "env"
	dbSourceDefault = "default"
)

// dbLocation is the resolved database path and how it was chosen.
type dbLocation struct {
	Path   string
	Source string
}

// defaultDataDir is the directory that holds the default database and the other domains.
func defaultDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(home, ".crm"), nil
}

// resolveDBLocation picks the database path: --db, then CRM_DB, then ~/.crm/crm.db.
func resolveDBLocation() (dbLocation, error) {
	if flagDB != "" {
		return dbLocation{Path: flagDB, Source: dbSourceFlag}, nil
	}
	if env := os.Getenv("CRM_DB"); env != "" {
		return dbLocation{Path: env, Source: dbSourceEnv}, nil
	}
	dir, err := defaultDataDir()
	if err != nil {
		return dbLocation{}, err
	}
	return dbLocation{Path: filepath.Join(dir, "crm.db"), Source: dbSourceDefault}, nil
}

// resolveFormat returns the current output format.
func resolveFormat() format.Format {
	return format.Resolve(flagFormat)
}
