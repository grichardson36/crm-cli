package recur

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/model"
)

var dayCodes = map[string]string{
	"mon": "MO", "monday": "MO", "tue": "TU", "tues": "TU", "tuesday": "TU",
	"wed": "WE", "weds": "WE", "wednesday": "WE", "thu": "TH", "thur": "TH", "thurs": "TH", "thursday": "TH",
	"fri": "FR", "friday": "FR", "sat": "SA", "saturday": "SA", "sun": "SU", "sunday": "SU",
}

var dayNames = map[string]string{"MO": "Mon", "TU": "Tue", "WE": "Wed", "TH": "Thu", "FR": "Fri", "SA": "Sat", "SU": "Sun"}

var ordinals = map[string]string{
	"1st": "1", "first": "1", "2nd": "2", "second": "2", "3rd": "3", "third": "3",
	"4th": "4", "fourth": "4", "last": "-1",
}

var units = map[string]string{
	"d": "DAILY", "day": "DAILY", "days": "DAILY",
	"w": "WEEKLY", "wk": "WEEKLY", "week": "WEEKLY", "weeks": "WEEKLY",
	"mo": "MONTHLY", "month": "MONTHLY", "months": "MONTHLY", "mon": "MONTHLY", "mons": "MONTHLY",
	"y": "YEARLY", "yr": "YEARLY", "year": "YEARLY", "years": "YEARLY",
}

func rule(freq string, interval int, extra ...string) string {
	parts := []string{"FREQ=" + freq}
	if interval > 1 {
		parts = append(parts, "INTERVAL="+strconv.Itoa(interval))
	}
	return strings.Join(append(parts, extra...), ";")
}

// Natural compiles a short recurrence phrase to an RRULE body. It also accepts an RRULE body,
// which is validated and returned in canonical form. Phrases (a leading "every" is optional):
//
//	day, daily, weekday, weekdays, week, weekly, biweekly, month, monthly, year, yearly
//	2w, 3d, 2mo, 1y, 2 weeks, 3 days
//	fri, mon,wed,fri, mon and wed
//	1st mon, last fri, 2nd tue
func Natural(text string) (string, error) {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return "", fmt.Errorf("empty recurrence: %w", model.ErrValidation)
	}
	if strings.HasPrefix(t, "freq=") || strings.HasPrefix(t, "rrule:") {
		return Normalize(strings.ToUpper(t))
	}
	t = strings.TrimSpace(strings.TrimPrefix(t, "every"))
	t = strings.TrimSpace(strings.TrimPrefix(t, "other"))
	bad := func() (string, error) {
		return "", fmt.Errorf("cannot read recurrence %q (try: daily, weekdays, weekly, every 2w, mon,wed, last fri, or an RRULE such as FREQ=WEEKLY;BYDAY=MO): %w", text, model.ErrValidation)
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(text)), "every other") {
		t = "2 " + t
	}

	switch t {
	case "day", "daily":
		return rule("DAILY", 1), nil
	case "weekday", "weekdays":
		return rule("WEEKLY", 1, "BYDAY=MO,TU,WE,TH,FR"), nil
	case "week", "weekly":
		return rule("WEEKLY", 1), nil
	case "biweekly", "fortnight", "fortnightly":
		return rule("WEEKLY", 2), nil
	case "month", "monthly":
		return rule("MONTHLY", 1), nil
	case "year", "yearly", "annually":
		return rule("YEARLY", 1), nil
	}

	fields := strings.Fields(strings.ReplaceAll(t, ",", " "))
	// "2w", "3d", "2mo", "1y"
	if len(fields) == 1 {
		if n, unit, ok := splitNumUnit(fields[0]); ok {
			if f, ok := units[unit]; ok {
				return rule(f, n), nil
			}
		}
	}
	// "2 weeks"
	if len(fields) == 2 {
		if n, err := strconv.Atoi(fields[0]); err == nil && n > 0 {
			if f, ok := units[fields[1]]; ok {
				return rule(f, n), nil
			}
		}
		// "1st mon", "last fri"
		if ord, ok := ordinals[fields[0]]; ok {
			if code, ok := dayCodes[fields[1]]; ok {
				return rule("MONTHLY", 1, "BYDAY="+ord+code), nil
			}
		}
	}
	// "mon wed fri", "mon and wed"
	var days []string
	for _, f := range fields {
		if f == "and" {
			continue
		}
		code, ok := dayCodes[f]
		if !ok {
			return bad()
		}
		days = append(days, code)
	}
	if len(days) == 0 {
		return bad()
	}
	return rule("WEEKLY", 1, "BYDAY="+strings.Join(days, ",")), nil
}

func splitNumUnit(s string) (int, string, bool) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 || i == len(s) {
		return 0, "", false
	}
	n, err := strconv.Atoi(s[:i])
	if err != nil || n < 1 {
		return 0, "", false
	}
	return n, s[i:], true
}

// FromTaskRecurrence maps a task recurrence such as "7 days", "2 weeks", "1 mon" or "weekly" to
// an RRULE body, so repeating tasks can show on the calendar. A task repeats after completion,
// the rule describes the nominal cadence.
func FromTaskRecurrence(s string) (string, error) {
	y, m, d, err := model.ParseRecurrence(s)
	if err != nil {
		return "", err
	}
	switch {
	case y > 0:
		return rule("YEARLY", y), nil
	case m > 0:
		return rule("MONTHLY", m), nil
	case d%7 == 0:
		return rule("WEEKLY", d/7), nil
	default:
		return rule("DAILY", d), nil
	}
}

// Describe renders a rule in words, such as "every 2 weeks on Mon, Wed until 2026-12-31". A rule
// it cannot phrase is returned unchanged.
func Describe(body string) string {
	parts, err := parseKV(body)
	if err != nil {
		return body
	}
	m := map[string]string{}
	for _, p := range parts {
		m[p.k] = p.v
	}
	unit := map[string]string{"DAILY": "day", "WEEKLY": "week", "MONTHLY": "month", "YEARLY": "year"}[m["FREQ"]]
	n := 1
	if v, ok := m["INTERVAL"]; ok {
		n, _ = strconv.Atoi(v)
	}
	var b strings.Builder
	switch {
	case n > 1:
		fmt.Fprintf(&b, "every %d %ss", n, unit)
	case m["FREQ"] == "DAILY":
		b.WriteString("every day")
	default:
		b.WriteString("every " + unit)
	}
	if by := m["BYDAY"]; by != "" {
		var names []string
		for _, d := range strings.Split(by, ",") {
			ord := strings.TrimRight(d, "MOTUWEHFRSA")
			code := d[len(ord):]
			name := dayNames[code]
			if name == "" {
				return body
			}
			switch ord {
			case "":
			case "-1":
				name = "last " + name
			default:
				name = ord + " " + name
			}
			names = append(names, name)
		}
		if by == "MO,TU,WE,TH,FR" {
			b.Reset()
			b.WriteString("every weekday")
		} else {
			b.WriteString(" on " + strings.Join(names, ", "))
		}
	}
	if c := m["COUNT"]; c != "" {
		b.WriteString(", " + c + " times")
	}
	if u := m["UNTIL"]; len(u) >= 8 {
		b.WriteString(" until " + u[0:4] + "-" + u[4:6] + "-" + u[6:8])
	}
	return b.String()
}
