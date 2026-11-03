package shortsyntax

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestParseDateForms runs the typed date forms against the reference time 2026-10-15.
func TestParseDateForms(t *testing.T) {
	tests := []struct {
		in   string
		want string // "" means not a date
	}{
		// month/day/year and month-day-year
		{"12/25/2026", "2026-12-25"},
		{"12-25-2026", "2026-12-25"},
		{"1/5/2027", "2027-01-05"},
		{"01-05-2027", "2027-01-05"},
		{"12/25/26", "2026-12-25"},
		{"12-25-26", "2026-12-25"},
		{"2/29/28", "2028-02-29"},
		// first number above 12 with a second up to 12 is day-first
		{"25/12/2026", "2026-12-25"},
		{"25-12-2026", "2026-12-25"},
		{"25/12/26", "2026-12-25"},
		{"25/12", "2026-12-25"},
		// month/day takes the next occurrence on or after today
		{"12/25", "2026-12-25"},
		{"10/15", "2026-10-15"},
		{"10/14", "2027-10-14"},
		{"1/5", "2027-01-05"},
		{"2/29", "2028-02-29"},
		// month names
		{"Dec 25", "2026-12-25"},
		{"Dec 25 2026", "2026-12-25"},
		{"December 25 2026", "2026-12-25"},
		{"December 25, 2026", "2026-12-25"},
		{"25 Dec 2026", "2026-12-25"},
		{"25 Dec", "2026-12-25"},
		{"25 December", "2026-12-25"},
		{"Oct 14", "2027-10-14"},
		// the forms that already worked
		{"2026-12-25", "2026-12-25"},
		{"next friday", "2026-10-23"},
		// everything else stays an error
		{"13/13", ""},
		{"13/13/2026", ""},
		{"0/5/2026", ""},
		{"12/32/2026", ""},
		{"2/30/2026", ""},
		{"2/29/2027", ""},
		{"12/25/2026/1", ""},
		{"12/25-2026", ""},
		{"12-25", ""},
		{"12/25/202", ""},
		{"12/25/20266", ""},
		{"25/25/2026", ""},
		{"Dec 32", ""},
		{"Foo 25 2026", ""},
		{"someday", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := ParseDate(tt.in, testNow)
			assert.Equal(t, tt.want != "", ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
