package dav

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/emersion/go-vcard"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Where the stored card came from.
const (
	rawSourceDevice = "device" // the phone's own PUT
	rawSourceServer = "server" // rendered by the CRM
)

type rawKey struct{}

// withRaw carries the exact bytes of a PUT body to Service.Put.
func withRaw(ctx context.Context, raw string) context.Context {
	return context.WithValue(ctx, rawKey{}, raw)
}

func rawFrom(ctx context.Context) string {
	v, _ := ctx.Value(rawKey{}).(string)
	return v
}

// patchStored edits the stored card of a person to match the CRM view now. It returns false when
// there is no usable stored card or the edit did not reproduce the view, and the caller renders a
// fresh card instead.
func patchStored(st *model.DavState, rendered vcard.Card, updatedAt string) (string, bool) {
	if st == nil || st.RawVCard == nil || st.BaseJSON == nil || st.RawSource == nil || strings.TrimSpace(*st.RawVCard) == "" {
		return "", false
	}
	view, _ := Parse(rendered)
	patched, _, err := patchRaw(*st.RawVCard, ParseBase(st.BaseJSON), view, updatedAt, *st.RawSource == rawSourceServer)
	if err != nil {
		return "", false
	}
	c, err := decodeCard(patched)
	if err != nil {
		return "", false
	}
	if got, _ := Parse(c); !got.Equal(view) {
		return "", false
	}
	return patched, true
}

// previousKeys are the email and phone keys of the stored card, nil when there is none. A list item
// missing from an incoming card is only a delete when the stored card held it.
func previousKeys(st *model.DavState) (emails, phones map[string]bool) {
	if st == nil || st.RawVCard == nil || strings.TrimSpace(*st.RawVCard) == "" {
		return nil, nil
	}
	return rawKeys(*st.RawVCard, vcard.FieldEmail), rawKeys(*st.RawVCard, vcard.FieldTelephone)
}

// This file patches a stored vCard instead of rendering a new one (research-sync-standard section 2,
// owner decisions Q14 to Q16). The phone's own text is kept: a property the CRM did not change
// stays the exact bytes it was, in the same place, with its group, folding, unknown parameters and
// X- neighbours (itemN.X-ABLabel). Only the properties whose field moved since the last sync are
// edited, and PRODID, UID and VERSION are never touched. REV is only touched on a card the CRM
// itself rendered.

// rawProp is one logical line of a stored vCard together with the exact text it came from.
type rawProp struct {
	// Text is the original text of the line, folds and line ending included.
	Text string
	// Unfolded is the logical line without its line ending.
	Unfolded string
	Group    string
	Name     string // upper case
	// Params is the raw text between the name and the colon, with its leading semicolon.
	Params string
	Value  string // raw (still escaped) value
}

// rawCard is a stored vCard as an ordered list of lines.
type rawCard struct {
	Props []rawProp
	EOL   string
}

var errRawCard = errors.New("vcard text cannot be read")

// parseLogical splits an unfolded line into group, name, parameters and value.
func parseLogical(line string) (group, name, params, value string, ok bool) {
	colon := -1
	quoted := false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '"':
			quoted = !quoted
		case ':':
			if !quoted {
				colon = i
			}
		}
		if colon >= 0 {
			break
		}
	}
	if colon <= 0 {
		return "", "", "", "", false
	}
	head, value := line[:colon], line[colon+1:]
	nameGroup := head
	if i := strings.IndexByte(head, ';'); i >= 0 {
		nameGroup, params = head[:i], head[i:]
	}
	name = nameGroup
	if i := strings.IndexByte(nameGroup, '.'); i >= 0 {
		group, name = nameGroup[:i], nameGroup[i+1:]
	}
	if name == "" {
		return "", "", "", "", false
	}
	return group, strings.ToUpper(name), params, value, true
}

// parseRaw reads vCard text into lines, keeping every byte.
func parseRaw(text string) (*rawCard, error) {
	rc := &rawCard{EOL: "\r\n"}
	if i := strings.IndexByte(text, '\n'); i >= 0 && (i == 0 || text[i-1] != '\r') {
		rc.EOL = "\n"
	}
	rest := text
	for len(rest) > 0 {
		var line string
		if i := strings.IndexByte(rest, '\n'); i < 0 {
			line, rest = rest, ""
		} else {
			line, rest = rest[:i+1], rest[i+1:]
		}
		body := strings.TrimRight(line, "\r\n")
		if len(rc.Props) > 0 && body != "" && (body[0] == ' ' || body[0] == '\t') {
			last := &rc.Props[len(rc.Props)-1]
			last.Text += line
			last.Unfolded += body[1:]
			continue
		}
		rc.Props = append(rc.Props, rawProp{Text: line, Unfolded: body})
	}
	for i := range rc.Props {
		p := &rc.Props[i]
		if p.Unfolded == "" {
			continue
		}
		g, n, params, v, ok := parseLogical(p.Unfolded)
		if !ok {
			return nil, fmt.Errorf("%w: line %q", errRawCard, truncate(p.Unfolded, 40))
		}
		p.Group, p.Name, p.Params, p.Value = g, n, params, v
	}
	first, last := -1, -1
	for i, p := range rc.Props {
		if p.Name != "" {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 || rc.Props[first].Name != "BEGIN" || !strings.EqualFold(rc.Props[first].Value, "VCARD") ||
		rc.Props[last].Name != "END" {
		return nil, fmt.Errorf("%w: no BEGIN:VCARD and END:VCARD", errRawCard)
	}
	return rc, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// String is the card text. A card that was not edited comes back byte for byte.
func (rc *rawCard) String() string {
	var b strings.Builder
	for _, p := range rc.Props {
		b.WriteString(p.Text)
	}
	return b.String()
}

// field decodes one line with go-vcard, so values are unescaped the way Parse sees them.
func (p rawProp) field() *vcard.Field {
	text := "BEGIN:VCARD\r\nVERSION:3.0\r\n" + p.Unfolded + "\r\nEND:VCARD\r\n"
	card, err := vcard.NewDecoder(strings.NewReader(text)).Decode()
	if err != nil {
		return &vcard.Field{Value: p.Value, Group: p.Group}
	}
	if f := card.Get(p.Name); f != nil {
		return f
	}
	return &vcard.Field{Value: p.Value, Group: p.Group}
}

var valueEscaper = strings.NewReplacer("\\", "\\\\", "\n", "\\n", ",", "\\,")

// line builds a property line the way the go-vcard encoder would write it.
func (rc *rawCard) line(group, name, params, value string) rawProp {
	head := name
	if group != "" {
		head = group + "." + name
	}
	unfolded := head + params + ":" + valueEscaper.Replace(value)
	return rawProp{Text: unfolded + rc.EOL, Unfolded: unfolded, Group: group, Name: name, Params: params, Value: valueEscaper.Replace(value)}
}

// index returns the positions of the lines with a name.
func (rc *rawCard) index(name string) []int {
	var out []int
	for i, p := range rc.Props {
		if p.Name == name {
			out = append(out, i)
		}
	}
	return out
}

// endIndex is where new lines go: before END:VCARD.
func (rc *rawCard) endIndex() int {
	for i := len(rc.Props) - 1; i >= 0; i-- {
		if rc.Props[i].Name == "END" {
			return i
		}
	}
	return len(rc.Props)
}

func (rc *rawCard) insert(at int, ps ...rawProp) {
	out := make([]rawProp, 0, len(rc.Props)+len(ps))
	out = append(out, rc.Props[:at]...)
	out = append(out, ps...)
	out = append(out, rc.Props[at:]...)
	rc.Props = out
}

func (rc *rawCard) remove(i int) {
	rc.Props = append(rc.Props[:i:i], rc.Props[i+1:]...)
}

// setValue changes the value of line i and keeps its group and parameters.
func (rc *rawCard) setValue(i int, value string) {
	p := rc.Props[i]
	rc.Props[i] = rc.line(p.Group, p.Name, p.Params, value)
}

// setProp sets the single property name: the first line is edited in place, an empty value removes
// the line, a missing line is added before END.
func (rc *rawCard) setProp(name, value string) {
	idx := rc.index(name)
	switch {
	case value == "" && len(idx) > 0:
		rc.removeWithLabel(idx[0])
	case value == "":
	case len(idx) > 0:
		if rc.fieldValue(idx[0]) != value {
			rc.setValue(idx[0], value)
		}
	default:
		rc.insert(rc.endIndex(), rc.line("", name, "", value))
	}
}

func (rc *rawCard) fieldValue(i int) string { return rc.Props[i].field().Value }

// removeWithLabel removes line i and the X-ABLabel of its group when nothing else uses the group.
func (rc *rawCard) removeWithLabel(i int) {
	group := rc.Props[i].Group
	rc.remove(i)
	if group == "" {
		return
	}
	used := false
	for _, p := range rc.Props {
		if p.Group == group && p.Name != "X-ABLABEL" {
			used = true
		}
	}
	if used {
		return
	}
	for j := len(rc.Props) - 1; j >= 0; j-- {
		if rc.Props[j].Group == group && rc.Props[j].Name == "X-ABLABEL" {
			rc.remove(j)
		}
	}
}

// freeGroup returns a group name not used by any line.
func (rc *rawCard) freeGroup() string {
	used := map[string]bool{}
	for _, p := range rc.Props {
		if p.Group != "" {
			used[strings.ToLower(p.Group)] = true
		}
	}
	for n := 1; ; n++ {
		g := fmt.Sprintf("crm%d", n)
		if !used[g] {
			return g
		}
	}
}

// --- patch -------------------------------------------------------------------------------

// patchRaw returns the stored card with only the fields that moved from base (what the phone last
// held) to view (what the CRM holds now) edited. changed is false and the text is the stored text
// byte for byte when nothing moved. serverRendered says the CRM wrote this card, so REV follows.
func patchRaw(raw string, base, view *Contact, updatedAt string, serverRendered bool) (string, bool, error) {
	rc, err := parseRaw(raw)
	if err != nil {
		return "", false, err
	}
	changed := false

	bs, vs := base.scalars(), view.scalars()
	moved := func(f string) bool { return bs[f] != vs[f] }
	if moved(fieldFirst) || moved(fieldLast) {
		rc.patchName(view.First, view.Last)
		changed = true
	}
	if moved(fieldCompany) {
		rc.patchOrg(view.Company)
		changed = true
	}
	if moved(fieldTitle) {
		rc.setProp(vcard.FieldTitle, view.Title)
		changed = true
	}
	if moved(fieldNotes) {
		rc.setProp(vcard.FieldNote, normNewlines(view.Notes))
		changed = true
	}
	if moved(fieldLocation) {
		rc.patchLocation(view.Location)
		changed = true
	}
	if moved(fieldBirthday) {
		rc.setProp(vcard.FieldBirthday, view.Birthday)
		changed = true
	}
	if rc.patchItems(vcard.FieldEmail, base.Emails, view.Emails, emailKey) {
		changed = true
	}
	if rc.patchItems(vcard.FieldTelephone, base.Phones, view.Phones, phoneKey) {
		changed = true
	}
	if rc.patchHandles(base.Handles, view.Handles) {
		changed = true
	}
	if !changed {
		return raw, false, nil
	}
	if serverRendered {
		rc.setProp(vcard.FieldRevision, parseTime(updatedAt).UTC().Format("20060102T150405Z"))
	}
	return rc.String(), true, nil
}

// patchName edits N and FN. The additional names, prefix and suffix of the card stay.
func (rc *rawCard) patchName(given, family string) {
	var additional, prefix, suffix string
	if idx := rc.index(vcard.FieldName); len(idx) > 0 {
		parts := splitStructured(rc.Props[idx[0]].field().Value)
		additional, prefix, suffix = comp(parts, 2), comp(parts, 3), comp(parts, 4)
	}
	n := strings.Join([]string{compOut(family), compOut(given), compOut(additional), compOut(prefix), compOut(suffix)}, ";")
	rc.setStructured(vcard.FieldName, n)
	var fn []string
	for _, s := range []string{prefix, given, additional, family, suffix} {
		if s != "" {
			fn = append(fn, s)
		}
	}
	text := strings.Join(fn, " ")
	if text == "" {
		text = "Unknown"
	}
	rc.setProp(vcard.FieldFormattedName, text)
}

// setStructured is setProp for a value that holds unescaped semicolons between components.
func (rc *rawCard) setStructured(name, value string) {
	idx := rc.index(name)
	if len(idx) == 0 {
		rc.insert(rc.endIndex(), rc.line("", name, "", value))
		return
	}
	if rc.Props[idx[0]].field().Value != value {
		rc.setValue(idx[0], value)
	}
}

// patchOrg changes the first ORG component and keeps the units after it.
func (rc *rawCard) patchOrg(company string) {
	idx := rc.index(vcard.FieldOrganization)
	rest := ""
	if len(idx) > 0 {
		parts := splitStructured(rc.Props[idx[0]].field().Value)
		if len(parts) > 1 {
			r := make([]string, 0, len(parts)-1)
			for _, s := range parts[1:] {
				r = append(r, compOut(strings.TrimSpace(s)))
			}
			rest = strings.Join(r, ";")
		}
	}
	if company == "" && strings.Trim(rest, ";") == "" {
		if len(idx) > 0 {
			rc.removeWithLabel(idx[0])
		}
		return
	}
	v := compOut(company)
	if rest != "" {
		v += ";" + rest
	}
	rc.setStructured(vcard.FieldOrganization, v)
}

// patchLocation edits the first ADR and leaves later addresses alone.
func (rc *rawCard) patchLocation(location string) {
	idx := rc.index(vcard.FieldAddress)
	if location == "" {
		if len(idx) > 0 {
			rc.removeWithLabel(idx[0])
		}
		return
	}
	v := ";;" + compOut(location) + ";;;;"
	if len(idx) > 0 {
		rc.setValue(idx[0], v)
		return
	}
	rc.insert(rc.endIndex(), rc.line("", vcard.FieldAddress, ";TYPE=HOME", v))
}

// labelLines builds the lines of one TEL or EMAIL item the way Render does.
func (rc *rawCard) labelLines(name string, it Item, internet bool) []rawProp {
	params := ""
	if internet {
		params += ";TYPE=INTERNET"
	}
	group := ""
	var extra []rawProp
	if t, ok := labelToType[strings.ToLower(it.Label)]; ok {
		params += ";TYPE=" + t
	} else if it.Label != "" {
		group = rc.freeGroup()
		extra = append(extra, rc.line(group, "X-ABLABEL", "", normNewlines(it.Label)))
	}
	main := rc.line(group, name, params, normNewlines(it.Value))
	return append([]rawProp{main}, extra...)
}

// patchItems applies the difference between base and view of a list (TEL or EMAIL) by item key.
func (rc *rawCard) patchItems(name string, base, view []Item, key func(string) string) bool {
	b, v := indexItems(base, key), indexItems(view, key)
	changed := false
	// Removed: gone from the CRM since the last sync.
	for k := range b {
		if _, still := v[k]; still {
			continue
		}
		for i := len(rc.Props) - 1; i >= 0; i-- {
			p := rc.Props[i]
			if p.Name == name && key(itemText(name, p)) == k {
				rc.removeWithLabel(i)
				changed = true
			}
		}
	}
	// Added or relabelled, in the CRM's order.
	for _, it := range view {
		k := key(it.Value)
		if k == "" {
			continue
		}
		old, had := b[k]
		if !had {
			if rc.hasItem(name, k, key) {
				continue // the phone already holds it
			}
			rc.insert(rc.endIndex(), rc.labelLines(name, it, name == vcard.FieldEmail)...)
			changed = true
			continue
		}
		if !strings.EqualFold(old.Label, it.Label) && strings.TrimSpace(it.Label) != "" {
			for i := range rc.Props {
				p := rc.Props[i]
				if p.Name == name && key(itemText(name, p)) == k {
					lines := rc.labelLines(name, Item{Value: itemText(name, p), Label: it.Label}, name == vcard.FieldEmail)
					// Keep the original value text of the line, replace its label.
					lines[0].Text, lines[0].Unfolded = rebuildValue(lines[0], p)
					rc.removeWithLabel(i)
					rc.insert(i, lines...)
					changed = true
					break
				}
			}
		}
	}
	return changed
}

// rebuildValue keeps the original value bytes of old on the new line.
func rebuildValue(n, old rawProp) (string, string) {
	head := n.Name
	if n.Group != "" {
		head = n.Group + "." + n.Name
	}
	unfolded := head + n.Params + ":" + old.Value
	eol := "\r\n"
	if strings.HasSuffix(n.Text, "\n") && !strings.HasSuffix(n.Text, "\r\n") {
		eol = "\n"
	}
	return unfolded + eol, unfolded
}

func (rc *rawCard) hasItem(name, k string, key func(string) string) bool {
	for _, p := range rc.Props {
		if p.Name == name && key(itemText(name, p)) == k {
			return true
		}
	}
	return false
}

// itemText is the value of a TEL or EMAIL line as Parse reads it.
func itemText(name string, p rawProp) string {
	v := p.field().Value
	if name == vcard.FieldEmail {
		return text(strings.TrimPrefix(strings.TrimPrefix(v, "mailto:"), "MAILTO:"))
	}
	return text(strings.TrimPrefix(strings.TrimPrefix(v, "tel:"), "TEL:"))
}

// patchHandles applies changed website and social profile values.
func (rc *rawCard) patchHandles(base, view map[string]string) bool {
	kinds := map[string]bool{}
	for k := range base {
		kinds[k] = true
	}
	for k := range view {
		kinds[k] = true
	}
	changed := false
	for _, kind := range sortedKeys(kinds) {
		b, v := base[kind], view[kind]
		if b == v || (b != "" && v != "" && handleKey(b) == handleKey(v)) {
			continue
		}
		changed = true
		if kind == "website" {
			rc.patchWebsite(b, v)
			continue
		}
		rc.patchSocial(kind, b, v)
	}
	return changed
}

func (rc *rawCard) patchWebsite(old, value string) {
	for _, i := range rc.index(vcard.FieldURL) {
		p := rc.Props[i]
		if p.Group != "" && rc.groupIsSocial(p.Group) {
			continue
		}
		if text(p.field().Value) == "" {
			continue
		}
		if value == "" {
			rc.removeWithLabel(i)
		} else {
			rc.setValue(i, value)
		}
		return
	}
	if value != "" {
		rc.insert(rc.endIndex(), rc.line("", vcard.FieldURL, "", value))
	}
}

func (rc *rawCard) groupIsSocial(group string) bool {
	for _, p := range rc.Props {
		if p.Group == group && p.Name == "X-ABLABEL" {
			_, ok := socialKinds[strings.ToLower(cleanABLabel(p.field().Value))]
			return ok
		}
	}
	return false
}

func (rc *rawCard) patchSocial(kind, old, value string) {
	for _, i := range rc.index("X-SOCIALPROFILE") {
		p := rc.Props[i]
		f := p.field()
		k := ""
		for _, t := range f.Params.Types() {
			if m, ok := socialKinds[t]; ok {
				k = m
				break
			}
		}
		if k == "" {
			k = "other"
		}
		if k != kind {
			continue
		}
		if value == "" {
			rc.removeWithLabel(i)
		} else {
			rc.setValue(i, value)
		}
		rc.patchSocialURL(kind, value)
		return
	}
	if value == "" {
		rc.patchSocialURL(kind, value)
		return
	}
	rc.insert(rc.endIndex(), rc.line("", "X-SOCIALPROFILE", ";TYPE="+kind, value))
	if strings.HasPrefix(strings.ToLower(value), "http") {
		g := rc.freeGroup()
		rc.insert(rc.endIndex(), rc.line(g, vcard.FieldURL, "", value), rc.line(g, "X-ABLABEL", "", titleCase(kind)))
	}
}

// patchSocialURL keeps the grouped URL fallback of a social profile in step.
func (rc *rawCard) patchSocialURL(kind, value string) {
	for _, i := range rc.index(vcard.FieldURL) {
		p := rc.Props[i]
		if p.Group == "" {
			continue
		}
		for _, l := range rc.Props {
			if l.Group == p.Group && l.Name == "X-ABLABEL" && socialKinds[strings.ToLower(cleanABLabel(l.field().Value))] == kind {
				if value == "" {
					rc.removeWithLabel(i)
				} else {
					rc.setValue(i, value)
				}
				return
			}
		}
	}
}

// encodeCard writes a card with the go-vcard encoder, for cards that have no stored text yet.
func encodeCard(card vcard.Card) string {
	var buf bytes.Buffer
	if err := vcard.NewEncoder(&buf).Encode(card); err != nil {
		return ""
	}
	return buf.String()
}

// decodeCard reads vCard text with go-vcard.
func decodeCard(text string) (vcard.Card, error) {
	return vcard.NewDecoder(strings.NewReader(text)).Decode()
}

// rawKeys lists the item keys (TEL or EMAIL) a stored card holds. Deletes are only honoured for these.
func rawKeys(raw string, name string) map[string]bool {
	out := map[string]bool{}
	card, err := decodeCard(raw)
	if err != nil {
		return out
	}
	c, _ := Parse(card)
	items, key := c.Phones, phoneKey
	if name == vcard.FieldEmail {
		items, key = c.Emails, emailKey
	}
	for _, it := range items {
		out[key(it.Value)] = true
	}
	return out
}

