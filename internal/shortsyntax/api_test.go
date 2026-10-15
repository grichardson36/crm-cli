package shortsyntax

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseBirthday(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"1985-03-04", "1985-03-04", true},
		{"--03-04", "--03-04", true},
		{"03-04", "--03-04", true},
		{"3/4", "--03-04", true},
		{"3/4/1985", "1985-03-04", true},
		{"Mar 4", "--03-04", true},
		{"mar 4", "--03-04", true},
		{"March 4", "--03-04", true},
		{"March 4th", "--03-04", true},
		{"4 Mar", "--03-04", true},
		{"4th march", "--03-04", true},
		{"Mar 4 1985", "1985-03-04", true},
		{"Mar 4, 1985", "1985-03-04", true},
		{"4 March 1985", "1985-03-04", true},
		{"feb 29", "--02-29", true},
		{"feb 29 2024", "2024-02-29", true},
		{"dec 31", "--12-31", true},
		{"jan 1", "--01-01", true},
		{"  Sept 9  ", "--09-09", true},
		{"feb 30", "", false},
		{"feb 29 2023", "", false},
		{"13/1", "", false},
		{"0/5", "", false},
		{"2023-02-30", "", false},
		{"2023-13-01", "", false},
		{"--13-01", "", false},
		{"--02-30", "", false},
		{"mar", "", false},
		{"mar 4 85", "", false},
		{"someday", "", false},
		{"", "", false},
		{"4", "", false},
		{"mar 0", "", false},
		{"mar 32", "", false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, ok := ParseBirthday(c.in)
			assert.Equal(t, c.ok, ok)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestParseCadenceAcceptsStoredAndShortForms(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"7 days", "7 days", true},
		{"14 days", "14 days", true},
		{"1 mon", "1 mon", true},
		{"42 days", "42 days", true},
		{"3 mons", "3 mons", true},
		{"6 mons", "6 mons", true},
		{"1 year", "1 year", true},
		{"never", "never", true},
		{"3m", "3 mons", true},
		{"3 m", "3 mons", true},
		{"6w", "42 days", true},
		{"monthly", "1 mon", true},
		{" Quarterly ", "3 mons", true},
		{"2 mons", "", false},
		{"5 days", "", false},
		{"", "", false},
		{"sometimes", "", false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, ok := ParseCadence(c.in)
			assert.Equal(t, c.ok, ok)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestParsePriorityAndTier(t *testing.T) {
	for in, want := range map[string]string{"High": "high", " top ": "top", "ASAP": "top", "5": "none", "med": "medium"} {
		got, ok := ParsePriority(in)
		assert.True(t, ok, in)
		assert.Equal(t, want, got, in)
	}
	_, ok := ParsePriority("urgent!!")
	assert.False(t, ok)
	for name, tier := range map[string]int{"top": 1, "high": 2, "medium": 3, "low": 4, "none": 5, "": 0, "bogus": 0} {
		assert.Equal(t, tier, TierOf(name), name)
	}
}

func TestParseDuration(t *testing.T) {
	n, ok := ParseDuration(" 1H30m ")
	assert.True(t, ok)
	assert.Equal(t, 90, n)
	_, ok = ParseDuration("soon")
	assert.False(t, ok)
	assert.Equal(t, "1h30m", FormatMinutes(90))
}

func TestNameSetMatchesLikePeople(t *testing.T) {
	ns := NewNameSet([]string{"Sam Lee", "José García", "", "Bob"})
	r := Parse("call +Jose Garcia now", Options{Now: testNow, Names: ns})
	assert.Equal(t, []string{"Jose Garcia"}, r.People)
	assert.Equal(t, "call now", r.Title)
	// The prepared set is reusable and wins over People.
	r = Parse("call +Sam Lee", Options{Now: testNow, Names: ns, People: []string{"Other Person"}})
	assert.Equal(t, []string{"Sam Lee"}, r.People)
	// An empty set does nothing.
	r = Parse("call +Sam Lee", Options{Now: testNow, Names: NewNameSet(nil)})
	assert.Equal(t, []string{"Sam"}, r.People)
	assert.Equal(t, "call Lee", r.Title)
}
