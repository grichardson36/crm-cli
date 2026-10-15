# CalDAV sync

Keep events in step with a CalDAV calendar such as Radicale. It is a sync source like any other: `crm sync run caldav`, the run log, the Sync view and `--dry-run` all work.

## Set up

Point it at one calendar collection with environment variables. The source appears in `crm sync list` once `CRM_CALDAV_URL` is set.

- `CRM_CALDAV_URL` is the calendar, such as `https://dav.example.com/grant/personal/`.
- `CRM_CALDAV_USER` and `CRM_CALDAV_PASSWORD` (or `CRM_CALDAV_PASSWORD_FILE`, first line used) log in.
- `CRM_CALDAV_SELF` lists your own addresses, comma separated, so they are left out of meetings.
- `CRM_CALDAV_TWO_WAY=1` also sends local changes (see below).

```sh crm-example
crm sync list
crm sync status
```

```sh crm-example-skip
crm sync run caldav --dry-run
crm sync run caldav
```

## What a run does

- New and changed server events are imported, matched by UID. The ETag says what changed.
- A server delete archives the event here. An empty server is never read as "delete everything".
- A past event with attendees logs one `meeting` on each matched person. Unknown attendees wait in the sync inbox, nobody is created.
- With `CRM_CALDAV_TWO_WAY=1` it also sends new events, edits and deletions. Off by default, because the first run copies the whole local calendar to the server.

If both sides changed the same event, each field is decided on its own: the side that changed it wins, and when both did, the later change wins. Repeats, exceptions and per-instance changes travel with the event. Tasks are not synced.

See also: `calendar`, `sync`, `tui` (Sync view).
