---
type: pattern
scale: interaction
status: decided
---

# Import is a snapshot, sync is a relationship

**Problem.** Import and sync share a code path and corrupt each other.

**Therefore.** Import copies a file once into a chosen destination, matches by UID, never deletes. Sync keeps state per calendar (href, etag, sync-token) and runs over a separate adapter. They share only the event writer.

**Larger patterns.** [[local-first]]

**Smaller patterns.** [[idempotent-by-uid]], [[conflicts-keep-server-store-local]]

**Sources.** DESIGN-PATTERNS.md E

**Inspired by.** [[dex]], [[outlook]], [[caldav-and-icalendar-standards]]

**Note (2026-10-16).** Archived with calendar: the CalDAV sync state it describes (href, etag, sync-token per calendar) and ICS import were removed from main. CardDAV keeps the same split.
