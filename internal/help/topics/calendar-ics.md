# Calendar files (ICS)

Events and tasks (`--tasks`, as VTODO) go in and out as iCalendar. Imports match by UID, so a second import updates instead of duplicating, and an export imported elsewhere keeps every UID.

```sh crm-example
crm cal export --tasks
crm cal export --from 2026-11-01 --to 2027-01-01 --out ./crm.ics
```

```sh crm-example-skip
crm cal import ./feed.ics --dry-run
crm cal import ./feed.ics --source work-feed
```

Attendees are matched to people by email. Unknown addresses are listed and never created. Use one `--source` name per feed. Times without a zone are read in this device's zone and kept floating. An event deleted here is not created again by a later import.

See also: calendar, caldav, calendar-plan.
