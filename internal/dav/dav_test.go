package dav_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/carddav"
	"github.com/jdanielnd/crm-cli/internal/dav"
	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testUser = "grant"
	testPass = "s3cret-pass"
)

var bookPath = dav.BookPath(testUser)

type env struct {
	t      *testing.T
	db     *sql.DB
	people *repo.PersonRepo
	davRep *repo.DavRepo
	svc    *dav.Service
	srv    *httptest.Server
	client *carddav.Client
	clock  time.Time
}

func newEnv(t *testing.T, readOnly bool) *env {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	e := &env{t: t, db: d, people: repo.NewPersonRepo(d), davRep: repo.NewDavRepo(d), clock: time.Now()}
	e.svc = dav.NewService(d, readOnly)
	e.svc.Now = func() time.Time { return e.clock }
	h := dav.NewHandler(dav.Options{Service: e.svc, User: testUser, Password: testPass, BookName: "CRM test"})
	e.srv = httptest.NewServer(h)
	t.Cleanup(e.srv.Close)
	c, err := carddav.NewClient(webdav.HTTPClientWithBasicAuth(nil, testUser, testPass), e.srv.URL+"/dav/")
	require.NoError(t, err)
	e.client = c
	return e
}

func ptr[T any](v T) *T { return &v }

func (e *env) person(first, last, email string) *model.Person {
	e.t.Helper()
	in := model.CreatePersonInput{FirstName: first}
	if last != "" {
		in.LastName = &last
	}
	if email != "" {
		in.Email = &email
	}
	p, err := e.people.Create(context.Background(), in)
	require.NoError(e.t, err)
	return p
}

// bump moves updated_at forward a second so the ETag changes inside one test second.
func (e *env) bump(id int64) {
	e.t.Helper()
	_, err := e.db.Exec(`UPDATE people SET updated_at = datetime(updated_at, '+1 second') WHERE id = ?`, id)
	require.NoError(e.t, err)
}

func (e *env) reload(id int64) *model.Person {
	e.t.Helper()
	p, err := e.people.FindByID(context.Background(), id)
	require.NoError(e.t, err)
	require.NoError(e.t, e.people.LoadDetails(context.Background(), p))
	return p
}

func (e *env) davID(p *model.Person) string {
	id, err := e.davRep.DavIDFor(context.Background(), p.ID)
	require.NoError(e.t, err)
	return id
}

// pull fetches the card like the phone does, which also records the merge base.
func (e *env) pull(p *model.Person) (vcard.Card, string) {
	e.t.Helper()
	o, err := e.svc.Get(context.Background(), e.davID(p), true)
	require.NoError(e.t, err)
	return o.Card, o.ETag
}

func (e *env) push(davID string, card vcard.Card, ifMatch string) (*dav.PutResult, error) {
	e.t.Helper()
	return e.svc.Put(context.Background(), davID, card, webdav.ConditionalMatch(ifMatch), "")
}

func etagHeader(etag string) string { return `"` + etag + `"` }

func parseCard(t *testing.T, text string) vcard.Card {
	t.Helper()
	c, err := vcard.NewDecoder(strings.NewReader(strings.ReplaceAll(text, "\n", "\r\n"))).Decode()
	require.NoError(t, err)
	return c
}

func (e *env) conflicts() []model.SyncConflict {
	e.t.Helper()
	cs, err := e.davRep.Conflicts(context.Background(), true, 0)
	require.NoError(e.t, err)
	return cs
}

func (e *env) do(method, path string, body string, hdr map[string]string, auth bool) *http.Response {
	e.t.Helper()
	req, err := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	require.NoError(e.t, err)
	if auth {
		req.SetBasicAuth(testUser, testPass)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	cl := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := cl.Do(req)
	require.NoError(e.t, err)
	e.t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// listETags does what DAVx5 does to find changes: PROPFIND depth 1 asking for the ETags.
func (e *env) listETags() map[string]string {
	e.t.Helper()
	resp := e.do("PROPFIND", bookPath, `<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:getetag/></d:prop></d:propfind>`,
		map[string]string{"Depth": "1", "Content-Type": "application/xml"}, true)
	require.Equal(e.t, http.StatusMultiStatus, resp.StatusCode)
	var ms struct {
		Responses []struct {
			Href string `xml:"href"`
			Prop struct {
				ETag string `xml:"getetag"`
			} `xml:"propstat>prop"`
		} `xml:"response"`
	}
	require.NoError(e.t, xml.NewDecoder(resp.Body).Decode(&ms))
	out := map[string]string{}
	for _, r := range ms.Responses {
		if strings.HasSuffix(r.Href, ".vcf") {
			out[r.Href] = r.Prop.ETag
		}
	}
	return out
}

const vcardHdr = "text/vcard; charset=utf-8"

func bodyOf(t *testing.T, r *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	return string(b)
}

// --- protocol ----------------------------------------------------------------------------

func TestDiscoveryListMultigetAndGet(t *testing.T) {
	e := newEnv(t, false)
	ctx := context.Background()
	jane := e.person("Jane", "Smith", "jane@example.com")
	bob := e.person("Bob", "Jones", "bob@example.com")
	gone := e.person("Gone", "Away", "gone@example.com")
	merged := e.person("Dup", "Licate", "dup@example.com")
	require.NoError(t, e.people.Archive(ctx, gone.ID))
	_, err := e.db.Exec(`UPDATE people SET archived = 1, merged_into = ? WHERE id = ?`, jane.ID, merged.ID)
	require.NoError(t, err)

	t.Run("well-known redirects to the principal", func(t *testing.T) {
		resp := e.do("GET", "/.well-known/carddav", "", nil, false)
		assert.Equal(t, http.StatusPermanentRedirect, resp.StatusCode)
		assert.Equal(t, "/dav/", resp.Header.Get("Location"))
	})

	t.Run("principal, home set, address book", func(t *testing.T) {
		principal, err := e.client.FindCurrentUserPrincipal(ctx)
		require.NoError(t, err)
		assert.Equal(t, "/dav/", principal)
		home, err := e.client.FindAddressBookHomeSet(ctx, principal)
		require.NoError(t, err)
		assert.Equal(t, "/dav/"+testUser+"/", home)
		books, err := e.client.FindAddressBooks(ctx, home)
		require.NoError(t, err)
		require.Len(t, books, 1)
		assert.Equal(t, bookPath, books[0].Path)
		assert.Equal(t, "CRM test", books[0].Name)
		require.NoError(t, e.client.HasSupport(ctx))
	})

	t.Run("the book reports a ctag that moves with changes", func(t *testing.T) {
		q := `<?xml version="1.0"?><d:propfind xmlns:d="DAV:" xmlns:cs="http://calendarserver.org/ns/"><d:prop><d:resourcetype/><cs:getctag/><d:current-user-privilege-set/></d:prop></d:propfind>`
		ctag := func() string {
			resp := e.do("PROPFIND", bookPath, q, map[string]string{"Depth": "0", "Content-Type": "application/xml"}, true)
			require.Equal(t, http.StatusMultiStatus, resp.StatusCode)
			body := bodyOf(t, resp)
			assert.Contains(t, body, "addressbook")
			assert.Contains(t, body, "write-content")
			_, rest, ok := strings.Cut(body, "<CS:getctag>")
			require.True(t, ok, body)
			v, _, _ := strings.Cut(rest, "</CS:getctag>")
			return v
		}
		before := ctag()
		assert.Equal(t, before, ctag(), "stable while nothing changes")
		e.bump(bob.ID)
		assert.NotEqual(t, before, ctag())
	})

	t.Run("listing hides archived and merged people", func(t *testing.T) {
		tags := e.listETags()
		var files []string
		for p, etag := range tags {
			assert.NotEmpty(t, etag, p)
			files = append(files, p)
		}
		assert.ElementsMatch(t, []string{bookPath + jane.UUID + ".vcf", bookPath + bob.UUID + ".vcf"}, files)
	})

	t.Run("query returns cards, multiget and GET agree", func(t *testing.T) {
		objs, err := e.client.QueryAddressBook(ctx, bookPath, &carddav.AddressBookQuery{DataRequest: carddav.AddressDataRequest{AllProp: true}})
		require.NoError(t, err)
		require.Len(t, objs, 2)
		for _, o := range objs {
			assert.NotEmpty(t, o.Card.Value(vcard.FieldUID))
			assert.Equal(t, "3.0", o.Card.Value(vcard.FieldVersion))
			assert.NotEmpty(t, o.ETag)
		}
		mg, err := e.client.MultiGetAddressBook(ctx, bookPath, &carddav.AddressBookMultiGet{
			Paths:       []string{bookPath + jane.UUID + ".vcf"},
			DataRequest: carddav.AddressDataRequest{AllProp: true}})
		require.NoError(t, err)
		require.Len(t, mg, 1)
		raw := e.do("REPORT", bookPath, `<?xml version="1.0"?><c:addressbook-multiget xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:carddav"><d:prop><d:getetag/><c:address-data/></d:prop><d:href>`+
			bookPath+gone.UUID+`.vcf</d:href></c:addressbook-multiget>`, map[string]string{"Depth": "1", "Content-Type": "application/xml"}, true)
		rawBody := bodyOf(t, raw)
		assert.Contains(t, rawBody, "404", "the archived contact is reported gone so the phone deletes it")
		assert.NotContains(t, rawBody, "BEGIN:VCARD")
		assert.Equal(t, jane.UUID, mg[0].Card.Value(vcard.FieldUID))
		one, err := e.client.GetAddressObject(ctx, bookPath+jane.UUID+".vcf")
		require.NoError(t, err)
		assert.Equal(t, "Jane Smith", one.Card.Value(vcard.FieldFormattedName))
		assert.Equal(t, mg[0].ETag, one.ETag)

		_, err = e.client.GetAddressObject(ctx, bookPath+gone.UUID+".vcf")
		assert.Error(t, err)
		_, err = e.client.GetAddressObject(ctx, bookPath+merged.UUID+".vcf")
		assert.Error(t, err)
	})

	t.Run("serving a card records the merge base", func(t *testing.T) {
		st, err := e.davRep.GetState(ctx, jane.ID)
		require.NoError(t, err)
		require.NotNil(t, st)
		assert.NotNil(t, st.BaseJSON)
		assert.NotNil(t, st.ETag)
	})
}

func TestAuthentication(t *testing.T) {
	e := newEnv(t, false)
	assert.Equal(t, http.StatusUnauthorized, e.do("PROPFIND", "/dav/", "", map[string]string{"Depth": "0"}, false).StatusCode)
	req, _ := http.NewRequest("PROPFIND", e.srv.URL+"/dav/", nil)
	req.SetBasicAuth(testUser, "wrong")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("WWW-Authenticate"), "Basic")

	_, err = carddav.NewClient(webdav.HTTPClientWithBasicAuth(nil, testUser, "wrong"), e.srv.URL+"/dav/")
	require.NoError(t, err)
	bad, _ := carddav.NewClient(webdav.HTTPClientWithBasicAuth(nil, testUser, "wrong"), e.srv.URL+"/dav/")
	_, err = bad.FindCurrentUserPrincipal(context.Background())
	assert.Error(t, err)
}

func TestPutCreateUpdateDeleteOverHTTP(t *testing.T) {
	e := newEnv(t, false)
	ctx := context.Background()
	card := `BEGIN:VCARD
VERSION:3.0
UID:phone-1
N:Quinn;Ada;;;
FN:Ada Quinn
ORG:Lovelace Ltd
TITLE:Analyst
EMAIL;TYPE=INTERNET;TYPE=WORK:ada@lovelace.example
TEL;TYPE=CELL:+1 555 0100
BDAY:1815-12-10
URL:https://ada.example
END:VCARD
`
	// create
	path := bookPath + "phone-1.vcf"
	resp := e.do("PUT", path, strings.ReplaceAll(card, "\n", "\r\n"), map[string]string{"Content-Type": vcardHdr, "If-None-Match": "*"}, true)
	require.Equal(t, http.StatusCreated, resp.StatusCode, bodyOf(t, resp))
	etag := resp.Header.Get("ETag")
	assert.NotEmpty(t, etag, "the server holds exactly what was sent")

	res, err := e.people.Search(ctx, "Quinn", 5)
	require.NoError(t, err)
	require.Len(t, res, 1)
	p := e.reload(res[0].ID)
	assert.Equal(t, "Ada", p.FirstName)
	assert.Equal(t, "Lovelace Ltd", *p.Company)
	org, _ := primaryOf(t, e, p.ID)
	created, err := repo.NewOrgRepo(e.db).FindByID(ctx, org)
	require.NoError(t, err, "a contact created from the phone creates its organization (X19)")
	assert.Equal(t, "Lovelace Ltd", created.Name)
	assert.Equal(t, "Analyst", *p.Title)
	require.Len(t, p.Emails, 1)
	assert.Equal(t, "Work", *p.Emails[0].Label)
	require.Len(t, p.Phones, 1)
	assert.Equal(t, "Mobile", *p.Phones[0].Label)
	assert.Equal(t, 1815, *p.BirthdayYear)
	assert.Equal(t, "website", p.Handles[0].Kind)
	ext, err := repo.NewExternalIDRepo(e.db).Lookup(ctx, "dav", "person", "phone-1")
	require.NoError(t, err)
	assert.Equal(t, p.ID, ext.LocalID)

	// the phone's id is the path, and a second create at the same id is refused
	resp = e.do("PUT", path, strings.ReplaceAll(card, "\n", "\r\n"), map[string]string{"Content-Type": vcardHdr, "If-None-Match": "*"}, true)
	assert.Equal(t, http.StatusPreconditionFailed, resp.StatusCode)

	// GET returns the card under the phone's id
	got, err := e.client.GetAddressObject(ctx, path)
	require.NoError(t, err)
	assert.Equal(t, "phone-1", got.Card.Value(vcard.FieldUID))

	// update with the right If-Match
	e.clock = e.clock.Add(time.Minute)
	upd := strings.ReplaceAll(strings.Replace(card, "Analyst", "Director", 1), "\n", "\r\n")
	resp = e.do("PUT", path, upd, map[string]string{"Content-Type": vcardHdr, "If-Match": etag}, true)
	require.Equal(t, http.StatusCreated, resp.StatusCode, bodyOf(t, resp))
	assert.Equal(t, "Director", *e.reload(p.ID).Title)

	// a wrong If-Match is refused
	resp = e.do("PUT", path, upd, map[string]string{"Content-Type": vcardHdr, "If-Match": `"nope"`}, true)
	assert.Equal(t, http.StatusPreconditionFailed, resp.StatusCode)

	// If-Match on something that does not exist
	resp = e.do("PUT", bookPath+"ghost.vcf", upd, map[string]string{"Content-Type": vcardHdr, "If-Match": `"x"`}, true)
	assert.Equal(t, http.StatusPreconditionFailed, resp.StatusCode)

	// delete with a stale tag is refused, with the current tag it archives
	resp = e.do("DELETE", path, "", map[string]string{"If-Match": `"stale"`}, true)
	assert.Equal(t, http.StatusPreconditionFailed, resp.StatusCode)
	cur, err := e.client.GetAddressObject(ctx, path)
	require.NoError(t, err)
	resp = e.do("DELETE", path, "", map[string]string{"If-Match": etagHeader(cur.ETag)}, true)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	_, err = e.people.FindByID(ctx, p.ID)
	assert.ErrorIs(t, err, model.ErrNotFound, "archived, not purged")
	var archived int
	require.NoError(t, e.db.QueryRow(`SELECT archived FROM people WHERE id = ?`, p.ID).Scan(&archived))
	assert.Equal(t, 1, archived)
	_, err = e.client.GetAddressObject(ctx, path)
	assert.Error(t, err)
	assert.Equal(t, http.StatusNotFound, e.do("DELETE", path, "", nil, true).StatusCode)

	// a stale phone edit of an archived contact does not bring it back
	resp = e.do("PUT", path, upd, map[string]string{"Content-Type": vcardHdr}, true)
	assert.Equal(t, http.StatusPreconditionFailed, resp.StatusCode)
}

func TestReadOnlyRejectsWrites(t *testing.T) {
	e := newEnv(t, true)
	p := e.person("Jane", "Smith", "jane@example.com")
	path := bookPath + p.UUID + ".vcf"
	card := "BEGIN:VCARD\r\nVERSION:3.0\r\nN:Smith;Jane;;;\r\nFN:Jane Smith\r\nEND:VCARD\r\n"
	assert.Equal(t, http.StatusForbidden, e.do("PUT", path, card, map[string]string{"Content-Type": vcardHdr}, true).StatusCode)
	assert.Equal(t, http.StatusForbidden, e.do("DELETE", path, "", nil, true).StatusCode)
	assert.Equal(t, http.StatusForbidden, e.do("PUT", bookPath+"new.vcf", card, map[string]string{"Content-Type": vcardHdr}, true).StatusCode)
	_, err := e.client.GetAddressObject(context.Background(), path)
	assert.NoError(t, err, "reads still work")
	resp := e.do("PROPFIND", bookPath, `<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:current-user-privilege-set/></d:prop></d:propfind>`,
		map[string]string{"Depth": "0", "Content-Type": "application/xml"}, true)
	body := bodyOf(t, resp)
	assert.Contains(t, body, "read")
	assert.NotContains(t, body, "write-content", "the phone is told the book is read-only")
	count, err := e.people.FindAll(context.Background(), model.PersonFilters{})
	require.NoError(t, err)
	assert.Len(t, count, 1)
}

func TestBookCannotBeDeleted(t *testing.T) {
	e := newEnv(t, false)
	assert.Equal(t, http.StatusForbidden, e.do("DELETE", bookPath, "", nil, true).StatusCode)
	assert.Equal(t, http.StatusForbidden, e.do("MKCOL", "/dav/"+testUser+"/other/", "", nil, true).StatusCode)
}

// --- ETags -------------------------------------------------------------------------------

func TestETagFollowsUpdatedAtAndIfMatchAcceptsTheKnownVersion(t *testing.T) {
	e := newEnv(t, false)
	ctx := context.Background()
	p := e.person("Jane", "Smith", "jane@example.com")
	card, etag1 := e.pull(p)
	assert.Equal(t, dav.ETag(p.ID, e.reload(p.ID).UpdatedAt), etag1)

	// the CRM edits the person: the ETag moves
	_, err := e.people.Update(ctx, p.ID, model.UpdatePersonInput{Company: ptr("Acme")})
	require.NoError(t, err)
	e.bump(p.ID)
	_, etag2 := func() (vcard.Card, string) {
		o, err := e.svc.Get(ctx, p.UUID, false)
		require.NoError(t, err)
		return o.Card, o.ETag
	}()
	assert.NotEqual(t, etag1, etag2)

	// the phone still holds version 1 and edits the title: its If-Match is the old tag and it is accepted and merged
	card.Set(vcard.FieldTitle, &vcard.Field{Value: "CTO"})
	res, err := e.push(p.UUID, card, etagHeader(etag1))
	require.NoError(t, err)
	assert.Empty(t, res.ETag, "the CRM kept its company, so the phone must refetch")
	got := e.reload(p.ID)
	assert.Equal(t, "CTO", *got.Title)
	assert.Equal(t, "Acme", *got.Company)

	// an unknown tag is refused
	_, err = e.push(p.UUID, card, `"unknown"`)
	assert.ErrorIs(t, err, dav.ErrPrecondition)
}

// --- mapping -----------------------------------------------------------------------------

func TestRenderMapsFields(t *testing.T) {
	e := newEnv(t, false)
	ctx := context.Background()
	p, err := e.people.Create(ctx, model.CreatePersonInput{FirstName: "Jane", LastName: ptr("Smith"),
		Email: ptr("jane@example.com"), Phone: ptr("+1 555 0100"), Title: ptr("CEO"), Company: ptr("Acme; Inc"),
		Location: ptr("12 Main St; Springfield, IL"), Notes: ptr("line one\r\nline two")})
	require.NoError(t, err)
	require.NoError(t, e.people.AddEmail(ctx, p.ID, "jane@work.example", ptr("Work"), false))
	require.NoError(t, e.people.AddPhone(ctx, p.ID, "+1 555 0200", ptr("Gym"), false))
	require.NoError(t, e.people.SetHandle(ctx, p.ID, "website", "https://jane.example"))
	require.NoError(t, e.people.SetHandle(ctx, p.ID, "twitter", "https://twitter.com/jane"))
	require.NoError(t, e.people.SetHandle(ctx, p.ID, "telegram", "@jane"))
	_, err = e.people.Update(ctx, p.ID, model.UpdatePersonInput{Birthday: ptr("--03-14"), PriorityTier: ptr(1), KeepInTouch: ptr("14 days")})
	require.NoError(t, err)

	card, _ := e.pull(p)
	var buf bytes.Buffer
	require.NoError(t, vcard.NewEncoder(&buf).Encode(card))
	text := buf.String()
	for _, want := range []string{
		"VERSION:3.0", "UID:" + p.UUID, "N:Smith;Jane;;;", "FN:Jane Smith", "ORG:Acme\\, Inc", "TITLE:CEO",
		"NOTE:line one\\nline two", "BDAY:--03-14", "EMAIL;TYPE=INTERNET;TYPE=PREF:jane@example.com",
		"EMAIL;TYPE=INTERNET;TYPE=WORK:jane@work.example", "ADR;TYPE=HOME:;;12 Main St\\, Springfield\\, IL;;;;",
		"URL:https://jane.example", "X-SOCIALPROFILE;TYPE=twitter:https://twitter.com/jane", "X-SOCIALPROFILE;TYPE=telegram:@jane",
		"REV:",
	} {
		assert.Contains(t, text, want)
	}
	assert.Contains(t, text, "TEL;TYPE=PREF:+1 555 0100")
	assert.Regexp(t, `crm\d\.TEL:\+1 555 0200`, text, "a custom label travels as a grouped X-ABLabel")
	assert.Regexp(t, `crm\d\.X-ABLABEL:Gym`, text)
	assert.Regexp(t, `crm\d\.URL:https://twitter.com/jane`, text, "URL fallback for socials")
	for _, absent := range []string{"priority", "KEEP", "CATEGORIES"} {
		assert.NotContains(t, strings.ToUpper(text), strings.ToUpper(absent), "CRM-only data stays home")
	}

	// the rendered card parses back to the same contact
	c1, extra := dav.Parse(card)
	assert.Nil(t, extra)
	assert.Equal(t, "Jane", c1.First)
	assert.Equal(t, "Acme, Inc", c1.Company, "semicolons inside structured values become commas")
	assert.Equal(t, "line one\nline two", c1.Notes)
	assert.Equal(t, "12 Main St, Springfield, IL", c1.Location)
	assert.Equal(t, "--03-14", c1.Birthday)
	assert.Equal(t, "https://jane.example", c1.Handles["website"])
	assert.Equal(t, "@jane", c1.Handles["telegram"])
	require.Len(t, c1.Phones, 2)
	assert.Equal(t, "Gym", c1.Phones[1].Label)
	c2, _ := dav.Parse(card)
	assert.True(t, c1.Equal(c2))
}

// A phone that pushes back exactly what it was given must change nothing, whatever the data
// looks like. Otherwise every sync would drift the CRM.
func TestUnchangedRoundTripChangesNothing(t *testing.T) {
	e := newEnv(t, false)
	ctx := context.Background()
	p, err := e.people.Create(ctx, model.CreatePersonInput{FirstName: "Jane  Q", LastName: ptr("O'Smith-Jones"),
		Email: ptr("jane@example.com"), Phone: ptr("(555) 010-0100"), Title: ptr("CEO; Founder"), Company: ptr("Acme; Inc"),
		Location: ptr("12 Main St; Apt 4\nSpringfield, IL"), Notes: ptr("  line one\r\nline two  \n")})
	require.NoError(t, err)
	require.NoError(t, e.people.AddEmail(ctx, p.ID, "J@Work.example", ptr("Weird Label!"), false))
	require.NoError(t, e.people.AddPhone(ctx, p.ID, "+44 20 7946 0958", ptr("Mobile"), false))
	require.NoError(t, e.people.SetHandle(ctx, p.ID, "website", "https://jane.example/a;b"))
	require.NoError(t, e.people.SetHandle(ctx, p.ID, "linkedin", "https://linkedin.com/in/jane"))
	_, err = e.people.Update(ctx, p.ID, model.UpdatePersonInput{Birthday: ptr("1985-03-14")})
	require.NoError(t, err)

	before := e.reload(p.ID)
	card, etag := e.pull(before)
	// Push it through the wire format, as a real phone would.
	var buf bytes.Buffer
	require.NoError(t, vcard.NewEncoder(&buf).Encode(card))
	wire, err := vcard.NewDecoder(&buf).Decode()
	require.NoError(t, err)

	res, err := e.push(p.UUID, wire, etagHeader(etag))
	require.NoError(t, err)
	assert.Equal(t, etag, res.ETag, "nothing changed, the version is the same")
	assert.Empty(t, e.conflicts())
	after := e.reload(p.ID)
	assert.Equal(t, before.UpdatedAt, after.UpdatedAt, "no write happened")
	assert.Equal(t, before.FirstName, after.FirstName)
	assert.Equal(t, *before.Company, *after.Company)
	assert.Equal(t, *before.Location, *after.Location)
	assert.Equal(t, *before.Notes, *after.Notes)
	assert.Equal(t, before.Emails, after.Emails)
	assert.Equal(t, before.Phones, after.Phones)
}

func TestParseVariants(t *testing.T) {
	t.Run("iOS style card", func(t *testing.T) {
		c, extra := dav.Parse(parseCard(t, `BEGIN:VCARD
VERSION:3.0
N:Doe;John;Q.;Dr.;Jr.
FN:Dr. John Q. Doe Jr.
ORG:Acme;Sales
BDAY;X-APPLE-OMIT-YEAR=1604:1604-07-04
TEL;type=IPHONE;type=CELL;type=VOICE;type=pref:(555) 123-4567
EMAIL;type=INTERNET;type=HOME:j@doe.example
item1.URL:https://doe.example
item1.X-ABLabel:_$!<HomePage>!$_
item2.ADR;type=HOME:;;1 Elm;Town;ST;12345;USA
PHOTO;ENCODING=b;TYPE=JPEG:AAAA
NICKNAME:Johnny
X-CUSTOM:keepme
END:VCARD
`))
		assert.Equal(t, "John", c.First)
		assert.Equal(t, "Doe", c.Last)
		assert.Equal(t, "Acme", c.Company)
		assert.Equal(t, "--07-04", c.Birthday)
		assert.Equal(t, "Mobile", c.Phones[0].Label)
		assert.Equal(t, "Home", c.Emails[0].Label)
		assert.Equal(t, "1 Elm, Town, ST, 12345, USA", c.Location)
		assert.Equal(t, "https://doe.example", c.Handles["website"])
		for _, k := range []string{"PHOTO", "NICKNAME", "X-CUSTOM", "N", "ORG"} {
			assert.NotEmpty(t, extra[k], k+" is kept")
		}
	})
	t.Run("name only in FN, company only, nameless", func(t *testing.T) {
		c, _ := dav.Parse(parseCard(t, "BEGIN:VCARD\nVERSION:3.0\nFN:Madonna Louise Ciccone\nEND:VCARD\n"))
		assert.Equal(t, "Madonna Louise", c.First)
		assert.Equal(t, "Ciccone", c.Last)
		c, _ = dav.Parse(parseCard(t, "BEGIN:VCARD\nVERSION:3.0\nN:;;;;\nORG:Pizza Place\nEND:VCARD\n"))
		assert.Equal(t, "Pizza Place", c.First)
		c, _ = dav.Parse(parseCard(t, "BEGIN:VCARD\nVERSION:3.0\nTEL:555 0100 999\nEND:VCARD\n"))
		assert.Equal(t, "555 0100 999", c.First)
		c, _ = dav.Parse(parseCard(t, "BEGIN:VCARD\nVERSION:3.0\nN:Solo;;;;\nEND:VCARD\n"))
		assert.Equal(t, "Solo", c.First)
		assert.Equal(t, "", c.Last)
	})
	t.Run("birthday formats", func(t *testing.T) {
		for in, want := range map[string]string{
			"1985-03-14": "1985-03-14", "19850314": "1985-03-14", "--03-14": "--03-14", "--0314": "--03-14",
			"1985-03-14T00:00:00Z": "1985-03-14", "1604-03-14": "--03-14", "nonsense": "", "1985-13-40": "",
		} {
			c, _ := dav.Parse(parseCard(t, "BEGIN:VCARD\nVERSION:3.0\nN:A;B;;;\nBDAY:"+in+"\nEND:VCARD\n"))
			assert.Equal(t, want, c.Birthday, in)
		}
	})
	t.Run("social profiles", func(t *testing.T) {
		c, _ := dav.Parse(parseCard(t, `BEGIN:VCARD
VERSION:3.0
N:A;B;;;
X-SOCIALPROFILE;type=twitter:https://twitter.com/b
X-SOCIALPROFILE;type=linkedin;x-user=bee:https://linkedin.com/in/bee
item3.URL:https://facebook.com/b
item3.X-ABLabel:Facebook
END:VCARD
`))
		assert.Equal(t, "https://twitter.com/b", c.Handles["twitter"])
		assert.Equal(t, "https://linkedin.com/in/bee", c.Handles["linkedin"])
		assert.Equal(t, "https://facebook.com/b", c.Handles["facebook"])
		assert.NotContains(t, c.Handles, "website")
	})
}

// --- merge matrix ------------------------------------------------------------------------

func TestMergeMatrix(t *testing.T) {
	ctx := context.Background()
	setup := func(t *testing.T) (*env, *model.Person, vcard.Card, string) {
		e := newEnv(t, false)
		p, err := e.people.Create(ctx, model.CreatePersonInput{FirstName: "Jane", LastName: ptr("Smith"),
			Email: ptr("jane@example.com"), Phone: ptr("+1 555 0100"), Title: ptr("Engineer"), Company: ptr("Acme")})
		require.NoError(t, err)
		card, etag := e.pull(p)
		return e, p, card, etag
	}
	set := func(card vcard.Card, key, value string) { card.Set(key, &vcard.Field{Value: value}) }

	t.Run("device-only change is taken without a conflict", func(t *testing.T) {
		e, p, card, etag := setup(t)
		set(card, vcard.FieldTitle, "CTO")
		res, err := e.push(p.UUID, card, etagHeader(etag))
		require.NoError(t, err)
		assert.NotEmpty(t, res.ETag, "nothing differs, the phone keeps its copy")
		assert.Equal(t, "CTO", *e.reload(p.ID).Title)
		assert.Empty(t, e.conflicts())
		field, err := repo.FieldChangedAt(ctx, e.db, "person", p.ID, "title")
		require.NoError(t, err)
		assert.NotEmpty(t, field, "applied through the repo, so field_versions is written")
	})

	t.Run("CRM-only change is kept and pushed on the next GET", func(t *testing.T) {
		e, p, card, etag := setup(t)
		_, err := e.people.Update(ctx, p.ID, model.UpdatePersonInput{Company: ptr("Beta")})
		require.NoError(t, err)
		e.bump(p.ID)
		set(card, vcard.FieldTitle, "CTO") // the phone edits another field
		res, err := e.push(p.UUID, card, etagHeader(etag))
		require.NoError(t, err)
		assert.Empty(t, res.ETag)
		got := e.reload(p.ID)
		assert.Equal(t, "Beta", *got.Company)
		assert.Equal(t, "CTO", *got.Title)
		assert.Empty(t, e.conflicts())
		next, _ := e.pull(got)
		assert.Equal(t, "Beta", next.Value(vcard.FieldOrganization))
	})

	t.Run("both changed, the CRM value is kept per field and the phone value is stored", func(t *testing.T) {
		e, p, card, etag := setup(t)
		_, err := e.people.Update(ctx, p.ID, model.UpdatePersonInput{Title: ptr("Manager")})
		require.NoError(t, err)
		e.clock = time.Now().Add(time.Hour)
		set(card, vcard.FieldTitle, "Director")
		_, err = e.push(p.UUID, card, etagHeader(etag))
		require.NoError(t, err)
		assert.Equal(t, "Manager", *e.reload(p.ID).Title)
		cs := e.conflicts()
		require.Len(t, cs, 1)
		assert.Equal(t, "title", cs[0].Field)
		assert.Equal(t, "crm", cs[0].Winner)
		assert.Equal(t, "Manager", *cs[0].CRMValue)
		assert.Equal(t, "Director", *cs[0].DeviceValue)

		// undo applies the stored phone value through the repo
		undone, err := dav.UndoConflict(ctx, e.db, cs[0].ID)
		require.NoError(t, err)
		assert.True(t, undone.Undone)
		assert.Equal(t, "Director", *e.reload(p.ID).Title)
		_, err = dav.UndoConflict(ctx, e.db, cs[0].ID)
		assert.ErrorIs(t, err, model.ErrConflict, "cannot undo twice")
	})

	t.Run("both changed, the CRM is newer, the CRM wins and undo restores the phone value", func(t *testing.T) {
		e, p, card, etag := setup(t)
		_, err := e.people.Update(ctx, p.ID, model.UpdatePersonInput{Title: ptr("Manager")})
		require.NoError(t, err)
		e.clock = time.Now().Add(-time.Hour)
		set(card, vcard.FieldTitle, "Director")
		res, err := e.push(p.UUID, card, etagHeader(etag))
		require.NoError(t, err)
		assert.Empty(t, res.ETag, "the phone must take the CRM value")
		assert.Equal(t, "Manager", *e.reload(p.ID).Title)
		cs := e.conflicts()
		require.Len(t, cs, 1)
		assert.Equal(t, "crm", cs[0].Winner)

		_, err = dav.UndoConflict(ctx, e.db, cs[0].ID)
		require.NoError(t, err)
		assert.Equal(t, "Director", *e.reload(p.ID).Title)
	})

	t.Run("undo refuses when the field changed again", func(t *testing.T) {
		e, p, card, etag := setup(t)
		_, err := e.people.Update(ctx, p.ID, model.UpdatePersonInput{Title: ptr("Manager")})
		require.NoError(t, err)
		e.clock = time.Now().Add(time.Hour)
		set(card, vcard.FieldTitle, "Director")
		_, err = e.push(p.UUID, card, etagHeader(etag))
		require.NoError(t, err)
		_, err = e.people.Update(ctx, p.ID, model.UpdatePersonInput{Title: ptr("VP")})
		require.NoError(t, err)
		_, err = dav.UndoConflict(ctx, e.db, e.conflicts()[0].ID)
		assert.ErrorIs(t, err, model.ErrConflict)
	})

	t.Run("lists merge by set", func(t *testing.T) {
		e, p, card, etag := setup(t)
		require.NoError(t, e.people.AddEmail(ctx, p.ID, "crm-added@example.com", nil, false)) // the CRM adds
		e.bump(p.ID)
		card.Add(vcard.FieldEmail, &vcard.Field{Value: "phone-added@example.com"}) // the phone adds an email
		card.Add(vcard.FieldTelephone, &vcard.Field{Value: "+1 555 0999", Params: vcard.Params{"TYPE": {"WORK"}}})
		_, err := e.push(p.UUID, card, etagHeader(etag))
		require.NoError(t, err)
		got := e.reload(p.ID)
		var emails, phones []string
		for _, x := range got.Emails {
			emails = append(emails, x.Email)
		}
		for _, x := range got.Phones {
			phones = append(phones, x.PhoneNumber)
		}
		assert.ElementsMatch(t, []string{"jane@example.com", "crm-added@example.com", "phone-added@example.com"}, emails)
		assert.ElementsMatch(t, []string{"+1 555 0100", "+1 555 0999"}, phones)
		for _, ph := range got.Phones {
			if ph.PhoneNumber == "+1 555 0999" {
				assert.Equal(t, "Work", *ph.Label)
			}
		}
		assert.Empty(t, e.conflicts(), "lists never conflict")
	})

	t.Run("a phone removal removes only items that were in the base", func(t *testing.T) {
		e, p, _, _ := setup(t)
		require.NoError(t, e.people.AddEmail(ctx, p.ID, "second@example.com", nil, false))
		e.bump(p.ID)
		card, etag := e.pull(e.reload(p.ID)) // the phone now holds both emails
		require.NoError(t, e.people.AddEmail(ctx, p.ID, "crm-new@example.com", nil, false))
		e.bump(p.ID)
		// the phone deletes second@ and keeps the rest
		var kept []*vcard.Field
		for _, f := range card[vcard.FieldEmail] {
			if f.Value != "second@example.com" {
				kept = append(kept, f)
			}
		}
		card[vcard.FieldEmail] = kept
		_, err := e.push(p.UUID, card, etagHeader(etag))
		require.NoError(t, err)
		var emails []string
		for _, x := range e.reload(p.ID).Emails {
			emails = append(emails, x.Email)
		}
		assert.ElementsMatch(t, []string{"jane@example.com", "crm-new@example.com"}, emails, "the CRM's own addition survives")
	})

	t.Run("website and socials", func(t *testing.T) {
		e, p, card, etag := setup(t)
		card.Set(vcard.FieldURL, &vcard.Field{Value: "https://jane.example"})
		_, err := e.push(p.UUID, card, etagHeader(etag))
		require.NoError(t, err)
		got := e.reload(p.ID)
		require.Len(t, got.Handles, 1)
		assert.Equal(t, "https://jane.example", got.Handles[0].Value)
		// the phone clears it again
		card2, etag2 := e.pull(got)
		delete(card2, vcard.FieldURL)
		_, err = e.push(p.UUID, card2, etagHeader(etag2))
		require.NoError(t, err)
		assert.Empty(t, e.reload(p.ID).Handles)
	})

	t.Run("birthday and location travel both ways", func(t *testing.T) {
		e, p, card, etag := setup(t)
		set(card, vcard.FieldBirthday, "1990-02-03")
		card.Set(vcard.FieldAddress, &vcard.Field{Value: ";;5 Oak Rd;Town;ST;999;US"})
		_, err := e.push(p.UUID, card, etagHeader(etag))
		require.NoError(t, err)
		got := e.reload(p.ID)
		assert.Equal(t, 1990, *got.BirthdayYear)
		assert.Equal(t, "5 Oak Rd, Town, ST, 999, US", *got.Location)
	})

	t.Run("unknown properties survive the round trip", func(t *testing.T) {
		e, p, card, etag := setup(t)
		card.Set("PHOTO", &vcard.Field{Value: "AAAABBBB", Params: vcard.Params{"ENCODING": {"b"}, "TYPE": {"JPEG"}}})
		card.Set("NICKNAME", &vcard.Field{Value: "JJ"})
		card.Set("X-CUSTOM", &vcard.Field{Value: "keep me"})
		card.Set(vcard.FieldName, &vcard.Field{Value: "Smith;Jane;Marie;Dr.;PhD"})
		card.Add(vcard.FieldAddress, &vcard.Field{Value: ";;1 First St;;;;"})
		card.Add(vcard.FieldAddress, &vcard.Field{Value: ";;2 Second St;;;;"})
		_, err := e.push(p.UUID, card, etagHeader(etag))
		require.NoError(t, err)
		next, _ := e.pull(e.reload(p.ID))
		assert.Equal(t, "AAAABBBB", next.Value("PHOTO"))
		assert.Equal(t, "JJ", next.Value("NICKNAME"))
		assert.Equal(t, "keep me", next.Value("X-CUSTOM"))
		assert.Equal(t, "Smith;Jane;Marie;Dr.;PhD", next.Value(vcard.FieldName))
		assert.Equal(t, "Jane Smith", next.Value(vcard.FieldFormattedName), "the phone's FN line is kept as it was sent")
		require.Len(t, next[vcard.FieldAddress], 2)
		assert.Equal(t, "Jane", e.reload(p.ID).FirstName, "the CRM columns stay clean")
		// the phone drops one: it is gone
		delete(next, "X-CUSTOM")
		etag2 := ""
		_, etag2 = e.pull(e.reload(p.ID))
		_, err = e.push(p.UUID, next, etagHeader(etag2))
		require.NoError(t, err)
		again, _ := e.pull(e.reload(p.ID))
		assert.Empty(t, again["X-CUSTOM"])
		assert.Equal(t, "AAAABBBB", again.Value("PHOTO"))
	})

	t.Run("the first name is never emptied", func(t *testing.T) {
		e, p, card, etag := setup(t)
		set(card, vcard.FieldName, ";;;;")
		set(card, vcard.FieldFormattedName, "")
		_, err := e.push(p.UUID, card, etagHeader(etag))
		require.NoError(t, err)
		assert.NotEmpty(t, e.reload(p.ID).FirstName)
	})
}

// --- creation and dedupe -----------------------------------------------------------------

func TestCreateLinksToAnExistingPerson(t *testing.T) {
	ctx := context.Background()
	t.Run("same email links instead of creating", func(t *testing.T) {
		e := newEnv(t, false)
		jane := e.person("Jane", "Smith", "jane@example.com")
		card := parseCard(t, `BEGIN:VCARD
VERSION:3.0
N:Smyth;J.;;;
EMAIL:JANE@example.com
TEL:+1 555 0111
ORG:Newco
TITLE:Founder
END:VCARD
`)
		res, err := e.push("device-77", card, "")
		require.NoError(t, err)
		assert.True(t, res.Created)
		people, err := e.people.FindAll(ctx, model.PersonFilters{})
		require.NoError(t, err)
		require.Len(t, people, 1, "no duplicate person")
		got := e.reload(jane.ID)
		assert.Equal(t, "Jane", got.FirstName, "the CRM's name is kept")
		assert.Equal(t, "Founder", *got.Title, "blanks are filled from the phone")
		assert.Equal(t, "Newco", *got.Company)
		assert.Len(t, got.Phones, 1)
		ext, err := repo.NewExternalIDRepo(e.db).Lookup(ctx, "dav", "person", "device-77")
		require.NoError(t, err)
		assert.Equal(t, jane.ID, ext.LocalID)
		assert.Equal(t, "device-77", e.davID(got), "the phone keeps its own path")
		_, err = e.client.GetAddressObject(ctx, bookPath+"device-77.vcf")
		require.NoError(t, err)
		cs := e.conflicts()
		require.Len(t, cs, 1)
		assert.Equal(t, "_link", cs[0].Field)
		assert.Equal(t, "link", cs[0].Winner)
		_, err = dav.UndoConflict(ctx, e.db, cs[0].ID)
		assert.ErrorIs(t, err, model.ErrValidation, "a link has no value to restore")
	})

	t.Run("normalized name links when no identity matches", func(t *testing.T) {
		e := newEnv(t, false)
		e.person("Robert", "Brown", "")
		card := parseCard(t, "BEGIN:VCARD\nVERSION:3.0\nN:Brown;Bob;;;\nTEL:+1 555 0123\nEND:VCARD\n")
		_, err := e.push("device-1", card, "")
		require.NoError(t, err)
		people, _ := e.people.FindAll(ctx, model.PersonFilters{})
		assert.Len(t, people, 1)
	})

	t.Run("a different person is created", func(t *testing.T) {
		e := newEnv(t, false)
		e.person("Jane", "Smith", "jane@example.com")
		card := parseCard(t, "BEGIN:VCARD\nVERSION:3.0\nN:Doe;John;;;\nEMAIL:john@example.com\nEND:VCARD\n")
		_, err := e.push("device-2", card, "")
		require.NoError(t, err)
		people, _ := e.people.FindAll(ctx, model.PersonFilters{})
		assert.Len(t, people, 2)
		assert.Empty(t, e.conflicts())
	})

	t.Run("a uid equal to a person uuid is that person", func(t *testing.T) {
		e := newEnv(t, false)
		jane := e.person("Jane", "Smith", "jane@example.com")
		card := parseCard(t, "BEGIN:VCARD\nVERSION:3.0\nN:Smith;Jane;;;\nTITLE:Boss\nEND:VCARD\n")
		res, err := e.push(jane.UUID, card, "")
		require.NoError(t, err)
		assert.False(t, res.Created)
		assert.Equal(t, "Boss", *e.reload(jane.ID).Title)
		people, _ := e.people.FindAll(ctx, model.PersonFilters{})
		assert.Len(t, people, 1)
	})
}

func TestMergedAwayPersonVanishesFromTheCollection(t *testing.T) {
	e := newEnv(t, false)
	ctx := context.Background()
	a := e.person("Ann", "Lee", "ann@example.com")
	b := e.person("Ann", "Leigh", "ann2@example.com")
	e.pull(a)
	e.pull(b)
	_, err := e.people.Merge(ctx, a.ID, []int64{b.ID}, nil)
	require.NoError(t, err)
	objs, err := e.svc.List(ctx)
	require.NoError(t, err)
	require.Len(t, objs, 1)
	assert.Equal(t, a.UUID, objs[0].DavID)
	_, err = e.svc.Get(ctx, b.UUID, false)
	assert.ErrorIs(t, err, model.ErrNotFound)
}

// --- performance -------------------------------------------------------------------------

func TestListingFiveThousandContactsIsFast(t *testing.T) {
	e := newEnv(t, false)
	ctx := context.Background()
	tx, err := e.db.BeginTx(ctx, nil)
	require.NoError(t, err)
	for i := 0; i < 5000; i++ {
		_, err := tx.ExecContext(ctx, `INSERT INTO people (uuid, first_name, last_name, email) VALUES (?, ?, ?, ?)`,
			fmt.Sprintf("00000000-0000-4000-8000-%012d", i), fmt.Sprintf("First%d", i), fmt.Sprintf("Last%d", i), fmt.Sprintf("p%d@example.com", i))
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit())

	start := time.Now()
	objs, err := e.svc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, objs, 5000)
	assert.Less(t, time.Since(start), time.Second, "service listing")

	start = time.Now()
	tags := e.listETags()
	elapsed := time.Since(start)
	assert.Len(t, tags, 5000)
	assert.Less(t, elapsed, time.Second, "PROPFIND depth 1 over HTTP")
	t.Logf("5000 contacts: service list and PROPFIND depth 1 took %s over HTTP", elapsed)
}
