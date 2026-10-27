package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// Company text creates or reuses an organization (owner decision 2026-10-27). people.company stays
// the source text. Wherever it is set, the text is matched to a live organization by NormalizeCompany
// (exact) or through merge_alias, else a new organization is made, and the person gets a membership.
// Close but different names never auto-merge: they show up as organization merge candidates
// (FindOrgCandidates). Junk strings (company_ignore) never create anything.
// See patterns/convert-text-to-records-in-review-passes.

// Match kinds of an organization found for a text.
const (
	MatchExact = "exact"
	MatchAlias = "alias"
)

// OrgMatch is an existing organization found for a company text.
type OrgMatch struct {
	OrgID int64   `json:"org_id"`
	Name  string  `json:"name"`
	Kind  string  `json:"kind"`
	Score float64 `json:"score,omitempty"`
}

type orgIndex struct {
	exact map[string]OrgMatch
	list  []OrgMatch
	keys  []string
}

func loadOrgIndex(ctx context.Context, db *sql.DB) (*orgIndex, error) {
	ix := &orgIndex{exact: map[string]OrgMatch{}}
	rows, err := db.QueryContext(ctx, `SELECT id, name FROM organizations WHERE archived = 0 ORDER BY id`)
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
		m.Kind = MatchExact
		if _, dup := ix.exact[k]; !dup {
			ix.exact[k] = m
		}
		ix.list = append(ix.list, m)
		ix.keys = append(ix.keys, k)
	}
	rows.Close()
	// Names of organizations that were merged away resolve to the survivor.
	rows, err = db.QueryContext(ctx, `SELECT o.name, sv.id, sv.name FROM organizations o
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
			m.Kind = MatchAlias
			ix.exact[k] = m
		}
	}
	return ix, rows.Err()
}

func (ix *orgIndex) add(m OrgMatch) {
	k := NormalizeCompany(m.Name)
	m.Kind = MatchExact
	ix.exact[k] = m
	ix.list = append(ix.list, m)
	ix.keys = append(ix.keys, k)
}

// nearName says whether two normalized names look like the same organization written differently.
func nearName(a, b string) (string, float64, bool) {
	if a == "" || b == "" {
		return "", 0, false
	}
	if a == b {
		return "same name", 1, true
	}
	if len(a) < 4 || len(b) < 4 {
		return "", 0, false
	}
	short, long := a, b
	if len(short) > len(long) {
		short, long = long, short
	}
	if strings.HasPrefix(long, short+" ") {
		return "one name starts with the other", 0.9, true
	}
	diff := len(long) - len(short)
	if float64(diff) > 0.15*float64(len(long)) {
		return "", 0, false
	}
	score := 1 - float64(levenshtein(a, b))/float64(max(len(a), len(b)))
	if score >= 0.85 {
		return "close spelling", score, true
	}
	return "", 0, false
}

// similar lists live organizations whose name is near key, best first, never the organization skip.
func (ix *orgIndex) similar(key string, skip int64) []OrgMatch {
	var out []OrgMatch
	for i, k := range ix.keys {
		if ix.list[i].OrgID == skip {
			continue
		}
		if _, score, ok := nearName(key, k); ok {
			m := ix.list[i]
			m.Score = score
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].OrgID < out[j].OrgID
	})
	return out
}

// OrgAutoRepo creates or reuses organizations from company text and attaches people to them.
type OrgAutoRepo struct {
	db     *sql.DB
	orgs   *OrgRepo
	links  *LinkRepo
	ignore *IgnoreRepo
}

// NewOrgAutoRepo creates an OrgAutoRepo.
func NewOrgAutoRepo(db *sql.DB) *OrgAutoRepo {
	return &OrgAutoRepo{db: db, orgs: NewOrgRepo(db), links: NewLinkRepo(db), ignore: NewIgnoreRepo(db)}
}

// resolver holds the organization index and the ignore set for a run of lookups.
type resolver struct {
	ix     *orgIndex
	ignore map[string]bool
}

func (r *OrgAutoRepo) newResolver(ctx context.Context) (*resolver, error) {
	ix, err := loadOrgIndex(ctx, r.db)
	if err != nil {
		return nil, err
	}
	set, err := r.ignore.Set(ctx)
	if err != nil {
		return nil, err
	}
	return &resolver{ix: ix, ignore: set}, nil
}

func (c *resolver) ignored(text string) bool { return ignoredBy(c.ignore, text) }

func (c *resolver) lookup(text string) (OrgMatch, bool) {
	m, ok := c.ix.exact[NormalizeCompany(text)]
	return m, ok
}

// ensure returns the organization for the text, creating it when none matches.
func (r *OrgAutoRepo) ensure(ctx context.Context, c *resolver, text string) (OrgMatch, bool, error) {
	text = strings.Join(strings.Fields(text), " ")
	if m, ok := c.lookup(text); ok {
		return m, false, nil
	}
	o, err := r.orgs.Create(ctx, model.CreateOrgInput{Name: text})
	if err != nil {
		var dup *DuplicateOrgError
		if errors.As(err, &dup) { // a domain twin cannot happen without a domain, but never fail on it
			return OrgMatch{OrgID: dup.Existing.ID, Name: dup.Existing.Name, Kind: MatchExact}, false, nil
		}
		return OrgMatch{}, false, err
	}
	m := OrgMatch{OrgID: o.ID, Name: o.Name, Kind: MatchExact}
	c.ix.add(m)
	return m, true, nil
}

// Lookup finds the organization a text names without creating anything. ok is false for no match,
// an ignored text and an empty text.
func (r *OrgAutoRepo) Lookup(ctx context.Context, text string) (OrgMatch, bool, error) {
	c, err := r.newResolver(ctx)
	if err != nil {
		return OrgMatch{}, false, err
	}
	if c.ignored(text) {
		return OrgMatch{}, false, nil
	}
	m, ok := c.lookup(text)
	return m, ok, nil
}

// Membership modes of ApplyCompany.
const (
	// ModePrimary makes the organization the person's primary one. An older primary stays as a
	// non-primary membership. Used by the entry points that edit a person (CLI, MCP, TUI, import).
	ModePrimary = "primary"
	// ModeRepoint moves the person's primary membership to the organization in place, keeping role
	// and dates (the DAV rule, E4). A person with no primary gets one.
	ModeRepoint = "repoint"
	// ModeAdd adds a membership, primary only when the person has none, and never moves an existing
	// one. A person already linked to the organization is left alone. Used by the backfill.
	ModeAdd = "add"
)

// CompanyChange is what one ApplyCompany did, enough to undo and redo it as one step.
type CompanyChange struct {
	PersonID int64  `json:"person_id"`
	Text     string `json:"text"`
	Ignored  bool   `json:"ignored,omitempty"`
	OrgID    int64  `json:"org_id,omitempty"`
	OrgName  string `json:"org_name,omitempty"`
	// OrgCreated is true when the organization was made by this call.
	OrgCreated bool `json:"org_created,omitempty"`
	// Similar are existing organizations with a close name to a new one: merge candidates, never
	// merged automatically.
	Similar []OrgMatch `json:"similar,omitempty"`

	// LinkCreated, LinkID, Before, After and Demoted describe the membership write.
	LinkCreated bool        `json:"link_created,omitempty"`
	LinkID      int64       `json:"link_id,omitempty"`
	Before      *LinkRecord `json:"-"`
	After       *LinkRecord `json:"-"`
	Demoted     []int64     `json:"-"`
	// Repoint is set when the primary membership moved in place.
	RepointID   int64 `json:"-"`
	RepointFrom int64 `json:"-"`
}

// Empty reports whether nothing was written.
func (c CompanyChange) Empty() bool {
	return !c.OrgCreated && !c.LinkCreated && c.Before == nil && c.RepointID == 0
}

// ApplyCompany matches the person's company text to an organization (creating it when none matches)
// and gives the person a membership according to mode. An empty or ignored text does nothing.
// people.company is not touched: the caller keeps it as the source text.
func (r *OrgAutoRepo) ApplyCompany(ctx context.Context, personID int64, text, mode string) (CompanyChange, error) {
	c, err := r.newResolver(ctx)
	if err != nil {
		return CompanyChange{}, err
	}
	return r.apply(ctx, c, personID, text, mode)
}

func (r *OrgAutoRepo) apply(ctx context.Context, c *resolver, personID int64, text, mode string) (CompanyChange, error) {
	text = strings.Join(strings.Fields(text), " ")
	ch := CompanyChange{PersonID: personID, Text: text}
	if text == "" {
		return ch, nil
	}
	if c.ignored(text) {
		ch.Ignored = true
		return ch, nil
	}
	m, created, err := r.ensure(ctx, c, text)
	if err != nil {
		return ch, err
	}
	ch.OrgID, ch.OrgName, ch.OrgCreated = m.OrgID, m.Name, created
	if created {
		ch.Similar = c.ix.similar(NormalizeCompany(m.Name), m.OrgID)
	}
	if err := r.attach(ctx, &ch, mode); err != nil {
		_ = r.Undo(ctx, ch)
		return CompanyChange{}, err
	}
	return ch, nil
}

func (r *OrgAutoRepo) attach(ctx context.Context, ch *CompanyChange, mode string) error {
	var memID, primaryOrg int64
	err := r.db.QueryRowContext(ctx, `SELECT id, org_id FROM membership WHERE person_id = ? AND is_primary = 1 AND deleted_at IS NULL`, ch.PersonID).Scan(&memID, &primaryOrg)
	hasPrimary := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("find primary membership: %w", err)
	}
	if hasPrimary && primaryOrg == ch.OrgID {
		return nil
	}
	if hasPrimary && mode == ModeRepoint {
		if _, err := r.db.ExecContext(ctx, `UPDATE membership SET org_id = ?, updated_at = datetime('now') WHERE id = ?`, ch.OrgID, memID); err != nil {
			return fmt.Errorf("move primary membership: %w", err)
		}
		if _, err := r.db.ExecContext(ctx, `UPDATE people SET updated_at = datetime('now') WHERE id = ?`, ch.PersonID); err != nil {
			return err
		}
		ch.RepointID, ch.RepointFrom = memID, primaryOrg
		return nil
	}
	var existing int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM membership WHERE person_id = ? AND org_id = ? AND deleted_at IS NULL`, ch.PersonID, ch.OrgID).Scan(&existing); err != nil {
		return fmt.Errorf("find membership: %w", err)
	}
	if existing > 0 && mode == ModeAdd {
		return nil
	}
	var attrs model.LinkAttrs
	if !hasPrimary || mode == ModePrimary || mode == ModeRepoint {
		yes := true
		attrs.Primary = &yes
	}
	if existing == 0 {
		var title string
		_ = r.db.QueryRowContext(ctx, `SELECT COALESCE(title, '') FROM people WHERE id = ?`, ch.PersonID).Scan(&title)
		if t := strings.TrimSpace(title); t != "" {
			attrs.Role = &t
		}
	}
	res, err := r.links.Link(ctx, Ref{Kind: "person", ID: ch.PersonID}, Ref{Kind: "organization", ID: ch.OrgID}, attrs)
	if err != nil {
		return err
	}
	if !res.Changed {
		return nil
	}
	ch.LinkCreated, ch.LinkID, ch.Before, ch.Demoted = res.Created, res.LinkID, res.Before, res.Demoted
	after, err := r.links.Get(ctx, "membership", res.LinkID)
	if err != nil {
		return err
	}
	ch.After = after
	return nil
}

func (r *OrgAutoRepo) setPrimaryFlag(ctx context.Context, ids []int64, on bool) error {
	for _, id := range ids {
		if _, err := r.db.ExecContext(ctx, `UPDATE membership SET is_primary = ?, updated_at = datetime('now') WHERE id = ?`, on, id); err != nil {
			return err
		}
	}
	return nil
}

// Undo reverses a change: the membership goes back, then a created organization is archived.
func (r *OrgAutoRepo) Undo(ctx context.Context, ch CompanyChange) error {
	if ch.RepointID != 0 {
		if _, err := r.db.ExecContext(ctx, `UPDATE membership SET org_id = ?, updated_at = datetime('now') WHERE id = ?`, ch.RepointFrom, ch.RepointID); err != nil {
			return err
		}
	}
	switch {
	case ch.LinkCreated:
		rec, err := r.links.Get(ctx, "membership", ch.LinkID)
		if err == nil && !rec.Deleted {
			if _, err := r.links.Unlink(ctx, Ref{Kind: "person", ID: rec.A}, Ref{Kind: "organization", ID: rec.B}, UnlinkOptions{LinkID: rec.ID}); err != nil {
				return err
			}
		}
	case ch.Before != nil:
		if _, err := r.links.Overwrite(ctx, *ch.Before); err != nil {
			return err
		}
	}
	if err := r.setPrimaryFlag(ctx, ch.Demoted, true); err != nil {
		return err
	}
	if ch.OrgCreated {
		if err := r.orgs.Archive(ctx, ch.OrgID); err != nil {
			return err
		}
	}
	return nil
}

// Redo repeats a change that Undo reversed.
func (r *OrgAutoRepo) Redo(ctx context.Context, ch CompanyChange) error {
	if ch.OrgCreated {
		if err := r.orgs.Restore(ctx, ch.OrgID); err != nil {
			return err
		}
	}
	if ch.RepointID != 0 {
		if _, err := r.db.ExecContext(ctx, `UPDATE membership SET org_id = ?, updated_at = datetime('now') WHERE id = ?`, ch.OrgID, ch.RepointID); err != nil {
			return err
		}
	}
	if err := r.setPrimaryFlag(ctx, ch.Demoted, false); err != nil {
		return err
	}
	switch {
	case ch.LinkCreated:
		if err := r.links.RestoreLink(ctx, "membership", ch.LinkID); err != nil {
			return err
		}
	case ch.After != nil:
		if _, err := r.links.Overwrite(ctx, *ch.After); err != nil {
			return err
		}
	}
	return nil
}

// ---- backfill ---------------------------------------------------------------------------------

// Backfill actions of a group.
const (
	BackfillCreate = "create"
	BackfillReuse  = "reuse"
	BackfillIgnore = "ignore"
)

// TextVariant is one spelling in a group and how many people use it.
type TextVariant struct {
	Text   string `json:"text"`
	People int    `json:"people"`
}

// BackfillGroup is a set of people whose company text normalizes to the same key.
type BackfillGroup struct {
	Key      string        `json:"key"`
	Display  string        `json:"display"`
	People   int           `json:"people"`
	Variants []TextVariant `json:"variants,omitempty"`
	// Action is create (no organization yet), reuse (an organization or alias matches) or ignore.
	Action  string `json:"action"`
	OrgID   int64  `json:"org_id,omitempty"`
	OrgName string `json:"org_name,omitempty"`
	// ToLink is how many people still need a membership, Linked how many already have one.
	ToLink int `json:"to_link"`
	Linked int `json:"linked"`

	personIDs []int64
}

// Record is the group as the CLI and MCP print it.
func (g BackfillGroup) Record() map[string]any {
	return map[string]any{
		"key": g.Key, "text": g.Display, "people": g.People, "action": g.Action, "organization": g.OrgName,
		"org_id": g.OrgID, "to_link": g.ToLink, "linked": g.Linked, "variants": g.Variants,
	}
}

// BackfillReport is the dry-run result: the counts and the groups, groups of 3 or more people first.
type BackfillReport struct {
	PeopleWithCompany int `json:"people_with_company"`
	Groups            int `json:"groups"`
	OrgsToCreate      int `json:"orgs_to_create"`
	OrgsReused        int `json:"orgs_reused"`
	IgnoredPeople     int `json:"ignored_people"`
	IgnoredGroups     int `json:"ignored_groups"`
	PeopleToLink      int `json:"people_to_link"`
	AlreadyLinked     int `json:"already_linked"`
	GroupsOf3Plus     int `json:"groups_of_3_plus"`
	// List holds every group: groups of 3 or more people first (biggest first), then the rest.
	List []BackfillGroup `json:"list"`
}

// Line renders the counts as one line.
func (r BackfillReport) Line() string {
	return fmt.Sprintf("%d people with company text, %d groups, %d organizations to create, %d reused, %d people ignored (%d groups), %d people to link",
		r.PeopleWithCompany, r.Groups, r.OrgsToCreate, r.OrgsReused, r.IgnoredPeople, r.IgnoredGroups, r.PeopleToLink)
}

// BackfillPlan reads the people with company text and says what a backfill would do. It writes nothing.
func (r *OrgAutoRepo) BackfillPlan(ctx context.Context) (BackfillReport, error) {
	c, err := r.newResolver(ctx)
	if err != nil {
		return BackfillReport{}, err
	}
	return r.plan(ctx, c)
}

func (r *OrgAutoRepo) plan(ctx context.Context, c *resolver) (BackfillReport, error) {
	var rep BackfillReport
	// Keys of the organizations each person is a live member of.
	member := map[int64]map[string]bool{}
	mrows, err := r.db.QueryContext(ctx, `SELECT m.person_id, o.name FROM membership m JOIN organizations o ON o.id = m.org_id
		WHERE m.deleted_at IS NULL AND o.archived = 0`)
	if err != nil {
		return rep, fmt.Errorf("list memberships: %w", err)
	}
	for mrows.Next() {
		var pid int64
		var name string
		if err := mrows.Scan(&pid, &name); err != nil {
			mrows.Close()
			return rep, err
		}
		if member[pid] == nil {
			member[pid] = map[string]bool{}
		}
		member[pid][NormalizeCompany(name)] = true
	}
	mrows.Close()

	rows, err := r.db.QueryContext(ctx, `SELECT id, company FROM people WHERE archived = 0 AND company IS NOT NULL AND trim(company) <> '' ORDER BY id`)
	if err != nil {
		return rep, fmt.Errorf("list company text: %w", err)
	}
	defer rows.Close()
	groups := map[string]*BackfillGroup{}
	variants := map[string]map[string]int{}
	for rows.Next() {
		var id int64
		var company string
		if err := rows.Scan(&id, &company); err != nil {
			return rep, err
		}
		text := strings.Join(strings.Fields(company), " ")
		key := NormalizeCompany(text)
		if key == "" {
			key = IgnoreKey(text)
		}
		g := groups[key]
		if g == nil {
			g = &BackfillGroup{Key: key}
			groups[key] = g
			variants[key] = map[string]int{}
		}
		g.People++
		g.personIDs = append(g.personIDs, id)
		variants[key][text]++
		rep.PeopleWithCompany++
	}
	if err := rows.Err(); err != nil {
		return rep, err
	}
	for key, g := range groups {
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
		switch {
		case c.ignored(g.Display):
			g.Action = BackfillIgnore
			rep.IgnoredGroups++
			rep.IgnoredPeople += g.People
		default:
			if m, ok := c.lookup(g.Display); ok {
				g.Action, g.OrgID, g.OrgName = BackfillReuse, m.OrgID, m.Name
				rep.OrgsReused++
			} else {
				g.Action, g.OrgName = BackfillCreate, g.Display
				rep.OrgsToCreate++
			}
			for _, pid := range g.personIDs {
				if g.Action == BackfillReuse && member[pid][NormalizeCompany(g.OrgName)] {
					g.Linked++
				} else {
					g.ToLink++
				}
			}
			rep.PeopleToLink += g.ToLink
			rep.AlreadyLinked += g.Linked
		}
		if g.People >= 3 {
			rep.GroupsOf3Plus++
		}
		rep.List = append(rep.List, *g)
	}
	rep.Groups = len(rep.List)
	sort.Slice(rep.List, func(i, j int) bool {
		a, b := rep.List[i], rep.List[j]
		if (a.People >= 3) != (b.People >= 3) {
			return a.People >= 3
		}
		if a.People != b.People {
			return a.People > b.People
		}
		return a.Key < b.Key
	})
	return rep, nil
}

// BackfillChange is what one BackfillApply did: one undo step.
type BackfillChange struct {
	Changes     []CompanyChange `json:"-"`
	OrgsCreated int             `json:"orgs_created"`
	Linked      int             `json:"people_linked"`
	Created     []string        `json:"created,omitempty"`
}

// Empty reports whether nothing was written.
func (b BackfillChange) Empty() bool { return len(b.Changes) == 0 }

// BackfillApply runs the backfill as one batch: it creates the missing organizations and gives each
// person with company text a membership (primary when they have none, existing memberships stay).
// people.company is not changed. It is idempotent: a second run finds nothing to do. On an error the
// partial work is undone.
func (r *OrgAutoRepo) BackfillApply(ctx context.Context) (BackfillReport, BackfillChange, error) {
	var out BackfillChange
	c, err := r.newResolver(ctx)
	if err != nil {
		return BackfillReport{}, out, err
	}
	rep, err := r.plan(ctx, c)
	if err != nil {
		return rep, out, err
	}
	fail := func(err error) (BackfillReport, BackfillChange, error) {
		_ = r.UndoBackfill(ctx, out)
		return rep, BackfillChange{}, err
	}
	for _, g := range rep.List {
		if g.Action == BackfillIgnore || g.ToLink == 0 {
			continue
		}
		for _, pid := range g.personIDs {
			ch, err := r.apply(ctx, c, pid, g.Display, ModeAdd)
			if err != nil {
				return fail(err)
			}
			if ch.Empty() {
				continue
			}
			out.Changes = append(out.Changes, ch)
			if ch.OrgCreated {
				out.OrgsCreated++
				out.Created = append(out.Created, ch.OrgName)
			}
			if ch.LinkCreated || ch.Before != nil {
				out.Linked++
			}
		}
	}
	return rep, out, nil
}

// PersonCompany is a person and the company text to apply to them.
type PersonCompany struct {
	PersonID int64
	Text     string
}

// ApplyCompanies applies the company text of several people with one organization index, as
// ApplyCompany does for one. On an error the work done so far is undone.
func (r *OrgAutoRepo) ApplyCompanies(ctx context.Context, items []PersonCompany, mode string) (BackfillChange, error) {
	var out BackfillChange
	if len(items) == 0 {
		return out, nil
	}
	c, err := r.newResolver(ctx)
	if err != nil {
		return out, err
	}
	for _, it := range items {
		ch, err := r.apply(ctx, c, it.PersonID, it.Text, mode)
		if err != nil {
			_ = r.UndoBackfill(ctx, out)
			return BackfillChange{}, err
		}
		if ch.Empty() {
			continue
		}
		out.Changes = append(out.Changes, ch)
		if ch.OrgCreated {
			out.OrgsCreated++
			out.Created = append(out.Created, ch.OrgName)
		}
		if ch.LinkCreated || ch.Before != nil || ch.RepointID != 0 {
			out.Linked++
		}
	}
	return out, nil
}

// UndoBackfill reverses a backfill, last change first.
func (r *OrgAutoRepo) UndoBackfill(ctx context.Context, b BackfillChange) error {
	for i := len(b.Changes) - 1; i >= 0; i-- {
		if err := r.Undo(ctx, b.Changes[i]); err != nil {
			return err
		}
	}
	return nil
}

// RedoBackfill repeats a backfill that UndoBackfill reversed.
func (r *OrgAutoRepo) RedoBackfill(ctx context.Context, b BackfillChange) error {
	for _, ch := range b.Changes {
		if err := r.Redo(ctx, ch); err != nil {
			return err
		}
	}
	return nil
}

// ---- organization merge candidates --------------------------------------------------------------

// OrgCandidate is a pair of organizations that may be the same one. Pairs are never merged on their
// own: merge them with OrgRepo.MergeOrgs (crm org merge).
type OrgCandidate struct {
	AID    int64   `json:"a_id"`
	AName  string  `json:"a_name"`
	BID    int64   `json:"b_id"`
	BName  string  `json:"b_name"`
	Reason string  `json:"reason"`
	Score  float64 `json:"score"`
}

// FindOrgCandidates lists live organization pairs with near-duplicate names (same normalized name,
// close spelling, one name starting with the other) or the same domain, best first. limit 0 means
// no limit.
func (r *OrgRepo) FindOrgCandidates(ctx context.Context, limit int) ([]OrgCandidate, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, COALESCE(domain, '') FROM organizations WHERE archived = 0 ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	defer rows.Close()
	type org struct {
		id           int64
		name, key, d string
	}
	var all []org
	for rows.Next() {
		var o org
		if err := rows.Scan(&o.id, &o.name, &o.d); err != nil {
			return nil, err
		}
		o.key, o.d = NormalizeCompany(o.name), normalizeDomain(o.d)
		all = append(all, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []OrgCandidate
	for i := range all {
		for j := i + 1; j < len(all); j++ {
			a, b := all[i], all[j]
			reason, score, ok := nearName(a.key, b.key)
			if !ok && a.d != "" && a.d == b.d {
				reason, score, ok = "same domain "+a.d, 0.95, true
			}
			if ok {
				out = append(out, OrgCandidate{AID: a.id, AName: a.name, BID: b.id, BName: b.name, Reason: reason, Score: score})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
