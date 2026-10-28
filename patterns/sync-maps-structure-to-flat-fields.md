---
type: pattern
scale: product
status: open (decided for DAV ORG only, standard still to research). Rule changed 2026-10-27: a miss creates the organization
---

# Two-way sync maps structure to flat fields without losing either side

**Problem.** Our data is structured (a person has memberships in organization records). Phones, CardDAV and other tools hold flat text (vCard ORG). Dropping the flat edit blocks the user from changing data where they are. Silently overwriting the flat side confuses them. A true conflict is only two edits to the same thing since the last sync.

**Therefore.** An edit on either side propagates. The receiving side resolves flat text to a structured record: match an existing record (normalized name, through [[merge-repoints-links]] aliases), else create it (owner decision 2026-10-27, see [[convert-text-to-records-in-review-passes]]). Junk text on the ignore list creates nothing. Close but different names never merge, they become merge candidates. Emptying a flat field never removes a structured link: the DAV code, tests and docs state this rule. Only an edit to the same field on both sides since the last sync is a conflict, and it follows [[conflicts-keep-server-store-local]].

**Because.** Owner decision 2026-10-19 (Q-H): "why can't I make changes on my phone that propagate?" The first build dropped phone ORG edits. That was our modeling limit, not a conflict.

**Applied so far.** DAV ORG: a match repoints the primary membership in place, a miss creates the organization and repoints or creates the primary membership, `people.company` takes the phone's text. Emptying ORG keeps the membership. Outbound ORG is the primary organization name. See [[schema-er]] E4 and open discrepancy X23.

**Larger patterns.** [[local-first]]

**Smaller patterns.** [[sync-safe-data]], [[conflicts-keep-server-store-local]], [[import-is-a-snapshot-sync-is-a-relationship]], [[unresolved-names-stay-text]], [[merge-repoints-links]]

**To research into a standard.** This pattern is a placeholder for a full sync standard. Questions: how local-first systems model field-level merge (CRDTs, last-write-wins registers, per-field versions), how they handle structured-to-flat mappings and round trips that rewrite the sender's text, deletes versus tombstones, and conflict display to users. Starting points from the local-first community: Ink & Switch "Local-first software", Automerge, cr-sqlite, Replicache and Linear's sync engine, ElectricSQL, Actual Budget's sync. Read first-party sources before deciding ([[surface-gaps-before-deciding]]).

**Sources.** Owner decision 2026-10-19, `internal/dav/org_test.go`
