---
type: inspiration
kind: standard
research: mined
---
# CalDAV and iCalendar standards

**What it is.** RFC 5545 (RRULE, TZID), RFC 6578 sync-collection with opaque tokens, RFC 6638 scheduling with If-Schedule-Tag-Match, ETag with If-Match and 412 on change.

**Research state.** mined. See [[inspirations]].

**What we take.**
- Sync state per calendar: href, etag, sync token. Fall back to full listing on an invalid token. [[import-is-a-snapshot-sync-is-a-relationship]] [[conflicts-keep-server-store-local]]
- Import matches by UID. [[idempotent-by-uid]]

**Sources.** rfc-editor.org/rfc/rfc6578, rfc5545, icalendar.org CalDAV Scheduling RFC 6638.
