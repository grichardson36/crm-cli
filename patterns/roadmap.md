---
type: roadmap
---
# Roadmap

Items here are not built. Decisions behind them live in this vault, start at [[index]].

## Next

- Custom themes ([[custom-themes]], design proposed on branch feat/themes-more): one file with `[dark]` and `[light]` tables, `base` of a custom theme, snippets in `~/.crm/themes/snippets`, theme checks in `internal/validate` with contrast warnings (`crm theme check`), a file watcher for live reload, `crm theme export` and `import`, a picker detail pane with a rendered sample.
- Theme generator in the style of the theme-factory skill: `crm theme generate` takes a name, a mood or a few seed colors (accent, background) and writes both variants with roles derived from the seeds, checks contrast (4.5:1 text, 3:1 kinds) and the 16-color mapping, then shows the sample in the picker before saving ([[custom-themes]]).
- Saved views follow-ups ([[saved-views]], built on branch feat/saved-views): saved views between the built-in tabs and a number key when all ten are taken, the bar search staying inside a saved view, OR groups in a filter, saving Follow-up and Birthdays as views, one text grammar shared by the CLI and the TUI search bar (B07).
- Settings gaps found while building the Settings view ([[settings-view]]): a TUI screen for the name alias table (CLI only today), a management screen for relation kinds (none), and saving short syntax and read-only between runs (session only today).
- Device sync follow-ups ([[replica-package]], built on branch feat/device-sync): a Peers view and a review queue screen in the terminal UI with palette actions (TUI parity), a Home count of open review items, a record created independently on two devices with one unique name after sync is on (tags, emails) adopts by name instead of staying blocked, a streamed snapshot endpoint and long-poll pull for very large databases, `tsnet` instead of tokens if the owner wants tailnet identity.

## Someday / maybe

- User-defined commands (shell templates like `lazygit` custom commands). The verb set stays fixed for now ([[research-tui-building-blocks]], Q30).
