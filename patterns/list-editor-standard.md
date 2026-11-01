---
type: pattern
scale: screen
status: decided by owner 2026-10-30, built in feat/list-editor (schema head 18, migration file 019)
---

# One list editor modifies every user-managed list

**Problem.** Deal stages, deal roles and note types each had their own Manage screen with a picker, a sub-menu and a prompt, and relation kinds had only a CLI. The Views screen already showed the better way: a keyboard-centric grouped list. Four lists, four ways to rename, order and remove.

**Therefore.** One component, the list editor (`internal/tui/listeditor.go`), edits every user-managed list ([[one-abstraction-many-contexts]]). A list is one descriptor (`listDesc`): title, the word for an item, what uses an item, optional groups, optional per-item cells, the storage functions (load, add, rename, setCell, reorder, remove) and the removal rules. A new list is one more descriptor in `internal/tui/listeditor_lists.go`.

| Key | Does |
|---|---|
| `a` | adds an item inline: type the name, `enter` saves, `esc` cancels. In a grouped list it goes to the group under the cursor |
| `enter`, `e` | edits the cell under the cursor. On the name it renames inline and the records that use it follow |
| `tab`, `shift+tab` | move across the cells of the row (name, then the descriptor's cells) |
| `shift+up`, `shift+down` | move the item inside its group |
| `d` | removes the selected items, else the highlighted one |
| `space`, `ctrl+a` | select, select all ([[bulk-actions-always]]) |
| `ctrl+z`, `ctrl+y` | undo, redo |
| `esc` | cancels an open input, else closes |

No new bare key ([[universal-verbs]]). A cell is a field from the field vocabulary ([[field-types-drive-editors]]): a choice cell shows its options in the row (`left`, `right`, `enter` saves), a text cell is an inline input, both checked by `validate.Field`.

**Removal with `d`.**

1. A rule can block it with a clear message (below).
2. Nothing uses the item: it goes at once (soft delete or archive as before). Several items ask `y/n` once.
3. Items are in use: a picker built from the same list asks where the records go. Rows: every remaining item of the group (`Move them to X`), then the extras the kind allows: blank, keep, unlink. The count of affected records is in the question.
4. A one-key `y/n` confirms. The whole removal, items and records, is ONE undo step: the repository snapshots the rows it touches (`RemoveMany`, `UndoRemoval`, `RedoRemoval`), including a deal's close date.

| List | Move to another | Blank | Other | Rule |
|---|---|---|---|---|
| Deal stages | yes | no, a deal always holds a listed stage | | at least one open, one won and one lost stage must remain ([[deal-stages]]) |
| Deal roles | yes | yes, the role is cleared on the links | | |
| Note types | yes | no, a note always has a type (the column is required) | keep the type on the old notes, the type is archived (as before) | `note` is the default and stays |
| Relation kinds | yes | no | unlink them (the links are deleted, undo restores) | built-in kinds stay. A move onto a kind that already links the same two records is refused |

Cells: stages have outcome (open, won, lost), note types have counts as contact, relation kinds have inverse name and mutual. A kind that links use cannot change between mutual and directional. Built-in kinds are only ordered.

**Order.** Stages and roles already had `vocabulary.position`. Migration file 019 adds `position` to `note_types` and `relation_kind` (today's order is kept), new items go last.

**Doors.** `openListEditor(id)` with ids `deal-stages`, `deal-roles`, `note-types`, `relation-kinds`. Palette entries Edit deal stages, Edit deal roles, Edit note types, Edit relation kinds (same ids), the bar finds them, Settings rows call the same function ([[settings-view]]). The Manage screens and their prompts are gone. CLI and MCP are unchanged except `crm relation-kind rename`, `set --inverse` and `rm --move-to|--unlink` for parity with the editor.

**Because.** Owner request 2026-10-30: one consistent standard for modifying lists, with the keyboard model of the Views screen.

**Larger patterns.** [[one-abstraction-many-contexts]], [[universal-verbs]], [[bulk-actions-always]]

**Smaller patterns.** [[views-screen]], [[settings-view]], [[deal-stages]], [[deal-roles]], [[field-types-drive-editors]], [[relation-kinds-have-inverses]]

**Sources.** Owner request 2026-10-30. `internal/tui/listeditor.go`, `internal/tui/listeditor_lists.go`, `internal/db/repo/list_removal.go`, `internal/db/repo/kind_edit.go`
