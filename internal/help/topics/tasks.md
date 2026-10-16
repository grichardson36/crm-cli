# Tasks

Follow-ups with a due date and priority, optionally linked to people and a deal.

## Add and list

Priorities: low, medium (default), high. Dates are `YYYY-MM-DD` or an ISO 8601 timestamp. `--person` can repeat. `--all` includes completed tasks. `--limit N` caps the list.

```sh crm-example
crm task add "Send proposal" --person 1 --due 2026-11-01 --priority high
crm task add "Intro call prep" --person 1 --person 2 --due 2026-11-05
crm task list
crm task list --overdue
crm task list --person 1 --all
crm task show 1
```

## Change and finish

```sh crm-example
crm task edit 1 --priority low
crm task edit 1 --due 2026-12-01
crm task done 1
crm task delete 1
```

`task delete` archives the task and `task restore 1` brings it back. `task done` marks it completed. Delete and done accept `--dry-run`. In the TUI: `a` adds, `enter` edits, `d` deletes (asks y/n), `u` restores, `ctrl+z` undoes. Link a task to people and deals with `crm task link person|deal`.

## Recurring tasks

A recurring task needs `--due`. When you complete it, the next occurrence is created. Examples of recurrence: `7 days`, `1 mon`, `weekly`.

```sh crm-example
crm task add "Monthly check-in" --person 1 --due 2026-11-01 --recurrence "1 mon"
crm task add "Weekly review" --due 2026-11-01 --recurrence weekly
crm task done 1 --dry-run
```

See also: calendar, short-syntax, keep-in-touch, links, agents, tui.
