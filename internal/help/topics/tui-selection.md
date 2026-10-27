# Terminal UI: selection, bulk actions, cells, copy and open

## Selection and bulk actions

Every verb acts on the selected rows, and on the highlighted row when nothing is selected, for every object type.

- `space` selects a row and moves down. `shift+up` and `shift+down` extend the selection by a row (back over a row takes it out). `ctrl+a` selects every visible row, again to clear. The sort and filter of the view (`ctrl+o`) decide what is visible. In the Modules screen `shift+up` and `shift+down` still move a module.
- `d`, `r`, `x`, `f`, `l`, `u`, `m` and the palette entries Tag, Set keep-in-touch and Set field on selected apply to all selected rows. `l` and `u` link and unlink every selected object to the one target you pick, with the role and dates asked once.
- Delete, unlink and purge of several objects show the count and ask for it typed back, for example `delete 5`. One object keeps the plain y/n (a purge asks for its name).
- Set field on selected (palette) sets a choice, date or number field on the selection: the priority tier of people, stage and value of deals, due date and priority of tasks. The value is checked by the type of the field.

## Cells

A relation field shows its linked rows as a small table. The first column is the linked object (`enter` opens it), the others are the attributes of the link: role, from, to, primary, kind, notes, and for a deal its stage and value. A note row has its type as a cell. `right` and `left` (and `tab`, `shift+tab`) move across the cells, `enter` or `e` edits the one under the cursor by its type, and each edit is one undo step. From the first cell `left` goes back to the list, and so does `tab` from the last cell. There is no `E` key.

## Copy and open

- `ctrl+c` copies the value of the focused field when a field is focused (the detail pane). Everywhere else it keeps its old job: it quits from a list and cancels in a prompt or picker.
- The palette has Copy value and Copy contact as vCard. `ctrl+shift+c` is bound to the vCard copy, but terminals such as Windows Terminal keep that chord for themselves, so use the palette entry.
- `o` opens the focused thing outside the app, for every object. A URL, mail address or phone number opens the browser, the mail app or the dialer. Any other text, or an object with no link of its own, opens a web search for it. `enter` stays "open further inside the app".
- The search address is a template in `config.toml`: `[open]` `search_url = "https://duckduckgo.com/?q={query}"`. The default is Google.

See also: tui-keys, tui-key-changes, links, purge, keys.
