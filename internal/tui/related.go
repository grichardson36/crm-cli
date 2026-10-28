package tui

import (
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	sg "github.com/jdanielnd/crm-cli/internal/suggest"
)

// The related segment (owner decision 2026-10-29, patterns/related-segment). Every add line ends with
// ONE segment, "related", that takes objects of every kind the relation registry joins to the new
// object. Its suggestions come from the context the user is in, not only from the text typed:
//
//	1. the object the user is on or has highlighted (relAnchor)
//	2. that object's directly linked objects of every kind (relDirect)
//	3. once the user chose related objects, the objects linked to those (one hop) (relHop)
//	4. objects used recently in this session (relRecent)
//
// Typing narrows the whole candidate set by name with the shared ranker (nickname aliases, typo
// tolerance). Context candidates come first, so on equal matches they win. A chosen object is never
// suggested again, and only the kinds in the segment (the registry's answer for the new object) are.
// The popup groups the rows by kind under dim headings and the chosen objects are drawn as chips
// grouped by kind (pipeview.go).

// relatedKindOrder is the stable order of kinds in the popup headings and in the chips.
var relatedKindOrder = []string{"person", "organization", "deal", "task", model.NoteKind, tagKind}

// kindHeading is the dim heading of a kind in the popup.
var kindHeading = map[string]string{"person": "People", "organization": "Organizations", "deal": "Deals", "task": "Tasks", model.NoteKind: "Notes", tagKind: "Tags"}

// kindPrefixes are the short prefixes that name a kind without color: p:, o:, d:, t:, n:, #.
var kindPrefixes = map[string]string{"person": "p:", "organization": "o:", "deal": "d:", "task": "t:", model.NoteKind: "n:", tagKind: "#"}

// kindPrefix is the short prefix of a kind, "" for a kind without one.
func kindPrefix(kind string) string { return kindPrefixes[kind] }

// kindOrderIndex is the position of a kind in relatedKindOrder. Unknown kinds go last.
func kindOrderIndex(kind string) int {
	for i, k := range relatedKindOrder {
		if k == kind {
			return i
		}
	}
	return len(relatedKindOrder)
}

// relatedMaxRows is the most rows the related popup holds.
const relatedMaxRows = 12

// relatedKindsOf are the kinds a new object of objKind may be related to: every kind the relation
// registry joins to it, in the stable order, then tags for an object that carries tags.
func relatedKindsOf(objKind string) []string {
	var out []string
	for _, k := range relatedKindOrder {
		if k == tagKind {
			if model.ValidTaggable(objKind) {
				out = append(out, k)
			}
			continue
		}
		if _, ok := model.RelationBetween(objKind, k); ok {
			out = append(out, k)
		}
	}
	return out
}

// relatedSeg is the one related segment of an add line, or false when the object relates to nothing.
func relatedSeg(objKind string) (pipeSeg, bool) {
	kinds := relatedKindsOf(objKind)
	if len(kinds) == 0 {
		return pipeSeg{}, false
	}
	return pipeSeg{Key: "related", Label: "related", Link: true, Related: true, Kinds: kinds,
		Spec: model.FieldSpec{Key: "related", Label: "related", Type: model.FieldRelation}}, true
}

// relTier says why a candidate is suggested. A lower tier ranks first.
type relTier int

const (
	relAnchor relTier = iota
	relDirect
	relHop
	relRecent
	relOther
)

// relCand is one suggested object or tag.
type relCand struct {
	ref   repo.Ref // zero for a tag
	tag   string
	label string
	desc  string
	tier  relTier
}

func (c relCand) key() string {
	if c.tag != "" {
		return "tag/" + strings.ToLower(c.tag)
	}
	return c.ref.String()
}

func (c relCand) kind() string {
	if c.tag != "" {
		return tagKind
	}
	return c.ref.Kind
}

// relAnchorInfo is the object the user is on: what the add flow started from, or the page the prompt
// belongs to. implicit says the new object is already linked to it (a task added on a record page), so
// it is not suggested but its links are.
type relAnchorInfo struct {
	ref      repo.Ref
	tag      string
	label    string
	implicit bool
}

// relatedAnchor finds the object the user is on. The highlighted object of the add picker wins. A prompt
// opened on a record page uses that page. Otherwise the object under the cursor is it.
func (m *Model) relatedAnchor(s *pipeSession) (relAnchorInfo, bool) {
	if h := m.addSugg; h != nil {
		if h.tag != "" {
			return relAnchorInfo{tag: h.tag, label: h.tag}, true
		}
		if h.ref.ID != 0 {
			return relAnchorInfo{ref: h.ref, label: h.label}, true
		}
	}
	switch s.kind {
	case "task-page", "note-object":
		if ref := m.pageRef(); ref.ID != 0 {
			return relAnchorInfo{ref: ref, label: m.refName(ref), implicit: true}, true
		}
	case "task", "note":
		if pid := m.curPerson(); pid != 0 {
			ref := repo.Ref{Kind: "person", ID: pid}
			return relAnchorInfo{ref: ref, label: m.refName(ref), implicit: true}, true
		}
	case personAddKind:
		return relAnchorInfo{}, false
	}
	if ref, ok := m.cursorObject(); ok && suggestableKind(ref.Kind) {
		return relAnchorInfo{ref: ref, label: m.refName(ref)}, true
	}
	return relAnchorInfo{}, false
}

// itemOf finds the live object of a kind by id in the session's candidates.
func (m *Model) itemOf(s *pipeSession, kind string, id int64) (overlayItem, bool) {
	if s.idx == nil {
		s.idx = map[string]map[int64]overlayItem{}
	}
	byID, ok := s.idx[kind]
	if !ok {
		byID = map[int64]overlayItem{}
		for _, it := range m.pipeCandidates(s, kind) {
			byID[it.ref] = it
		}
		s.idx[kind] = byID
	}
	it, ok := byID[id]
	return it, ok
}

// linkedCands are the objects directly linked to ref, of the kinds the segment takes, and the tags ref
// carries when the segment takes tags. The links come from the repository, both ways round.
func (m *Model) linkedCands(s *pipeSession, ref repo.Ref, kinds []string, tier relTier) []relCand {
	if s.direct == nil {
		s.direct = map[string][]relCand{}
	}
	key := ref.String() + "|" + strings.Join(kinds, ",")
	if c, ok := s.direct[key]; ok {
		return withTier(c, tier)
	}
	var out []relCand
	for _, k := range kinds {
		if k == tagKind {
			if model.ValidTaggable(ref.Kind) && m.tagr != nil {
				if tags, err := m.tagr.GetForEntity(m.ctx, ref.Kind, ref.ID); err == nil {
					for _, t := range tags {
						out = append(out, relCand{tag: t.Name, label: t.Name})
					}
				}
			}
			continue
		}
		if _, ok := model.RelationBetween(ref.Kind, k); !ok || m.links == nil {
			continue
		}
		items, err := m.links.Linked(m.ctx, ref, k)
		if err != nil {
			continue
		}
		for _, li := range items {
			it, live := m.itemOf(s, k, li.ID)
			if !live {
				continue
			}
			out = append(out, relCand{ref: repo.Ref{Kind: k, ID: li.ID}, label: it.label, desc: it.desc})
		}
	}
	s.direct[key] = out
	return withTier(out, tier)
}

func withTier(in []relCand, tier relTier) []relCand {
	out := make([]relCand, len(in))
	for i, c := range in {
		c.tier = tier
		out[i] = c
	}
	return out
}

// recentCands are the objects picked in the bar this session, newest first, as candidates.
func (m *Model) recentCands(s *pipeSession, kinds []string) []relCand {
	var out []relCand
	allowed := map[string]bool{}
	for _, k := range kinds {
		allowed[k] = true
	}
	for _, it := range m.recent {
		t, ok := barTargetOf(it)
		if !ok {
			continue
		}
		if t.kind == "tag" {
			if allowed[tagKind] {
				out = append(out, relCand{tag: t.name, label: t.name, tier: relRecent})
			}
			continue
		}
		kind := barKindAddKind(t.kind)
		if !allowed[kind] {
			continue
		}
		if li, live := m.itemOf(s, kind, t.id); live {
			out = append(out, relCand{ref: repo.Ref{Kind: kind, ID: t.id}, label: li.label, desc: li.desc, tier: relRecent})
		}
	}
	return out
}

// chosenLink is an object the line already holds.
type chosenLink struct {
	ref   repo.Ref
	tag   string
	label string
}

func (c chosenLink) key() string { return relCand{ref: c.ref, tag: c.tag}.key() }

func (c chosenLink) kind() string {
	if c.tag != "" {
		return tagKind
	}
	return c.ref.Kind
}

// chosenLinks are the names of the line that stand for an object or a tag, in line order. The name under
// the cursor (skipTok in segment skipSeg) is still being typed and is left out. Names that stand for
// nothing yet are returned in typed so they are not suggested again either.
func (m *Model) chosenLinks(s *pipeSession, line, skipTok string, skipSeg int) (chosen []chosenLink, typed map[string]bool) {
	typed = map[string]bool{}
	seen := map[string]bool{}
	for _, t := range s.pl.tokens(line) {
		if t.Seg == skipSeg && skipTok != "" && strings.EqualFold(strings.TrimPrefix(t.Text, "#"), skipTok) {
			continue
		}
		res := m.autoResolve(s, s.pl.segs[t.Seg], t.Text)
		if res.state != resolved {
			typed[strings.ToLower(strings.TrimPrefix(t.Text, "#"))] = true
			continue
		}
		c := chosenLink{ref: res.link.ref, tag: res.link.tag, label: strings.TrimPrefix(res.link.label, "#")}
		if c.ref.ID == 0 && c.tag == "" { // a name that will be created
			typed[strings.ToLower(c.label)] = true
			continue
		}
		if seen[c.key()] {
			continue
		}
		seen[c.key()] = true
		chosen = append(chosen, c)
	}
	return
}

// relatedCands lists the context candidates of a link segment in rank order: the object the user is
// on, its links, the links of what is already chosen, then recent objects. Nothing chosen is listed, and
// only kinds of the segment are.
func (m *Model) relatedCands(s *pipeSession, seg pipeSeg, chosen []chosenLink, typed map[string]bool) []relCand {
	skip := map[string]bool{}
	for _, c := range chosen {
		skip[c.key()] = true
	}
	var out []relCand
	add := func(cs []relCand) {
		for _, c := range cs {
			if skip[c.key()] || typed[strings.ToLower(c.label)] || !segTakes(seg, c.kind()) {
				continue
			}
			skip[c.key()] = true
			out = append(out, c)
		}
	}
	if a, ok := m.relatedAnchor(s); ok {
		if a.tag != "" {
			add([]relCand{{tag: a.tag, label: a.tag, desc: "highlighted", tier: relAnchor}})
		} else {
			if !a.implicit {
				c := relCand{ref: a.ref, label: a.label, desc: "highlighted", tier: relAnchor}
				if it, live := m.itemOf(s, a.ref.Kind, a.ref.ID); live {
					c.label = it.label
				}
				add([]relCand{c})
			}
			add(m.linkedCands(s, a.ref, seg.Kinds, relDirect))
		}
	}
	for _, c := range chosen {
		if c.tag != "" {
			continue
		}
		add(m.linkedCands(s, c.ref, seg.Kinds, relHop))
	}
	add(m.recentCands(s, seg.Kinds))
	return out
}

// segTakes says whether the segment takes objects of a kind.
func segTakes(seg pipeSeg, kind string) bool {
	for _, k := range seg.Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// pipeLinkRows are the rows of a link segment for the name being typed (tok), given the whole line.
// With nothing typed the context candidates are the rows. With text, every object of the segment's kinds
// is ranked by name, context candidates first so they win ties. A segment of several kinds groups its
// rows by kind under headings (relatedGroup). A name that starts with # is looked up among the tags only.
func (m *Model) pipeLinkRows(s *pipeSession, seg pipeSeg, tok, line string, curSeg int) []Suggestion {
	if strings.HasPrefix(tok, "#") && segTakes(seg, tagKind) { // a # narrows the rows to tags
		seg.Kinds = []string{tagKind}
	}
	tok = strings.TrimSpace(strings.TrimPrefix(tok, "#"))
	chosen, typed := m.chosenLinks(s, line, tok, curSeg)
	ctx := m.relatedCands(s, seg, chosen, typed)
	multi := len(seg.Kinds) > 1
	var picked []relCand
	if tok == "" {
		picked = ctx
		if len(picked) > relatedMaxRows {
			picked = picked[:relatedMaxRows]
		}
	} else {
		skip := map[string]bool{}
		for _, c := range chosen {
			skip[c.key()] = true
		}
		all := append([]relCand(nil), ctx...)
		for _, c := range ctx {
			skip[c.key()] = true
		}
		for _, k := range seg.Kinds {
			for _, it := range m.pipeCandidates(s, k) {
				c := relCand{label: it.label, desc: it.desc, tier: relOther}
				if k == tagKind {
					c.tag = it.label
				} else {
					c.ref = repo.Ref{Kind: k, ID: it.ref}
				}
				if skip[c.key()] || typed[strings.ToLower(c.label)] {
					continue
				}
				all = append(all, c)
			}
		}
		picked = sg.Rank(all, tok, func(c relCand) string { return c.label }, sg.Options{Aliases: m.aliasTable(), Limit: relatedMaxRows})
	}
	rows := make([]Suggestion, 0, len(picked))
	for _, c := range picked {
		r := Suggestion{Label: c.label, Desc: c.desc, Ref: c.ref, Tag: c.tag}
		switch {
		case multi:
			r.Kind = c.kind()
			if c.tag != "" {
				r.Desc = ""
			}
		case c.tag != "":
			r.Desc = "tag"
		}
		rows = append(rows, r)
	}
	if multi {
		rows = relatedGroup(rows)
	}
	return rows
}

// relatedGroup puts rows of one kind together under a heading. Groups come in the order of their best
// ranked row, so the first row is still the best match, and rows keep their rank inside a group.
func relatedGroup(rows []Suggestion) []Suggestion {
	var order []string
	by := map[string][]Suggestion{}
	for _, r := range rows {
		if _, ok := by[r.Kind]; !ok {
			order = append(order, r.Kind)
		}
		by[r.Kind] = append(by[r.Kind], r)
	}
	out := make([]Suggestion, 0, len(rows))
	for _, k := range order {
		for i, r := range by[k] {
			if i == 0 {
				r.Group = kindHeading[k]
				if r.Group == "" {
					r.Group = relationLabel(k)
				}
			}
			out = append(out, r)
		}
	}
	return out
}

// chipGroup is the chosen objects of one kind.
type chipGroup struct {
	kind  string
	names []string
}

// chipGroups groups chosen objects by kind in the stable order of relatedKindOrder. Inside a group the
// order of the line is kept.
func chipGroups(chosen []chosenLink) []chipGroup {
	by := map[string][]string{}
	var kinds []string
	for _, c := range chosen {
		k := c.kind()
		if _, ok := by[k]; !ok {
			kinds = append(kinds, k)
		}
		by[k] = append(by[k], c.label)
	}
	for i := 1; i < len(kinds); i++ { // stable insertion sort by kind order
		for j := i; j > 0 && kindOrderIndex(kinds[j]) < kindOrderIndex(kinds[j-1]); j-- {
			kinds[j], kinds[j-1] = kinds[j-1], kinds[j]
		}
	}
	out := make([]chipGroup, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, chipGroup{kind: k, names: by[k]})
	}
	return out
}
