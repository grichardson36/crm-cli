package caldav

import (
	"bytes"
	"context"
	"crypto/sha1"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	wdcaldav "github.com/emersion/go-webdav/caldav"
	"github.com/stretchr/testify/require"
)

const (
	fakeUser = "grant"
	fakePass = "secret"
	fakeColl = "/grant/calendars/main/"
)

type fakeObject struct {
	data string // iCalendar text, CRLF
	etag string
}

// fakeServer is an in-memory CalDAV server built on go-webdav's own handler, so the adapter
// talks real CalDAV over HTTP: PROPFIND, REPORT, conditional PUT and DELETE.
type fakeServer struct {
	mu      sync.Mutex
	objects map[string]fakeObject
	version int
	puts    []string
	deletes []string
	srv     *httptest.Server
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	f := &fakeServer{objects: map[string]fakeObject{}}
	h := &wdcaldav.Handler{Backend: f}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != fakeUser || p != fakePass {
			w.Header().Set("WWW-Authenticate", `Basic realm="fake"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeServer) config() Config {
	return Config{URL: f.srv.URL + fakeColl, User: fakeUser, Password: fakePass}
}

func crlf(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
}

// set stores an object as the server would hold it after another client wrote it.
func (f *fakeServer) set(uid, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.version++
	data := crlf(body)
	sum := sha1.Sum([]byte(fmt.Sprintf("%d%s", f.version, data)))
	f.objects[fakeColl+uid+".ics"] = fakeObject{data: data, etag: fmt.Sprintf("%x", sum[:6])}
}

func (f *fakeServer) remove(uid string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, fakeColl+uid+".ics")
}

func (f *fakeServer) get(uid string) (fakeObject, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o, ok := f.objects[fakeColl+uid+".ics"]
	return o, ok
}

func (f *fakeServer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.objects)
}

func (f *fakeServer) requests() (puts, deletes []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.puts...), append([]string(nil), f.deletes...)
}

func (f *fakeServer) resetRequests() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.puts, f.deletes = nil, nil
}

func parse(data string) (*ical.Calendar, error) {
	return ical.NewDecoder(strings.NewReader(data)).Decode()
}

func (f *fakeServer) object(path string, o fakeObject) (wdcaldav.CalendarObject, error) {
	cal, err := parse(o.data)
	if err != nil {
		return wdcaldav.CalendarObject{}, err
	}
	return wdcaldav.CalendarObject{Path: path, ETag: o.etag, ContentLength: int64(len(o.data)), ModTime: time.Unix(1, 0), Data: cal}, nil
}

func (f *fakeServer) all() ([]wdcaldav.CalendarObject, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var paths []string
	for p := range f.objects {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var out []wdcaldav.CalendarObject
	for _, p := range paths {
		o, err := f.object(p, f.objects[p])
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

// Backend implementation.

func (f *fakeServer) CurrentUserPrincipal(ctx context.Context) (string, error) {
	return "/grant/", nil
}
func (f *fakeServer) CalendarHomeSetPath(ctx context.Context) (string, error) {
	return "/grant/calendars/", nil
}
func (f *fakeServer) CreateCalendar(ctx context.Context, c *wdcaldav.Calendar) error { return nil }
func (f *fakeServer) calendar() wdcaldav.Calendar {
	return wdcaldav.Calendar{Path: fakeColl, Name: "main", SupportedComponentSet: []string{"VEVENT"}}
}
func (f *fakeServer) ListCalendars(ctx context.Context) ([]wdcaldav.Calendar, error) {
	return []wdcaldav.Calendar{f.calendar()}, nil
}
func (f *fakeServer) GetCalendar(ctx context.Context, path string) (*wdcaldav.Calendar, error) {
	if !strings.HasPrefix(path, fakeColl) {
		return nil, webdav.NewHTTPError(http.StatusNotFound, fmt.Errorf("no calendar"))
	}
	c := f.calendar()
	return &c, nil
}
func (f *fakeServer) GetCalendarObject(ctx context.Context, path string, req *wdcaldav.CalendarCompRequest) (*wdcaldav.CalendarObject, error) {
	f.mu.Lock()
	o, ok := f.objects[path]
	f.mu.Unlock()
	if !ok {
		return nil, webdav.NewHTTPError(http.StatusNotFound, fmt.Errorf("not found"))
	}
	co, err := f.object(path, o)
	return &co, err
}
func (f *fakeServer) ListCalendarObjects(ctx context.Context, path string, req *wdcaldav.CalendarCompRequest) ([]wdcaldav.CalendarObject, error) {
	return f.all()
}
func (f *fakeServer) QueryCalendarObjects(ctx context.Context, path string, query *wdcaldav.CalendarQuery) ([]wdcaldav.CalendarObject, error) {
	objs, err := f.all()
	if err != nil {
		return nil, err
	}
	return wdcaldav.Filter(query, objs)
}
func (f *fakeServer) PutCalendarObject(ctx context.Context, path string, cal *ical.Calendar, opts *wdcaldav.PutCalendarObjectOptions) (*wdcaldav.CalendarObject, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, exists := f.objects[path]
	if opts != nil {
		if opts.IfNoneMatch.IsSet() && exists {
			return nil, webdav.NewHTTPError(http.StatusPreconditionFailed, fmt.Errorf("exists"))
		}
		if opts.IfMatch.IsSet() {
			ok, err := opts.IfMatch.MatchETag(cur.etag)
			if !exists || err != nil || !ok {
				return nil, webdav.NewHTTPError(http.StatusPreconditionFailed, fmt.Errorf("etag mismatch"))
			}
		}
	}
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return nil, err
	}
	f.version++
	sum := sha1.Sum([]byte(fmt.Sprintf("%d%s", f.version, buf.String())))
	o := fakeObject{data: buf.String(), etag: fmt.Sprintf("%x", sum[:6])}
	f.objects[path] = o
	f.puts = append(f.puts, path)
	co, err := f.object(path, o)
	return &co, err
}
func (f *fakeServer) DeleteCalendarObject(ctx context.Context, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.objects[path]; !ok {
		return webdav.NewHTTPError(http.StatusNotFound, fmt.Errorf("not found"))
	}
	delete(f.objects, path)
	f.deletes = append(f.deletes, path)
	return nil
}

func TestFakeServerSpeaksCalDAV(t *testing.T) {
	f := newFakeServer(t)
	f.set("one", "BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//t//EN\nBEGIN:VEVENT\nUID:one\nDTSTAMP:20261001T000000Z\nSUMMARY:One\nDTSTART:20261016T170000Z\nDTEND:20261016T180000Z\nEND:VEVENT\nEND:VCALENDAR\n")
	cl, err := NewClient(f.config(), nil)
	require.NoError(t, err)
	objs, err := cl.List(context.Background())
	require.NoError(t, err)
	require.Len(t, objs, 1)
	require.Equal(t, fakeColl+"one.ics", objs[0].Href)
	require.Contains(t, string(objs[0].Data), "SUMMARY:One")

	wrong := f.config()
	wrong.Password = "nope"
	bad, err := NewClient(wrong, nil)
	require.NoError(t, err)
	_, err = bad.List(context.Background())
	require.Error(t, err)
}
