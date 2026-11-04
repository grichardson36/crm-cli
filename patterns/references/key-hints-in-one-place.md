---
type: reference
status: decided by owner 2026-10-30
---

# Key hints live in one place

**Problem.** Hints about which keys work were scattered: dim lines inside detail panes ("r restores, ctrl+o filters by type"), popup titles ("enter run esc close"), descriptions that restated keys, empty-state texts, status lines. They disagreed with each other and drifted when a key was rebound.

**Therefore.** Every screen and mode shows its key hints on its last rows, the footer, and nowhere else. Hints that do not fit the width wrap to a second row (two hint rows at most, a hint is never split, a terminal under 8 rows keeps one row and cuts the rest). One component, `internal/tui/footer.go`, lays them out, and every screen subtracts its hint rows from its body. One order: verbs, then navigation, then what is specific to the screen, then the keys that control the view and the app (find, help, views). One format: the key in the name style, the label dim, two spaces between hints.

- The main screen builds its footer from the action registry (`footerSegs`). The order comes from the same layer table as the grouped key list ([[features-tui-keys-and-modules]]): framework verbs, navigation, app-specific, view control, then the `foot` number inside a class.
- A mode that takes over the screen (field edit, prompt, question, key capture, picker, palette, command bar, names panel, view editor, text page, keys page) builds its footer from `internal/tui/hints.go`. A hint names a registry action, so the key shown is the key bound today, or a fixed key that no action owns (enter, esc, the arrows, y, n).
- Not allowed outside the footer: rows or panes that say "enter opens it", titles with keys, descriptions or empty states that restate a key, confirm prompts ending in "y/n".
- Allowed: the dim syntax hint after the cursor of an input ([[input-grammar]]), the keys page and the rebind list (they list keys by design), the key column of the palette ([[keyboard-and-feedback]]).
- A status line that reports a result may still say that ctrl+z undoes it. It is feedback, not a screen hint.

**Label words (CRM-023, owner decision 2026-11-03).** The same action reads the same word on every screen, in the footer, the popups and the modes. The words are the constants of `internal/tui/hintwords.go`. The action registry (`footShort`, `footLabel`, `footDyn`) and every `fk(...)` and `ak(...)` hint use them, so one rename follows everywhere. A key keeps its word unless its meaning changes:

| Key | Word | Where |
|---|---|---|
| `d` | delete | everywhere (person with marks, Merge, Sync inbox, tags, the saved view editor, a record row). Only the Archived view reads purge |
| `a` / `l` / `u` / `r` | add / link / unlink / restore | all screens |
| `f` / `s` / `m` | favorite / snooze / merge | people, people, objects that can merge |
| `enter` in a list | open | every list (People, Organizations, Deals, Tasks, Tags, Notes, Home, Follow-up). Compare in Merge, accept in the Sync inbox, rename on a tag in the bar, because enter does that instead of opening |
| `enter` in the detail pane | edit | the field under the cursor |
| `enter` in a prompt or editor | save | writes a value |
| `enter` in a picker | pick | chooses from a list |
| `enter` in the palette and command bar | run | executes a command |
| `enter` on a prepared command | apply | after the command bar shows its summary |
| `tab` or `right` on a list | details | moves into the detail pane |
| `esc` | cancel | editors, prompts, pickers, the palette, confirms |
| `esc` | back | leaves a screen, a pane, the detail pane, a tag page, the compare screen, a text page |
| `esc` | close | a read-only popup, help, history, the Views popup, a suggestion popup |
| `esc` | unmark | a list with marked rows. The last layer esc peels: after a search, the tag page, the detail pane and every popup or mode |
| `space` | select | marks rows. Toggle only flips a value in place (the Compare screen) |

`esc` never reads save, close list or confirm. Prompts and editors never read confirm or ok. Words that are not verbs (move, find, help, views, options, cell, outside and the screen specifics) are in the same file.

**Key text (CRM-025, owner decision 2026-11-04).** Rule: all key text comes from the key text table. `internal/tui/hintkeys.go` holds the table (`keyTexts`) and the one function every renderer calls, `showKey(name)`. It turns a key name or a key group into the text a person reads. Matching code (`case "up":`, `keys: []string{"left"}`) keeps the plain names. Only drawing changes. A modifier keeps its name and the key after it follows the table, a key outside the table reads as typed, so a rebound key shows what it is rebound to.

| Key name | Text | Note |
|---|---|---|
| `up` `down` `left` `right` | `↑` `↓` `←` `→` | the main footer already showed arrows |
| `up/down` `left/right` | `↑/↓` `←/→` | groups, constants `hkUpDown` and `hkLeftRight` |
| `shift+up` `ctrl+left` and the like | `shift+↑` `ctrl+←` | `hkShiftUpDown` reads `shift+↑/↓` |
| `enter` `esc` `tab` `shift+tab` | same | chrome keys keep their names |
| `" "` | `space` | |
| `home` `end` `home/end` `pgup` `pgdown` | same | |
| `ctrl+x` forms and letters | same | |

Hints name arrows with the `hk*` constants (`fk(hkUpDown, hwSelect)`), registry `hint:` fields too (`hint: hkLeft`). The registry `hint` stays a key name, the renderer converts it: footer (`footerSegs`), modes and popups (`keyText` in `hints.go`), the picker key column and the palette (`overlay.go`), the keys page and the Settings Keys pane (`joinKeys`), the command bar verbs, the Views popup, rebind messages, and the key names inside action descriptions. The CLI (`crm keys`, `SwallowedChordWarning`) still prints the config file names. The glyphs are one cell wide, so footer wrapping and mouse hit boxes use the shown text.

**Contextual hints.** `unlink` (a linked row of the detail pane), `favorite` and `snooze` (a person row) show in the footer only while the highlighted row supports them (`footCtx` in the registry, with the same `when` and `footIf` as the key). They come after the existing verbs of their class. The footer is two rows, so when the keys of the view and the app (find, help, views, options) would be cut, `footerSegs` drops contextual hints first, the last one first. `unmark` (`esc`, while rows are marked, CRM-026) is the exception: it ranks higher (`footKeep`), so it is the last contextual hint dropped and the chrome keys still survive at 40 to 130 columns. `restore` already shows only on the Archived view with rows, where `r` works.

**Check.** `TestKeyTextTable`, `TestNoFooterNamesAnArrowWithAWord`, `TestMainFooterAndModesShowArrowGlyphs`, `TestListEditorSettingsPaneAndHistoryShowArrowGlyphs` and `TestFooterWrapAndHitBoxesCountGlyphWidth` (`internal/tui/hintkeys_test.go`) cover the table, the screens and the hit boxes. `TestHintLabelsAreNeverLiterals` fails when a label is typed into the code instead of using the word table. `TestHintLabelsStayInTheWordTable` checks the footer and the mode tables on many screens against the table and the word each verb key may carry. The scenario tests in `internal/tui/hintwords_test.go` (`TestDReadsDeleteEverywhereExceptArchived`, `TestEnterReadsOpenInEveryObjectList`, `TestEscReadsCancelBackOrClose`, `TestContextualHintsNeverPushTheChromeOffTheFooter` and the rest) cover each key. `TestKeyHintsAreOnlyInTheFooter` renders every screen and mode and fails when a line above the footer reads like a hint (a key name followed by a verb). `unmark_test.go` (`TestEsc*`, `TestCtrlZ*`, `TestFooterShowsEscUnmarkOnlyWhileMarksExist`, `TestEscUnmarkOutlastsTheOtherContextualHints`) covers CRM-026. `TestFooterHintsFollowTheKeymap` rebinds a key and expects the footer to follow. `TestFooterOrderIsVerbsThenNavigationThenSpecifics` checks the order.

**Because.** Owner request 2026-10-30.

**Related.** [[keyboard-and-feedback]], [[keyboard-and-feedback]], [[current-tui-keys]]

**Sources.** `internal/tui/hintwords.go`, `internal/tui/hintkeys.go`, `internal/tui/hints.go`, `internal/tui/hints_test.go`, `internal/tui/hintwords_test.go`, `internal/tui/view.go`
