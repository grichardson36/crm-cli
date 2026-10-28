---
type: research
---
# Sync standardization: what exists, and the smallest database-agnostic contract

Date 2026-10-26. Trigger: the owner is defining a stack- and database-agnostic standard for local-first programs (views as queries over data, a shared registry of actions). Sync must be part of it without assuming one database. Feeds [[sync-safe-data]]. Builds on [[research-sync-engines]] (E-numbers), [[research-sync-standard]] (S-numbers), [[research-local-first-stacks]] (L-numbers) and [[research-view-module-standard]] (V-numbers, Q5 asks which data layer the standard assumes). Not repeated here: engine-by-engine fit for Go + SQLite.

Evidence tags: doc (first-party page read on 2026-10-26 through a summarizing fetch, so re-read field names before they go in a spec), search (search summary only), gap (not verified). Ink & Switch, Keyhive, p2panda, DXOS, Evolu, Zero wire formats, Solid details and Willow entry fields were mostly not read at spec level. See "Could not verify".

## Findings

### Part 1. Standardization attempts

- Y1. **No standard for local-first sync or the local-first data layer exists.** One attendee account (secondhand social post, search) says the opening talk at Local-First Conf 2026 (Berlin, 12 to 14 July 2026) suggested an IETF protocol standard for local-first sync "in the next few years", and that frameworks (Jazz, DXOS, PowerSync) are converging on shared patterns. The official talk list and recording were not found (gap). The FOSDEM 2026 local-first, sync engines and CRDTs devroom happened (doc, listed in [[research-local-first-stacks]] L10). A GNOME BoF on 2026-07-19 covered a system-level sync service (search). Treat all of this as signal of interest, not a spec. https://buttondown.com/local-first/archive/ , https://events.gnome.org/event/306/contributions/1596/ , https://archive.fosdem.org/2026/schedule/event/TDGH7X-local-first/
- Y2. **IETF Braid-HTTP is the only IETF-track attempt, and it is a dead individual draft.** `draft-toomim-httpbis-braid-http` latest revision -04 dated 2023-11-18, status "Expired Internet-Draft (individual)", no stream, not adopted by a working group, "not endorsed by the IETF" (doc). Its stated parts are versioning, subscriptions, range-based patches and merge types (search summaries of -01 to -04; the datatracker page excerpt cut off before listing them, so header names Version, Parents, Merge-Type are gap). Origin 2020-03-09 (search). Spec, not product. https://datatracker.ietf.org/doc/draft-toomim-httpbis-braid-http/
- Y3. **WebDAV sync-collection (RFC 6578) is the only finished IETF sync standard, and it fits one-way pull of a collection.** The client sends the last `sync-token` and gets only changed members plus a new token. Token is opaque and a valid URI. Deletions are reported as a member with 404 status. A member added then removed before the client syncs must still be reported removed. A server may truncate with 507 and a token covering the partial set. An invalid token forces a resync from an empty token. Initial sync uses an empty token (doc). It has no write-conflict model beyond ETag and `If-Match`, no field granularity, no clocks. Spec. https://www.rfc-editor.org/rfc/rfc6578
- Y4. **CardDAV (RFC 6352) itself uses ETag comparison, not sync-collection.** The visible text says clients may store objects offline, check changes by PROPFIND on `getetag`, and "SHOULD use If-Match on PUT"; it does not mention RFC 6578 in the part read (the last 15k characters were not read, gap). Whole-vCard replace only (S1 to S3). CalDAV was not fetched (gap). Spec. https://www.rfc-editor.org/rfc/rfc6352
- Y5. **Electric's HTTP protocol is the clearest published shape for a log-and-offset sync API, but it is Postgres-derived.** Client requests a shape with `offset=-1` for everything, then `handle` plus `offset`. `electric-offset` header gives the next offset. `live=true` holds the request until new data or timeout, `live_sse=true` uses SSE. Messages are JSON with `headers`, `key`, `value`, with an operation header (insert shown, update and delete values not confirmed on the page). Control messages: `up-to-date`, `must-refetch` (discard local shape and resync), `snapshot-end` (carries Postgres `xmin`, `xmax`, `xip_list`). Published as OpenAPI in the repo, so third-party clients are possible. Product with a documented protocol, not a standards-body spec. (doc) https://electric.ax/docs/api/http (old URL electric-sql.com/docs/api/http 301s here).
- Y6. **Replicache push is a documented request shape, pull was not read.** Push is `POST` with `pushVersion`, `clientGroupID`, `profileID`, `schemaVersion` and `mutations[]` each with `clientID`, `id` (sequential per client), `name`, `args`, `timestamp` (not used by the protocol). A mutation is done when pull returns `lastMutationID >= id`. Auth failure is 401. The pull request/response (cookie, patch ops) was not read (gap, https://doc.replicache.dev/reference/server-push). Replicache is archived and Zero succeeded it ([[research-sync-engines]] E6). Zero 1.0 reached stable in 2026 (search, https://infoq.com/news/2026/06/zero-version-1), its wire protocol was not found (gap). Product, protocol is documented only for Replicache. Mutation = named action plus args, which matches the shared action registry idea.
- Y7. **LiveStore is event sourced.** Docs index lists Events, Materializers, and sync providers (Cloudflare Workers, ElectricSQL, S2, custom). Docs version 0.5.0, "still work in progress" (doc). Mechanics (eventlog sequence numbers, rebase, schema evolution) were not read (gap). Product. The model matters because it is the opposite of per-field LWW: record intent events and derive state with deterministic materializers. https://docs.livestore.dev/
- Y8. **AT Protocol repo sync is a real, published, multi-implementer spec.** Two paths: full repo export as CAR over HTTP, and a firehose `com.atproto.sync.subscribeRepos` (CBOR over WebSocket). Each commit has a TID `rev` used as a logical clock that must increase per repo, with PDS hosts told to use wall time and consumers told to reject revs far in the future. `#commit` events carry `rev`, `since` (previous rev), `commit` CID, `blocks` (CAR diff), `ops[]` (`action` create/update/delete, `path`, `cid` null on delete, `prev`) and `prevData`. `#sync` re-asserts state after a reset. Limits: 200 ops, 2 MB blocks, 1 MB record. Page shows no spec version or date, and says the lexicon may change (doc). It is single-writer-per-repo, signed, content-addressed, with no concurrent multi-writer merge. Spec plus products (Bluesky, others). https://atproto.com/specs/sync
- Y9. **Matrix is a published spec for multi-writer rooms.** Spec v1.19 shown, copyright 2026, no release date on the page. Events form a DAG, each with parent events and a `depth`. State resolution is described as deterministic and transitive so all servers pick the same event. Room versions 1 to 12 exist; which algorithm each uses was not on the page (gap). Spec plus products. Relevant as the proof that a DAG plus a deterministic resolution function plus room version numbers can be standardized. Heavy for one owner. https://spec.matrix.org/latest/
- Y10. **Nostr is a minimal event format with a built-in LWW rule.** Event fields: `id` (SHA-256 of serialized pubkey, created_at, kind, tags, content), `pubkey`, `created_at`, `kind`, `tags`, `content`, `sig` (Schnorr, secp256k1). Replaceable kinds (0, 3, 10000 to 19999) keep the latest per pubkey and kind. Addressable kinds (30000 to 39999) key on kind, pubkey and `d` tag. Equal timestamps tie-break on lowest `id` lexically. `REQ` filters (`ids`, `authors`, `kinds`, `#tag`, `since`, `until`, `limit`) then `EOSE`. NIP-09 deletion was not read (gap). Spec (NIPs) plus many products. Clearest example of "conflict policy declared by the type of the record" (doc). https://github.com/nostr-protocol/nips/blob/master/01.md
- Y11. **Solid has a protocol but no sync.** Solid Protocol v0.11.0, Draft Community Group Report dated 2024-05-12 (doc). Change notification is by reference to Solid Notifications Protocol v0.3.0, same date. The spec warns a container `Last-Modified` is not a reliable change signal. PATCH on RDF uses N3 Patch (`solid:inserts`, `solid:deletes`, `solid:where`), 409 on mismatch. Spec (W3C CG, not a Recommendation). https://solid.github.io/specification/protocol
- Y12. **Keyhive and Beelay (Ink & Switch) address access control plus sync over untrusted servers.** Keyhive: capability-based write permission and key agreement for read control. Beelay: RPC sync protocol, server sees only encrypted data, usable over any confidential transport (HTTPS, WebSocket). Peers exchange Keyhive membership graphs, then compare document collections and sync out-of-date documents. Messages signed with Ed25519. Only a search summary of a lab note was found. No release status or 2026 update (gap). Research product. https://www.inkandswitch.com/keyhive/notebook/05/
- Y13. **Automerge sync protocol and automerge-repo.** automerge-repo has pluggable storage and network adapters (V6). The sync protocol page (heads, Bloom filters) was a 404 at the URL tried, so its wire format is unverified (gap). Product with an implementation-defined protocol.
- Y14. **Willow is a family of specs, aimed at being a standard.** Listed specs: Data Model, Meadowcap (capabilities), Drop Format, Willow Transfer Protocol, Willow'25 (recommended parameters), Confidential Sync, plus concepts such as region-based set reconciliation (doc). The page gives no status or dates and does not describe entry fields or deletion. My recollection (not verified today) is that entries are keyed by namespace, subspace and path with timestamp and payload digest, and that newer entries prune older ones under a path prefix. Spec family. https://willowprotocol.org/specs/index.html
- Y15. **p2panda is a Rust framework built on an append-only operation log.** Docs describe the Operation data type as a single-writer append-only log with pruning and fork resistance, CBOR, BLAKE3, Ed25519, and separate crates for groups, encryption, discovery and blobs. APIs "not yet stable", core data types may change before 1.0 (search). An interop "p2panda specification" is mentioned in an older note but contents were not found (gap). Product with spec ambitions. https://docs.rs/p2panda
- Y16. **DXOS, Evolu, Jazz, PowerSync, Zero have no cross-product interop spec.** DXOS ECHO protocol detail not found, sources conflict on whether it uses Automerge or Yjs (search, gap). Evolu protocol not found (gap, only [[research-sync-engines]] E8). Products only.

### Part 2. Database-agnostic change formats

- Y17. **Debezium envelope is the most widely deployed CDC record shape.** Value payload: `before` (null on create, filled per replica identity), `after`, `source` (version, connector, db, schema, table, txId, LSN, `snapshot` flag), `op` (`c`, `u`, `d`, `r` for snapshot read, and `t` truncate not visible on the page), `ts_ms` and variants, optional `transaction` (`id`, `total_order`, `data_collection_order`). Key payload holds primary key columns. Tables without a primary key emit only creates under default replica identity. Delete and tombstone emission was not on the read portion (gap, 78% of the page unread). Stable 3.7 (doc). It records row images, not per-field intent, and has no clock for merge. Product convention, not a standards body spec. https://debezium.io/documentation/reference/stable/connectors/postgresql.html
- Y18. **CloudEvents is an envelope, not a data-change format.** CNCF graduated 2024-01-25. Core spec latest listed v1.0.2 (2022-02-05). HTTP and WebSocket bindings, JSON and Protobuf formats. CloudEvents SQL v1 on 2024-06-13 is a filter language (doc). Required attributes were not on the page read; from the spec as I know it they are `id`, `source`, `specversion`, `type` (not verified today). Reusable as an outer wrapper for transport and routing only. Spec. https://cloudevents.io/
- Y19. **JSON Patch (RFC 6902, April 2013) addresses by JSON Pointer and is order-dependent.** Ops `add`, `remove`, `replace`, `move`, `copy`, `test`. Atomic: one failed op voids the patch. Array indices shift as ops apply, so a patch computed against one base misapplies against another. `test` gives optimistic concurrency (doc). AG-UI uses it for `StateDelta` (V24). Fit: document stores with object paths. Poor for concurrent merge. https://www.rfc-editor.org/rfc/rfc6902
- Y20. **JSON Merge Patch (RFC 7396, October 2014) cannot express null or partial array edits.** `null` means remove the member, so it cannot set a member to null. Arrays and non-objects are replaced whole. Media type `application/merge-patch+json`. Obsoletes RFC 7386 (doc). Good for a flat per-field set where null is never a value and deletion of a field is allowed. Our model treats NULL and cleared as meaningful, so it conflicts. https://www.rfc-editor.org/rfc/rfc7396
- Y21. **SQLite session extension changesets are SQLite-only and schema-strict.** Changeset stores full old values for update and delete. Patchset stores only primary key for delete and only new values for update, so it detects fewer conflicts. A table needs a declared PRIMARY KEY. Rows with NULL in a primary key column are ignored. A changeset applies only to tables matching name, column count and primary key columns. Conflict cases listed: INSERT with existing key or constraint violation, DELETE with missing row or differing values, UPDATE with missing row or differing originals (the callback type names were not on the page, gap). No clocks, no per-field LWW, no unknown-column handling (doc). Library format, not portable. https://sqlite.org/sessionintro.html
- Y22. **Hybrid logical clocks come from one 2014 tech report with many independent implementations.** "Logical Physical Clocks" (Kulkarni, Demirbas, Madappa, Avva, Leone), SUNY Buffalo tech report 2014-04. HLC keeps Lamport ordering and stays near physical time. Two parts, physical `l` and counter `c`. One source packs them in 64 bits (32 bit seconds, 16 bit fraction, 16 bit counter) but layouts differ by implementation, so there is no interoperable wire encoding. Drift is bounded in practice by configuration (CockroachDB `max_offset` default 500 ms, YugabyteDB comparable, search, secondary). Actual Budget uses a string-comparable HLC with 1 minute max drift ([[research-local-first-stacks]] L1). Paper, not a standard. https://cse.buffalo.edu/tech-reports/2014-04.pdf , https://www.cse.buffalo.edu//~demirbas/publications/hlc.pdf
- Y23. **The reuse gap.** Nothing found defines a portable per-field change record with an HLC, a tombstone and a schema version. Closest are Nostr (event identity and replace rule), Debezium (entity addressing and op codes), AT Protocol (rev as clock, create/update/delete ops with path), Electric (offset log, must-refetch) and RFC 6578 (opaque token, deleted-reporting, truncation).

## Comparison

| Item | Kind | Unit of change | Clock / order | Delete | Transport | Merge policy | Schema handling | Status (checked 2026-10-26) |
|---|---|---|---|---|---|---|---|---|
| Braid-HTTP | Spec (individual I-D) | Versioned resource, range patches | Version DAG | Not read | HTTP | Merge-Type per resource | Not read | Expired, -04 2023-11 |
| RFC 6578 sync-collection | Spec (IETF) | Collection member | Opaque server token | 404 member | HTTP/WebDAV | None (ETag, If-Match) | n/a | Final |
| CardDAV RFC 6352 | Spec (IETF) | Whole vCard | ETag | DELETE | HTTP | None | X- props kept by client | Final |
| Electric HTTP | Product, documented API | Row op in shape log | Offset + handle | delete op, must-refetch | HTTP long-poll / SSE | Server decides | Postgres | Active |
| Replicache push | Product, documented (archived) | Named mutation + args | Per-client mutation id | In mutator | HTTP POST | Server mutator | `schemaVersion` field | Archived |
| LiveStore | Product | Event | Event sequence (not read) | In event | Pluggable | Materializer | Not read | 0.5.0 |
| AT Protocol sync | Spec + products | Record op in signed commit | TID `rev` | `delete` op, null cid | CAR over HTTP, WS firehose | Single writer per repo | Lexicons | Spec shown without version |
| Matrix | Spec + products | Event in DAG | DAG + depth | Redaction (not read) | HTTP | State resolution per room version | Room versions | v1.19 |
| Nostr | Spec (NIPs) + products | Signed event | `created_at`, id tie-break | NIP-09 (not read) | WebSocket | Replaceable by kind range | Tags / kinds | Active |
| Willow | Spec family | Entry (path keyed) | Timestamp | Prefix pruning (unverified) | Several protocols | Newest wins | Not read | Dates not found |
| Solid | Spec (W3C CG draft) | Resource | None | DELETE | HTTP | None | RDF | v0.11.0 2024-05 |
| Keyhive/Beelay | Research product | Document | Per doc (Automerge) | n/a | Any confidential RPC | Automerge | n/a | Unknown |
| Automerge sync | Product | Doc changes | Causal heads | Built in | Adapters | Property CRDT | n/a | Wire format unverified |
| p2panda | Product, spec ambitions | Operation in per-author log | Per-author log | Pruning | iroh | Per app | n/a | Pre-1.0 |
| Debezium envelope | Convention | Row image (before, after) | LSN / ts_ms | `d` op | Kafka etc. | n/a | Schema registry | 3.7 |
| CloudEvents | Spec (CNCF graduated) | Opaque event | `time` optional | n/a | HTTP, WS, others | n/a | `dataschema` attr (recalled) | v1.0.2 |
| JSON Patch | Spec (IETF) | Path op | Order of ops | `remove` | HTTP PATCH | None | n/a | RFC 2013 |
| JSON Merge Patch | Spec (IETF) | Member set | None | `null` | HTTP PATCH | Last applied | n/a | RFC 2014 |
| SQLite session | Library format | Row, whole row images | None | Row delete | File / bytes | Callback | Strict match | Stable |
| HLC | Paper | Timestamp only | Hybrid clock | n/a | n/a | Total order | n/a | 2014 report |

## Contract needs

What a database-agnostic sync contract must define, and what to reuse. "Write" means no reusable existing work was found.

| Need | Minimum the contract says | Reuse | Write |
|---|---|---|---|
| Change record shape | One record = one field set or one entity delete. Fields: `id`, `hlc`, `device`, `collection`, `key`, `field` (path), `op`, `value`, `schema_version`, optional `tx` | Op codes and `source` naming from Debezium (Y17). Signed-event ideas from Nostr (Y10) and AT Protocol `ops[]` (Y8) | The record itself, 8 to 10 fields |
| Identity | `key` is a UUID minted by the creator, never a database rowid. `id` = (device, seq) gives idempotent replay | UUID (RFC 9562, recalled not read). Nostr event id as hash precedent | Rule that adapters map UUID to native key |
| Clocks | Total order by HLC string compare. Max drift bound. Reject far-future | HLC paper (Y22), Actual Budget's string form (L1), AT Protocol TID rev rule (Y8) | Exact byte encoding, since none is portable (Y22) |
| Tombstones | Delete is a change on the entity with its own hlc. Purge keeps a tombstone until all known devices pass it | RFC 6578 "added then removed still reported" (Y3). AT `delete` op (Y8) | Tombstone retention and the delete-versus-edit rule |
| Schema versioning and unknown columns | Each change carries `schema_version` and `field`. A receiver holding a change for an unknown field stores it unapplied, never drops it, never fails. Renames are explicit changes | Cambria lenses for version translation (V4, "not production-ready" per V1). Room versions (Y9) and `schemaVersion` (Y6) as precedent for a version number on the wire | The park-unknown-field rule and the rename change. No existing portable format found |
| Transport | A pull call "changes since cursor, limit" and a push call "append changes". Cursor opaque. Truncation allowed. Reset signal when cursor is too old | RFC 6578 token, 507 truncation and empty-token resync (Y3). Electric offset, `must-refetch`, `up-to-date`, long-poll or SSE (Y5). Replicache push (Y6) | Binding of the above to the change record |
| Auth and device identity | Each device has a keypair and an id. Changes carry the device id. Server accepts only enrolled devices | Ed25519 as used by Nostr-like (Schnorr), AT, Beelay, p2panda (Y8, Y12, Y15). Keyhive for multi-user (Y12, not needed for one owner) | Enrollment flow. Whether changes are signed or only the channel |
| Compaction and snapshots | A snapshot = current state per field with its hlc, plus the cursor it covers. A log may be truncated below the minimum cursor of all enrolled devices | `must-refetch` plus `snapshot-end` (Y5). AT `#sync` and CAR export (Y8). Actual Budget keeps log beside data (L1) | Snapshot format and retention rule |
| Conflict policy declaration | Per field type: `lww` (default), `keep-both-text` (notes), `set-union`, `max`, `immutable`. Declared in the schema, not the adapter | Nostr kind-range declares replace rule (Y10). Braid `Merge-Type` (Y2, search). Matrix room version fixes the resolver (Y9) | The vocabulary of policy names. Keep it to 3 to 5 |
| Partial replication | A subscription = collection plus filter (the view's query). Server sends matching changes only | Electric shapes (Y5), Nostr `REQ` filters (Y10), Willow regions (Y14, unread) | Mapping from a view query to a filter. The hard part is a change leaving the filter (needs a synthetic remove) |
| Ordering of related changes | Group by `tx` so a multi-field edit applies atomically | Debezium `transaction` block (Y17), session changeset as whole-row unit (Y21) | `tx` semantics |
| Actions as sync unit (optional) | Replicate named action plus args instead of field changes | Replicache mutations (Y6), LiveStore events (Y7) | Do not adopt for the base contract. Needs deterministic replay of every action on every peer |

## Mapping our op-log to adapters

Our design (from [[research-sync-engines]] E13): `changes(hlc, device, table, row_uuid, column, value)`, per-field LWW by hlc, tombstone as a `deleted_at` change, replay idempotent by (device, seq).

### SQLite adapter

Maps directly. `table` = collection, `row_uuid` = key, `column` = field. Apply = `UPDATE ... SET col=? WHERE uuid=?` guarded by "incoming hlc > stored per-field hlc" (needs a per-field hlc store, a `field_versions` table or a shadow column). Where it breaks:

1. B1. **Per-field hlc storage is not in the changes table.** LWW needs the winner's hlc per (key, field) at apply time. Either keep a clock table or scan the log. This is a required adapter duty.
2. B2. **Multi-field edits are not atomic** unless `tx` is added. A CHECK constraint or NOT NULL pair can be violated by partial application.
3. B3. **Constraint conflicts the log cannot see.** Two devices insert the same UNIQUE value (email, handle). Session extension also lists constraint failure as a conflict case (Y21). Contract needs a "rejected, park for review" outcome, tying to [[conflicts-keep-server-store-local]].
4. B4. **Foreign keys and arrival order.** A change for a child can arrive before its parent. The adapter must defer or disable FK checks during apply, then verify.
5. B5. **Types.** SQLite columns are dynamically typed and blobs and NULL are legal values. The `value` encoding must tag type, and NULL must be distinct from "no change". JSON Merge Patch cannot do this (Y20).
6. B6. **Rowid and autoincrement keys.** Any table keyed by integer rowid needs a UUID column. This is already rule 4 in [[sync-safe-data]].
7. B7. **Triggers and generated columns** (FTS, derived indexes) must not emit changes during apply, or replay loops. The adapter needs an "applying remote" flag.
8. B8. **Concurrent delete and edit.** LWW per field leaves a deleted row with newer field edits. The contract must state delete wins, edit-after-delete restores, or review. Not decided by any reused work.
9. B9. **Schema drift.** Session changesets require identical column count (Y21), so they cannot be the wire format. Our per-column changes can, which is the reason to keep them.
10. B10. **Counters and ordered lists.** LWW loses concurrent increments and manual order. Need a `max` or position policy.

### Document store adapter

`collection` = collection, `row_uuid` = document id, `column` = field path (JSON Pointer-like). Where it breaks:

1. B11. **Paths.** Nested fields work as long as every path segment is an object key. Arrays break: JSON Patch index shift (Y19) and Merge Patch whole-array replace (Y20). Arrays must be modeled as keyed maps (id to item) or declared whole-value LWW.
2. B12. **Null versus absent.** Document stores usually distinguish them. Needs explicit `op = set` versus `op = unset`, not "value null means delete".
3. B13. **Missing field versions.** Same as B1, stored in a sibling document or a side collection. Many stores cannot hold hidden per-field metadata atomically with the document, so the adapter needs a transaction or a single-writer apply.
4. B14. **Renames and moves across paths** have no merge meaning under per-path LWW (JSON Patch `move` is the only precedent, Y19). Treat as set plus unset and accept the race.
5. B15. **Relations are denormalized** (embedded copies), so one logical edit produces many changes with no `tx` link back to the source.
6. B16. **Atomicity scope.** Stores that only guarantee single-document atomicity cannot apply a multi-document `tx`. Contract must say `tx` is advisory.
7. B17. **Unknown fields are free** in a schemaless store, so the park-unknown rule only matters for validation layers.

Common to both: the contract record works; the adapter must supply B1 (clock per field), apply-time guards, and an "apply in progress" flag. Nothing in either adapter requires changing the record shape.

## Recommended

Not a decision. The smallest real contract, then what to adopt and what to write.

### Smallest real contract

1. **Record.** `{id: (device, seq), hlc, collection, key, field, op: set|unset|delete|restore, value (typed JSON), schema_version, tx?}`. Eight required fields, one optional. This is our `changes` row plus `op`, `schema_version` and `tx`.
2. **Identity.** `key` is a UUID. `id` makes replay idempotent.
3. **Clock.** One HLC string, fixed width, lexicographically sortable, max drift stated (start at Actual Budget's 1 minute, L1).
4. **Tombstone.** `delete` is a record. Purge keeps the tombstone until every enrolled device has passed it.
5. **Unknown handling.** Store, do not apply, retry after a schema upgrade. Never discard.
6. **Policy.** Schema declares one of `lww`, `keep-both`, `max`, `set-union` per field. Default `lww`.
7. **Two calls.** `pull(cursor, filter?, limit) -> {changes, next_cursor, up_to_date}` plus `reset` when the cursor is stale. `push(changes) -> {accepted_through}`. Cursor is opaque.
8. **Snapshot.** State per field with hlc, plus the cursor it represents. Used for new devices and for reset.
9. **Adapter interface.** `apply(change)`, `read_field_clock(collection, key, field)`, `emit_local(change)` hook on every local write, `begin_remote/end_remote` flag, `snapshot()`. Five methods.
10. **Conformance test.** Fixed change sets, shuffled and duplicated, applied through each adapter, must give identical state. This is the property test from [[research-sync-engines]] and it is the only thing that makes the contract real across two databases.

Not in the minimum: peer-to-peer discovery, encryption, multi-user permissions, text CRDTs, signed changes, action replication. Add later as optional layers.

### Adopt versus write

| Adopt | Source |
|---|---|
| HLC algorithm | Kulkarni et al. (Y22), encoding borrowed from Actual Budget's string form (L1) |
| Opaque token pull, truncation, empty-token resync, deleted-member reporting | RFC 6578 semantics (Y3), not its XML |
| `up-to-date` and `must-refetch` control messages, offset log over HTTP, long-poll or SSE | Electric HTTP protocol shape (Y5) |
| Op codes and `source` naming | Debezium (Y17) |
| Per-field merge policy chosen by record type, tie-break on lowest id | Nostr (Y10) |
| `rev`/`since` chaining so a consumer can detect a gap | AT Protocol (Y8) |
| Mutation id for exactly-once push of actions | Replicache (Y6), only if actions are ever replicated |
| CardDAV edge | Unchanged, via [[research-sync-standard]] |

| Write | Why |
|---|---|
| The record schema and the JSON wire encoding | No portable per-field record exists (Y23) |
| Unknown-field parking and the rename change | No reusable format found |
| Policy vocabulary and delete-versus-edit rule | Not specified by any work reviewed (B8) |
| Snapshot and compaction rule | Each product does its own |
| Adapter interface and the conformance suite | Core of the "database-agnostic" claim |
| View query to subscription filter mapping, including the synthetic remove when a row leaves the filter | Only Electric shapes are close, and they are Postgres-bound |

Do not adopt as the base: Braid-HTTP (expired, Y2), Solid (no sync, Y11), JSON Patch or Merge Patch as the record (Y19, Y20), session changesets (SQLite only, Y21), CloudEvents beyond an optional outer envelope (Y18), Matrix or Willow (heavy, multi-writer, signed, Y9, Y14).

## Could not verify

- Local-First Conf 2026 official program, the "IETF standard" remark (secondhand only), and any recording.
- Braid-HTTP header names and any 2025 or 2026 revision. Only -04 (2023-11-18) was confirmed.
- CalDAV and CardDAV use of RFC 6578 (the CardDAV text read did not cite it, last 15k characters unread).
- Debezium delete and tombstone emission, and the `t` op. 78% of the page unread.
- CloudEvents required attributes (the page read did not list them).
- Willow entry fields, pruning, spec dates and statuses.
- Keyhive and Beelay status, Automerge sync wire format (URL 404), p2panda spec contents, DXOS ECHO protocol, Evolu protocol, Zero wire protocol, LiveStore sequence and rebase mechanics, Replicache pull format.
- Matrix state resolution versions per room version, redaction. Nostr NIP-09.
- AT Protocol spec version and date.
- Solid Notifications content beyond the pointer.
- SQLite `sqlite3changeset_apply` conflict callback names (the intro page did not list them).
- Anything about RFC 9562 UUIDs, written from memory.
- All fetched pages passed through a summarizing model. Re-read exact field names and status before quoting them in a spec.

## Open questions for the owner

- Q1. Is the contract a record format plus two HTTP calls (log protocol), or only an adapter interface that each program wires itself? The first gives cross-program sync, the second only code reuse.
- Q2. Should actions (the shared registry) ever be the unit of sync, or only field changes? Action replication needs deterministic replay on every peer (Replicache, LiveStore) and conflicts with different databases.
- Q3. What is the delete-versus-edit rule: delete wins, later wins, or park for review (B8)?
- Q4. Single owner only, or must the contract allow several users? Multi-user adds signed changes and access control (Keyhive, Meadowcap), which the minimum leaves out.
- Q5. Are signed changes needed, or is channel auth between enrolled devices enough?
- Q6. Which policy names go in the vocabulary, and does `keep-both` (notes) belong in the base?
- Q7. Does a view's query double as the replication filter? If yes, the standard's query language must be expressible as a filter every adapter can evaluate (answers V-set Q5 in [[research-view-module-standard]]).
- Q8. Does the owner want a published spec with a second implementation (document store adapter) as proof, or an internal convention until the SQLite version is stable?
- Q9. Compaction: is "every enrolled device has passed it" acceptable, given a lost device blocks purge forever?

## Sources
- Braid-HTTP: https://datatracker.ietf.org/doc/draft-toomim-httpbis-braid-http/
- RFC 6578: https://www.rfc-editor.org/rfc/rfc6578 . RFC 6352: https://www.rfc-editor.org/rfc/rfc6352 . RFC 6902: https://www.rfc-editor.org/rfc/rfc6902 . RFC 7396: https://www.rfc-editor.org/rfc/rfc7396
- Electric HTTP API: https://electric.ax/docs/api/http
- Replicache push: https://doc.replicache.dev/reference/server-push . Zero 1.0 news: https://infoq.com/news/2026/06/zero-version-1
- LiveStore: https://docs.livestore.dev/
- AT Protocol sync: https://atproto.com/specs/sync
- Matrix: https://spec.matrix.org/latest/
- Nostr NIP-01: https://github.com/nostr-protocol/nips/blob/master/01.md
- Solid Protocol: https://solid.github.io/specification/protocol
- Willow: https://willowprotocol.org/specs/index.html
- Keyhive notebook: https://www.inkandswitch.com/keyhive/notebook/05/
- p2panda: https://docs.rs/p2panda
- Debezium: https://debezium.io/documentation/reference/stable/connectors/postgresql.html
- CloudEvents: https://cloudevents.io/
- SQLite session extension: https://sqlite.org/sessionintro.html
- HLC: https://cse.buffalo.edu/tech-reports/2014-04.pdf
- Local-First Conf and FOSDEM: https://buttondown.com/local-first/archive/ , https://archive.fosdem.org/2026/schedule/event/TDGH7X-local-first/ , https://events.gnome.org/event/306/contributions/1596/
