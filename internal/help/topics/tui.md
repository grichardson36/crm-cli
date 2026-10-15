# Terminal UI

A full-screen, keyboard-first interface for browsing and updating contacts. Mouse works too: click rows and tabs, scroll with the wheel.

```sh crm-example-skip
crm tui
crm tui --read-only
```

`--read-only` disables every key that changes data. Press `?` inside for the full key list. The command palette (`ctrl+k`, also `:`) searches and runs every action, so you never have to remember a key.

## Views

Jump with the number keys, cycle with `[` and `]`, or press `v` for the view switcher. The tab bar shows Home, People, Due and Tasks, with the rest under More.

| Key | View | Shows |
|---|---|---|
| 1 | Home | what needs attention: who is due, birthdays, tasks, recent notes, sync status |
| 2 | People | everyone, searchable |
| 3 | Due | who you are due to contact (keep-in-touch) |
| 4 | Tasks | open tasks |
| 5 | Birthdays | upcoming birthdays |
| 6 | Merge | groups of probable duplicates |
| 7 | Archived | archived people |
| 8 | Sync | sync sources and the inbox of unmatched records |

## Palette actions that mirror CLI commands

These work in `--read-only` mode. Each opens a scrollable page (`j` / `k`, `g` / `G`, `esc` goes back).

- "Help: topics" lists topics with a filter. "Help: search" lists matching lines.
- "Doctor" runs the checks of `crm doctor` with the fix line for each problem.
- "Where" shows the open database. "Domains" lists the databases in the data directory.

All keys, editing, merge, archive, notes, sync and Home are in `crm help tui-keys`.

See also: tui-keys, keep-in-touch, duplicates, archive.
