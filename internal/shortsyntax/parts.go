package shortsyntax

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

var (
	durSimple   = regexp.MustCompile(`^(\d+(?:\.\d+)?)(m|min|mins|minute|minutes|h|hr|hrs|hour|hours|d|day|days)$`)
	durCompound = regexp.MustCompile(`^(\d+)(?:h|hr|hrs|hour|hours)(\d+)(?:m|min|mins|minutes)?$`)
	cadenceRe   = regexp.MustCompile(`^(\d+)(d|day|days|w|wk|wks|week|weeks|m|mo|mos|mon|mons|month|months|y|yr|yrs|year|years)$`)
	compactRe   = regexp.MustCompile(`^(\d+)(d|w|m|y)$`)
)

// maxDurationMin caps a duration at seven days.
const maxDurationMin = 7 * 24 * 60

// parseDuration reads 30m, 90min, 1h, 1.5h, 1h30m, 1h30 and 2d. It returns minutes.
func parseDuration(s string) (int, bool) {
	if m := durCompound.FindStringSubmatch(s); m != nil {
		h, _ := strconv.Atoi(m[1])
		mi, _ := strconv.Atoi(m[2])
		if mi >= 60 {
			return 0, false
		}
		return checkMinutes(h*60 + mi)
	}
	m := durSimple.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	mult := 1.0
	switch m[2][0] {
	case 'h':
		mult = 60
	case 'd':
		mult = 24 * 60
	}
	return checkMinutes(int(math.Round(n * mult)))
}

func checkMinutes(n int) (int, bool) {
	if n < 1 || n > maxDurationMin {
		return 0, false
	}
	return n, true
}

// formatMinutes writes minutes as 45m, 2h or 1h30m.
func formatMinutes(n int) string {
	h, m := n/60, n%60
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%dm", h, m)
}

// parseCadence maps *3m, *6w, *monthly and the like to a keep-in-touch cadence value. Anything
// that is not one of the allowed cadences is not a cadence.
func parseCadence(s string) (string, bool) {
	switch s {
	case "weekly":
		return "7 days", true
	case "biweekly", "fortnightly":
		return "14 days", true
	case "monthly":
		return "1 mon", true
	case "quarterly":
		return "3 mons", true
	case "biannual", "semiannual", "semiannually":
		return "6 mons", true
	case "yearly", "annual", "annually":
		return "1 year", true
	case "never", "off", "none":
		return "never", true
	}
	m := cadenceRe.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	n, _ := strconv.Atoi(m[1])
	switch m[2][0] {
	case 'd':
		if n == 7 || n == 14 || n == 42 {
			return fmt.Sprintf("%d days", n), true
		}
	case 'w':
		switch n {
		case 1:
			return "7 days", true
		case 2:
			return "14 days", true
		case 6:
			return "42 days", true
		}
	case 'm':
		switch n {
		case 1:
			return "1 mon", true
		case 3:
			return "3 mons", true
		case 6:
			return "6 mons", true
		case 12:
			return "1 year", true
		}
	case 'y':
		if n == 1 {
			return "1 year", true
		}
	}
	return "", false
}

// parsePriority reads ^high and friends.
func parsePriority(s string) (string, bool) {
	switch s {
	case "top", "asap", "urgent", "1":
		return "top", true
	case "high", "hi", "2":
		return "high", true
	case "medium", "med", "mid", "normal", "3":
		return "medium", true
	case "low", "lo", "4":
		return "low", true
	case "none", "5":
		return "none", true
	}
	return "", false
}

// parseCompactUnit reads 2w, 3d, 1m, 5y.
func parseCompactUnit(s string) (compact, bool) {
	m := compactRe.FindStringSubmatch(s)
	if m == nil {
		return compact{}, false
	}
	n, _ := strconv.Atoi(m[1])
	return compact{n: n, unit: m[2][0]}, true
}

type compact struct {
	n    int
	unit byte
}

// foldString lowercases s and strips accents.
func foldString(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
