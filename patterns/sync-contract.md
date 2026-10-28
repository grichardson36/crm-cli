---
type: pattern
status: decided by owner 2026-10-27, not built
---
# Sync contract (v1)

One contract for syncing any program built on the framework. Database-agnostic in shape, SQLite is the only adapter built in v1. Evidence: [[research-sync-standardization]], [[research-sync-engines]], [[research-mesh-architectures]]. Extends [[sync-safe-data]] and [[local-first]].

## Rules

1. **Unit of change is a field change.** Actions are never replicated.
2. **Record.** `{id: (device, seq), hlc, collection, key, field, op: set|unset|delete|restore, value, schema_version, tx?}`. `key` is a UUID. The `id` makes replay safe to repeat.
3. **Clock.** One fixed-width, sortable hybrid logical clock string with a stated maximum drift.
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

## Not in v1

Radio or low-bandwidth links, compact binary encoding, encryption, signed authorship, multiple users, untrusted relays, Reticulum or any mesh transport.

Related: [[field-sources-are-ranked]], [[conflicts-keep-server-store-local]], [[soft-delete-restorable]].
