package dedupe_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/dedupe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		name         string
		first, last  string
		wantF, wantL string
	}{
		{"plain", "Jane", "Smith", "jane", "smith"},
		{"case and accents", "JOSÉ", "Muñoz", "jose", "munoz"},
		{"punctuation", "Mary-Ann", "O'Brien", "mary", "obrien"},
		{"middle name in first", "John Michael", "Smith", "john", "smith"},
		{"middle name in last", "John", "Michael Smith", "john", "smith"},
		{"suffix", "John", "Smith Jr.", "john", "smith"},
		{"suffix with comma", "John", "Smith, PhD", "john", "smith"},
		{"prefix", "Dr. Jane", "Smith", "jane", "smith"},
		{"full name in first", "Jane Q Smith", "", "jane", "smith"},
		{"single name", "Madonna", "", "madonna", ""},
		{"honorific only first name", "Dr.", "Robert (Bob) Borer", "robert", "borer"},
		{"honorific with last name only", "Mr.", "Garis", "garis", ""},
		{"email stored as name", "john@topshelfcrane.com", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, l := dedupe.NormalizeName(tt.first, tt.last)
			assert.Equal(t, tt.wantF, f)
			assert.Equal(t, tt.wantL, l)
		})
	}
}

func TestKeys(t *testing.T) {
	assert.Equal(t, "5551234567", dedupe.PhoneKey("+1 (555) 123-4567"))
	assert.Equal(t, "5551234567", dedupe.PhoneKey("555.123.4567"))
	assert.Equal(t, "", dedupe.PhoneKey("12345"))
	assert.Equal(t, "a@b.com", dedupe.EmailKey("  A@B.com "))
	assert.Equal(t, "", dedupe.EmailKey("nope"))
	for _, v := range []string{"https://www.linkedin.com/in/Jane-Doe/?x=1", "linkedin.com/in/jane-doe", "jane-doe"} {
		assert.Equal(t, "jane-doe", dedupe.HandleKey(v), v)
	}
	assert.Equal(t, "jane", dedupe.HandleKey("@Jane"))
	assert.Equal(t, "jane", dedupe.HandleKey("https://twitter.com/jane"))
}

func p(id int64, first, last string) dedupe.Person {
	return dedupe.Person{ID: id, FirstName: first, LastName: last}
}

func find(min dedupe.Level, people ...dedupe.Person) []dedupe.Group {
	return dedupe.Find(people, dedupe.Options{Min: min})
}

func TestFind_Levels(t *testing.T) {
	tests := []struct {
		name   string
		people []dedupe.Person
		min    dedupe.Level
		want   dedupe.Level
		ids    []int64
	}{
		{"shared email ignoring case", []dedupe.Person{
			{ID: 1, FirstName: "Al", Emails: []string{"Jane@X.com"}},
			{ID: 2, FirstName: "Bo", Emails: []string{"jane@x.com"}},
		}, dedupe.Medium, dedupe.High, []int64{1, 2}},
		{"shared phone last 10 digits", []dedupe.Person{
			{ID: 1, FirstName: "Al", Phones: []string{"+1 555 123 4567"}},
			{ID: 2, FirstName: "Bo", Phones: []string{"(555) 123-4567"}},
		}, dedupe.Medium, dedupe.High, []int64{1, 2}},
		{"shared social handle", []dedupe.Person{
			{ID: 1, FirstName: "Al", Handles: map[string]string{"linkedin": "https://linkedin.com/in/al"}},
			{ID: 2, FirstName: "Bo", Handles: map[string]string{"linkedin": "linkedin.com/in/al/"}},
		}, dedupe.Medium, dedupe.High, []int64{1, 2}},
		{"same normalized name", []dedupe.Person{p(1, "José", "Muñoz"), p(2, "jose michael", "MUNOZ Jr")}, dedupe.Medium, dedupe.Medium, []int64{1, 2}},
		{"nickname", []dedupe.Person{p(1, "Bob", "Jones"), p(2, "Robert", "Jones")}, dedupe.Low, dedupe.Low, []int64{1, 2}},
		{"fuzzy spelling", []dedupe.Person{p(1, "Jonathan", "Hartwell"), p(2, "Jonathon", "Hartwell")}, dedupe.Fuzzy, dedupe.Fuzzy, []int64{1, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups := find(tt.min, tt.people...)
			require.Len(t, groups, 1)
			assert.Equal(t, tt.want, groups[0].Level)
			assert.Equal(t, tt.ids, groups[0].IDs())
			assert.NotEmpty(t, groups[0].Reasons)
		})
	}
}

func TestFind_MinFiltersLevels(t *testing.T) {
	people := []dedupe.Person{
		p(1, "Bob", "Jones"), p(2, "Robert", "Jones"), // low
		p(3, "Jonathan", "Hartwell"), p(4, "Jonathon", "Hartwell"), // fuzzy
	}
	assert.Empty(t, find(dedupe.Medium, people...))
	assert.Len(t, find(dedupe.Low, people...), 1)
	assert.Len(t, find(dedupe.Fuzzy, people...), 2)
	assert.Empty(t, dedupe.Find(people, dedupe.Options{}), "default minimum is medium")
}

func TestFind_FuzzyNeedsSameInitialAndThreshold(t *testing.T) {
	assert.Empty(t, find(dedupe.Fuzzy, p(1, "Aaron", "Hartwell"), p(2, "Karon", "Hartwell")))
	assert.Empty(t, find(dedupe.Fuzzy, p(1, "Jane", "Smith"), p(2, "June", "Smyth")), "two edits is below 0.88")
}

func TestFind_SocialConflict(t *testing.T) {
	a := dedupe.Person{ID: 1, FirstName: "Jane", LastName: "Smith", Handles: map[string]string{"linkedin": "linkedin.com/in/jane-a"}}
	b := dedupe.Person{ID: 2, FirstName: "Jane", LastName: "Smith", Handles: map[string]string{"linkedin": "linkedin.com/in/jane-b"}}
	assert.Empty(t, find(dedupe.Fuzzy, a, b), "different LinkedIn profiles are never grouped")

	for _, kind := range []string{"facebook", "twitter"} {
		a.Handles = map[string]string{kind: "one"}
		b.Handles = map[string]string{kind: "two"}
		a.Emails, b.Emails = []string{"x@y.com"}, []string{"x@y.com"}
		assert.Empty(t, find(dedupe.Fuzzy, a, b), "%s conflict beats a shared email", kind)
	}
	a.Emails, b.Emails = nil, nil

	// Only one side has a profile: no conflict.
	a.Handles = map[string]string{"linkedin": "linkedin.com/in/jane-a"}
	b.Handles = nil
	assert.Len(t, find(dedupe.Medium, a, b), 1)

	// Other kinds do not conflict.
	a.Handles = map[string]string{"instagram": "one"}
	b.Handles = map[string]string{"instagram": "two"}
	assert.Len(t, find(dedupe.Medium, a, b), 1)
}

func TestFind_SocialConflictBlocksTransitiveJoin(t *testing.T) {
	// 1-2 and 2-3 are linked, but 1 and 3 have different LinkedIn profiles.
	a := dedupe.Person{ID: 1, FirstName: "A", Emails: []string{"x@y.com"}, Handles: map[string]string{"linkedin": "one"}}
	b := dedupe.Person{ID: 2, FirstName: "B", Emails: []string{"x@y.com", "z@y.com"}}
	c := dedupe.Person{ID: 3, FirstName: "C", Emails: []string{"z@y.com"}, Handles: map[string]string{"linkedin": "two"}}
	groups := find(dedupe.Medium, a, b, c)
	require.Len(t, groups, 1)
	assert.Len(t, groups[0].Members, 2, "the third person must stay out of the group")
}

func TestFind_ArchivedExcluded(t *testing.T) {
	a := p(1, "Jane", "Smith")
	b := p(2, "Jane", "Smith")
	b.Archived = true
	assert.Empty(t, find(dedupe.Fuzzy, a, b))
	c := p(3, "Jane", "Smith")
	groups := find(dedupe.Medium, a, b, c)
	require.Len(t, groups, 1)
	assert.Equal(t, []int64{1, 3}, groups[0].IDs())
}

func TestFind_UnrelatedPeopleNotGrouped(t *testing.T) {
	// dedupe.Person has no company field, so company can never cause a match.
	assert.Empty(t, find(dedupe.Fuzzy, p(1, "Jane", "Smith"), p(2, "Mark", "Taylor")))
}

func TestFind_GroupLevelIsWeakestNeededEdge(t *testing.T) {
	people := []dedupe.Person{
		{ID: 1, FirstName: "Al", LastName: "One", Emails: []string{"s@x.com"}},
		{ID: 2, FirstName: "Bo", LastName: "Two", Emails: []string{"s@x.com"}},
		{ID: 3, FirstName: "Bo", LastName: "Two"}, // same name as 2 (medium), no tie to 1
	}
	groups := find(dedupe.Medium, people...)
	require.Len(t, groups, 1)
	assert.Equal(t, []int64{1, 2, 3}, groups[0].IDs())
	assert.Equal(t, dedupe.Medium, groups[0].Level, "joining 3 needs a medium edge")

	groups = find(dedupe.High, people...)
	require.Len(t, groups, 1)
	assert.Equal(t, []int64{1, 2}, groups[0].IDs())
	assert.Equal(t, dedupe.High, groups[0].Level)
}

func TestFind_SortedHighestFirst(t *testing.T) {
	people := []dedupe.Person{
		p(1, "Jane", "Smith"), p(2, "Jane", "Smith"),
		{ID: 3, FirstName: "Al", Emails: []string{"a@b.com"}},
		{ID: 4, FirstName: "Bo", Emails: []string{"a@b.com"}},
	}
	groups := find(dedupe.Medium, people...)
	require.Len(t, groups, 2)
	assert.Equal(t, dedupe.High, groups[0].Level)
	assert.Equal(t, dedupe.Medium, groups[1].Level)
}

func TestFind_Dismissed(t *testing.T) {
	people := []dedupe.Person{p(1, "Jane", "Smith"), p(2, "Jane", "Smith")}
	opts := dedupe.Options{Min: dedupe.Medium, Dismissed: map[[2]int64]bool{{1, 2}: true}}
	assert.Empty(t, dedupe.Find(people, opts))

	// A dismissed pair cannot be re-joined through a third person.
	people = append(people, p(3, "Jane", "Smith"))
	groups := dedupe.Find(people, opts)
	require.Len(t, groups, 1)
	assert.Len(t, groups[0].Members, 2)
}

func TestFind_SkipsGenericBuckets(t *testing.T) {
	var people []dedupe.Person
	for i := 1; i <= 60; i++ {
		people = append(people, dedupe.Person{ID: int64(i), FirstName: fmt.Sprintf("P%d", i), Phones: []string{"555-000-1111"}})
	}
	assert.Empty(t, find(dedupe.High, people...), "a phone shared by 60 people is a switchboard, not a duplicate")
}

// pseudoSurname returns a deterministic 8 letter surname that differs a lot between i values.
func pseudoSurname(i int) string {
	b := make([]byte, 8)
	x := uint32(i)*2654435761 + 12345
	for n := range b {
		x = x*1664525 + 1013904223
		b[n] = 'a' + byte((x>>16)%26)
	}
	return string(b)
}

func TestFind_Performance(t *testing.T) {
	firsts := []string{"Alice", "Bob", "Carol", "David", "Emma", "Frank", "Grace", "Henry", "Irene", "Jack", "Karen", "Liam", "Maria", "Noah", "Olivia", "Peter", "Quinn", "Rosa", "Sam", "Tina"}
	var people []dedupe.Person
	for i := 0; i < 4500; i++ {
		people = append(people, dedupe.Person{
			ID: int64(i + 1), FirstName: firsts[i%len(firsts)], LastName: pseudoSurname(i),
			Emails: []string{fmt.Sprintf("u%d@example.com", i)},
		})
	}
	start := time.Now()
	dedupe.Find(people, dedupe.Options{Min: dedupe.Fuzzy})
	assert.Less(t, time.Since(start), time.Second)
}
