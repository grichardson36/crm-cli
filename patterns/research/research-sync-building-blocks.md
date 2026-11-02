---
type: research
---
# Sync building blocks: libraries and implementations to adopt or study for the Go + SQLite sync contract

Checked 2026-10-27 with the GitHub API (`gh`), the GitLab API for modernc.org/sqlite, proxy.golang.org, and file reads of repo sources. Feeds [[sync-contract]]. Full engines were already rejected in [[research-sync-engines]]. Wire-format precedents are in [[research-sync-standardization]]. This note covers only parts: clocks, op-log code, SQLite change capture, transport.

Contract recap: field-change records, HLC, per-field LWW, tombstones, version-vector cursor, pull/push over HTTP, relay as a role, Tailscale trust, SQLite adapter first, Go, modernc.org/sqlite, no CGO.

## Findings

### Clocks

- P1. **Go HLC libraries exist but none is a good dependency.** All are tiny and none enforces a maximum drift (checked by reading source: no drift check in `lafikl/hlc`, `glycerine/hlc`, `olekukonko/hlc`).
  - `lafikl/hlc`: BSD-3-Clause, 44 stars, last push 2017-07-03, no releases. `Timestamp` has `ts` and `count`, string form is `"TS=%d Count=%d"` (not sortable), no mutex. https://github.com/lafikl/hlc
  - `olekukonko/hlc`: MIT, 1 star, last push 2026-05-29, no releases. Mutex-protected clock, packs into `uint64`, JSON and binary marshal. https://github.com/olekukonko/hlc
  - `glycerine/hlc`: license file says "BSD-style" (GitHub reports NOASSERTION), 1 star, last push 2026-01-24. Packs 48-bit physical plus 16-bit counter into `int64`. https://github.com/glycerine/hlc
  - Others found by search (`alinz/hlc`, `avatar29A/hlc`, `nikosl/go-hlclock`, `shafreeck/hlc`) have no push since 2020 to 2021.
  - `cockroachdb/cockroach` `pkg/util/hlc`: the file header says it is governed by the "CockroachDB Software License" in `/LICENSE`. That is not an OSI license. Skip.
  - Gap: none of these is a string form that sorts lexicographically, which [[sync-contract]] rule 3 requires. The HLC algorithm is about 40 lines (see `lafikl/hlc` `Now` and `Update`). Writing it ourselves costs less than auditing a one-star dependency.

### Actual Budget

- P2. **Actual Budget's CRDT layer is the closest design match and is small, but it is TypeScript with no Go port.** `packages/crdt` is a separate npm package `@actual-app/crdt` 3.1.4, license MIT in its package.json, last commit touching it 2026-10-17. Contents: `crdt/timestamp.ts`, `crdt/merkle.ts`, `proto/sync.proto`. Repo: MIT, 29,777 stars, latest release v26.10.0 on 2026-10-02. https://github.com/actualbudget/actual/tree/master/packages/crdt
  - **HLC string form** (from `timestamp.ts`): `ISO-8601 millis` + `-` + 4-hex counter (`MAX_COUNTER` 0xFFFF) + `-` + 16-char node id. Fixed width, sorts as a string. This is the encoding to copy.
  - **Max drift is 5 minutes** (`maxDrift: 5 * 60 * 1000`, comment "Allow 5 minutes of clock drift"). [[research-sync-standardization]] Y22 and its "Smallest real contract" say 1 minute. That figure does not match the current source. Fix before it goes into the spec.
  - **Wire message** (`sync.proto`): `Message {dataset, row, column, value}` inside `MessageEnvelope {timestamp, isEncrypted, content}`. `SyncRequest` has `since`, `groupId`, `fileId`. `SyncResponse` has `messages` and a `merkle` string. This is a per-field change record with a clock, same shape as ours, but with no `op`, no `schema_version`, and no device/seq id.
  - **Merkle diff** (`merkle.ts`): base-3 trie keyed by minute of the timestamp, murmurhash of the timestamp string, `diff()` returns the earliest differing minute. It finds where two peers diverge. We chose a version-vector cursor instead, so this is not needed. Study only.
  - **Separability**: `packages/crdt` has no dependency on the rest of the app. `packages/loot-core` (client apply logic in `src/server/sync/`) is licensed ISC in its package.json (differs from the repo-level MIT). The sync server (`packages/sync-server`, `app-sync.ts`) rejects a request with no `since` ("since-required").
  - **Go port**: none found. Searched GitHub for Go repos mentioning Actual crdt and sync server. Zero results.
  - Verdict: study. License permits copying the algorithm with notice.

### any-sync

- P3. **any-sync is a full protocol stack and not separable.** Go, MIT (LICENSE.md, Copyright 2023 Any Association), 1,748 stars, last commit 2026-10-13, latest tag v0.13.8. https://github.com/anyproto/any-sync
  - Packages: `commonspace` (spaces, `headsync`, `object/tree`, `acl`, `sync`), `net` (peer, pool, `secureservice`, `quicdemotion`, `streampool`), `nodeconf`, `consensus`, `coordinator`, `paymentservice`, `nameservice`, `identityrepo`, `accountservice`.
  - Data model: signed, end-to-end encrypted DAGs per object, with an ACL chain. Its go.mod pulls in `go-libp2p` v0.50.0, `quic-go`, `webtransport-go`, `storj.io/drpc`, and a `modernc.org/sqlite` indirect dependency.
  - Conflict model is DAG-tree change merging (document-oriented), not per-field LWW. Out-of-scope features in [[sync-contract]] ("Not in v1": encryption, signed authorship, untrusted relays, multi-user) are what it is built around.
  - `commonspace/headsync` (`remotediff.go`, `diffsyncer.go`) is the part worth reading: it compares sets of object heads between peers to decide what to pull. Related: `anyproto/any-store`, MIT, 98 stars, pure-Go embedded document DB, last push 2026-10-21 (not read in depth).
  - Verdict: study `headsync` only. Skip as a dependency.

### CRDT and op-log libraries in Go

- P4. **No maintained Go library fits an LWW field-change op-log over SQL.** Candidates found by search (language:go, topics crdt and local-first):
  - `ipfs/go-ds-crdt`: Merkle-CRDT key-value datastore for go-datastore. Dual MIT and Apache-2.0 (GitHub: NOASSERTION), 490 stars, release v0.6.8 on 2026-02-05, last push 2026-10-08. Needs IPFS/libp2p pubsub and DAG services. Key-value, not rows. https://github.com/ipfs/go-ds-crdt . Skip.
  - `soundcloud/roshi`: LWW-element-set on Redis, BSD-2-Clause, 3,175 stars, last commit 2026-03-23, no releases. Large-scale server that needs Redis. https://github.com/soundcloud/roshi . Skip.
  - `yorkie-team/yorkie`: document store for collaborative apps, Apache-2.0, release v0.7.25 on 2026-10-13. A server plus SDK, document CRDT. Skip.
  - `sourcenetwork/defradb`: peer-to-peer edge database, release v1.1.0 on 2026-09-08. GitHub reports no license (the licence file was not read, gap). Database, not a library. Skip.
  - `Deln0r/ygo`: Go port of Yjs, MIT, 88 stars, last push 2026-10-26. Text and document CRDT. Not needed for LWW. Skip unless `keep-both` for notes ever needs real text merge.
  - `redwood/redwood` (MIT, last push 2023-03-07), `neurodrone/crdt` (2018), `weaveworks/mesh` (2023-12) are stale.
  - `Sambigeara/fuzzynote`: AGPL-3.0, 387 stars, release v0.46.2 on 2026-10-26. Terminal note app with its own CRDT-backed append-only WAL files (`primary.db`, `wal_*.db`) synced through an S3 remote. App, not a library, and AGPL blocks copying. Study the idea of per-device WAL files only.
  - Gap: I did not find a Go package named for "LWW-element-set" that is both maintained and general purpose.

### SQLite change capture

- P5. **Triggers are the capture mechanism to use, and modernc exposes the pieces for an alternative.**
  - modernc.org/sqlite: latest tag v1.61.0 on 2026-10-21 (proxy.golang.org, GitLab tags). Hosted on GitLab `cznic/sqlite`, BSD-3-style licence plus SQLite public domain (not re-read today). Its CHANGELOG records `RegisterPreUpdateHook`, `RegisterCommitHook`, `RegisterRollbackHook` and `SQLitePreUpdateData` (`Depth`, `Count`, `Old`, `New`) since v1.40.0 (2025-10-17), reached through `(*sql.Conn).Raw()`. A later entry fixes silent truncation of large 64-bit rowids in the pre-update trampoline, so use a version past that fix. Also exposed: `RegisterConnectionHook`, `NewConnector`, an authorizer, custom functions, and a pure-Go vtab API. https://gitlab.com/cznic/sqlite
  - **Session extension**: the generated `lib/` is built with `-DSQLITE_ENABLE_SESSION` and `-DSQLITE_ENABLE_PREUPDATE_HOOK` (read from the generator header of `lib/sqlite_linux_amd64.go`), and `sqlite3session_create` is present in `lib/sqlite_g_*.go`. The driver's root package has no session wrapper (no session file in the tree, no mention in README or CHANGELOG). Using it would mean calling the transpiled C functions through `modernc.org/libc` by hand. Not verified: whether the Windows build has the same flag (the header read was Linux). Also, [[research-sync-standardization]] Y21 shows session changesets need identical schema and carry no clocks, so they cannot be the wire format regardless.
  - **Triggers pattern**: an `AFTER INSERT/UPDATE/DELETE` trigger per synced table writes into a local `changes` table inside the same transaction, guarded by a flag row set during `begin_remote`/`end_remote` (contract rule 13, B7). `go-oversync` `oversqlite` does trigger-based dirty-row capture (see P6). No library needed. This is the only approach that is pure SQL and survives driver changes.
  - **`vlcn-io/cr-sqlite` as schema reference only**: MIT, 3,817 stars, release v0.16.3 on 2024-01-17, last commit 2026-08-10. Its change table `crsql_changes` has columns `table`, `pk`, `cid`, `val`, `col_version`, `db_version`, `site_id`, `cl`, `seq` (README). Maps to ours as: `table`=collection, `pk`=key, `cid`=field, `val`=value, `site_id`+`db_version`=(device, seq), `col_version` is a per-column Lamport counter where we use an HLC, and `cl` is a causal length (odd means alive, even means deleted) that replaces tombstone flags and handles delete then restore. Useful for rule 8 and the `restore` op. It is a C loadable extension, so it stays rejected as an engine ([[research-sync-engines]] E1). https://github.com/vlcn-io/cr-sqlite
  - **`sqliteai/sqlite-sync`**: Apache-2.0 (GitHub and LICENSE text), 595 stars, release 1.2.0 on 2026-09-28, last push 2026-10-22. SQLite loadable extension, CRDT algorithms (Causal-Length Set, Delete-Wins, Add-Wins, Grow-Only Set), per-table `cloudsync_init('tasks')`. Sync goes to SQLite Cloud CloudSync, PostgreSQL, or self-hosted Supabase. It is a C extension, so modernc cannot load it. The sync backend is a service we would not run. Skip, but its delete-wins and add-wins policy names are a vocabulary reference for rule 7. https://github.com/sqliteai/sqlite-sync
  - Not verified: SQLite Cloud's offline sync licensing and pricing (the README points to a pricing page), and whether sqlite-sync's protocol is documented.

### Closest open-source SQLite sync in Go

- P6. **`mobiletoly/go-oversync` is the closest Go implementation found and it is the wrong topology.** README: "Sync local SQLite clients with a PostgreSQL-backed Go server." Apache-2.0 per `LICENSE.txt` header (GitHub reports NOASSERTION), 6 stars, release v0.4.0, last push 2026-10-21. Parts: `oversqlite` (Go SQLite client SDK), `oversync` (server runtime and HTTP handlers). Features from the README: trigger-based dirty-row capture on SQLite, staged push sessions, complete committed-bundle pulls with stable checkpoints, conflict hooks, frozen snapshot rebuilds, fail-closed schema validation. The server is authoritative PostgreSQL, so there is no peer or relay role. Its go.mod uses `mattn/go-sqlite3` v1.14.37, which needs CGO. https://github.com/mobiletoly/go-oversync . Verdict: study `oversqlite` for trigger design, push staging and snapshot rebuild. Do not depend on it: one maintainer, 6 stars, v0.4.0, Postgres-centered, CGO driver.
- P7. **Other Go hits for local-first/sync are not relevant.** Search results for topic `local-first` (language:go) are mostly AI-agent tools (`YaoApp/yao`, `kenn-io/agentsview`, `zzet/gortex`). `anyproto/any-sync` is the only sync protocol in the top 15 (P3). Topic `crdt` yields P4. `maulanashalihin/hook-sync` (1 star, last push 2026-09-11) claims "SQLite replication via sqlite3_preupdate_hook plus batched HTTP shipping" (description only, code not read). No project implements the contract's combination of HLC, per-field LWW and a version-vector pull/push in Go.

### Transport and relay

- P8. **Plain `net/http` is enough for the two calls.** Go standard library, BSD-3. `pull` is a GET or POST with a cursor and `limit`, `push` is a POST. Long-poll can be added later (Electric's pattern, Y5 in [[research-sync-standardization]]). No dependency.
- P9. **Tailscale: `tailscale.com/tsnet` is optional.** `tailscale/tailscale`: BSD-3-Clause, 38,132 stars, last push 2026-10-27. `tsnet` embeds a tailnet node inside a Go program so a relay listens on the tailnet with no separate daemon and can read the caller's identity. The contract assumes the host's Tailscale. `tsnet` pulls a large dependency tree and I did not measure binary size or check Windows behavior (gap). It would replace the per-device token with tailnet identity, which is a decision for the owner, not required.
- P10. **NATS: skip.** `nats-io/nats-server` Apache-2.0, v2.15.2 on 2026-10-26, `nats.go` v1.55.0 on 2026-10-23. Embeddable broker with JetStream persistence. It adds a message system and ops surface where the contract needs two HTTP calls, and its streams are not a version-vector store.
- P11. **libp2p: skip.** `libp2p/go-libp2p` MIT, v0.51.0 on 2026-10-23. Gives NAT traversal, discovery, and encrypted streams. Tailscale already provides reachability and the contract excludes mesh transports ("Not in v1").
- P12. **Syncthing protocol libs: skip as a dependency, study as a model.** Syncthing: MPL-2.0, v2.1.7 on 2026-10-21. The block exchange protocol lives in `lib/protocol` inside the main module (files `bep_*.go`, `vector.go`, `deviceid.go`, `encryption.go`), not as a separate module. It is file and block oriented (Index, IndexUpdate, Request, Response), with per-file version vectors in `vector.go`. MPL-2.0 is file-level copyleft, workable but unnecessary. Its `vector.go` is a reference for version-vector semantics and `deviceid.go` for device ids. Syncthing as a file transport for per-device append-only files was already noted in [[research-sync-engines]] E12.
- P13. **rqlite: skip.** `rqlite/rqlite` MIT, v10.6.5 on 2026-10-24, 17,872 stars, very active. Raft-replicated SQLite with one leader. Clients cannot write offline and conflicts never arise because writes are serialized. That is replication, not local-first sync. Go client `rqlite/gorqlite` MIT, last push 2026-10-20.
- P14. **Turso Go driver: not now.** `tursodatabase/turso-go` is archived (MIT, last push 2026-01-06). The current driver location and CGO status were not verified. See [[research-sync-engines]] E10.
- P15. **PocketBase is archived on GitHub.** `pocketbase/pocketbase`, MIT, archived flag true as of today, last push 2026-10-26. Not a sync library. Noted because it is a common Go-and-SQLite reference and its status changed (cause not checked).

## Comparison

| ID | Item | What it is | License | Maintenance (checked 2026-10-27) | Fit with contract | Verdict |
|---|---|---|---|---|---|---|
| P1 | lafikl/hlc | Go HLC, struct only | BSD-3-Clause | Last push 2017-07-03 | Non-sortable string, no drift check | Skip |
| P1 | olekukonko/hlc | Go HLC, uint64 | MIT | Last push 2026-05-29, 1 star | Mutex, no drift check, not string-sortable | Skip (read as reference) |
| P1 | glycerine/hlc | Go HLC, int64 | BSD-style | Last push 2026-01-24, 1 star | No drift check | Skip |
| P1 | cockroach pkg/util/hlc | HLC in CockroachDB | CockroachDB Software License | Active | Not OSI | Skip |
| P2 | Actual Budget packages/crdt | TS HLC string, merkle, proto | MIT (package), loot-core ISC | Package commit 2026-10-17 | Same record idea, HLC string to copy | Study |
| P3 | any-sync | Go encrypted DAG sync stack | MIT | Commit 2026-10-13, tag v0.13.8 | Different model, heavy deps | Study headsync only |
| P4 | go-ds-crdt | Merkle-CRDT datastore | MIT + Apache-2.0 | v0.6.8, 2026-02-05 | IPFS key-value | Skip |
| P4 | roshi | LWW-set on Redis | BSD-2-Clause | Last commit 2026-03-23 | Needs Redis | Skip |
| P4 | yorkie | Collaborative doc store | Apache-2.0 | v0.7.25, 2026-10-13 | Server plus doc CRDT | Skip |
| P4 | ygo | Yjs port in Go | MIT | Push 2026-10-26 | Text CRDT only | Skip for now |
| P4 | fuzzynote | App with WAL CRDT sync | AGPL-3.0 | v0.46.2, 2026-10-26 | App, license blocks reuse | Study idea only |
| P5 | modernc.org/sqlite hooks | Pre-update, commit hooks | BSD-style | v1.61.0, 2026-10-21 | Optional capture alternative | Adopt (driver already chosen) |
| P5 | SQLite session extension | Changeset capture | Public domain | Compiled in, no Go wrapper | No clocks, strict schema | Skip |
| P5 | triggers | SQL capture | n/a | n/a | Direct match to rule 13 | Write |
| P5 | cr-sqlite | C extension, CRDT tables | MIT | Release 2024-01-17, commit 2026-08-10 | Cannot load in modernc | Study schema |
| P5 | sqlite-sync | C extension, SQLite Cloud sync | Apache-2.0 | 1.2.0, 2026-09-28 | Cannot load, needs cloud backend | Skip, vocabulary only |
| P6 | go-oversync | Go SQLite client plus Postgres server | Apache-2.0 (header) | v0.4.0, 2026-10-21, 6 stars | Wrong topology, CGO driver | Study |
| P8 | net/http | Go stdlib | BSD-3 | n/a | Exact fit | Adopt |
| P9 | tsnet | Embedded Tailscale node | BSD-3-Clause | Active | Optional | Skip for v1 |
| P10 | NATS | Message broker | Apache-2.0 | v2.15.2, 2026-10-26 | Extra service | Skip |
| P11 | go-libp2p | P2P network stack | MIT | v0.51.0, 2026-10-23 | Mesh out of scope | Skip |
| P12 | Syncthing lib/protocol | BEP, version vectors | MPL-2.0 | v2.1.7, 2026-10-21 | File/block oriented, not a module | Study `vector.go` |
| P13 | rqlite | Raft-replicated SQLite | MIT | v10.6.5, 2026-10-24 | No offline writes | Skip |

## Recommended

**Adopt**
1. Go standard library `net/http`, `encoding/json`, `database/sql` (P8).
2. `modernc.org/sqlite` at v1.61.0 or later, which is already chosen. Use triggers for capture and `RegisterConnectionHook` if a per-connection "applying remote" flag is wanted (P5). Treat the pre-update hook as a fallback, not the plan.

**Write ourselves (small, and nothing safe to import)**
3. HLC: about 40 lines plus a fixed-width string form. Copy Actual's encoding (ISO millis, 4-hex counter, node id) and its algorithm shape, set drift in config, reject far-future timestamps on receive (P1, P2).
4. `changes` table, trigger set, apply with per-field clock guard, tombstone and review queue, version-vector cursor, snapshot, adapter interface and the shuffled-and-duplicated conformance test ([[sync-contract]] rules 2, 4, 8, 9, 11, 13, 14).

**Study before coding (read, do not import)**
5. Actual `packages/crdt` (`timestamp.ts`, `sync.proto`) for HLC string and message shape.
6. cr-sqlite `crsql_changes` columns and its causal-length delete/restore, for rule 8 and the `restore` op.
7. go-oversync `oversqlite` for trigger design, push staging and snapshot rebuild.
8. Syncthing `lib/protocol/vector.go` for version vector semantics.
9. any-sync `commonspace/headsync` only if cursor comparison needs a faster diff later.

**Skip**: all Go HLC packages, go-ds-crdt, roshi, yorkie, defradb, NATS, libp2p, rqlite, sqlite-sync, SQLite session extension, tsnet for v1.

Net: there is nothing in Go to adopt for the core. The contract's novelty is small enough (HLC, a table, a loop) that reading three reference implementations and writing about 500 lines is the lowest-risk route.

## Could not verify

- Whether the Windows modernc build has `SQLITE_ENABLE_SESSION` (header read was `linux/amd64`).
- modernc.org/sqlite license text today (assumed BSD-3-style from memory).
- Exact first-party license of `sourcenetwork/defradb` (GitHub shows none) and of `ipfs/go-ds-crdt` beyond the README statement of MIT and Apache-2.0 dual license.
- `hook-sync` and `any-store` code quality (descriptions only).
- Memory and binary size cost of `tsnet`, libp2p, NATS embed.
- sqlite-sync/SQLite Cloud pricing, protocol documentation, and the CGO/driver status of the current Turso Go client.
- Why PocketBase is archived.
- Search coverage: GitHub search ranks by stars and misses new or small repos. A Go port of Actual's CRDT could exist under a name not matched.

## Open questions

- Q1. Which maximum drift: Actual's source says 5 minutes, [[research-sync-standardization]] says 1 minute. Pick one for [[sync-contract]] rule 3.
- Q2. Capture by triggers (pure SQL, adapter-portable) or by the pre-update hook (Go, less SQL to maintain, driver-specific)? Triggers are recommended here.
- Q3. Should delete/restore use cr-sqlite's causal length instead of tombstone plus `restore` op? It removes the "delete wins or edit wins" ambiguity for repeated delete/restore cycles but is a different record shape from [[sync-contract]] rule 2.
- Q4. Per-device token or Tailscale identity via `tsnet` for relay auth?
- Q5. Is fuzzynote's per-device WAL file model (carried by Syncthing or S3) worth keeping as a no-server transport alongside HTTP? See [[research-sync-engines]] E12.
