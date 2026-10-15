package shortsyntax

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseCadence turns a typed cadence into a keep-in-touch value. It accepts the stored values
// ("3 mons", "1 year", "never") and the short forms used after * ("3m", "6w", "monthly").
func ParseCadence(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "7 days", "14 days", "1 mon", "42 days", "3 mons", "6 mons", "1 year", "never":
		return s, true
	}
	return parseCadence(strings.ReplaceAll(s, " ", ""))
}

// ParsePriority turns top, high, medium, low, none or 1 to 5 into a canonical priority name.
func ParsePriority(s string) (string, bool) {
	return parsePriority(strings.ToLower(strings.TrimSpace(s)))
}

// ParseDuration turns 30m, 1h or 1h30m into minutes.
func ParseDuration(s string) (int, bool) {
	return parseDuration(strings.ToLower(strings.TrimSpace(s)))
}

// ParseBirthday reads a birthday as typed by a person: 1985-03-04, --03-04, 03-04, 3/4, 3/4/1985,
// "Mar 4", "March 4 1985", "4 Mar 1985". It returns the stored form: YYYY-MM-DD or --MM-DD.
func ParseBirthday(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer(",", " ").Replace(s)
	if s == "" {
		return "", false
	}
	if m := isoRe.FindStringSubmatch(s); m != nil {
		return birthdayOut(atoi(m[2]), atoi(m[3]), atoi(m[1]))
	}
	if strings.HasPrefix(s, "--") || (len(s) == 5 && s[2] == '-') {
		parts := strings.Split(strings.TrimPrefix(s, "--"), "-")
		if len(parts) == 2 && allDigits(parts[0]) && allDigits(parts[1]) {
			return birthdayOut(atoi(parts[0]), atoi(parts[1]), 0)
		}
		return "", false
	}
	if m := mdyRe.FindStringSubmatch(s); m != nil {
		return birthdayOut(atoi(m[1]), atoi(m[2]), atoi(m[3]))
	}
	if m := mdRe.FindStringSubmatch(s); m != nil {
		return birthdayOut(atoi(m[1]), atoi(m[2]), 0)
	}
	w := strings.Fields(s)
	if len(w) < 2 || len(w) > 3 {
		return "", false
	}
	year := 0
	if len(w) == 3 {
		if !yearRe.MatchString(w[2]) {
			return "", false
		}
		year = atoi(w[2])
	}
	if mo, ok := months[w[0]]; ok {
		if d, ok := dayOf(w[1]); ok {
			return birthdayOut(mo, d, year)
		}
	}
	if d, ok := dayOf(w[0]); ok {
		if mo, ok := months[w[1]]; ok {
			return birthdayOut(mo, d, year)
		}
	}
	return "", false
}

// FormatMinutes writes minutes as 45m, 2h or 1h30m.
func FormatMinutes(n int) string { return formatMinutes(n) }

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// birthdayOut validates month and day (29 Feb is allowed without a year) and formats the result.
func birthdayOut(month, day, year int) (string, bool) {
	if month < 1 || month > 12 || day < 1 {
		return "", false
	}
	if day > time.Date(2000, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day() {
		return "", false
	}
	if year == 0 {
		return fmt.Sprintf("--%02d-%02d", month, day), true
	}
	if year < 1 || year > 9999 {
		return "", false
	}
	if !(dateVal{year, month, day}).valid() {
		return "", false
	}
	return fmt.Sprintf("%04d-%02d-%02d", year, month, day), true
}
