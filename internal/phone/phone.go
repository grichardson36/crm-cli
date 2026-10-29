// Package phone normalizes phone numbers to the E.164 standard (patterns/phone-numbers-are-e164).
// It wraps github.com/nyaruka/phonenumbers, a pure Go port of Google's libphonenumber, so the rest of
// the code never imports the library. A number is parsed with a region (the default is US, set once
// from config.toml at startup) when it has no country code of its own.
//
// The typed text is never changed here. Callers store it as typed and keep the E.164 form next to it.
package phone

import (
	"strings"
	"sync/atomic"

	"github.com/nyaruka/phonenumbers"
)

// FallbackRegion is the default region when config.toml names none.
const FallbackRegion = "US"

// defaultRegion holds the process-wide default region. It is the one deliberate piece of shared
// state in this package: it is written once at startup from config.toml (a per-device setting)
// and read by every door, so no call site has to thread a region through.
var defaultRegion atomic.Value

// DefaultRegion is the region used to parse numbers that have no country code.
func DefaultRegion() string {
	if r, ok := defaultRegion.Load().(string); ok && r != "" {
		return r
	}
	return FallbackRegion
}

// SetDefaultRegion sets the default region from a two-letter ISO code (case-insensitive). An empty
// value restores the fallback. It returns false and changes nothing for an unknown code.
func SetDefaultRegion(code string) bool {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		defaultRegion.Store("")
		return true
	}
	if !ValidRegion(code) {
		return false
	}
	defaultRegion.Store(code)
	return true
}

// ValidRegion reports whether code is a region the library knows (US, GB, DE ...).
func ValidRegion(code string) bool {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) != 2 {
		return false
	}
	return phonenumbers.GetCountryCodeForRegion(code) != 0
}

// Number is the result of parsing one phone number.
type Number struct {
	// E164 is the number in E.164 form (+14155550100), without an extension. Empty when the text could
	// not be parsed into a number of a possible length.
	E164 string
	// Ext is the extension, digits only.
	Ext string
	// Region is the ISO region the number belongs to, "" when unknown.
	Region string
	// Valid is true when the number matches a real numbering plan for its region.
	Valid bool
	// Parsed is true when the library could read a number at all, even one that is not possible.
	Parsed bool
	// Possible is true when the length fits the region. E164 is only set for possible numbers.
	Possible bool

	parsed *phonenumbers.PhoneNumber
}

// Parse reads raw with the default region.
func Parse(raw string) Number { return ParseRegion(raw, DefaultRegion()) }

// ParseRegion reads raw, using region for numbers without a country code.
func ParseRegion(raw, region string) Number {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Number{}
	}
	if region == "" {
		region = DefaultRegion()
	}
	n, err := phonenumbers.Parse(raw, strings.ToUpper(region))
	if err != nil || n == nil {
		return Number{}
	}
	out := Number{Parsed: true, parsed: n, Ext: n.GetExtension()}
	out.Valid = phonenumbers.IsValidNumber(n)
	out.Possible = phonenumbers.IsPossibleNumberWithReason(n) == phonenumbers.IS_POSSIBLE
	out.Region = phonenumbers.GetRegionCodeForNumber(n)
	if out.Possible {
		out.E164 = phonenumbers.Format(n, phonenumbers.E164)
	}
	return out
}

// Key is the matching key of a number: its E.164 form plus the extension, so two people at one
// switchboard with different extensions do not match. Empty when there is no E.164 form.
func (n Number) Key() string {
	if n.E164 == "" {
		return ""
	}
	if n.Ext != "" {
		return n.E164 + "x" + n.Ext
	}
	return n.E164
}

// National formats the number the way people dial it inside its own country: (415) 555-0100.
func (n Number) National() string {
	if n.parsed == nil {
		return ""
	}
	return phonenumbers.Format(n.parsed, phonenumbers.NATIONAL)
}

// International formats the number with its country code: +1 415-555-0100.
func (n Number) International() string {
	if n.parsed == nil {
		return ""
	}
	return phonenumbers.Format(n.parsed, phonenumbers.INTERNATIONAL)
}

// E164 returns the E.164 form of raw with the default region, "" when it cannot be normalized.
func E164(raw string) string { return Parse(raw).E164 }

// Key returns the matching key of raw with the default region.
func Key(raw string) string { return Parse(raw).Key() }

// Display formats a stored number for people. A number from the default region shows in national
// format, any other in international format. stored is the E.164 form when known, else empty and
// raw is parsed. A number that cannot be parsed shows as typed.
func Display(raw, stored string) string {
	return DisplayRegion(raw, stored, DefaultRegion())
}

// DisplayRegion is Display with an explicit region.
func DisplayRegion(raw, stored, region string) string {
	src := raw
	if stored != "" {
		src = stored
		// Keep an extension that only the typed text carries.
		if ext := ParseRegion(raw, region).Ext; ext != "" {
			src = stored + ";ext=" + ext
		}
	}
	n := ParseRegion(src, region)
	if !n.Possible {
		return strings.TrimSpace(raw)
	}
	if n.Region != "" && strings.EqualFold(n.Region, region) {
		return n.National()
	}
	return n.International()
}
