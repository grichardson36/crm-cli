package quick

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/shortsyntax"
)

func formatMinutes(n int) string { return shortsyntax.FormatMinutes(n) }

// TaskFields are the task columns a parse result sets. Tasks have no deadline or estimate column
// yet, so a deadline becomes the due date when there is no @date, and otherwise (like the
// estimate) goes into the description.
type TaskFields struct {
	Due         string
	Priority    string
	Description string
}

// TaskFieldsOf maps a parse result to task columns. Priority defaults to medium.
func TaskFieldsOf(res shortsyntax.Result) TaskFields {
	tf := TaskFields{Priority: res.TaskPriority()}
	if tf.Priority == "" {
		tf.Priority = "medium"
	}
	var desc []string
	switch {
	case res.Due != nil:
		tf.Due = res.Due.String()
		if res.Deadline != nil {
			desc = append(desc, "Deadline: "+res.Deadline.String())
		}
	case res.Deadline != nil:
		tf.Due = res.Deadline.String()
	}
	if res.DurationMin > 0 {
		desc = append(desc, "Estimate: "+formatMinutes(res.DurationMin))
	}
	tf.Description = strings.Join(desc, "\n")
	return tf
}

// OccurredAt is the stored time of a logged interaction for an @date: the date with its time, or noon.
func OccurredAt(w shortsyntax.When) string { return occurredAt(w) }

// LoadEnv reads the people, tags and note types from the database into an Env.
func LoadEnv(ctx context.Context, db *sql.DB, now time.Time, contextPerson int64) (Env, error) {
	pr := repo.NewPersonRepo(db)
	people, err := pr.ListSummaries(ctx)
	if err != nil {
		return Env{}, err
	}
	tags, err := repo.NewTagRepo(db).FindAll(ctx)
	if err != nil {
		return Env{}, err
	}
	names := make([]string, 0, len(tags))
	for _, t := range tags {
		names = append(names, t.Name)
	}
	types, err := repo.NewNoteTypeRepo(db).List(ctx, false)
	if err != nil {
		return Env{}, err
	}
	typeNames := make([]string, 0, len(types))
	for _, t := range types {
		typeNames = append(typeNames, t.Name)
	}
	return Env{
		Now: now, Index: NewIndex(people), Tags: names, NoteTypes: typeNames, Context: contextPerson,
		LoadPerson: func(id int64) (*model.Person, error) {
			p, err := pr.FindByID(ctx, id)
			if err != nil {
				return nil, err
			}
			return p, pr.LoadDetails(ctx, p)
		},
	}, nil
}

// Output is the JSON shape printed by crm do and returned by the crm_do tool.
type Output struct {
	Input        string              `json:"input"`
	Verb         string              `json:"verb,omitempty"`
	DryRun       bool                `json:"dry_run"`
	Applied      bool                `json:"applied"`
	Preview      string              `json:"preview"`
	Subject      *Person             `json:"subject,omitempty"`
	Links        []Person            `json:"links,omitempty"`
	Changes      []Change            `json:"changes"`
	Warnings     []string            `json:"warnings,omitempty"`
	Problems     []Problem           `json:"problems,omitempty"`
	NeedsConfirm bool                `json:"needs_confirm,omitempty"`
	Syntax       *shortsyntax.Result `json:"syntax,omitempty"`
}

// OutputOf describes a plan, and the result if it was applied.
func OutputOf(p *Plan, res *Result) Output {
	o := Output{
		Input: p.Input, Verb: p.Verb, DryRun: res == nil, Preview: p.Preview, Subject: p.Subject, Links: p.Links,
		Changes: p.Changes, Warnings: p.Warnings, Problems: p.Problems, NeedsConfirm: p.NeedsConfirm, Syntax: p.Syntax,
	}
	if res != nil {
		o.Applied = res.Applied
		o.Changes = res.Changes
	}
	if o.Changes == nil {
		o.Changes = []Change{}
	}
	return o
}

// NewRepos builds the repositories Apply needs over one database.
func NewRepos(db *sql.DB) Repos {
	return Repos{
		People:       repo.NewPersonRepo(db),
		Interactions: repo.NewInteractionRepo(db),
		Tasks:        repo.NewTaskRepo(db),
		Tags:         repo.NewTagRepo(db),
	}
}

// Apply runs a plan. If a step fails, the steps already done are undone and the error says so.
func Apply(ctx context.Context, r Repos, p *Plan) (*Result, error) {
	if len(p.Problems) > 0 {
		return nil, fmt.Errorf("%s: %w", p.Problems[0].Message, errorFor(p.Problems[0].Code))
	}
	if len(p.ops) == 0 {
		return nil, fmt.Errorf("nothing to do: %w", model.ErrValidation)
	}
	st := &runState{}
	res := &Result{Verb: p.Verb, Warnings: p.Warnings, Changes: []Change{}}
	for _, o := range p.ops {
		changes, err := o.run(ctx, r, st)
		if err != nil {
			if len(res.Changes) > 0 {
				if uerr := Undo(ctx, r, res.Changes); uerr != nil {
					return nil, fmt.Errorf("%w (and the undo failed: %v)", err, uerr)
				}
				return nil, fmt.Errorf("%w (earlier steps were undone)", err)
			}
			return nil, err
		}
		res.Changes = append(res.Changes, changes...)
	}
	res.Applied = true
	return res, nil
}

func errorFor(code string) error {
	switch code {
	case "not_found":
		return model.ErrNotFound
	default:
		return model.ErrValidation
	}
}

// ErrorFor maps a problem code to a sentinel error (not_found is exit 3, the rest are usage errors).
func ErrorFor(code string) error { return errorFor(code) }

// Undo reverses changes, newest first. Created notes and tasks are archived, tags are removed or
// re-applied, and updated fields get their old value back.
func Undo(ctx context.Context, r Repos, changes []Change) error {
	var firstErr error
	keep := func(err error) {
		if err != nil && !errors.Is(err, model.ErrNotFound) && firstErr == nil {
			firstErr = err
		}
	}
	// Person fields are restored together, one update per person.
	updates := map[int64]*model.UpdatePersonInput{}
	var order []int64
	for i := len(changes) - 1; i >= 0; i-- {
		c := changes[i]
		switch {
		case c.Op == "create" && c.Entity == "interaction":
			keep(r.Interactions.Archive(ctx, c.ID))
		case c.Op == "create" && c.Entity == "task":
			keep(r.Tasks.Archive(ctx, c.ID))
		case c.Op == "tag":
			keep(r.Tags.Remove(ctx, c.Entity, c.ID, c.New))
		case c.Op == "untag":
			keep(r.Tags.Apply(ctx, c.Entity, c.ID, c.New))
		case c.Op == "update" && c.Entity == "person":
			if kind, ok := handleKey(c.Field); ok {
				if c.Old == "" {
					keep(r.People.RemoveHandle(ctx, c.ID, kind))
				} else {
					keep(r.People.SetHandle(ctx, c.ID, kind, c.Old))
				}
				continue
			}
			def, ok := lookupField(c.Field)
			if !ok {
				continue
			}
			in := updates[c.ID]
			if in == nil {
				in = &model.UpdatePersonInput{}
				updates[c.ID] = in
				order = append(order, c.ID)
			}
			def.set(in, c.Old)
		}
	}
	for _, id := range order {
		_, err := r.People.Update(ctx, id, *updates[id])
		keep(err)
	}
	return firstErr
}

// Describe writes a change as one human line.
func (c Change) Describe() string {
	switch c.Op {
	case "create":
		return fmt.Sprintf("created %s: %s", c.Entity, c.Label)
	case "tag":
		return fmt.Sprintf("tagged %s %s #%s", c.Entity, labelOrID(c), c.New)
	case "untag":
		return fmt.Sprintf("removed #%s from %s %s", c.New, c.Entity, labelOrID(c))
	case "update":
		return fmt.Sprintf("%s %s: %s -> %s", c.Label, c.Field, orEmpty(c.Old), orEmpty(c.New))
	}
	return strings.TrimSpace(c.Op + " " + c.Entity)
}

func labelOrID(c Change) string {
	if c.Label != "" {
		return c.Label
	}
	return fmt.Sprintf("#%d", c.ID)
}
