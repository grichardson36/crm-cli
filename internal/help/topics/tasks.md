# Tasks

Follow-ups with a due date and priority, optionally linked to people and a deal.

## Add and list

Priorities: low, medium (default), high. Dates are `YYYY-MM-DD` or an ISO 8601 timestamp.

```sh crm-example
crm task add "Send proposal" --person 1 --due 2026-11-01 --priority high
crm task add "Intro call prep" --person 1 --person 2 --due 2026-11-05
crm task list
crm task list --overdue
crm task list --person 1 --all
crm task show 1
```

`--person` can repeat. `--all` includes completed tasks. `--limit N` caps the list.

## Change and finish

```sh crm-example
crm task edit 1 --priority low
crm task edit 1 --due 2026-12-01
crm task done 1
crm task delete 1
```

`task delete` archives the task. `task done` marks it completed.

## Recurring tasks

A recurring task needs `--due`. When you complete it, the next occurrence is created.

```sh crm-example
crm task add "Monthly check-in" --person 1 --due 2026-11-01 --recurrence "1 mon"
crm task add "Weekly review" --due 2026-11-01 --recurrence weekly
```

Examples of recurrence: `7 days`, `1 mon`, `weekly`.

## Preview before changing

```sh crm-example
crm task done 1 --dry-run
crm task delete 1 --dry-run -f json
```

## Related topics

`keep-in-touch`, `agents`, `tui` (the Tasks view, key 4; `t` adds a task, `x` completes one).
