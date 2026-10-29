# Saved views: a named filter, sort and group

A saved view is a name, an object kind (person, organization, deal, task, note, tag), a filter, a sort, a group and optional columns. It shows in the terminal UI as a view like any other: in the Views screen (`v`), in the tabs and under a number key while it is shown, and it renders with the list of its kind with the filter, sort and group applied. Marks, bulk verbs, search and `ctrl+o` work as in any list.

The filter is a list of conditions, all must hold, with any-of inside one condition (`tag has client|lead`). There is no SQL. Operators by field type:

| Type | Operators |
|---|---|
| text | `is`, `is_not`, `contains`, `empty`, `not_empty` |
| date | `before`, `after`, `within_last N`, `in_next N`, `empty`, `not_empty` |
| choice | `is`, `is_not`, `in` |
| number | `=`, `<`, `>`, `between` |
| relation | `linked_to`, `not_linked_to`, `has_any` (a name, id or uuid) |
| tag | `has` (a parent tag also matches its children), `does_not_have` |

A date is `2026-12-01`, `today`, `90 days ago`, `in 30 days`, `-90d`, `next friday`. It is read when the view runs, so "last contacted before 90 days ago" stays true tomorrow. A birthday takes `within_last` and `in_next` (month and day, any year). `crm views fields person` lists every field with its operators. Unknown fields and operators fail with `E_QUERY_*` codes (`crm help validation`).

```sh crm-example
crm views list
crm views run "Overdue follow-up" -f json
crm views export "Tasks due this week"
```

```sh
crm views save "Clients to call" --kind person --where "tag has client" --where "last_contacted before 60 days ago" --sort last_contacted
crm views edit "Clients to call" --add-where "tier in 1|2"
crm views delete "Clients to call"   # soft, crm views restore brings it back
```

Three example views ship off by default: Overdue follow-up (people not contacted in 90 days), Open deals by stage and Tasks due this week (grouped by day). Enable or delete them like your own. Groups of a date field are a day, week or month (`--bucket`), groups of a choice follow its order (deal stages in stage order).

`crm views export` writes a small TOML file (kind, filter, sort, group, relation values as names, nothing else), `crm views import FILE` makes a view from one. MCP: `crm_view_list`, `_show`, `_run`, `_save`, `_edit`, `_rename`, `_delete`, `_restore`, `_move`, `_export`, `_import`, `_fields`.

## In the terminal UI

`v` then `x` shows or hides a view, `shift+up` and `shift+down` move it, `enter` opens it, `e` edits it, `d` asks y or n and deletes it, `r` restores a deleted one. The palette has "Save current view as..." (captures the list on screen: kind, search, sort, and the options of a saved view), "Edit view", "Delete view" and "Restore view". `ctrl+o` in a saved view shows its filter, sort and group and lets you change them for this session without saving. The editor shows one row per field and one row per condition. `a` adds a condition, `d` removes it, `enter` edits the cell under the cursor (a relation or tag value uses a typeahead), `ctrl+s` saves, `esc` closes. Views are stored in the database (table `saved_view`), so they travel with device sync. Which built-in modules are shown stays per device in `config.toml`.

See also: modules, tui, tags, validation.
