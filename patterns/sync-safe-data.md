---
type: pattern
scale: object
status: decided
---

# Data is sync-safe from day one

**Problem.** Retrofit of sync breaks identity and deletes.

**Therefore.** Every row has a UUID. updated_at is bumped on every change. Deletes are soft. Purge writes a tombstone. Field-level versions support three-way merge.

**Larger patterns.** [[local-first]]

**Smaller patterns.** [[soft-delete-restorable]], [[conflicts-keep-server-store-local]], [[sync-maps-structure-to-flat-fields]]

**Sources.** FEATURES.md rule 4

**Inspired by.** [[folk]]
