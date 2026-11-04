---
type: standard
domains:
  - integration
---
# Integration standard

## Scope

Import, export, sync, enrichment, CardDAV, replica transport and external protocols.

## Rules

- External formats translate at the boundary. They do not define CRM objects, fields or link semantics.
- An integration declares its source identity, mapping, cursor or change detection, conflict behavior and failure reporting.
- Imports and enrichment propose or map data into canonical CRM facts. They do not silently create relationships or overwrite user-owned facts.
- Sync and import work is idempotent, observable, local-first and safe to rerun.
- Credentials and network access remain optional. Core CRM use does not require either.

## Checks

- A new adapter states its mapping, user approval point, duplicate handling and recovery behavior.
- An external value either maps to a canonical fact or stays integration metadata.
- A sync change preserves archive, restore and identity behavior.

## Evidence

[[research-sync-standard]] and [[research-relationship-models]] are evidence. Feature work is in [[features/integration]].
