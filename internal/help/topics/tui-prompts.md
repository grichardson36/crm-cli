# Terminal UI: pipe-line prompts

Every pipe-line prompt (deal, task, note, organization, tag and the name of a new person) checks a segment through the one validation framework as soon as `|` closes it. An error shows inline in dim error style with its code, what was expected and the fix (`! E_DATE_INVALID due: ...`). The segment being typed shows only a soft indicator (`next friday -> Fri Oct 23`, `matches prospect, proposal`), never an error. Smart corrections (an email typed in a name) show as hints. `enter` still validates the whole line.

A popup of at most six rows opens under the segment being typed, fed by the field type: relation segments list objects (nicknames and typos match), choice segments their options, date segments natural phrases and dates tasks use, tag segments existing tags, text segments nothing. While the popup is open `tab` accepts the highlighted row (after an object a `, ` is added for the next one, typing `|` takes it back), `up`/`down` move, `esc` closes the popup (a second `esc` cancels the prompt), `enter` submits the line. With no popup `tab` keeps its old job (the note type). Backspacing over a name takes that link out before it is saved. See crm help short-syntax for the grayscale syntax hint that still shows after the cursor.

See also: tui-keys, tui-key-changes, short-syntax, validation, links.
