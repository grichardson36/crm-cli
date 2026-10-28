# Modules: turn views on or off and reorder them

Every view of the terminal UI is a module: home, people, follow-up, tasks, organizations, birthdays, merge, archived, sync, deals and settings. You choose which are shown and in what order. Off hides a module from the tabs, the number keys, `[` and `]` and the palette, and moves it under Hidden in the Views screen (`v`). Nothing is deleted: its data stays, links still open its record pages (an organization from a person page works with Organizations off), and turning it on again brings it back.

```sh crm-example
crm modules list
crm modules off birthdays merge
crm modules on birthdays
crm modules move organizations 3
crm modules reset
```

The default order is Home, People, Follow-up, Tasks, Organizations, Deals, Birthdays, Merge, Archived, Sync, Settings. **Archived and Sync are hidden by default**, so a new setup has nine tabs. They moved under Settings: the rows "Open Archived" and "Open Sync" in the Settings view, the bar (type `archived` or `sync`) and the Hidden group of the Views screen all open them. `crm modules on archived sync` shows them again. A `[modules]` table that is already in your `config.toml` is respected as it is written, so nothing you arranged changes. Settings is a module that never turns off (`crm modules off settings` is refused), and the Views screen is the `v` key, not a module, so you can always get back to both.

The number key of a module is its place among the modules that are shown (1 to 9, then 0 for the tenth). One module must stay on, Home or People, so there is always a start screen.

## In the terminal UI

Press `v` for the Views screen. It lists every view: the shown ones first with their number key, then a Hidden divider and the hidden ones. `enter` goes to the view, `x` shows or hides it, `shift+up` and `shift+down` move it up and down inside its group, `esc` closes. The palette entry "Modules: reset" puts the default arrangement back. The separate Modules screen is gone. A key you rebound with `crm keys set` stays yours.

## Where it is stored

Per device, in the `[modules]` table of `~/.crm/config.toml`, next to `[keys]` and the theme:

```toml
[modules]
order  = ["home", "people", "follow-up", "tasks", "organizations", "deals", "birthdays", "merge", "archived", "sync", "settings"]
hidden = ["birthdays"]
```

A name that does not exist, bad TOML, an arrangement with no start screen or a `hidden` list that names Settings is reported and that part is ignored. MCP: `crm_modules_list`, `crm_modules_set` (`module`, `on`, `position`), `crm_modules_reset`.

See also: tui, tui-keys, keys, orgs.
