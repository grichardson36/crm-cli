package repo

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// Company text to organization conversion (owner decision Q21). people.company stays free text.
// This turns it into organizations in review passes: people are grouped by the normalized text, each
// group is matched to an existing organization when possible, and nothing is created without an
// explicit decision for that group. See patterns/convert-text-to-records-in-review-passes.

// ReviewTag marks people whose phone ORG matched no organization (set by DAV). They are flagged in
// the review and lose the tag when converted.
const ReviewTag = "org-review"

const (
	skipPrefix = "orgconvert.skip:"
	donePrefix = "orgconvert.done:"
)

// Convert action names.
const (
	ConvertCreate = "create"
	ConvertLink   = "link"
	ConvertSkip   = "skip"
	ConvertUnskip = "unskip"
)

// companySuffixes are dropped from the end of a company text for grouping only.
var companySuffixes = map[string]bool{
	"inc": true, "incorporated": true, "llc": true, "ltd": true, "limited": true, "corp": true, "corporation": true,
	"co": true, "company": true, "gmbh": true, "plc": true, "llp": true, "lp": true, "pllc": true, "sa": true, "ag": true, "bv": true,
}

// NormalizeCompany is the grouping key of a company text: trimmed, case-folded, whitespace collapsed,
// punctuation removed, "&" read as "and", a leading "the" and trailing legal suffixes (Inc, LLC, Ltd,
// Corp, Co and their dotted forms) dropped. It is used for grouping and matching only, the text
// itself is never rewritten. A text that is only a suffix keeps it.
func NormalizeCompany(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r == '&':
			b.WriteString(" and ")
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '.' || r == '\'' || r == '’':
			// "L.L.C." and "Bob's" lose the mark without splitting the word
		default:
			b.WriteByte(' ')
		}
	}
	words := strings.Fields(b.String())
	full := strings.Join(words, " ")
	if len(words) > 1 && words[0] == "the" {
		words = words[1:]
	}
	for len(words) > 1 && companySuffixes[words[len(words)-1]] {
		words = words[:len(words)-1]
	}
	if len(words) == 0 {
		return full
	}
	return strings.Join(words, " ")
}

// ConvertPerson is one person in a group.
type ConvertPerson struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Title      string `json:"title,omitempty"`
	Company    string `json:"company"`
	Review     bool   `json:"review,omitempty"`
	HasPrimary bool   `json:"has_primary,omitempty"`
}

// TextVariant is one spelling in a group and how many people use it.
type TextVariant struct {
	Text   string `json:"text"`
	People int    `json:"people"`
}

// OrgMatch is an existing organization suggested for a group. Kind is exact (same normalized name),
// alias (the name of a merged-away organization) or fuzzy (a close name, a suggestion only).
type OrgMatch struct {
	OrgID int64   `json:"org_id"`
	Name  string  `json:"name"`
	Kind  string  `json:"kind"`
	Score float64 `json:"score,omitempty"`
}

// ConvertGroup is a set of people whose company text normalizes to the same key.
type ConvertGroup struct {
	Key      string          `json:"key"`
	Display  string          `json:"display"`
	Variants []TextVariant   `json:"variants"`
	Count    int             `json:"people"`
	Review   int             `json:"review"`
	Skipped  bool            `json:"skipped,omitempty"`
	Match    *OrgMatch       `json:"match,omitempty"`
	People   []ConvertPerson `json:"people_list"`
}

// Record is the group as the CLI and MCP print it. withPeople adds the people list.
func (g ConvertGroup) Record(withPeople bool) map[string]any {
	m := map[string]any{
		"key": g.Key, "display": g.Display, "people": g.Count, "variants": g.Variants, "review": g.Review, "skipped": g.Skipped,
	}
	if g.Match != nil {
		m["match"] = g.Match
		m["match_text"] = fmt.Sprintf("%s #%d (%s)", g.Match.Name, g.Match.OrgID, g.Match.Kind)
	}
	if withPeople {
		m["people_list"] = g.People
	}
	return m
}

// Exact reports whether the suggestion is an exact or alias match, the only kind that bulk accept takes.
func (g ConvertGroup) Exact() bool {
	return g.Match != nil && (g.Match.Kind == "exact" || g.Match.Kind == "alias")
}

// ConvertStats is the stats line: "N people with company text, M groups, K matched".
type ConvertStats struct {
	People  int `json:"people"`
	Groups  int `json:"groups"`
	Matched int `json:"matched"`
	Fuzzy   int `json:"fuzzy"`
	Skipped int `json:"skipped_groups"`
	Review  int `json:"review_people"`
}

// Line renders the stats line.
func (s ConvertStats) Line() string {
	return fmt.Sprintf("%d people with company text, %d groups, %d matched", s.People, s.Groups, s.Matched)
}

// ConvertRepo groups company text and applies conversion decisions.
type ConvertRepo struct {
	db    *sql.DB
	links *LinkRepo
	orgs  *OrgRepo
	tags  *TagRepo
}

// NewConvertRepo creates a ConvertRepo.
func NewConvertRepo(db *sql.DB) *ConvertRepo {
	return &ConvertRepo{db: db, links: NewLinkRepo(db), orgs: NewOrgRepo(db), tags: NewTagRepo(db)}
}

// Skipped lists the group keys the user chose to skip.
func (r *ConvertRepo) Skipped(ctx context.Context) (map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT key FROM meta WHERE key LIKE ?`, skipPrefix+"%")
	if err != nil {
		return nil, fmt.Errorf("list skipped groups: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out[strings.TrimPrefix(k, skipPrefix)] = true
	}
	return out, rows.Err()
}

// done maps a person id to the group key their text had when they were converted. A person whose
// text still has that key is not shown again, even when the organization was named differently.
func (r *ConvertRepo) done(ctx context.Context) (map[int64]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT key, value FROM meta WHERE key LIKE ?`, donePrefix+"%")
	if err != nil {
		return nil, fmt.Errorf("list converted people: %w", err)
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		var id int64
		if _, err := fmt.Sscan(strings.TrimPrefix(k, donePrefix), &id); err == nil {
			out[id] = v
		}
	}
	return out, rows.Err()
}

func (r *ConvertRepo) setDone(ctx context.Context, personID int64, key string, on bool) error {
	k := fmt.Sprintf("%s%d", donePrefix, personID)
	if on {
		_, err := r.db.ExecContext(ctx, `INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, key)
		return err
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM meta WHERE key = ?`, k)
	return err
}

func (r *ConvertRepo) setSkip(ctx context.Context, key string, on bool) error {
	if on {
		_, err := r.db.ExecContext(ctx, `INSERT INTO meta (key, value) VALUES (?, datetime('now')) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, skipPrefix+key)
		return err
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM meta WHERE key = ?`, skipPrefix+key)
	return err
}

type orgIndex struct {
	exact map[string]OrgMatch
	list  []OrgMatch
	keys  []string
}

func (r *ConvertRepo) orgIndex(ctx context.Context) (*orgIndex, error) {
	ix := &orgIndex{exact: map[string]OrgMatch{}}
	rows, err := r.db.QueryContext(ctx, `SELECT id, name FROM organizations WHERE archived = 0 ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	for rows.Next() {
		var m OrgMatch
		if err := rows.Scan(&m.OrgID, &m.Name); err != nil {
			rows.Close()
			return nil, err
		}
		k := NormalizeCompany(m.Name)
		if _, dup := ix.exact[k]; !dup {
			m.Kind = "exact"
			ix.exact[k] = m
		}
		ix.list = append(ix.list, m)
		ix.keys = append(ix.keys, k)
	}
	rows.Close()
	// Names of organizations that were merged away resolve to the survivor.
	rows, err = r.db.QueryContext(ctx, `SELECT o.name, sv.id, sv.name FROM organizations o
		JOIN merge_alias a ON a.kind = 'organization' AND a.old_id = o.id AND a.deleted_at IS NULL
		JOIN organizations sv ON sv.id = a.survivor_id AND sv.archived = 0 WHERE o.archived = 1`)
	if err != nil {
		return nil, fmt.Errorf("list organization aliases: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var oldName string
		var m OrgMatch
		if err := rows.Scan(&oldName, &m.OrgID, &m.Name); err != nil {
			return nil, err
		}
		k := NormalizeCompany(oldName)
		if _, dup := ix.exact[k]; !dup {
			m.Kind = "alias"
			ix.exact[k] = m
		}
	}
	return ix, rows.Err()
}

// closeMatch finds the nearest organization name to key, 0.85 or better, as a suggestion only.
func (ix *orgIndex) closeMatch(key string) *OrgMatch {
	if len(key) < 4 {
		return nil
	}
	var best *OrgMatch
	bestScore := 0.0
	for i, k := range ix.keys {
		if k == "" || len(k) < 4 {
			continue
		}
		diff := len(k) - len(key)
		if diff < 0 {
			diff = -diff
		}
		if float64(diff) > 0.15*float64(max(len(k), len(key))) {
			continue
		}
		score := 1 - float64(levenshtein(key, k))/float64(max(len(key), len(k)))
		if score >= 0.85 && score > bestScore {
			m := ix.list[i]
			m.Kind, m.Score = "fuzzy", score
			best, bestScore = &m, score
		}
	}
	return best
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

// Groups returns the groups still to review, biggest first, and the stats. People who already have a
// live primary membership are left out unless their text differs from that organization's name.
// Groups the user skipped are left out too (IncludeSkipped brings them back). Tagged people
// (ReviewTag) are included and flagged.
func (r *ConvertRepo) Groups(ctx context.Context, includeSkipped bool) ([]ConvertGroup, ConvertStats, error) {
	var stats ConvertStats
	skipped, err := r.Skipped(ctx)
	if err != nil {
		return nil, stats, err
	}
	doneKeys, err := r.done(ctx)
	if err != nil {
		return nil, stats, err
	}
	ix, err := r.orgIndex(ctx)
	if err != nil {
		return nil, stats, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT p.id, trim(p.first_name || ' ' || COALESCE(p.last_name, '')), COALESCE(p.title, ''), p.company,
			(SELECT o.name FROM membership m JOIN organizations o ON o.id = m.org_id
			  WHERE m.person_id = p.id AND m.is_primary = 1 AND m.deleted_at IS NULL AND o.archived = 0),
			EXISTS (SELECT 1 FROM taggings tg JOIN tags t ON t.id = tg.tag_id
			  WHERE tg.entity_type = 'person' AND tg.entity_id = p.id AND t.name = ?)
		FROM people p WHERE p.archived = 0 AND p.company IS NOT NULL AND trim(p.company) <> '' ORDER BY p.id`, ReviewTag)
	if err != nil {
		return nil, stats, fmt.Errorf("list company text: %w", err)
	}
	defer rows.Close()
	groups := map[string]*ConvertGroup{}
	variants := map[string]map[string]int{}
	for rows.Next() {
		var p ConvertPerson
		var primaryOrg sql.NullString
		var review bool
		if err := rows.Scan(&p.ID, &p.Name, &p.Title, &p.Company, &primaryOrg, &review); err != nil {
			return nil, stats, err
		}
		text := strings.Join(strings.Fields(p.Company), " ")
		key := NormalizeCompany(text)
		if doneKeys[p.ID] == key {
			continue // converted in an earlier pass and the text has not changed
		}
		if primaryOrg.Valid {
			if NormalizeCompany(primaryOrg.String) == key {
				continue // already converted
			}
			p.HasPrimary = true
		}
		p.Review = review
		g := groups[key]
		if g == nil {
			g = &ConvertGroup{Key: key}
			groups[key] = g
			variants[key] = map[string]int{}
		}
		g.Count++
		if review {
			g.Review++
		}
		variants[key][text]++
		g.People = append(g.People, p)
	}
	if err := rows.Err(); err != nil {
		return nil, stats, err
	}
	out := make([]ConvertGroup, 0, len(groups))
	for key, g := range groups {
		g.Skipped = skipped[key]
		for text, n := range variants[key] {
			g.Variants = append(g.Variants, TextVariant{Text: text, People: n})
		}
		sort.Slice(g.Variants, func(i, j int) bool {
			if g.Variants[i].People != g.Variants[j].People {
				return g.Variants[i].People > g.Variants[j].People
			}
			return g.Variants[i].Text < g.Variants[j].Text
		})
		g.Display = g.Variants[0].Text
		if m, ok := ix.exact[key]; ok {
			mm := m
			g.Match = &mm
		} else {
			g.Match = ix.closeMatch(key)
		}
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	shown := out[:0]
	for _, g := range out {
		if g.Skipped {
			stats.Skipped++
			if !includeSkipped {
				continue
			}
		} else {
			stats.Groups++
			stats.People += g.Count
			stats.Review += g.Review
			if g.Exact() {
				stats.Matched++
			} else if g.Match != nil {
				stats.Fuzzy++
			}
		}
		shown = append(shown, g)
	}
	return shown, stats, nil
}

// ConvertDecision is the choice for one group. Create makes an organization named Name (the most
// common spelling when empty) and links the group. Link links to OrgID. Skip remembers the group as
// skipped. Unskip forgets that.
type ConvertDecision struct {
	Key    string `json:"key"`
	Action string `json:"action"`
	OrgID  int64  `json:"org_id,omitempty"`
	Name   string `json:"name,omitempty"`
}

// ExactDecisions is the bulk accept: link every group that has an exact or alias match.
func ExactDecisions(groups []ConvertGroup) []ConvertDecision {
	var out []ConvertDecision
	for _, g := range groups {
		if g.Exact() {
			out = append(out, ConvertDecision{Key: g.Key, Action: ConvertLink, OrgID: g.Match.OrgID})
		}
	}
	return out
}

// ConvertChange is what one Apply did, exactly enough to undo and redo it as one step.
type ConvertChange struct {
	OrgsCreated []int64          `json:"orgs_created,omitempty"`
	LinkIDs     []int64          `json:"link_ids,omitempty"`
	TagRemoved  []int64          `json:"tag_removed,omitempty"`
	DoneAdded   map[int64]string `json:"done_added,omitempty"`
	SkipsAdded  []string         `json:"skips_added,omitempty"`
	SkipsLifted []string         `json:"skips_lifted,omitempty"`
	GroupsDone  int              `json:"groups_done"`
	PeopleDone  int              `json:"people_done"`
	// Created names the organizations made, for messages.
	Created []string `json:"created,omitempty"`
}

// Empty reports whether nothing changed.
func (c ConvertChange) Empty() bool {
	return len(c.OrgsCreated) == 0 && len(c.LinkIDs) == 0 && len(c.SkipsAdded) == 0 && len(c.SkipsLifted) == 0
}

// Apply runs decisions against the groups as they are now. Only the groups named are touched, a key
// that is no longer a group (already converted) is skipped quietly, which makes a re-run safe. The
// result is one ConvertChange. On an error the partial work is undone and the error returned.
func (r *ConvertRepo) Apply(ctx context.Context, decisions []ConvertDecision) (ConvertChange, error) {
	var ch ConvertChange
	groups, _, err := r.Groups(ctx, true)
	if err != nil {
		return ch, err
	}
	byKey := map[string]ConvertGroup{}
	for _, g := range groups {
		byKey[g.Key] = g
	}
	fail := func(err error) (ConvertChange, error) {
		_ = r.Undo(ctx, ch)
		return ConvertChange{}, err
	}
	for _, d := range decisions {
		switch d.Action {
		case ConvertSkip:
			if err := r.setSkip(ctx, d.Key, true); err != nil {
				return fail(err)
			}
			ch.SkipsAdded = append(ch.SkipsAdded, d.Key)
			continue
		case ConvertUnskip:
			if err := r.setSkip(ctx, d.Key, false); err != nil {
				return fail(err)
			}
			ch.SkipsLifted = append(ch.SkipsLifted, d.Key)
			continue
		case ConvertCreate, ConvertLink:
		default:
			return fail(fmt.Errorf("unknown action %q for %q (use create, link, skip or unskip): %w", d.Action, d.Key, model.ErrValidation))
		}
		g, ok := byKey[d.Key]
		if !ok {
			continue
		}
		orgID := d.OrgID
		if d.Action == ConvertCreate {
			name := strings.TrimSpace(d.Name)
			if name == "" {
				name = g.Display
			}
			o, err := r.orgs.Create(ctx, model.CreateOrgInput{Name: name})
			if err != nil {
				return fail(err)
			}
			orgID = o.ID
			ch.OrgsCreated = append(ch.OrgsCreated, o.ID)
			ch.Created = append(ch.Created, o.Name)
		} else if _, err := r.orgs.FindByID(ctx, orgID); err != nil {
			return fail(fmt.Errorf("organization %d for %q: %w", orgID, d.Key, err))
		}
		org := Ref{Kind: "organization", ID: orgID}
		for _, p := range g.People {
			person := Ref{Kind: "person", ID: p.ID}
			var attrs model.LinkAttrs
			if p.Title != "" {
				t := p.Title
				attrs.Role = &t
			}
			existing, err := r.links.Linked(ctx, person, "organization")
			if err != nil {
				return fail(err)
			}
			has := false
			for _, li := range existing {
				has = has || li.ID == orgID
			}
			if !has {
				res, err := r.links.Link(ctx, person, org, attrs)
				if err != nil {
					return fail(err)
				}
				if res.Created {
					ch.LinkIDs = append(ch.LinkIDs, res.LinkID)
				}
			}
			if err := r.setDone(ctx, p.ID, g.Key, true); err != nil {
				return fail(err)
			}
			if ch.DoneAdded == nil {
				ch.DoneAdded = map[int64]string{}
			}
			ch.DoneAdded[p.ID] = g.Key
			ch.PeopleDone++
			if p.Review {
				if err := r.tags.Remove(ctx, "person", p.ID, ReviewTag); err == nil {
					ch.TagRemoved = append(ch.TagRemoved, p.ID)
				}
			}
		}
		ch.GroupsDone++
	}
	return ch, nil
}

// Undo reverses a change: links are removed, created organizations deleted (hidden), review tags put
// back, skips restored.
func (r *ConvertRepo) Undo(ctx context.Context, ch ConvertChange) error {
	for i := len(ch.LinkIDs) - 1; i >= 0; i-- {
		rec, err := r.links.Get(ctx, "membership", ch.LinkIDs[i])
		if err != nil || rec.Deleted {
			continue
		}
		if _, err := r.links.Unlink(ctx, Ref{Kind: "person", ID: rec.A}, Ref{Kind: "organization", ID: rec.B}, UnlinkOptions{LinkID: rec.ID}); err != nil {
			return err
		}
	}
	for _, id := range ch.OrgsCreated {
		if err := r.orgs.Archive(ctx, id); err != nil {
			return err
		}
	}
	for _, pid := range ch.TagRemoved {
		if err := r.tags.Apply(ctx, "person", pid, ReviewTag); err != nil {
			return err
		}
	}
	for pid := range ch.DoneAdded {
		if err := r.setDone(ctx, pid, "", false); err != nil {
			return err
		}
	}
	for _, k := range ch.SkipsAdded {
		if err := r.setSkip(ctx, k, false); err != nil {
			return err
		}
	}
	for _, k := range ch.SkipsLifted {
		if err := r.setSkip(ctx, k, true); err != nil {
			return err
		}
	}
	return nil
}

// Redo repeats a change that Undo reversed.
func (r *ConvertRepo) Redo(ctx context.Context, ch ConvertChange) error {
	for _, id := range ch.OrgsCreated {
		if err := r.orgs.Restore(ctx, id); err != nil {
			return err
		}
	}
	for _, id := range ch.LinkIDs {
		if err := r.links.RestoreLink(ctx, "membership", id); err != nil {
			return err
		}
	}
	for _, pid := range ch.TagRemoved {
		if err := r.tags.Remove(ctx, "person", pid, ReviewTag); err != nil {
			return err
		}
	}
	for pid, key := range ch.DoneAdded {
		if err := r.setDone(ctx, pid, key, true); err != nil {
			return err
		}
	}
	for _, k := range ch.SkipsAdded {
		if err := r.setSkip(ctx, k, true); err != nil {
			return err
		}
	}
	for _, k := range ch.SkipsLifted {
		if err := r.setSkip(ctx, k, false); err != nil {
			return err
		}
	}
	return nil
}

// ConvertPlanRow says what one decision would do, for a preview before anything is written.
type ConvertPlanRow struct {
	Key     string `json:"key"`
	Action  string `json:"action"`
	People  int    `json:"people"`
	OrgID   int64  `json:"org_id,omitempty"`
	OrgName string `json:"org_name,omitempty"`
	// Note is set when the decision does nothing now (the group is gone or already converted).
	Note string `json:"note,omitempty"`
}

// Plan previews decisions against the groups as they are now. It writes nothing.
func (r *ConvertRepo) Plan(ctx context.Context, decisions []ConvertDecision) ([]ConvertPlanRow, error) {
	groups, _, err := r.Groups(ctx, true)
	if err != nil {
		return nil, err
	}
	byKey := map[string]ConvertGroup{}
	for _, g := range groups {
		byKey[g.Key] = g
	}
	out := make([]ConvertPlanRow, 0, len(decisions))
	for _, d := range decisions {
		row := ConvertPlanRow{Key: d.Key, Action: d.Action, OrgID: d.OrgID}
		g, ok := byKey[d.Key]
		switch d.Action {
		case ConvertSkip, ConvertUnskip:
			row.People = g.Count
		case ConvertCreate, ConvertLink:
			if !ok {
				row.Note = "no such group (already converted or the text changed)"
				break
			}
			row.People = g.Count
			if d.Action == ConvertCreate {
				row.OrgName = strings.TrimSpace(d.Name)
				if row.OrgName == "" {
					row.OrgName = g.Display
				}
			} else if o, err := r.orgs.FindByID(ctx, d.OrgID); err != nil {
				return nil, fmt.Errorf("organization %d for %q: %w", d.OrgID, d.Key, err)
			} else {
				row.OrgName = o.Name
			}
		default:
			return nil, fmt.Errorf("unknown action %q for %q (use create, link, skip or unskip): %w", d.Action, d.Key, model.ErrValidation)
		}
		out = append(out, row)
	}
	return out, nil
}
