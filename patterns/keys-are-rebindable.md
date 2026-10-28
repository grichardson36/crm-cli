---
type: pattern
scale: interaction
status: decided
---

# Keys are rebindable with conflict detection

**Problem.** Defaults collide with terminal or user habits.

**Therefore.** The [keys] table in config.toml and crm keys list, set, reset, path rebind keys. Conflicts are reported when setting.

**Larger patterns.** [[same-key-same-verb]]

**Sources.** Obsidian hotkeys. Feature M08

**Inspired by.** [[vim-and-lazygit]]

**Update 2026-10-27 (feat/keymap-a).** The conflict check compares scopes and modes, not action groups: two actions conflict only when they share a key in the same effective scope and mode ([[keymap-layers-and-scopes]]). `crm keys set` and the Keys: customize flow warn about chords terminals swallow (`ctrl+i`, `ctrl+m`, `ctrl+h`, `ctrl+[`, `ctrl+j`, the `ctrl+shift` arrows). An old `[keys]` file that names a gone action or a key now owned by the framework is reported and skipped, never a crash. `crm keys list` shows each action's layer and scope.
