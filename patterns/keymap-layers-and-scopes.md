---
type: pattern
scale: interaction
status: decided (owner 2026-10-27), built in feat/keymap-a
---

# Keymap layers and scopes

**Problem.** One flat action list with groups. A key conflicted with anything in a "global" group and with nothing else, so rebinding was either too strict or too loose, and a crm feature such as snooze could quietly take a key the whole interface relies on.

**Therefore.** Two layers and named scopes.

- **Layers.** The framework layer holds the universal verbs, navigation, help, the palette and view switching ([[universal-verbs]], [[navigation-units-and-homes]]). The app layer holds what is specific to this CRM: snooze, cadence, birthdays, the deal pipeline, sync, merge compare. The app layer may add actions but may not use a key the framework layer owns, enforced by `TestAppLayerKeepsOffFrameworkKeys`.
- **Scopes.** `app` (everywhere), a view type (`type:person`, `type:task`, `type:organization`, `type:deal`, `type:dupes`, `type:archive`, `type:inbox`, `type:tag`, `form`, `compare`) and one view (`view:birthdays`). A key is looked up from the most specific scope of the current context outward, and a scope can erase an inherited key so the lookup stops there. The scope table and the layer table are in `internal/tui/keymap.go`, one place each.
- **Modes.** Normal keys and text-entry keys (a footer prompt) are separate. A key typed into a field never conflicts with a bare key.
- **Conflicts.** Two actions conflict only when they share a key in the same effective scope and mode. A person action and a Sync action may share a letter because the two views never show together. An action in a narrower scope may deliberately answer a key a wider scope also has, and the audit lists each such shadow in `testdata/keyaudit_allow.txt` with a reason. After keymap-a the list has one line (`q` leaves the compare screen instead of quitting).

**Checks.** `go test ./internal/tui` fails on a new conflict (`TestKeyAudit`), on two actions sharing a key in one scope (`TestNoTwoActionsShareAKeyInOneScope`), on an app action using a framework key and on a stale allow-list line. `?` lists the keys live in the current view and mode, grouped by layer. `ConflictFor` and `crm keys set` use the same overlap rule, and warn about chords terminals swallow (`ctrl+i`, `ctrl+m`, `ctrl+h`, `ctrl+[`, `ctrl+j`, the `ctrl+shift` arrows).

**Old key files.** A `[keys]` entry that names an action or key that no longer exists is reported by `crm keys list` and in the status line and skipped. The file format is unchanged and the rest of the file still applies. An action that lost its default key (tier, sync run) can still be bound by name.

**Because.** Owner decision 2026-10-27. Sharing one commands registry between the keymap, the bar and the palette is a later step.

**Larger patterns.** [[keys-are-rebindable]], [[same-key-same-verb]], [[objects-first]]

**Smaller patterns.** [[audit-keybindings-2026-10]], [[show-the-key-beside-the-action]], [[surface-gaps-before-deciding]]

**Sources.** Owner decision 2026-10-27. `internal/tui/keymap.go`, `internal/tui/keyaudit2_test.go`

**Inspired by.** [[vim-and-lazygit]], [[obsidian]]
