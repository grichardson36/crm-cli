---
type: pattern
scale: object
status: decided
---

# Delete is soft and restorable everywhere

**Problem.** Fear of losing data slows the user down.

**Therefore.** Delete archives or soft deletes in every door. Restore exists in every door. Purge is the only hard delete, writes a tombstone and asks for a typed confirmation.

**Because.** Restore needs no confirm. The Archived view lists every object kind with a type filter and `u` restores. Links of a deleted object are hidden and come back ([[hidden-not-removed-links]]).

**Larger patterns.** [[one-verb-set]], [[sync-safe-data]]

**Smaller patterns.** [[undo-redo-two-stacks]], [[hidden-not-removed-links]]

**Sources.** Office undo, Gmail undo toast, [[feature-rules]] rule 4

**Inspired by.** [[dex]], [[us]]
