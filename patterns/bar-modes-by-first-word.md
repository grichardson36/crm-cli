---
type: pattern
scale: interaction
status: decided
---

# The first word picks verb mode or search mode

**Problem.** A capture command competes with search results.

**Therefore.** If the first word is an exact verb (note, log, task, set, tag, snooze, cal) followed by a space and at least one more character, the bar is in verb mode: show the parsed preview, tab completes names, tags and dates, enter runs. Anything else is search mode with grouped sections in fixed order (owner decision 2026-10-16): objects first (People, Orgs, Deals, Tasks, Notes, Tags, an exact tag name leading them), then Actions, then Settings, at most four rows per object group, down past the end expands. An empty box lists Recent first. A verb being typed is the exception: while the first word is the start of a verb (n, t, set, tag, snooze) its action line (Add note (n), Snooze (snooze)) leads the list, ahead of the objects. A lone n is search. Tab on an object row moves the focus into the results ([[bar-results-act-like-views]]); Tab on a typed verb still completes it.

**Larger patterns.** [[one-bar]]

**Smaller patterns.** [[show-parsed-chips]]

**Sources.** Raycast command then arguments (DESIGN-PATTERNS.md C)

**Inspired by.** [[raycast]]
