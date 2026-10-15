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

See also: `tasks`, `keep-in-touch`, `tui`, `agents`.
