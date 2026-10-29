package query_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/query"
	"github.com/stretchr/testify/require"
)

// now is the fixed clock of every test: Thursday 2026-10-29.
var now = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

var stages = []string{"lead", "prospect", "proposal", "negotiation", "won", "lost"}

func testEnv() query.Env {
	return query.Env{Now: now, Choices: func(v string) []string {
		if v == "deal_stage" {
			return stages
		}
		return nil
	}}
}

func exec(t *testing.T, d *sql.DB, q string, args ...any) {
	t.Helper()
	_, err := d.Exec(q, args...)
	require.NoError(t, err, q)
}

// fixture builds a database with six people, three organizations, four deals, five tasks, two notes
// and a few tags. The uuids are readable: uuid-ann, uuid-initech.
func fixture(t *testing.T) *sql.DB {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	p := func(first, last string, company, title, loc any, bm, bd, tier any, lc any, fav int) {
		exec(t, d, `INSERT INTO people (uuid, first_name, last_name, company, title, location, birthday_month, birthday_day, priority_tier, last_contacted_at, is_favorite)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, "uuid-"+first, first, last, company, title, loc, bm, bd, tier, lc, fav)
	}
	p("Ann", "Alder", "Initech", "CTO", "Austin", 11, 5, 1, "2026-01-10", 1)
	p("Bob", "Brown", "Globex", "CEO", "Boston", 10, 30, 2, "2026-10-20", 0)
	p("Cy", "Cole", nil, nil, nil, nil, nil, nil, nil, 0)
	p("Dee", "Dunn", "Initech", "Engineer", nil, 10, 20, 3, "2026-08-01", 0)
	p("Eve", "Egan", "100% Real_Co", nil, nil, 12, 25, 1, "2026-10-28", 0)
	p("Fay", "Frost", nil, nil, nil, 1, 5, nil, nil, 0)
	exec(t, d, `INSERT INTO person_emails (person_id, email, is_primary) VALUES (1, 'ann@x.com', 1), (2, 'bob@globex.io', 1)`)

	o := func(name string, industry any) {
		exec(t, d, `INSERT INTO organizations (uuid, name, industry) VALUES (?, ?, ?)`, "uuid-"+name, name, industry)
	}
	o("Initech", "software")
	o("Globex", "manufacturing")
	o("Umbrella", nil)
	exec(t, d, `INSERT INTO membership (uuid, person_id, org_id) VALUES ('m1', 1, 1), ('m2', 4, 1), ('m3', 2, 2)`)

	dl := func(title string, value any, stage string) {
		exec(t, d, `INSERT INTO deals (uuid, title, value, stage) VALUES (?, ?, ?, ?)`, "uuid-"+title, title, value, stage)
	}
	dl("Site build", 5000, "lead")
	dl("Big deal", 20000, "negotiation")
	dl("Won deal", 1000, "won")
	dl("Small", nil, "lead")
	exec(t, d, `INSERT INTO deal_org (uuid, deal_id, org_id) VALUES ('do1', 1, 1), ('do2', 2, 2)`)
	exec(t, d, `INSERT INTO deal_person (uuid, deal_id, person_id) VALUES ('dp1', 2, 2)`)

	tk := func(title string, due any, prio string, done int) {
		exec(t, d, `INSERT INTO tasks (uuid, title, due_at, priority, completed) VALUES (?, ?, ?, ?, ?)`, "uuid-"+title, title, due, prio, done)
	}
	tk("Call Ann", "2026-10-30", "high", 0)
	tk("Send quote", "2026-11-02", "medium", 0)
	tk("Old thing", "2026-10-01", "low", 1)
	tk("Someday", nil, "medium", 0)
	tk("Next month", "2026-12-15", "high", 0)
	exec(t, d, `INSERT INTO activity_target (uuid, source_kind, source_id, target_kind, target_id) VALUES ('a1', 'task', 1, 'person', 1)`)

	exec(t, d, `INSERT INTO interactions (uuid, type, subject, content, occurred_at) VALUES
		('n1', 'call', 'Intro call', 'Talked about the roadmap', '2026-10-01 10:00:00'),
		('n2', 'note', NULL, 'Follow up on pricing', '2026-10-25 09:00:00')`)
	exec(t, d, `INSERT INTO activity_target (uuid, source_kind, source_id, target_kind, target_id) VALUES
		('a2', 'interaction', 1, 'organization', 1), ('a3', 'interaction', 2, 'organization', 1), ('a4', 'interaction', 2, 'deal', 1)`)

	exec(t, d, `INSERT INTO tags (uuid, name) VALUES ('t1', 'client'), ('t2', 'client/vip'), ('t3', 'lead')`)
	exec(t, d, `INSERT INTO taggings (tag_id, entity_type, entity_id) VALUES
		(1, 'person', 1), (2, 'person', 2), (3, 'person', 3), (1, 'organization', 1)`)
	return d
}

func labels(rows []query.Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Label
	}
	return out
}

func run(t *testing.T, d *sql.DB, kind string, q query.Query) []query.Row {
	t.Helper()
	rows, err := query.Run(context.Background(), d, kind, q, testEnv())
	require.NoError(t, err)
	return rows
}
