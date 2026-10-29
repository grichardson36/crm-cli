package phone

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRegion(t *testing.T) {
	tests := []struct {
		name, raw, region string
		e164, key         string
		valid, possible   bool
	}{
		{"us national", "(415) 555-2671", "US", "+14155552671", "+14155552671", true, true},
		{"us dashed", "415-555-2671", "US", "+14155552671", "+14155552671", true, true},
		{"us with country code", "+1 415 555 2671", "US", "+14155552671", "+14155552671", true, true},
		{"us 1 prefix", "1-415-555-2671", "US", "+14155552671", "+14155552671", true, true},
		{"gb national under GB", "020 7946 0958", "GB", "+442079460958", "+442079460958", true, true},
		{"gb national under US fails", "020 7946 0958", "US", "", "", false, false},
		{"gb international under US", "+44 20 7946 0958", "US", "+442079460958", "+442079460958", true, true},
		{"de mobile", "0151 23456789", "DE", "+4915123456789", "+4915123456789", true, true},
		{"extension kept in key", "415-555-2671 x23", "US", "+14155552671", "+14155552671x23", true, true},
		{"area 555 parses but invalid", "555-123-4567", "US", "+15551234567", "+15551234567", false, true},
		{"seven digits not possible", "555-0100", "US", "", "", false, false},
		{"empty", "  ", "US", "", "", false, false},
		{"letters", "call me", "US", "", "", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := ParseRegion(tt.raw, tt.region)
			assert.Equal(t, tt.e164, n.E164)
			assert.Equal(t, tt.key, n.Key())
			assert.Equal(t, tt.valid, n.Valid)
			assert.Equal(t, tt.possible, n.Possible)
		})
	}
}

func TestDefaultRegion(t *testing.T) {
	t.Cleanup(func() { SetDefaultRegion("") })
	assert.Equal(t, "US", DefaultRegion())
	assert.Equal(t, "", E164("020 7946 0958"))
	assert.True(t, SetDefaultRegion("gb"))
	assert.Equal(t, "GB", DefaultRegion())
	assert.Equal(t, "+442079460958", E164("020 7946 0958"))
	assert.False(t, SetDefaultRegion("ZZ"))
	assert.False(t, SetDefaultRegion("USA"))
	assert.Equal(t, "GB", DefaultRegion(), "an unknown code changes nothing")
	assert.True(t, ValidRegion("de"))
}

func TestDisplay(t *testing.T) {
	tests := []struct {
		name, raw, stored, region, want string
	}{
		{"home region is national", "+14155552671", "+14155552671", "US", "(415) 555-2671"},
		{"other region is international", "+442079460958", "+442079460958", "US", "+44 20 7946 0958"},
		{"from typed text", "415.555.2671", "", "US", "(415) 555-2671"},
		{"gb home", "+442079460958", "+442079460958", "GB", "020 7946 0958"},
		{"unparseable shows as typed", " 555-0100 ", "", "US", "555-0100"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DisplayRegion(tt.raw, tt.stored, tt.region))
		})
	}
}
