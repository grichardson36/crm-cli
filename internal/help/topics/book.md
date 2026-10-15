# Booking links

Let people pick a time without a server. A link holds the rules. You send the open times as text, a web page or mail links, and confirm the reply.

## Make a link

```sh crm-example
crm book add "Intro call" --email me@example.com --len 30m,45m --notice 4h --buffer 10 --max-day 3
crm book add "Sales demo" --question "Company!" --remind 1d --followup 1d --dry-run
crm book list
```

Times come from your working hours (`crm help calendar-plan`), after the notice, inside `--horizon` days, on the `--step` grid, clear of events and bookings plus the buffers. `--max-day`, `--max-week` and `--max-month` cap meetings. `--uses 1` makes a one-time link. A trailing `!` makes a question required.

## Send times

```sh crm-example-skip
crm book offer intro-call --count 8 --tz America/New_York
crm book export intro-call --out ./site
```

`offer` prints text to paste. `export` writes one HTML page where each time is a mail button. Both show free times only, never event titles. Regenerate the page when your calendar changes, it is not live.

## Confirm a reply

The mail button puts a `Slot:` line in the reply. Paste it as the argument, or give `--at`.

```sh crm-example-skip
crm book confirm intro-call/2026-10-20T15:00:00Z/30 --name "Jane Doe" --email jane@example.com --answer "Agenda=pricing" --invite invite.ics
crm book cancel 3 --invite cancel.ics
```

The time must still be open, and two confirmations cannot both take it. Confirm makes the event, links the contact with that email (none is created), logs a meeting, and with `--follow-up` adds a task. Attach the `.ics` to your reply. `crm cal invite EVENT --to a@b.c` makes one for any event.

`crm book pause|resume|rm LINK` and `crm book list --bookings` manage the rest. Reminders and follow-ups: `crm help reminders`.

See also: calendar, calendar-plan, reminders, tasks.
