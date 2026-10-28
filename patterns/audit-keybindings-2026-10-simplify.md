---
type: audit
status: decided by the owner 2026-10-28, applied in feat/keymap-b
date: 2026-10-27
---
# Audit: keys to simplify, 2026-10-27

## Outcome (owner decisions 2026-10-28, applied in feat/keymap-b)

All clear removals (C) are applied. Decisions on the D items:

| Code | Decision |
|---|---|
| F1 | Removed `ctrl+n`, `ctrl+p`. |
| F2 | Kept `ctrl+d`, `ctrl+u` (half page). |
| F3 | Removed `:`. |
| F4 | `y` removed. `ctrl+c` copies the focused field, palette "Copy value" and "Copy contact as vCard". |
| F5 | Kept, and widened: `o` opens anything outside the app (URL, mail, phone, else a web search). |
| F6 | `E` removed. Link attributes and the note type are cells of the linked row. |
| F7 | `F` moved to the palette. |
| F8 | `*` removed. `ctrl+a` selects all, `shift+up`/`shift+down` extend. |
| F9, F10, F11 | D1: kept the digits, `[ ]` and `v`. Added `ctrl+left` and `ctrl+right` as aliases of `[` and `]`. |
| F12 to F14 | D2: one `ctrl+o` view options overlay for every list view (Sort, Archived type, Merge minimum level). `ctrl+l` and `ctrl+f` removed. |
| F16, F17 | `ctrl+b`, `ctrl+w` moved to the palette. |
| F18, F20 | `q` removed in the compare and the text viewer. In the compare `q` now reaches the app quit key. |
| F19 | `shift+tab` and the `tab` toggle removed from the compare. `tab` cycles the fields, `left`/`right` choose the side. |
| F21, F22 | `J`, `K` and `r` removed from Modules. The reset is the palette entry "Modules: reset". |
| F15, F23 to F28 | Kept. |

Counts after B2: see the generated table (`KEYS_AUDIT_OUT`), the old-to-new table is in [[current-tui-keys]] and `crm help tui-keys`.

The text below is the original proposal, kept for the reasoning.

Read-only pass on main head a04adc5. Source of truth is the action registry (`internal/tui/actions.go`, `verbs.go`, `links.go`, `globaladd.go`, `orgmerge.go`, `tagops.go`, `barverbs.go`) and the generated table from `TestKeyAudit` (`KEYS_AUDIT_OUT`, run today, 64 distinct keys in normal mode). Mode handlers outside the registry (`overlay.go`, `tools.go`, `modules.go`, `model.go`) were read too. Earlier audit: [[audit-keybindings-2026-10]]. Rules applied: [[ctrl-moves-the-view-bare-moves-the-object]], [[universal-verbs]], [[navigation-units-and-homes]], [[keymap-layers-and-scopes]], [[same-key-same-verb]]. Comparisons: [[research-tui-inspirations]].

Usage is an estimate. There is no key telemetry. High means several times per session, medium daily, low weekly or less.

## Fixed set (not audited)

`a d e/enter r l u x f m s space`, `ctrl+z ctrl+y ctrl+r`, `up down left right tab shift+tab`, `ctrl+up/down`, `ctrl+shift+up/down`, plus `esc`, `ctrl+c`, `?`, `home end pgup pgdn` (fallbacks and hardware keys, kept).

## Findings: every other bound key

Code `C` means clear (apply it). `D` means owner decision.

| Code | Key | Action | Active in | Use | Generic verb or palette today | Recommendation | Status |
|---|---|---|---|---|---|---|---|
| F1 | `ctrl+n` `ctrl+p` | down, up | all lists, bar, overlays, command line, help | low (arrows exist) | arrows | Remove. Arrows are the navigation set. | C |
| F2 | `ctrl+d` `ctrl+u` | half page | lists, help, text viewer | low | `pgdn`/`pgup`, `ctrl+up/down` | Remove. Three paging keys remain (`pgup pgdn`, ctrl+arrows). [[navigation-units-and-homes]] currently lists them. | D |
| F3 | `:` | open palette | all lists | low | `/` and `ctrl+k` do the same | Remove. Keep `/` (search) and `ctrl+k`. | C |
| F4 | `y` | copy row value | detail form (person, org, deal) | medium | palette "Copy value" | Move to palette. Frees bare `y` for the confirm prompt only. | D |
| F5 | `o` | open row as URL, mailto, tel | detail form | medium | palette "Open link" | Move to palette. Alternative: keep, since it is the point of a contact row. | D |
| F6 | `E` | edit link or note type | detail form, linked or note row | low | palette "Edit link or type", "Change note type" | Move to palette. Removes the only uppercase object key. [[universal-verbs]] documents `E` as the twin of `e`. | D |
| F7 | `F` | set keep-in-touch cadence | people views | low (set once per person) | palette "Set keep-in-touch"; tier is already palette only | Move to palette. Same treatment as tier. | C |
| F8 | `*` | mark all visible | all lists | low | palette "Mark all visible" | Move to palette. | D |
| F9 | `0` to `9` | go to the Nth module | all lists | high | `v` picker, `[` `]`, palette view entries | See D1 below. | D |
| F10 | `[` `]` | previous, next module | all lists | medium | digits, `v` | Keep (D1). Option: `ctrl+left` `ctrl+right`, which follows the ctrl rule and pairs with `ctrl+up/down`. | D |
| F11 | `v` | module picker, digit hints inside | all lists | medium | palette | Keep (D1). | D |
| F12 | `ctrl+o` | toggle sort | People | low | palette "Toggle sort" | Merge into one "view options" chord (D2). | D |
| F13 | `ctrl+l` | cycle duplicate level | Merge | low | palette "Duplicate level" | Merge into `ctrl+o` (D2). | D |
| F14 | `ctrl+f` | filter archived by type | Archived | low | palette "Filter archived by type" | Merge into `ctrl+o` (D2). | D |
| F15 | `ctrl+e` | change survivor | Merge compare | medium | palette (compare actions are enabled in compare) | Keep. A real per-merge decision. | C |
| F16 | `ctrl+b` | next pair | Merge compare, groups of 3 or more | low | palette "Change pair" | Move to palette. | C |
| F17 | `ctrl+w` | switch pane | Merge compare, narrow terminals only | low | palette "Switch pane" | Move to palette. | C |
| F18 | `q` | close compare | Merge compare | low | `esc` | Remove in compare. `esc` already does it and the allow-list shadow line goes away. `q` stays as quit. | C |
| F19 | `tab` `shift+tab` | toggle value | Merge compare | medium | `space` does the same | Remove from compare. Elsewhere tab moves focus, here it changes data. `space` is the select verb. | D |
| F20 | `q` | close | text viewer (Doctor, Where, DAV) | low | `esc` | Remove. | C |
| F21 | `J` `K` | move module | Modules screen | low | `shift+up/down` do the same | Remove. | C |
| F22 | `r` | reset modules | Modules screen | low | none | Move to palette "Modules: reset". Bare `r` means restore everywhere else. | C |
| F23 | `m` | apply merge without summary | Merge compare | medium | `enter` then `enter` | Keep. It is the merge verb and undo exists. | C |
| F24 | `d` | dismiss group | Merge compare, Merge list | medium | none | Keep. It is the delete verb. | C |
| F25 | `y` `Y` `n` `N` | answer yes or no | confirm prompt | high | none | Keep. Text answer to a question, not a binding. | C |
| F26 | digits | pick by number hint | picker overlays | medium | typing filters | Keep. Inside a picker only. | C |
| F27 | `alt+d` | dismiss a chip | bar command line | low | none | Keep. Text mode, no conflict. | C |
| F28 | `ctrl+k` | open palette | all | high | `/` | Keep. | C |

Notes. `/` and `ctrl+k` stay as the two palette keys. `shift+tab` in the list and detail pane duplicates `tab` with only two panes, but it is part of the decided navigation set and is left alone. There is no bare `j k h l g G J K` anywhere any more (checked in the handlers). Bar results route unknown keys through the registry, so every removal also applies there.

## Decisions

- **D1. Module switching.** Three mechanisms exist (digits, `[ ]`, `v`). Digits break the rule (a bare key moves the view) and cost 10 keys. Terminals cannot send `ctrl+1`. Option A (recommended): remove digits, keep `[ ]` and `v`. `v` plus a digit hint is two strokes to any view. Option B: keep digits, remove `[ ]` and `v`. Matches yazi and taskwarrior-tui, never needs a picker. Option C: A plus `ctrl+left` and `ctrl+right` instead of `[ ]`, which is the only fully rule-compliant form. Earlier decisions D6 and X28 chose to keep digits, so this reverses them.
- **D2. View options chord.** One chord, `ctrl+o`, does the view's own setting: sort in People, duplicate level in Merge, type filter in Archived. Three keys become one. Needs a small code change. Without it, move all three to the palette (also valid, they are rare).
- **D3.** F2, F4, F5, F6, F8, F19 are low-risk key removals the owner should confirm because they were documented in earlier patterns.

## Recommended

Apply F1, F3, F7, F15 to F18, F20 to F22 now. Ask the owner for D1 to D3, then apply F2, F4, F5, F6, F8, F9 to F14, F19.

### Proposed keymap (recommended options: D1 option A, D2 merged, D3 all accepted)

| Group | Keys |
|---|---|
| Object verbs | `a d e/enter r l u x f m s space` |
| History and refresh | `ctrl+z ctrl+y ctrl+r` |
| Move the pointer | `up down left right tab shift+tab home end pgup pgdn` |
| Move the view | `ctrl+up ctrl+down ctrl+shift+up ctrl+shift+down shift+up shift+down` (Modules screen), `[ ]`, `v`, `ctrl+o` (view options) |
| App | `/` `ctrl+k` palette, `?` help, `esc`, `q`, `ctrl+c` |
| Merge compare | `left right space enter m d esc ctrl+e` (ctrl+e is the only extra) |
| Mode keys, not counted | `y n` confirm, `tab` in prompts, `alt+d` in the command line, digits in pickers |
| Palette only | copy, open link, edit link or type, set cadence, tier, mark all, change pair, switch pane, modules reset, tag, sync now, dry run, attach, go to today, scroll detail |

### Count

| | Distinct normal-mode keys |
|---|---|
| Before (generated table, 2026-10-27) | 64 |
| Removed: `ctrl+n ctrl+p ctrl+d ctrl+u : y o E F *` (10), `0-9` (10), `ctrl+l ctrl+f` (2), `ctrl+b ctrl+w` (2) | -24 |
| After, D1 option A | 40 |
| After, D1 option B (digits kept, `[ ] v` removed) | 47 |
| After, only the clear items (F1, F3, F7, F16, F17: six keys) | 58 |

The function behind every removed key stays reachable through the palette or a kept key.
