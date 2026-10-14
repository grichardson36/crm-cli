# Keep in touch

Give a person a cadence and crm tells you when you are due to reach out.

## Set a cadence

Cadences: `7 days`, `14 days`, `1 mon`, `42 days`, `3 mons`, `6 mons`, `1 year`, `never`.

```sh crm-example
crm person add "Sam Lee" --keep-in-touch "1 mon"
crm person edit 2 --keep-in-touch "14 days"
crm person edit 2 --keep-in-touch ""
```

An empty value clears the cadence.

## See who is due

```sh crm-example
crm due
crm due --within 14
crm due -f json
```

Due is the last contact plus the cadence. With no contact yet, it counts from when the cadence was set. Fields: `person_id`, `name`, `keep_in_touch`, `last_seen`, `due_at`, `days_overdue`. `--within N` also lists people who become due in the next N days.

## Record contact

```sh crm-example
crm person contacted 1
crm person contacted 1 --at 2026-10-01
```

Logging a call, email, meeting or message also counts as contact. Notes count only for note types defined with `--counts-as-contact` (see `crm help notes`).

## Snooze

```sh crm-example
crm person edit 1 --snooze-until 2027-01-01
crm person edit 1 --snooze-until ""
```

A snoozed person is hidden from `crm due` until the date. Marking contacted clears the snooze.

## Related topics

`birthdays`, `notes`, `tui` (the Due view, key 2).
