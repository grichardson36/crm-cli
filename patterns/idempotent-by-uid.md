---
type: pattern
scale: interaction
status: archived
---

# Imports are idempotent by UID

**Problem.** Re-importing creates duplicates.

**Therefore.** Match key is calendar, UID and RECURRENCE-ID. New inserts. Existing updates only if SEQUENCE is higher or LAST-MODIFIED newer, else skip. Output counts: new, updated, skipped, failed. --dry-run lists them. Unknown properties are kept raw so export round-trips.

**Because.** No app was confirmed to update on re-import, so this is our own choice.

**Larger patterns.** [[import-is-a-snapshot-sync-is-a-relationship]]

**Sources.** DESIGN-PATTERNS.md E

**Inspired by.** [[outlook]], [[thunderbird]], [[caldav-and-icalendar-standards]]

**Archived with calendar (2026-10-16).** ICS import and export were removed from main. The rule still applies to any future import by UID. Code: branch `archive/calendar`, tag `archive-calendar-2026-10-16`.
