---
type: guide
---
# The replica package (device sync in code)

How [[sync-contract]] is built. Read this before changing sync code or adding a synced table. The commands are in [[peers-cli]].

## Layout

| Package | What it holds |
|---|---|
| `internal/replica` | The contract, pure Go. Hybrid logical clock (`hlc.go`), change record and version vector (`change.go`), conflict policies (`policy.go`), schema declaration (`schema.go`), the apply engine (`engine.go`), the `Adapter` interface (`adapter.go`) and an in-memory adapter (`mem.go`) that is the reference implementation. |
| `internal/replica/replicasql` | The SQLite adapter. Registry (`registry.go`), capture triggers and baseline (`triggers.go`), enable and refresh (`enable.go`), the store and projector (`store.go`), the adapter (`adapter.go`), devices and tokens (`devices.go`). |
| `internal/replica/relay` | HTTP transport. Relay server, client and the pull-then-push exchange. Plain `net/http` and JSON. |
| `internal/replica/replicatest` | The conformance test every adapter runs. |

The name `internal/sync` is the source sync (sources and inbox), and `crm sync` is its command. Device sync is `replica` and `crm peers`.

## The record

`{id: {device, seq}, hlc, collection, key, field, op, value, schema_version, tx?, seen?}`.

- `collection` is the table name. `key` is the row uuid. `field` is the column name, empty for a delete or restore.
- `op` is `set`, `unset`, `delete` or `restore`. `value` is typed JSON. A reference to another row is the uuid of that row, never an integer id.
- `hlc` is one fixed-width string: `2026-10-28T12:00:00.000Z-00AF-0123456789abcdef` (UTC millis, four hex digits of counter, sixteen hex digits of device). It sorts as a string. The largest accepted gap to the wall clock is 5 minutes (`MaxDrift`), the same as Actual Budget. A change further ahead is rejected, and so are all later changes of that device in the batch, so a cursor never moves past a change that was not stored.
- `seen` is the one addition to the contract record. It is the writer's version vector, written only for deletes and for keep-both fields. It tells a reader what the writer had already seen, which is what separates a concurrent edit from a later one without comparing clocks.

## What is stored

One table, `replica_changes`, keyed by `(collection, key, field, device)`. It holds the latest change of each device for each field. That is the per-field state, the log peers pull from, and the place unknown fields wait, in one table. Older changes of the same device and field are replaced, so the log compacts itself. A change that is superseded is still safe: every device that matters has the newer one.

`replica_vector` is the version vector: the highest `seq` seen from each device. The row of this device is also the counter for its own changes. It is the pull cursor, so any device can answer a pull and act as a relay.

The row in the application table is a projection of those changes. The engine resolves each field from the per-device entries by its policy, then the projector writes the row.

## Capture of local writes

SQL triggers, one set per synced table (`rpl_<table>_ai|au|ad`). Why not the driver hooks: a trigger runs in the same transaction as the row edit, it is pure SQL so every process that opens the file is captured (CLI, TUI, MCP, DAV), and it does the uuid lookups for references in the same statement. The driver's pre-update hook (modernc v1.40.0 and later) sees one connection and calls Go code per row.

- Insert writes a change for every column that has a value. Update writes only the columns that changed. Delete writes a `delete`. Inserting a uuid that was deleted before writes a `restore` first (merge undo does this).
- Each trigger ticks the clock once. All fields of one statement share one `hlc` and get consecutive `seq` numbers.
- `replica_clock.applying` is the `begin_remote` / `end_remote` flag. The triggers do nothing while it is 1. Remote applies run in a transaction that sets it and clears it before commit, so a rollback clears it too.
- A column that is a foreign key to another registered table syncs as the target's uuid. Columns that point to one of several tables by a kind column (taggings, activity targets, custom fields) are declared as `Poly`.
- Triggers are generated from the registry and the table's columns. `replicasql.Refresh` runs inside `db.Open` and rebuilds them when the signature changes, for example after a migration adds a column. The new column then syncs without a registry change.

Turning sync on (`Enable`) gives the database a device id (`meta.replica_device_id`), writes a baseline change for every current value without touching a row, and installs the triggers. The baseline clock of a row is its `updated_at`, so old data loses to newer edits on another device. Running it again repairs the triggers and fills in what is missing.

## Applying remote changes

`Engine.ApplyBatch` in one transaction:

1. Check the shape and the clock drift. Skip a change that is already stored (same device, same field, `seq` not higher). That is the idempotent replay by `(device, seq)`.
2. Store it. If the collection or the field is not in this build's schema it is stored with status `unknown` and not applied. It is still passed on to other devices. After an upgrade `Refresh` retries them.
3. Resolve each touched record and project it. A record that cannot be written yet (a required field or a referenced row has not arrived, or a table rule refuses it) is kept in `replica_pending` with the reason and retried after every batch. `crm peers status` lists them as blocked records.
4. Raise the version vector and move the clock past the newest change.

The result depends only on the set of changes, never on their order or on duplicates. The conformance test proves it for both adapters.

### Conflict policies

Declared per column in the registry, default `lww`.

| Policy | Result | Used for |
|---|---|---|
| `lww` | The change with the latest `hlc` wins, the device id breaks ties. | Everything else. |
| `keep-both` | Concurrent edits are all kept, oldest first, joined with a separator line. An edit that a later edit had already seen is dropped. | `notes` of people, organizations, deals. `content` of interactions. `description` of tasks. |
| `max` | The largest value wins. | `people.last_contacted_at`. |
| `set-union` | JSON arrays are merged. | Implemented, no column uses it yet. |

### Delete versus edit and the review queue

A `delete` carries `seen`. An edit that the delete had not seen (its `seq` is above `seen[device]`) is concurrent with it. Then:

- The record is parked: an open item in `replica_review`. The record stays visible, with all the edits, on every device that has both changes. A device that had already deleted the row gets it back. Nothing is lost.
- `crm peers resolve ID --keep` writes a `restore`. Every device brings the record back and closes its own copy of the item.
- `crm peers resolve ID --delete` writes a new `delete` whose `seen` covers the edits. Every device removes the record and closes the item.
- A device that sees the `restore` or the new `delete` close its own item by itself (status `auto`).

A delete that had seen every edit is applied at once and nothing is queued.

### Tombstones

`delete` is a record and stays in `replica_changes` until every enrolled device has passed it. `PurgeTombstones` (command `crm peers purge`, and `crm peers serve` on start and every hour) removes a delete, and the whole history of its record, when every enrolled and not retired device has reported a cursor that includes it and this database holds everything that device had written when it reported. With no other device enrolled nothing is purged, because a device enrolled later could bring the record back. Records in the review queue are kept. `crm peers retire DEVICE` releases the hold of a lost device. Run the purge on the relay, which knows every enrolled device.

## Adding a synced table

1. Make sure the table has `uuid TEXT` (unique) and, if it has timestamps, `created_at` and `updated_at`. If it has no uuid, set `AddUUID: true` in the registry line and `Enable` adds and fills the column.
2. Add one line to `DefaultRegistry()` in `internal/replica/replicasql/registry.go`. That is all for a table whose columns are plain values or foreign keys to registered tables.
3. Add options only when needed:
   - `Policy: {"col": replica.KeepBoth}` for a conflict policy other than lww.
   - `Poly: [...]` for an id column whose table depends on a kind column.
   - `Skip` and `Local` for a column that must not sync but is `NOT NULL` (give the SQL literal rows made by sync get).
   - `Natural: []string{"name"}` when the table has a unique name. Rows that exist at `Enable` get a uuid derived from the name, so the same built-in or same-named row made on two devices is one record.
   - `Pair: [2]string{"a","b"}` for a CHECK that needs the smaller id first.
4. Bump `SchemaVersion` in `registry.go` when peers should notice the change.
5. Run the tests. `TestRegistryCoversEveryTableOrExplainsWhy` fails for a table that is neither in the registry nor in `NotCovered()`. Add the table to one of them, with the reason.

A foreign key to a table that is not registered excludes that column and lists it in `crm peers status`.

## What is covered

25 tables: people, organizations, interactions (notes and activity), deals, tasks, tags, taggings, person_emails, person_phones, person_handles, custom_fields, relation_kind, membership, person_relation, org_relation, deal_person, deal_org, activity_target, merge_alias, note_types, vocabulary (deal roles and stages), name_aliases, company_ignore, dupe_dismissals, org_dupe_dismissals.

Not covered, with the reason (the code list is `NotCovered()`): external_ids, sync_runs, sync_cursors, sync_inbox, sync_conflicts, dav_state, field_versions (all tied to this device or to a source account), tombstones (replaced by delete changes), merge_log (its undo data holds device-local row ids), meta. `merge_alias.old_id` is local: rows made by sync get `-1`, the old uuid and the survivor do sync. FTS tables and the keep-in-touch view are derived.

## Known limits

- A unique rule can refuse a record that arrives from another device: two tags with one name created after sync was on, the same email on one person twice, a unique name. The record is kept as blocked, with the reason, and is never dropped.
- A delete of a row that another row still references without a cascade (`people.merged_into`, a relation kind in use) waits as blocked until the reference is gone.
- Replication is complete in v1: a view's query does not filter what syncs.
- No encryption, no signed authorship, one owner. The relay refuses a public bind unless `--allow-public`.
- A device id must belong to one database. Copy a database before `crm peers init`, never after. The relay refuses to enroll a device id twice, and refuses a push that claims its own id with a sequence it never wrote.
- No terminal UI screen for review or peers yet (see [[roadmap]]).

## Cost of capture

Measured on 5000 contacts with about 14 synced values each (`go test -v -run Overhead ./internal/replica/replicasql`): inserting them in one transaction takes 0.23 s without sync and 1.4 s with it (+0.24 ms per contact). Turning sync on for 5000 existing contacts takes 2.5 s and writes 70,000 baseline changes. A single field edit costs 1.1 ms. Pulling all 70,000 changes in pages of 5000 takes 0.7 s.

Related: [[sync-safe-data]], [[soft-delete-restorable]], [[conflicts-keep-server-store-local]], [[research-sync-building-blocks]].
