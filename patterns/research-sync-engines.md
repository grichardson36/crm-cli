---
type: research
---
# Sync engines: what exists and what fits a Go + SQLite local-first CRM

Read 2026-10-25 from GitHub repo metadata (API) and first-party docs. Feeds [[sync-safe-data]] and [[local-first]]. Related: [[research-sync-standard]], [[conflicts-keep-server-store-local]], [[soft-delete-restorable]]. Not verified: whether Turso's Go driver needs CGO, whether Loro has a maintained Go binding, PowerSync license terms in detail. Test before relying on them.

Constraints: one owner, several devices (Windows PC, Pi "pibrain", phone via CardDAV), Go + modernc SQLite with no CGO, offline, field-level merge, tombstones, positional migrations, no vendor lock, self-hostable.

## Findings

- E1. **cr-sqlite is a C loadable extension, last release v0.16.3 on 2024-01-17.** Last commit 2026-08-10 (build fixes for Android, iOS, macOS). An open issue asks "Is this project dead?" (#444). It adds CRDT tables (per-column LWW, causal-length tombstones) to SQLite and merges by exchanging changesets. It needs a loadable extension, which modernc.org/sqlite does not load, so it forces CGO or a different driver. Its automigrate has open gaps (non-constant DEFAULT, triggers). https://github.com/vlcn-io/cr-sqlite
- E2. **Automerge is a JSON-document CRDT, not a SQL engine.** JS release 3.5.0 on 2026-09-16, active. The Go wrapper `automerge-go` uses cgo, last push 2024-10-30. Merge is per property and same-property concurrent writes resolve deterministically. Mapping relational rows plus links into documents means rebuilding the data model. https://github.com/automerge/automerge-go and https://www.inkandswitch.com/essay/local-first/
- E3. **Loro is a Rust CRDT library (maps, lists, movable tree, text).** loro-crdt 1.17.1 on 2026-10-22, very active, MIT. Official bindings are Rust, JS, Swift, Python. No maintained Go binding found. Same document-model mismatch as Automerge. https://github.com/loro-dev/loro
- E4. **Electric SQL syncs Postgres to clients over HTTP (Shapes).** Apache-2.0, active (last push 2026-10-06). It needs a Postgres source and is primarily a read path. Writes go back through your own API. No Go client, and SQLite is not the source of truth. https://github.com/electric-sql/electric
- E5. **PowerSync syncs a backend DB (Postgres, MongoDB, MySQL) to client SQLite through a sync service.** Service v1.27.2 on 2026-10-24, very active. The service license is source-available, not OSI (GitHub reports NOASSERTION, read the LICENSE). Client SDKs target JS, Flutter, Swift, Kotlin, .NET. No Go SDK. The write path is an app-defined upload queue, so merge logic is ours. https://github.com/powersync-ja/powersync-service
- E6. **Zero (Rocicorp) needs Postgres and a zero-cache server, with TypeScript clients.** Active (zero/v1.12.0-canary tags, 2026-10-24). Replicache is archived (2022). Mutators are server-authoritative, so it is not peer-to-peer offline-first. https://github.com/rocicorp/mono
- E7. **Triplit does property-level conflict resolution, TypeScript, AGPL-3.0.** Last release 2025-07-31, last push 2026-01-19. A founder joined Supabase on 2025-10-08 and Supabase said it will not integrate Triplit. Low maintenance signal. https://github.com/aspen-cloud/triplit and https://supabase.com/blog/triplit-joins-supabase
- E8. **Evolu is TypeScript local-first on SQLite, with soft-delete system columns and self-hostable relays.** Relay 4.2.17 on 2026-10-17, MIT, active. Evolu 8 does not yet migrate Evolu 7 databases. JS and TS only, so a Go process cannot be a peer. https://github.com/evoluhq/evolu and https://evolu.dev/docs/local-first
- E9. **Jazz is a TypeScript CoValue (CRDT) framework.** Still v2.0.0-alpha.59 (2026-10-04), MIT, small project (216 stars). Pre-stable API. https://github.com/garden-co/jazz
- E10. **Turso has two stacks.** Legacy libSQL embedded replicas forward writes to a cloud primary, so they are not offline-first. The new Turso (Rust rewrite, 0.8.3 on 2026-10-12, not yet 1.0) has `tursogo` with a sync DB: local reads and writes, explicit push and pull. `tursodb --sync-server` is documented as a local development and testing tool. Supabase has agreed to buy Turso (reported 2026), a vendor-direction risk. Conflict semantics, CGO need and migration behavior of the new sync are not stated on the pages read. https://docs.turso.tech/sdk/go/reference , https://docs.turso.tech/sync/local-sync-server , https://github.com/tursodatabase/turso
- E11. **Litestream (v0.5.17, 2026-08-31) and LiteFS (v0.5.14, 2025-04-22) are not sync.** Litestream streams one writer's WAL to object storage or a path for restore. LiteFS replicates a primary to read-only replicas. Neither merges concurrent writes. Litestream is a good backup layer beside any option. https://github.com/benbjohnson/litestream and https://github.com/superfly/litefs
- E12. **Syncthing (v2.1.7, 2026-10-21) syncs files, not records.** Two devices editing the same SQLite or export file produce a conflict copy of the whole file. It works as a transport for append-only per-device files, because those never conflict. https://github.com/syncthing/syncthing
- E13. **A hand-rolled op-log with hybrid logical clocks is small and fits our schema.** HLC (Kulkarni et al.) gives a total order that tolerates clock skew. Pattern: a `changes` table with (hlc, device, table, row_uuid, column, value), per-field last-writer-wins by HLC, tombstone as a `deleted_at` change, idempotent replay by (device, seq). It matches the uuid, updated_at, deleted_at and per-field versions already in [[sync-safe-data]]. Migrations stay positional because each change carries column name and schema version, so a peer can defer unknown columns. Cost: we own convergence tests, compaction and transport. HLC paper: https://cse.buffalo.edu/tech-reports/2014-04.pdf
- E14. **CardDAV stays a separate edge.** The phone is not a peer. It is mapped through the rules in [[research-sync-standard]], so no engine here has to run on the phone.

## Comparison

| Option | Model | Go + modernc, no CGO | Field-level merge | Tombstones | Migration story | Self-host | Maintenance (checked 2026-10-25) | Fit |
|---|---|---|---|---|---|---|---|---|
| cr-sqlite | SQLite extension, column CRDT | No (loadable C extension) | Yes, per column | Yes | Automigrate with gaps | Yes, no server needed | Release 2024-01, commits 2026-08, "dead?" issue | Poor |
| Automerge | JSON doc CRDT | cgo wrapper, stale | Yes, per property | Built in | Own document schema | Yes | Active | Poor (model mismatch) |
| Loro | Rust CRDT library | No Go binding found | Yes | Built in | Own | Yes | Very active | Poor |
| Electric | Postgres to client shapes | No client | Server decides | Via Postgres | Postgres migrations | Yes, needs Postgres | Active | Poor |
| PowerSync | Backend DB to client SQLite | No Go SDK | App-defined | App-defined | Sync rules | Source-available service | Very active | Poor |
| Zero | Postgres plus cache server | No | Server mutators | Via Postgres | Postgres | Yes, needs Postgres | Active | Poor |
| Triplit | TS full-stack DB | No | Property level | Yes | TS schema | Yes, AGPL | Stale since 2025 | Poor |
| Evolu | TS SQLite plus relay | No (JS only) | Yes | Soft delete column | v8 lacks v7 migration | Yes, relay | Active | Poor |
| Jazz | TS CoValues | No | Yes | Yes | Alpha | Yes | Alpha | Poor |
| Turso sync (tursogo) | SQLite-compatible Rust DB, push/pull | Go driver exists, CGO unknown | Unknown | Unknown | Unknown | Local sync server (dev tool) | 0.8.3 active, acquisition pending | Maybe, unverified |
| Litestream / LiteFS | WAL backup / replica | Separate process | None | n/a | n/a | Yes | Litestream active, LiteFS slow | Backup only |
| Syncthing of files | File sync | Separate process | None (whole file) | n/a | n/a | Yes | Very active | Transport only |
| Op-log plus HLC (hand-rolled) | Change log, per-field LWW | Yes, pure Go | Yes | Yes | Positional, versioned log | Yes, any transport | Ours | Best |

## Recommended

Ranked shortlist.

1. **Hand-rolled op-log with HLC (E13).** It is the only option that is pure Go on modernc, keeps our schema and positional migrations, merges per field, and has no vendor. Start with a `changes` table written in the same transaction as each row edit. Move changes between devices over the existing HTTP surface on pibrain, or as append-only per-device files carried by Syncthing (E12) if no server is wanted. Add Litestream for backup (E11). Risk: we own convergence correctness. Mitigate with a property test that applies shuffled and duplicated change sets and asserts identical state.
2. **Turso sync (`tursogo`), as a watched alternative (E10).** It is the only engine here with a Go driver, SQLite semantics and local-first push and pull. Do not adopt now. It is pre-1.0, conflict behavior and CGO need are unverified, the sync server is documented for dev use, and the acquisition is pending. Re-check when it reaches 1.0 and documents field-level conflict handling and a production self-hosted server.

Rejected: cr-sqlite (E1) needs a C extension modernc cannot load and has had no release in 33 months. Document CRDTs (E2, E3, E9) and TypeScript stacks (E6 to E8) force a data model or runtime rewrite. Postgres-centric engines (E4 to E6) add a server we do not need.
