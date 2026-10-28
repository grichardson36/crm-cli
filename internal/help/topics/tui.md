# Terminal UI

A full-screen, keyboard-first interface for browsing and updating contacts. Mouse works too: click rows and tabs, scroll with the wheel.

```sh crm-example-skip
crm tui
crm tui --read-only
```

`--read-only` disables every key that changes data. Press `?` inside for the full key list. The bar (`ctrl+k` or `/`) is the one box. Type to find a person, org, deal, task or note, run an action, check or change a setting (`theme`, `keys`, `syntax`), or enter a command (`n Sam called`). Prefixes narrow it: `>` actions, `@` people, `#` tags, `=` settings, `?` help. An empty box lists what you used recently. A name with no match offers to create the person or add a note. A tag name lists the tag without `#`. `tab` puts the focus on the results, where the keys of the object's view work on the highlighted row (see tui-keys).

## Views

Every view is a module you can turn on or off and reorder (see modules). Jump with the number keys, cycle with `[` and `]`, or press `v` for the Views screen, which also shows, hides and reorders them. Archived and Sync are hidden by default and open from Settings, the bar or the Hidden group of the Views screen. The keys below are the default order, the tenth module takes `0`.

| Key | View | Shows |
|---|---|---|
| 1 | Home | people due for follow-up and person-linked tasks due today |
| 2 | People | everyone |
| 3 | Follow-up | who you are due to contact (keep-in-touch) |
| 4 | Tasks | open tasks |
| 5 | Organizations | companies and groups |
| 6 | Deals | deals with stage and value |
| 7 | Birthdays | upcoming birthdays |
| 8 | Merge | groups of probable duplicates |
| 9 | Settings | every setting, with where it is stored (device, synced or session) |
| hidden | Archived, Sync | archived records, sync sources and the inbox (Settings: "Open Archived", "Open Sync") |

## Palette actions that mirror CLI commands

These work in `--read-only` mode. Each opens a scrollable page (`up` / `down`, `home` / `end`, `esc` goes back).

- "Help: topics" lists topics with a filter. "Help: search" lists matching lines.
- "Doctor" runs the checks of `crm doctor`. "Where" shows the open database. "Domains" lists the databases in the data directory.
- "DAV: info", "DAV: conflicts" and "DAV: status" show the phone sync details, logged conflicts and counts. They never start the server (`crm help dav`).

All keys, editing, merge, archive, notes, sync and Home are in `crm help tui-keys`.

See also: short-syntax, tui-keys, theme, keep-in-touch, duplicates, archive.
