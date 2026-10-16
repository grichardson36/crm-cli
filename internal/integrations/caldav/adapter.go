package caldav

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/ics"
	"github.com/jdanielnd/crm-cli/internal/cal/icsio"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	crmsync "github.com/jdanielnd/crm-cli/internal/sync"
)

// Source is the name of this adapter in external_ids, cursors and the run log.
const Source = "caldav"

// scalar fields compared three ways in a conflict.
var scalarFields = []string{"title", "notes", "location", "when", "rrule", "status", "busy"}

// Adapter is the CalDAV sync adapter.
type Adapter struct {
	Cfg  Config
	HTTP *http.Client
	// Now is the clock. Past meetings are those that ended before it. Default time.Now.
	Now func() time.Time
	// Loc is the zone for times a server sends without one. Default: this device.
	Loc *time.Location
}

// Name implements sync.Adapter.
func (a *Adapter) Name() string { return Source }

func (a *Adapter) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

type run struct {
	a    *Adapter
	cl   *Client
	host crmsync.Host
	sink crmsync.Sink
	db   *sql.DB
	er   *repo.EventRepo
	ext  *repo.ExternalIDRepo
	res  *repo.CaldavRepo
	loc  *time.Location
	dry  bool
}

// Pull implements sync.Adapter: it pulls the server's changes, and with Config.TwoWay sends the
// local ones. The cursor is only the time of the last good run.
func (a *Adapter) Pull(ctx context.Context, since string, sink crmsync.Sink) (string, error) {
	host, ok := sink.(crmsync.Host)
	if !ok {
		return since, fmt.Errorf("the caldav adapter needs the built-in sink")
	}
	cl, err := NewClient(a.Cfg, a.HTTP)
	if err != nil {
		return since, err
	}
	loc := a.Loc
	if loc == nil {
		loc = tz.Device()
	}
	db := host.DB()
	r := &run{a: a, cl: cl, host: host, sink: sink, db: db, er: repo.NewEventRepo(db), ext: repo.NewExternalIDRepo(db), res: repo.NewCaldavRepo(db), loc: loc, dry: host.IsDryRun()}

	objs, err := cl.List(ctx)
	if err != nil {
		return since, err
	}
	known, err := r.res.List(ctx, Source)
	if err != nil {
		return since, err
	}
	if len(objs) == 0 && len(known) > 3 {
		return since, fmt.Errorf("the server returned an empty calendar but %d events were synced before: not treating that as a mass delete", len(known))
	}
	byHref := map[string]repo.CaldavResource{}
	for _, k := range known {
		byHref[k.Href] = k
	}
	seen := map[string]bool{}
	for _, o := range objs {
		seen[o.Href] = true
		if err := ctx.Err(); err != nil {
			return since, err
		}
		if err := r.pullOne(ctx, o, byHref[o.Href], byHref[o.Href].Href != ""); err != nil {
			host.Fail(fmt.Errorf("%s: %w", o.Href, err))
		}
	}
	for _, k := range known {
		if !seen[k.Href] {
			if err := r.remoteDeleted(ctx, k); err != nil {
				host.Fail(fmt.Errorf("%s: %w", k.Href, err))
			}
		}
	}
	if a.Cfg.TwoWay {
		r.push(ctx)
	}
	return a.now().UTC().Format(time.RFC3339), nil
}

// localID returns the local event of a UID and whether it is archived. 0 means there is none.
func (r *run) localID(ctx context.Context, uid string) (id int64, archived bool, err error) {
	if l, lerr := r.ext.Lookup(ctx, Source, "event", uid); lerr == nil {
		exists, arch, serr := repo.EntityState(ctx, r.db, "event", l.LocalID)
		if serr != nil {
			return 0, false, serr
		}
		if exists {
			return l.LocalID, arch, nil
		}
	} else if !errors.Is(lerr, model.ErrNotFound) {
		return 0, false, lerr
	}
	if e, ferr := r.er.FindByUUID(ctx, uid); ferr == nil {
		return e.ID, false, nil
	}
	return 0, false, nil
}

func (r *run) snapshot(ctx context.Context, id int64) (hash, js string, err error) {
	hash, err = icsio.Fingerprint(ctx, r.db, Source, id)
	if err != nil {
		return "", "", err
	}
	e, err := r.er.FindByID(ctx, id)
	if err != nil {
		return "", "", err
	}
	b, err := json.Marshal(repo.EventFieldValues(e))
	return hash, string(b), err
}

// remember records that both sides now agree on the event.
func (r *run) remember(ctx context.Context, href, uid, etag string, id int64) error {
	hash, js, err := r.snapshot(ctx, id)
	if err != nil {
		return err
	}
	return r.res.Save(ctx, repo.CaldavResource{Source: Source, Href: href, UID: uid, ETag: etag, BaseHash: hash, BaseJSON: js})
}

func firstUID(cal *ics.Calendar) (string, *ics.Event) {
	for _, e := range cal.Events {
		if e.RecurrenceID == nil {
			return e.UID, e
		}
	}
	if len(cal.Events) > 0 {
		return cal.Events[0].UID, cal.Events[0]
	}
	return "", nil
}

func (r *run) importObject(ctx context.Context, o Object) (*icsio.ImportResult, error) {
	return icsio.Import(ctx, r.db, o.Data, icsio.ImportOptions{Source: Source, Loc: r.loc, SkipTasks: true})
}

func (r *run) pullOne(ctx context.Context, o Object, rec repo.CaldavResource, known bool) error {
	cal, err := ics.Decode(o.Data, r.loc)
	if err != nil {
		return err
	}
	uid, master := firstUID(cal)
	if uid == "" {
		r.host.Count(crmsync.Skipped)
		return nil
	}
	if !known {
		if _, archived, err := r.localID(ctx, uid); err == nil && archived {
			r.host.Count(crmsync.Skipped) // deleted here: not brought back
			return nil
		} else if err != nil {
			return err
		}
		if r.dry {
			r.host.Count(crmsync.Created)
			return nil
		}
		res, err := r.importObject(ctx, o)
		if err != nil {
			return err
		}
		id, _, err := r.localID(ctx, uid)
		if err != nil || id == 0 {
			r.host.Count(crmsync.Skipped)
			return err
		}
		if err := r.remember(ctx, o.Href, uid, o.ETag, id); err != nil {
			return err
		}
		if res.Events.Created > 0 {
			r.host.Count(crmsync.Created)
		} else {
			r.host.Count(crmsync.Updated)
		}
		r.logMeeting(ctx, master)
		return nil
	}
	if rec.ETag == o.ETag {
		r.host.Count(crmsync.Skipped)
		return nil
	}

	id, archived, err := r.localID(ctx, uid)
	if err != nil {
		return err
	}
	if id == 0 || archived {
		r.host.Count(crmsync.Skipped) // deleted here: a two-way run deletes it on the server
		return nil
	}
	if r.dry {
		r.host.Count(crmsync.Updated)
		return nil
	}
	nowHash, err := icsio.Fingerprint(ctx, r.db, Source, id)
	if err != nil {
		return err
	}
	if nowHash == rec.BaseHash {
		if _, err := r.importObject(ctx, o); err != nil {
			return err
		}
		if err := r.remember(ctx, o.Href, uid, o.ETag, id); err != nil {
			return err
		}
		r.host.Count(crmsync.Updated)
		r.logMeeting(ctx, master)
		return nil
	}
	return r.merge(ctx, o, rec, id, uid, master)
}

// merge folds a changed server copy into an event that also changed here. Each tracked field is
// decided on its own against the state both sides last agreed on: the side that changed it wins,
// and when both changed it differently the later change wins (local time from field_versions,
// server time from LAST-MODIFIED). Sets (people, reminders, tags, exceptions) and per-instance
// changes follow the server, which this run has just read.
func (r *run) merge(ctx context.Context, o Object, rec repo.CaldavResource, id int64, uid string, master *ics.Event) error {
	local, err := r.er.FindByID(ctx, id)
	if err != nil {
		return err
	}
	before := repo.EventFieldValues(local)
	changedAt := map[string]string{}
	for _, f := range scalarFields {
		at, err := repo.FieldChangedAt(ctx, r.db, "event", id, f)
		if err != nil {
			return err
		}
		changedAt[f] = at
	}
	base := map[string]string{}
	_ = json.Unmarshal([]byte(rec.BaseJSON), &base)

	if _, err := r.importObject(ctx, o); err != nil {
		return err
	}
	remote, err := r.er.FindByID(ctx, id)
	if err != nil {
		return err
	}
	after := repo.EventFieldValues(remote)
	remoteAt := r.a.now().UTC().Format("2006-01-02 15:04:05")
	if master != nil && !master.Modified.IsZero() {
		remoteAt = master.Modified.UTC().Format("2006-01-02 15:04:05")
	}
	var keep []string
	for _, f := range scalarFields {
		b, l, s := base[f], before[f], after[f]
		switch {
		case l == b || l == s:
			// local did not change it, or both agree: the server copy stands.
		case s == b:
			keep = append(keep, f)
		case changedAt[f] > remoteAt:
			keep = append(keep, f)
		}
	}
	if len(keep) > 0 {
		in := restoreInput(local, remote, keep)
		if _, err := r.er.Update(ctx, id, in, model.ScopeAll, ""); err != nil {
			return err
		}
	}
	r.host.Note(fmt.Sprintf("merged %q (kept local: %s)", local.Title, strings.Join(keep, ", ")))
	if len(keep) == 0 {
		if err := r.remember(ctx, o.Href, uid, o.ETag, id); err != nil {
			return err
		}
	} else {
		// The server is behind the merged event. The base stays what the server last held, with a
		// hash that never matches, so a two-way run sends the merged event.
		js, err := json.Marshal(after)
		if err != nil {
			return err
		}
		if err := r.res.Save(ctx, repo.CaldavResource{Source: Source, Href: o.Href, UID: uid, ETag: o.ETag, BaseHash: "", BaseJSON: string(js)}); err != nil {
			return err
		}
	}
	r.host.Count(crmsync.Updated)
	return nil
}

// restoreInput builds the update that puts the kept local fields back on the event.
func restoreInput(local, current *model.Event, keep []string) model.UpdateEventInput {
	var in model.UpdateEventInput
	for _, f := range keep {
		switch f {
		case "title":
			in.Title = &local.Title
		case "notes":
			v := derefStr(local.Notes)
			in.Notes = &v
		case "location":
			v := derefStr(local.Location)
			in.Location = &v
		case "rrule":
			v := derefStr(local.RRule)
			in.RRule = &v
		case "status":
			in.Status = &local.Status
		case "busy":
			in.Busy = &local.Busy
		case "when":
			if local.AllDay != current.AllDay {
				in.AllDay = &local.AllDay
			}
			if s, e, ok := span(local); ok {
				in.Start, in.End = &s, &e
			}
			if local.TZ != nil && !local.AllDay {
				in.TZ = local.TZ
			}
		}
	}
	return in
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func span(e *model.Event) (s, en time.Time, ok bool) {
	var err1, err2 error
	if e.AllDay {
		s, err1 = tz.ParseDate(derefStr(e.StartDate))
		en, err2 = tz.ParseDate(derefStr(e.EndDate))
	} else {
		s, err1 = tz.ParseInstant(derefStr(e.StartUTC))
		en, err2 = tz.ParseInstant(derefStr(e.EndUTC))
	}
	return s, en, err1 == nil && err2 == nil
}

// remoteDeleted handles an event that is gone from the server.
func (r *run) remoteDeleted(ctx context.Context, k repo.CaldavResource) error {
	id, archived, err := r.localID(ctx, k.UID)
	if err != nil {
		return err
	}
	if r.dry {
		r.host.Count(crmsync.Skipped)
		return nil
	}
	if id == 0 || archived {
		return r.res.Delete(ctx, Source, k.Href)
	}
	hash, err := icsio.Fingerprint(ctx, r.db, Source, id)
	if err != nil {
		return err
	}
	if hash != k.BaseHash && r.a.Cfg.TwoWay {
		// Changed here after the server lost it: keep it and send it again.
		return r.res.Delete(ctx, Source, k.Href)
	}
	if err := r.er.Archive(ctx, id, model.ScopeAll, ""); err != nil {
		return err
	}
	r.host.Note("archived an event the server deleted")
	r.host.Count(crmsync.Updated)
	return r.res.Delete(ctx, Source, k.Href)
}

// logMeeting writes one meeting interaction for a past, single event with attendees. The sink
// matches attendees to people by email and queues the unmatched in the inbox.
func (r *run) logMeeting(ctx context.Context, e *ics.Event) {
	if e == nil || r.dry || e.RRule != "" || e.RecurrenceID != nil || e.AllDay || !e.End.Before(r.a.now()) || e.Status == "CANCELLED" {
		return
	}
	self := map[string]bool{}
	for _, s := range r.a.Cfg.SelfEmails {
		self[s] = true
	}
	var refs []crmsync.PersonRef
	add := func(email string) {
		if email != "" && !self[email] {
			refs = append(refs, crmsync.PersonRef{Emails: []string{email}})
		}
	}
	if e.Organizer != nil {
		add(e.Organizer.Email)
	}
	for _, a := range e.Attendees {
		if e.Organizer == nil || a.Email != e.Organizer.Email {
			add(a.Email)
		}
	}
	if len(refs) == 0 {
		return
	}
	title := e.Summary
	rec := crmsync.InteractionRecord{Type: "meeting", Subject: &title, OccurredAt: e.Start.UTC().Format("2006-01-02 15:04:05"), People: refs}
	_, _ = r.sink.LogInteraction(ctx, Source, e.UID+"@"+tz.FormatInstant(e.Start), rec)
}

// push sends local changes: new events, edits and deletions.
func (r *run) push(ctx context.Context) {
	ids, err := r.er.SeriesIDs(ctx)
	if err != nil {
		r.host.Fail(err)
		return
	}
	for _, id := range ids {
		if err := r.pushOne(ctx, id); err != nil {
			r.host.Fail(err)
		}
	}
	known, err := r.res.List(ctx, Source)
	if err != nil {
		r.host.Fail(err)
		return
	}
	for _, k := range known {
		id, archived, err := r.localID(ctx, k.UID)
		if err != nil {
			r.host.Fail(err)
			continue
		}
		if id == 0 || !archived {
			continue
		}
		if r.dry {
			r.host.Count(crmsync.Updated)
			continue
		}
		if err := r.cl.Delete(ctx, k.Href, k.ETag); err != nil {
			r.host.Fail(err)
			continue
		}
		if err := r.res.Delete(ctx, Source, k.Href); err != nil {
			r.host.Fail(err)
			continue
		}
		r.host.Count(crmsync.Updated)
	}
}

func (r *run) pushOne(ctx context.Context, id int64) error {
	data, uid, err := icsio.EventICS(ctx, r.db, Source, id)
	if err != nil {
		return err
	}
	rec, err := r.res.ByUID(ctx, Source, uid)
	switch {
	case errors.Is(err, model.ErrNotFound):
		if r.dry {
			r.host.Count(crmsync.Created)
			return nil
		}
		href := r.cl.HrefFor(uid)
		etag, err := r.cl.Put(ctx, href, data, "")
		if err != nil {
			return err
		}
		if _, err := r.ext.Upsert(ctx, repo.ExternalIDInput{Entity: "event", LocalID: id, Source: Source, ExternalID: uid}); err != nil {
			return err
		}
		if err := r.remember(ctx, href, uid, etag, id); err != nil {
			return err
		}
		r.host.Count(crmsync.Created)
		return nil
	case err != nil:
		return err
	}
	hash, err := icsio.Fingerprint(ctx, r.db, Source, id)
	if err != nil {
		return err
	}
	if hash == rec.BaseHash {
		return nil
	}
	if r.dry {
		r.host.Count(crmsync.Updated)
		return nil
	}
	etag, err := r.cl.Put(ctx, rec.Href, data, rec.ETag)
	if err != nil {
		return err
	}
	if err := r.remember(ctx, rec.Href, uid, etag, id); err != nil {
		return err
	}
	r.host.Count(crmsync.Updated)
	return nil
}
