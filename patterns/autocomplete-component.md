---
type: pattern
scale: interaction
status: decided by owner 2026-10-29, built in feat/pipe-autocomplete (not merged)
---
# One autocomplete component for every small input

**Problem.** The command bar completed verbs and names, the relation typeahead completed objects under a field, and the pipe-line prompts completed nothing. Each had its own matching, so a nickname or a typo worked in one and not in the others.

**Therefore.** One matching implementation and one popup component. Any small input in the program gets an incremental search popup by implementing a two-method interface. The popup is small (at most six rows, up to twelve and scrolling for grouped rows), sits under the segment being typed and filters while the user types. It is fed by completion sources taken from the field type ([[field-types-drive-editors]]):

| Field type | Rows |
|---|---|
| relation | objects of the allowed kinds, ranked with nickname aliases and typo tolerance. A segment of several kinds ([[related-segment]]) suggests from the context and groups its rows by kind under dim headings |
| choice | its options |
| date | natural phrases ("fri", "next week") and the dates open tasks already use |
| tag | existing tags |
| text | none |

**Keys.** `tab` accepts the highlighted row, `up` and `down` move, `esc` closes the popup (a second `esc` cancels the prompt), `enter` still submits the prompt and is never taken by the popup. With no popup open every key keeps its old job (`tab` still changes the note type). In the command bar the popup is the existing results list, the bar does not change.

**After a name.** Accepting an object in a segment that holds several (the people of a deal, the objects a tag is put on) adds `, ` so another can be chosen. Typing `|` takes the comma back and starts the next segment. Backspacing over a name takes it out before it is saved, which is the unlink (links are pipe segments, see [[one-line-pipe-prompt]]).

**Grouped rows.** A row may carry `Kind` (color and short prefix) and `Group` (a heading drawn above the row). The component builds the headings with the grouped list ([[grouped-lists-with-dividers]]) and scrolls inside ten lines.

**The interface** (`internal/tui/autocomplete.go`):

```go
type Completer interface {
    Suggest(text string, cursor int) []Suggestion
    Accept(text string, cursor int, s Suggestion) (string, int)
}
ac := NewAutocomplete(source)   // keys: ac.Key("tab"), rows: ac.Rows(), draw: ac.Lines(width, styles)
```

The pipe line is one implementation (`pipecomplete.go`, `pipeCompleter`). A new small input implements `Completer` and calls `Refresh` when its text changes. Matching is never written in the source: it calls `internal/suggest` (`Rank`, `Score`, `Typo`, `Prefix`, `DatePhrases`), the same ranker the relation typeahead and the bar completions call.

**Beside it.** The popup does not replace the grayscale syntax hint ([[show-syntax-inline-as-you-type]]), both draw at once. The live check of a closed segment and the soft indicator of the open one come from [[one-validation-framework]].

**Because.** Owner decision 2026-10-29. One matcher means a fix to nickname or typo handling reaches every input ([[one-abstraction-many-contexts]]). A popup anchored to the segment keeps the eyes where the typing is, which a full screen like the bar does not.

**Larger patterns.** [[one-line-pipe-prompt]], [[pick-or-create]], [[related-segment]]

**Smaller patterns.** [[show-syntax-inline-as-you-type]], [[one-validation-framework]], [[field-types-drive-editors]]

**Sources.** `internal/tui/autocomplete.go`, `internal/tui/pipecomplete.go`, `internal/suggest`

**Inspired by.** [[obsidian]], [[notion]], [[raycast]]
