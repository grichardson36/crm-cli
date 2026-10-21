# Modules: turn views on or off and reorder them

Every view of the terminal UI is a module: home, people, follow-up, tasks, organizations, birthdays, merge, archived, sync and deals. You choose which are on and in what order. Off hides a module from the tabs, the number keys, `[` and `]`, the view list (`v`) and the palette. Nothing is deleted: its data stays, links still open its record pages (an organization from a person page works with Organizations off), and turning it on again brings it back.

```sh crm-example
crm modules list
crm modules off birthdays merge
crm modules on birthdays
crm modules move organizations 3
crm modules reset
```

The default is loose: every module is on, in the order Home, People, Follow-up, Tasks, Organizations, Deals, Birthdays, Merge, Archived, Sync. You hide and reorder from there. The number key of a module is its place among the modules that are on (1 to 9, then 0 for the tenth). One module must stay on, and Home or People, so there is always a start screen.

## In the terminal UI

Open "Modules" from the palette (`ctrl+k`). `space` or `enter` turns the module under the cursor on or off, `K` and `J` move it up and down, `r` puts the default arrangement back, `esc` closes. The key of each module shows beside it. A key you rebound with `crm keys set` stays yours.

## Where it is stored

Per device, in the `[modules]` table of `~/.crm/config.toml`, next to `[keys]` and the theme:

```toml
[modules]
order  = ["home", "people", "follow-up", "tasks", "organizations", "deals", "birthdays", "merge", "archived", "sync"]
hidden = ["birthdays"]
```

A name that does not exist, bad TOML or an arrangement with no start screen is reported and the default is used for that part. MCP: `crm_modules_list`, `crm_modules_set` (`module`, `on`, `position`), `crm_modules_reset`.

See also: tui, tui-keys, keys, orgs.
