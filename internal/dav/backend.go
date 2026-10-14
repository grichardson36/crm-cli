package dav

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/carddav"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Paths of the one user's resources.
type paths struct {
	Principal string // /dav/
	Home      string // /dav/<user>/
	Book      string // /dav/<user>/contacts/
}

func newPaths(user string) paths {
	return paths{Principal: "/dav/", Home: "/dav/" + user + "/", Book: "/dav/" + user + "/contacts/"}
}

func (p paths) objectPath(davID string) string { return p.Book + davID + ".vcf" }

// davIDFromPath extracts the device id from /dav/<user>/contacts/<id>.vcf.
func (p paths) davIDFromPath(reqPath string) (string, bool) {
	if !strings.HasPrefix(reqPath, p.Book) {
		return "", false
	}
	name := strings.TrimPrefix(reqPath, p.Book)
	if name == "" || strings.Contains(name, "/") || !strings.HasSuffix(name, ".vcf") {
		return "", false
	}
	id := strings.TrimSuffix(name, ".vcf")
	return id, id != ""
}

type ctxKey int

const serveKey ctxKey = 1

// withServing marks a request whose response carries card data to the phone.
func withServing(ctx context.Context) context.Context { return context.WithValue(ctx, serveKey, true) }

func isServing(ctx context.Context) bool { v, _ := ctx.Value(serveKey).(bool); return v }

// Backend adapts Service to the go-webdav CardDAV backend.
type Backend struct {
	Svc      *Service
	Paths    paths
	BookName string
}

var _ carddav.Backend = (*Backend)(nil)

func httpErr(code int, err error) error { return webdav.NewHTTPError(code, err) }

// mapErr turns service errors into HTTP errors.
func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, model.ErrNotFound):
		return httpErr(http.StatusNotFound, err)
	case errors.Is(err, ErrPrecondition):
		return httpErr(http.StatusPreconditionFailed, err)
	case errors.Is(err, ErrReadOnly):
		return httpErr(http.StatusForbidden, err)
	case errors.Is(err, model.ErrValidation):
		return httpErr(http.StatusBadRequest, err)
	case errors.Is(err, model.ErrConflict):
		return httpErr(http.StatusConflict, err)
	default:
		return err
	}
}

// CurrentUserPrincipal implements webdav.UserPrincipalBackend.
func (b *Backend) CurrentUserPrincipal(context.Context) (string, error) {
	return b.Paths.Principal, nil
}

// AddressBookHomeSetPath implements carddav.Backend.
func (b *Backend) AddressBookHomeSetPath(context.Context) (string, error) { return b.Paths.Home, nil }

func (b *Backend) book() carddav.AddressBook {
	return carddav.AddressBook{
		Path: b.Paths.Book, Name: b.BookName, Description: "Contacts from the crm database",
		SupportedAddressData: []carddav.AddressDataType{{ContentType: vcard.MIMEType, Version: "3.0"}},
	}
}

// ListAddressBooks implements carddav.Backend.
func (b *Backend) ListAddressBooks(context.Context) ([]carddav.AddressBook, error) {
	return []carddav.AddressBook{b.book()}, nil
}

func (b *Backend) isBook(p string) bool {
	return path.Clean(p) == path.Clean(b.Paths.Book)
}

// GetAddressBook implements carddav.Backend.
func (b *Backend) GetAddressBook(_ context.Context, p string) (*carddav.AddressBook, error) {
	if !b.isBook(p) {
		return nil, httpErr(http.StatusNotFound, fmt.Errorf("no address book at %s", p))
	}
	ab := b.book()
	return &ab, nil
}

// CreateAddressBook implements carddav.Backend. There is exactly one address book.
func (b *Backend) CreateAddressBook(context.Context, *carddav.AddressBook) error {
	return httpErr(http.StatusForbidden, errors.New("the address book is fixed: one per database"))
}

// DeleteAddressBook implements carddav.Backend.
func (b *Backend) DeleteAddressBook(context.Context, string) error {
	return httpErr(http.StatusForbidden, errors.New("the address book cannot be deleted"))
}

func (b *Backend) toObject(o *Object) carddav.AddressObject {
	return carddav.AddressObject{Path: b.Paths.objectPath(o.DavID), ModTime: o.ModTime, ETag: o.ETag, Card: o.Card}
}

// GetAddressObject implements carddav.Backend.
func (b *Backend) GetAddressObject(ctx context.Context, p string, _ *carddav.AddressDataRequest) (*carddav.AddressObject, error) {
	id, ok := b.Paths.davIDFromPath(p)
	if !ok {
		return nil, httpErr(http.StatusNotFound, fmt.Errorf("no contact at %s", p))
	}
	o, err := b.Svc.Get(ctx, id, isServing(ctx))
	if err != nil {
		return nil, mapErr(err)
	}
	ao := b.toObject(o)
	return &ao, nil
}

// ListAddressObjects implements carddav.Backend. It renders no vCards.
func (b *Backend) ListAddressObjects(ctx context.Context, p string, _ *carddav.AddressDataRequest) ([]carddav.AddressObject, error) {
	if !b.isBook(p) {
		return nil, httpErr(http.StatusNotFound, fmt.Errorf("no address book at %s", p))
	}
	objs, err := b.Svc.List(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]carddav.AddressObject, len(objs))
	for i := range objs {
		out[i] = b.toObject(&objs[i])
	}
	return out, nil
}

// QueryAddressObjects implements carddav.Backend.
func (b *Backend) QueryAddressObjects(ctx context.Context, p string, q *carddav.AddressBookQuery) ([]carddav.AddressObject, error) {
	if !b.isBook(p) {
		return nil, httpErr(http.StatusNotFound, fmt.Errorf("no address book at %s", p))
	}
	serving := isServing(ctx) && q != nil && (q.DataRequest.AllProp || len(q.DataRequest.Props) > 0)
	objs, err := b.Svc.All(ctx, serving)
	if err != nil {
		return nil, mapErr(err)
	}
	all := make([]carddav.AddressObject, len(objs))
	for i := range objs {
		all[i] = b.toObject(&objs[i])
	}
	if q == nil || len(q.PropFilters) == 0 {
		// RFC 6352: no filter matches every card. go-webdav's matcher matches none, so ask it
		// for "has a VERSION", which every card has.
		cp := carddav.AddressBookQuery{PropFilters: []carddav.PropFilter{{Name: vcard.FieldVersion}}}
		if q != nil {
			cp.DataRequest, cp.Limit = q.DataRequest, q.Limit
		}
		q = &cp
	}
	return carddav.Filter(q, all)
}

// PutAddressObject implements carddav.Backend.
func (b *Backend) PutAddressObject(ctx context.Context, p string, card vcard.Card, opts *carddav.PutAddressObjectOptions) (*carddav.AddressObject, error) {
	id, ok := b.Paths.davIDFromPath(p)
	if !ok {
		return nil, httpErr(http.StatusBadRequest, fmt.Errorf("contacts live at %s<id>.vcf", b.Paths.Book))
	}
	var ifMatch, ifNone webdav.ConditionalMatch
	if opts != nil {
		ifMatch, ifNone = opts.IfMatch, opts.IfNoneMatch
	}
	res, err := b.Svc.Put(ctx, id, card, ifMatch, ifNone)
	if err != nil {
		return nil, mapErr(err)
	}
	return &carddav.AddressObject{ETag: res.ETag}, nil
}

// DeleteAddressObject implements carddav.Backend.
func (b *Backend) DeleteAddressObject(ctx context.Context, p string) error {
	id, ok := b.Paths.davIDFromPath(p)
	if !ok {
		return httpErr(http.StatusNotFound, fmt.Errorf("no contact at %s", p))
	}
	ifMatch, _ := ctx.Value(ifMatchKey).(webdav.ConditionalMatch)
	return mapErr(b.Svc.Delete(ctx, id, ifMatch))
}

const ifMatchKey ctxKey = 2
