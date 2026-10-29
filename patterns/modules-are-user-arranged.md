---
type: pattern
scale: product
status: decided
---

# The user arranges the modules and turning one off loses nothing

**Problem.** A fixed set of tabs forces one person's idea of the workflow on everyone. Hiding a feature by deleting its data is unsafe.

**Therefore.** Every view and feature area is a module the user can turn on or off and place in any order. The program ships a default arrangement. Off hides the module from navigation, the bar and the palette. Its data, links and history stay and every other module still reads them. Turning it on again restores everything. The arrangement is stored per device in config, never in the data, and is changeable from the CLI, MCP and TUI.

Built (M14): `[modules] order, hidden` in config.toml, `crm modules`, `crm_modules_*`. Changed 2026-10-29 (M31, M32): the TUI has one Views screen on `v` that lists, opens, shows, hides and orders the views ([[views-screen]]), so the separate Modules screen and its palette entry are gone and "Modules: reset" stays. Settings is a module that is always on and last in the default order ([[settings-view]]). Archived and Sync are hidden by default and reached from Settings, the bar and the Views screen. An explicit `[modules]` table in config.toml is respected as written. The default is otherwise loose: the order is Home, People, Follow-up, Tasks, Organizations, Deals, Birthdays, Merge, Archived, Sync, Settings, and the user hides and reorders from there (owner 2026-10-20, [[advanced-objects-off-main-path]]). The number key of a view is its place among the modules that are on, 1 to 9 and 0 for the tenth. On a narrow terminal tabs shrink to their key and the rest fold into a "v +N" entry that opens the Views screen. A module that is off is still reachable from a link or a record page (it shows as "v Name" in the tab bar), because hiding is for navigation and not for data. One module must stay on, Home or People, so there is always a start screen, and Settings can never be turned off, so the user cannot lock themselves out. Company review is a screen under Organizations, not a module.

**Because.** Owner decision 2026-10-19: "User maintains autonomy. Plug and play modules on or off, in whatever order." This is how an infinitely extensible local-first CRM stays the user's own.

**Larger patterns.** [[local-first]], [[extension-ladder]]

**Smaller patterns.** [[views-screen]], [[settings-view]], [[saved-views]], [[advanced-objects-off-main-path]], [[keys-are-rebindable]], [[objects-first]]

**Sources.** Owner decision 2026-10-19

**Inspired by.** [[notion]], [[obsidian]], [[raycast]], [[hubspot]]
