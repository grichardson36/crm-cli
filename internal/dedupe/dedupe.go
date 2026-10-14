// Package dedupe finds groups of people who are probably the same person.
//
// The package is pure: it works on in-memory Person values and never touches the database.
//
// Confidence levels, highest first:
//
//	High   shared email (case-insensitive), phone (last 10 digits) or social handle value
//	Medium same normalized first and last name
//	Low    nickname match (bob/robert) with the same normalized last name
//	Fuzzy  Levenshtein similarity of the normalized full name >= FuzzyThreshold, same first initial
//
// Rules (Dex compatible):
//   - Two people never share a group when both have a LinkedIn, Facebook or Twitter value and the values differ.
//   - Archived people are ignored, so every group has at least one non-archived person.
//   - Company is never compared.
//   - A dismissed pair is never joined directly, and two sets are not merged when a dismissed pair spans them.
//
// Grouping: every pair of people gets at most one edge carrying its highest level. Edges below
// Options.Min are dropped. Groups are the connected components of the remaining edges, built
// transitively (A-B and B-C puts A, B and C together). The level of a group is the highest level L
// at which all members are connected using edges of level L or higher, which is the weakest edge
// that was needed to join the group.
package dedupe

import (
	"fmt"
	"sort"
	"strings"
)

// Level is a confidence level. Higher is more confident.
type Level int

// Confidence levels.
const (
	Fuzzy Level = iota + 1
	Low
	Medium
	High
)

// FuzzyThreshold is the minimum Levenshtein ratio for a Fuzzy edge.
const FuzzyThreshold = 0.88

// maxBucket is the largest set of people sharing one key that is still paired. Larger sets
// are generic values (a shared front desk phone, a very common name) and would only produce noise.
const maxBucket = 40

// String returns the lowercase name of the level.
func (l Level) String() string {
	switch l {
	case High:
		return "high"
	case Medium:
		return "medium"
	case Low:
		return "low"
	case Fuzzy:
		return "fuzzy"
	}
	return "unknown"
}

// ParseLevel parses high, medium, low or fuzzy.
func ParseLevel(s string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "high":
		return High, nil
	case "medium":
		return Medium, nil
	case "low":
		return Low, nil
	case "fuzzy":
		return Fuzzy, nil
	}
	return 0, fmt.Errorf("invalid level %q (must be high, medium, low or fuzzy)", s)
}

// Person is the data compared for one person. Company is deliberately absent.
type Person struct {
	ID        int64
	FirstName string
	LastName  string
	Archived  bool
	Emails    []string
	Phones    []string
	// Handles maps handle kind (linkedin, twitter, ...) to value.
	Handles map[string]string
}

// Name returns the display name.
func (p Person) Name() string {
	return strings.TrimSpace(p.FirstName + " " + p.LastName)
}

// Options controls Find.
type Options struct {
	// Min is the lowest level to report. Zero means Medium.
	Min Level
	// Dismissed holds dismissed pairs as [2]int64{low, high}.
	Dismissed map[[2]int64]bool
}

// Member is one person in a group.
type Member struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Group is a set of probable duplicates.
type Group struct {
	Level   Level    `json:"-"`
	Members []Member `json:"members"`
	Reasons []string `json:"reasons"`
}

// IDs returns the member IDs in ascending order.
func (g Group) IDs() []int64 {
	ids := make([]int64, len(g.Members))
	for i, m := range g.Members {
		ids[i] = m.ID
	}
	return ids
}

// conflictKinds are handle kinds that must match when both people have one.
var conflictKinds = []string{"linkedin", "facebook", "twitter"}

// matchKinds are handle kinds whose shared value is a High signal. Website and other are excluded
// because people at one company share a site.
var matchKinds = map[string]bool{
	"linkedin": true, "twitter": true, "facebook": true, "instagram": true,
	"telegram": true, "whatsapp": true, "tiktok": true, "youtube": true,
}

type pair [2]int

type edge struct {
	level   Level
	reasons []string
}

type prepared struct {
	p        Person
	first    string
	last     string
	social   map[string]string // conflict kind -> normalized value
	fullName string
}

// Find returns duplicate groups, highest level first, then larger groups first, then lowest ID.
func Find(people []Person, opts Options) []Group {
	min := opts.Min
	if min == 0 {
		min = Medium
	}

	var ps []prepared
	for _, p := range people {
		if p.Archived {
			continue
		}
		f, l := NormalizeName(p.FirstName, p.LastName)
		pp := prepared{p: p, first: f, last: l, fullName: FullName(f, l), social: map[string]string{}}
		for _, k := range conflictKinds {
			if v := HandleKey(p.Handles[k]); v != "" {
				pp.social[k] = v
			}
		}
		ps = append(ps, pp)
	}

	edges := map[pair]*edge{}
	add := func(a, b int, lvl Level, reason string) {
		if lvl < min {
			return
		}
		if a > b {
			a, b = b, a
		}
		if conflicts(ps[a].social, ps[b].social) {
			return
		}
		k := pair{a, b}
		e := edges[k]
		if e == nil {
			e = &edge{}
			edges[k] = e
		}
		if lvl > e.level {
			e.level = lvl
		}
		e.reasons = append(e.reasons, reason)
	}
	link := func(buckets map[string][]int, lvl Level, reason func(key string) string) {
		for key, idx := range buckets {
			if len(idx) < 2 || len(idx) > maxBucket {
				continue
			}
			r := reason(key)
			for i := 0; i < len(idx); i++ {
				for j := i + 1; j < len(idx); j++ {
					add(idx[i], idx[j], lvl, r)
				}
			}
		}
	}

	// High: shared email, phone or social handle.
	emails, phones, handles := map[string][]int{}, map[string][]int{}, map[string][]int{}
	for i, pp := range ps {
		for _, e := range pp.p.Emails {
			if k := EmailKey(e); k != "" {
				emails[k] = appendUnique(emails[k], i)
			}
		}
		for _, ph := range pp.p.Phones {
			if k := PhoneKey(ph); k != "" {
				phones[k] = appendUnique(phones[k], i)
			}
		}
		for kind, v := range pp.p.Handles {
			if !matchKinds[kind] {
				continue
			}
			if k := HandleKey(v); k != "" {
				key := kind + ":" + k
				handles[key] = appendUnique(handles[key], i)
			}
		}
	}
	link(emails, High, func(k string) string { return "email " + k })
	link(phones, High, func(k string) string { return "phone " + k })
	link(handles, High, func(k string) string { return "handle " + k })

	// Medium: same normalized first and last name. Low: nickname match.
	if min <= Medium {
		names, canon := map[string][]int{}, map[string][]int{}
		for i, pp := range ps {
			if pp.first == "" || pp.last == "" {
				continue
			}
			nk := pp.first + " " + pp.last
			names[nk] = append(names[nk], i)
			ck := Canonical(pp.first) + " " + pp.last
			canon[ck] = append(canon[ck], i)
		}
		link(names, Medium, func(k string) string { return "same name " + k })
		if min <= Low {
			for _, idx := range canon {
				if len(idx) < 2 || len(idx) > maxBucket {
					continue
				}
				for i := 0; i < len(idx); i++ {
					for j := i + 1; j < len(idx); j++ {
						a, b := ps[idx[i]], ps[idx[j]]
						if a.first != b.first {
							add(idx[i], idx[j], Low, fmt.Sprintf("nickname %s/%s", a.first, b.first))
						}
					}
				}
			}
		}
	}

	// Fuzzy: similar spelling, same first initial. People are bucketed by first initial and sorted
	// by name length so that only names within the length tolerance are compared.
	if min <= Fuzzy {
		byInitial := map[rune][]int{}
		for i, pp := range ps {
			r := []rune(pp.fullName)
			if len(r) < 5 || pp.first == "" || pp.last == "" {
				continue
			}
			byInitial[r[0]] = append(byInitial[r[0]], i)
		}
		type cand struct {
			idx   int
			runes []rune
		}
		var bufA, bufB []int
		for _, idx := range byInitial {
			cands := make([]cand, len(idx))
			for n, i := range idx {
				cands[n] = cand{i, []rune(ps[i].fullName)}
			}
			sort.Slice(cands, func(a, b int) bool { return len(cands[a].runes) < len(cands[b].runes) })
			for a := 0; a < len(cands); a++ {
				for b := a + 1; b < len(cands); b++ {
					// ratio >= T needs distance <= (1-T)*maxLen, and distance >= length difference.
					la, lb := len(cands[a].runes), len(cands[b].runes)
					if float64(lb-la) > (1-FuzzyThreshold)*float64(lb) {
						break
					}
					if ps[cands[a].idx].fullName == ps[cands[b].idx].fullName {
						continue
					}
					if r := similarity(cands[a].runes, cands[b].runes, &bufA, &bufB); r >= FuzzyThreshold {
						add(cands[a].idx, cands[b].idx, Fuzzy, fmt.Sprintf("similar name %s ~ %s (%.2f)", ps[cands[a].idx].fullName, ps[cands[b].idx].fullName, r))
					}
				}
			}
		}
	}

	return group(ps, edges, opts.Dismissed)
}

func appendUnique(s []int, v int) []int {
	if len(s) > 0 && s[len(s)-1] == v {
		return s
	}
	return append(s, v)
}

func conflicts(a, b map[string]string) bool {
	for _, k := range conflictKinds {
		if a[k] != "" && b[k] != "" && a[k] != b[k] {
			return true
		}
	}
	return false
}

// comp tracks one connected set while groups are built.
type comp struct {
	members []int
	social  map[string]string
	bottom  Level // weakest edge used to join the set
	reasons map[string]bool
}

func group(ps []prepared, edges map[pair]*edge, dismissed map[[2]int64]bool) []Group {
	isDismissed := func(a, b int64) bool {
		return dismissed[[2]int64{a, b}] || dismissed[[2]int64{b, a}]
	}
	type kv struct {
		p pair
		e *edge
	}
	list := make([]kv, 0, len(edges))
	for p, e := range edges {
		if isDismissed(ps[p[0]].p.ID, ps[p[1]].p.ID) {
			continue
		}
		list = append(list, kv{p, e})
	}
	// Strongest edges first so the weakest edge that joins a set defines its level.
	sort.Slice(list, func(i, j int) bool {
		if list[i].e.level != list[j].e.level {
			return list[i].e.level > list[j].e.level
		}
		if list[i].p[0] != list[j].p[0] {
			return list[i].p[0] < list[j].p[0]
		}
		return list[i].p[1] < list[j].p[1]
	})

	parent := map[int]int{}
	comps := map[int]*comp{}
	find := func(x int) int {
		for {
			p, ok := parent[x]
			if !ok || p == x {
				return x
			}
			parent[x] = parent[p]
			x = parent[x]
		}
	}
	get := func(x int) (int, *comp) {
		r := find(x)
		if _, ok := parent[r]; !ok {
			parent[r] = r
			c := &comp{members: []int{r}, social: map[string]string{}, bottom: High, reasons: map[string]bool{}}
			for k, v := range ps[r].social {
				c.social[k] = v
			}
			comps[r] = c
		}
		return r, comps[r]
	}
	spanDismissed := func(a, b *comp) bool {
		if len(dismissed) == 0 {
			return false
		}
		if len(a.members) > len(b.members) {
			a, b = b, a
		}
		for _, x := range a.members {
			for _, y := range b.members {
				if isDismissed(ps[x].p.ID, ps[y].p.ID) {
					return true
				}
			}
		}
		return false
	}

	for _, it := range list {
		ra, ca := get(it.p[0])
		rb, cb := get(it.p[1])
		if ra == rb {
			for _, r := range it.e.reasons {
				ca.reasons[r] = true
			}
			continue
		}
		if conflicts(ca.social, cb.social) || spanDismissed(ca, cb) {
			continue
		}
		// Merge the smaller set into the larger one.
		if len(ca.members) < len(cb.members) {
			ra, rb = rb, ra
			ca, cb = cb, ca
		}
		parent[rb] = ra
		ca.members = append(ca.members, cb.members...)
		for k, v := range cb.social {
			ca.social[k] = v
		}
		for r := range cb.reasons {
			ca.reasons[r] = true
		}
		for _, r := range it.e.reasons {
			ca.reasons[r] = true
		}
		if cb.bottom < ca.bottom {
			ca.bottom = cb.bottom
		}
		if it.e.level < ca.bottom {
			ca.bottom = it.e.level
		}
		delete(comps, rb)
	}

	var out []Group
	for _, c := range comps {
		if len(c.members) < 2 {
			continue
		}
		g := Group{Level: c.bottom}
		for _, m := range c.members {
			g.Members = append(g.Members, Member{ID: ps[m].p.ID, Name: ps[m].p.Name()})
		}
		sort.Slice(g.Members, func(i, j int) bool { return g.Members[i].ID < g.Members[j].ID })
		for r := range c.reasons {
			g.Reasons = append(g.Reasons, r)
		}
		sort.Strings(g.Reasons)
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Level != out[j].Level {
			return out[i].Level > out[j].Level
		}
		if len(out[i].Members) != len(out[j].Members) {
			return len(out[i].Members) > len(out[j].Members)
		}
		return out[i].Members[0].ID < out[j].Members[0].ID
	})
	return out
}

// similarity returns 1 - editDistance/maxLen, or 0 as soon as it is certain that the ratio
// is below FuzzyThreshold (the smallest row value is a lower bound on the final distance).
// bufA and bufB are scratch rows reused between calls.
func similarity(a, b []rune, bufA, bufB *[]int) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	m := len(a)
	if len(b) > m {
		m = len(b)
	}
	maxDist := int((1 - FuzzyThreshold) * float64(m))
	if cap(*bufA) < len(b)+1 {
		*bufA = make([]int, len(b)+1)
		*bufB = make([]int, len(b)+1)
	}
	prev, cur := (*bufA)[:len(b)+1], (*bufB)[:len(b)+1]
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		rowMin := cur[0]
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = minInt(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if cur[j] < rowMin {
				rowMin = cur[j]
			}
		}
		if rowMin > maxDist {
			return 0
		}
		prev, cur = cur, prev
	}
	return 1 - float64(prev[len(b)])/float64(m)
}

func minInt(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}
