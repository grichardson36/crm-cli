---
type: pattern
status: decided
core-gate: 7
domains: [integration, data, architecture]
---
# Edge adapters

**Rule.** External formats translate into CRM facts at the boundary.

**Applies when.** Importing, exporting, synchronizing or integrating under [[core]] gate 7.

**Default.** Map external data into the canonical object, field, link or activity model.

**Exception.** Preserve unmapped external detail as adapter state when dropping it would lose data.

**Check.** No external field creates a parallel CRM fact or changes the core model without an owner decision.
