# Calendar

Events with attendees, repeats, reminders and tags, kept in the same database as your contacts.

## Add and list

Times are in this device's zone unless you pass `--tz`. A date alone makes an all-day event.

```sh crm-example
crm cal add "Lunch with Sam" --start "fri 12:30" --duration 1h --location "Cafe Rio" --person 1
crm cal add "Conference" --start 2026-11-10 --end 2026-11-12
crm cal add "Standup" --start "2026-11-02 09:15" --duration 15m --repeat weekdays --tz America/Chicago
crm cal list
crm cal list --from 2026-11-01 --days 14 --tz Europe/London -f json
crm cal show 1
```

Start phrases: `2026-11-02 09:15`, `fri 12:30`, `tomorrow 3pm`, `oct 20 10:00`, `noon`. Instants are stored in UTC and the event keeps its own zone, so a 09:15 standup stays at 09:15 local across daylight saving. `--tz` on `list` changes only how times are shown.

## Repeats

`--repeat` takes `daily`, `weekdays`, `every fri`, `every 2w`, `last fri`, `1st mon`, or an RRULE like `FREQ=WEEKLY;BYDAY=MO`. End it with `--until DATE` or `--count N`. Repeats are expanded when listed, at most 1000 instances per query.

Edit or delete a repeating event with `--scope all` (default), `this` or `following`. `--at` names the instance: use the `occurrence` value from `crm cal list -f json`.

```sh crm-example
crm cal edit 1 --location "Room 4"
crm cal edit 1 --title "Intro call (moved)" --start "2026-11-03 11:00"
crm cal rm 1 --dry-run
```

## People and reminders

```sh crm-example
crm cal attendee add 1 2 --role organizer
crm cal edit 1 --remind 15m --remind 1d:email
crm tag apply event 1 work
crm search "intro" --type event
```

`cal purge ID --yes` removes a deleted event for good. Add `--dry-run` to any change to preview it. Reminders are stored, delivery is up to a scheduler or an agent.

## ICS files

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

## Working hours

Mon-Fri 09:00-17:00 in this device's zone until you change them. Hours are wall clock, so they follow daylight saving time. Buffer 10 min, step 15 min, horizon 14 days.

```sh crm-example
crm cal hours show
crm cal hours set mon-fri 08:30-17:00 --tz America/Chicago
crm cal hours set sat off
crm cal hours override 2026-12-24 09:00-12:00
crm cal hours config --buffer 15 --step 30 --horizon 21
crm cal hours show --expand today
```

## Find a time

Free slots inside working hours, clear of busy events (repeats included), locked task blocks and the buffer. Slots start on the step grid, after the minimum notice. Only your own calendar is checked.

```sh crm-example
crm cal free --len 45m --within 7d
crm cal free --len 30m --from "mon" --notice 4h --buffer 15 --limit 5 -f json
```

See also: `tasks`, `keep-in-touch`, `tui` (Calendar view, key 9), `agents`.
