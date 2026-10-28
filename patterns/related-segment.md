---
type: pattern
scale: interaction
status: decided by owner 2026-10-29, built in feat/related-segment (not merged)
---
# One related segment, suggested from context

**Problem.** An object added inline (a task, a note, a deal) had one pipe segment per linkable kind. The suggestions ignored where the user was: with a deal highlighted, its own people and organizations were never offered. Only the highlighted object itself was, and only in the segment of its own kind.

**Therefore.** Every add line ends with ONE segment, `related`, that takes objects of every kind the relation registry allows for the new object, and tags ([[schema-er]]). The suggestions come from the context, in this order:

1. The object the user is on: the record page, or the highlighted row.
2. That object's directly linked objects of every kind (a deal's people, organizations, tags).
3. Once the user chose related objects: the objects linked to those, one hop.
4. Objects used recently in the session.

Typing narrows the whole set by name with the shared ranker, nickname aliases and typo tolerance ([[autocomplete-component]]). Context candidates come first, so they win ties. A name that starts with `#` looks at tags only. Candidates are deduped, a chosen object is never suggested again, and only the kinds the Relations registry joins to the new object appear. On a record page the page is already linked to the new object, so the page itself is not offered but its links are.

**Popup.** Rows are grouped by kind under dim rule headings, the same grouped-list component as [[grouped-lists-with-dividers]]. Groups come in the order of their best ranked row, so the first row is the best match. At most twelve rows scroll inside ten lines. Each row starts with a short kind prefix so the kinds read without color: `p:` person, `o:` organization, `d:` deal, `t:` task, `n:` note, `#` tag.

**Chips.** The chosen objects show above the prompt as chips, grouped by kind in a stable order (people, organizations, deals, tasks, notes, tags). Each group is in its kind color, taken from the theme roles `kind_person`, `kind_org`, `kind_deal`, `kind_task`, `kind_note`, `kind_tag` (never a literal color, see [[one-abstraction-many-contexts]]). The chips wrap onto extra lines when they do not fit. The prompt itself stays one line ([[one-line-pipe-prompt]]).

**Kept.** Comma separated names, `tab` accepts and adds `, `, typing `|` takes the comma back, backspacing over a name unlinks it before it is saved. Names that are no object yet offer [[pick-or-create]] with a choice per kind and a tag. Links are made through the existing link path and the registry decides the relation kind of each pair ([[link-from-either-side]]). A deal line keeps its organization and person fields, so the people and organizations chosen there or in `related` keep their deal_person and deal_org links, the first organization is primary. A link's role is plain text for now ([[roles-are-free-text]]).

**Because.** Owner decision 2026-10-29. The user adds a note while looking at a deal: the people and organizations of that deal are the likely links, and the next hop is likely once one is chosen.

**Larger patterns.** [[one-line-pipe-prompt]]

**Smaller patterns.** [[autocomplete-component]], [[show-syntax-inline-as-you-type]]

**Sources.** `internal/tui/related.go`, `internal/tui/pipeview.go`, `internal/tui/autocomplete.go`, `internal/theme`

**Inspired by.** [[notion]], [[raycast]]
