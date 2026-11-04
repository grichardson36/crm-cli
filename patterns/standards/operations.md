---
type: standard
domains:
  - operations
  - runtime
---
# Operations standard

## Scope

Build, deployment, backup, recovery, local service operation and release verification.

## Current shape

The primary deployment is one local binary with one local SQLite database. Migrations run through the application. `crm backup` makes safe backups. CardDAV and the replica relay are optional services.

## Rules

- Do not manipulate a live database file directly. Use CRM commands and safe backup operations.
- Deployment runs through `deploy/deploy.ps1`, which verifies, builds, checks migrations on copies and keeps the prior binary for rollback.
- Operational state and credentials stay local unless an optional integration is explicitly configured.
- A running service states its network boundary and transport assumptions.

## Checks

- A deployable change has a tested upgrade, rollback and database-copy migration path.
- An operational command reports actionable failure and does not silently destroy user data.
- A service exposed beyond a trusted local network has an explicit security decision.

## Evidence

Feature work is in [[features/operations]].
