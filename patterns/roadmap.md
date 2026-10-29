---
type: roadmap
---
# Roadmap

Items here are not built. Decisions behind them live in this vault, start at [[index]].

## Next

- Saved views (owner request 2026-10-29): user-created modules with a filter, sort and grouping over any object kind, shown in the Views screen, stored in the database so they sync, shareable as TOML. Not built, see [[saved-views]].
- Settings gaps found while building the Settings view ([[settings-view]]): a TUI screen for the name alias table (CLI only today), a management screen for relation kinds (none), and saving short syntax and read-only between runs (session only today).
- Device sync follow-ups ([[replica-package]], built on branch feat/device-sync): a Peers view and a review queue screen in the terminal UI with palette actions (TUI parity), a Home count of open review items, a record created independently on two devices with one unique name after sync is on (tags, emails) adopts by name instead of staying blocked, a streamed snapshot endpoint and long-poll pull for very large databases, `tsnet` instead of tokens if the owner wants tailnet identity.

## Someday / maybe

- User-defined commands (shell templates like `lazygit` custom commands). The verb set stays fixed for now ([[research-tui-building-blocks]], Q30).
