---
type: pattern
scale: interaction
status: decided
---

# Small objects add and edit on one pipe-separated line

**Problem.** A full form is heavy for orgs and deals.

**Therefore.** One line, fields split by a pipe. Edit opens the same line filled in. Org: name | domain | industry | notes. Deal: title | value | stage | organization | person | notes. Related objects take an id or a name. A unique match is accepted, several matches ask for the id.

**Because.** Matches the task and booking prompts. X4 is closed by [[field-types-drive-editors]].

**Live validation (2026-10-29).** Every pipe-line prompt checks a segment through [[one-validation-framework]] the moment `|` closes it. The error shows inline in dim error style with its code, what was expected and the fix. The segment still being typed shows a soft indicator only (a date preview, the choices that match), never an error. `enter` keeps the final full validation. A popup helps while typing: [[autocomplete-component]].

**Links are one segment (2026-10-29).** A line ends with ONE `related` segment that takes objects of every kind the relation registry joins to the object, and tags, separated by commas ([[related-segment]]). Its suggestions come from the context: the highlighted object or record page, its links, the links of what is chosen, recent objects. A deal keeps its own organization and person fields before it. `tab` accepts a suggestion. Nothing is linked while the text is typed, so backspacing over a name is the unlink. On `enter` every resolved name is linked and an unresolved one offers [[pick-or-create]]. This replaces the grayscale tab-to-accept offer and the dropped link chip.

**Larger patterns.** [[same-input-style]]

**Smaller patterns.** [[related-segment]], [[autocomplete-component]], [[show-syntax-inline-as-you-type]]

**Sources.** Todoist and Fantastical quick-add lines. DESIGN-PATTERNS.md G

**Note (2026-10-16).** Archived with calendar: the booking prompts were removed from main. The task prompt remains the reference.
