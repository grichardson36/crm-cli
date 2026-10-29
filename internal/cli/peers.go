package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/replica/relay"
	"github.com/jdanielnd/crm-cli/internal/replica/replicasql"
	"github.com/spf13/cobra"
)

// DefaultPeersAddr is where `crm peers serve` listens when no address is given.
const DefaultPeersAddr = "127.0.0.1:7478"

func registerPeersCommands(rootCmd *cobra.Command) {
	peers := &cobra.Command{
		Use:   "peers",
		Short: "Sync this database with your other devices",
		Long: "Device-to-device sync of the whole database, one field at a time. Turn it on with `crm peers init`. " +
			"One device runs `crm peers serve` (the relay), the others add it with a one-time token from `crm peers enroll`. " +
			"Traffic is plain HTTP with a token, so keep the relay on Tailscale or a LAN. " +
			"When one device deletes a record that another edited, the record waits in `crm peers review`.",
	}
	peers.AddCommand(peersInitCmd(), peersStatusCmd(), peersServeCmd(), peersEnrollCmd(), peersAddCmd(), peersListCmd(),
		peersRemoveCmd(), peersPullCmd(), peersPushCmd(), peersSyncCmd(), peersRetireCmd(), peersReviewCmd(), peersResolveCmd(), peersPurgeCmd())
	rootCmd.AddCommand(peers)
}

// openReplica opens the active database and its sync adapter.
func openReplica(ctx context.Context) (*sql.DB, *replicasql.Adapter, error) {
	db, err := openDB()
	if err != nil {
		return nil, nil, err
	}
	a, err := replicasql.Open(ctx, db, replicasql.DefaultRegistry())
	if err != nil {
		_ = db.Close()
		if errors.Is(err, replicasql.ErrNotEnabled) {
			return nil, nil, model.NewExitError(model.ErrValidation, "%v", err)
		}
		return nil, nil, err
	}
	return db, a, nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	return enc.Encode(v)
}

func hostName() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "device"
	}
	return strings.ToLower(h)
}

// --- init --------------------------------------------------------------------------------

func peersInitCmd() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Turn on device sync for this database",
		Long: "Gives the database a sync identity, records every current value as a baseline change (rows are not touched) " +
			"and installs the capture triggers. Run it once per device, on the database that holds your data. " +
			"Do not copy a database after this step: the copy would share the identity. Safe to run again.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			res, err := replicasql.Enable(cmd.Context(), db, replicasql.DefaultRegistry())
			if err != nil {
				return err
			}
			if name != "" {
				if _, err := db.ExecContext(cmd.Context(), `UPDATE replica_devices SET name = ? WHERE self = 1`, name); err != nil {
					return err
				}
			}
			if resolveFormat() == format.FormatJSON {
				return printJSON(res)
			}
			fmt.Fprintf(os.Stderr, "sync is on. device %s, %d tables, %d baseline changes\n", res.Device, res.Tables, res.Baseline)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "name of this device as peers see it")
	return cmd
}

// --- status ------------------------------------------------------------------------------

func peersStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the sync state: device, cursor, peers, review queue, blocked records",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			if !replicasql.IsEnabled(db) {
				out := map[string]any{"enabled": false, "hint": "run `crm peers init`"}
				if resolveFormat() == format.FormatJSON {
					return printJSON(out)
				}
				fmt.Fprintln(os.Stdout, "sync is off. Run `crm peers init`.")
				return nil
			}
			a, err := replicasql.Open(cmd.Context(), db, replicasql.DefaultRegistry())
			if err != nil {
				return err
			}
			st, err := a.Status(cmd.Context())
			if err != nil {
				return err
			}
			devs, err := a.Devices(cmd.Context())
			if err != nil {
				return err
			}
			if resolveFormat() == format.FormatJSON {
				return printJSON(map[string]any{"enabled": true, "status": st, "devices": devs})
			}
			w := os.Stdout
			fmt.Fprintf(w, "device:           %s\n", st.Device)
			fmt.Fprintf(w, "tables synced:    %d\n", len(st.Tables))
			fmt.Fprintf(w, "stored changes:   %d (%d waiting for a newer build)\n", st.Changes, st.Unknown)
			fmt.Fprintf(w, "open reviews:     %d\n", st.OpenReviews)
			fmt.Fprintf(w, "blocked records:  %d\n", len(st.Pending))
			for _, p := range st.Pending {
				fmt.Fprintf(w, "  %s %s: %s\n", p.Collection, p.Key, p.Reason)
			}
			fmt.Fprintf(w, "devices:\n")
			for _, d := range devs {
				fmt.Fprintf(w, "  %-14s %s %s\n", d.Name, d.Device, deviceNote(d))
			}
			return nil
		},
	}
}

func deviceNote(d replicasql.Device) string {
	switch {
	case d.Self:
		return "(this device)"
	case d.RetiredAt != "":
		return "(retired " + d.RetiredAt + ")"
	case d.URL != "":
		return "relay " + d.URL + lastNote(d)
	}
	return "enrolled" + lastNote(d)
}

func lastNote(d replicasql.Device) string {
	switch {
	case d.LastError != "":
		return " last error: " + d.LastError
	case d.LastSeenAt != "":
		return " last seen " + d.LastSeenAt + " UTC"
	}
	return ""
}

// --- serve -------------------------------------------------------------------------------

func peersServeCmd() *cobra.Command {
	var addr string
	var allowPublic, readOnly bool
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the relay: store changes for enrolled devices and serve pulls and pushes",
		Long: "Runs the relay role on this device. Any device can hold it, an always-on one usually does. " +
			"Plain HTTP with a bearer token per device and no encryption, so bind a Tailscale or LAN address. " +
			"It refuses a public or wildcard address unless --allow-public is passed (outside the contract). " +
			"Tombstones are purged on start and every hour.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := relay.CheckBind(addr, allowPublic); err != nil {
				return model.NewExitError(model.ErrValidation, "%v", err)
			}
			db, a, err := openReplica(cmd.Context())
			if err != nil {
				return err
			}
			defer db.Close()
			srv := &relay.Server{A: a, ReadOnly: readOnly, Log: logToStderr()}
			purge := func() {
				if n, err := a.PurgeTombstones(cmd.Context()); err != nil {
					fmt.Fprintf(os.Stderr, "crm peers: purge failed: %v\n", err)
				} else if n > 0 {
					fmt.Fprintf(os.Stderr, "crm peers: purged %d tombstones\n", n)
				}
			}
			purge()
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()
			go func() {
				t := time.NewTicker(time.Hour)
				defer t.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-t.C:
						purge()
					}
				}
			}()
			mode := "read-write"
			if readOnly {
				mode = "read-only"
			}
			return srv.ListenAndServe(ctx, addr, func(bound net.Addr) {
				fmt.Fprintf(os.Stderr, "crm peers: relay for device %s listening on http://%s (%s, plain HTTP, no encryption)\n", a.Device(), bound, mode)
			})
		},
	}
	cmd.Flags().StringVar(&addr, "addr", DefaultPeersAddr, "address to listen on (a Tailscale, LAN or loopback IP and port)")
	cmd.Flags().BoolVar(&allowPublic, "allow-public", false, "allow a public or wildcard address (outside the contract: no encryption)")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "serve pulls only: reject pushes and enrollments with 403")
	return cmd
}

// --- enroll ------------------------------------------------------------------------------

func peersEnrollCmd() *cobra.Command {
	var label string
	var ttl time.Duration
	cmd := &cobra.Command{
		Use:   "enroll",
		Short: "Make a one-time token a new device uses to join this relay",
		Long: "Prints a one-time token. On the new device run `crm peers add NAME URL --token TOKEN`. " +
			"The token works once and expires. Only its hash is stored, so this is the only time it shows.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, a, err := openReplica(cmd.Context())
			if err != nil {
				return err
			}
			defer db.Close()
			tok, err := a.CreateEnrollToken(cmd.Context(), label, ttl)
			if err != nil {
				return err
			}
			out := map[string]any{"token": tok, "label": label, "expires_in": ttl.String(), "device": a.Device()}
			if resolveFormat() == format.FormatJSON {
				return printJSON(out)
			}
			fmt.Fprintln(os.Stdout, tok)
			fmt.Fprintf(os.Stderr, "one-time token, valid %s. On the new device: crm peers add NAME http://THIS-ADDRESS:PORT --token %s\n", ttl, tok)
			return nil
		},
	}
	cmd.Flags().StringVar(&label, "label", "", "note for yourself, for example the device it is for")
	cmd.Flags().DurationVar(&ttl, "ttl", 24*time.Hour, "how long the token stays valid")
	return cmd
}

// --- add / list / remove -----------------------------------------------------------------

func peersAddCmd() *cobra.Command {
	var token, as string
	cmd := &cobra.Command{
		Use:   "add <name> <url>",
		Short: "Join a relay with a one-time token and remember it as a peer",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if token == "" {
				return model.NewExitError(model.ErrValidation, "pass --token with the one-time token from `crm peers enroll` on the relay")
			}
			db, a, err := openReplica(cmd.Context())
			if err != nil {
				return err
			}
			defer db.Close()
			self, err := a.FindDevice(cmd.Context(), a.Device())
			if err != nil {
				return err
			}
			if as == "" {
				as = hostName()
			}
			if as != self.Name {
				if _, err := db.ExecContext(cmd.Context(), `UPDATE replica_devices SET name = ? WHERE self = 1`, as); err != nil {
					return err
				}
			}
			c := relay.NewClient(args[1], "")
			resp, err := c.Enroll(cmd.Context(), relay.EnrollRequest{Token: token, Device: a.Device(), Name: as})
			if err != nil {
				var se *relay.StatusError
				if errors.As(err, &se) && (se.Code == 401 || se.Code == 409) {
					return model.NewExitError(model.ErrConflict, "the relay refused: %s", se.Message)
				}
				return fmt.Errorf("enroll at %s: %w", args[1], err)
			}
			if err := a.AddPeer(cmd.Context(), args[0], args[1], resp.Token, resp.Device); err != nil {
				return model.NewExitError(model.ErrValidation, "%v", err)
			}
			out := map[string]any{"name": args[0], "url": args[1], "device": resp.Device, "as": as}
			if resolveFormat() == format.FormatJSON {
				return printJSON(out)
			}
			fmt.Fprintf(os.Stderr, "joined relay %s (device %s) as %q. Run `crm peers sync`.\n", args[0], resp.Device, as)
			return nil
		},
	}
	cmd.Flags().StringVar(&token, "token", "", "one-time token from `crm peers enroll` on the relay")
	cmd.Flags().StringVar(&as, "as", "", "name of this device at the relay (default the host name)")
	return cmd
}

var peerColumns = []format.ColumnDef{
	{Header: "Name", Field: "name"}, {Header: "Device", Field: "device"}, {Header: "Role", Field: "role"},
	{Header: "URL", Field: "url"}, {Header: "Last pull", Field: "last_pull_at"}, {Header: "Last push", Field: "last_push_at"},
	{Header: "Last error", Field: "last_error"}, {Header: "Retired", Field: "retired_at"},
}

func peersListCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the relays this device connects to and the devices enrolled here",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, a, err := openReplica(cmd.Context())
			if err != nil {
				return err
			}
			defer db.Close()
			devs, err := a.Devices(cmd.Context())
			if err != nil {
				return err
			}
			rows := []map[string]any{}
			for _, d := range devs {
				if d.Self || (d.RetiredAt != "" && !all) {
					continue
				}
				rows = append(rows, map[string]any{"name": d.Name, "device": d.Device, "role": d.Role, "url": d.URL, "last_pull_at": d.LastPullAt,
					"last_push_at": d.LastPushAt, "last_error": d.LastError, "retired_at": d.RetiredAt, "head": d.Head, "cursor": d.Vector})
			}
			return format.Output(os.Stdout, resolveFormat(), rows, peerColumns, flagQuiet)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include retired devices")
	return cmd
}

func peersRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Forget a relay this device connects to",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, a, err := openReplica(cmd.Context())
			if err != nil {
				return err
			}
			defer db.Close()
			if err := a.RemovePeer(cmd.Context(), args[0]); err != nil {
				return model.NewExitError(model.ErrNotFound, "%v", err)
			}
			fmt.Fprintf(os.Stderr, "removed %s\n", args[0])
			return nil
		},
	}
}

// --- pull / push / sync ------------------------------------------------------------------

var reportColumns = []format.ColumnDef{
	{Header: "Peer", Field: "peer"}, {Header: "Pulled", Field: "pulled"}, {Header: "Applied", Field: "applied"},
	{Header: "Unknown", Field: "unknown"}, {Header: "Pushed", Field: "pushed"}, {Header: "Rejected", Field: "rejected"},
	{Header: "Reviews", Field: "open_reviews"},
}

func runExchange(cmd *cobra.Command, args []string, opt relay.Options, all bool) error {
	db, a, err := openReplica(cmd.Context())
	if err != nil {
		return err
	}
	defer db.Close()
	peers, err := a.Peers(cmd.Context())
	if err != nil {
		return err
	}
	var chosen []replicasql.Device
	switch {
	case len(args) == 1:
		d, err := a.FindDevice(cmd.Context(), args[0])
		if err != nil {
			return model.NewExitError(model.ErrNotFound, "%v", err)
		}
		if d.URL == "" {
			return model.NewExitError(model.ErrValidation, "%s is not a relay this device connects to", d.Name)
		}
		chosen = []replicasql.Device{d}
	case len(peers) == 0:
		return model.NewExitError(model.ErrValidation, "no relay added yet: run `crm peers add NAME URL --token TOKEN`")
	default:
		chosen = peers
	}
	rows := []map[string]any{}
	var failed []string
	for _, p := range chosen {
		rep, err := relay.Sync(cmd.Context(), a, p, opt)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", p.Name, err))
			fmt.Fprintf(os.Stderr, "crm peers: %s: %v\n", p.Name, err)
		}
		rows = append(rows, map[string]any{"peer": rep.Peer, "device": rep.Device, "pulled": rep.Pulled, "applied": rep.Applied, "duplicates": rep.Duplicates,
			"unknown": rep.Unknown, "pushed": rep.Pushed, "push_applied": rep.PushApplied, "rejected": len(rep.Rejected), "rejections": rep.Rejected,
			"open_reviews": rep.OpenReviews, "error": errText(err)})
		if rep.OpenReviews > 0 {
			fmt.Fprintf(os.Stderr, "crm peers: %d record(s) wait in the review queue: crm peers review\n", rep.OpenReviews)
		}
	}
	if err := format.Output(os.Stdout, resolveFormat(), rows, reportColumns, flagQuiet); err != nil {
		return err
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d of %d peers failed", len(failed), len(chosen))
	}
	return nil
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func peersPullCmd() *cobra.Command {
	var full bool
	cmd := &cobra.Command{
		Use:   "pull [name]",
		Short: "Get changes from a relay and apply them",
		Long:  "Gets the changes this device has not seen. --full ignores this device's cursor and takes the relay's whole per-field state, which is the snapshot for a reset or a new device. Nothing is applied twice.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExchange(cmd, args, relay.Options{Pull: true, Full: full}, false)
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "pull the relay's whole state, not only what is new")
	return cmd
}

func peersPushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "push [name]",
		Short: "Send the changes a relay does not have",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExchange(cmd, args, relay.Options{Push: true}, false)
		},
	}
}

func peersSyncCmd() *cobra.Command {
	var full bool
	cmd := &cobra.Command{
		Use:   "sync [name]",
		Short: "Pull, then push, with one relay or all of them",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExchange(cmd, args, relay.Options{Pull: true, Push: true, Full: full}, true)
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "pull the relay's whole state first")
	return cmd
}

// --- retire / purge ----------------------------------------------------------------------

func peersRetireCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "retire <device>",
		Short: "Drop a lost or replaced device so it no longer holds back tombstone purging",
		Long:  "Marks the device retired, revokes its tokens and stops waiting for it before deletes are purged. The device can join again with a new enroll token.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, a, err := openReplica(cmd.Context())
			if err != nil {
				return err
			}
			defer db.Close()
			d, err := a.Retire(cmd.Context(), args[0])
			if err != nil {
				return model.NewExitError(model.ErrNotFound, "%v", err)
			}
			fmt.Fprintf(os.Stderr, "retired %s (%s)\n", d.Name, d.Device)
			return nil
		},
	}
}

func peersPurgeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "purge",
		Short: "Remove delete records that every enrolled device has passed",
		Long:  "Run it on the relay, which knows every enrolled device. With no other device enrolled nothing is purged. `crm peers serve` does this on start and every hour.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, a, err := openReplica(cmd.Context())
			if err != nil {
				return err
			}
			defer db.Close()
			n, err := a.PurgeTombstones(cmd.Context())
			if err != nil {
				return err
			}
			if resolveFormat() == format.FormatJSON {
				return printJSON(map[string]any{"purged": n})
			}
			fmt.Fprintf(os.Stderr, "purged %d tombstones\n", n)
			return nil
		},
	}
}

// --- review / resolve --------------------------------------------------------------------

var reviewColumns = []format.ColumnDef{
	{Header: "ID", Field: "id"}, {Header: "Kind", Field: "collection"}, {Header: "Record", Field: "label"},
	{Header: "Status", Field: "status"}, {Header: "Detail", Field: "detail"}, {Header: "Since", Field: "created_at"},
}

func peersReviewCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "review",
		Short: "List records that one device deleted and another edited",
		Long: "Nothing is lost: the record stays visible with the edits while it waits here. " +
			"Decide with `crm peers resolve ID --keep` (restore it on every device) or `--delete` (delete it for good).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, a, err := openReplica(cmd.Context())
			if err != nil {
				return err
			}
			defer db.Close()
			rs, err := a.Reviews(cmd.Context(), all)
			if err != nil {
				return err
			}
			rows := make([]map[string]any, len(rs))
			for i, r := range rs {
				rows[i] = map[string]any{"id": r.ID, "collection": r.Collection, "key": r.Key, "label": r.Label, "status": r.Status, "detail": r.Detail,
					"created_at": r.CreatedAt, "resolved_at": r.ResolvedAt}
			}
			return format.Output(os.Stdout, resolveFormat(), rows, reviewColumns, flagQuiet)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include items already decided")
	return cmd
}

func peersResolveCmd() *cobra.Command {
	var keep, del bool
	cmd := &cobra.Command{
		Use:   "resolve <id>",
		Short: "Decide a review item: keep the record or delete it for good",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if keep == del {
				return model.NewExitError(model.ErrUsage, "pass exactly one of --keep and --delete")
			}
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil || id <= 0 {
				return model.NewExitError(model.ErrValidation, "invalid review ID: %s", args[0])
			}
			db, a, err := openReplica(cmd.Context())
			if err != nil {
				return err
			}
			defer db.Close()
			if err := a.Resolve(cmd.Context(), id, keep); err != nil {
				return model.NewExitError(model.ErrNotFound, "%v", err)
			}
			verb := "deleted"
			if keep {
				verb = "kept"
			}
			fmt.Fprintf(os.Stderr, "review %d: %s. Run `crm peers sync` to send the decision.\n", id, verb)
			return nil
		},
	}
	cmd.Flags().BoolVar(&keep, "keep", false, "keep the record and its edits on every device")
	cmd.Flags().BoolVar(&del, "delete", false, "delete the record for good on every device")
	return cmd
}

func logToStderr() *log.Logger { return log.New(os.Stderr, "crm peers: ", log.LstdFlags) }
