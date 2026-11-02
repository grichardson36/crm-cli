---
type: pattern
status: decided
core-gate: 6
domains: [interface]
---
# Add standard

**Rule.** Every object kind is added the same way.

**Applies when.** Adding or changing how a person, organization, deal, task, note or tag is created in a door under [[core]] gate 6.

**Default.**

1. `a` opens the kind picker, the view's own kind first. `enter` on the view's own kind opens one footer line.
2. The line is the name or title first, then the optional fields of the object registry ([[object-registry]]) in its order, separated by `|`, then the related segment that links other objects ([[input-grammar]]). The hints, validation and autocomplete come from the same pipe line for every kind.
3. `enter` saves the line as one undo step, with its links. `esc` abandons it and adds nothing.
4. The cursor lands on the new object in its list, and the list view stays the one that was open. A kind added from a view of another kind uses an empty view of that kind and then returns.
5. The form is the second way. A person can also be added in the detail form (palette: "Person: add with the form"), which is the same record edited row by row.

**Exception.** A note needs a person or object to attach to, so it asks for one first when nothing is highlighted.

**Check.** `TestEveryKindAddsFromAFooterLineAndLandsOnTheObject` in `internal/tui/addstandard_test.go`. A new kind is added to its table in the same change.

See [[navigation-standard]].
