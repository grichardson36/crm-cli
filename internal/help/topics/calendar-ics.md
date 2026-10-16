# Calendar files (ICS)

Events and tasks (`--tasks`, as VTODO) go in and out as iCalendar. An import is one-shot and takes the file as it is: a UID already here is updated, any other is added. Nothing is skipped or protected. To stay in sync use the CalDAV source (`crm help caldav`).

```sh crm-example
crm cal export --tasks
crm cal export --from 2026-11-01 --to 2027-01-01
```

```sh crm-example-skip
crm cal import ./feed.ics --dry-run
crm cal import ./feed.ics --source work-feed
```

Attendees with a contact's email link to the contact. Other addresses stay on the event as guests and no contact is created. Times without a zone are read in this device's zone and kept floating.

See also: calendar, caldav, calendar-plan.
