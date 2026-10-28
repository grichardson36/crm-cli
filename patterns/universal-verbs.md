---
type: pattern
scale: interaction
status: decided (owner 2026-10-27 and 2026-10-28), built in feat/keymap-a and feat/keymap-b
---

# Universal verbs

**Problem.** Every object grew its own add key, edit key and mark key (`t`, `n`, `+`, `c`, `e`, `M`, `L`), so a key meant a different verb per view and users relearned each screen ([[same-key-same-verb]]).

**Therefore.** A small set of verbs means the same thing in every view and on every object. The framework layer owns them ([[keymap-layers-and-scopes]]). A verb that cannot apply does nothing. Every function a retired key had stays in the palette or on a ctrl chord, so no function is lost.

| Key | Verb | What it does |
|---|---|---|
| `a` | add | Opens a picker of kinds with the view's own kind preselected, so `a` then `enter` adds the view's own object. Typing a kind's letter picks it: `a t` task, `a n` note, `a o` organization, `a p` person, `a d` deal, `a g` tag. Each kind declares its letter in the object registry (`objDef.shortcut`, default the first letter, else the next free letter of its name) and a test keeps them unique. Links are pipe segments of the add prompt ([[one-line-pipe-prompt]]): with an object highlighted, it is the first suggestion of the segment that takes its kind (a task's people, a deal's organizations, a tag's objects) and `tab` accepts it. Several objects are separated by commas, backspacing over a name is the unlink, and `enter` links every name. There is no `l`/`u` chip: `tab` accepts the highlighted suggestion while the popup is open ([[autocomplete-component]]). Owner decision 2026-10-29. A new tag is the prompt `name | on`: an existing name picks the tag, a new name creates it, and the objects in `on` get it. |
| `enter`, `e` | activate | One action id. Open the highlighted thing further, or edit the field under the cursor. Rename is this. |
| `d` | delete | Soft delete of the highlighted object, undoable. In the Archived view it purges the object, of any type, after its name is typed. Purge is a verb of every archivable type. |
| `r` | restore | Brings back an archived or soft-deleted object (the Archived screen). |
| `l` | link | Links the highlighted object to another. Tags are objects, so tagging is linking. |
| `u` | unlink | Removes a link. |
| `x` | mark | One meaning per type, no menu: a task is done (again reopens it), a person is contacted. |
| `f` | favorite | Where the type supports it. |
| `m` | merge | Acts on the `space`-selected rows, or opens the compare for the highlighted duplicate group. |
| `space` | select | Multi-selects rows. `shift+up` and `shift+down` extend the selection, `ctrl+a` selects all visible. Every verb acts on the selection, else on the highlighted row ([[bulk-actions-always]]). |
| `o` | open outside | Opens the focused thing outside the app, for any object: a URL, mail address or phone number opens the browser, mail app or dialer, anything else a web search (`[open] search_url` in `config.toml`). `enter` stays "open further inside the app". |
| `ctrl+c` | copy | Copies the focused field's value in the detail pane. Elsewhere it keeps its old job, quit in a list and cancel in a prompt. Palette: Copy value, Copy contact as vCard (`ctrl+shift+c`, which many terminals keep for themselves). |
| `ctrl+z`, `ctrl+y` | undo, redo | No bare `z` and no vim `u`. |
| `ctrl+r` | refresh | Reload moved off bare `r`, which is restore. |

**The first-letter rule.** A verb's key is its first letter: add, delete, restore, link, unlink, favorite, merge. `enter` and `e` are activate because open and edit are one gesture. Mark and merge both start with m, so the two exceptions are decided: `m` is merge and `x` is mark. There is no heavier uppercase twin any more: the attributes of a link (role, dates, kind) and the type of a note are cells of the linked row, edited with `enter` or `e` like any field ([[field-types-drive-editors]]).

**Not verbs.** Rename as a separate verb does not exist. Bare `z`, `u` for undo, `j k h l g G`, `t`, `n`, `+`, `*`, `E`, `y`, `F` and every per-object add key are retired. `ctrl+a` is select all.

**Because.** Owner decision 2026-10-27. One verb per key lets the footer, the bar results and `?` show the same key for the same job, and it is checked by `TestKeyAudit` ([[audit-keybindings-2026-10]]).

**Larger patterns.** [[one-verb-set]], [[objects-first]], [[ctrl-moves-the-view-bare-moves-the-object]]

**Smaller patterns.** [[keymap-layers-and-scopes]], [[navigation-units-and-homes]], [[keys-are-rebindable]], [[show-the-key-beside-the-action]], [[bar-results-act-like-views]], [[enrichment-is-suggested-then-accepted]]

**Sources.** Owner decision 2026-10-27. Comparison tables in [[audit-keybindings-2026-10]]

**Inspired by.** [[vim-and-lazygit]], [[linear]], [[raycast]]
