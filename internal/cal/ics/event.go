package ics

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
)

// Calendar is the decoded content of an iCalendar file.
type Calendar struct {
	Name     string
	Events   []*Event
	Todos    []*Todo
	Skipped  map[string]int // components ignored, by name (VJOURNAL, VFREEBUSY, ...)
	Warnings []string
}

// Person is an organizer or attendee.
type Person struct {
	Email    string
	Name     string
	Role     string // organizer or attendee
	PartStat string // NEEDS-ACTION, ACCEPTED, DECLINED, TENTATIVE
}

// Alarm is a reminder before the start.
type Alarm struct {
	MinutesBefore int
	Action        string // DISPLAY or EMAIL
}

// Event is a VEVENT.
type Event struct {
	UID         string
	Summary     string
	Description string
	Location    string
	Status      string // CONFIRMED, TENTATIVE, CANCELLED
	Transparent bool

	AllDay bool
	// Start and End are UTC instants. For an all-day event they are midnight UTC of the dates and
	// End is exclusive.
	Start, End time.Time
	// TZ is the IANA zone of the wall clock: "UTC" for a time ending in Z, "" for all-day.
	TZ       string
	Floating bool // no zone in the file: the wall clock of whoever reads it

	RRule   string // rule body
	Exdates []time.Time
	// RecurrenceID marks an override of one instance of the series with the same UID.
	RecurrenceID *time.Time

	Organizer  *Person
	Attendees  []Person
	Alarms     []Alarm
	Categories []string

	Created, Modified time.Time
}

// Todo is a VTODO.
type Todo struct {
	UID         string
	Summary     string
	Description string
	Status      string // NEEDS-ACTION, COMPLETED, IN-PROCESS, CANCELLED
	Priority    int    // 1-9, 0 unset
	// Due is a UTC instant, or midnight UTC of the date when DueIsDate.
	Due       *time.Time
	DueIsDate bool
	Completed *time.Time
	RRule     string
	Created   time.Time
	Modified  time.Time
}

// Decode reads an iCalendar file. deviceLoc is the zone given to floating times.
func Decode(data []byte, deviceLoc *time.Location) (*Calendar, error) {
	roots, err := Parse(data)
	if err != nil {
		return nil, err
	}
	cal := &Calendar{Skipped: map[string]int{}}
	found := false
	for _, root := range roots {
		if root.Name != "VCALENDAR" {
			continue
		}
		found = true
		if n := root.Text("X-WR-CALNAME"); n != "" && cal.Name == "" {
			cal.Name = n
		}
		for _, c := range root.Children {
			switch c.Name {
			case "VEVENT":
				ev, warns := decodeEvent(c, deviceLoc)
				cal.Warnings = append(cal.Warnings, warns...)
				if ev != nil {
					cal.Events = append(cal.Events, ev)
				}
			case "VTODO":
				td, warns := decodeTodo(c)
				cal.Warnings = append(cal.Warnings, warns...)
				if td != nil {
					cal.Todos = append(cal.Todos, td)
				}
			case "VTIMEZONE":
				// Zones come from the tz database, so the file's own rules are not needed.
			default:
				cal.Skipped[c.Name]++
			}
		}
	}
	if !found {
		return nil, fmt.Errorf("no VCALENDAR found")
	}
	return cal, nil
}

// knownEvent lists the VEVENT properties that are mapped. Others are reported.
var knownEvent = map[string]bool{
	"UID": true, "SUMMARY": true, "DESCRIPTION": true, "LOCATION": true, "STATUS": true, "TRANSP": true,
	"DTSTART": true, "DTEND": true, "DURATION": true, "RRULE": true, "EXDATE": true, "RECURRENCE-ID": true,
	"ORGANIZER": true, "ATTENDEE": true, "CATEGORIES": true, "CREATED": true, "LAST-MODIFIED": true,
	"DTSTAMP": true, "SEQUENCE": true, "PRODID": true,
}

var knownTodo = map[string]bool{
	"UID": true, "SUMMARY": true, "DESCRIPTION": true, "STATUS": true, "PRIORITY": true, "DUE": true,
	"COMPLETED": true, "RRULE": true, "CREATED": true, "LAST-MODIFIED": true, "DTSTAMP": true, "SEQUENCE": true,
	"DTSTART": true, "PERCENT-COMPLETE": true, "CATEGORIES": true,
}

// timeValue is a decoded date or date-time property.
type timeValue struct {
	t        time.Time // UTC instant, or midnight UTC of the date
	date     bool
	tzName   string
	floating bool
}

// resolveZone maps a TZID parameter to a location. It accepts IANA names, Windows names and
// names with a path prefix such as /mozilla.org/20050126_1/America/Chicago.
func resolveZone(id string) (*time.Location, string, bool) {
	id = strings.Trim(strings.TrimSpace(id), `"`)
	if id == "" {
		return nil, "", false
	}
	candidates := []string{id}
	if i := strings.Index(id, "/"); i >= 0 {
		parts := strings.Split(strings.Trim(id, "/"), "/")
		if len(parts) >= 2 {
			candidates = append(candidates, strings.Join(parts[len(parts)-2:], "/"))
		}
		candidates = append(candidates, parts[len(parts)-1])
	}
	candidates = append(candidates, tz.WindowsToIANA(id))
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if loc, err := tz.Load(c); err == nil && c != "Local" {
			return loc, tz.Name(loc), true
		}
	}
	return nil, "", false
}

func parseTimeValue(p Property, value string, deviceLoc *time.Location, warn func(string)) (timeValue, error) {
	value = strings.TrimSpace(value)
	if strings.EqualFold(p.Param("VALUE"), "DATE") || len(value) == 8 {
		d, err := time.ParseInLocation("20060102", value, time.UTC)
		if err != nil {
			return timeValue{}, fmt.Errorf("bad date %q", value)
		}
		return timeValue{t: d, date: true}, nil
	}
	if strings.HasSuffix(value, "Z") {
		t, err := time.ParseInLocation("20060102T150405Z", value, time.UTC)
		if err != nil {
			return timeValue{}, fmt.Errorf("bad time %q", value)
		}
		return timeValue{t: t, tzName: "UTC"}, nil
	}
	w, err := time.ParseInLocation("20060102T150405", value, time.UTC)
	if err != nil {
		return timeValue{}, fmt.Errorf("bad time %q", value)
	}
	if id := p.Param("TZID"); id != "" {
		if loc, name, ok := resolveZone(id); ok {
			return timeValue{t: tz.Instant(w, loc), tzName: name}, nil
		}
		warn(fmt.Sprintf("unknown time zone %q, treated as floating time", id))
	}
	return timeValue{t: tz.Instant(w, deviceLoc), tzName: tz.Name(deviceLoc), floating: true}, nil
}

// timeList decodes an EXDATE-style property whose value is a comma separated list.
func timeList(p Property, deviceLoc *time.Location, warn func(string)) []time.Time {
	var out []time.Time
	for _, v := range strings.Split(p.Value, ",") {
		tv, err := parseTimeValue(p, v, deviceLoc, warn)
		if err != nil {
			warn(fmt.Sprintf("%s ignored: %v", p.Name, err))
			continue
		}
		out = append(out, tv.t)
	}
	return out
}

var durationRE = regexp.MustCompile(`^([+-]?)P(?:(\d+)W)?(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?$`)

// parseDuration reads an RFC 5545 duration such as PT1H30M, P1D or -PT15M.
func parseDuration(s string) (time.Duration, bool) {
	m := durationRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, false
	}
	num := func(i int) time.Duration {
		n, _ := strconv.Atoi(m[i])
		return time.Duration(n)
	}
	d := num(2)*7*24*time.Hour + num(3)*24*time.Hour + num(4)*time.Hour + num(5)*time.Minute + num(6)*time.Second
	if m[1] == "-" {
		d = -d
	}
	return d, true
}

func parseTime(s string) time.Time {
	if t, err := time.ParseInLocation("20060102T150405Z", strings.TrimSpace(s), time.UTC); err == nil {
		return t
	}
	return time.Time{}
}

func mailto(v string) string {
	v = strings.TrimSpace(v)
	if len(v) > 7 && strings.EqualFold(v[:7], "mailto:") {
		v = v[7:]
	}
	return strings.ToLower(v)
}

func decodePerson(p Property, role string) Person {
	per := Person{Email: mailto(p.Value), Name: p.Param("CN"), Role: role, PartStat: strings.ToUpper(p.Param("PARTSTAT"))}
	if role == "attendee" && strings.EqualFold(p.Param("ROLE"), "CHAIR") {
		per.Role = "organizer"
	}
	if per.PartStat == "" {
		per.PartStat = "NEEDS-ACTION"
	}
	return per
}

func decodeEvent(c *Component, deviceLoc *time.Location) (*Event, []string) {
	var warns []string
	uid := strings.TrimSpace(c.Text("UID"))
	label := uid
	if label == "" {
		label = c.Text("SUMMARY")
	}
	warn := func(msg string) { warns = append(warns, fmt.Sprintf("event %q: %s", label, msg)) }
	if uid == "" {
		warn("no UID, skipped")
		return nil, warns
	}
	ev := &Event{
		UID: uid, Summary: c.Text("SUMMARY"), Description: c.Text("DESCRIPTION"), Location: c.Text("LOCATION"),
		Status: strings.ToUpper(strings.TrimSpace(c.Text("STATUS"))), Transparent: strings.EqualFold(strings.TrimSpace(c.Text("TRANSP")), "TRANSPARENT"),
		Created: parseTime(c.Text("CREATED")), Modified: parseTime(c.Text("LAST-MODIFIED")),
	}
	if ev.Status == "" {
		ev.Status = "CONFIRMED"
	}
	if ev.Summary == "" {
		ev.Summary = "(no title)"
	}
	reported := map[string]bool{}
	for _, p := range c.Props {
		if !knownEvent[p.Name] && !reported[p.Name] {
			reported[p.Name] = true
			warn("property " + p.Name + " is not kept")
		}
	}

	sp, ok := c.First("DTSTART")
	if !ok {
		warn("no DTSTART, skipped")
		return nil, warns
	}
	start, err := parseTimeValue(sp, sp.Value, deviceLoc, warn)
	if err != nil {
		warn(err.Error() + ", skipped")
		return nil, warns
	}
	ev.Start, ev.AllDay, ev.TZ, ev.Floating = start.t, start.date, start.tzName, start.floating
	endProp, hasEnd := c.First("DTEND")
	durProp, hasDur := c.First("DURATION")
	switch {
	case hasEnd:
		end, err := parseTimeValue(endProp, endProp.Value, deviceLoc, warn)
		if err != nil {
			warn("bad DTEND, using the start")
			ev.End = ev.Start
		} else {
			ev.End = end.t
		}
	case hasDur:
		if d, ok := parseDuration(durProp.Value); ok && d >= 0 {
			ev.End = ev.Start.Add(d)
		} else {
			warn("bad DURATION, using the start")
			ev.End = ev.Start
		}
	case ev.AllDay:
		ev.End = ev.Start.AddDate(0, 0, 1)
	default:
		ev.End = ev.Start
	}
	if ev.End.Before(ev.Start) {
		warn("ends before it starts, using the start")
		ev.End = ev.Start
	}
	if ev.AllDay && !ev.End.After(ev.Start) {
		ev.End = ev.Start.AddDate(0, 0, 1)
	}

	if rp, ok := c.First("RRULE"); ok {
		ev.RRule = strings.TrimSpace(rp.Value)
	}
	for _, p := range c.All("EXDATE") {
		ev.Exdates = append(ev.Exdates, timeList(p, deviceLoc, warn)...)
	}
	if rp, ok := c.First("RECURRENCE-ID"); ok {
		if tv, err := parseTimeValue(rp, rp.Value, deviceLoc, warn); err == nil {
			t := tv.t
			ev.RecurrenceID = &t
		} else {
			warn("bad RECURRENCE-ID, imported as a separate event")
		}
	}
	if p, ok := c.First("ORGANIZER"); ok {
		per := decodePerson(p, "organizer")
		ev.Organizer = &per
	}
	for _, p := range c.All("ATTENDEE") {
		ev.Attendees = append(ev.Attendees, decodePerson(p, "attendee"))
	}
	for _, p := range c.All("CATEGORIES") {
		for _, cat := range SplitText(p.Value) {
			if cat = strings.TrimSpace(cat); cat != "" {
				ev.Categories = append(ev.Categories, cat)
			}
		}
	}
	for _, al := range c.Children {
		if al.Name != "VALARM" {
			continue
		}
		trig, ok := al.First("TRIGGER")
		if !ok {
			continue
		}
		d, ok := parseDuration(trig.Value)
		if !ok || strings.EqualFold(trig.Param("VALUE"), "DATE-TIME") || strings.EqualFold(trig.Param("RELATED"), "END") || d > 0 {
			warn("a reminder that is not relative to the start was skipped")
			continue
		}
		action := strings.ToUpper(strings.TrimSpace(al.Text("ACTION")))
		if action != "EMAIL" {
			action = "DISPLAY"
		}
		ev.Alarms = append(ev.Alarms, Alarm{MinutesBefore: int(-d / time.Minute), Action: action})
	}
	return ev, warns
}

func decodeTodo(c *Component) (*Todo, []string) {
	var warns []string
	uid := strings.TrimSpace(c.Text("UID"))
	warn := func(msg string) {
		warns = append(warns, fmt.Sprintf("todo %q: %s", firstNonEmpty(uid, c.Text("SUMMARY")), msg))
	}
	if uid == "" {
		warn("no UID, skipped")
		return nil, warns
	}
	reported := map[string]bool{}
	for _, p := range c.Props {
		if !knownTodo[p.Name] && !reported[p.Name] {
			reported[p.Name] = true
			warn("property " + p.Name + " is not kept")
		}
	}
	td := &Todo{
		UID: uid, Summary: c.Text("SUMMARY"), Description: c.Text("DESCRIPTION"),
		Status: strings.ToUpper(strings.TrimSpace(c.Text("STATUS"))), RRule: strings.TrimSpace(c.Text("RRULE")),
		Created: parseTime(c.Text("CREATED")), Modified: parseTime(c.Text("LAST-MODIFIED")),
	}
	if td.Summary == "" {
		td.Summary = "(no title)"
	}
	if p, ok := c.First("PRIORITY"); ok {
		td.Priority, _ = strconv.Atoi(strings.TrimSpace(p.Value))
	}
	if p, ok := c.First("DUE"); ok {
		tv, err := parseTimeValue(p, p.Value, time.UTC, warn)
		if err != nil {
			warn("bad DUE ignored")
		} else {
			t := tv.t
			td.Due, td.DueIsDate = &t, tv.date
		}
	}
	if p, ok := c.First("COMPLETED"); ok {
		if t := parseTime(p.Value); !t.IsZero() {
			td.Completed = &t
		}
	}
	return td, warns
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// ---- encoding ----

// Encode writes the calendar as iCalendar text. Each zone used gets a VTIMEZONE with explicit
// observances for the years the events cover, so consumers need no tz database.
func Encode(name string, events []*Event, todos []*Todo) []byte {
	return EncodeMethod(name, "", events, todos)
}

// EncodeMethod is Encode with an iTIP METHOD (REQUEST, CANCEL, ...) for invitations.
func EncodeMethod(name, method string, events []*Event, todos []*Todo) []byte {
	var w writer
	w.begin("VCALENDAR")
	w.line("VERSION", nil, "2.0")
	w.line("PRODID", nil, "-//crm-cli//calendar//EN")
	w.line("CALSCALE", nil, "GREGORIAN")
	if method != "" {
		w.line("METHOD", nil, method)
	}
	if name != "" {
		w.text("X-WR-CALNAME", nil, name)
	}

	zones := map[string]bool{}
	var lo, hi time.Time
	for _, ev := range events {
		if !ev.AllDay && !ev.Floating && ev.TZ != "" && ev.TZ != "UTC" {
			zones[ev.TZ] = true
		}
		for _, t := range []time.Time{ev.Start, ev.End} {
			if lo.IsZero() || t.Before(lo) {
				lo = t
			}
			if hi.IsZero() || t.After(hi) {
				hi = t
			}
		}
	}
	names := make([]string, 0, len(zones))
	for z := range zones {
		names = append(names, z)
	}
	sort.Strings(names)
	for _, z := range names {
		writeTimezone(&w, z, lo, hi)
	}

	for _, ev := range events {
		writeEvent(&w, ev)
	}
	for _, td := range todos {
		writeTodo(&w, td)
	}
	w.end("VCALENDAR")
	return []byte(w.b.String())
}

func fmtOffset(sec int) string {
	sign := "+"
	if sec < 0 {
		sign, sec = "-", -sec
	}
	return fmt.Sprintf("%s%02d%02d", sign, sec/3600, sec%3600/60)
}

func writeTimezone(w *writer, name string, lo, hi time.Time) {
	loc, err := tz.Load(name)
	if err != nil {
		return
	}
	from := lo.AddDate(-1, 0, 0)
	to := hi.AddDate(6, 0, 0)
	if to.Year() > 2100 {
		to = time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	w.begin("VTIMEZONE")
	w.text("TZID", nil, name)
	init := tz.StateAt(loc, from)
	kind := "STANDARD"
	if init.Daylight {
		kind = "DAYLIGHT"
	}
	w.begin(kind)
	w.line("DTSTART", nil, "19700101T000000")
	w.line("TZOFFSETFROM", nil, fmtOffset(init.Offset))
	w.line("TZOFFSETTO", nil, fmtOffset(init.Offset))
	w.text("TZNAME", nil, init.Name)
	w.end(kind)
	for _, tr := range tz.Transitions(loc, from, to) {
		kind := "STANDARD"
		if tr.Daylight {
			kind = "DAYLIGHT"
		}
		w.begin(kind)
		w.line("DTSTART", nil, tr.WallBefore.Format("20060102T150405"))
		w.line("TZOFFSETFROM", nil, fmtOffset(tr.From))
		w.line("TZOFFSETTO", nil, fmtOffset(tr.To))
		w.text("TZNAME", nil, tr.Name)
		w.end(kind)
	}
	w.end("VTIMEZONE")
}

// timeLine writes a DTSTART-style property for an instant in the event's zone.
func timeLine(w *writer, prop string, t time.Time, allDay, floating bool, zone string) {
	switch {
	case allDay:
		w.line(prop, map[string]string{"VALUE": "DATE"}, t.Format("20060102"))
	case zone == "UTC" || zone == "":
		w.line(prop, nil, t.UTC().Format("20060102T150405Z"))
	default:
		loc, err := tz.Load(zone)
		if err != nil {
			w.line(prop, nil, t.UTC().Format("20060102T150405Z"))
			return
		}
		wall := tz.Wall(t, loc).Format("20060102T150405")
		if floating {
			w.line(prop, nil, wall)
			return
		}
		w.line(prop, map[string]string{"TZID": zone}, wall)
	}
}

func stamp(t time.Time) string {
	if t.IsZero() {
		t = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return t.UTC().Format("20060102T150405Z")
}

func writePerson(w *writer, prop string, p Person) {
	params := map[string]string{}
	if p.Name != "" {
		params["CN"] = p.Name
	}
	if prop == "ATTENDEE" {
		if p.Role == "organizer" {
			params["ROLE"] = "CHAIR"
		} else {
			params["ROLE"] = "REQ-PARTICIPANT"
		}
		ps := p.PartStat
		if ps == "" {
			ps = "NEEDS-ACTION"
		}
		params["PARTSTAT"] = ps
	}
	w.line(prop, params, "mailto:"+p.Email)
}

func writeEvent(w *writer, ev *Event) {
	w.begin("VEVENT")
	w.text("UID", nil, ev.UID)
	w.line("DTSTAMP", nil, stamp(ev.Modified))
	if !ev.Created.IsZero() {
		w.line("CREATED", nil, stamp(ev.Created))
	}
	if !ev.Modified.IsZero() {
		w.line("LAST-MODIFIED", nil, stamp(ev.Modified))
	}
	if ev.RecurrenceID != nil {
		timeLine(w, "RECURRENCE-ID", *ev.RecurrenceID, ev.AllDay, ev.Floating, ev.TZ)
	}
	w.text("SUMMARY", nil, ev.Summary)
	timeLine(w, "DTSTART", ev.Start, ev.AllDay, ev.Floating, ev.TZ)
	timeLine(w, "DTEND", ev.End, ev.AllDay, ev.Floating, ev.TZ)
	if ev.RRule != "" {
		w.line("RRULE", nil, ev.RRule)
	}
	for _, x := range ev.Exdates {
		timeLine(w, "EXDATE", x, ev.AllDay, ev.Floating, ev.TZ)
	}
	status := ev.Status
	if status == "" {
		status = "CONFIRMED"
	}
	w.line("STATUS", nil, status)
	if ev.Transparent {
		w.line("TRANSP", nil, "TRANSPARENT")
	} else {
		w.line("TRANSP", nil, "OPAQUE")
	}
	if ev.Location != "" {
		w.text("LOCATION", nil, ev.Location)
	}
	if ev.Description != "" {
		w.text("DESCRIPTION", nil, ev.Description)
	}
	if len(ev.Categories) > 0 {
		cats := make([]string, len(ev.Categories))
		for i, c := range ev.Categories {
			cats[i] = Escape(c)
		}
		w.line("CATEGORIES", nil, strings.Join(cats, ","))
	}
	if ev.Organizer != nil {
		writePerson(w, "ORGANIZER", *ev.Organizer)
	}
	for _, a := range ev.Attendees {
		writePerson(w, "ATTENDEE", a)
	}
	for _, al := range ev.Alarms {
		w.begin("VALARM")
		action := al.Action
		if action == "" {
			action = "DISPLAY"
		}
		w.line("ACTION", nil, action)
		w.text("DESCRIPTION", nil, "Reminder")
		w.line("TRIGGER", nil, triggerText(al.MinutesBefore))
		w.end("VALARM")
	}
	w.end("VEVENT")
}

func triggerText(minutes int) string {
	switch {
	case minutes == 0:
		return "PT0S"
	case minutes%1440 == 0:
		return fmt.Sprintf("-P%dD", minutes/1440)
	case minutes%60 == 0:
		return fmt.Sprintf("-PT%dH", minutes/60)
	}
	return fmt.Sprintf("-PT%dM", minutes)
}

func writeTodo(w *writer, td *Todo) {
	w.begin("VTODO")
	w.text("UID", nil, td.UID)
	w.line("DTSTAMP", nil, stamp(td.Modified))
	if !td.Created.IsZero() {
		w.line("CREATED", nil, stamp(td.Created))
	}
	if !td.Modified.IsZero() {
		w.line("LAST-MODIFIED", nil, stamp(td.Modified))
	}
	w.text("SUMMARY", nil, td.Summary)
	if td.Description != "" {
		w.text("DESCRIPTION", nil, td.Description)
	}
	if td.Due != nil {
		if td.DueIsDate {
			w.line("DUE", map[string]string{"VALUE": "DATE"}, td.Due.Format("20060102"))
		} else {
			w.line("DUE", nil, td.Due.UTC().Format("20060102T150405Z"))
		}
	}
	if td.RRule != "" {
		w.line("RRULE", nil, td.RRule)
	}
	if td.Priority > 0 {
		w.line("PRIORITY", nil, strconv.Itoa(td.Priority))
	}
	status := td.Status
	if status == "" {
		status = "NEEDS-ACTION"
	}
	w.line("STATUS", nil, status)
	if td.Completed != nil {
		w.line("COMPLETED", nil, td.Completed.UTC().Format("20060102T150405Z"))
	}
	w.end("VTODO")
}
