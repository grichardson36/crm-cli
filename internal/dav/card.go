// Package dav is the CardDAV server built into crm (H06). It exposes the active people of one
// database as one address book, accepts edits from the phone, and merges them per field.
//
// vCard mapping (vCard 3.0, which DAVx5, iOS and macOS all read):
//
//	UID         the device's external id, or the person's uuid for people created in the CRM
//	N, FN       last name, first name (middle name, prefix and suffix from the phone are kept aside)
//	EMAIL, TEL  every address and number, TYPE carries the label, the primary one gets TYPE=PREF
//	ORG, TITLE  company and title (extra ORG components such as a department are kept aside)
//	NOTE        the person's notes
//	BDAY        1985-03-14, or --03-14 when the year is unknown
//	URL         the website handle
//	ADR         the whole location text, in the street component (see locationFromAdr)
//	X-SOCIALPROFILE;TYPE=kind plus a grouped URL with X-ABLabel as a fallback, one per social handle
//	REV         updated_at
//
// Not synced, they stay in the CRM: priority tier, tags, keep-in-touch cadence, last contacted,
// interactions and notes timeline, custom fields. Every other vCard property the phone sends
// (photo, nickname, extra addresses, IM, categories, ...) is stored in dav_state.extra_vcard
// and sent back unchanged.
package dav

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-vcard"
	"github.com/jdanielnd/crm-cli/internal/dedupe"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// MapVersion is part of every ETag, so a change to the mapping makes phones refetch.
const MapVersion = "1"

const (
	fieldFirst    = "first_name"
	fieldLast     = "last_name"
	fieldCompany  = "company"
	fieldTitle    = "title"
	fieldNotes    = "notes"
	fieldLocation = "location"
	fieldBirthday = "birthday"
	handlePrefix  = "handle:"
	fieldLink     = "_link"
)

// scalarFields are the single-value fields that merge three-way. The names match field_versions.
var scalarFields = []string{fieldFirst, fieldLast, fieldCompany, fieldTitle, fieldNotes, fieldLocation, fieldBirthday}

// Item is one email address or phone number with its label.
type Item struct {
	Value string `json:"v"`
	Label string `json:"l,omitempty"`
}

// Contact is the part of a person that travels to and from the phone, in the form the phone
// sees it. It is always produced by parsing a vCard, so the CRM side and the phone side are
// canonicalized the same way.
type Contact struct {
	First    string            `json:"first,omitempty"`
	Last     string            `json:"last,omitempty"`
	Company  string            `json:"company,omitempty"`
	Title    string            `json:"title,omitempty"`
	Notes    string            `json:"notes,omitempty"`
	Location string            `json:"location,omitempty"`
	Birthday string            `json:"birthday,omitempty"`
	Emails   []Item            `json:"emails,omitempty"`
	Phones   []Item            `json:"phones,omitempty"`
	Handles  map[string]string `json:"handles,omitempty"`
}

func (c *Contact) scalars() map[string]string {
	return map[string]string{
		fieldFirst: c.First, fieldLast: c.Last, fieldCompany: c.Company, fieldTitle: c.Title,
		fieldNotes: c.Notes, fieldLocation: c.Location, fieldBirthday: c.Birthday,
	}
}

func emailKey(v string) string { return dedupe.EmailKey(v) }

func phoneKey(v string) string {
	if k := dedupe.PhoneKey(v); k != "" {
		return k
	}
	return strings.ToLower(strings.TrimSpace(v))
}

func handleKey(v string) string {
	if k := dedupe.HandleKey(v); k != "" {
		return k
	}
	return strings.ToLower(strings.TrimSpace(v))
}

// signature is a canonical string used to tell whether two contacts are the same to the phone.
func (c *Contact) signature() string {
	var b strings.Builder
	sc := c.scalars()
	for _, f := range scalarFields {
		fmt.Fprintf(&b, "%s=%q;", f, sc[f])
	}
	list := func(name string, items []Item, key func(string) string) {
		keys := make([]string, 0, len(items))
		for _, it := range items {
			keys = append(keys, key(it.Value)+"|"+strings.ToLower(it.Label))
		}
		sort.Strings(keys)
		fmt.Fprintf(&b, "%s=%q;", name, keys)
	}
	list("emails", c.Emails, emailKey)
	list("phones", c.Phones, phoneKey)
	kinds := make([]string, 0, len(c.Handles))
	for k := range c.Handles {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		fmt.Fprintf(&b, "h:%s=%q;", k, handleKey(c.Handles[k]))
	}
	return b.String()
}

// Equal reports whether two contacts look the same to the phone.
func (c *Contact) Equal(o *Contact) bool { return c.signature() == o.signature() }

// MarshalBase serializes the contact for dav_state.base_json.
func (c *Contact) MarshalBase() string {
	b, _ := json.Marshal(c)
	return string(b)
}

// ParseBase reads dav_state.base_json. An empty or broken base reads as an empty contact.
func ParseBase(s *string) *Contact {
	c := &Contact{}
	if s != nil && *s != "" {
		_ = json.Unmarshal([]byte(*s), c)
	}
	return c
}

// --- labels ------------------------------------------------------------------------------

var typeToLabel = map[string]string{
	"cell": "Mobile", "mobile": "Mobile", "home": "Home", "work": "Work",
	"fax": "Fax", "pager": "Pager", "other": "Other", "main": "Main",
}

var labelToType = map[string]string{
	"mobile": "CELL", "cell": "CELL", "home": "HOME", "work": "WORK", "fax": "FAX", "pager": "PAGER", "other": "OTHER",
}

var ignoredTypes = map[string]bool{
	"pref": true, "voice": true, "internet": true, "text": true, "video": true, "msg": true,
	"textphone": true, "x400": true, "iphone": true,
}

// socialKinds maps TYPE values and X-ABLabel text to a handle kind.
var socialKinds = map[string]string{
	"linkedin": "linkedin", "twitter": "twitter", "x": "twitter", "facebook": "facebook",
	"instagram": "instagram", "telegram": "telegram", "whatsapp": "whatsapp", "tiktok": "tiktok",
	"youtube": "youtube", "other": "other",
}

func titleCase(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// cleanABLabel turns Apple's "_$!<Mobile>!$_" into "Mobile".
func cleanABLabel(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "_$!<") && strings.HasSuffix(s, ">!$_") {
		s = s[4 : len(s)-4]
	}
	return s
}

func labelFromField(f *vcard.Field, groupLabel map[string]string) string {
	types := f.Params.Types()
	for _, t := range types {
		if l, ok := typeToLabel[t]; ok {
			return l
		}
	}
	if f.Group != "" {
		if l := cleanABLabel(groupLabel[f.Group]); l != "" {
			if mapped, ok := typeToLabel[strings.ToLower(l)]; ok {
				return mapped
			}
			return l
		}
	}
	for _, t := range types {
		if !ignoredTypes[t] && t != "" {
			return titleCase(t)
		}
	}
	return ""
}

// --- vCard text helpers ------------------------------------------------------------------

// splitStructured splits a structured value on unescaped semicolons. The vcard decoder leaves
// "\;" in place, so it is unescaped here.
func splitStructured(v string) []string {
	var parts []string
	var cur strings.Builder
	rs := []rune(v)
	for i := 0; i < len(rs); i++ {
		switch {
		case rs[i] == '\\' && i+1 < len(rs) && rs[i+1] == ';':
			cur.WriteRune(';')
			i++
		case rs[i] == ';':
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(rs[i])
		}
	}
	return append(parts, cur.String())
}

func comp(parts []string, i int) string {
	if i < len(parts) {
		return strings.TrimSpace(parts[i])
	}
	return ""
}

// text unescapes the one escape the decoder leaves behind and trims.
func text(v string) string {
	return strings.TrimSpace(strings.ReplaceAll(v, `\;`, ";"))
}

// compOut makes a value safe inside a structured field. The encoder escapes backslashes, so a
// literal semicolon cannot be written as "\;" and becomes a comma.
func compOut(v string) string {
	v = strings.ReplaceAll(v, "\r\n", "\n")
	v = strings.ReplaceAll(v, "\r", "\n")
	return strings.ReplaceAll(v, ";", ",")
}

func normNewlines(v string) string {
	v = strings.ReplaceAll(v, "\r\n", "\n")
	return strings.ReplaceAll(v, "\r", "\n")
}

// --- birthday ----------------------------------------------------------------------------

func parseBDay(f *vcard.Field) string {
	v := strings.TrimSpace(f.Value)
	if i := strings.IndexAny(v, "Tt"); i > 0 {
		v = v[:i]
	}
	var year, month, day int
	var err error
	atoi := func(s string) int {
		n, e := strconv.Atoi(s)
		if e != nil {
			err = e
		}
		return n
	}
	switch {
	case strings.HasPrefix(v, "--"):
		d := strings.ReplaceAll(v[2:], "-", "")
		if len(d) != 4 {
			return ""
		}
		month, day = atoi(d[:2]), atoi(d[2:])
	default:
		d := strings.ReplaceAll(v, "-", "")
		if len(d) != 8 {
			return ""
		}
		year, month, day = atoi(d[:4]), atoi(d[4:6]), atoi(d[6:])
		if year == 1604 || f.Params.Get("X-APPLE-OMIT-YEAR") != "" {
			year = 0
		}
	}
	if err != nil {
		return ""
	}
	out := fmt.Sprintf("--%02d-%02d", month, day)
	if year > 0 {
		out = fmt.Sprintf("%04d-%02d-%02d", year, month, day)
	}
	if _, _, _, perr := model.ParseBirthday(out); perr != nil {
		return ""
	}
	return out
}

func personBirthday(p *model.Person) string {
	if p.BirthdayMonth == nil || p.BirthdayDay == nil {
		return ""
	}
	if p.BirthdayYear != nil {
		return fmt.Sprintf("%04d-%02d-%02d", *p.BirthdayYear, *p.BirthdayMonth, *p.BirthdayDay)
	}
	return fmt.Sprintf("--%02d-%02d", *p.BirthdayMonth, *p.BirthdayDay)
}

// --- location ----------------------------------------------------------------------------

// locationFromAdr joins the filled components of an ADR into one line. A location written by
// this server sits whole in the street component, so it comes back exactly. An address the
// phone structured into street, city, region, postcode and country comes back as
// "street, city, region, postcode, country".
func locationFromAdr(f *vcard.Field) string {
	parts := splitStructured(f.Value)
	var out []string
	for _, i := range []int{0, 1, 2, 3, 4, 5, 6} {
		if c := comp(parts, i); c != "" {
			out = append(out, c)
		}
	}
	return strings.Join(out, ", ")
}

// --- render ------------------------------------------------------------------------------

func nonNil(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

// Render builds the vCard the phone gets for a person. p must have its emails, phones and
// handles loaded. extra holds the properties kept from the phone, and may be nil.
func Render(p *model.Person, extra vcard.Card, uid string) vcard.Card {
	card := vcard.Card{}
	card.SetValue(vcard.FieldVersion, "3.0")
	card.SetValue(vcard.FieldProductID, "-//crm-cli//CardDAV//EN")
	card.SetValue(vcard.FieldUID, uid)

	given, family := strings.TrimSpace(p.FirstName), nonNil(p.LastName)
	var additional, prefix, suffix string
	if f := extra.Get(vcard.FieldName); f != nil {
		parts := splitStructured(f.Value)
		additional, prefix, suffix = comp(parts, 2), comp(parts, 3), comp(parts, 4)
	}
	card.SetValue(vcard.FieldName, strings.Join([]string{
		compOut(family), compOut(given), compOut(additional), compOut(prefix), compOut(suffix)}, ";"))
	var fn []string
	for _, s := range []string{prefix, given, additional, family, suffix} {
		if s != "" {
			fn = append(fn, s)
		}
	}
	fnText := strings.Join(fn, " ")
	if fnText == "" {
		fnText = "Unknown"
	}
	card.SetValue(vcard.FieldFormattedName, normNewlines(fnText))

	company := nonNil(p.Company)
	orgRest := ""
	if f := extra.Get(vcard.FieldOrganization); f != nil {
		parts := splitStructured(f.Value)
		if len(parts) > 1 {
			rest := make([]string, 0, len(parts)-1)
			for _, s := range parts[1:] {
				rest = append(rest, compOut(strings.TrimSpace(s)))
			}
			orgRest = strings.Join(rest, ";")
		}
	}
	if company != "" || strings.Trim(orgRest, ";") != "" {
		v := compOut(company)
		if orgRest != "" {
			v += ";" + orgRest
		}
		card.SetValue(vcard.FieldOrganization, v)
	}
	if t := nonNil(p.Title); t != "" {
		card.SetValue(vcard.FieldTitle, normNewlines(t))
	}
	if n := nonNil(p.Notes); n != "" {
		card.SetValue(vcard.FieldNote, normNewlines(n))
	}
	if b := personBirthday(p); b != "" {
		card.SetValue(vcard.FieldBirthday, b)
	}
	if l := nonNil(p.Location); l != "" {
		f := &vcard.Field{Value: ";;" + compOut(l) + ";;;;", Params: vcard.Params{}}
		f.Params.Set(vcard.ParamType, "HOME")
		card.Set(vcard.FieldAddress, f)
	}

	group := 0
	addLabeled := func(key, value string, label, kind string, primary bool) {
		f := &vcard.Field{Value: normNewlines(value), Params: vcard.Params{}}
		if kind != "" {
			f.Params.Add(vcard.ParamType, kind)
		}
		if t, ok := labelToType[strings.ToLower(label)]; ok {
			f.Params.Add(vcard.ParamType, t)
		} else if label != "" {
			group++
			f.Group = fmt.Sprintf("crm%d", group)
			card.Add("X-ABLABEL", &vcard.Field{Group: f.Group, Value: normNewlines(label)})
		}
		if primary {
			f.Params.Add(vcard.ParamType, "PREF")
		}
		card.Add(key, f)
	}
	for i, e := range p.Emails {
		addLabeled(vcard.FieldEmail, e.Email, nonNil(e.Label), "INTERNET", i == 0)
	}
	for i, ph := range p.Phones {
		addLabeled(vcard.FieldTelephone, ph.PhoneNumber, nonNil(ph.Label), "", i == 0)
	}

	for _, h := range p.Handles {
		if v := strings.TrimSpace(h.Value); h.Kind == "website" && v != "" {
			card.Add(vcard.FieldURL, &vcard.Field{Value: v}) // first, so it is the website on the way back
		}
	}
	for _, h := range p.Handles {
		v := strings.TrimSpace(h.Value)
		if v == "" || h.Kind == "website" {
			continue
		}
		sp := &vcard.Field{Value: v, Params: vcard.Params{}}
		sp.Params.Set(vcard.ParamType, h.Kind)
		card.Add("X-SOCIALPROFILE", sp)
		if strings.HasPrefix(strings.ToLower(v), "http") {
			group++
			g := fmt.Sprintf("crm%d", group)
			card.Add(vcard.FieldURL, &vcard.Field{Group: g, Value: v})
			card.Add("X-ABLABEL", &vcard.Field{Group: g, Value: titleCase(h.Kind)})
		}
	}

	card.SetRevision(parseTime(p.UpdatedAt))

	for key, fields := range extra {
		if key == vcard.FieldName || key == vcard.FieldOrganization || key == vcard.FieldVersion {
			continue
		}
		for _, f := range fields {
			cp := *f
			card[key] = append(card[key], &cp)
		}
	}
	return card
}

// parseTime reads a SQLite datetime. A value that cannot be read gives the zero time.
func parseTime(s string) time.Time {
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// --- parse -------------------------------------------------------------------------------

var managedKeys = map[string]bool{
	vcard.FieldVersion: true, vcard.FieldProductID: true, vcard.FieldUID: true,
	vcard.FieldFormattedName: true, vcard.FieldName: true, vcard.FieldOrganization: true,
	vcard.FieldTitle: true, vcard.FieldNote: true, vcard.FieldBirthday: true,
	vcard.FieldEmail: true, vcard.FieldTelephone: true, vcard.FieldAddress: true,
	vcard.FieldURL: true, "X-SOCIALPROFILE": true, vcard.FieldRevision: true, "X-ABLABEL": true,
}

// Parse reads the part of a vCard this server models. The second result holds every property
// it does not model, ready to store in dav_state.extra_vcard.
func Parse(card vcard.Card) (*Contact, vcard.Card) {
	c := &Contact{}
	extra := vcard.Card{}

	groupLabel := map[string]string{}
	for _, f := range card["X-ABLABEL"] {
		if f.Group != "" {
			groupLabel[f.Group] = f.Value
		}
	}
	consumed := map[string]bool{}

	// Name.
	var fam, giv string
	if n := card.Get(vcard.FieldName); n != nil {
		parts := splitStructured(n.Value)
		fam, giv = comp(parts, 0), comp(parts, 1)
		if comp(parts, 2) != "" || comp(parts, 3) != "" || comp(parts, 4) != "" {
			extra.Set(vcard.FieldName, &vcard.Field{Value: n.Value})
		}
	}
	c.First, c.Last = giv, fam
	if c.First == "" && c.Last != "" {
		c.First, c.Last = c.Last, ""
	}

	// Organization and title.
	if o := card.Get(vcard.FieldOrganization); o != nil {
		parts := splitStructured(o.Value)
		c.Company = comp(parts, 0)
		for i := 1; i < len(parts); i++ {
			if comp(parts, i) != "" {
				extra.Set(vcard.FieldOrganization, &vcard.Field{Value: o.Value})
				break
			}
		}
	}
	c.Title = text(card.Value(vcard.FieldTitle))
	c.Notes = text(normNewlines(card.Value(vcard.FieldNote)))

	if b := card.Get(vcard.FieldBirthday); b != nil {
		if bd := parseBDay(b); bd != "" {
			c.Birthday = bd
		} else {
			extra.Add(vcard.FieldBirthday, b)
		}
	}

	// Location: the first address maps, the others are kept.
	for i, f := range card[vcard.FieldAddress] {
		if i == 0 {
			c.Location = locationFromAdr(f)
			continue
		}
		extra.Add(vcard.FieldAddress, f)
	}

	// Emails and phones.
	seenE, seenP := map[string]bool{}, map[string]bool{}
	for _, f := range card[vcard.FieldEmail] {
		v := text(strings.TrimPrefix(strings.TrimPrefix(f.Value, "mailto:"), "MAILTO:"))
		k := emailKey(v)
		if k == "" {
			extra.Add(vcard.FieldEmail, f)
			continue
		}
		if f.Group != "" {
			consumed[f.Group] = true
		}
		if seenE[k] {
			continue
		}
		seenE[k] = true
		c.Emails = append(c.Emails, Item{Value: v, Label: labelFromField(f, groupLabel)})
	}
	for _, f := range card[vcard.FieldTelephone] {
		v := text(strings.TrimPrefix(strings.TrimPrefix(f.Value, "tel:"), "TEL:"))
		if v == "" {
			continue
		}
		if f.Group != "" {
			consumed[f.Group] = true
		}
		k := phoneKey(v)
		if seenP[k] {
			continue
		}
		seenP[k] = true
		c.Phones = append(c.Phones, Item{Value: v, Label: labelFromField(f, groupLabel)})
	}

	// Websites and social profiles.
	c.Handles = map[string]string{}
	for _, f := range card["X-SOCIALPROFILE"] {
		kind := ""
		for _, t := range f.Params.Types() {
			if k, ok := socialKinds[t]; ok {
				kind = k
				break
			}
		}
		if kind == "" {
			kind = "other"
		}
		v := text(f.Value)
		if v == "" {
			v = text(f.Params.Get("X-USER"))
		}
		if _, dup := c.Handles[kind]; v == "" || dup {
			extra.Add("X-SOCIALPROFILE", f)
			continue
		}
		c.Handles[kind] = v
	}
	var fallbacks []*vcard.Field
	websiteTaken := false
	for _, f := range card[vcard.FieldURL] {
		v := text(f.Value)
		if f.Group != "" {
			if kind, ok := socialKinds[strings.ToLower(cleanABLabel(groupLabel[f.Group]))]; ok && v != "" {
				consumed[f.Group] = true
				if _, have := c.Handles[kind]; !have {
					fallbacks = append(fallbacks, f)
				}
				continue
			}
		}
		if !websiteTaken && v != "" {
			c.Handles["website"] = v
			websiteTaken = true
			continue
		}
		extra.Add(vcard.FieldURL, f)
	}
	for _, f := range fallbacks {
		kind := socialKinds[strings.ToLower(cleanABLabel(groupLabel[f.Group]))]
		if _, have := c.Handles[kind]; !have {
			c.Handles[kind] = text(f.Value)
		}
	}
	if len(c.Handles) == 0 {
		c.Handles = nil
	}

	// Name fallbacks for contacts that came without N.
	if c.First == "" {
		if fn := card.Get(vcard.FieldFormattedName); fn != nil && text(fn.Value) != "" {
			words := strings.Fields(text(fn.Value))
			if len(words) == 1 {
				c.First = words[0]
			} else {
				c.First, c.Last = strings.Join(words[:len(words)-1], " "), words[len(words)-1]
			}
		}
	}
	if c.First == "" {
		switch {
		case c.Company != "":
			c.First = c.Company
		case len(c.Emails) > 0:
			c.First = c.Emails[0].Value
		case len(c.Phones) > 0:
			c.First = c.Phones[0].Value
		}
	}

	// Everything else is kept as it came.
	for key, fields := range card {
		if managedKeys[key] {
			if key == "X-ABLABEL" {
				for _, f := range fields {
					if f.Group == "" || !consumed[f.Group] {
						extra.Add(key, f)
					}
				}
			}
			continue
		}
		for _, f := range fields {
			extra.Add(key, f)
		}
	}
	// Groups of kept fields keep their X-ABLabel next to them already. Drop labels of groups
	// that were consumed, which the loop above skips.
	if len(extra) == 0 {
		return c, nil
	}
	return c, extra
}

// EncodeExtra serializes kept properties for dav_state.extra_vcard. It returns "" when there are none.
func EncodeExtra(extra vcard.Card) string {
	if len(extra) == 0 {
		return ""
	}
	cp := vcard.Card{}
	for k, v := range extra {
		cp[k] = v
	}
	cp.SetValue(vcard.FieldVersion, "3.0")
	var buf bytes.Buffer
	if err := vcard.NewEncoder(&buf).Encode(cp); err != nil {
		return ""
	}
	return buf.String()
}

// DecodeExtra reads dav_state.extra_vcard. Broken or empty text gives no properties.
func DecodeExtra(s *string) vcard.Card {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	card, err := vcard.NewDecoder(strings.NewReader(*s)).Decode()
	if err != nil {
		return nil
	}
	delete(card, vcard.FieldVersion)
	return card
}
