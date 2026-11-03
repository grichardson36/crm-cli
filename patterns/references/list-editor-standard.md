---
type: reference
status: decided by owner 2026-10-30, built in feat/list-editor (schema head 18, migration file 019)
---

# One list editor modifies every user-managed list

**Problem.** Deal stages, deal roles and note types each had their own Manage screen with a picker, a sub-menu and a prompt, and relation kinds had only a CLI. The Views screen already showed the better way: a keyboard-centric grouped list. Four lists, four ways to rename, order and remove.

**Therefore.** One component, the list editor (`internal/tui/listeditor.go`), edits every user-managed list ([[interface-conventions]]). A list is one descriptor (`listDesc`): title, the word for an item, what uses an item, optional groups, optional per-item cells, the storage functions (load, add, rename, setCell, reorder, remove) and the removal rules. A new list is one more descriptor in `internal/tui/listeditor_lists.go`.

The list lives ONLY in the right pane of its Settings row (owner 2026-11-03: lesser used configuration stays in the pane, no popup, no standalone key or palette entry). `enter` on the row enters the pane, `esc` leaves it ([[navigation-standard]]). The footer shows a key only when it works on the highlighted row, one word per key.

| Key | Does |
|---|---|
| `up`, `down` | select |
| `a` | adds an item inline: type the name, `enter` saves, `esc` cancels. In a grouped list it goes to the group under the cursor |
| `e`, `enter` | renames inline and the records that use the name follow. Not on a built-in item |
| `tab` | cycles the value of the row: outcome of a stage (open, won, lost), mark contacted of a note type (yes, no). On Links it moves between the name and the inverse, `e` then edits it |
| `shift+up`, `shift+down` | move the item inside its group |
| `d` | deletes the selected items, else the highlighted one. Not on a built-in item |
| `shift+r` | resets to the built-in defaults after a confirm that lists what changes. One undo step |
| `space`, `ctrl+a` | select, select all ([[keyboard-and-feedback]]) |
| `ctrl+z`, `ctrl+y` | undo, redo |
| `esc`, `left`, `shift+tab` | cancel an open input, else leave the pane |

No new bare key outside the pane ([[keyboard-and-feedback]]). A cell is a field from the field vocabulary ([[interface-conventions]]), checked by `validate.Field`. A choice cell cycles with `tab`, a text cell is an inline input.

**Columns.** Every list shows a name column, its cells and `used by`, the count of records that use each item. The count is queried with the repositories and never stored. Links has two groups (People, Organizations) and the column `inverse`: a name equal to its inverse is mutual. Notes has `mark contacted` (the `counts_as_contact` flag). Deal stages has `outcome`. Ignored company texts has `state` and no `used by`.

**Reset.** `shift+r` restores the built-in items that are missing, sets the cells of built-in items back to their defaults and puts the built-in items first in their default order. Items the user added stay after them and nothing is removed, so no record loses its value. The confirm lists each change (`add lead`, `negotiation outcome open`, `order`). The whole reset is one undo step.

**Removal with `d`.**

1. A rule can block it with a clear message (below).
2. Nothing uses the item: it goes at once (soft delete or archive as before). Several items ask `y/n` once.
3. Items are in use: a picker built from the same list asks where the records go. Rows: every remaining item of the group (`Move them to X`), then the extras the kind allows: blank, keep, unlink. The count of affected records is in the question.
4. A one-key `y/n` confirms. The whole removal, items and records, is ONE undo step: the repository snapshots the rows it touches (`RemoveMany`, `UndoRemoval`, `RedoRemoval`), including a deal's close date.

| List | Move to another | Blank | Other | Rule |
|---|---|---|---|---|
| Deal stages | yes | no, a deal always holds a listed stage | | at least one open, one won and one lost stage must remain ([[deal-vocabulary]]) |
| Deal roles | yes | yes, the role is cleared on the links | | |
| Note types | yes | no, a note always has a type (the column is required) | keep the type on the old notes, the type is archived (as before) | `note` is the default and stays |
| Relation kinds | yes | no | unlink them (the links are deleted, undo restores) | built-in kinds stay. A move onto a kind that already links the same two records is refused |

Cells: stages have outcome (open, won, lost), note types have mark contacted, relation kinds have the inverse name. A kind that links use cannot change between mutual and directional. Built-in kinds are only ordered.

**Order.** Stages and roles already had `vocabulary.position`. Migration file 019 adds `position` to `note_types` and `relation_kind` (today's order is kept), new items go last.

**Doors.** The Settings rows `deal-stages`, `deal-roles`, `note-types` (shown as Notes), `relation-kinds` (shown as Links) and `company-ignore` open `m.openListPane(id)`, which puts the cursor in the pane. The bar `=` settings list reaches the same rows. There is no palette action, key or popup for a list, and the Manage screens and their prompts are gone ([[UI-modules]]). CLI and MCP are unchanged except `crm relation-kind rename`, `set --inverse` and `rm --move-to|--unlink` for parity with the editor.

**Because.** Owner request 2026-10-30: one consistent standard for modifying lists, with the keyboard model of the Views screen.

**Larger patterns.** [[core]], [[interface-conventions]]

**Smaller patterns.** [[keyboard-and-feedback]], [[deal-vocabulary]], [[UI-modules]], [[link-registry]]

**Sources.** Owner request 2026-10-30. `internal/tui/listeditor.go`, `internal/tui/listeditor_lists.go`, `internal/db/repo/list_removal.go`, `internal/db/repo/kind_edit.go`
