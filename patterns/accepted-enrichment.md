---
type: pattern
status: decided
core-gate: 7
domains: [data, integration, interface]
---
# Accepted enrichment

**Rule.** Fetched data is a suggestion until the user accepts it.

**Applies when.** An integration, import or model proposes a value under [[core]] gate 7.

**Default.** Show the source and proposed value without silently replacing the current CRM fact.

**Exception.** A deterministic adapter mapping may update its own managed state.

**Check.** An enrichment proposal can be accepted, rejected or ignored without losing the prior value.
