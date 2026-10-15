# Working hours, planning and free time

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

## Plan tasks

`crm cal plan` proposes times for tasks with `--auto-schedule` and a duration. It never writes. Order: hard deadlines, then least slack, priority, deadline, creation. Each task takes the earliest free time inside its working hours, clear of events, locked tasks and buffers. Every row says why. `--min-chunk` lets a task split into blocks.

```sh crm-example
crm task edit 1 --duration 2h --deadline 2026-11-06 --auto-schedule
crm cal plan
crm cal plan --apply --dry-run -f json
```

`--apply` writes only `scheduled_start` and the blocks, as movable times. `--reject ID` skips a row and `--lock ID` pins it. Re-run the plan after events change. `--history` lists applied plans.

## Find a time

Free slots inside working hours, clear of busy events (repeats included), locked task blocks and the buffer. Slots start on the step grid, after the minimum notice. Only your own calendar is checked.

```sh crm-example
crm cal free --len 45m --within 7d
crm cal free --len 30m --from "mon" --notice 4h --buffer 15 --limit 5 -f json
```

See also: calendar, tasks, calendar-ics.
