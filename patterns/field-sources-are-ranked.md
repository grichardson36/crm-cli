---
type: pattern
status: proposed by owner 2026-10-25, ranking order not yet decided
---
# Field sources are ranked

Every field value remembers where it came from (typed by the owner, enrichment such as LinkedIn, phone sync, import). When two sources disagree, the higher-ranked source wins and the other value is kept as a suggestion, never silently dropped.

Why: a LinkedIn profile saying someone works at a company is more reliable than a phone note, which is more likely to be outdated. Recency matters too, so rank and age are both inputs.

Draft order, to be confirmed by the owner: typed by the owner, enrichment provider, phone sync, bulk import. Owner-typed values stay highest because they are deliberate.

Applies to: org membership from company text, title and role, and every enrichment field. Related: [[enrichment-is-suggested-then-accepted]], [[conflicts-keep-server-store-local]], [[sync-maps-structure-to-flat-fields]], [[sync-safe-data]].
