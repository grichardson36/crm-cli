package dav

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/carddav"
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
