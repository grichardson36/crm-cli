---
type: pattern
scale: product
status: decided (changed by the owner 2026-10-27, replaces the review passes of Q21)
---

# Convert text to records automatically, send near duplicates to merge

**Problem.** Years of imported contacts carry a company as plain text (2,266 people, 1,278 distinct spellings in the owner's data). Leaving it as text keeps the organization objects empty. A review screen with a decision per group (the first design) asked the owner to do work the program can do safely.

**Therefore.** Company text creates or reuses an organization on its own, at every door that sets `people.company`. There is no review screen.
- Matching key: `NormalizeCompany` (trim, case-fold, collapse spaces, drop punctuation, "&" as "and", a leading "the", trailing Inc, LLC, Ltd, Corp, Co endings) exact match, or a merge alias ([[merge-repoints-links]]). The text is never rewritten and `people.company` stays the source text.
- No match creates the organization and gives the person a membership (the title is the role). The person's primary membership points to it. Creation and the text edit undo as one step wherever undo exists.
- Fuzzy or similar names (typos, `Acme Holdings` next to `Acme`, same domain) never auto-merge. They are organization merge candidates (`crm org dupes`, MCP `crm_org_dupes`, a "similar name" row on the organization page), merged through the existing org merge, the same way person duplicates appear.
- Junk strings (N/A, none, self-employed, retired, unemployed, `-` and similar) never create organizations. The ignore list is data: built-in defaults plus the user's additions, in the `company_ignore` table (uuid and updated_at, so it can sync), managed by `crm org ignore add|list|remove`, MCP `crm_org_ignore_*` and the palette. A text with no letter or digit is always ignored.
- Existing people: `crm org backfill` (MCP `crm_org_backfill`, palette "Backfill organizations from company text") is a dry run that prints counts: people with company text, distinct groups, organizations to create, organizations reused, ignored, groups of 3 or more listed first. `--apply` runs it as one batch and one undo step. It is idempotent and never moves an existing primary membership. `crm org convert` is a deprecated alias.
- The doors: CLI person add and edit, import, MCP, the command bar and `crm do`, device sync, the TUI person form, DAV inbound ORG ([[sync-maps-structure-to-flat-fields]]).

**Because.** Owner decision 2026-10-27. Matching by exact normalized name is safe to automate, the merge flow already exists for the uncertain cases, and one batch with one undo makes a mistake cheap to reverse. Dropping the review also removed two device-local pieces: the `org-review` tag and the `orgconvert.skip:*` / `orgconvert.done:*` meta keys ([[sync-safe-data]]).

**Larger patterns.** [[unresolved-names-stay-text]] (changed: a name that matches nothing now creates the record, except junk), [[confirm-names-in-one-batch]]

**Smaller patterns.** [[pick-or-create]], [[person-belongs-to-many-organizations]], [[roles-are-free-text]]

**Sources.** [[schema-er]] (Q21, E4), HubSpot and Attio company association docs (read as general knowledge, not re-verified this session), `internal/db/repo/org_auto.go`

**Inspired by.** [[hubspot]], [[attio]], [[dex]]
