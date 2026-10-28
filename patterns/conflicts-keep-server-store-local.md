---
type: pattern
scale: interaction
status: decided
---

# On conflict keep the server value and store the local one

**Problem.** Silent data loss when both sides changed the same field.

**Therefore.** Write with If-Match. On 412 refetch and merge per field using field_versions. If both sides changed the same field, keep the server value and save the local value as a conflict note. Never lose silently. Pull with sync-collection and fall back to a full listing on an invalid token. Treat 404 entries as soft deletes.

**Larger patterns.** [[import-is-a-snapshot-sync-is-a-relationship]], [[sync-safe-data]]

**Sources.** RFC 6578, DESIGN-PATTERNS.md E

**Inspired by.** [[caldav-and-icalendar-standards]]
