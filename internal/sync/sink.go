package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/dedupe"
	"github.com/jdanielnd/crm-cli/internal/model"
)

const (
	entityPerson      = "person"
	entityInteraction = "interaction"
)

// dbSink writes adapter output into the database, or only counts it in a dry run.
type dbSink struct {
	opts   RunOptions
	people *repo.PersonRepo
	inter  *repo.InteractionRepo
	ext    *repo.ExternalIDRepo
	sync   *repo.SyncRepo

	idx       *matchIndex
	nextDummy int64 // negative ids for people a dry run would have created

	created, updated, skipped, inboxed, errs int
	lastErr                                  string
}

func newSink(db *sql.DB, opts RunOptions) *dbSink {
	return &dbSink{
		opts:   opts,
		people: repo.NewPersonRepo(db),
		inter:  repo.NewInteractionRepo(db),
		ext:    repo.NewExternalIDRepo(db),
		sync:   repo.NewSyncRepo(db),
		idx:    &matchIndex{db: db},
	}
}

func (s *dbSink) count(o Outcome, err error) (Outcome, error) {
	if err != nil {
		s.errs++
		s.lastErr = err.Error()
		return o, err
	}
	switch o {
	case Created:
		s.created++
	case Updated:
		s.updated++
	case Skipped:
		s.skipped++
	case Inboxed:
		s.inboxed++
	}
	return o, nil
}

// UpsertPerson implements Sink.
func (s *dbSink) UpsertPerson(ctx context.Context, source, externalID string, patch PersonPatch) (Outcome, error) {
	if strings.TrimSpace(source) == "" || strings.TrimSpace(externalID) == "" {
		return s.count("", fmt.Errorf("source and external id are required: %w", model.ErrValidation))
	}
	return s.count(s.upsertPerson(ctx, source, externalID, patch))
}

// LogInteraction implements Sink.
func (s *dbSink) LogInteraction(ctx context.Context, source, externalID string, rec InteractionRecord) (Outcome, error) {
	if strings.TrimSpace(source) == "" || strings.TrimSpace(externalID) == "" {
		return s.count("", fmt.Errorf("source and external id are required: %w", model.ErrValidation))
	}
	return s.count(s.logInteraction(ctx, source, externalID, rec))
}

// --- people ------------------------------------------------------------------------------

type inboxPayload struct {
	Kind       string             `json:"kind"`
	Reason     string             `json:"reason"`
	Candidates []int64            `json:"candidates,omitempty"`
	Patch      *PersonPatch       `json:"patch,omitempty"`
	Record     *InteractionRecord `json:"record,omitempty"`
}

func (s *dbSink) toInbox(ctx context.Context, source, externalID string, p inboxPayload) (Outcome, error) {
	if s.opts.DryRun {
		return Inboxed, nil
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("encode inbox payload: %w", err)
	}
	item, _, err := s.sync.InboxUpsert(ctx, source, externalID, p.Kind, string(raw))
	if err != nil {
		return "", err
	}
	if item.Status == model.InboxIgnored {
		return Skipped, nil
	}
	return Inboxed, nil
}

func (s *dbSink) upsertPerson(ctx context.Context, source, externalID string, patch PersonPatch) (Outcome, error) {
	link, err := s.ext.Lookup(ctx, source, entityPerson, externalID)
	switch {
	case err == nil:
		return s.updateLinked(ctx, source, link, patch)
	case !errors.Is(err, model.ErrNotFound):
		return "", err
	}

	cands, err := s.idx.candidates(ctx, patch.Emails, patch.Phones, patch.Handles)
	if err != nil {
		return "", err
	}
	switch len(cands) {
	case 1:
		return s.attach(ctx, source, externalID, cands[0], patch)
	case 0:
		if !s.opts.AllowCreate {
			return s.toInbox(ctx, source, externalID, inboxPayload{Kind: "person", Reason: "unmatched", Patch: &patch})
		}
		if !patchCreatable(patch) {
			return s.toInbox(ctx, source, externalID, inboxPayload{Kind: "person", Reason: "no_name", Patch: &patch})
		}
		return s.create(ctx, source, externalID, patch)
	default:
		return s.toInbox(ctx, source, externalID, inboxPayload{Kind: "person", Reason: "ambiguous", Candidates: cands, Patch: &patch})
	}
}

func patchCreatable(p PersonPatch) bool {
	return (p.FirstName != nil && strings.TrimSpace(*p.FirstName) != "") || len(p.Emails) > 0
}

// updateLinked refreshes a person already mapped to this external record.
func (s *dbSink) updateLinked(ctx context.Context, source string, link *model.ExternalID, patch PersonPatch) (Outcome, error) {
	id, active, err := s.resolveLinked(ctx, link.LocalID)
	if err != nil {
		return "", err
	}
	if !active {
		return Skipped, nil // archived by the owner: never resurrect it
	}
	changed, err := s.applyPatch(ctx, id, patch, true)
	if err != nil {
		return "", err
	}
	if !s.opts.DryRun {
		if _, err := s.ext.Upsert(ctx, repo.ExternalIDInput{Entity: entityPerson, LocalID: link.LocalID, LocalUUID: link.LocalUUID,
			Source: source, ExternalID: link.ExternalID, ETag: etagPtr(patch.ETag)}); err != nil {
			return "", err
		}
	}
	if changed {
		return Updated, nil
	}
	return Skipped, nil
}

// resolveLinked follows a merge to the surviving person. active is false for people the
// owner archived without merging.
func (s *dbSink) resolveLinked(ctx context.Context, localID int64) (id int64, active bool, err error) {
	id = localID
	for hops := 0; hops < 8; hops++ {
		archived, mergedInto, err := s.sync.PersonState(ctx, id)
		if errors.Is(err, model.ErrNotFound) {
			return 0, false, nil
		}
		if err != nil {
			return 0, false, err
		}
		if !archived {
			return id, true, nil
		}
		if mergedInto == 0 {
			return 0, false, nil
		}
		id = mergedInto
	}
	return 0, false, nil
}

// attach links an unmapped record to the one person it matched, filling only blank fields.
func (s *dbSink) attach(ctx context.Context, source, externalID string, personID int64, patch PersonPatch) (Outcome, error) {
	if personID < 0 { // person a dry run pretended to create
		return Skipped, nil
	}
	if _, err := s.applyPatch(ctx, personID, patch, false); err != nil {
		return "", err
	}
	if !s.opts.DryRun {
		if _, err := s.ext.Upsert(ctx, repo.ExternalIDInput{Entity: entityPerson, LocalID: personID,
			Source: source, ExternalID: externalID, ETag: etagPtr(patch.ETag)}); err != nil {
			if errors.Is(err, model.ErrConflict) {
				// The person is linked to a different record from this source already.
				return s.toInbox(ctx, source, externalID, inboxPayload{Kind: "person", Reason: "already_linked", Candidates: []int64{personID}, Patch: &patch})
			}
			return "", err
		}
	}
	return Updated, nil
}

func (s *dbSink) create(ctx context.Context, source, externalID string, patch PersonPatch) (Outcome, error) {
	if s.opts.DryRun {
		s.nextDummy--
		s.idx.add(s.nextDummy, patch.Emails, patch.Phones, patch.Handles)
		return Created, nil
	}
	_, err := s.createPerson(ctx, source, externalID, patch)
	if errors.Is(err, model.ErrConflict) {
		return s.toInbox(ctx, source, externalID, inboxPayload{Kind: "person", Reason: "name_conflict", Patch: &patch})
	}
	if err != nil {
		return "", err
	}
	return Created, nil
}

// createPerson inserts the person and maps it. It returns ErrConflict when the repo's
// duplicate check rejects it.
func (s *dbSink) createPerson(ctx context.Context, source, externalID string, patch PersonPatch) (*model.Person, error) {
	if !patchCreatable(patch) {
		return nil, fmt.Errorf("record has no name or email to create a person from: %w", model.ErrValidation)
	}
	in := model.CreatePersonInput{FirstName: strDeref(patch.FirstName), LastName: nonEmpty(patch.LastName),
		Title: nonEmpty(patch.Title), Company: nonEmpty(patch.Company), Location: nonEmpty(patch.Location)}
	if in.FirstName == "" {
		in.FirstName = strings.TrimSpace(patch.Emails[0])
	}
	if len(patch.Emails) > 0 {
		in.Email = &patch.Emails[0]
	}
	if len(patch.Phones) > 0 {
		in.Phone = &patch.Phones[0]
	}
	p, err := s.people.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	if _, err := s.applyPatch(ctx, p.ID, PersonPatch{Emails: patch.Emails, Phones: patch.Phones, Handles: patch.Handles}, false); err != nil {
		return nil, err
	}
	if _, err := s.ext.Upsert(ctx, repo.ExternalIDInput{Entity: entityPerson, LocalID: p.ID, LocalUUID: p.UUID,
		Source: source, ExternalID: externalID, ETag: etagPtr(patch.ETag)}); err != nil {
		return nil, err
	}
	s.idx.add(p.ID, patch.Emails, patch.Phones, patch.Handles)
	return p, nil
}

// applyPatch writes the differences between the patch and the person. With overwrite the patch
// wins on scalar fields it sets. Without it only blank fields are filled. Emails, phones and
// handles are only ever added. It reports whether anything changed (or would, in a dry run).
func (s *dbSink) applyPatch(ctx context.Context, id int64, patch PersonPatch, overwrite bool) (bool, error) {
	p, err := s.people.FindByID(ctx, id)
	if err != nil {
		return false, err
	}
	if err := s.people.LoadDetails(ctx, p); err != nil {
		return false, err
	}

	var in model.UpdatePersonInput
	changed := false
	pick := func(cur *string, want *string) *string {
		if want == nil || strings.TrimSpace(*want) == "" {
			return nil
		}
		w := strings.TrimSpace(*want)
		curBlank := cur == nil || strings.TrimSpace(*cur) == ""
		if !curBlank && (!overwrite || *cur == w) {
			return nil
		}
		changed = true
		return &w
	}
	if patch.FirstName != nil && strings.TrimSpace(*patch.FirstName) != "" && overwrite && p.FirstName != *patch.FirstName {
		in.FirstName = patch.FirstName
		changed = true
	}
	in.LastName = pick(p.LastName, patch.LastName)
	in.Title = pick(p.Title, patch.Title)
	in.Company = pick(p.Company, patch.Company)
	in.Location = pick(p.Location, patch.Location)

	var newEmails, newPhones []string
	haveE, haveP := map[string]bool{}, map[string]bool{}
	for _, e := range p.Emails {
		haveE[dedupe.EmailKey(e.Email)] = true
	}
	for _, ph := range p.Phones {
		haveP[dedupe.PhoneKey(ph.PhoneNumber)] = true
	}
	for _, e := range patch.Emails {
		if k := dedupe.EmailKey(e); k != "" && !haveE[k] {
			haveE[k] = true
			newEmails = append(newEmails, strings.TrimSpace(e))
		}
	}
	for _, ph := range patch.Phones {
		if k := dedupe.PhoneKey(ph); k != "" && !haveP[k] {
			haveP[k] = true
			newPhones = append(newPhones, strings.TrimSpace(ph))
		}
	}
	haveH := map[string]string{}
	for _, h := range p.Handles {
		haveH[h.Kind] = h.Value
	}
	newHandles := map[string]string{}
	for kind, v := range patch.Handles {
		v = strings.TrimSpace(v)
		if v == "" || !model.ValidHandleKind(kind) {
			continue
		}
		cur, has := haveH[kind]
		if has && (!overwrite || dedupe.HandleKey(cur) == dedupe.HandleKey(v)) {
			continue
		}
		newHandles[kind] = v
	}
	if len(newEmails)+len(newPhones)+len(newHandles) > 0 {
		changed = true
	}

	if s.opts.DryRun || !changed {
		return changed, nil
	}
	if in != (model.UpdatePersonInput{}) {
		if _, err := s.people.Update(ctx, id, in); err != nil {
			return false, err
		}
	}
	for _, e := range newEmails {
		if err := s.people.AddEmail(ctx, id, e, nil, false); err != nil {
			return false, err
		}
	}
	for _, ph := range newPhones {
		if err := s.people.AddPhone(ctx, id, ph, nil, false); err != nil {
			return false, err
		}
	}
	kinds := make([]string, 0, len(newHandles))
	for k := range newHandles {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		if err := s.people.SetHandle(ctx, id, k, newHandles[k]); err != nil {
			return false, err
		}
	}
	s.idx.add(id, newEmails, newPhones, newHandles)
	return true, nil
}

// --- interactions ------------------------------------------------------------------------

func (s *dbSink) logInteraction(ctx context.Context, source, externalID string, rec InteractionRecord) (Outcome, error) {
	_, err := s.ext.Lookup(ctx, source, entityInteraction, externalID)
	switch {
	case err == nil:
		return Skipped, nil // synced interactions are not rewritten
	case !errors.Is(err, model.ErrNotFound):
		return "", err
	}

	seen := map[int64]bool{}
	var personIDs []int64
	for _, ref := range rec.People {
		id, err := s.resolveRef(ctx, source, ref)
		if err != nil {
			return "", err
		}
		if id != 0 && !seen[id] {
			seen[id] = true
			personIDs = append(personIDs, id)
		}
	}
	if len(personIDs) == 0 {
		return s.toInbox(ctx, source, externalID, inboxPayload{Kind: "interaction", Reason: "no_person", Record: &rec})
	}
	if s.opts.DryRun {
		return Created, nil
	}
	return s.createInteraction(ctx, source, externalID, rec, personIDs)
}

// createInteraction logs the interaction and maps it. If mapping fails the new row is archived
// so a retry cannot duplicate it.
func (s *dbSink) createInteraction(ctx context.Context, source, externalID string, rec InteractionRecord, personIDs []int64) (Outcome, error) {
	in := model.CreateInteractionInput{Type: rec.Type, Subject: rec.Subject, Content: rec.Content, Direction: rec.Direction, PersonIDs: personIDs}
	if rec.OccurredAt != "" {
		in.OccurredAt = &rec.OccurredAt
	}
	i, err := s.inter.Create(ctx, in)
	if err != nil {
		return "", err
	}
	if _, err := s.ext.Upsert(ctx, repo.ExternalIDInput{Entity: entityInteraction, LocalID: i.ID, LocalUUID: i.UUID,
		Source: source, ExternalID: externalID}); err != nil {
		_ = s.inter.Archive(ctx, i.ID)
		return "", err
	}
	return Created, nil
}

// resolveRef finds the one active person a reference points at, or 0.
func (s *dbSink) resolveRef(ctx context.Context, source string, ref PersonRef) (int64, error) {
	if ref.ExternalID != "" {
		link, err := s.ext.Lookup(ctx, source, entityPerson, ref.ExternalID)
		if err == nil {
			id, active, err := s.resolveLinked(ctx, link.LocalID)
			if err != nil {
				return 0, err
			}
			if active {
				return id, nil
			}
			return 0, nil
		}
		if !errors.Is(err, model.ErrNotFound) {
			return 0, err
		}
	}
	cands, err := s.idx.candidates(ctx, ref.Emails, ref.Phones, ref.Handles)
	if err != nil {
		return 0, err
	}
	if len(cands) == 1 && cands[0] > 0 {
		return cands[0], nil
	}
	return 0, nil
}

func etagPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func strDeref(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func nonEmpty(p *string) *string {
	if p == nil || strings.TrimSpace(*p) == "" {
		return nil
	}
	v := strings.TrimSpace(*p)
	return &v
}

// --- identity matching -------------------------------------------------------------------

// matchIndex maps normalized emails, phones and handles to active people. It loads on first
// use and stays in memory for the run.
type matchIndex struct {
	db     *sql.DB
	loaded bool
	keys   map[string]map[int64]bool
}

func (m *matchIndex) add(id int64, emails, phones []string, handles map[string]string) {
	if m.keys == nil {
		m.keys = map[string]map[int64]bool{}
	}
	put := func(k string) {
		if k == "" {
			return
		}
		if m.keys[k] == nil {
			m.keys[k] = map[int64]bool{}
		}
		m.keys[k][id] = true
	}
	for _, e := range emails {
		if k := dedupe.EmailKey(e); k != "" {
			put("e:" + k)
		}
	}
	for _, p := range phones {
		if k := dedupe.PhoneKey(p); k != "" {
			put("p:" + k)
		}
	}
	for kind, v := range handles {
		if k := dedupe.HandleKey(v); k != "" {
			put("h:" + kind + ":" + k)
		}
	}
}

func (m *matchIndex) load(ctx context.Context) error {
	if m.loaded {
		return nil
	}
	m.keys = map[string]map[int64]bool{}
	queries := []struct {
		q    string
		kind string
	}{
		{`SELECT p.id, p.email FROM people p WHERE p.archived = 0 AND p.email IS NOT NULL`, "e"},
		{`SELECT pe.person_id, pe.email FROM person_emails pe JOIN people p ON p.id = pe.person_id WHERE p.archived = 0`, "e"},
		{`SELECT p.id, p.phone FROM people p WHERE p.archived = 0 AND p.phone IS NOT NULL`, "p"},
		{`SELECT pp.person_id, pp.phone_number FROM person_phones pp JOIN people p ON p.id = pp.person_id WHERE p.archived = 0`, "p"},
		{`SELECT ph.person_id, ph.kind || '|' || ph.value FROM person_handles ph JOIN people p ON p.id = ph.person_id WHERE p.archived = 0`, "h"},
	}
	for _, q := range queries {
		rows, err := m.db.QueryContext(ctx, q.q)
		if err != nil {
			return fmt.Errorf("load match index: %w", err)
		}
		for rows.Next() {
			var id int64
			var v string
			if err := rows.Scan(&id, &v); err != nil {
				rows.Close()
				return fmt.Errorf("load match index: %w", err)
			}
			switch q.kind {
			case "e":
				m.add(id, []string{v}, nil, nil)
			case "p":
				m.add(id, nil, []string{v}, nil)
			default:
				kind, val, _ := strings.Cut(v, "|")
				m.add(id, nil, nil, map[string]string{kind: val})
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("load match index: %w", err)
		}
		rows.Close()
	}
	m.loaded = true
	return nil
}

// candidates returns the distinct people any of the identities point at, sorted.
func (m *matchIndex) candidates(ctx context.Context, emails, phones []string, handles map[string]string) ([]int64, error) {
	if err := m.load(ctx); err != nil {
		return nil, err
	}
	found := map[int64]bool{}
	hit := func(k string) {
		for id := range m.keys[k] {
			found[id] = true
		}
	}
	for _, e := range emails {
		if k := dedupe.EmailKey(e); k != "" {
			hit("e:" + k)
		}
	}
	for _, p := range phones {
		if k := dedupe.PhoneKey(p); k != "" {
			hit("p:" + k)
		}
	}
	for kind, v := range handles {
		if k := dedupe.HandleKey(v); k != "" {
			hit("h:" + kind + ":" + k)
		}
	}
	out := make([]int64, 0, len(found))
	for id := range found {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}
