---
type: pattern
scale: screen
status: decided by owner 2026-10-29, built in feat/settings-views
---

# Settings is one module over one registry

**Problem.** Settings were scattered: theme and keys in the palette, the stages and roles in their own screens, the search address only in `config.toml`, and a short list under the `=` prefix of the bar that nothing else shared. A user could not see what is configurable or where it is stored.

**Therefore.** Settings is a module (id `settings`, key 9 in the default arrangement, last in the default order, always on, see [[views-screen]]). It is a grouped list ([[grouped-lists-with-dividers]]) with one divider per section, built from a single settings registry in `internal/tui/settings.go`. The `=` settings of the command bar read the same registry ([[one-bar]]). There is no second list.

**One declaration per setting.** Each entry has an id, a section, a label, a description, search words, a field type, a scope and how it is read and written, through code that already existed:

| Part | Meaning |
|---|---|
| section | Appearance, Keys, Views, Deals, Types and vocabularies, Names, Behavior, Data, Sync, Archive |
| field | a spec from the field-type vocabulary ([[field-types-drive-editors]]) when the value can be edited in place |
| action, run | the existing registry action or the small function that opens the screen behind it (theme choose, deal stages, doctor) |
| value | the current value shown at the row and in the bar |
| scope | where it is stored, below |

**Editing.** A field setting is edited in place with the same editors as a record page: `enter` or `e` on the row, `enter` saves, `esc` cancels, `d` resets after a yes or no. Short syntax is a choice, the web search address is text. A setting without a field opens its screen on `enter` ([[universal-verbs]]). Read-only mode blocks edits.

**Scope.** Every row shows its scope as a dim tag at the right:

- `device`: stored in `config.toml` on this computer (theme, keys, views, web search address). Other devices keep their own.
- `synced`: stored in the database, so it travels with the database to other devices (deal stages, deal roles, note types, the name alias table, the ignored company texts, the archive).
- `session`: not stored, it lasts until the program closes (short syntax, read-only).

The detail pane says the same in a sentence. A new setting declares its scope once, where it is declared.

**Archived and Sync.** Settings has the rows "Open Archived", "Open Sync" and "Inbox". The two views are hidden by default and reachable from here ([[views-screen]]).

**What has no storage yet.** The registry lists only what exists. Relation kinds have no management screen, the name alias table has a CLI (`crm alias`) and no TUI screen (its row shows a message), short syntax and read-only are not saved between runs. They are listed in [[open-discrepancies]] as gaps, not invented here.

Built (M32): `internal/tui/settings.go` (`settingList`, `settingRows`, `buildSettingForm`, `openSetting`), `modules.SetValue` for plain text device settings, `db.ListBackups` for the Backups row, `viewSettings` in `internal/tui/model.go`. Tests in `settings_view_test.go`.

**Because.** Owner request 2026-10-29: Settings module, grouped, one registry for the bar and the view, scope visible on each row.

**Larger patterns.** [[modules-are-user-arranged]], [[one-bar]], [[field-types-drive-editors]]

**Smaller patterns.** [[views-screen]], [[deal-stages]], [[deal-roles]], [[keys-are-rebindable]]

**Sources.** Owner request 2026-10-29. `internal/tui/settings.go`, `internal/modules/config.go`, `internal/db/backup.go`
