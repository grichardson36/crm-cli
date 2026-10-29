---
type: pattern
status: decided by owner 2026-10-27, built 2026-10-29 on branch feat/device-sync (SQLite adapter), see Built below
---
# Sync contract (v1)

One contract for syncing any program built on the framework. Database-agnostic in shape, SQLite is the only adapter built in v1. Evidence: [[research-sync-standardization]], [[research-sync-engines]], [[research-mesh-architectures]]. Extends [[sync-safe-data]] and [[local-first]].

## Rules

1. **Unit of change is a field change.** Actions are never replicated.
2. **Record.** `{id: (device, seq), hlc, collection, key, field, op: set|unset|delete|restore, value, schema_version, tx?}`. `key` is a UUID. The `id` makes replay safe to repeat.
3. **Clock.** One fixed-width, sortable hybrid logical clock string with a stated maximum drift of 5 minutes, the same as Actual Budget. A change further ahead of the receiver's wall clock is rejected.
4. **Two calls.** `pull(cursor, limit)` and `push(changes)`. The cursor is a version vector: for each device, the highest `seq` seen. Any peer can compute it, so any device can be a relay.
5. **Relay is a role.** It stores changes for enrolled devices and serves pulls. Any device can hold the role. An always-on device such as pibrain usually does.
6. **Network and trust.** IP only, JSON over HTTP. Peers are the owner's own devices. There is no encryption layer in the contract. Endpoints are reachable only over Tailscale or the LAN, and each enrolled device presents a token. Exposing a relay to the public internet is outside the contract.
7. **Conflict policy per field**, declared in the schema: `lww` (default), `keep-both` (notes), `max`, `set-union`.
8. **Delete versus edit.** When one device deletes a record and another edits it, the record parks in a review queue. Nothing is lost silently. Undo restores.
9. **Tombstones.** `delete` is a record. A tombstone is purged only after every enrolled device has passed it. A manual retire-device action releases the block from a lost device.
10. **Unknown fields.** Store them without applying them, and retry after a schema upgrade. Never discard.
11. **Snapshot.** Per-field state plus its cursor, for new devices and resets.
12. **Replication is complete in v1.** A view's query does not filter what syncs. Revisit when data size demands it.
13. **Adapter interface.** `apply(change)`, `read_field_clock(collection, key, field)`, `emit_local(change)` on every local write, a `begin_remote` / `end_remote` flag, and `snapshot()`.
14. **Conformance test.** Fixed change sets, shuffled and duplicated, applied through the adapter must give identical state. This is the only thing that makes the contract real across two databases.
15. **Proof.** An internal convention until the SQLite version is stable. A second adapter (a document store) comes after that.

## Built

`internal/replica` (core and in-memory reference adapter), `internal/replica/replicasql` (SQLite adapter), `internal/replica/relay` (HTTP), `internal/replica/replicatest` (conformance), CLI `crm peers`, migration 016. How it works is in [[replica-package]], the commands in [[peers-cli]].

Choices the contract left open:

- **Capture** is by SQL triggers in the same transaction as the row edit, not by the driver hooks. Reasons are in [[replica-package]].
- **Record** has one addition, `seen`: the writer's version vector, set only on deletes and keep-both fields. It is how a concurrent edit is told from a later one. Rule 8 needs it, and keep-both needs it to know which edits were already merged.
- **State** is one table, `replica_changes`, with the latest change of each device for each field. It is the per-field state, the pull log and the parking place for unknown fields. Snapshot (rule 11) is the same data plus the cursor: `Adapter.Snapshot()`, and over HTTP a pull with an empty cursor (`crm peers pull --full`).
- **Delete versus edit** (rule 8): the record stays visible with the edits and waits in a review queue. `--keep` writes a restore, `--delete` writes a delete that has seen the edits.
- **Tombstones** (rule 9): purged by `crm peers purge` on the relay once every enrolled, not retired device has reported a cursor past the delete. With no other device enrolled nothing is purged.
- **Trust** (rule 6): a bearer token per device, a one-time enroll token for a new device, `serve` refuses a public or wildcard bind without `--allow-public`.
- **Rows made on two devices with one natural key** (built-in vocabulary, relation kinds, note types, tag names that exist when sync is turned on) get a uuid derived from the key, so they are one record. Tag names and other unique names created independently after sync is on can collide. The later record is kept as a blocked record, never dropped.

Covered: 25 tables (people, organizations, interactions, deals, tasks, tags, taggings, person_emails, person_phones, person_handles, custom_fields, relation_kind, membership, person_relation, org_relation, deal_person, deal_org, activity_target, merge_alias, note_types, vocabulary, name_aliases, company_ignore, dupe_dismissals, org_dupe_dismissals). Not covered: external_ids, sync_runs, sync_cursors, sync_inbox, sync_conflicts, dav_state, field_versions, tombstones, merge_log, meta. Not built: a terminal UI screen, a second adapter (rule 15).

## Not in v1

Radio or low-bandwidth links, compact binary encoding, encryption, signed authorship, multiple users, untrusted relays, Reticulum or any mesh transport.

Related: [[field-sources-are-ranked]], [[conflicts-keep-server-store-local]], [[soft-delete-restorable]].
