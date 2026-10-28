---
type: pattern
scale: object
status: decided (owner 2026-10-28), built in feat/fields-editors
---

# Field types drive editors

**Problem.** Each record page and form decided for itself how a field is edited and checked. The person form took free text for everything, deal stage and priority hid in the palette, the organization and deal pages were read-only, and the same check (a date, a number, a stage) was written again at every door.

**Therefore.** The schema declares one type per field from a closed list. The type decides the editor, the check and the hint, and the vocabulary lives in one place in the registry (`internal/model/fields.go`, `model.Fields`) so a future program reuses it.

| Type | Editor | Check |
|---|---|---|
| text | one-line input in the row | trimmed, required fields refuse empty |
| longtext | the same input, markdown, a new line is `alt+enter` | trimmed |
| date | input with a live preview of what the text means, "next friday" works through the existing date parser | ISO date or phrase, a birthday also takes month and day |
| choice | the options sit in the row, up and down or a letter pick | one of the options, by value or label |
| number | input with a preview | a number, `$` and thousands separators ignored, optional minimum |
| list | one row per entry (phones, emails), `a` adds an entry, `d` removes it | each entry checked on its own |
| relation | typeahead under the row ([[pick-or-create]]) | the object exists or is created |

**The editing gesture is the same for every type.** `enter` or `e` on a field edits it in place ([[universal-verbs]]). `enter` or `tab` commits, `esc` cancels. A failed check keeps the editor open and shows a field error: a stable code, the field, what was expected, what was received and a fix ([[validate-and-correct-at-every-boundary]]). `d` clears an optional field after a yes or no.

**Relation fields.** Typing searches existing objects. Names match through the shared aliases (Zach finds Zachary) and forgive a typo, which plain search does not. `enter` picks the highlighted match, the last row reads Create 'X' and creates and links it, `tab` completes the highlighted name into the field. A linked object is a chip, `u` unlinks the focused one.

**Hints.** The grayscale hint component draws what the field takes after the cursor ([[show-syntax-inline-as-you-type]]), never inside the input.

**Because.** Owner decision 2026-10-28. One declaration per field means the record pages, the add flows and later the CLI and MCP checks agree, and a new object type gets editors by listing its fields. Deal stage and priority are choice fields edited this way, so their palette-only entries are gone. This closes the question of the person form with no hint: every record page edits field by field.

**Larger patterns.** [[objects-first]], [[one-record-page]], [[validate-and-correct-at-every-boundary]]

**Smaller patterns.** [[pick-or-create]], [[show-syntax-inline-as-you-type]], [[dates-are-plain-words]], [[universal-verbs]]

**Sources.** Owner decision 2026-10-28. `internal/model/fields.go`, `internal/tui/fieldedit.go`, `internal/tui/typeahead.go`

**Inspired by.** [[notion]], [[attio]], [[hubspot]]
