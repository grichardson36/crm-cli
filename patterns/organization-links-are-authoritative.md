---
type: pattern
status: decided by owner 2026-10-29
core-gate: 5
domains: [domain-model, data, integration]
---
# Organization links are authoritative

**Rule.** A Person–Organization link is the only CRM representation of a person's organization.

**Applies when.** Creating, importing, showing or syncing a person's organization under [[core]] gate 5.

**Default.** Remove `company` from the core person model and CRM interfaces. Show the primary link and export its organization name to flat formats.

**Exception.** Import and CardDAV adapters resolve incoming text to an organization, then make its link primary. They do not write a raw company field.

**Check.** No core person field stores company text, and every displayed organization comes from a Person–Organization link.
