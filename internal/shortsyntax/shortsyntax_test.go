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

func opts() Options { return Options{Now: testNow} }

func TestTestNowIsAThursday(t *testing.T) {
	assert.Equal(t, time.Thursday, testNow.Weekday())
}

func whenStr(w *When) string {
	if w == nil {
		return ""
	}
	return w.String()
}

// TestDueExpressions checks the @ grammar. Each input is the whole text, so Title must be empty.
func TestDueExpressions(t *testing.T) {
	cases := []struct{ in, want string }{
		// relative days
		{"@today", "2026-10-15"},
		{"@tod", "2026-10-15"},
		{"@tomorrow", "2026-10-16"},
		{"@tom", "2026-10-16"},
		{"@tmr", "2026-10-16"},
		{"@tmrw", "2026-10-16"},
		{"@yesterday", "2026-10-14"},
		{"@yday", "2026-10-14"},
		{"@TOMORROW", "2026-10-16"},
		{"@Tomorrow", "2026-10-16"},
		// weekdays are always in the future
		{"@fri", "2026-10-16"},
		{"@friday", "2026-10-16"},
		{"@sat", "2026-10-17"},
		{"@sun", "2026-10-18"},
		{"@mon", "2026-10-19"},
		{"@tue", "2026-10-20"},
		{"@tues", "2026-10-20"},
		{"@wed", "2026-10-21"},
		{"@weds", "2026-10-21"},
		{"@thu", "2026-10-22"},
		{"@thur", "2026-10-22"},
		{"@thurs", "2026-10-22"},
		{"@thursday", "2026-10-22"},
		{"@Fri", "2026-10-16"},
		{"@FRIDAY", "2026-10-16"},
		// this / next / last
		{"@this thu", "2026-10-15"},
		{"@this fri", "2026-10-16"},
		{"@this mon", "2026-10-19"},
		{"@next mon", "2026-10-19"},
		{"@next thu", "2026-10-22"},
		{"@next fri", "2026-10-23"},
		{"@next sat", "2026-10-24"},
		{"@next sun", "2026-10-25"},
		{"@last fri", "2026-10-09"},
		{"@last thu", "2026-10-08"},
		{"@last mon", "2026-10-12"},
		{"@next week", "2026-10-19"},
		{"@next month", "2026-11-01"},
		{"@next year", "2027-01-01"},
		{"@nxt week", "2026-10-19"},
		// offsets
		{"@+1d", "2026-10-16"},
		{"@+3d", "2026-10-18"},
		{"@+1w", "2026-10-22"},
		{"@+2w", "2026-10-29"},
		{"@+1m", "2026-11-15"},
		{"@+3m", "2027-01-15"},
		{"@+1y", "2027-10-15"},
		{"@2w", "2026-10-29"},
		{"@10d", "2026-10-25"},
		{"@in 3 days", "2026-10-18"},
		{"@in 1 day", "2026-10-16"},
		{"@in 2 weeks", "2026-10-29"},
		{"@in 2 months", "2026-12-15"},
		{"@in 1 year", "2027-10-15"},
		{"@in 2w", "2026-10-29"},
		{"@in 3d", "2026-10-18"},
		// calendar dates
		{"@2026-12-25", "2026-12-25"},
		{"@2027-01-01", "2027-01-01"},
		{"@2026-10-15", "2026-10-15"},
		{"@2024-02-29", "2024-02-29"},
		{"@11/1", "2026-11-01"},
		{"@10/15", "2026-10-15"},
		{"@10/14", "2027-10-14"},
		{"@1/5", "2027-01-05"},
		{"@11/1/2027", "2027-11-01"},
		{"@mar 4", "2027-03-04"},
		{"@march 4", "2027-03-04"},
		{"@Mar 4", "2027-03-04"},
		{"@mar 4th", "2027-03-04"},
		{"@4 mar", "2027-03-04"},
		{"@4th march", "2027-03-04"},
		{"@oct 15", "2026-10-15"},
		{"@oct 16", "2026-10-16"},
		{"@oct 14", "2027-10-14"},
		{"@dec 25", "2026-12-25"},
		{"@sept 9", "2027-09-09"},
		{"@mar 4 2028", "2028-03-04"},
		{"@4 mar 2028", "2028-03-04"},
		{"@feb 29", "2028-02-29"},
		{"@jan 1", "2027-01-01"},
		// month end clamps
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			r := Parse(c.in, opts())
			if c.want == "" {
				return
			}
			require.NotNil(t, r.Due, "no due for %q", c.in)
			assert.Equal(t, c.want, r.Due.String())
			assert.Equal(t, "", r.Title)
		})
	}
}

func TestTimeExpressions(t *testing.T) {
	cases := []struct{ in, want string }{
		{"@3pm", "2026-10-15 15:00"},
		{"@3PM", "2026-10-15 15:00"},
		{"@3:30pm", "2026-10-15 15:30"},
		{"@11pm", "2026-10-15 23:00"},
		{"@12:30pm", "2026-10-15 12:30"},
		{"@12:01pm", "2026-10-15 12:01"},
		{"@15:00", "2026-10-15 15:00"},
		{"@23:59", "2026-10-15 23:59"},
		{"@16:45", "2026-10-15 16:45"},
		{"@9am", "2026-10-16 09:00"},
		{"@9:15am", "2026-10-16 09:15"},
		{"@09:00", "2026-10-16 09:00"},
		{"@0:30", "2026-10-16 00:30"},
		{"@12am", "2026-10-16 00:00"},
		{"@midnight", "2026-10-16 00:00"},
		{"@noon", "2026-10-16 12:00"},
		{"@11:59", "2026-10-16 11:59"},
		{"@12:01", "2026-10-15 12:01"},
		{"@3 pm", "2026-10-15 15:00"},
		{"@3:30 pm", "2026-10-15 15:30"},
		{"@9 am", "2026-10-16 09:00"},
		// date and time
		{"@fri 3pm", "2026-10-16 15:00"},
		{"@fri 15:00", "2026-10-16 15:00"},
		{"@fri at 3pm", "2026-10-16 15:00"},
		{"@3pm fri", "2026-10-16 15:00"},
		{"@tomorrow 14:00", "2026-10-16 14:00"},
		{"@tomorrow 9am", "2026-10-16 09:00"},
		{"@today 9am", "2026-10-15 09:00"},
		{"@today 3pm", "2026-10-15 15:00"},
		{"@next week 9am", "2026-10-19 09:00"},
		{"@next fri 10:30", "2026-10-23 10:30"},
		{"@mar 4 2pm", "2027-03-04 14:00"},
		{"@mar 4 2:30 pm", "2027-03-04 14:30"},
		{"@2026-11-02 08:15", "2026-11-02 08:15"},
		{"@11/2 8:15", "2026-11-02 08:15"},
		{"@+2w 4pm", "2026-10-29 16:00"},
		{"@in 3 days 9am", "2026-10-18 09:00"},
		// merged tokens
		{"@fri @3pm", "2026-10-16 15:00"},
		{"@3pm @fri", "2026-10-16 15:00"},
		{"@tomorrow @14:00", "2026-10-16 14:00"},
		{"@14:00 @tomorrow", "2026-10-16 14:00"},
		// relative to the clock
		{"@now", "2026-10-15 12:00"},
		{"@+2h", "2026-10-15 14:00"},
		{"@+30min", "2026-10-15 12:30"},
		{"@+90min", "2026-10-15 13:30"},
		{"@in 30 min", "2026-10-15 12:30"},
		{"@in 30 minutes", "2026-10-15 12:30"},
		{"@in 2 hours", "2026-10-15 14:00"},
		{"@in 1 hour", "2026-10-15 13:00"},
		{"@in 2h", "2026-10-15 14:00"},
		{"@in 45min", "2026-10-15 12:45"},
		{"@+13h", "2026-10-16 01:00"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			r := Parse(c.in, opts())
			require.NotNil(t, r.Due, "no due for %q", c.in)
			assert.Equal(t, c.want, r.Due.String())
			assert.Equal(t, "", r.Title)
		})
	}
}

func TestDeadlineUsesTheSameGrammar(t *testing.T) {
	cases := []struct{ in, want string }{
		{"!fri", "2026-10-16"},
		{"!tomorrow", "2026-10-16"},
		{"!next week", "2026-10-19"},
		{"!+2w", "2026-10-29"},
		{"!mar 4", "2027-03-04"},
		{"!fri 5pm", "2026-10-16 17:00"},
		{"!2026-12-31", "2026-12-31"},
		{"!5pm", "2026-10-15 17:00"},
		{"!9am", "2026-10-16 09:00"},
		{"!fri !5pm", "2026-10-16 17:00"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			r := Parse(c.in, opts())
			require.NotNil(t, r.Deadline)
			assert.Nil(t, r.Due)
			assert.Equal(t, c.want, r.Deadline.String())
		})
	}
	r := Parse("ship it @mon !fri", opts())
	assert.Equal(t, "2026-10-19", whenStr(r.Due))
	assert.Equal(t, "2026-10-16", whenStr(r.Deadline))
	assert.Equal(t, "ship it", r.Title)
}

// TestNotSyntax lists text that looks like syntax but is not. The title must stay unchanged.
func TestNotSyntax(t *testing.T) {
	texts := []string{
		// emails, phones, urls
		"mail jane@example.com now",
		"jane@example.com",
		"a@b.co",
		"cc: bob+tag@example.com",
		"call +1 555 0100",
		"call +44 20 7946 0958",
		"phone: +15551234567",
		"+1",
		"+1-555-0100",
		"see https://example.com/#top",
		"see https://example.com/a#b",
		"price is $5 * 3",
		"2 * 3 = 6",
		"C++ and C# are languages",
		"a+b",
		"x^2 + y^2",
		"AT&T and R&D",
		"Q&A session",
		"fish & chips",
		"Tom & Jerry",
		"hello!",
		"wow! really? yes!",
		"approximately ~5 items",
		"about ~ half",
		"~",
		"~abc",
		"~0m",
		"~10x",
		"~99999m",
		"#",
		"# heading",
		"#42",
		"ticket #1234 is open",
		"#1st",
		"#-dash",
		"#_under",
		"issue#12",
		"@",
		"@ home",
		"@home",
		"@sam",
		"@Sam called",
		"@3",
		"@25:00",
		"@13pm",
		"@0am",
		"@00pm",
		"@12:60",
		"@2026-02-30",
		"@2026-13-01",
		"@2026-00-10",
		"@13/45",
		"@0/5",
		"@feb 30",
		"@feb 31",
		"@may",
		"@next",
		"@next foo",
		"@this week",
		"@in",
		"@in 0 days",
		"@in days",
		"@0d",
		"@+0w",
		"@1234d",
		"*",
		"*bold*",
		"*5d",
		"*2m",
		"*0",
		"*3",
		"*hello",
		"*13m",
		"*2y",
		"*7",
		"^",
		"^2x",
		"^6",
		"^0",
		"^hello",
		"^urgentish",
		"+",
		"+ Sam",
		"++Sam",
		"+1Sam",
		"+_x",
		"+-x",
		"+Sam@x.com",
		"+Sam#x",
		"&",
		"&amp;",
		"&nbsp;",
		"&1",
		"back\\slash",
		"path C:\\temp\\new",
		"\\n",
		"tag#notatag",
		"mid@word",
		"half!done",
		"star*power",
		"a~b",
		"a^b",
	}
	for _, text := range texts {
		t.Run(text, func(t *testing.T) {
			r := Parse(text, opts())
			assert.Equal(t, strings.TrimSpace(text), r.Title, "title changed")
			assert.True(t, r.Empty(), "spans: %+v", r.Spans)
			assert.Nil(t, r.Due)
			assert.Nil(t, r.Deadline)
			assert.Empty(t, r.Tags)
			assert.Empty(t, r.People)
			assert.Zero(t, r.DurationMin)
			assert.Empty(t, r.Cadence)
			assert.Empty(t, r.Priority)
		})
	}
}

func TestDurations(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"~1m", 1},
		{"~5m", 5},
		{"~15m", 15},
		{"~30m", 30},
		{"~30min", 30},
		{"~30mins", 30},
		{"~30minutes", 30},
		{"~45M", 45},
		{"~90m", 90},
		{"~1h", 60},
		{"~2h", 120},
		{"~2hr", 120},
		{"~2hrs", 120},
		{"~2hours", 120},
		{"~1H", 60},
		{"~1.5h", 90},
		{"~0.5h", 30},
		{"~2.25h", 135},
		{"~1h30m", 90},
		{"~1h30", 90},
		{"~2h15min", 135},
		{"~1h5m", 65},
		{"~1d", 1440},
		{"~2d", 2880},
		{"~7d", 10080},
		{"~168h", 10080},
		{"~10080m", 10080},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			r := Parse(c.in, opts())
			assert.Equal(t, c.want, r.DurationMin)
			assert.Equal(t, "", r.Title)
		})
	}
	assert.Equal(t, "write report", Parse("write report ~2h", opts()).Title)
	assert.Equal(t, "30m", formatMinutes(30))
	assert.Equal(t, "2h", formatMinutes(120))
	assert.Equal(t, "1h30m", formatMinutes(90))
	assert.Equal(t, "1m", formatMinutes(1))
	// A second duration is plain text.
	r := Parse("a ~30m b ~1h", opts())
	assert.Equal(t, 30, r.DurationMin)
	assert.Equal(t, "a b ~1h", r.Title)
}

func TestCadences(t *testing.T) {
	cases := []struct{ in, want string }{
		{"*1w", "7 days"},
		{"*7d", "7 days"},
		{"*7days", "7 days"},
		{"*weekly", "7 days"},
		{"*2w", "14 days"},
		{"*14d", "14 days"},
		{"*biweekly", "14 days"},
		{"*fortnightly", "14 days"},
		{"*6w", "42 days"},
		{"*42d", "42 days"},
		{"*1m", "1 mon"},
		{"*1mo", "1 mon"},
		{"*1mon", "1 mon"},
		{"*1month", "1 mon"},
		{"*monthly", "1 mon"},
		{"*3m", "3 mons"},
		{"*3mo", "3 mons"},
		{"*3mons", "3 mons"},
		{"*3months", "3 mons"},
		{"*quarterly", "3 mons"},
		{"*6m", "6 mons"},
		{"*6months", "6 mons"},
		{"*biannual", "6 mons"},
		{"*semiannual", "6 mons"},
		{"*1y", "1 year"},
		{"*1yr", "1 year"},
		{"*1year", "1 year"},
		{"*12m", "1 year"},
		{"*yearly", "1 year"},
		{"*annually", "1 year"},
		{"*annual", "1 year"},
		{"*never", "never"},
		{"*off", "never"},
		{"*none", "never"},
		{"*3M", "3 mons"},
		{"*Monthly", "1 mon"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			r := Parse(c.in, opts())
			assert.Equal(t, c.want, r.Cadence)
			assert.Equal(t, "", r.Title)
		})
	}
	r := Parse("Sam Lee #friend *3m", opts())
	assert.Equal(t, "Sam Lee", r.Title)
	assert.Equal(t, "3 mons", r.Cadence)
}

func TestEveryCadenceIsAValidKeepInTouchValue(t *testing.T) {
	valid := map[string]bool{"7 days": true, "14 days": true, "1 mon": true, "42 days": true, "3 mons": true, "6 mons": true, "1 year": true, "never": true}
	for _, in := range []string{"*1w", "*2w", "*6w", "*1m", "*3m", "*6m", "*1y", "*never", "*monthly", "*weekly", "*quarterly", "*12m", "*biweekly"} {
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
		{"^top", "top", 1, "high"},
		{"^asap", "top", 1, "high"},
		{"^urgent", "top", 1, "high"},
		{"^1", "top", 1, "high"},
		{"^high", "high", 2, "high"},
		{"^hi", "high", 2, "high"},
		{"^2", "high", 2, "high"},
		{"^medium", "medium", 3, "medium"},
		{"^med", "medium", 3, "medium"},
		{"^mid", "medium", 3, "medium"},
		{"^normal", "medium", 3, "medium"},
		{"^3", "medium", 3, "medium"},
		{"^low", "low", 4, "low"},
		{"^lo", "low", 4, "low"},
		{"^4", "low", 4, "low"},
		{"^none", "none", 5, "low"},
		{"^5", "none", 5, "low"},
		{"^HIGH", "high", 2, "high"},
		{"^High", "high", 2, "high"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			r := Parse(c.in, opts())
			assert.Equal(t, c.want, r.Priority)
			assert.Equal(t, c.tier, r.Tier())
			assert.Equal(t, c.taskPrio, r.TaskPriority())
		})
	}
	assert.Equal(t, 0, Result{}.Tier())
	assert.Equal(t, "", Result{}.TaskPriority())
}

func TestTags(t *testing.T) {
	cases := []struct {
		in    string
		tags  []string
		title string
	}{
		{"#deal", []string{"deal"}, ""},
		{"note #deal", []string{"deal"}, "note"},
		{"#deal note", []string{"deal"}, "note"},
		{"a #x b", []string{"x"}, "a b"},
		{"a #x #y b", []string{"x", "y"}, "a b"},
		{"#a #b #c", []string{"a", "b", "c"}, ""},
		{"#front-end work", []string{"front-end"}, "work"},
		{"#snake_case", []string{"snake_case"}, ""},
		{"#v2", []string{"v2"}, ""},
		{"#Deal #deal", []string{"Deal"}, ""},
		{"#DEAL", []string{"DEAL"}, ""},
		{"deal done #deal.", []string{"deal"}, "deal done."},
		{"deal done #deal, ok", []string{"deal"}, "deal done, ok"},
		{"(#deal)", []string{"deal"}, ""},
		{"call (re #deal) now", []string{"deal"}, "call (re) now"},
		{"\"#deal\"", []string{"deal"}, ""},
		{"#café talk", []string{"café"}, "talk"},
		{"#naïve", []string{"naïve"}, ""},
		{"#日本語", []string{"日本語"}, ""},
		{"x #a;", []string{"a"}, "x;"},
		{"x #a!", []string{"a"}, "x!"},
		{"x #a?", []string{"a"}, "x?"},
		{"line one\nline two #t", []string{"t"}, "line one\nline two"},
		{"#t\nline", []string{"t"}, "line"},
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

	// Without KnownTags nothing is flagged.
	r = Parse("#friend", opts())
	assert.Empty(t, r.NewTags)

	// An empty list means every tag is new.
	o.KnownTags = []string{}
	r = Parse("#x", o)
	assert.Equal(t, []string{"x"}, r.NewTags)

	// Accents are ignored when matching.
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
		{"+Sam", []string{"Sam"}, ""},
		{"&Sam", []string{"Sam"}, ""},
		{"lunch +Sam", []string{"Sam"}, "lunch"},
		{"+Sam lunch", []string{"Sam"}, "lunch"},
		{"+Sam +Bob", []string{"Sam", "Bob"}, ""},
		{"+Sam_Lee", []string{"Sam Lee"}, ""},
		{"+sam", []string{"sam"}, ""},
		{"+O'Brien", []string{"O'Brien"}, ""},
		{"+Mary-Jane", []string{"Mary-Jane"}, ""},
		{"+José", []string{"José"}, ""},
		{"+Zoë", []string{"Zoë"}, ""},
		{"+Sam,", []string{"Sam"}, ","},
		{"call +Sam.", []string{"Sam"}, "call."},
		{"(+Sam)", []string{"Sam"}, ""},
		{"+\"Sam Lee\"", []string{"Sam Lee"}, ""},
		{"call +\"Sam Lee\" now", []string{"Sam Lee"}, "call now"},
		{"&\"Sam Lee\"", []string{"Sam Lee"}, ""},
		{"+\"Sam\" and +\"Bob Ray\"", []string{"Sam", "Bob Ray"}, "and"},
		{"+Sam +sam", []string{"Sam"}, ""},
		{"+Sam1", []string{"Sam1"}, ""},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			r := Parse(c.in, opts())
			assert.Equal(t, c.people, r.People)
			assert.Equal(t, c.title, r.Title)
		})
	}
}

func TestKnownFullNamesMatchTwoWords(t *testing.T) {
	o := opts()
	o.People = []string{"Sam Lee", "Sam Ng", "José García", "Mary Jane Watson", "Bob"}
	cases := []struct {
		in     string
		people []string
		title  string
	}{
		{"call +Sam Lee tomorrow", []string{"Sam Lee"}, "call tomorrow"},
		{"call +sam lee tomorrow", []string{"sam lee"}, "call tomorrow"},
		{"call +Sam Ng", []string{"Sam Ng"}, "call"},
		{"call +Sam Smith", []string{"Sam"}, "call Smith"},
		{"+Sam", []string{"Sam"}, ""},
		{"+Jose Garcia hi", []string{"Jose Garcia"}, "hi"},
		{"+Mary Jane Watson went", []string{"Mary Jane Watson"}, "went"},
		{"+Mary Jane went", []string{"Mary"}, "Jane went"},
		{"+Bob Lee", []string{"Bob"}, "Lee"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			r := Parse(c.in, o)
			assert.Equal(t, c.people, r.People)
			assert.Equal(t, c.title, r.Title)
		})
	}
}

func TestEscapes(t *testing.T) {
	cases := []struct{ in, title string }{
		{`\#notatag`, "#notatag"},
		{`fix \#42 now #real`, "fix #42 now"},
		{`email \@fri`, "email @fri"},
		{`\!important`, "!important"},
		{`\~5 items`, "~5 items"},
		{`\*3m`, "*3m"},
		{`\+Sam`, "+Sam"},
		{`\&Sam`, "&Sam"},
		{`\^high`, "^high"},
		{`a \#x \#y b`, "a #x #y b"},
		{`(\#x)`, "(#x)"},
		{`path C:\temp`, `path C:\temp`},
		{`\\#x`, `\\#x`},
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
}

func TestDisabled(t *testing.T) {
	o := opts()
	o.Disabled = true
	text := "call Sam @fri #work ~15m ^high *3m"
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
	r := Parse("call Sam @fri #work ~15m", o)
	assert.Empty(t, r.Tags)
	assert.Nil(t, r.Due)
	assert.Equal(t, 15, r.DurationMin)
	assert.Equal(t, "call Sam @fri #work", r.Title)
}

func TestCombinedExample(t *testing.T) {
	r := Parse("Call Sam @fri #work ~15m +Sam ^high", opts())
	assert.Equal(t, "Call Sam", r.Title)
	assert.Equal(t, "2026-10-16", whenStr(r.Due))
	assert.Equal(t, []string{"work"}, r.Tags)
	assert.Equal(t, 15, r.DurationMin)
	assert.Equal(t, []string{"Sam"}, r.People)
	assert.Equal(t, "high", r.Priority)

	r = Parse("Sam called about the deal #deal @fri", opts())
	assert.Equal(t, "Sam called about the deal", r.Title)

	r = Parse("Send proposal @tomorrow 3pm !fri ~2h ^high #work +Sam_Lee *monthly", opts())
	assert.Equal(t, "Send proposal", r.Title)
	assert.Equal(t, "2026-10-16 15:00", whenStr(r.Due))
	assert.Equal(t, "2026-10-16", whenStr(r.Deadline))
	assert.Equal(t, 120, r.DurationMin)
	assert.Equal(t, "1 mon", r.Cadence)
	assert.Equal(t, []string{"Sam Lee"}, r.People)
}

func TestDatePhraseDoesNotSwallowTheTitle(t *testing.T) {
	cases := []struct{ in, title, due string }{
		{"call @fri about the deal", "call about the deal", "2026-10-16"},
		{"call @fri 3pm about the deal", "call about the deal", "2026-10-16 15:00"},
		{"call @next week about the deal", "call about the deal", "2026-10-19"},
		{"call @fri, then email", "call, then email", "2026-10-16"},
		{"call @fri.", "call.", "2026-10-16"},
		{"call (@fri)", "call", "2026-10-16"},
		{"@fri call", "call", "2026-10-16"},
		{"@tomorrow at 3pm call", "call", "2026-10-16 15:00"},
		{"call @mar 4 about it", "call about it", "2027-03-04"},
		{"call @in 3 days about it", "call about it", "2026-10-18"},
		{"call @fri @3pm about it", "call about it", "2026-10-16 15:00"},
		{"call @fri sat", "call sat", "2026-10-16"},
		{"call @fri morning", "call morning", "2026-10-16"},
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
	r := Parse("a @fri b @sat", opts())
	assert.Equal(t, "2026-10-16", whenStr(r.Due))
	assert.Equal(t, "a b @sat", r.Title)

	r = Parse("a @3pm b @4pm", opts())
	assert.Equal(t, "2026-10-15 15:00", whenStr(r.Due))
	assert.Equal(t, "a b @4pm", r.Title)

	r = Parse("a @+2h b @fri", opts())
	assert.Equal(t, "2026-10-15 14:00", whenStr(r.Due))
	assert.Equal(t, "a b @fri", r.Title)
}

func TestSecondSingletonTokensStayText(t *testing.T) {
	r := Parse("a *3m b *6m ^high c ^low", opts())
	assert.Equal(t, "3 mons", r.Cadence)
	assert.Equal(t, "high", r.Priority)
	assert.Equal(t, "a b *6m c ^low", r.Title)
}

func TestPhrasesAreOffByDefault(t *testing.T) {
	text := "lunch at Cafe Rio every friday"
	r := Parse(text, opts())
	assert.Equal(t, text, r.Title)
	assert.Empty(t, r.Location)
	assert.Empty(t, r.Recurrence)
}

func TestLocationPhrase(t *testing.T) {
	o := opts()
	o.Phrases = true
	cases := []struct{ in, loc, title string }{
		{"lunch at Cafe Rio", "Cafe Rio", "lunch"},
		{"lunch at Cafe Rio @fri 12:30", "Cafe Rio", "lunch"},
		{"lunch at Cafe Rio, then walk", "Cafe Rio", "lunch, then walk"},
		{"at the office", "the office", ""},
		{"meet at home #family", "home", "meet"},
		{"AT Cafe Rio", "Cafe Rio", ""},
		{"standup at HQ ~15m", "HQ", "standup"},
		{"drinks at 3pm", "", "drinks at 3pm"},
		{"drinks at", "", "drinks at"},
		{"drinks at @fri", "", "drinks at"},
		{"at . .", "", "at . ."},
		{"look at one two three four five six", "one two three four five", "look six"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			r := Parse(c.in, o)
			assert.Equal(t, c.loc, r.Location)
			assert.Equal(t, c.title, r.Title)
		})
	}
}

func TestRecurrencePhrase(t *testing.T) {
	o := opts()
	o.Phrases = true
	cases := []struct{ in, rec, title string }{
		{"standup every weekday", "every weekday", "standup"},
		{"standup every weekday @9:15", "every weekday", "standup"},
		{"pay rent every month", "every month", "pay rent"},
		{"every 2 weeks", "every 2 weeks", ""},
		{"every 2w", "every 2w", ""},
		{"every other week", "every other week", ""},
		{"gym every mon and wed", "every mon and wed", "gym"},
		{"gym every mon, wed, fri", "every mon wed fri", "gym"},
		{"water plants every day.", "every day", "water plants."},
		{"review every first monday", "every first monday", "review"},
		{"every friday at 5pm", "every friday", "at 5pm"},
		{"every year", "every year", ""},
		{"EVERY Friday", "every friday", ""},
		{"every time I see him", "", "every time I see him"},
		{"every", "", "every"},
		{"every 3", "", "every 3"},
		{"every and", "", "every and"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			r := Parse(c.in, o)
			assert.Equal(t, c.rec, r.Recurrence)
			assert.Equal(t, c.title, r.Title)
		})
	}
}

func TestSpans(t *testing.T) {
	text := "Call Sam @fri 3pm #work ~15m +Sam ^high *3m !mon"
	r := Parse(text, opts())
	var got [][3]string
	for _, s := range r.Spans {
		assert.Equal(t, text[s.Start:s.End], s.Text)
		got = append(got, [3]string{string(s.Kind), s.Text, s.Value})
	}
	assert.Equal(t, [][3]string{
		{"due", "@fri 3pm", "2026-10-16 15:00"},
		{"tag", "#work", "work"},
		{"duration", "~15m", "15m"},
		{"person", "+Sam", "Sam"},
		{"priority", "^high", "high"},
		{"cadence", "*3m", "3 mons"},
		{"deadline", "!mon", "2026-10-19"},
	}, got)
	assert.Equal(t, "Call Sam", r.Title)
}

func TestSpansUseByteOffsets(t *testing.T) {
	text := "café #naïve @fri"
	r := Parse(text, opts())
	require.Len(t, r.Spans, 2)
	for _, s := range r.Spans {
		assert.Equal(t, text[s.Start:s.End], s.Text)
	}
	assert.Equal(t, "#naïve", r.Spans[0].Text)
	assert.Equal(t, "@fri", r.Spans[1].Text)
}

func TestSpansExcludeTrailingPunctuationAndBrackets(t *testing.T) {
	r := Parse("see (#deal), then @fri.", opts())
	require.Len(t, r.Spans, 2)
	assert.Equal(t, "#deal", r.Spans[0].Text)
	assert.Equal(t, "@fri", r.Spans[1].Text)
}

func TestSplitDueSpansShowTheMergedValue(t *testing.T) {
	r := Parse("a @fri b @3pm", opts())
	require.Len(t, r.Spans, 2)
	assert.Equal(t, "2026-10-16 15:00", r.Spans[0].Value)
	assert.Equal(t, "2026-10-16 15:00", r.Spans[1].Value)
}

func TestWhitespaceAndNewlinesSurvive(t *testing.T) {
	r := Parse("first line\n\nsecond   line  #t", opts())
	assert.Equal(t, "first line\n\nsecond   line", r.Title)
	r = Parse("  padded  ", opts())
	assert.Equal(t, "padded", r.Title)
	r = Parse("", opts())
	assert.Equal(t, "", r.Title)
	r = Parse("   ", opts())
	assert.Equal(t, "", r.Title)
}

func TestOnlyTokensGiveAnEmptyTitle(t *testing.T) {
	r := Parse("#a @fri ~5m", opts())
	assert.Equal(t, "", r.Title)
}

func TestTokenAtStartAndEndAreTrimmed(t *testing.T) {
	assert.Equal(t, "hello", Parse("#a hello", opts()).Title)
	assert.Equal(t, "hello", Parse("hello #a", opts()).Title)
	assert.Equal(t, "hello world", Parse("hello #a world", opts()).Title)
	assert.Equal(t, "hello  world", Parse("hello  #a  world", opts()).Title)
}

func TestDeterministicForTheSameClock(t *testing.T) {
	text := "Call Sam @fri 3pm #work ~15m +Sam ^high *3m !mon every day at HQ"
	o := opts()
	o.Phrases = true
	a, b := Parse(text, o), Parse(text, o)
	assert.Equal(t, a, b)
}

func TestClockMovesRelativeResults(t *testing.T) {
	later := Options{Now: time.Date(2026, 10, 16, 9, 30, 0, 0, time.UTC)} // Friday morning
	r := Parse("@fri", later)
	assert.Equal(t, "2026-10-23", whenStr(r.Due), "a weekday is strictly after today")
	r = Parse("@tomorrow", later)
	assert.Equal(t, "2026-10-17", whenStr(r.Due))
	r = Parse("@3pm", later)
	assert.Equal(t, "2026-10-16 15:00", whenStr(r.Due))
	r = Parse("@9am", later)
	assert.Equal(t, "2026-10-17 09:00", whenStr(r.Due), "9:00 has passed at 9:30")
	r = Parse("@9:30", later)
	assert.Equal(t, "2026-10-17 09:30", whenStr(r.Due), "the current minute counts as passed")
	r = Parse("@next mon", later)
	assert.Equal(t, "2026-10-19", whenStr(r.Due))
	r = Parse("@next week", later)
	assert.Equal(t, "2026-10-19", whenStr(r.Due))
}

func TestWeekdayMath(t *testing.T) {
	// For every starting weekday, check "@wd" and "@next wd" against hand-computed offsets.
	mon := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC) // a Monday
	names := []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}
	for start := 0; start < 7; start++ {
		now := mon.AddDate(0, 0, start)
		for target := 0; target < 7; target++ {
			r := Parse("@"+names[target], Options{Now: now})
			ahead := (target - start + 7) % 7
			if ahead == 0 {
				ahead = 7
			}
			want := now.AddDate(0, 0, ahead).Format("2006-01-02")
			assert.Equal(t, want, whenStr(r.Due), "from %s @%s", names[start], names[target])

			r = Parse("@next "+names[target], Options{Now: now})
			want = mon.AddDate(0, 0, 7+target).Format("2006-01-02")
			assert.Equal(t, want, whenStr(r.Due), "from %s @next %s", names[start], names[target])

			r = Parse("@this "+names[target], Options{Now: now})
			want = now.AddDate(0, 0, (target-start+7)%7).Format("2006-01-02")
			assert.Equal(t, want, whenStr(r.Due), "from %s @this %s", names[start], names[target])
		}
	}
}

func TestMonthEndClamp(t *testing.T) {
	now := time.Date(2026, 1, 31, 10, 0, 0, 0, time.UTC)
	assert.Equal(t, "2026-02-28", whenStr(Parse("@+1m", Options{Now: now}).Due))
	assert.Equal(t, "2026-03-31", whenStr(Parse("@+2m", Options{Now: now}).Due))
	assert.Equal(t, "2027-01-31", whenStr(Parse("@+1y", Options{Now: now}).Due))
	leap := time.Date(2028, 2, 29, 10, 0, 0, 0, time.UTC)
	assert.Equal(t, "2029-02-28", whenStr(Parse("@+1y", Options{Now: leap}).Due))
	dec := time.Date(2026, 12, 15, 10, 0, 0, 0, time.UTC)
	assert.Equal(t, "2027-01-15", whenStr(Parse("@+1m", Options{Now: dec}).Due))
	assert.Equal(t, "2027-01-01", whenStr(Parse("@next month", dec2(dec)).Due))
}

func dec2(t time.Time) Options { return Options{Now: t} }

func TestTimeZoneOfTheClockIsKept(t *testing.T) {
	loc := time.FixedZone("X", -5*3600)
	now := time.Date(2026, 10, 15, 23, 30, 0, 0, loc)
	assert.Equal(t, "2026-10-16", whenStr(Parse("@tomorrow", Options{Now: now}).Due))
	assert.Equal(t, "2026-10-16 00:15", whenStr(Parse("@+45min", Options{Now: now}).Due))
	assert.Equal(t, "2026-10-16 01:00", whenStr(Parse("@1am", Options{Now: now}).Due))
}

func TestZeroNowUsesTheRealClock(t *testing.T) {
	r := Parse("@today", Options{})
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
// spans are ordered, inside the input and do not overlap, their text matches the input, the title
// never contains an unescaped recognized token, and parsing is deterministic.
func TestInvariants(t *testing.T) {
	frags := []string{
		"call", "Sam", "about", "the", "deal", "lunch", "note", "x", "a@b.co", "+1", "555-0100", "C++", "50%",
		"#deal", "#work", "#42", "#", "@fri", "@3pm", "@tomorrow", "@next", "week", "@sam", "!mon", "!", "~30m", "~", "~5",
		"*3m", "*", "*bold*", "^high", "^", "+Sam", "+Sam_Lee", "&Bob", "at", "every", "day", "weekday", "3pm",
		"\\#esc", "(#t)", "[+Bob]", "\"#q\"", "fri,", "mon.", "Mar", "4", "2026-11-01", "11/2", "naïve", "#café", "+José",
		"\n", "\t", "  ", "...", "!!", "??", "-", "—", "~1h30m", "@in", "3", "days", "@noon", "ok.",
	}
	rng := rand.New(rand.NewSource(7))
	for _, phrases := range []bool{false, true} {
		o := opts()
		o.Phrases = phrases
		o.People = []string{"Sam Lee", "José García"}
		for n := 0; n < 600; n++ {
			count := 1 + rng.Intn(9)
			parts := make([]string, count)
			for i := range parts {
				parts[i] = frags[rng.Intn(len(frags))]
			}
			text := strings.Join(parts, " ")
			r := Parse(text, o)
			r2 := Parse(text, o)
			require.Equal(t, r, r2, text)
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
			// The title never holds text from a recognized span, apart from escapes.
			if len(r.Spans) == 0 {
				require.Equal(t, strings.TrimSpace(text), strings.TrimSpace(r.Title), text)
			}
			// Re-parsing the title finds no tags or people that the first pass already took.
			again := Parse(r.Title, o)
			for _, tag := range r.Tags {
				require.NotContains(t, again.Tags, tag, "tag %q left in title %q of %q", tag, r.Title, text)
			}
		}
	}
}
