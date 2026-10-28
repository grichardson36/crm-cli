---
type: pattern
scale: object
status: decided
---

# Deleting an object hides its links, it does not remove them

**Problem.** Deleting an organization that has twelve members must not silently cut twelve links, and restoring it must not leave the user to rebuild them.

**Therefore.** A delete is the soft delete of the object only. Its links stay stored. Every link list joins on the live object, so the links of a deleted organization, deal or person disappear from the other side's pages and counts, and reappear when the object is restored. Unlinking is its own soft delete (deleted_at on the link row) that undo reverses exactly. Purge is the only hard delete: it removes the links with the object and writes tombstones. Merging and exporting also read live objects only.

**Because.** [[soft-delete-restorable]] says restore needs no confirm. Q26 of the owner decisions applies it to links, and the Archived view lists every object kind with a type filter and `u` restores, so nothing deleted is out of reach.

**Larger patterns.** [[soft-delete-restorable]], [[sync-safe-data]]

**Smaller patterns.** [[undo-redo-two-stacks]]

**Sources.** [[schema-er]] (rules every table follows, Q26), FEATURES.md rule 4

**Inspired by.** [[dex]], [[us]]
