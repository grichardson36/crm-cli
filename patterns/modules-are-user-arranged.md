---
type: pattern
scale: product
status: decided
---

# The user arranges the modules and turning one off loses nothing

**Problem.** A fixed set of tabs forces one person's idea of the workflow on everyone. Hiding a feature by deleting its data is unsafe.

**Therefore.** Every view and feature area is a module the user can turn on or off and place in any order. The program ships a default arrangement. Off hides the module from navigation, the bar and the palette. Its data, links and history stay and every other module still reads them. Turning it on again restores everything. The arrangement is stored per device in config, never in the data, and is changeable from the CLI, MCP and TUI.

Built (M14): `[modules] order, hidden` in config.toml, `crm modules`, `crm_modules_*`, the palette entry "Modules". The default is loose: every module is on, in the order Home, People, Follow-up, Tasks, Organizations, Deals, Birthdays, Merge, Archived, Sync, and the user hides and reorders from there (owner 2026-10-20, [[advanced-objects-off-main-path]]). The number key of a view is its place among the modules that are on, 1 to 9 and 0 for the tenth. On a narrow terminal tabs shrink to their key and the rest fold into a "v +N" entry that opens the view list. A module that is off is still reachable from a link or a record page (it shows as "v Name" in the tab bar), because hiding is for navigation and not for data. One module must stay on, and Home or People, so there is always a start screen. Company review is a screen under Organizations, not a module.

**Because.** Owner decision 2026-10-19: "User maintains autonomy. Plug and play modules on or off, in whatever order." This is how an infinitely extensible local-first CRM stays the user's own.

**Larger patterns.** [[local-first]], [[extension-ladder]]

**Smaller patterns.** [[advanced-objects-off-main-path]], [[keys-are-rebindable]], [[objects-first]]

**Sources.** Owner decision 2026-10-19

**Inspired by.** [[notion]], [[obsidian]], [[raycast]], [[hubspot]]
