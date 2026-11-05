---
type: standard
domains:
  - data
  - application
---
# Data standard

## Scope

Persistence, identity, validation, migrations, archival, restoration and data lifecycle.

## Current shape

The application uses one local SQLite database per CRM domain. Repositories own persistence operations; the application owns validation and domain behavior.

## Rules

- Every fact has one canonical stored representation. Read models and convenience IDs are derived or boundary translations.
- Every write validates and normalizes through the shared behavior used by its applicable doors.
- Record removal is reversible unless a deliberately destructive operation is explicit, confirmed and documented.
- A migration is additive, ordered, testable on a database copy and never rewrites a released migration.
- Stable identities and archive state survive import, sync and restore.

## Checks

- A write path identifies validation, normalization, migration and rollback behavior.
- A schema change has an upgrade path and does not duplicate a canonical fact.
- A destructive operation has preview, confirmation and recovery behavior appropriate to its risk.

## Evidence

[[open-discrepancies]] records data-model violations. Current work is in the [[core#Roadmap|roadmap]].
