// Package sync runs adapters that pull records from external systems into the CRM.
//
// Rules every adapter run follows:
//   - Records are matched to people through external_ids first, then by exact email, phone or
//     social handle using the dedupe normalization. Names never match on their own.
//   - A record that matches nobody is created only when AllowCreate is set. Otherwise it waits
//     in the sync inbox for accept, attach or ignore. An ambiguous match always waits there.
//   - Nothing is ever deleted. Missing records on the other side change nothing here.
//   - A dry run reports counts and writes nothing, not even the run log or the cursor.
package sync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Adapter pulls changes from one external system.
type Adapter interface {
	// Name is the source name used in external_ids, cursors and the run log.
	Name() string
	// Pull reads changes after since ("" on the first run), hands each record to sink, and
	// returns the cursor to resume from next time.
	Pull(ctx context.Context, since string, sink Sink) (next string, err error)
}

// Outcome says what the sink did with one record.
type Outcome string

// Outcomes.
const (
	Created Outcome = "created"
	Updated Outcome = "updated"
	Skipped Outcome = "skipped"
	Inboxed Outcome = "inbox"
)

// PersonPatch is the person data an adapter knows. Nil and empty fields mean "no opinion".
type PersonPatch struct {
	FirstName *string           `json:"first_name,omitempty"`
	LastName  *string           `json:"last_name,omitempty"`
	Title     *string           `json:"title,omitempty"`
	Company   *string           `json:"company,omitempty"`
	Location  *string           `json:"location,omitempty"`
	Emails    []string          `json:"emails,omitempty"`
	Phones    []string          `json:"phones,omitempty"`
	Handles   map[string]string `json:"handles,omitempty"` // kind -> value
	ETag      string            `json:"etag,omitempty"`
}

// PersonRef points at a person in an incoming interaction.
type PersonRef struct {
	ExternalID string            `json:"external_id,omitempty"`
	Emails     []string          `json:"emails,omitempty"`
	Phones     []string          `json:"phones,omitempty"`
	Handles    map[string]string `json:"handles,omitempty"`
}

// InteractionRecord is an incoming call, message, meeting and so on.
type InteractionRecord struct {
	Type       string      `json:"type"`
	Subject    *string     `json:"subject,omitempty"`
	Content    *string     `json:"content,omitempty"`
	Direction  *string     `json:"direction,omitempty"`
	OccurredAt string      `json:"occurred_at,omitempty"`
	People     []PersonRef `json:"people"`
}

// Sink is what an adapter writes into. It has no delete operation by design.
type Sink interface {
	UpsertPerson(ctx context.Context, source, externalID string, patch PersonPatch) (Outcome, error)
	LogInteraction(ctx context.Context, source, externalID string, rec InteractionRecord) (Outcome, error)
}

// Host is what an adapter that manages more than people and interactions (events, say) can ask
// of its sink: the database, whether this is a dry run, and a way to put its own work in the
// run's counts. The built-in sink implements it, and adapters must work without it.
type Host interface {
	DB() *sql.DB
	IsDryRun() bool
	// Count adds one outcome to the run's counts.
	Count(Outcome)
	// Fail counts one error and keeps its message for the run note.
	Fail(error)
	// Note adds a line to the run note.
	Note(string)
}

// Registry holds the adapters a process can run.
type Registry struct {
	adapters map[string]Adapter
}

// NewRegistry builds a registry from adapters.
func NewRegistry(adapters ...Adapter) *Registry {
	r := &Registry{adapters: map[string]Adapter{}}
	for _, a := range adapters {
		r.adapters[a.Name()] = a
	}
	return r
}

// DefaultRegistry returns the adapters available in this build. There are no network
// adapters yet. The test-only fake adapter is included when CRM_SYNC_FAKE_FILE names a file.
func DefaultRegistry(getenv func(string) string) *Registry {
	var list []Adapter
	if path := getenv("CRM_SYNC_FAKE_FILE"); path != "" {
		list = append(list, &FakeAdapter{Path: path})
	}
	return NewRegistry(list...)
}

// Add registers another adapter.
func (r *Registry) Add(a Adapter) { r.adapters[a.Name()] = a }

// Get returns an adapter by name.
func (r *Registry) Get(name string) (Adapter, error) {
	a, ok := r.adapters[name]
	if !ok {
		return nil, fmt.Errorf("unknown sync source %q: %w", name, model.ErrNotFound)
	}
	return a, nil
}

// Names lists adapter names in order.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.adapters))
	for n := range r.adapters {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// RunOptions tunes one run.
type RunOptions struct {
	DryRun      bool
	AllowCreate bool // create people nobody matches instead of queueing them in the inbox
}

// RunResult is the outcome of one run.
type RunResult struct {
	Source     string `json:"source"`
	DryRun     bool   `json:"dry_run"`
	RunID      int64  `json:"run_id,omitempty"`
	Status     string `json:"status"`
	Created    int    `json:"created"`
	Updated    int    `json:"updated"`
	Skipped    int    `json:"skipped"`
	Inbox      int    `json:"inbox"`
	Errors     int    `json:"errors"`
	Since      string `json:"since"`
	NextCursor string `json:"next_cursor"`
	Note       string `json:"note,omitempty"`
}

// Runner executes adapters against a database.
type Runner struct {
	db       *sql.DB
	registry *Registry
}

// NewRunner creates a Runner.
func NewRunner(db *sql.DB, registry *Registry) *Runner {
	return &Runner{db: db, registry: registry}
}

// Run pulls one source. Setup problems return an error. A failed pull returns a result with
// Status "failed" and a nil error, so callers can still show the counts.
func (r *Runner) Run(ctx context.Context, source string, opts RunOptions) (*RunResult, error) {
	adapter, err := r.registry.Get(source)
	if err != nil {
		return nil, err
	}
	sr := repo.NewSyncRepo(r.db)
	since, err := sr.Cursor(ctx, source)
	if err != nil {
		return nil, err
	}

	res := &RunResult{Source: source, DryRun: opts.DryRun, Since: since}
	var run *model.SyncRun
	if !opts.DryRun {
		if run, err = sr.StartRun(ctx, source); err != nil {
			return nil, err
		}
		res.RunID = run.ID
	}

	sink := newSink(r.db, opts)
	next, pullErr := adapter.Pull(ctx, since, sink)

	res.Created, res.Updated, res.Skipped, res.Inbox, res.Errors = sink.created, sink.updated, sink.skipped, sink.inboxed, sink.errs
	switch {
	case pullErr != nil:
		res.Status = model.SyncFailed
		res.Note = pullErr.Error()
	case sink.errs > 0:
		res.Status = model.SyncPartial
		res.Note = sink.lastErr
		res.NextCursor = since // keep the cursor so failed records are retried
	default:
		res.Status = model.SyncOK
		res.NextCursor = next
	}
	if res.Status == model.SyncFailed {
		res.NextCursor = since
	}
	for _, n := range sink.notes {
		if res.Note != "" {
			res.Note += "; "
		}
		res.Note += n
	}
	if res.Inbox > 0 {
		if res.Note != "" {
			res.Note += "; "
		}
		res.Note += fmt.Sprintf("%d in inbox", res.Inbox)
	}

	if opts.DryRun {
		return res, nil
	}
	// A cancelled context must not stop the run from being closed out.
	fin := context.WithoutCancel(ctx)
	if err := sr.FinishRun(fin, run.ID, res.Status, res.Created, res.Updated, res.Skipped+res.Inbox, res.Errors, res.Note); err != nil {
		return res, err
	}
	if res.Status == model.SyncOK && next != "" && next != since {
		if err := sr.SetCursor(fin, source, next); err != nil {
			return res, err
		}
	}
	return res, nil
}

// Status is a source's last run and cursor, for display.
type Status struct {
	Source     string         `json:"source"`
	Registered bool           `json:"registered"`
	Cursor     string         `json:"cursor"`
	LastRun    *model.SyncRun `json:"last_run"`
}

// Statuses lists every registered source and every source that has history.
func (r *Runner) Statuses(ctx context.Context) ([]Status, error) {
	sr := repo.NewSyncRepo(r.db)
	cursors, err := sr.Cursors(ctx)
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for _, n := range r.registry.Names() {
		names[n] = true
	}
	for _, c := range cursors {
		names[c.Source] = true
	}
	runs, err := sr.ListRuns(ctx, "", 200)
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		names[run.Source] = true
	}
	var out []Status
	for n := range names {
		st := Status{Source: n}
		_, err := r.registry.Get(n)
		st.Registered = !errors.Is(err, model.ErrNotFound)
		if st.Cursor, err = sr.Cursor(ctx, n); err != nil {
			return nil, err
		}
		if st.LastRun, err = sr.LastRun(ctx, n); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Source < out[j].Source })
	return out, nil
}
