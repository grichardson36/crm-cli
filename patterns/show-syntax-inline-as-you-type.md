---
type: pattern
status: decided by owner 2026-10-25
---
# Show the syntax inline as you type

When an input takes a special syntax (pipe line `title | due`, `@person`, command bar verbs), the field shows the expected shape in grayscale as the user types. The user never has to remember the syntax of each object type. A form or popup is the fallback for objects with many fields.

Why: first-order retrieval. The most-used tools sit within reach and explain themselves where they are used, so nothing has to be recalled from memory.

Applies to: the pipe line prompt ([[one-line-pipe-prompt]]), the command bar, and any new object type added to the registry. Settles the input-style split in [[same-input-style]]: one behavior (inline hint), with the form as the wide-object variant.

The hint keeps working beside the autocomplete popup ([[autocomplete-component]]), which fills the segment being typed while the hint shows the segments still to come.

Related: [[objects-first]], [[three-doors-one-core]].
