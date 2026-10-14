package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/spf13/cobra"
)

// domainName is the domain of a database file: its name without the extension.
func domainName(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// whereInfo describes the active database.
type whereInfo struct {
	Path                string         `json:"path"`
	Source              string         `json:"source"`
	Domain              string         `json:"domain"`
	DataDir             string         `json:"data_dir"`
	Exists              bool           `json:"exists"`
	SchemaVersion       *int           `json:"schema_version"`
	LatestSchemaVersion int            `json:"latest_schema_version"`
	NeedsMigration      bool           `json:"needs_migration"`
	DeviceID            string         `json:"device_id,omitempty"`
	SizeBytes           int64          `json:"size_bytes"`
	Counts              map[string]int `json:"counts,omitempty"`
	Note                string         `json:"note,omitempty"`
}

func collectWhere() (*whereInfo, error) {
	loc, err := resolveDBLocation()
	if err != nil {
		return nil, err
	}
	path := loc.Path
	if abs, err := filepath.Abs(path); err == nil && path != ":memory:" {
		path = abs
	}
	info := &whereInfo{
		Path: path, Source: loc.Source, Domain: domainName(path), DataDir: filepath.Dir(path),
		LatestSchemaVersion: db.LatestVersion(),
	}
	st, err := os.Stat(path)
	if err != nil {
		info.Note = "database does not exist yet: run 'crm init' or any command that writes"
		return info, nil
	}
	info.Exists = true
	info.SizeBytes = st.Size()
	got, err := db.Inspect(path)
	if err != nil {
		info.Note = "cannot open the database read-only: " + err.Error()
		return info, nil
	}
	info.SchemaVersion = &got.SchemaVersion
	info.NeedsMigration = got.SchemaVersion < info.LatestSchemaVersion
	info.DeviceID = got.DeviceID
	if got.IsCRM {
		info.Counts = got.Counts
	} else {
		info.Note = "file is not a CRM database"
	}
	if info.NeedsMigration {
		info.Note = "older schema: the next crm command that opens it migrates it (run 'crm backup' first)"
	}
	return info, nil
}

func registerWhereCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "where",
		Short: "Show the active database, how it was chosen, and its schema and counts",
		Long: "Show which database crm is using, so you never have to guess. Reports the path, whether it came from\n" +
			"--db, CRM_DB or the default, the domain name (file name without .db), schema version, device id,\n" +
			"file size and record counts. Opens the database read-only: it never creates or migrates anything.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			info, err := collectWhere()
			if err != nil {
				return err
			}
			switch resolveFormat() {
			case format.FormatJSON:
				return json.NewEncoder(os.Stdout).Encode(info)
			case format.FormatTable:
				printWhere(info)
				return nil
			default:
				return format.Output(os.Stdout, resolveFormat(), whereRows(info), []format.ColumnDef{
					{Header: "Field", Field: "field"}, {Header: "Value", Field: "value"}}, flagQuiet)
			}
		},
	})
}

func whereRows(i *whereInfo) []map[string]any {
	ver := ""
	if i.SchemaVersion != nil {
		ver = fmt.Sprintf("%d", *i.SchemaVersion)
	}
	rows := []map[string]any{
		{"field": "path", "value": i.Path},
		{"field": "source", "value": i.Source},
		{"field": "domain", "value": i.Domain},
		{"field": "exists", "value": i.Exists},
		{"field": "schema_version", "value": ver},
		{"field": "latest_schema_version", "value": i.LatestSchemaVersion},
		{"field": "device_id", "value": i.DeviceID},
		{"field": "size_bytes", "value": i.SizeBytes},
	}
	keys := make([]string, 0, len(i.Counts))
	for k := range i.Counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		rows = append(rows, map[string]any{"field": "count_" + k, "value": i.Counts[k]})
	}
	return rows
}

func printWhere(i *whereInfo) {
	w := os.Stdout
	fmt.Fprintf(w, "path:            %s\n", i.Path)
	fmt.Fprintf(w, "chosen by:       %s\n", i.Source)
	fmt.Fprintf(w, "domain:          %s\n", i.Domain)
	if !i.Exists {
		fmt.Fprintln(w, "exists:          no")
	} else {
		fmt.Fprintf(w, "schema version:  %d (this build expects %d)\n", *i.SchemaVersion, i.LatestSchemaVersion)
		if i.DeviceID != "" {
			fmt.Fprintf(w, "device id:       %s\n", i.DeviceID)
		}
		fmt.Fprintf(w, "size:            %d bytes\n", i.SizeBytes)
		keys := make([]string, 0, len(i.Counts))
		for k := range i.Counts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(w, "%-16s %d\n", k+":", i.Counts[k])
		}
	}
	if i.Note != "" {
		fmt.Fprintf(os.Stderr, "note: %s\n", i.Note)
	}
}

// domainRow is one database found by `crm domains`.
type domainRow struct {
	Domain        string `json:"domain"`
	Path          string `json:"path"`
	SchemaVersion int    `json:"schema_version"`
	People        int    `json:"people"`
	SizeBytes     int64  `json:"size_bytes"`
	Active        bool   `json:"active"`
}

func registerDomainsCommand(rootCmd *cobra.Command) {
	var dir string
	cmd := &cobra.Command{
		Use:   "domains",
		Short: "List the databases (domains) in the data directory",
		Long: "List every CRM database (*.db) in the data directory, default ~/.crm. Each file is a domain: its name is the\n" +
			"file name without .db. Files that are not CRM databases are skipped. Databases are opened read-only.\n" +
			"Select a domain with --db PATH or the CRM_DB environment variable.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dir == "" {
				d, err := defaultDataDir()
				if err != nil {
					return err
				}
				dir = d
			}
			rows, err := scanDomains(dir)
			if err != nil {
				return err
			}
			data := make([]map[string]any, len(rows))
			for i, r := range rows {
				data[i] = map[string]any{"domain": r.Domain, "path": r.Path, "schema_version": r.SchemaVersion,
					"people": r.People, "size_bytes": r.SizeBytes, "active": r.Active, "id": r.Domain}
			}
			if resolveFormat() == format.FormatJSON && !flagQuiet {
				if rows == nil {
					rows = []domainRow{}
				}
				return json.NewEncoder(os.Stdout).Encode(rows)
			}
			return format.Output(os.Stdout, resolveFormat(), data, []format.ColumnDef{
				{Header: "Domain", Field: "domain"}, {Header: "Path", Field: "path"},
				{Header: "Schema", Field: "schema_version"}, {Header: "People", Field: "people"},
				{Header: "Active", Field: "active"}}, flagQuiet)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "data directory to scan (default ~/.crm)")
	rootCmd.AddCommand(cmd)
}

// scanDomains lists the CRM databases directly inside dir.
func scanDomains(dir string) ([]domainRow, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read data directory %s: %w", dir, err)
	}
	var active os.FileInfo
	if loc, err := resolveDBLocation(); err == nil {
		active, _ = os.Stat(loc.Path)
	}
	var rows []domainRow
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".db") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := db.Inspect(path)
		if err != nil || !info.IsCRM {
			continue
		}
		row := domainRow{Domain: domainName(path), Path: path, SchemaVersion: info.SchemaVersion, People: info.Counts["people"]}
		if st, err := os.Stat(path); err == nil {
			row.SizeBytes = st.Size()
			row.Active = active != nil && os.SameFile(active, st)
		}
		rows = append(rows, row)
	}
	return rows, nil
}
