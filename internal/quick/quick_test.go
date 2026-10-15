package quick

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testNow is Thursday 2026-10-15 12:00 UTC.
var testNow = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

type fixture struct {
	t     *testing.T
	repos Repos
	pr    *repo.PersonRepo
	ids   map[string]int64
}

func sp(s string) *string { return &s }

func newFixture(t *testing.T) *fixture {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	f := &fixture{t: t, repos: NewRepos(d), pr: repo.NewPersonRepo(d), ids: map[string]int64{}}
	ctx := context.Background()
	add := func(first, last string) {
		p, err := f.pr.Create(ctx, model.CreatePersonInput{FirstName: first, LastName: sp(last)})
		require.NoError(t, err)
		f.ids[first+" "+last] = p.ID
	}
	add("Sam", "Lee")
	add("Sam", "Ng")
	add("Bob", "Baker")
	add("Zoë", "Müller")
	add("Mark", "Twain")
	require.NoError(t, f.repos.Tags.Apply(ctx, "person", f.ids["Bob Baker"], "friend"))
	return f
}

// env builds an Env from the current database. ctxName selects a person ("" for none).
func (f *fixture) env(ctxName string) Env {
	f.t.Helper()
	ctx := context.Background()
	people, err := f.pr.ListSummaries(ctx)
	require.NoError(f.t, err)
	tags, err := f.repos.Tags.FindAll(ctx)
	require.NoError(f.t, err)
	names := []string{}
	for _, tg := range tags {
		names = append(names, tg.Name)
	}
	e := Env{
		Now: testNow, Index: NewIndex(people), Tags: names,
		LoadPerson: func(id int64) (*model.Person, error) {
			p, err := f.pr.FindByID(ctx, id)
			if err != nil {
				return nil, err
			}
			return p, f.pr.LoadDetails(ctx, p)
		},
	}
	if ctxName != "" {
		e.Context = f.ids[ctxName]
	}
	return e
}

func (f *fixture) person(name string) *model.Person {
	p, err := f.pr.FindByID(context.Background(), f.ids[name])
	require.NoError(f.t, err)
	require.NoError(f.t, f.pr.LoadDetails(context.Background(), p))
	return p
}

func problemCodes(p *Plan) []string {
	var out []string
	for _, pr := range p.Problems {
		out = append(out, pr.Code)
	}
	return out
}

func markKinds(p *Plan) []string {
	var out []string
	for _, m := range p.Marks {
		out = append(out, m.Kind)
	}
	return out
}

func TestVerbs(t *testing.T) {
	for in, want := range map[string]string{"n": "note", "NOTE": "note", "t": "task", "task": "task", "log": "log", "set": "set", "tag": "tag", "snooze": "snooze"} {
		got, ok := VerbOf(in)
		assert.True(t, ok, in)
		assert.Equal(t, want, got, in)
	}
	_, ok := VerbOf("nope")
	assert.False(t, ok)
	assert.True(t, IsCommand("n hello"))
	assert.True(t, IsCommand("  note hello"))
	assert.True(t, IsCommand("n "))
	assert.False(t, IsCommand("n"))
	assert.False(t, IsCommand("favorite"))
	assert.False(t, IsCommand("nothing here"))
	assert.False(t, IsCommand(""))
}

func TestNotACommandHasNoVerb(t *testing.T) {
	f := newFixture(t)
	p := Parse("hello there", f.env(""))
	assert.Equal(t, "", p.Verb)
	assert.False(t, p.Ready())
	assert.Empty(t, p.Problems)
}

func TestNoteWithSelectedPerson(t *testing.T) {
	f := newFixture(t)
	env := f.env("Bob Baker")
	p := Parse("n called about the deal #deal @fri", env)
	require.Empty(t, p.Problems)
	require.True(t, p.Ready())
	assert.Equal(t, "note", p.Verb)
	assert.Equal(t, "Bob Baker", p.Subject.Name)
	assert.Equal(t, []string{"verb", "tag", "due"}, markKinds(p))
	assert.Contains(t, p.Preview, `note on Bob Baker: "called about the deal"`)
	assert.Contains(t, p.Preview, "tag #deal (new)")
	assert.Contains(t, p.Preview, "remind Fri 16 Oct")
	require.Len(t, p.Changes, 3)
	assert.Equal(t, "interaction", p.Changes[0].Entity)
	assert.False(t, p.NeedsConfirm)

	res, err := Apply(context.Background(), f.repos, p)
	require.NoError(t, err)
	assert.True(t, res.Applied)
	require.Len(t, res.Changes, 3)

	ctx := context.Background()
	its, err := f.repos.Interactions.FindAll(ctx, model.InteractionFilters{})
	require.NoError(t, err)
	require.Len(t, its, 1)
	assert.Equal(t, "note", its[0].Type)
	assert.Equal(t, "called about the deal", *its[0].Content)
	assert.Equal(t, []int64{f.ids["Bob Baker"]}, its[0].PersonIDs)
	tags, err := f.repos.Tags.GetForEntity(ctx, "interaction", its[0].ID)
	require.NoError(t, err)
	require.Len(t, tags, 1)
	assert.Equal(t, "deal", tags[0].Name)
	tasks, err := f.repos.Tasks.FindAll(ctx, model.TaskFilters{})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "Follow up: called about the deal", tasks[0].Title)
	assert.Equal(t, "2026-10-16", *tasks[0].DueAt)
	assert.Equal(t, f.ids["Bob Baker"], *tasks[0].PersonID)
}

func TestNoteLeadingName(t *testing.T) {
	f := newFixture(t)
	env := f.env("")
	p := Parse("n Bob called about the deal #friend", env)
	require.Empty(t, p.Problems)
	assert.Equal(t, "Bob Baker", p.Subject.Name)
	assert.Contains(t, p.Preview, `"called about the deal"`)
	assert.Contains(t, p.Preview, "tag #friend")
	assert.NotContains(t, p.Preview, "(new)", "friend exists")
	assert.Equal(t, []string{"verb", "subject", "tag"}, markKinds(p))

	// A full name wins over the first name.
	p = Parse("n Sam Lee had lunch", env)
	require.Empty(t, p.Problems)
	assert.Equal(t, "Sam Lee", p.Subject.Name)
	assert.Contains(t, p.Preview, `"had lunch"`)

	// Last name alone is enough when it is unique.
	p = Parse("n Twain sent a letter", env)
	require.Empty(t, p.Problems)
	assert.Equal(t, "Mark Twain", p.Subject.Name)

	// Accents are ignored.
	p = Parse("n zoe called", env)
	require.Empty(t, p.Problems)
	assert.Equal(t, "Zoë Müller", p.Subject.Name)
}

func TestNoteAmbiguousName(t *testing.T) {
	f := newFixture(t)
	p := Parse("n Sam called", f.env(""))
	assert.False(t, p.Ready())
	require.Len(t, p.Problems, 1)
	assert.Equal(t, "ambiguous", p.Problems[0].Code)
	assert.Len(t, p.Problems[0].Candidates, 2)
	assert.Contains(t, p.Preview, "2 people match")
	_, err := Apply(context.Background(), f.repos, p)
	assert.ErrorIs(t, err, model.ErrValidation)
}

func TestNoteNeedsAPerson(t *testing.T) {
	f := newFixture(t)
	p := Parse("n called about the deal", f.env(""))
	assert.Equal(t, []string{"no_person"}, problemCodes(p))
	assert.False(t, p.Ready())

	// Escape: a backslash turns name matching off.
	p = Parse(`n \Mark the calendar`, f.env(""))
	assert.Equal(t, []string{"no_person"}, problemCodes(p))
	assert.Contains(t, markKinds(p), "escape")
}

func TestSelectedPersonBeatsLeadingName(t *testing.T) {
	f := newFixture(t)
	p := Parse("n Sam called", f.env("Bob Baker"))
	require.Empty(t, p.Problems)
	assert.Equal(t, "Bob Baker", p.Subject.Name)
	assert.Contains(t, p.Preview, `"Sam called"`)
}

func TestPlusLinks(t *testing.T) {
	f := newFixture(t)
	env := f.env("Bob Baker")
	p := Parse("n lunch with +Sam_Lee", env)
	require.Empty(t, p.Problems)
	assert.Equal(t, "Bob Baker", p.Subject.Name)
	require.Len(t, p.Links, 1)
	assert.Equal(t, "Sam Lee", p.Links[0].Name)
	res, err := Apply(context.Background(), f.repos, p)
	require.NoError(t, err)
	its, _ := f.repos.Interactions.FindAll(context.Background(), model.InteractionFilters{})
	require.Len(t, its, 1)
	assert.ElementsMatch(t, []int64{f.ids["Bob Baker"], f.ids["Sam Lee"]}, its[0].PersonIDs)
	assert.Equal(t, "lunch with", *its[0].Content)
	assert.Len(t, res.Changes, 1)

	// Without a selected person the first link is the subject.
	p = Parse("n coffee +Bob +Zoe", f.env(""))
	require.Empty(t, p.Problems, p.Preview)
	assert.Equal(t, "Bob Baker", p.Subject.Name)
	assert.Equal(t, "Zoë Müller", p.Links[0].Name)

	// Unknown and ambiguous links are problems and are marked bad.
	p = Parse("n coffee +Nobody", env)
	assert.Equal(t, []string{"not_found"}, problemCodes(p))
	assert.True(t, p.Marks[len(p.Marks)-1].Bad)
	p = Parse("n coffee +Sam", env)
	assert.Equal(t, []string{"ambiguous"}, problemCodes(p))
}

func TestNoteCadence(t *testing.T) {
	f := newFixture(t)
	p := Parse("n Bob great chat *3m", f.env(""))
	require.Empty(t, p.Problems)
	assert.Contains(t, p.Preview, "keep in touch 3 mons")
	_, err := Apply(context.Background(), f.repos, p)
	require.NoError(t, err)
	assert.Equal(t, "3 mons", *f.person("Bob Baker").KeepInTouch)

	// Setting the same cadence again is a warning, not a change.
	p = Parse("n Bob again *3m", f.env(""))
	assert.NotEmpty(t, p.Warnings)
	assert.Len(t, p.Changes, 1)
}

func TestNoteWithoutTextIsEmpty(t *testing.T) {
	f := newFixture(t)
	p := Parse("n #deal", f.env("Bob Baker"))
	assert.Equal(t, []string{"empty"}, problemCodes(p))
	p = Parse("n ", f.env("Bob Baker"))
	assert.False(t, p.Ready())
	assert.Empty(t, p.Problems)
	assert.Contains(t, p.Preview, "note [name] text")
}

func TestNoSyntaxLeavesTokensInTheText(t *testing.T) {
	f := newFixture(t)
	env := f.env("Bob Baker")
	env.NoSyntax = true
	p := Parse("n called #deal @fri", env)
	require.Empty(t, p.Problems)
	assert.Contains(t, p.Preview, `"called #deal @fri"`)
	assert.Len(t, p.Changes, 1)
}

func TestTask(t *testing.T) {
	f := newFixture(t)
	env := f.env("")
	p := Parse("t Bob call back @fri 3pm ^high ~30m !mon", env)
	require.Empty(t, p.Problems, p.Preview)
	assert.Equal(t, "task", p.Verb)
	assert.Contains(t, p.Preview, `task for Bob Baker: "call back"`)
	assert.Contains(t, p.Preview, "due Fri 16 Oct 15:00")
	assert.Contains(t, p.Preview, "high priority")
	assert.Contains(t, p.Preview, "30m")
	res, err := Apply(context.Background(), f.repos, p)
	require.NoError(t, err)
	require.Len(t, res.Changes, 1)
	task, err := f.repos.Tasks.FindByID(context.Background(), res.Changes[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "call back", task.Title)
	assert.Equal(t, "2026-10-16 15:00", *task.DueAt)
	assert.Equal(t, "high", task.Priority)
	assert.Equal(t, f.ids["Bob Baker"], *task.PersonID)
	assert.Equal(t, "Deadline: 2026-10-19\nEstimate: 30m", *task.Description)
}

func TestTaskDeadlineOnlyBecomesDue(t *testing.T) {
	f := newFixture(t)
	p := Parse("t Bob send deck !fri", f.env(""))
	require.Empty(t, p.Problems)
	assert.Contains(t, p.Preview, "(deadline)")
	res, err := Apply(context.Background(), f.repos, p)
	require.NoError(t, err)
	task, _ := f.repos.Tasks.FindByID(context.Background(), res.Changes[0].ID)
	assert.Equal(t, "2026-10-16", *task.DueAt)
	assert.Nil(t, task.Description)
}

func TestTaskWithoutPersonIsAllowed(t *testing.T) {
	f := newFixture(t)
	p := Parse("t buy milk @tomorrow", f.env(""))
	require.Empty(t, p.Problems)
	assert.Nil(t, p.Subject)
	assert.Contains(t, p.Preview, `task: "buy milk"`)
	res, err := Apply(context.Background(), f.repos, p)
	require.NoError(t, err)
	task, _ := f.repos.Tasks.FindByID(context.Background(), res.Changes[0].ID)
	assert.Nil(t, task.PersonID)
	assert.Equal(t, "medium", task.Priority)
	assert.Equal(t, "2026-10-16", *task.DueAt)
}

func TestTaskTagsStayInTheTitleWithAWarning(t *testing.T) {
	f := newFixture(t)
	p := Parse("t call plumber #home", f.env(""))
	require.Empty(t, p.Problems)
	assert.Contains(t, p.Preview, `"call plumber #home"`)
	require.Len(t, p.Warnings, 1)
	assert.Contains(t, p.Warnings[0], "tasks cannot carry tags yet")
}

func TestTaskManyPeople(t *testing.T) {
	f := newFixture(t)
	p := Parse("t plan offsite +Bob +Zoe", f.env(""))
	require.Empty(t, p.Problems)
	res, err := Apply(context.Background(), f.repos, p)
	require.NoError(t, err)
	task, _ := f.repos.Tasks.FindByID(context.Background(), res.Changes[0].ID)
	assert.ElementsMatch(t, []int64{f.ids["Bob Baker"], f.ids["Zoë Müller"]}, task.PersonIDs)
}

func TestLog(t *testing.T) {
	f := newFixture(t)
	env := f.env("")
	env.NoteTypes = []string{"call", "meeting", "note"}
	p := Parse("log call Bob talked about the offsite @yesterday #trip", env)
	require.Empty(t, p.Problems, p.Preview)
	assert.Contains(t, p.Preview, `call on Bob Baker: "talked about the offsite"`)
	assert.Contains(t, p.Preview, "Wed 14 Oct")
	res, err := Apply(context.Background(), f.repos, p)
	require.NoError(t, err)
	it, err := f.repos.Interactions.FindByID(context.Background(), res.Changes[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "call", it.Type)
	assert.Equal(t, "2026-10-14 12:00:00", it.OccurredAt)
	// A logged @ is when it happened, so no reminder is created.
	tasks, _ := f.repos.Tasks.FindAll(context.Background(), model.TaskFilters{})
	assert.Empty(t, tasks)

	p = Parse("log lunch Bob ate", env)
	assert.Equal(t, []string{"unknown_type"}, problemCodes(p))
	p = Parse("log ", env)
	assert.Contains(t, p.Preview, "log <type>")

	// With a time.
	p = Parse("log meeting Bob kickoff @tue 9:30", env)
	require.Empty(t, p.Problems)
	res, err = Apply(context.Background(), f.repos, p)
	require.NoError(t, err)
	it, _ = f.repos.Interactions.FindByID(context.Background(), res.Changes[0].ID)
	assert.Equal(t, "2026-10-20 09:30:00", it.OccurredAt)
}

func TestSetSingleFieldAppliesAtOnce(t *testing.T) {
	f := newFixture(t)
	p := Parse("set Bob: company=Acme", f.env(""))
	require.Empty(t, p.Problems)
	require.True(t, p.Ready())
	assert.False(t, p.NeedsConfirm)
	require.Len(t, p.Changes, 1)
	assert.Equal(t, Change{Op: "update", Entity: "person", ID: f.ids["Bob Baker"], Label: "Bob Baker", Field: "company", Old: "", New: "Acme"}, p.Changes[0])
	assert.Equal(t, []string{"verb", "subject", "field"}, markKinds(p))
	_, err := Apply(context.Background(), f.repos, p)
	require.NoError(t, err)
	assert.Equal(t, "Acme", *f.person("Bob Baker").Company)
}

func TestSetMultiFieldNeedsConfirmAndUndoes(t *testing.T) {
	f := newFixture(t)
	_, err := f.pr.Update(context.Background(), f.ids["Sam Lee"], model.UpdatePersonInput{Company: sp("Old Co")})
	require.NoError(t, err)

	p := Parse("set Sam Lee: company=Acme title=CTO birthday=Mar 4", f.env(""))
	require.Empty(t, p.Problems, p.Preview)
	assert.True(t, p.NeedsConfirm)
	require.Len(t, p.Changes, 3)
	assert.Equal(t, "Old Co", p.Changes[0].Old)
	assert.Equal(t, "Acme", p.Changes[0].New)
	assert.Equal(t, "", p.Changes[1].Old)
	assert.Equal(t, "CTO", p.Changes[1].New)
	assert.Equal(t, "birthday", p.Changes[2].Field)
	assert.Equal(t, "--03-04", p.Changes[2].New)
	assert.Equal(t, "company -> Acme", "company -> "+p.Changes[0].New)

	res, err := Apply(context.Background(), f.repos, p)
	require.NoError(t, err)
	person := f.person("Sam Lee")
	assert.Equal(t, "Acme", *person.Company)
	assert.Equal(t, "CTO", *person.Title)
	assert.Equal(t, 3, *person.BirthdayMonth)
	assert.Equal(t, 4, *person.BirthdayDay)
	assert.Nil(t, person.BirthdayYear)

	require.NoError(t, Undo(context.Background(), f.repos, res.Changes))
	person = f.person("Sam Lee")
	assert.Equal(t, "Old Co", *person.Company)
	assert.True(t, person.Title == nil || *person.Title == "")
	assert.Nil(t, person.BirthdayMonth)
}

func TestSetSubjectForms(t *testing.T) {
	f := newFixture(t)
	// Selected person, no name.
	p := Parse("set title=CTO", f.env("Bob Baker"))
	require.Empty(t, p.Problems)
	assert.Equal(t, "Bob Baker", p.Subject.Name)
	// Name without a colon.
	p = Parse("set Bob title=CTO", f.env(""))
	require.Empty(t, p.Problems)
	assert.Equal(t, "Bob Baker", p.Subject.Name)
	// An explicit name beats the selected person.
	p = Parse("set Zoe: title=CTO", f.env("Bob Baker"))
	require.Empty(t, p.Problems)
	assert.Equal(t, "Zoë Müller", p.Subject.Name)
	// No person at all.
	p = Parse("set title=CTO", f.env(""))
	assert.Equal(t, []string{"no_person"}, problemCodes(p))
	// Unknown name.
	p = Parse("set Nobody: title=CTO", f.env(""))
	assert.Equal(t, []string{"not_found"}, problemCodes(p))
	// Ambiguous name.
	p = Parse("set Sam: title=CTO", f.env(""))
	assert.Equal(t, []string{"ambiguous"}, problemCodes(p))
}

func TestSetValuesAndKeys(t *testing.T) {
	f := newFixture(t)
	env := f.env("Bob Baker")
	p := Parse(`set company=Acme Corp title="VP, Sales" email=bob@acme.com location=Berlin`, env)
	require.Empty(t, p.Problems, p.Preview)
	got := map[string]string{}
	for _, c := range p.Changes {
		got[c.Field] = c.New
	}
	assert.Equal(t, map[string]string{"company": "Acme Corp", "title": "VP, Sales", "email": "bob@acme.com", "location": "Berlin"}, got)

	// Aliases and case.
	p = Parse("set Org=Acme Role=CTO BDAY=1985-03-04 kit=3m priority=high fav=yes", env)
	require.Empty(t, p.Problems, p.Preview)
	got = map[string]string{}
	for _, c := range p.Changes {
		got[c.Field] = c.New
	}
	assert.Equal(t, map[string]string{"company": "Acme", "title": "CTO", "birthday": "1985-03-04", "cadence": "3 mons", "tier": "high", "favorite": "yes"}, got)
	_, err := Apply(context.Background(), f.repos, p)
	require.NoError(t, err)
	person := f.person("Bob Baker")
	assert.Equal(t, 1985, *person.BirthdayYear)
	assert.Equal(t, "3 mons", *person.KeepInTouch)
	assert.Equal(t, 2, *person.PriorityTier)
	assert.True(t, person.IsFavorite)
}

func TestSetErrors(t *testing.T) {
	f := newFixture(t)
	env := f.env("Bob Baker")
	for _, line := range []string{
		"set foo=bar",
		"set birthday=someday",
		"set cadence=5d",
		"set priority=urgentish",
		"set favorite=maybe",
		"set name=",
	} {
		p := Parse(line, env)
		assert.Equal(t, []string{"invalid"}, problemCodes(p), line)
		assert.False(t, p.Ready(), line)
	}
	// Clearing a field is allowed.
	_, err := f.pr.Update(context.Background(), f.ids["Bob Baker"], model.UpdatePersonInput{Company: sp("Acme")})
	require.NoError(t, err)
	p := Parse("set company=", env)
	require.Empty(t, p.Problems)
	assert.Equal(t, "Acme", p.Changes[0].Old)
	assert.Equal(t, "", p.Changes[0].New)
	// Unchanged values are skipped with a warning.
	p = Parse("set company=Acme", env)
	assert.Empty(t, p.Changes)
	assert.NotEmpty(t, p.Warnings)
	assert.False(t, p.Ready())
	assert.Contains(t, p.Preview, "nothing to change")
}

func TestSetWithTokens(t *testing.T) {
	f := newFixture(t)
	p := Parse("set Bob: *6m ^top #vip #friend", f.env(""))
	require.Empty(t, p.Problems, p.Preview)
	// friend is already on Bob, so only vip is added.
	fields := []string{}
	for _, c := range p.Changes {
		fields = append(fields, c.Op+":"+c.Field)
	}
	assert.Equal(t, []string{"update:cadence", "update:tier", "tag:tag"}, fields)
	assert.True(t, p.NeedsConfirm)
	res, err := Apply(context.Background(), f.repos, p)
	require.NoError(t, err)
	tags, _ := f.repos.Tags.GetForEntity(context.Background(), "person", f.ids["Bob Baker"])
	assert.Len(t, tags, 2)
	assert.Equal(t, 1, *f.person("Bob Baker").PriorityTier)
	require.NoError(t, Undo(context.Background(), f.repos, res.Changes))
	tags, _ = f.repos.Tags.GetForEntity(context.Background(), "person", f.ids["Bob Baker"])
	assert.Len(t, tags, 1)
	assert.Nil(t, f.person("Bob Baker").PriorityTier)
	assert.Nil(t, f.person("Bob Baker").KeepInTouch)
}

func TestSetHandle(t *testing.T) {
	f := newFixture(t)
	p := Parse("set Bob: linkedin=https://linkedin.com/in/bob", f.env(""))
	require.Empty(t, p.Problems, p.Preview)
	res, err := Apply(context.Background(), f.repos, p)
	require.NoError(t, err)
	assert.Equal(t, "https://linkedin.com/in/bob", handleValue(f.person("Bob Baker"), "linkedin"))
	require.NoError(t, Undo(context.Background(), f.repos, res.Changes))
	assert.Equal(t, "", handleValue(f.person("Bob Baker"), "linkedin"))
}

func TestTagVerb(t *testing.T) {
	f := newFixture(t)
	p := Parse("tag Bob vip #client -friend", f.env(""))
	require.Empty(t, p.Problems, p.Preview)
	assert.Contains(t, p.Preview, "+#vip (new)")
	assert.Contains(t, p.Preview, "-#friend")
	require.Len(t, p.Changes, 3)
	res, err := Apply(context.Background(), f.repos, p)
	require.NoError(t, err)
	tags, _ := f.repos.Tags.GetForEntity(context.Background(), "person", f.ids["Bob Baker"])
	var names []string
	for _, tg := range tags {
		names = append(names, tg.Name)
	}
	assert.ElementsMatch(t, []string{"vip", "client"}, names)
	require.NoError(t, Undo(context.Background(), f.repos, res.Changes))
	tags, _ = f.repos.Tags.GetForEntity(context.Background(), "person", f.ids["Bob Baker"])
	require.Len(t, tags, 1)
	assert.Equal(t, "friend", tags[0].Name)

	// Selected person, existing and missing tags.
	p = Parse("tag friend -nothing", f.env("Bob Baker"))
	assert.Empty(t, p.Changes)
	assert.Len(t, p.Warnings, 2)
	p = Parse("tag bad!name", f.env("Bob Baker"))
	assert.Equal(t, []string{"invalid"}, problemCodes(p))
	p = Parse("tag vip", f.env(""))
	assert.Equal(t, []string{"no_person"}, problemCodes(p))
}

func TestSnooze(t *testing.T) {
	f := newFixture(t)
	env := f.env("")
	p := Parse("snooze Bob 7", env)
	require.Empty(t, p.Problems, p.Preview)
	assert.Equal(t, "snooze Bob Baker 7 days", p.Preview)
	assert.Equal(t, "2026-10-22 12:00:00", p.Changes[0].New)
	_, err := Apply(context.Background(), f.repos, p)
	require.NoError(t, err)
	assert.Equal(t, "2026-10-22 12:00:00", *f.person("Bob Baker").KeepInTouchSnoozedUntil)

	p = Parse("snooze Bob @fri", env)
	require.Empty(t, p.Problems)
	assert.Equal(t, "2026-10-16 00:00:00", p.Changes[0].New)
	assert.Equal(t, "2026-10-22 12:00:00", p.Changes[0].Old)

	p = Parse("snooze Bob 2w", env)
	require.Empty(t, p.Problems)
	assert.Equal(t, "2026-10-29 00:00:00", p.Changes[0].New)

	p = Parse("snooze Bob next week", env)
	require.Empty(t, p.Problems)
	assert.Equal(t, "2026-10-19 00:00:00", p.Changes[0].New)

	p = Parse("snooze 3", f.env("Bob Baker"))
	require.Empty(t, p.Problems)
	assert.Equal(t, "2026-10-18 12:00:00", p.Changes[0].New)

	p = Parse("snooze Bob whenever", env)
	assert.Equal(t, []string{"invalid"}, problemCodes(p))
	p = Parse("snooze Bob", env)
	assert.False(t, p.Ready())
	assert.Empty(t, p.Problems)
}

func TestApplyUndoRoundTripForANote(t *testing.T) {
	f := newFixture(t)
	p := Parse("n Bob great call #deal @mon *1m", f.env(""))
	require.Empty(t, p.Problems)
	res, err := Apply(context.Background(), f.repos, p)
	require.NoError(t, err)
	require.Len(t, res.Changes, 4)
	require.NoError(t, Undo(context.Background(), f.repos, res.Changes))
	its, _ := f.repos.Interactions.FindAll(context.Background(), model.InteractionFilters{})
	assert.Empty(t, its)
	tasks, _ := f.repos.Tasks.FindAll(context.Background(), model.TaskFilters{})
	assert.Empty(t, tasks)
	assert.Nil(t, f.person("Bob Baker").KeepInTouch)
}

func TestApplyRollsBackWhenAStepFails(t *testing.T) {
	f := newFixture(t)
	good := Parse("n Bob great call #deal", f.env(""))
	require.True(t, good.Ready())
	// Add a step that fails after the note and its tag were written.
	good.ops = append(good.ops, op{run: func(ctx context.Context, r Repos, st *runState) ([]Change, error) {
		return nil, fmt.Errorf("disk on fire")
	}})
	_, err := Apply(context.Background(), f.repos, good)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk on fire")
	assert.Contains(t, err.Error(), "earlier steps were undone")
	its, _ := f.repos.Interactions.FindAll(context.Background(), model.InteractionFilters{})
	assert.Empty(t, its, "the note is archived again")
}

func TestApplyWithNothingToDo(t *testing.T) {
	_, err := Apply(context.Background(), Repos{}, &Plan{})
	assert.ErrorIs(t, err, model.ErrValidation)
	assert.Equal(t, model.ErrNotFound, ErrorFor("not_found"))
	assert.Equal(t, model.ErrValidation, ErrorFor("ambiguous"))
}

// A command line is re-read on every keystroke, so it must stay fast on a real address book.
func TestParseAndCompleteStayFastOnABigList(t *testing.T) {
	firsts := []string{"Sam", "Bob", "Zoë", "Mark", "Ana", "Li", "Omar", "Priya", "Noah", "Eva"}
	lasts := []string{"Lee", "Baker", "Müller", "Twain", "Smith", "Ng", "Khan", "Rossi", "Silva", "Novak"}
	var people []model.PersonSummary
	for i := 0; i < 5000; i++ {
		people = append(people, model.PersonSummary{
			ID: int64(i + 1), Name: firsts[i%10] + " " + lasts[(i/10)%10] + " " + string(rune('A'+i%26)) + string(rune('a'+(i/26)%26)),
			Tags: "friend, work",
		})
	}
	ix := NewIndex(people)
	env := Env{Now: testNow, Index: ix, Tags: []string{"friend", "work"}, Context: 0}
	lines := []string{
		"n Sam Lee Aa called about the deal #friend @fri 3pm +Bob Baker Ab ~30m",
		"set Mark Twain Ba: company=Acme title=CTO",
		"t buy milk +",
	}
	start := time.Now()
	const rounds = 20
	for i := 0; i < rounds; i++ {
		for _, l := range lines {
			Parse(l, env)
			Complete(l, len(l), env)
		}
	}
	per := time.Since(start) / (rounds * time.Duration(len(lines)))
	assert.Less(t, per, 25*time.Millisecond, "one keystroke should cost far less than a frame")
}

func TestChangeDescribe(t *testing.T) {
	assert.Equal(t, "created task: x", Change{Op: "create", Entity: "task", Label: "x"}.Describe())
	assert.Equal(t, "Bob company: (empty) -> Acme", Change{Op: "update", Label: "Bob", Field: "company", New: "Acme"}.Describe())
	assert.Equal(t, "tagged person Bob with #vip", Change{Op: "tag", Entity: "person", Label: "Bob", New: "vip"}.Describe())
	assert.Equal(t, "removed #vip from person #3", Change{Op: "untag", Entity: "person", ID: 3, New: "vip"}.Describe())
}

func TestMarksCoverTheInput(t *testing.T) {
	f := newFixture(t)
	line := "  n Bob called #deal @fri 3pm ~1h"
	p := Parse(line, f.env(""))
	require.Empty(t, p.Problems)
	for _, m := range p.Marks {
		assert.True(t, m.Start >= 0 && m.End <= len(line) && m.Start < m.End, "%+v", m)
	}
	texts := []string{}
	for _, m := range p.Marks {
		texts = append(texts, line[m.Start:m.End])
	}
	assert.Equal(t, []string{"n", "Bob", "#deal", "@fri 3pm"}, texts)
}

func TestMarksForSetKeys(t *testing.T) {
	f := newFixture(t)
	line := "set Bob: company=Acme title=CTO"
	p := Parse(line, f.env(""))
	require.Empty(t, p.Problems)
	var texts []string
	for _, m := range p.Marks {
		texts = append(texts, line[m.Start:m.End])
	}
	assert.Equal(t, []string{"set", "Bob:", "company=", "title="}, texts)
}

func TestSuggestAndResolve(t *testing.T) {
	f := newFixture(t)
	ix := f.env("").Index
	m, exact := ix.Resolve("sam lee")
	assert.True(t, exact)
	assert.Len(t, m, 1)
	m, exact = ix.Resolve("Sam")
	assert.False(t, exact)
	assert.Len(t, m, 2)
	m, _ = ix.Resolve("Mü")
	assert.Empty(t, m, "no prefix matching in Resolve")
	m, _ = ix.ResolveLoose("Mül")
	assert.Len(t, m, 0, "prefix is on the full name, which starts with Zoë")
	m, _ = ix.ResolveLoose("zoe m")
	assert.Len(t, m, 1)
	assert.Len(t, ix.Suggest("sa", 10), 2)
	assert.Equal(t, "Mark Twain", ix.Suggest("twa", 10)[0].Name)
	assert.Len(t, ix.Suggest("", 3), 3)
	var nilIndex *Index
	m, _ = nilIndex.Resolve("x")
	assert.Empty(t, m)
	assert.Equal(t, 0, nilIndex.Len())
	assert.Equal(t, 5, ix.Len())
	assert.True(t, ix.HasTag(f.ids["Bob Baker"], "FRIEND"))
	assert.False(t, ix.HasTag(f.ids["Sam Lee"], "friend"))
}

func TestComplete(t *testing.T) {
	f := newFixture(t)
	env := f.env("")
	env.NoteTypes = []string{"call", "meeting", "note"}
	cases := []struct {
		line string
		kind string
		want []string
	}{
		{"s", "verb", []string{"set ", "snooze "}},
		{"ta", "verb", []string{"task ", "tag "}},
		{"n", "verb", []string{"n ", "note "}},
		{"n Bob called #fr", "tag", []string{"#friend"}},
		{"n Bob called #", "tag", []string{"#friend"}},
		{"n Bob called @fr", "date", []string{"@fri"}},
		{"n Bob called @tom", "date", []string{"@tomorrow"}},
		{"n Bob called @next", "date", []string{"@next week", "@next month"}},
		{"n Bob called !s", "date", []string{"!sat", "!sun"}},
		{"n Bob called *3", "cadence", []string{"*3m"}},
		{"n Bob called ^h", "priority", []string{"^high"}},
		{"n Bob called ~3", "duration", []string{"~30m"}},
		{"n Bob called +sa", "person", []string{"+Sam Lee", "+Sam Ng"}},
		{"n Bo", "person", []string{"Bob Baker "}},
		{"n sam", "person", []string{"Sam Lee ", "Sam Ng "}},
		{"set Zo", "person", []string{"Zoë Müller: "}},
		{"set Bob: com", "field", []string{"company="}},
		{"set Bob: company=Acme ti", "field", []string{"title=", "tier="}},
		{"log ca", "type", []string{"call "}},
		{"log call Bo", "person", []string{"Bob Baker "}},
		{"tag Bob fr", "tag", []string{"friend"}},
		{"tag Bob -fr", "tag", []string{"-friend"}},
		{"snooze Bob 1", "date", []string{"14"}},
	}
	for _, c := range cases {
		t.Run(c.line, func(t *testing.T) {
			got := Complete(c.line, len(c.line), env)
			assert.Equal(t, c.kind, got.Kind)
			assert.Equal(t, c.want, got.Candidates)
			assert.LessOrEqual(t, got.Start, got.End)
			assert.Equal(t, len(c.line), got.End)
		})
	}
	// A finished word has nothing left to complete.
	done := "n Bob hi *3m"
	assert.Empty(t, Complete(done, len(done), env).Candidates)
	// With a selected person there are no name suggestions.
	env.Context = f.ids["Bob Baker"]
	got := Complete("n Bo", 4, env)
	assert.Empty(t, got.Candidates)
	// The cursor can be in the middle of the line.
	line := "n called #fr and more"
	got = Complete(line, len("n called #fr"), env)
	assert.Equal(t, []string{"#friend"}, got.Candidates)
	assert.Equal(t, "#fr", line[got.Start:got.End])
	// Out of range cursors are clamped.
	got = Complete("n", 99, env)
	assert.Equal(t, 1, got.End)
	got = Complete("n", -4, env)
	assert.Equal(t, 0, got.End)
}
