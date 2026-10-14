package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/dav"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

func registerDavCommands(rootCmd *cobra.Command) {
	davCmd := &cobra.Command{
		Use:   "dav",
		Short: "Two-way contact sync with phones over CardDAV",
		Long: "Serve the active people of this database as a CardDAV address book that a phone (DAVx5 on Android, iOS, macOS) syncs with, " +
			"in both directions. Edits to the same field on both sides resolve to the newest change and are logged.",
	}
	davCmd.AddCommand(davServeCmd(), davInfoCmd(), davConflictsCmd(), davStatusCmd())
	rootCmd.AddCommand(davCmd)
}

// withDavDB opens the active database and runs fn.
func withDavDB(fn func(db *sql.DB) error) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	return fn(db)
}

func davUserFlag(flag string) (string, error) {
	user := flag
	if user == "" {
		user = os.Getenv("CRM_DAV_USER")
	}
	if user == "" {
		user = dav.DefaultUser
	}
	if !dav.ValidUser(user) {
		return "", model.NewExitError(model.ErrValidation, "invalid --user %q: use letters, digits and . _ @ - (up to 64)", user)
	}
	return user, nil
}

func davBaseURL(flag string) string {
	if flag != "" {
		return flag
	}
	return os.Getenv("CRM_DAV_BASE_URL")
}

// --- serve -------------------------------------------------------------------------------

func davServeCmd() *cobra.Command {
	var addr, user, passwordEnv, passwordFile, baseURL string
	var readOnly, allowRemote bool
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the CardDAV server for this database",
		Long: "Run the CardDAV server for the active database. One address book at /dav/USER/contacts/ holds every active person. " +
			"HTTP basic auth is required, so credentials must be supplied. The server speaks plain HTTP: put a TLS-terminating " +
			"reverse proxy in front of it. It listens on loopback only unless --allow-remote is passed. Request lines go to stderr without credentials.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			u, err := davUserFlag(user)
			if err != nil {
				return err
			}
			password, err := davPassword(passwordEnv, passwordFile)
			if err != nil {
				return err
			}
			if !dav.IsLoopback(addr) && !allowRemote {
				return model.NewExitError(model.ErrValidation,
					"refusing to listen on %s: it is not a loopback address. Bind 127.0.0.1 and use a reverse proxy, or pass --allow-remote", addr)
			}
			return withDavDB(func(db *sql.DB) error {
				return runDavServer(cmd.Context(), db, addr, u, password, davBaseURL(baseURL), readOnly)
			})
		},
	}
	cmd.Flags().StringVar(&addr, "addr", dav.DefaultAddr, "address to listen on")
	cmd.Flags().StringVar(&user, "user", "", "account user name (default $CRM_DAV_USER or crm)")
	cmd.Flags().StringVar(&passwordEnv, "password-env", "", "name of the environment variable that holds the password")
	cmd.Flags().StringVar(&passwordFile, "password-file", "", "file that holds the password")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "reject PUT and DELETE with 403 and tell phones the book is read-only")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "public URL phones use (printed in the start-up lines, default $CRM_DAV_BASE_URL)")
	cmd.Flags().BoolVar(&allowRemote, "allow-remote", false, "allow listening on a non-loopback address")
	return cmd
}

// davPassword reads the password from the named environment variable or file. It never takes
// one on the command line, which would show in process lists.
func davPassword(envName, file string) (string, error) {
	switch {
	case envName != "" && file != "":
		return "", model.NewExitError(model.ErrUsage, "use only one of --password-env and --password-file")
	case envName != "":
		v := os.Getenv(envName)
		if v == "" {
			return "", model.NewExitError(model.ErrValidation, "environment variable %s is empty or not set", envName)
		}
		return v, nil
	case file != "":
		b, err := os.ReadFile(file)
		if err != nil {
			return "", model.NewExitError(model.ErrValidation, "cannot read --password-file: %v", err)
		}
		v := strings.TrimRight(string(b), "\r\n")
		if v == "" {
			return "", model.NewExitError(model.ErrValidation, "password file %s is empty", file)
		}
		return v, nil
	}
	return "", model.NewExitError(model.ErrValidation,
		"refusing to start without credentials: pass --password-env VAR or --password-file FILE")
}

func runDavServer(ctx context.Context, db *sql.DB, addr, user, password, baseURL string, readOnly bool) error {
	svc := dav.NewService(db, readOnly)
	logger := log.New(os.Stderr, "", log.LstdFlags)
	bookName := dav.BookNameFromDB(ctx, db)
	handler := dav.NewHandler(dav.Options{Service: svc, User: user, Password: password, BookName: bookName, Logger: logger})

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	if baseURL == "" {
		baseURL = "http://" + ln.Addr().String()
	}
	info := dav.BuildInfo(baseURL, user, bookName)
	mode := "read-write"
	if readOnly {
		mode = "read-only"
	}
	fmt.Fprintf(os.Stderr, "crm dav: listening on http://%s (%s, plain HTTP: put TLS in front)\n", ln.Addr(), mode)
	fmt.Fprintf(os.Stderr, "crm dav: address book %q at %s\n", bookName, info.AddressBookURL)

	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
		fmt.Fprintln(os.Stderr, "crm dav: stopped")
		return nil
	}
}

// --- info --------------------------------------------------------------------------------

func davInfoCmd() *cobra.Command {
	var user, baseURL string
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show the server URL, user and step-by-step phone setup",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			u, err := davUserFlag(user)
			if err != nil {
				return err
			}
			loc, err := resolveDBLocation()
			if err != nil {
				return err
			}
			info := dav.BuildInfo(davBaseURL(baseURL), u, dav.BookNameFor(loc.Path))
			if resolveFormat() == format.FormatJSON {
				return json.NewEncoder(os.Stdout).Encode(info)
			}
			printDavInfo(info)
			return nil
		},
	}
	cmd.Flags().StringVar(&user, "user", "", "account user name (default $CRM_DAV_USER or crm)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "public URL phones use (default $CRM_DAV_BASE_URL or the local address)")
	return cmd
}

func printDavInfo(i dav.Info) {
	w := os.Stdout
	fmt.Fprintf(w, "server:        %s\n", i.BaseURL)
	fmt.Fprintf(w, "user:          %s\n", i.User)
	fmt.Fprintf(w, "password:      %s\n", i.Password)
	fmt.Fprintf(w, "address book:  %s\n", i.AddressBook)
	fmt.Fprintf(w, "book URL:      %s\n", i.AddressBookURL)
	fmt.Fprintf(w, "discovery URL: %s\n", i.DiscoveryURL)
	for _, p := range []struct{ key, title string }{{"android", "Android (DAVx5)"}, {"ios", "iOS"}, {"macos", "macOS"}} {
		fmt.Fprintf(w, "\n%s\n", p.title)
		for n, step := range i.Steps[p.key] {
			fmt.Fprintf(w, "  %d. %s\n", n+1, step)
		}
	}
	fmt.Fprintln(w)
	for _, n := range i.Notes {
		fmt.Fprintf(w, "- %s\n", n)
	}
}

// --- status ------------------------------------------------------------------------------

func davStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show how many contacts are exposed, last device activity and conflict counts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withDavDB(func(db *sql.DB) error {
				st, err := repo.NewDavRepo(db).Status(cmd.Context())
				if err != nil {
					return err
				}
				if resolveFormat() == format.FormatJSON {
					return json.NewEncoder(os.Stdout).Encode(st)
				}
				at := func(p *string) string {
					if p == nil {
						return "never"
					}
					return *p + " UTC"
				}
				w := os.Stdout
				fmt.Fprintf(w, "contacts exposed:      %d\n", st.ContactsExposed)
				fmt.Fprintf(w, "fetched by a device:   %d\n", st.SyncedToDevice)
				fmt.Fprintf(w, "created from a device: %d\n", st.DeviceCreated)
				fmt.Fprintf(w, "last request:          %s\n", at(st.LastRequestAt))
				fmt.Fprintf(w, "last contact served:   %s\n", at(st.LastGetAt))
				fmt.Fprintf(w, "last device edit:      %s\n", at(st.LastPutAt))
				fmt.Fprintf(w, "conflicts:             %d (%d open)\n", st.Conflicts, st.OpenConflicts)
				return nil
			})
		},
	}
}

// --- conflicts ---------------------------------------------------------------------------

const davConflictFields = "id,uuid,person_id,person_name,field,crm_value,device_value,winner,resolved_at,undone,created_at,updated_at"

var davConflictColumns = []format.ColumnDef{
	{Header: "ID", Field: "id"},
	{Header: "Person", Field: "person_name"},
	{Header: "Field", Field: "field"},
	{Header: "CRM", Field: "crm_value"},
	{Header: "Device", Field: "device_value"},
	{Header: "Winner", Field: "winner"},
	{Header: "Undone", Field: "undone"},
	{Header: "When", Field: "created_at"},
}

func conflictRow(c model.SyncConflict) map[string]any {
	val := func(p *string) any {
		if p == nil {
			return ""
		}
		return *p
	}
	return map[string]any{"id": c.ID, "uuid": c.UUID, "person_id": c.PersonID, "person_name": c.PersonName,
		"field": c.Field, "crm_value": val(c.CRMValue), "device_value": val(c.DeviceValue), "winner": c.Winner,
		"resolved_at": val(c.ResolvedAt), "undone": c.Undone, "created_at": c.CreatedAt, "updated_at": c.UpdatedAt}
}

func parseConflictID(arg string) (int64, error) {
	id, err := strconv.ParseInt(arg, 10, 64)
	if err != nil || id <= 0 {
		return 0, model.NewExitError(model.ErrValidation, "invalid conflict ID: %s", arg)
	}
	return id, nil
}

func davConflictsCmd() *cobra.Command {
	var all bool
	var limit int
	list := &cobra.Command{
		Use:   "list",
		Short: "List fields where the phone and the CRM both changed (newest first)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withDavDB(func(db *sql.DB) error {
				cs, err := repo.NewDavRepo(db).Conflicts(cmd.Context(), all, limit)
				if err != nil {
					return err
				}
				rows := make([]map[string]any, len(cs))
				for i, c := range cs {
					rows[i] = conflictRow(c)
				}
				return format.Output(os.Stdout, resolveFormat(), rows, davConflictColumns, flagQuiet)
			})
		},
	}
	list.Flags().BoolVar(&all, "all", false, "include conflicts already undone")
	list.Flags().IntVar(&limit, "limit", 0, "max results")

	show := &cobra.Command{
		Use:   "show <id>",
		Short: "Show one conflict with both values",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseConflictID(args[0])
			if err != nil {
				return err
			}
			return withDavDB(func(db *sql.DB) error {
				c, err := repo.NewDavRepo(db).Conflict(cmd.Context(), id)
				if err != nil {
					return err
				}
				if resolveFormat() == format.FormatJSON {
					return json.NewEncoder(os.Stdout).Encode(c)
				}
				row := conflictRow(*c)
				for _, col := range davConflictColumns {
					fmt.Fprintf(os.Stdout, "%-8s %v\n", strings.ToLower(col.Header)+":", row[col.Field])
				}
				return nil
			})
		},
	}

	undo := &cobra.Command{
		Use:   "undo <id>",
		Short: "Restore the value that lost the conflict",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseConflictID(args[0])
			if err != nil {
				return err
			}
			return withDavDB(func(db *sql.DB) error {
				c, err := dav.UndoConflict(cmd.Context(), db, id)
				if err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "Conflict #%d undone: %s of person #%d restored\n", c.ID, c.Field, c.PersonID)
				if resolveFormat() == format.FormatJSON {
					return json.NewEncoder(os.Stdout).Encode(c)
				}
				return nil
			})
		},
	}

	group := &cobra.Command{
		Use:   "conflicts",
		Short: "Review and undo sync conflicts",
		Long: "Review fields where the phone and the CRM both changed. The newest change won and the loser is stored here, so " +
			"`undo` can put it back. `crm dav conflicts` alone lists them.",
		Args: cobra.NoArgs,
		RunE: list.RunE,
	}
	group.Flags().AddFlagSet(list.Flags())
	group.AddCommand(list, show, undo)
	return group
}
