package shortsyntax

import (
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testNow is Thursday 2026-10-15 12:00 UTC.
var testNow = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

// opts reads plain-word dates, like the task, log and event commands.
func opts() Options { return Options{Now: testNow, Dates: true} }

// noteOpts is for notes: bare words are not dates.
func noteOpts() Options { return Options{Now: testNow} }

func TestTestNowIsAThursday(t *testing.T) {
	assert.Equal(t, time.Thursday, testNow.Weekday())
}

func whenStr(w *When) string {
	if w == nil {
		return ""
	}
	return w.String()
}

// plainOK says whether a date form is also read as a bare word in a sentence. The forms that are
// common in ordinary text (now, tod, tom, 1/2, a bare 2w) need by, remind or in to count.
func plainOK(in string) bool {
	switch {
	case in == "now" || in == "tod" || in == "tom" || in == "tomorow":
		return false
	case strings.HasPrefix(in, "+") && strings.HasSuffix(in, "h"), strings.HasPrefix(in, "+") && strings.Contains(in, "min"):
		return true
	}
	if len(in) >= 2 && in[0] >= '0' && in[0] <= '9' {
		// 2w, 10d, 11/1, 10/15: bare numbers with a unit or a slash.
		last := in[len(in)-1]
		if strings.ContainsAny(in, "/") && strings.Count(in, "/") == 1 {
			return false
		}
		if (last == 'd' || last == 'w' || last == 'm' || last == 'y') && !strings.ContainsAny(in, "- :") && len(in) <= 4 {
			return false
		}
	}
	return true
}

// TestPlainDates checks dates written as plain words. Each input is the whole text, so Title is empty.
func TestPlainDates(t *testing.T) {
	n := 0
	for _, c := range append(append([]dateCase{}, dateCases...), timeCases...) {
		if !plainOK(c.in) {
			continue
		}
		n++
		t.Run(c.in, func(t *testing.T) {
			r := Parse(c.in, opts())
			require.NotNil(t, r.Due, "no due for %q", c.in)
			assert.Equal(t, c.want, r.Due.String())
			assert.Equal(t, "", r.Title)
		})
	}
	assert.Greater(t, n, 100)
}

// TestByIsADeadlineAndTakesEveryDateForm runs the whole table after "by", where nothing is held back.
func TestByIsADeadlineAndTakesEveryDateForm(t *testing.T) {
	for _, c := range append(append([]dateCase{}, dateCases...), timeCases...) {
		in := "by " + c.in
		t.Run(in, func(t *testing.T) {
			r := Parse(in, noteOpts())
			require.NotNil(t, r.Deadline, "no deadline for %q", in)
			assert.Nil(t, r.Due)
			assert.Equal(t, c.want, r.Deadline.String())
			assert.Equal(t, "", r.Title)
		})
	}
}

// TestRemindWorksInNotes: bare dates are off in notes, remind <date> still sets the due date.
func TestRemindWorksInNotes(t *testing.T) {
	for _, c := range append(append([]dateCase{}, dateCases...), timeCases...) {
		in := "remind " + c.in
		t.Run(in, func(t *testing.T) {
			r := Parse(in, noteOpts())
			require.NotNil(t, r.Due, "no due for %q", in)
			assert.Equal(t, c.want, r.Due.String())
			assert.Equal(t, "", r.Title)
		})
	}
	r := Parse("remind me fri 3pm to call", noteOpts())
	assert.Equal(t, "2026-10-16 15:00", whenStr(r.Due))
	assert.Equal(t, "to call", r.Title)
	r = Parse("call Sam remind next week about the deal", noteOpts())
	assert.Equal(t, "2026-10-19", whenStr(r.Due))
	assert.Equal(t, "call Sam about the deal", r.Title)
}

func TestBareDatesAreNotReadInNotes(t *testing.T) {
	for _, text := range []string{"call fri 3pm", "see you tomorrow", "next week we meet", "in 3 days", "mar 4", "2026-11-01", "noon", "on fri", "at 3pm"} {
		r := Parse(text, noteOpts())
		assert.Nil(t, r.Due, text)
		assert.Equal(t, text, r.Title, text)
	}
}

func TestAtAndOnLeadADate(t *testing.T) {
	r := Parse("meet at 3pm", opts())
	assert.Equal(t, "2026-10-15 15:00", whenStr(r.Due))
	assert.Equal(t, "meet", r.Title)
	r = Parse("dinner on fri at 7pm", opts())
	assert.Equal(t, "2026-10-16 19:00", whenStr(r.Due))
	assert.Equal(t, "dinner", r.Title)
	r = Parse("lunch tomorrow at noon with Sam", opts())
	assert.Equal(t, "2026-10-16 12:00", whenStr(r.Due))
	assert.Equal(t, "lunch with Sam", r.Title)
}

func TestDeadlineAndDueTogether(t *testing.T) {
	r := Parse("ship it tue by fri", opts())
	assert.Equal(t, "2026-10-20", whenStr(r.Due))
	assert.Equal(t, "2026-10-16", whenStr(r.Deadline))
	assert.Equal(t, "ship it", r.Title)
	r = Parse("report by friday 5pm", opts())
	assert.Equal(t, "2026-10-16 17:00", whenStr(r.Deadline))
	assert.Nil(t, r.Due)
	assert.Equal(t, "report", r.Title)
}

// TestNotSyntax lists text that looks like syntax but is not. The title must stay unchanged, even
// with plain-word dates switched on.
func TestNotSyntax(t *testing.T) {
	texts := []string{
		// emails, phones, urls
		"mail jane@example.com now",
		"jane@example.com",
		"a@b.co",
		"cc: bob+tag@example.com",
		"call +1 555 0100",
		"phone: +15551234567",
		"+1-555-0100",
		"see https://example.com/#top",
		"see https://example.com/a#b",
		"@",
		"@ home",
		"@1",
		"@-x",
		"@_x",
		"@.",
		"mid@word",
		// symbols
		"price is $5 * 3",
		"2 * 3 = 6",
		"C++ and C# are languages",
		"x^2 + y^2",
		"AT&T and R&D",
		"Q&A session",
		"fish & chips",
		"hello!",
		"wow! really? yes!",
		"half!done",
		"!",
		"! important",
		"!!!!",
		"!x",
		"!urgentish",
		"!6",
		"!0",
		"approximately ~5 items",
		"about ~ half",
		"~",
		"~abc",
		"~0m",
		"~10x",
		"~99999m",
		"a~b",
		"#",
		"# heading",
		"#42",
		"ticket #1234 is open",
		"#1st",
		"#-dash",
		"#_under",
		"issue#12",
		"back\\slash",
		"path C:\\temp\\new",
		// English that is not a date, duration or repeat
		"by the way",
		"written by Sam",
		"by",
		"by 25:00",
		"remind me later",
		"remind",
		"for the team",
		"for 5 people",
		"for 0m",
		"for",
		"every time I see him",
		"every",
		"every 3",
		"every and",
		"every other thing",
		// dates that need more than one word or a number
		"may be late",
		"I may call",
		"the sat nav",
		"Sat down with Sam",
		"call Sun Microsystems",
		"met Mon Mothma",
		"Tom called",
		"tod",
		"tom",
		"now",
		"1/2 cup of flour",
		"3/4 done",
		"10d",
		"2w",
		"next",
		"next foo",
		"this week",
		"in",
		"in days",
		"in 0 days",
		"24:00 is not a time",
		"13pm",
		"0am",
		"2026-02-30",
		"2026-13-01",
		"feb 30",
		"sept",
		"mar",
	}
	for _, text := range texts {
		t.Run(text, func(t *testing.T) {
			r := Parse(text, opts())
			assert.Equal(t, strings.TrimSpace(text), r.Title, "title changed: %+v", r.Spans)
			assert.True(t, r.Empty(), "spans: %+v", r.Spans)
			assert.Nil(t, r.Due)
			assert.Nil(t, r.Deadline)
			assert.Empty(t, r.Tags)
			assert.Empty(t, r.People)
			assert.Zero(t, r.DurationMin)
			assert.Empty(t, r.Cadence)
			assert.Empty(t, r.Priority)
			assert.Empty(t, r.Recurrence)
		})
	}
}

func TestSeveralDayNamesNeedLowerCaseMidSentence(t *testing.T) {
	// A capitalised short day name inside a sentence is a name. Lower case, full names and the
	// start of the text are dates.
	for in, want := range map[string]string{
		"call fri":        "2026-10-16",
		"call friday":     "2026-10-16",
		"call Friday":     "2026-10-16",
		"Fri call":        "2026-10-16",
		"call by Fri":     "2026-10-16",
		"call next Sat":   "2026-10-24",
		"call on sat":     "2026-10-17",
		"call SATURDAY":   "2026-10-17",
		"call on Sun":     "2026-10-18",
		"lunch next sun":  "2026-10-25",
		"sat":             "2026-10-17",
		"call sat 3pm":    "2026-10-17 15:00",
		"report due thu":  "2026-10-22",
		"call Wednesday":  "2026-10-21",
		"call on Mon":     "2026-10-19",
		"call tues":       "2026-10-20",
		"remind us thur":  "2026-10-22",
		"call sat 3pm.":   "2026-10-17 15:00",
	} {
		r := Parse(in, opts())
		got := whenStr(r.Due)
		if r.Due == nil && r.Deadline != nil {
			got = whenStr(r.Deadline)
		}
		assert.Equal(t, want, got, in)
	}
	for _, in := range []string{"call Fri", "ask Sat", "hi Sun", "see Mon", "met Tue", "I sat down", "the sun is out", "met mon mothma", "call wed"} {
		assert.Nil(t, Parse(in, opts()).Due, in)
	}
}

func TestDurations(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"~1m", 1}, {"~5m", 5}, {"~15m", 15}, {"~30m", 30}, {"~30min", 30}, {"~30mins", 30}, {"~30minutes", 30},
		{"~45M", 45}, {"~90m", 90}, {"~1h", 60}, {"~2h", 120}, {"~2hr", 120}, {"~2hrs", 120}, {"~2hours", 120},
		{"~1H", 60}, {"~1.5h", 90}, {"~0.5h", 30}, {"~2.25h", 135}, {"~1h30m", 90}, {"~1h30", 90}, {"~2h15min", 135},
		{"~1h5m", 65}, {"~1d", 1440}, {"~2d", 2880}, {"~7d", 10080}, {"~168h", 10080}, {"~10080m", 10080},
		{"for 30m", 30}, {"for 45min", 45}, {"for 1h", 60}, {"for 1.5h", 90}, {"for 1h30m", 90},
		{"for 2 hours", 120}, {"for 1 hour", 60}, {"for 45 min", 45}, {"for 30 minutes", 30}, {"For 15m", 15},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			r := Parse(c.in, opts())
			assert.Equal(t, c.want, r.DurationMin)
			assert.Equal(t, "", r.Title)
		})
	}
	assert.Equal(t, "write report", Parse("write report ~2h", opts()).Title)
	assert.Equal(t, "write report", Parse("write report for 2 hours", opts()).Title)
	assert.Equal(t, "write report today", Parse("write report for 2 hours today", noteOpts()).Title)
	assert.Equal(t, "30m", formatMinutes(30))
	assert.Equal(t, "2h", formatMinutes(120))
	assert.Equal(t, "1h30m", formatMinutes(90))
	// A second duration is plain text.
	r := Parse("a ~30m b ~1h", opts())
	assert.Equal(t, 30, r.DurationMin)
	assert.Equal(t, "a b ~1h", r.Title)
	r = Parse("a for 30m b for 1h", opts())
	assert.Equal(t, 30, r.DurationMin)
	assert.Equal(t, "a b for 1h", r.Title)
}

func TestEveryIsARepeatAndSometimesACadence(t *testing.T) {
	cases := []struct{ in, rec, cad string }{
		{"every day", "every day", ""},
		{"every weekday", "every weekday", ""},
		{"every fri", "every fri", ""},
		{"every friday", "every friday", ""},
		{"every mon and wed", "every mon and wed", ""},
		{"every first monday", "every first monday", ""},
		{"every other week", "every other week", "14 days"},
		{"every 2 weeks", "every 2 weeks", "14 days"},
		{"every 2w", "every 2w", "14 days"},
		{"every week", "every week", "7 days"},
		{"every 1w", "every 1w", "7 days"},
		{"every 7d", "every 7d", "7 days"},
		{"every 6w", "every 6w", "42 days"},
		{"every month", "every month", "1 mon"},
		{"every 1m", "every 1m", "1 mon"},
		{"every 3m", "every 3m", "3 mons"},
		{"every 3 months", "every 3 months", "3 mons"},
		{"every 6m", "every 6m", "6 mons"},
		{"every year", "every year", "1 year"},
		{"every 1y", "every 1y", "1 year"},
		{"every 12m", "every 12m", "1 year"},
		{"every 3d", "every 3d", ""},
		{"every 5m", "every 5m", ""},
		{"EVERY Friday", "every friday", ""},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			r := Parse(c.in, opts())
			assert.Equal(t, c.rec, r.Recurrence)
			assert.Equal(t, c.cad, r.Cadence)
			assert.Equal(t, "", r.Title)
		})
	}
	r := Parse("gym every mon, wed, fri after work", opts())
	assert.Equal(t, "every mon wed fri", r.Recurrence)
	assert.Equal(t, "gym after work", r.Title)
	r = Parse("Sam Lee #friend every 3m", opts())
	assert.Equal(t, "Sam Lee", r.Title)
	assert.Equal(t, "3 mons", r.Cadence)
	r = Parse("water plants every day.", opts())
	assert.Equal(t, "every day", r.Recurrence)
	assert.Equal(t, "water plants.", r.Title)
}

func TestEveryAsCadenceOnlyWhenRepeatsAreSkipped(t *testing.T) {
	o := noteOpts()
	o.Skip = []Kind{KindRecurrence}
	r := Parse("great chat every 3m", o)
	assert.Equal(t, "3 mons", r.Cadence)
	assert.Empty(t, r.Recurrence)
	assert.Equal(t, "great chat", r.Title)
	assert.Equal(t, KindCadence, r.Spans[0].Kind)
	// Not a cadence: stays in the text.
	r = Parse("we meet every fri", o)
	assert.Equal(t, "we meet every fri", r.Title)
	assert.Empty(t, r.Recurrence)
	r = Parse("every 5m", o)
	assert.Equal(t, "every 5m", r.Title)

	// Skipping the cadence leaves the repeat alone.
	o = noteOpts()
	o.Skip = []Kind{KindCadence}
	r = Parse("every 3m", o)
	assert.Equal(t, "every 3m", r.Recurrence)
	assert.Empty(t, r.Cadence)

	// Skipping both leaves the text.
	o.Skip = []Kind{KindCadence, KindRecurrence}
	r = Parse("every 3m", o)
	assert.Equal(t, "every 3m", r.Title)

	// A second repeat stays text.
	r = Parse("a every day b every week", opts())
	assert.Equal(t, "every day", r.Recurrence)
	assert.Equal(t, "a b every week", r.Title)
}

func TestEveryCadenceIsAValidKeepInTouchValue(t *testing.T) {
	valid := map[string]bool{"7 days": true, "14 days": true, "1 mon": true, "42 days": true, "3 mons": true, "6 mons": true, "1 year": true, "never": true}
	for _, in := range []string{"every 1w", "every 2w", "every 6w", "every 1m", "every 3m", "every 6m", "every 1y", "every week", "every month", "every year", "every 12m"} {
		assert.True(t, valid[Parse(in, opts()).Cadence], in)
	}
}

func TestPriorities(t *testing.T) {
	cases := []struct {
		in       string
		want     string
		tier     int
		taskPrio string
	}{
		{"!top", "top", 1, "high"}, {"!asap", "top", 1, "high"}, {"!urgent", "top", 1, "high"}, {"!1", "top", 1, "high"},
		{"!high", "high", 2, "high"}, {"!hi", "high", 2, "high"}, {"!2", "high", 2, "high"}, {"!!!", "high", 2, "high"},
		{"!medium", "medium", 3, "medium"}, {"!med", "medium", 3, "medium"}, {"!mid", "medium", 3, "medium"},
		{"!normal", "medium", 3, "medium"}, {"!3", "medium", 3, "medium"}, {"!!", "medium", 3, "medium"},
		{"!low", "low", 4, "low"}, {"!lo", "low", 4, "low"}, {"!4", "low", 4, "low"},
		{"!none", "none", 5, "low"}, {"!5", "none", 5, "low"},
		{"!HIGH", "high", 2, "high"}, {"!High", "high", 2, "high"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			r := Parse(c.in, opts())
			assert.Equal(t, c.want, r.Priority)
			assert.Equal(t, c.tier, r.Tier())
			assert.Equal(t, c.taskPrio, r.TaskPriority())
			assert.Equal(t, "", r.Title)
		})
	}
	assert.Equal(t, 0, Result{}.Tier())
	assert.Equal(t, "", Result{}.TaskPriority())
	r := Parse("fix the sink !!! today", opts())
	assert.Equal(t, "high", r.Priority)
	assert.Equal(t, "fix the sink", r.Title)
	r = Parse("call !high, then !low", opts())
	assert.Equal(t, "high", r.Priority)
	assert.Equal(t, "call, then !low", r.Title)
}

func TestTags(t *testing.T) {
	cases := []struct {
		in    string
		tags  []string
		title string
	}{
		{"#deal", []string{"deal"}, ""}, {"note #deal", []string{"deal"}, "note"}, {"#deal note", []string{"deal"}, "note"},
		{"a #x b", []string{"x"}, "a b"}, {"a #x #y b", []string{"x", "y"}, "a b"}, {"#a #b #c", []string{"a", "b", "c"}, ""},
		{"#front-end work", []string{"front-end"}, "work"}, {"#snake_case", []string{"snake_case"}, ""}, {"#v2", []string{"v2"}, ""},
		{"#Deal #deal", []string{"Deal"}, ""}, {"#DEAL", []string{"DEAL"}, ""},
		{"deal done #deal.", []string{"deal"}, "deal done."}, {"deal done #deal, ok", []string{"deal"}, "deal done, ok"},
		{"(#deal)", []string{"deal"}, ""}, {"call (re #deal) now", []string{"deal"}, "call (re) now"},
		{"#café talk", []string{"café"}, "talk"}, {"#naïve", []string{"naïve"}, ""}, {"#日本語", []string{"日本語"}, ""},
		{"x #a;", []string{"a"}, "x;"}, {"x #a!", []string{"a"}, "x!"}, {"x #a?", []string{"a"}, "x?"},
		{"line one\nline two #t", []string{"t"}, "line one\nline two"}, {"#t\nline", []string{"t"}, "line"},
		{"tab\t#t\tend", []string{"t"}, "tab\tend"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			r := Parse(c.in, opts())
			assert.Equal(t, c.tags, r.Tags)
			assert.Equal(t, c.title, r.Title)
		})
	}
}

func TestNewTagFlag(t *testing.T) {
	o := opts()
	o.KnownTags = []string{"friend", "Work"}
	r := Parse("hi #friend #work #Deal #deal #café", o)
	assert.Equal(t, []string{"friend", "work", "Deal", "café"}, r.Tags)
	assert.Equal(t, []string{"Deal", "café"}, r.NewTags)
	newSpans := 0
	for _, s := range r.Spans {
		if s.New {
			newSpans++
		}
	}
	assert.Equal(t, 3, newSpans, "every occurrence of an unknown tag is flagged: Deal, deal, café")
	r = Parse("#friend", opts())
	assert.Empty(t, r.NewTags)
	o.KnownTags = []string{}
	r = Parse("#x", o)
	assert.Equal(t, []string{"x"}, r.NewTags)
	o.KnownTags = []string{"cafe"}
	r = Parse("#café", o)
	assert.Empty(t, r.NewTags)
}

func TestPeople(t *testing.T) {
	cases := []struct {
		in     string
		people []string
		title  string
	}{
		{"@Sam", []string{"Sam"}, ""}, {"lunch @Sam", []string{"Sam"}, "lunch"}, {"@Sam lunch", []string{"Sam"}, "lunch"},
		{"@Sam @Bob", []string{"Sam", "Bob"}, ""}, {"@Sam_Lee", []string{"Sam Lee"}, ""}, {"@sam", []string{"sam"}, ""},
		{"@O'Brien", []string{"O'Brien"}, ""}, {"@Mary-Jane", []string{"Mary-Jane"}, ""}, {"@José", []string{"José"}, ""},
		{"@Zoë", []string{"Zoë"}, ""}, {"@Sam,", []string{"Sam"}, ","}, {"call @Sam.", []string{"Sam"}, "call."},
		{"(@Sam)", []string{"Sam"}, ""}, {"@\"Sam Lee\"", []string{"Sam Lee"}, ""},
		{"call @\"Sam Lee\" now", []string{"Sam Lee"}, "call now"},
		{"@\"Sam\" and @\"Bob Ray\"", []string{"Sam", "Bob Ray"}, "and"},
		{"@Sam @sam", []string{"Sam"}, ""}, {"@Sam1", []string{"Sam1"}, ""},
		{"lunch @Sam fri", []string{"Sam"}, "lunch fri"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			r := Parse(c.in, noteOpts())
			assert.Equal(t, c.people, r.People)
			assert.Equal(t, c.title, r.Title)
		})
	}
	// With dates on, "@Sam fri" reads Sam as a person and fri as a date.
	r := Parse("lunch @Sam fri", opts())
	assert.Equal(t, []string{"Sam"}, r.People)
	assert.Equal(t, "2026-10-16", whenStr(r.Due))
	assert.Equal(t, "lunch", r.Title)
}

func TestKnownFullNamesMatchTwoWords(t *testing.T) {
	o := noteOpts()
	o.People = []string{"Sam Lee", "Sam Ng", "José García", "Mary Jane Watson", "Bob"}
	cases := []struct {
		in     string
		people []string
		title  string
	}{
		{"call @Sam Lee tomorrow", []string{"Sam Lee"}, "call tomorrow"},
		{"call @sam lee tomorrow", []string{"sam lee"}, "call tomorrow"},
		{"call @Sam Ng", []string{"Sam Ng"}, "call"},
		{"call @Sam Smith", []string{"Sam"}, "call Smith"},
		{"@Sam", []string{"Sam"}, ""},
		{"@Jose Garcia hi", []string{"Jose Garcia"}, "hi"},
		{"@Mary Jane Watson went", []string{"Mary Jane Watson"}, "went"},
		{"@Mary Jane went", []string{"Mary"}, "Jane went"},
		{"@Bob Lee", []string{"Bob"}, "Lee"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			r := Parse(c.in, o)
			assert.Equal(t, c.people, r.People)
			assert.Equal(t, c.title, r.Title)
		})
	}
}

func TestEscapesAndQuotes(t *testing.T) {
	cases := []struct{ in, title string }{
		{`\#notatag`, "#notatag"},
		{`fix \#42 now #real`, "fix #42 now"},
		{`email \@Sam`, "email @Sam"},
		{`\!important`, "!important"},
		{`\~5 items`, "~5 items"},
		{`a \#x \#y b`, "a #x #y b"},
		{`(\#x)`, "(#x)"},
		{`path C:\temp`, `path C:\temp`},
		{`\\#x`, `\\#x`},
		{`say "fri #deal @Sam ~5m" please`, `say "fri #deal @Sam ~5m" please`},
		{`"#tag"`, `"#tag"`},
		{`"remind fri" to #real`, `"remind fri" to`},
		{`one "unterminated #tag`, `one "unterminated`},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			r := Parse(c.in, opts())
			assert.Equal(t, c.title, r.Title)
		})
	}
	r := Parse(`\#x #y`, opts())
	assert.Equal(t, []string{"y"}, r.Tags)
	assert.True(t, Parse(`\#x`, opts()).Empty(), "an escape alone is not a token")
	assert.Equal(t, KindEscape, Parse(`\#x`, opts()).Spans[0].Kind)
	// Quotes after @ belong to a name, and quoted words never join a date.
	assert.Equal(t, []string{"Sam Lee"}, Parse(`@"Sam Lee"`, opts()).People)
	r = Parse(`call fri "3pm"`, opts())
	require.NotNil(t, r.Due)
	assert.Equal(t, "", r.Due.Time, "the quoted 3pm is not joined to fri")
}

func TestDismissTurnsATokenBackIntoText(t *testing.T) {
	o := opts()
	o.Dismiss = []string{"fri", "#deal"}
	r := Parse("call fri #deal #other tomorrow", o)
	assert.Equal(t, []string{"other"}, r.Tags)
	assert.Equal(t, "2026-10-16", whenStr(r.Due), "tomorrow is still a date")
	assert.Equal(t, "call fri #deal", r.Title)
	o.Dismiss = []string{"FRI 3PM"}
	r = Parse("call fri 3pm", o)
	assert.Nil(t, r.Due)
	assert.Equal(t, "call fri 3pm", r.Title)
}

func TestDisabled(t *testing.T) {
	o := opts()
	o.Disabled = true
	text := "call @Sam fri #work ~15m !high every 3m by mon"
	r := Parse(text, o)
	assert.Equal(t, text, r.Title)
	assert.Empty(t, r.Spans)
	assert.Nil(t, r.Due)
	assert.Empty(t, r.Tags)
	assert.Equal(t, text, r.Input)
}

func TestSkipKinds(t *testing.T) {
	o := opts()
	o.Skip = []Kind{KindTag, KindDue}
	r := Parse("call Sam fri #work ~15m", o)
	assert.Empty(t, r.Tags)
	assert.Nil(t, r.Due)
	assert.Equal(t, 15, r.DurationMin)
	assert.Equal(t, "call Sam fri #work", r.Title)
	o.Skip = []Kind{KindDeadline}
	r = Parse("call by fri", o)
	assert.Nil(t, r.Deadline)
	assert.Equal(t, "call by", r.Title, "the date after by is a plain date now")
}

func TestCombinedExamples(t *testing.T) {
	r := Parse("Call Sam fri 3pm #work ~15m @Sam !high by mon every 3m", opts())
	assert.Equal(t, "Call Sam", r.Title)
	assert.Equal(t, "2026-10-16 15:00", whenStr(r.Due))
	assert.Equal(t, "2026-10-19", whenStr(r.Deadline))
	assert.Equal(t, []string{"work"}, r.Tags)
	assert.Equal(t, 15, r.DurationMin)
	assert.Equal(t, []string{"Sam"}, r.People)
	assert.Equal(t, "high", r.Priority)
	assert.Equal(t, "3 mons", r.Cadence)
	assert.Equal(t, "every 3m", r.Recurrence)

	r = Parse("Sam called about the deal #deal remind fri", noteOpts())
	assert.Equal(t, "Sam called about the deal", r.Title)
	assert.Equal(t, "2026-10-16", whenStr(r.Due))

	r = Parse("Send proposal tomorrow 3pm by fri for 2h !!! #work @Sam_Lee", opts())
	assert.Equal(t, "Send proposal", r.Title)
	assert.Equal(t, "2026-10-16 15:00", whenStr(r.Due))
	assert.Equal(t, "2026-10-16", whenStr(r.Deadline))
	assert.Equal(t, 120, r.DurationMin)
	assert.Equal(t, "high", r.Priority)
	assert.Equal(t, []string{"Sam Lee"}, r.People)

	r = Parse("lunch at Cafe Rio fri 12:30 for 1h", Options{Now: testNow, Dates: true})
	assert.Equal(t, "2026-10-16 12:30", whenStr(r.Due))
	assert.Equal(t, 60, r.DurationMin)
	assert.Equal(t, "lunch at Cafe Rio", r.Title, "an at-phrase is plain text")
}

func TestDatePhraseDoesNotSwallowTheTitle(t *testing.T) {
	cases := []struct{ in, title, due string }{
		{"call fri about the deal", "call about the deal", "2026-10-16"},
		{"call fri 3pm about the deal", "call about the deal", "2026-10-16 15:00"},
		{"call next week about the deal", "call about the deal", "2026-10-19"},
		{"call fri, then email", "call, then email", "2026-10-16"},
		{"call fri.", "call.", "2026-10-16"},
		{"call (fri)", "call", "2026-10-16"},
		{"fri call", "call", "2026-10-16"},
		{"tomorrow at 3pm call", "call", "2026-10-16 15:00"},
		{"call mar 4 about it", "call about it", "2027-03-04"},
		{"call in 3 days about it", "call about it", "2026-10-18"},
		{"call fri 3pm about it", "call about it", "2026-10-16 15:00"},
		{"call fri morning", "call morning", "2026-10-16"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			r := Parse(c.in, opts())
			assert.Equal(t, c.title, r.Title)
			assert.Equal(t, c.due, whenStr(r.Due))
		})
	}
}

func TestSecondDueThatCannotMergeStaysText(t *testing.T) {
	r := Parse("a fri b sat", opts())
	assert.Equal(t, "2026-10-16", whenStr(r.Due))
	assert.Equal(t, "a b sat", r.Title)
	r = Parse("a 3pm b 4pm", opts())
	assert.Equal(t, "2026-10-15 15:00", whenStr(r.Due))
	assert.Equal(t, "a b 4pm", r.Title)
	r = Parse("a fri b 3pm", opts())
	assert.Equal(t, "2026-10-16 15:00", whenStr(r.Due), "a date and a time far apart still join")
	assert.Equal(t, "a b", r.Title)
}

func TestSpans(t *testing.T) {
	text := "Call Sam fri 3pm #work ~15m @Sam !high every 3m by mon"
	r := Parse(text, opts())
	var got [][3]string
	for _, s := range r.Spans {
		assert.Equal(t, text[s.Start:s.End], s.Text)
		got = append(got, [3]string{string(s.Kind), s.Text, s.Value})
	}
	assert.Equal(t, [][3]string{
		{"due", "fri 3pm", "2026-10-16 15:00"},
		{"tag", "#work", "work"},
		{"duration", "~15m", "15m"},
		{"person", "@Sam", "Sam"},
		{"priority", "!high", "high"},
		{"recurrence", "every 3m", "every 3m"},
		{"deadline", "by mon", "2026-10-19"},
	}, got)
	assert.Equal(t, "Call Sam", r.Title)
}

func TestSpansUseByteOffsets(t *testing.T) {
	text := "café #naïve fri"
	r := Parse(text, opts())
	require.Len(t, r.Spans, 2)
	for _, s := range r.Spans {
		assert.Equal(t, text[s.Start:s.End], s.Text)
	}
	assert.Equal(t, "#naïve", r.Spans[0].Text)
	assert.Equal(t, "fri", r.Spans[1].Text)
}

func TestSplitDueSpansShowTheMergedValue(t *testing.T) {
	r := Parse("a fri b 3pm", opts())
	require.Len(t, r.Spans, 2)
	assert.Equal(t, "2026-10-16 15:00", r.Spans[0].Value)
	assert.Equal(t, "2026-10-16 15:00", r.Spans[1].Value)
}

func TestWhitespaceAndNewlinesSurvive(t *testing.T) {
	r := Parse("first line\n\nsecond   line  #t", opts())
	assert.Equal(t, "first line\n\nsecond   line", r.Title)
	assert.Equal(t, "padded", Parse("  padded  ", opts()).Title)
	assert.Equal(t, "", Parse("", opts()).Title)
	assert.Equal(t, "", Parse("   ", opts()).Title)
	assert.Equal(t, "hello", Parse("#a hello", opts()).Title)
	assert.Equal(t, "hello world", Parse("hello #a world", opts()).Title)
	assert.Equal(t, "hello  world", Parse("hello  #a  world", opts()).Title)
	assert.Equal(t, "", Parse("#a fri ~5m", opts()).Title)
}

func TestDeterministicForTheSameClock(t *testing.T) {
	text := "Call Sam fri 3pm #work ~15m @Sam !high every 3m by mon at HQ"
	o := opts()
	assert.Equal(t, Parse(text, o), Parse(text, o))
}

func TestAtPlaceIsPlainText(t *testing.T) {
	r := Parse("lunch at Cafe Rio", Options{Now: testNow})
	assert.Equal(t, "lunch at Cafe Rio", r.Title)
	assert.True(t, r.Empty())
}

func TestClockMovesRelativeResults(t *testing.T) {
	later := Options{Now: time.Date(2026, 10, 16, 9, 30, 0, 0, time.UTC), Dates: true} // Friday morning
	assert.Equal(t, "2026-10-23", whenStr(Parse("fri", later).Due), "a weekday is strictly after today")
	assert.Equal(t, "2026-10-17", whenStr(Parse("tomorrow", later).Due))
	assert.Equal(t, "2026-10-16 15:00", whenStr(Parse("3pm", later).Due))
	assert.Equal(t, "2026-10-17 09:00", whenStr(Parse("9am", later).Due), "9:00 has passed at 9:30")
	assert.Equal(t, "2026-10-17 09:30", whenStr(Parse("9:30", later).Due), "the current minute counts as passed")
	assert.Equal(t, "2026-10-19", whenStr(Parse("next mon", later).Due))
	assert.Equal(t, "2026-10-19", whenStr(Parse("next week", later).Due))
}

func TestWeekdayMath(t *testing.T) {
	mon := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC) // a Monday
	names := []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}
	for start := 0; start < 7; start++ {
		now := mon.AddDate(0, 0, start)
		for target := 0; target < 7; target++ {
			o := Options{Now: now, Dates: true}
			ahead := (target - start + 7) % 7
			if ahead == 0 {
				ahead = 7
			}
			assert.Equal(t, now.AddDate(0, 0, ahead).Format("2006-01-02"), whenStr(Parse(names[target], o).Due), "from %s %s", names[start], names[target])
			assert.Equal(t, mon.AddDate(0, 0, 7+target).Format("2006-01-02"), whenStr(Parse("next "+names[target], o).Due), "from %s next %s", names[start], names[target])
			assert.Equal(t, now.AddDate(0, 0, (target-start+7)%7).Format("2006-01-02"), whenStr(Parse("this "+names[target], o).Due), "from %s this %s", names[start], names[target])
		}
	}
}

func TestMonthEndClamp(t *testing.T) {
	by := func(now time.Time, text string) string { return whenStr(Parse("by "+text, Options{Now: now}).Deadline) }
	jan31 := time.Date(2026, 1, 31, 10, 0, 0, 0, time.UTC)
	assert.Equal(t, "2026-02-28", by(jan31, "+1m"))
	assert.Equal(t, "2026-03-31", by(jan31, "+2m"))
	assert.Equal(t, "2027-01-31", by(jan31, "+1y"))
	leap := time.Date(2028, 2, 29, 10, 0, 0, 0, time.UTC)
	assert.Equal(t, "2029-02-28", by(leap, "+1y"))
	dec := time.Date(2026, 12, 15, 10, 0, 0, 0, time.UTC)
	assert.Equal(t, "2027-01-15", by(dec, "+1m"))
	assert.Equal(t, "2027-01-01", by(dec, "next month"))
}

func TestTimeZoneOfTheClockIsKept(t *testing.T) {
	loc := time.FixedZone("X", -5*3600)
	now := time.Date(2026, 10, 15, 23, 30, 0, 0, loc)
	o := Options{Now: now, Dates: true}
	assert.Equal(t, "2026-10-16", whenStr(Parse("tomorrow", o).Due))
	assert.Equal(t, "2026-10-16 00:15", whenStr(Parse("+45min", o).Due))
	assert.Equal(t, "2026-10-16 01:00", whenStr(Parse("1am", o).Due))
}

func TestZeroNowUsesTheRealClock(t *testing.T) {
	r := Parse("today", Options{Dates: true})
	require.NotNil(t, r.Due)
	assert.Equal(t, time.Now().Format("2006-01-02"), r.Due.Date)
}

func TestFold(t *testing.T) {
	assert.Equal(t, "jose garcia", Fold("  José   GARCÍA "))
	assert.Equal(t, "naive", Fold("Naïve"))
	assert.Equal(t, "", Fold(""))
}

func TestWhenString(t *testing.T) {
	assert.Equal(t, "2026-10-16", When{Date: "2026-10-16"}.String())
	assert.Equal(t, "2026-10-16 15:00", When{Date: "2026-10-16", Time: "15:00"}.String())
	assert.Equal(t, "15:00", When{Time: "15:00"}.String())
	assert.True(t, When{}.IsZero())
	assert.False(t, When{Date: "x"}.IsZero())
}

// TestInvariants generates many texts from token fragments and checks structural guarantees:
// spans are ordered, inside the input and do not overlap, their text matches the input, and
// parsing is deterministic.
func TestInvariants(t *testing.T) {
	frags := []string{
		"call", "Sam", "about", "the", "deal", "lunch", "note", "x", "a@b.co", "+1", "555-0100", "C++", "50%",
		"#deal", "#work", "#42", "#", "@Sam", "@Sam_Lee", "@", "@\"Sam Lee\"", "fri", "Fri", "3pm", "tomorrow", "next", "week", "may", "4",
		"by", "remind", "me", "for", "30m", "2", "hours", "every", "3m", "day", "mon", "and", "wed", "at", "Cafe", "Rio",
		"~30m", "~", "~5", "!high", "!!", "!!!", "!", "!x", "\\#esc", "(#t)", "[@Bob]", "\"#q\"", "\"quoted", "text\"", "fri,", "mon.",
		"Mar", "2026-11-01", "11/2", "naïve", "#café", "@José", "\n", "\t", "  ", "...", "??", "-", "—", "~1h30m", "in", "days", "noon",
		"ok.", "1/2", "now", "tom", "Tom",
	}
	rng := rand.New(rand.NewSource(7))
	{
		for _, dates := range []bool{false, true} {
			o := opts()
			o.Dates = dates
			o.People = []string{"Sam Lee", "José García"}
			for n := 0; n < 400; n++ {
				count := 1 + rng.Intn(9)
				parts := make([]string, count)
				for i := range parts {
					parts[i] = frags[rng.Intn(len(frags))]
				}
				text := strings.Join(parts, " ")
				r := Parse(text, o)
				require.Equal(t, r, Parse(text, o), text)
				last := 0
				for _, s := range r.Spans {
					require.GreaterOrEqual(t, s.Start, last, "overlap or order in %q: %+v", text, r.Spans)
					require.Greater(t, s.End, s.Start, text)
					require.LessOrEqual(t, s.End, len(text), text)
					require.Equal(t, text[s.Start:s.End], s.Text, text)
					last = s.End
				}
				require.LessOrEqual(t, len(r.Title), len(text), text)
				require.Equal(t, strings.TrimSpace(r.Title), r.Title, "title is trimmed: %q", text)
				if len(r.Spans) == 0 {
					require.Equal(t, strings.TrimSpace(text), strings.TrimSpace(r.Title), text)
				}
				// Without Dates a due date only comes from remind.
				if !dates && r.Due != nil {
					require.Contains(t, strings.ToLower(text), "remind", "a bare date in %q", text)
				}
			}
		}
	}
}

func TestNestedTagTokens(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"met Ann #work/clients/acme today", []string{"work/clients/acme"}},
		{"#work/clients and #work", []string{"work/clients", "work"}},
		{"#a/b-c/d_e", []string{"a/b-c/d_e"}},
		{"#work/ trailing slash is not part of it", nil},
		{"#work//clients", nil},
		{"#/work", nil},
		{"#work/1st", []string{"work/1st"}},
		{"see a/b and issue #12/13", nil},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			res := Parse(tt.in, noteOpts())
			assert.Equal(t, tt.want, res.Tags)
		})
	}
}
