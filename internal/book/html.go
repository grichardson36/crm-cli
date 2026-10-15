package book

import (
	"bytes"
	"fmt"
	"html/template"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/model"
)

type pageDay struct {
	Label string
	Slots []pageSlot
}

type pageSlot struct {
	Time string
	Link template.URL
}

var page = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
:root { --bg: #ffffff; --fg: #1d2330; --muted: #5c6577; --line: #d9dde6; --accent: #1f5fd6; }
@media (prefers-color-scheme: dark) { :root { --bg: #14171f; --fg: #e8ebf2; --muted: #9aa3b5; --line: #2a3040; --accent: #6b9bff; } }
body { margin: 0; background: var(--bg); color: var(--fg); font: 16px/1.5 system-ui, sans-serif; }
main { max-width: 40rem; margin: 0 auto; padding: 2rem 1rem 4rem; }
h1 { font-size: 1.5rem; margin: 0 0 .25rem; }
p, small { color: var(--muted); }
section { border-top: 1px solid var(--line); padding: 1rem 0; }
h2 { font-size: 1rem; margin: 0 0 .5rem; }
a.slot { display: inline-block; margin: 0 .5rem .5rem 0; padding: .4rem .8rem; border: 1px solid var(--accent); border-radius: .4rem; color: var(--accent); text-decoration: none; }
a.slot:hover, a.slot:focus-visible { background: var(--accent); color: var(--bg); }
</style>
</head>
<body>
<main>
<h1>{{.Title}}</h1>
<p>{{.Length}} minutes{{if .Where}}. {{.Where}}{{end}}</p>
{{if .Description}}<p>{{.Description}}</p>{{end}}
{{if .Days}}<p>Times are in {{.TZ}}. Choose one and your mail program opens a request. The host replies to confirm.</p>
{{range .Days}}<section><h2>{{.Label}}</h2>{{range .Slots}}<a class="slot" href="{{.Link}}">{{.Time}}</a>{{end}}</section>
{{end}}{{else}}<p>No times are open right now. Check back later.</p>{{end}}
<small>Updated {{.Updated}}. Times can be taken after this page was made.</small>
</main>
</body>
</html>
`))

// HTML renders a self-contained page with one mailto button per slot. The page holds free times
// only: no event titles, no attendees, nothing about the rest of the calendar.
func HTML(r *Result, now time.Time) ([]byte, error) {
	loc, err := tz.Load(r.TZ)
	if err != nil {
		return nil, err
	}
	var days []pageDay
	for _, s := range r.Slots {
		label := s.Start.In(loc).Format("Monday 2 January")
		if len(days) == 0 || days[len(days)-1].Label != label {
			days = append(days, pageDay{Label: label})
		}
		d := &days[len(days)-1]
		d.Slots = append(d.Slots, pageSlot{Time: s.Start.In(loc).Format("15:04"), Link: template.URL(Mailto(r.Link, s, r.LengthMin, loc))})
	}
	data := map[string]any{
		"Title": r.Link.Title, "Length": r.LengthMin, "Where": r.Link.Location, "Description": r.Link.Description,
		"TZ": r.TZ, "Days": days, "Updated": now.In(loc).Format("2 Jan 2006 15:04 MST"),
	}
	var buf bytes.Buffer
	if err := page.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// NeedsHost errors for a link that cannot make mail links.
func NeedsHost(l *model.BookingLink) error {
	if l.HostEmail == "" {
		return fmt.Errorf("link %q has no host email, set one with --email: %w", l.Slug, model.ErrValidation)
	}
	return nil
}
