package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/replica"
	"github.com/jdanielnd/crm-cli/internal/replica/replicasql"
)

// Server is the relay role (contract rule 5): it holds changes for enrolled devices and serves
// pulls and pushes. Any device with an enabled database can run one.
type Server struct {
	A        *replicasql.Adapter
	ReadOnly bool
	Log      *log.Logger
}

const maxBody = 64 << 20

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	p := "/" + Version
	mux.HandleFunc("POST "+p+"/enroll", s.enroll)
	mux.HandleFunc("POST "+p+"/pull", s.auth(s.pull))
	mux.HandleFunc("POST "+p+"/push", s.auth(s.push))
	mux.HandleFunc("GET "+p+"/status", s.auth(s.status))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		rec := &statusRecorder{ResponseWriter: w, code: 200}
		mux.ServeHTTP(rec, r)
		if s.Log != nil {
			s.Log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.code, time.Since(started).Round(time.Millisecond))
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (r *statusRecorder) WriteHeader(c int) { r.code = c; r.ResponseWriter.WriteHeader(c) }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, format string, a ...any) {
	writeJSON(w, code, ErrorResponse{Error: fmt.Sprintf(format, a...)})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "bad request body: %v", err)
		return false
	}
	return true
}

type deviceHandler func(w http.ResponseWriter, r *http.Request, device string)

// auth checks the bearer token and passes the device it belongs to.
func (s *Server) auth(next deviceHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if tok == "" || tok == r.Header.Get("Authorization") {
			fail(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		dev, err := s.A.Authenticate(r.Context(), tok)
		if err != nil {
			fail(w, http.StatusUnauthorized, "token is not valid")
			return
		}
		next(w, r, dev)
	}
}

func (s *Server) enroll(w http.ResponseWriter, r *http.Request) {
	if s.ReadOnly {
		fail(w, http.StatusForbidden, "relay is read-only")
		return
	}
	var req EnrollRequest
	if !decode(w, r, &req) {
		return
	}
	tok, err := s.A.Enroll(r.Context(), req.Token, req.Device, req.Name)
	switch {
	case errors.Is(err, replicasql.ErrBadToken):
		fail(w, http.StatusUnauthorized, "%v", err)
		return
	case err != nil:
		fail(w, http.StatusConflict, "%v", err)
		return
	}
	self, _ := s.A.FindDevice(r.Context(), s.A.Device())
	writeJSON(w, http.StatusOK, EnrollResponse{Device: s.A.Device(), Name: self.Name, Token: tok, SchemaVersion: replicasql.SchemaVersion})
}

func clampLimit(n int) int {
	switch {
	case n <= 0:
		return DefaultLimit
	case n > MaxLimit:
		return MaxLimit
	}
	return n
}

func (s *Server) pull(w http.ResponseWriter, r *http.Request, device string) {
	var req PullRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Cursor == nil {
		req.Cursor = replica.VersionVector{}
	}
	limit := clampLimit(req.Limit)
	cs, err := s.A.Pull(r.Context(), req.Cursor, limit+1)
	if err != nil {
		fail(w, http.StatusInternalServerError, "%v", err)
		return
	}
	more := len(cs) > limit
	if more {
		cs = cs[:limit]
	}
	cur := req.Cursor.Clone()
	for _, c := range cs {
		if c.ID.Seq > cur[c.ID.Device] {
			cur[c.ID.Device] = c.ID.Seq
		}
	}
	vec, err := s.A.Vector(r.Context())
	if err != nil {
		fail(w, http.StatusInternalServerError, "%v", err)
		return
	}
	_ = s.A.NoteDevice(r.Context(), device, req.Cursor, req.Head, true, false, "")
	if cs == nil {
		cs = []replica.Change{}
	}
	writeJSON(w, http.StatusOK, PullResponse{Device: s.A.Device(), SchemaVersion: replicasql.SchemaVersion, Changes: cs, Cursor: cur, Vector: vec, More: more})
}

func (s *Server) push(w http.ResponseWriter, r *http.Request, device string) {
	if s.ReadOnly {
		fail(w, http.StatusForbidden, "relay is read-only")
		return
	}
	var req PushRequest
	if !decode(w, r, &req) {
		return
	}
	if len(req.Changes) > MaxLimit {
		fail(w, http.StatusRequestEntityTooLarge, "at most %d changes per push", MaxLimit)
		return
	}
	own, _ := s.A.Vector(r.Context())
	for _, c := range req.Changes {
		if c.ID.Device == s.A.Device() && c.ID.Seq > own[s.A.Device()] {
			fail(w, http.StatusConflict, "change %s#%d claims the relay's own device id with a sequence it never wrote: two databases share one device id", c.ID.Device, c.ID.Seq)
			return
		}
	}
	res, err := s.A.ApplyBatch(r.Context(), req.Changes)
	if err != nil {
		fail(w, http.StatusInternalServerError, "%v", err)
		return
	}
	vec, _ := s.A.Vector(r.Context())
	_ = s.A.NoteDevice(r.Context(), device, req.Vector, req.Head, false, true, "")
	writeJSON(w, http.StatusOK, PushResponse{Device: s.A.Device(), Applied: res.Applied, Duplicates: res.Duplicates, Unknown: res.Unknown, Rejected: res.Rejected, Vector: vec})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request, _ string) {
	st, err := s.A.Status(r.Context())
	if err != nil {
		fail(w, http.StatusInternalServerError, "%v", err)
		return
	}
	devs, _ := s.A.Devices(r.Context())
	writeJSON(w, http.StatusOK, StatusResponse{Device: st.Device, SchemaVersion: st.SchemaVersion, ReadOnly: s.ReadOnly, Vector: st.Vector,
		Changes: st.Changes, OpenReviews: st.OpenReviews, Devices: len(devs)})
}

// CheckBind refuses an address that is reachable from the public internet, unless allowPublic is
// set. Private LAN ranges, link-local, loopback, the Tailscale range (100.64.0.0/10) and IPv6
// unique-local addresses are fine. A wildcard address (0.0.0.0, ::, or no host) listens on every
// interface, so it is refused. A host name other than localhost is refused because it cannot be
// checked.
func CheckBind(addr string, allowPublic bool) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("bad address %q: %w", addr, err)
	}
	if allowPublic {
		return nil
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("refusing to listen on host name %q: pass an IP address (Tailscale or LAN) or --allow-public", host)
	}
	if ip.IsUnspecified() {
		return fmt.Errorf("refusing to listen on %s: that is every network interface. Bind a Tailscale or LAN address, or pass --allow-public", addr)
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || inCGNAT(ip) {
		return nil
	}
	return fmt.Errorf("refusing to listen on %s: it is a public address. The relay has no encryption, so keep it on Tailscale or a LAN, or pass --allow-public", addr)
}

func inCGNAT(ip net.IP) bool {
	_, n, _ := net.ParseCIDR("100.64.0.0/10")
	return n.Contains(ip)
}

// ListenAndServe serves until ctx is done. ready, if set, is called with the bound address.
func (s *Server) ListenAndServe(ctx context.Context, addr string, ready func(net.Addr)) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	if ready != nil {
		ready(ln.Addr())
	}
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
		return nil
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
