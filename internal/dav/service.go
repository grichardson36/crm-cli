package dav

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/dedupe"
	"github.com/jdanielnd/crm-cli/internal/model"
	crmsync "github.com/jdanielnd/crm-cli/internal/sync"
)

// Errors the HTTP layer maps to status codes.
var (
	ErrPrecondition = errors.New("precondition failed")
	ErrReadOnly     = errors.New("server is read-only")
)

const timeLayout = "2006-01-02 15:04:05"

// Service is the CardDAV server's logic, free of HTTP. All writes are serialized.
type Service struct {
	DB       *sql.DB
	People   *repo.PersonRepo
	Dav      *repo.DavRepo
	Ext      *repo.ExternalIDRepo
	ReadOnly bool
	// Now is the clock. A device change counts as made at the time of the request.
	Now func() time.Time

	mu sync.Mutex
}

// NewService builds a Service on an open database.
func NewService(db *sql.DB, readOnly bool) *Service {
	return &Service{
		DB: db, People: repo.NewPersonRepo(db), Dav: repo.NewDavRepo(db), Ext: repo.NewExternalIDRepo(db),
		ReadOnly: readOnly, Now: time.Now,
	}
}

func (s *Service) now() string { return s.Now().UTC().Format(timeLayout) }

// ETag is a cheap hash of the person's update time and the mapping version. It does not need the
// vCard to be rendered, so listing thousands of contacts stays fast.
func ETag(personID int64, updatedAt string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%s", MapVersion, personID, updatedAt)))
	return hex.EncodeToString(sum[:8])
}

// CTag changes whenever a contact is added, edited, archived, merged away or restored.
func (s *Service) CTag(ctx context.Context) (string, error) {
	count, idSum, timeSum, err := s.Dav.Collection(ctx)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d|%d", MapVersion, count, idSum, timeSum)))
	return hex.EncodeToString(sum[:8]), nil
}

// Object is one contact as the phone sees it.
type Object struct {
	DavID   string
	Card    vcard.Card
	ETag    string
	ModTime time.Time
}

// stubCard is returned by listings, where no vCard is rendered.
var stubCard = vcard.Card{vcard.FieldVersion: {{Value: "3.0"}}}

// List returns every visible contact with its ETag and no card data.
func (s *Service) List(ctx context.Context) ([]Object, error) {
	rows, err := s.Dav.ListVisible(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Object, len(rows))
	for i, r := range rows {
		out[i] = Object{DavID: r.DavID, Card: stubCard, ETag: ETag(r.PersonID, r.UpdatedAt), ModTime: parseTime(r.UpdatedAt)}
	}
	return out, nil
}

// visible resolves a device id to an active person. Archived and merged people count as gone.
func (s *Service) visible(ctx context.Context, davID string) (*repo.DavTarget, error) {
	t, err := s.Dav.Resolve(ctx, davID)
	if err != nil {
		return nil, err
	}
	if t.Archived || t.MergedInto != 0 {
		return nil, fmt.Errorf("dav contact %s is archived or merged: %w", davID, model.ErrNotFound)
	}
	return t, nil
}

// loadPerson reads a person as the phone sees them. The vCard ORG is the person's primary
// organization when there is one (E4), so Company is replaced by its name for rendering. The
// stored people.company text is used when the person has no primary membership, and TITLE stays
// people.title.
func (s *Service) loadPerson(ctx context.Context, id int64) (*model.Person, error) {
	p, err := s.People.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.People.LoadDetails(ctx, p); err != nil {
		return nil, err
	}
	if org, err := repo.PrimaryOrgName(ctx, s.DB, id); err != nil {
		return nil, err
	} else if org != "" {
		p.Company = &org
	}
	return p, nil
}

// Get renders one contact. With serving set the card is about to reach the phone, so what the
// phone will hold becomes the new merge base.
func (s *Service) Get(ctx context.Context, davID string, serving bool) (*Object, error) {
	if serving {
		s.mu.Lock()
		defer s.mu.Unlock()
	}
	t, err := s.visible(ctx, davID)
	if err != nil {
		return nil, err
	}
	p, err := s.loadPerson(ctx, t.PersonID)
	if err != nil {
		return nil, err
	}
	st, err := s.Dav.GetState(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	var extra vcard.Card
	if st != nil {
		extra = DecodeExtra(st.ExtraVCard)
	}
	card := Render(p, extra, davID)
	etag := ETag(p.ID, p.UpdatedAt)
	if serving && (st == nil || st.BaseJSON == nil || st.ETag == nil || *st.ETag != etag) {
		view, _ := Parse(card)
		base := view.MarshalBase()
		next := model.DavState{PersonID: p.ID, BaseJSON: &base, ETag: &etag}
		if st != nil {
			next.ExtraVCard = st.ExtraVCard
		}
		if err := s.Dav.SaveState(ctx, next); err != nil {
			return nil, err
		}
	}
	return &Object{DavID: davID, Card: card, ETag: etag, ModTime: parseTime(p.UpdatedAt)}, nil
}

// All renders every visible contact. It backs addressbook-query, which phones rarely use.
func (s *Service) All(ctx context.Context, serving bool) ([]Object, error) {
	rows, err := s.Dav.ListVisible(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Object, 0, len(rows))
	for _, r := range rows {
		o, err := s.Get(ctx, r.DavID, serving)
		if errors.Is(err, model.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, nil
}

// PutResult is the outcome of a PUT.
type PutResult struct {
	Created bool
	// ETag is the new version when the phone's card is exactly what the server now holds. It is
	// empty when the server's version differs (the CRM kept its own values for some fields), so
	// the phone fetches the merged card.
	ETag string
}

func (s *Service) checkPreconditions(ifMatch, ifNoneMatch webdav.ConditionalMatch, curETag string, known *string, exists bool) error {
	if ifNoneMatch.IsSet() {
		if !exists {
			// nothing to protect
		} else if ifNoneMatch.IsWildcard() {
			return fmt.Errorf("contact already exists: %w", ErrPrecondition)
		} else if ok, _ := ifNoneMatch.MatchETag(curETag); ok {
			return fmt.Errorf("etag matches the current version: %w", ErrPrecondition)
		}
	}
	if ifMatch.IsSet() {
		if !exists {
			return fmt.Errorf("contact does not exist: %w", ErrPrecondition)
		}
		if ifMatch.IsWildcard() {
			return nil
		}
		tag, err := ifMatch.ETag()
		if err != nil {
			return fmt.Errorf("bad If-Match: %w", model.ErrValidation)
		}
		// The phone may hold the version it last fetched, which is older than the current one
		// when the CRM changed meanwhile. That edit is merged, not refused.
		if tag != curETag && (known == nil || tag != *known) {
			return fmt.Errorf("etag does not match: %w", ErrPrecondition)
		}
	}
	return nil
}

// Put stores a card from the phone.
func (s *Service) Put(ctx context.Context, davID string, card vcard.Card, ifMatch, ifNoneMatch webdav.ConditionalMatch) (*PutResult, error) {
	if s.ReadOnly {
		return nil, ErrReadOnly
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	contact, extra := Parse(card)
	t, err := s.Dav.Resolve(ctx, davID)
	switch {
	case errors.Is(err, model.ErrNotFound):
		if err := s.checkPreconditions(ifMatch, ifNoneMatch, "", nil, false); err != nil {
			return nil, err
		}
		return s.create(ctx, davID, contact, extra)
	case err != nil:
		return nil, err
	}
	if t.Archived || t.MergedInto != 0 {
		return nil, fmt.Errorf("contact was archived or merged in the CRM: %w", ErrPrecondition)
	}
	p, err := s.loadPerson(ctx, t.PersonID)
	if err != nil {
		return nil, err
	}
	st, err := s.Dav.GetState(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	var known *string
	if st != nil {
		known = st.ETag
	}
	if err := s.checkPreconditions(ifMatch, ifNoneMatch, ETag(p.ID, p.UpdatedAt), known, true); err != nil {
		return nil, err
	}
	res, err := s.mergeInto(ctx, p, st, davID, contact, extra, modeUpdate)
	if err != nil {
		return nil, err
	}
	s.markPut(ctx)
	return res, nil
}

func (s *Service) markPut(ctx context.Context) {
	_ = s.Dav.SetMeta(ctx, repo.MetaDavLastPut, s.now())
}

// Delete archives the person. It never purges.
func (s *Service) Delete(ctx context.Context, davID string, ifMatch webdav.ConditionalMatch) error {
	if s.ReadOnly {
		return ErrReadOnly
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.visible(ctx, davID)
	if err != nil {
		return err
	}
	p, err := s.People.FindByID(ctx, t.PersonID)
	if err != nil {
		return err
	}
	st, err := s.Dav.GetState(ctx, p.ID)
	if err != nil {
		return err
	}
	var known *string
	if st != nil {
		known = st.ETag
	}
	if err := s.checkPreconditions(ifMatch, "", ETag(p.ID, p.UpdatedAt), known, true); err != nil {
		return err
	}
	if err := s.People.Archive(ctx, p.ID); err != nil {
		return err
	}
	s.markPut(ctx)
	return nil
}

// --- create ------------------------------------------------------------------------------

func (s *Service) create(ctx context.Context, davID string, contact *Contact, extra vcard.Card) (*PutResult, error) {
	matchID, err := s.findMatch(ctx, contact)
	if err != nil {
		return nil, err
	}
	if matchID != 0 {
		if _, lerr := s.Ext.LookupByLocal(ctx, repo.DavSource, "person", matchID); errors.Is(lerr, model.ErrNotFound) {
			return s.link(ctx, davID, matchID, contact, extra)
		} else if lerr != nil {
			return nil, lerr
		}
		// Already tied to another phone contact: this one is a separate card.
	}

	in := model.CreatePersonInput{FirstName: contact.First, LastName: nilIfEmpty(contact.Last),
		Title: nilIfEmpty(contact.Title), Company: nilIfEmpty(contact.Company),
		Location: nilIfEmpty(contact.Location), Notes: nilIfEmpty(contact.Notes)}
	if in.FirstName == "" {
		in.FirstName = "Unknown"
	}
	if len(contact.Emails) > 0 {
		in.Email = &contact.Emails[0].Value
	}
	if len(contact.Phones) > 0 {
		in.Phone = &contact.Phones[0].Value
	}
	p, err := s.People.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	if _, err := repo.NewOrgAutoRepo(s.DB).ApplyCompany(ctx, p.ID, contact.Company, repo.ModeRepoint); err != nil {
		_ = s.People.Archive(ctx, p.ID)
		return nil, err
	}
	if _, err := s.Ext.Upsert(ctx, repo.ExternalIDInput{Entity: "person", LocalID: p.ID, LocalUUID: p.UUID,
		Source: repo.DavSource, ExternalID: davID}); err != nil {
		_ = s.People.Archive(ctx, p.ID)
		return nil, err
	}
	p, err = s.loadPerson(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	res, err := s.mergeInto(ctx, p, nil, davID, contact, extra, modeUpdate)
	if err != nil {
		return nil, err
	}
	res.Created = true
	s.markPut(ctx)
	return res, nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// link attaches a new phone contact to an existing person and logs it.
func (s *Service) link(ctx context.Context, davID string, personID int64, contact *Contact, extra vcard.Card) (*PutResult, error) {
	p, err := s.loadPerson(ctx, personID)
	if err != nil {
		return nil, err
	}
	if _, err := s.Ext.Upsert(ctx, repo.ExternalIDInput{Entity: "person", LocalID: p.ID, LocalUUID: p.UUID,
		Source: repo.DavSource, ExternalID: davID}); err != nil {
		return nil, err
	}
	crmName := strings.TrimSpace(p.FirstName + " " + nonNil(p.LastName))
	devName := strings.TrimSpace(contact.First + " " + contact.Last)
	if _, err := s.Dav.AddConflict(ctx, p.ID, fieldLink, &crmName, &devName, model.ConflictWinnerLink); err != nil {
		return nil, err
	}
	st, err := s.Dav.GetState(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	res, err := s.mergeInto(ctx, p, st, davID, contact, extra, modeLink)
	if err != nil {
		return nil, err
	}
	res.Created = true
	s.markPut(ctx)
	return res, nil
}

// findMatch returns the one active person a new phone contact is the same as, or 0. A match is
// an exact email, phone or social handle held by exactly one person (the websites do not count),
// or, with no identity match, a normalized full name held by exactly one person.
func (s *Service) findMatch(ctx context.Context, c *Contact) (int64, error) {
	var emails, phones []string
	for _, e := range c.Emails {
		emails = append(emails, e.Value)
	}
	for _, p := range c.Phones {
		phones = append(phones, p.Value)
	}
	handles := map[string]string{}
	for k, v := range c.Handles {
		if k != "website" {
			handles[k] = v
		}
	}
	cands, err := crmsync.FindCandidates(ctx, s.DB, emails, phones, handles)
	if err != nil {
		return 0, err
	}
	if len(cands) == 1 {
		return cands[0], nil
	}
	nf, nl := dedupe.NormalizeName(c.First, c.Last)
	if nf == "" || nl == "" {
		return 0, nil
	}
	nf = dedupe.Canonical(nf)
	if len(cands) > 0 {
		return s.nameAmong(ctx, cands, nf, nl)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, first_name, COALESCE(last_name, '') FROM people WHERE archived = 0 AND merged_into IS NULL`)
	if err != nil {
		return 0, fmt.Errorf("match by name: %w", err)
	}
	var found []int64
	for rows.Next() {
		var id int64
		var first, last string
		if err := rows.Scan(&id, &first, &last); err != nil {
			rows.Close()
			return 0, fmt.Errorf("match by name: %w", err)
		}
		if f, l := dedupe.NormalizeName(first, last); l == nl && dedupe.Canonical(f) == nf {
			found = append(found, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(found) == 1 {
		return found[0], nil
	}
	return 0, nil
}

// nameAmong picks the one candidate whose name equals the normalized name, or 0.
func (s *Service) nameAmong(ctx context.Context, ids []int64, nf, nl string) (int64, error) {
	var found []int64
	for _, id := range ids {
		var first, last string
		err := s.DB.QueryRowContext(ctx, `SELECT first_name, COALESCE(last_name, '') FROM people WHERE id = ?`, id).Scan(&first, &last)
		if err != nil {
			return 0, fmt.Errorf("match by name: %w", err)
		}
		if f, l := dedupe.NormalizeName(first, last); l == nl && dedupe.Canonical(f) == nf {
			found = append(found, id)
		}
	}
	if len(found) == 1 {
		return found[0], nil
	}
	return 0, nil
}

// applyDeviceOrg applies an ORG value the phone changed (owner decision 2026-10-27, replaces the
// org-review queue). It returns true when the caller must leave people.company alone, false when the
// caller stores the phone's text there (people.company stays the source text).
//   - A text that names an organization (normalized name, or the name of a merged-away one): the
//     person's primary membership moves to it in place, role and dates stay. A person with no
//     primary membership gets one.
//   - A text that matches no organization creates it and does the same.
//   - A junk text (the ignore list: N/A, none, retired ...) creates nothing and changes no membership.
//   - EMPTYING ORG NEVER REMOVES A MEMBERSHIP. The phone clearing the field only means "no company
//     text there", the organization link is the CRM's own data. The primary membership stays, so the
//     next GET shows the organization name again.
func (s *Service) applyDeviceOrg(ctx context.Context, personID int64, value string) (bool, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		var n int
		err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM membership WHERE person_id = ? AND is_primary = 1 AND deleted_at IS NULL`, personID).Scan(&n)
		if err != nil {
			return false, fmt.Errorf("find primary membership: %w", err)
		}
		return n > 0, nil
	}
	if _, err := repo.NewOrgAutoRepo(s.DB).ApplyCompany(ctx, personID, name, repo.ModeRepoint); err != nil {
		return false, err
	}
	return false, nil
}

// --- merge -------------------------------------------------------------------------------

func rawScalars(p *model.Person) map[string]string {
	return map[string]string{
		fieldFirst: strings.TrimSpace(p.FirstName), fieldLast: nonNil(p.LastName), fieldCompany: nonNil(p.Company),
		fieldTitle: nonNil(p.Title), fieldNotes: nonNil(p.Notes), fieldLocation: nonNil(p.Location),
		fieldBirthday: personBirthday(p),
	}
}

// mergeInto folds the phone's contact into a person and records the new device state.
func (s *Service) mergeInto(ctx context.Context, p *model.Person, st *model.DavState, davID string, dev *Contact, extra vcard.Card, mode mergeMode) (*PutResult, error) {
	var oldExtra vcard.Card
	var base *Contact
	if st != nil {
		oldExtra = DecodeExtra(st.ExtraVCard)
		base = ParseBase(st.BaseJSON)
	} else {
		base = &Contact{}
	}
	crmView, _ := Parse(Render(p, oldExtra, davID))
	raw := rawScalars(p)
	now := s.now()

	changedAt := func(field string) string {
		at, err := repo.FieldChangedAt(ctx, s.DB, "person", p.ID, field)
		if err != nil {
			return ""
		}
		if field == fieldCompany {
			// The CRM's ORG is the primary membership, so changing it counts as changing the field.
			var mt sql.NullString
			_ = s.DB.QueryRowContext(ctx, `SELECT MAX(updated_at) FROM membership WHERE person_id = ? AND is_primary = 1 AND deleted_at IS NULL`, p.ID).Scan(&mt)
			if mt.Valid && mt.String > at {
				at = mt.String
			}
		}
		return at
	}

	// Scalars. ORG is the primary organization (E4). An ORG edit on the phone is applied by
	// applyDeviceOrg: it moves the primary membership to the organization of that name, creating the
	// organization when none matches. Emptying ORG never removes a membership. When both sides
	// changed ORG since the last sync the usual last-write-wins conflict path decides.
	decisions := mergeScalars(base.scalars(), crmView.scalars(), dev.scalars(), changedAt, now, mode)
	var in model.UpdatePersonInput
	for _, d := range decisions {
		if d.Take {
			v := d.Device
			switch d.Field {
			case fieldFirst:
				in.FirstName = &v
			case fieldLast:
				in.LastName = &v
			case fieldCompany:
				handled, err := s.applyDeviceOrg(ctx, p.ID, v)
				if err != nil {
					return nil, err
				}
				if !handled {
					in.Company = &v
				}
			case fieldTitle:
				in.Title = &v
			case fieldNotes:
				in.Notes = &v
			case fieldLocation:
				in.Location = &v
			case fieldBirthday:
				in.Birthday = &v
			}
		}
	}
	if in != (model.UpdatePersonInput{}) {
		if _, err := s.People.Update(ctx, p.ID, in); err != nil {
			return nil, err
		}
	}
	for _, d := range decisions {
		if !d.Conflict {
			continue
		}
		crmRaw, devV := raw[d.Field], d.Device
		if _, err := s.Dav.AddConflict(ctx, p.ID, d.Field, &crmRaw, &devV, d.Winner); err != nil {
			return nil, err
		}
	}

	// Emails and phones.
	ep := mergeItems(base.Emails, crmView.Emails, dev.Emails, emailKey)
	rawEmail := map[string]string{}
	for _, e := range p.Emails {
		rawEmail[emailKey(e.Email)] = e.Email
	}
	for _, k := range ep.RemoveKeys {
		if v, ok := rawEmail[k]; ok {
			if err := s.People.RemoveEmail(ctx, p.ID, v); err != nil && !errors.Is(err, model.ErrNotFound) {
				return nil, err
			}
		}
	}
	for _, it := range append(append([]Item{}, ep.Add...), ep.Relabel...) {
		if err := s.People.AddEmail(ctx, p.ID, it.Value, nilIfEmpty(it.Label), false); err != nil {
			return nil, err
		}
	}
	pp := mergeItems(base.Phones, crmView.Phones, dev.Phones, phoneKey)
	rawPhone := map[string]string{}
	for _, ph := range p.Phones {
		rawPhone[phoneKey(ph.PhoneNumber)] = ph.PhoneNumber
	}
	for _, k := range pp.RemoveKeys {
		if v, ok := rawPhone[k]; ok {
			if err := s.People.RemovePhone(ctx, p.ID, v); err != nil && !errors.Is(err, model.ErrNotFound) {
				return nil, err
			}
		}
	}
	for _, it := range append(append([]Item{}, pp.Add...), pp.Relabel...) {
		if err := s.People.AddPhone(ctx, p.ID, it.Value, nilIfEmpty(it.Label), false); err != nil {
			return nil, err
		}
	}

	// Handles and the website.
	for _, h := range mergeHandles(base.Handles, crmView.Handles, dev.Handles, mode) {
		if h.Conflict {
			crmV, devV := h.CRM, h.Device
			if _, err := s.Dav.AddConflict(ctx, p.ID, handlePrefix+h.Kind, &crmV, &devV, model.ConflictWinnerDevice); err != nil {
				return nil, err
			}
		}
		if h.Remove {
			if err := s.People.RemoveHandle(ctx, p.ID, h.Kind); err != nil && !errors.Is(err, model.ErrNotFound) {
				return nil, err
			}
			continue
		}
		if err := s.People.SetHandle(ctx, p.ID, h.Kind, h.Value); err != nil {
			return nil, err
		}
	}

	// Record what the phone now holds.
	after, err := s.loadPerson(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	afterView, _ := Parse(Render(after, extra, davID))
	etag := ETag(after.ID, after.UpdatedAt)
	same := afterView.Equal(dev)
	stored := ""
	if same {
		stored = etag
	}
	baseJSON := dev.MarshalBase()
	next := model.DavState{PersonID: p.ID, BaseJSON: &baseJSON, ETag: &stored}
	if enc := EncodeExtra(extra); enc != "" {
		next.ExtraVCard = &enc
	}
	if err := s.Dav.SaveState(ctx, next); err != nil {
		return nil, err
	}
	return &PutResult{ETag: stored}, nil
}
