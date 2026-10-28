---
type: pattern
scale: interaction
status: decided
---

# Undo refuses when the target changed

**Problem.** Undo overwrites newer edits.

**Therefore.** If the record was edited or deleted since, refuse with Cannot undo: Sam Lee was edited since, and point to undo history.

**Larger patterns.** [[undo-redo-two-stacks]]

**Sources.** DESIGN-PATTERNS.md D, orgs and deals stale-state guards
