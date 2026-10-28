---
type: pattern
scale: object
status: decided
---

# A merge re-points every link and keeps an alias

**Problem.** A merge that moves only the person row leaves memberships, relations, deal roles and tasks on the archived duplicate, and old ids stop resolving.

**Therefore.** Merging people moves every link of every dropped person to the survivor. An exact duplicate is dropped (soft deleted). A link that becomes a link to itself is dropped. Memberships are never folded: two at one organization can be different roles (CTO and Advisor), so both rows stay. Only a membership identical to one the survivor has (same organization, role, start and end) is dropped, and a primary flag the survivor lacks moves to that identical row. A merge_alias row records the old id and uuid, so old references and imported ids keep resolving, and `people.merged_into` still marks the archived duplicate. The compare screen shows the combined organizations, related people and deals before the merge and the summary says what moved and what was dropped.

Organizations merge the same way (`crm org merge`, `crm_org_merge`, M on marked rows in the Organizations view). The survivor keeps its fields and empty ones are filled from the duplicates. Memberships all move, only an exact duplicate row is dropped and its primary flag moves to the surviving twin. Parent links move, a link that becomes a self link or a duplicate is dropped, and a merge that would make an organization its own parent is refused. Deal links (primary flag kept), task and interaction links and tags move. The duplicate is archived and its merge_alias row points at the survivor, which is also how the company review recognises its old name. Organizations have no merged_into column, the alias row is the record. The merge records each row it changed, so one undo step and one redo step replay it exactly.

**Because.** A merge leaves alias ids in real data: 1,007 contacts and 194 companies carry merged-in old ids ([[research-hubspot-export]] H9).

**Larger patterns.** [[link-from-either-side]], [[sync-safe-data]]

**Smaller patterns.** [[person-belongs-to-many-organizations]]

**Sources.** [[schema-er]] (merge_alias, Q27, owner change: no folding), [[research-hubspot-export]], Salesforce AccountContactRelation (one row per pair, several roles)

**Inspired by.** [[hubspot]], [[us]]
