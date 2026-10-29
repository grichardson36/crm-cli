package dav

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/carddav"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Options configures the HTTP handler.
type Options struct {
	Service  *Service
	User     string
	Password string
	// BookName is the address book's display name on the phone.
	BookName string
	// Logger receives one line per request. It never sees credentials. May be nil.
	Logger *log.Logger
}

var userPattern = regexp.MustCompile(`^[A-Za-z0-9._@-]{1,64}$`)

// ValidUser reports whether a name is safe to use in the address book path.
func ValidUser(name string) bool { return userPattern.MatchString(name) }

// BookPath returns the address book path for a user.
func BookPath(user string) string { return newPaths(user).Book }

const (
	nsDAV    = "DAV:"
	nsCard   = "urn:ietf:params:xml:ns:carddav"
	nsCalSrv = "http://calendarserver.org/ns/"
)

type handler struct {
	o     Options
	paths paths
	inner *carddav.Handler

	touchMu   sync.Mutex
	lastTouch time.Time
}

// NewHandler returns the CardDAV handler: basic auth, the well-known redirect, and one address
// book at /dav/<user>/contacts/. TLS is the reverse proxy's job.
func NewHandler(o Options) http.Handler {
	p := newPaths(o.User)
	if o.BookName == "" {
		o.BookName = "CRM contacts"
	}
	return &handler{
		o: o, paths: p,
		inner: &carddav.Handler{Backend: &Backend{Svc: o.Service, Paths: p, BookName: o.BookName}},
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	h.serve(rec, r)
	if h.o.Logger != nil {
		h.o.Logger.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	}
}

func hash(s string) [32]byte { return sha256.Sum256([]byte(s)) }

func (h *handler) authorized(r *http.Request) bool {
	user, pass, ok := r.BasicAuth()
	if !ok {
		return false
	}
	u1, u2 := hash(user), hash(h.o.User)
	p1, p2 := hash(pass), hash(h.o.Password)
	okUser := subtle.ConstantTimeCompare(u1[:], u2[:])
	okPass := subtle.ConstantTimeCompare(p1[:], p2[:])
	return okUser&okPass == 1
}

func (h *handler) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/.well-known/carddav" {
		http.Redirect(w, r, h.paths.Principal, http.StatusPermanentRedirect)
		return
	}
	if !h.authorized(r) {
		time.Sleep(250 * time.Millisecond) // slows password guessing
		w.Header().Set("WWW-Authenticate", `Basic realm="crm", charset="UTF-8"`)
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	h.touch(r.Context())

	// Collections are addressed with or without the trailing slash.
	for _, p := range []string{h.paths.Principal, h.paths.Home, h.paths.Book} {
		if r.URL.Path == strings.TrimSuffix(p, "/") {
			r.URL.Path = p
		}
	}
	if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/dav/") {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodPut, http.MethodDelete, "MKCOL", "PROPPATCH", "COPY", "MOVE", "LOCK", "UNLOCK":
		if h.o.Service.ReadOnly {
			http.Error(w, "this server is read-only", http.StatusForbidden)
			return
		}
	}

	ctx := r.Context()
	switch r.Method {
	case http.MethodGet, "REPORT":
		ctx = withServing(ctx)
	case http.MethodDelete:
		ctx = context.WithValue(ctx, ifMatchKey, webdav.ConditionalMatch(r.Header.Get("If-Match")))
	}
	r = r.WithContext(ctx)

	if r.Method == "PROPFIND" && h.collectionPropfind(w, r) {
		return
	}
	if r.Method == http.MethodPut && !h.capturePut(w, r) {
		return
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		if h.serveRaw(w, r) {
			return
		}
	}
	if r.Method == "REPORT" {
		h.serveReport(w, r)
		return
	}
	h.inner.ServeHTTP(w, r)
}

func (h *handler) touch(ctx context.Context) {
	h.touchMu.Lock()
	defer h.touchMu.Unlock()
	now := h.o.Service.Now()
	if !h.lastTouch.IsZero() && now.Sub(h.lastTouch) < 30*time.Second {
		return
	}
	h.lastTouch = now
	_ = h.o.Service.Dav.SetMeta(ctx, "dav.last_request_at", now.UTC().Format(timeLayout))
}

// --- PROPFIND on the collections ----------------------------------------------------------
//
// go-webdav answers PROPFIND for the address book without getctag and with a fixed privilege
// set. Phones read the CTag to skip a full listing when nothing changed, and the privilege set
// to decide whether the book is writable, so those two requests are answered here.

type propfindBody struct {
	Prop *struct {
		Items []struct {
			XMLName xml.Name
		} `xml:",any"`
	} `xml:"DAV: prop"`
}

func (h *handler) collectionPropfind(w http.ResponseWriter, r *http.Request) bool {
	isBook := r.URL.Path == h.paths.Book
	isHome := r.URL.Path == h.paths.Home
	if !isBook && !isHome {
		return false
	}
	depth := r.Header.Get("Depth")
	if isBook && depth != "0" {
		return false // the listing of contacts is go-webdav's
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return false
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	var pf propfindBody
	if len(bytes.TrimSpace(body)) == 0 || xml.Unmarshal(body, &pf) != nil || pf.Prop == nil {
		return false // allprop and propname use the default answer
	}
	var names []xml.Name
	for _, it := range pf.Prop.Items {
		names = append(names, it.XMLName)
	}

	ctag, err := h.o.Service.CTag(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return true
	}
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	fmt.Fprintf(&sb, `<D:multistatus xmlns:D="DAV:" xmlns:C="%s" xmlns:CS="%s">`, nsCard, nsCalSrv)
	if isHome {
		sb.WriteString(h.response(h.paths.Home, names, h.homeProps()))
		if depth != "0" {
			sb.WriteString(h.response(h.paths.Book, names, h.bookProps(ctag)))
		}
	} else {
		sb.WriteString(h.response(h.paths.Book, names, h.bookProps(ctag)))
	}
	sb.WriteString(`</D:multistatus>`)

	w.Header().Set("Content-Type", `application/xml; charset="utf-8"`)
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = io.WriteString(w, sb.String())
	return true
}

func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func (h *handler) privileges() string {
	if h.o.Service.ReadOnly {
		return `<D:current-user-privilege-set><D:privilege><D:read/></D:privilege></D:current-user-privilege-set>`
	}
	return `<D:current-user-privilege-set><D:privilege><D:read/></D:privilege><D:privilege><D:write/></D:privilege>` +
		`<D:privilege><D:write-content/></D:privilege><D:privilege><D:bind/></D:privilege><D:privilege><D:unbind/></D:privilege></D:current-user-privilege-set>`
}

func (h *handler) principalProp() string {
	return `<D:current-user-principal><D:href>` + esc(h.paths.Principal) + `</D:href></D:current-user-principal>`
}

func (h *handler) homeProps() map[xml.Name]string {
	return map[xml.Name]string{
		{Space: nsDAV, Local: "resourcetype"}:               `<D:resourcetype><D:collection/></D:resourcetype>`,
		{Space: nsDAV, Local: "displayname"}:                `<D:displayname>` + esc(h.o.User) + `</D:displayname>`,
		{Space: nsDAV, Local: "current-user-principal"}:     h.principalProp(),
		{Space: nsDAV, Local: "current-user-privilege-set"}: h.privileges(),
	}
}

func (h *handler) bookProps(ctag string) map[xml.Name]string {
	return map[xml.Name]string{
		{Space: nsDAV, Local: "resourcetype"}:               `<D:resourcetype><D:collection/><C:addressbook/></D:resourcetype>`,
		{Space: nsDAV, Local: "displayname"}:                `<D:displayname>` + esc(h.o.BookName) + `</D:displayname>`,
		{Space: nsDAV, Local: "current-user-principal"}:     h.principalProp(),
		{Space: nsDAV, Local: "current-user-privilege-set"}: h.privileges(),
		{Space: nsCalSrv, Local: "getctag"}:                 `<CS:getctag>` + ctag + `</CS:getctag>`,
		{Space: nsCard, Local: "addressbook-description"}:   `<C:addressbook-description>Contacts from the crm database</C:addressbook-description>`,
		{Space: nsCard, Local: "supported-address-data"}:    `<C:supported-address-data><C:address-data-type content-type="text/vcard" version="3.0"/></C:supported-address-data>`,
		{Space: nsDAV, Local: "supported-report-set"}: `<D:supported-report-set>` +
			`<D:supported-report><D:report><C:addressbook-multiget/></D:report></D:supported-report>` +
			`<D:supported-report><D:report><C:addressbook-query/></D:report></D:supported-report></D:supported-report-set>`,
	}
}

func (h *handler) response(href string, requested []xml.Name, known map[xml.Name]string) string {
	var ok, missing strings.Builder
	for _, n := range requested {
		if v, found := known[n]; found {
			ok.WriteString(v)
		} else {
			fmt.Fprintf(&missing, `<%s xmlns="%s"/>`, n.Local, n.Space)
		}
	}
	var sb strings.Builder
	sb.WriteString(`<D:response><D:href>` + esc(href) + `</D:href>`)
	if ok.Len() > 0 {
		sb.WriteString(`<D:propstat><D:prop>` + ok.String() + `</D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat>`)
	}
	if missing.Len() > 0 {
		sb.WriteString(`<D:propstat><D:prop>` + missing.String() + `</D:prop><D:status>HTTP/1.1 404 Not Found</D:status></D:propstat>`)
	}
	sb.WriteString(`</D:response>`)
	return sb.String()
}

// --- stored cards ------------------------------------------------------------------------

// maxCardBytes bounds a PUT body.
const maxCardBytes = 8 << 20

// capturePut reads the PUT body so the exact text the phone sent is stored (patterns/research-sync-standard
// section 2). A card that cannot be read is skipped and flagged as a sync issue, it is never read as
// an emptied contact. It returns false when it has answered the request.
func (h *handler) capturePut(w http.ResponseWriter, r *http.Request) bool {
	id, ok := h.paths.davIDFromPath(r.URL.Path)
	if !ok {
		return true
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxCardBytes+1))
	if err != nil {
		http.Error(w, "could not read the request body", http.StatusBadRequest)
		return false
	}
	if len(body) > maxCardBytes {
		http.Error(w, "vCard is too large", http.StatusRequestEntityTooLarge)
		return false
	}
	raw := string(body)
	if problem := unreadable(raw); problem != "" {
		var personID int64
		if t, err := h.o.Service.Dav.Resolve(r.Context(), id); err == nil {
			personID = t.PersonID
		}
		_, _ = h.o.Service.Dav.AddIssue(r.Context(), id, personID, model.DavIssueUnreadable, problem, raw)
		http.Error(w, "the vCard could not be read and was skipped: "+problem, http.StatusBadRequest)
		return false
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	*r = *r.WithContext(withRaw(r.Context(), raw))
	return true
}

// unreadable returns why a vCard cannot be used, or "" when it parses.
func unreadable(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "the request body is empty"
	}
	if _, err := parseRaw(raw); err != nil {
		return err.Error()
	}
	if _, err := decodeCard(raw); err != nil {
		return err.Error()
	}
	return ""
}

// serveRaw answers GET and HEAD of one contact with the stored card text, so lines the CRM did not
// change reach the phone byte for byte. It returns false for paths that are not a contact.
func (h *handler) serveRaw(w http.ResponseWriter, r *http.Request) bool {
	id, ok := h.paths.davIDFromPath(r.URL.Path)
	if !ok {
		return false
	}
	o, err := h.o.Service.Get(r.Context(), id, r.Method == http.MethodGet)
	if err != nil {
		http.Error(w, err.Error(), statusFor(err))
		return true
	}
	w.Header().Set("Content-Type", "text/vcard; charset=utf-8")
	w.Header().Set("ETag", `"`+o.ETag+`"`)
	w.Header().Set("Last-Modified", o.ModTime.UTC().Format(http.TimeFormat))
	w.Header().Set("Content-Length", strconv.Itoa(len(o.Raw)))
	if r.Method == http.MethodGet {
		_, _ = io.WriteString(w, o.Raw)
	}
	return true
}

// bufferedWriter holds a response so serveReport can rewrite it.
type bufferedWriter struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (b *bufferedWriter) Header() http.Header         { return b.header }
func (b *bufferedWriter) WriteHeader(code int)        { b.code = code }
func (b *bufferedWriter) Write(p []byte) (int, error) { return b.body.Write(p) }

var (
	reportResponse = regexp.MustCompile(`(?s)<response[^>]*><href>([^<]*)</href>(.*?)</response>`)
	reportData     = regexp.MustCompile(`(?s)(<address-data[^>]*>)(.*?)(</address-data>)`)
)

// serveReport answers REPORT (addressbook-multiget and addressbook-query, which phones use to fetch
// cards). go-webdav writes the cards from a parsed form, so the address-data of each contact is put
// back to the stored card text: lines the CRM did not change reach the phone byte for byte.
func (h *handler) serveReport(w http.ResponseWriter, r *http.Request) {
	bw := &bufferedWriter{header: http.Header{}, code: http.StatusOK}
	h.inner.ServeHTTP(bw, r)
	out := bw.body.Bytes()
	if bw.code == http.StatusMultiStatus && bytes.Contains(out, []byte("address-data")) {
		out = h.restoreRaw(r.Context(), out)
	}
	for k, v := range bw.header {
		w.Header()[k] = v
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(out)))
	w.WriteHeader(bw.code)
	_, _ = w.Write(out)
}

// restoreRaw swaps the address-data of every response for the stored card text of that contact.
func (h *handler) restoreRaw(ctx context.Context, body []byte) []byte {
	return reportResponse.ReplaceAllFunc(body, func(resp []byte) []byte {
		m := reportResponse.FindSubmatch(resp)
		var href string
		if err := xml.Unmarshal([]byte("<h>"+string(m[1])+"</h>"), &href); err != nil {
			return resp
		}
		id, ok := h.paths.davIDFromPath(href)
		if !ok {
			return resp
		}
		t, err := h.o.Service.Dav.Resolve(ctx, id)
		if err != nil {
			return resp
		}
		st, err := h.o.Service.Dav.GetState(ctx, t.PersonID)
		if err != nil || st == nil || st.RawVCard == nil || *st.RawVCard == "" {
			return resp
		}
		var esc bytes.Buffer
		if err := xml.EscapeText(&esc, []byte(*st.RawVCard)); err != nil {
			return resp
		}
		// A function, not a template: the card text holds "$" (Apple labels are _$!<Mobile>!$_).
		return reportData.ReplaceAllFunc(resp, func(d []byte) []byte {
			sub := reportData.FindSubmatch(d)
			return append(append(append([]byte{}, sub[1]...), esc.Bytes()...), sub[3]...)
		})
	})
}

// statusFor is the HTTP status of a service error, the same mapping mapErr uses.
func statusFor(err error) int {
	switch {
	case errors.Is(err, model.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrPrecondition):
		return http.StatusPreconditionFailed
	case errors.Is(err, ErrReadOnly):
		return http.StatusForbidden
	case errors.Is(err, model.ErrValidation):
		return http.StatusBadRequest
	case errors.Is(err, model.ErrConflict):
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}
