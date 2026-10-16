package quick

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/shortsyntax"
)

func formatMinutes(n int) string { return shortsyntax.FormatMinutes(n) }

// TaskFields are the task columns a parse result sets.
type TaskFields struct {
	Due      string
	Priority string
	// Description holds text that has no column: a repeat such as "every fri" that tasks cannot
	// store as an interval.
	Description string
	// Recurrence is the stored repeat ("2 weeks"), when the phrase is an interval.
	Recurrence string
}

// TaskFieldsOf maps a parse result to task columns. Priority defaults to medium. A deadline
// (by fri) becomes the due date when no other due date is given.
func TaskFieldsOf(res shortsyntax.Result) TaskFields {
	tf := TaskFields{Priority: res.TaskPriority()}
	if tf.Priority == "" {
		tf.Priority = "medium"
	}
	if res.Due != nil {
		tf.Due = res.Due.String()
	}
	if tf.Due == "" && res.Deadline != nil {
		tf.Due = res.Deadline.String()
	}
	if res.Recurrence != "" {
		if rec, ok := RecurrenceOf(res.Recurrence); ok {
			tf.Recurrence = rec
		} else {
			tf.Description = "Repeats: " + res.Recurrence
		}
	}
	return tf
}

// RecurrenceOf turns "every 2 weeks" or "every month" into a task repeat such as "2 weeks" or
// "monthly". Repeats on named days (every fri) are not intervals and return false.
func RecurrenceOf(raw string) (string, bool) {
	words := strings.Fields(strings.ToLower(strings.TrimPrefix(strings.ToLower(raw), "every")))
	if len(words) == 0 {
		return "", false
	}
	if words[0] == "other" && len(words) == 2 {
		words = []string{"2", words[1]}
	}
	unit := func(s string) (string, bool) {
		switch strings.TrimSuffix(s, "s") {
		case "day", "d":
			return "days", true
		case "week", "w":
			return "weeks", true
		case "month", "mo", "m":
			return "mon", true
		case "year", "y":
			return "year", true
		}
		return "", false
	}
	var n int
	var u string
	switch len(words) {
	case 1:
		if v, ok := unit(words[0]); ok {
			return map[string]string{"days": "daily", "weeks": "weekly", "mon": "monthly", "year": "yearly"}[v], true
		}
		// 2w, 3m
		w := words[0]
		k := 0
		for k < len(w) && w[k] >= '0' && w[k] <= '9' {
			k++
		}
		if k == 0 || k == len(w) {
			return "", false
		}
		n, _ = strconv.Atoi(w[:k])
		var ok bool
		if u, ok = unit(w[k:]); !ok {
			return "", false
		}
	case 2:
		var err error
		if n, err = strconv.Atoi(words[0]); err != nil {
			return "", false
		}
		var ok bool
		if u, ok = unit(words[1]); !ok {
			return "", false
		}
	default:
		return "", false
	}
	if n < 1 {
		return "", false
	}
	if u == "year" && n > 1 {
		u = "years"
	}
	if u == "mon" && n > 1 {
		u = "mons"
	}
	return fmt.Sprintf("%d %s", n, u), true
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
	Names        []NameRef           `json:"names,omitempty"`
	NeedsConfirm bool                `json:"needs_confirm,omitempty"`
	Syntax       *shortsyntax.Result `json:"syntax,omitempty"`
}

// OutputOf describes a plan, and the result if it was applied.
func OutputOf(p *Plan, res *Result) Output {
	o := Output{
		Input: p.Input, Verb: p.Verb, DryRun: res == nil, Preview: p.Preview, Subject: p.Subject, Links: p.Links,
		Changes: p.Changes, Warnings: p.Warnings, Problems: p.Problems, Names: p.Names, NeedsConfirm: p.NeedsConfirm, Syntax: p.Syntax,
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

// Redo applies changes again after Undo reversed them, oldest first. Archived notes and tasks
// are restored, tags are put back and fields get their new value again.
func Redo(ctx context.Context, r Repos, changes []Change) error {
	var firstErr error
	keep := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	updates := map[int64]*model.UpdatePersonInput{}
	var order []int64
	for _, c := range changes {
		switch {
		case c.Op == "create" && c.Entity == "interaction":
			keep(r.Interactions.Restore(ctx, c.ID))
		case c.Op == "create" && c.Entity == "task":
			keep(r.Tasks.Restore(ctx, c.ID))
		case c.Op == "tag":
			keep(r.Tags.Apply(ctx, c.Entity, c.ID, c.New))
		case c.Op == "untag":
			keep(r.Tags.Remove(ctx, c.Entity, c.ID, c.New))
		case c.Op == "update" && c.Entity == "person":
			if kind, ok := handleKey(c.Field); ok {
				if c.New == "" {
					keep(r.People.RemoveHandle(ctx, c.ID, kind))
				} else {
					keep(r.People.SetHandle(ctx, c.ID, kind, c.New))
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
			def.set(in, c.New)
		}
	}
	for _, id := range order {
		_, err := r.People.Update(ctx, id, *updates[id])
		keep(err)
	}
	return firstErr
}

// Summarize words what changes did, for messages: "add note on Sam Lee, tag #deal".
func Summarize(changes []Change) string {
	var parts []string
	fields := map[int64][]string{}
	var fieldOrder []int64
	names := map[int64]string{}
	for _, c := range changes {
		switch {
		case c.Op == "create":
			parts = append(parts, "add "+c.Label)
		case c.Op == "tag":
			parts = append(parts, "tag #"+c.New)
		case c.Op == "untag":
			parts = append(parts, "remove tag #"+c.New)
		case c.Op == "update" && c.Entity == "person":
			if _, ok := fields[c.ID]; !ok {
				fieldOrder = append(fieldOrder, c.ID)
				names[c.ID] = c.Label
			}
			fields[c.ID] = append(fields[c.ID], c.Field)
		}
	}
	for _, id := range fieldOrder {
		parts = append(parts, "set "+strings.Join(fields[id], ", ")+" on "+names[id])
	}
	if len(parts) == 0 {
		return "change"
	}
	if len(parts) > 3 {
		return strings.Join(parts[:2], ", ") + fmt.Sprintf(" (+%d more)", len(parts)-2)
	}
	return strings.Join(parts, ", ")
}

// CheckUndo says why changes cannot be undone any more: a field was edited since. It returns nil
// when it is safe to call Undo.
func CheckUndo(ctx context.Context, r Repos, changes []Change) error {
	return checkFields(ctx, r, changes, func(c Change) string { return c.New }, "now")
}

// CheckRedo says why changes cannot be redone: a field was edited since the undo.
func CheckRedo(ctx context.Context, r Repos, changes []Change) error {
	return checkFields(ctx, r, changes, func(c Change) string { return c.Old }, "now")
}

func checkFields(ctx context.Context, r Repos, changes []Change, expect func(Change) string, _ string) error {
	for _, c := range changes {
		if c.Op != "update" || c.Entity != "person" || c.Field == "snooze" {
			continue
		}
		p, err := r.People.FindByID(ctx, c.ID)
		if err != nil {
			return fmt.Errorf("%s is gone or archived", c.Label)
		}
		var cur string
		if kind, ok := handleKey(c.Field); ok {
			if err := r.People.LoadDetails(ctx, p); err != nil {
				return err
			}
			cur = handleValue(p, kind)
		} else if def, ok := lookupField(c.Field); ok {
			cur = def.get(p)
		} else {
			continue
		}
		if cur != expect(c) {
			return fmt.Errorf("%s of %s changed since: it is now %s", c.Field, c.Label, orEmpty(cur))
		}
	}
	return nil
}

// StaleFields lists the fields of cur whose value is not what expected sets. Name parts, birthday,
// last contacted and organization are not compared.
func StaleFields(cur *model.Person, expected model.UpdatePersonInput) []string {
	var out []string
	cmp := func(set *string, have string, name string) {
		if set != nil && *set != have {
			out = append(out, name)
		}
	}
	cmp(expected.Email, strOf(cur.Email), "email")
	cmp(expected.Phone, strOf(cur.Phone), "phone")
	cmp(expected.Title, strOf(cur.Title), "title")
	cmp(expected.Company, strOf(cur.Company), "company")
	cmp(expected.Location, strOf(cur.Location), "location")
	cmp(expected.Notes, strOf(cur.Notes), "notes")
	cmp(expected.Summary, strOf(cur.Summary), "summary")
	cmp(expected.KeepInTouch, strOf(cur.KeepInTouch), "keep in touch")
	if expected.IsFavorite != nil && *expected.IsFavorite != cur.IsFavorite {
		out = append(out, "favorite")
	}
	if expected.PriorityTier != nil {
		have := 0
		if cur.PriorityTier != nil {
			have = *cur.PriorityTier
		}
		if have != *expected.PriorityTier {
			out = append(out, "priority")
		}
	}
	return out
}

// InverseUpdate returns the update that puts back the values an update is about to overwrite.
// before is the person as it was.
func InverseUpdate(before *model.Person, in model.UpdatePersonInput) model.UpdatePersonInput {
	var inv model.UpdatePersonInput
	str := func(set bool, v string) *string {
		if !set {
			return nil
		}
		return &v
	}
	inv.FirstName = str(in.FirstName != nil, before.FirstName)
	inv.LastName = str(in.LastName != nil, strOf(before.LastName))
	inv.Email = str(in.Email != nil, strOf(before.Email))
	inv.Phone = str(in.Phone != nil, strOf(before.Phone))
	inv.Title = str(in.Title != nil, strOf(before.Title))
	inv.Company = str(in.Company != nil, strOf(before.Company))
	inv.Location = str(in.Location != nil, strOf(before.Location))
	inv.Notes = str(in.Notes != nil, strOf(before.Notes))
	inv.Summary = str(in.Summary != nil, strOf(before.Summary))
	if in.OrgID != nil && before.OrgID != nil {
		v := *before.OrgID
		inv.OrgID = &v
	}
	if def, ok := lookupField("birthday"); ok {
		inv.Birthday = str(in.Birthday != nil, def.get(before))
	}
	inv.KeepInTouch = str(in.KeepInTouch != nil, strOf(before.KeepInTouch))
	inv.SnoozedUntil = str(in.SnoozedUntil != nil, strOf(before.KeepInTouchSnoozedUntil))
	inv.LastContactedAt = str(in.LastContactedAt != nil, strOf(before.LastContactedAt))
	if in.IsFavorite != nil {
		v := before.IsFavorite
		inv.IsFavorite = &v
	}
	if in.PriorityTier != nil {
		v := 0
		if before.PriorityTier != nil {
			v = *before.PriorityTier
		}
		inv.PriorityTier = &v
	}
	return inv
}

// UpdateFields names the fields an update sets, for messages.
func UpdateFields(in model.UpdatePersonInput) []string {
	var out []string
	add := func(set bool, name string) {
		if set {
			out = append(out, name)
		}
	}
	add(in.FirstName != nil || in.LastName != nil, "name")
	add(in.Email != nil, "email")
	add(in.Phone != nil, "phone")
	add(in.Title != nil, "title")
	add(in.Company != nil, "company")
	add(in.Location != nil, "location")
	add(in.Notes != nil, "notes")
	add(in.Summary != nil, "summary")
	add(in.OrgID != nil, "organization")
	add(in.Birthday != nil, "birthday")
	add(in.KeepInTouch != nil, "keep in touch")
	add(in.SnoozedUntil != nil, "snooze")
	add(in.LastContactedAt != nil, "last contacted")
	add(in.IsFavorite != nil, "favorite")
	add(in.PriorityTier != nil, "priority")
	return out
}

// Describe writes a change as one human line.
func (c Change) Describe() string {
	switch c.Op {
	case "create":
		return fmt.Sprintf("created %s: %s", c.Entity, c.Label)
	case "tag":
		return fmt.Sprintf("tagged %s %s with #%s", c.Entity, labelOrID(c), c.New)
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
	if c.ID == 0 {
		return "(new)"
	}
	return fmt.Sprintf("#%d", c.ID)
}

// SettleNames answers the names of a plan without asking, for commands and agents. keep turns
// every unsettled name into plain text. linkNew creates a contact for a name that matched nobody
// and links it. In a dry run nothing is created: those names are kept as text and returned in
// created, which lists the names that would have been made. The plan is parsed again.
func SettleNames(ctx context.Context, r Repos, line string, env Env, plan *Plan, keep, linkNew, dry bool) (*Plan, []string, error) {
	if len(plan.Names) == 0 || (!keep && !linkNew) {
		return plan, nil, nil
	}
	// Do nothing when a name would stay unsettled: no contact is made for a line that still fails.
	for _, ref := range plan.Names {
		if !keep && !(len(ref.Candidates) == 0 && linkNew) {
			return plan, nil, nil
		}
	}
	choices := map[string]NameChoice{}
	var created []string
	for _, ref := range plan.Names {
		switch {
		case len(ref.Candidates) == 0 && linkNew:
			created = append(created, ref.Text)
			if dry {
				choices[ref.Key] = NameChoice{Action: "text"}
				continue
			}
			parts := strings.Fields(strings.ReplaceAll(ref.Text, "_", " "))
			in := model.CreatePersonInput{FirstName: parts[0]}
			if len(parts) > 1 {
				in.FirstName = strings.Join(parts[:len(parts)-1], " ")
				last := parts[len(parts)-1]
				in.LastName = &last
			}
			p, err := r.People.Create(ctx, in)
			if err != nil {
				return plan, created, err
			}
			choices[ref.Key] = NameChoice{Action: "link", PersonID: p.ID, Name: strings.TrimSpace(p.FirstName + " " + strOf(p.LastName))}
		case keep:
			choices[ref.Key] = NameChoice{Action: "text"}
		}
	}
	env.Choices = choices
	return Parse(line, env), created, nil
}
