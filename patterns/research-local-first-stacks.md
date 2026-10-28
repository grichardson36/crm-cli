---
type: research
---
# Local-first stacks in practice: stay in Go, rewrite in Rust, or hybrid

Read 2026-10-25 from GitHub repo metadata (API), crates.io API, first-party docs and web search summaries. Feeds [[sync-safe-data]] and [[local-first]]. Related: [[research-sync-engines]], [[research-sync-standard]], [[research-terminal-images]]. Not verified: Local-First Conf 2026 lineup (no program found), Anytype and Logseq storage details, Rust CardDAV server crates, Windows Terminal image behavior with ratatui-image (its matrix does not list Windows), rusqlite build needs on Windows. Items tagged (secondary) came from search summaries or third-party write-ups.

Question: (a) keep Go with a hand-rolled op-log plus HLC, (b) rewrite in Rust, (c) something else.

## Findings

### What real apps use

- L1. **Actual Budget is the closest match to our shape.** Single owner or household, several devices, full local SQLite copy, optional sync server. Sync is an op-log of CRDT messages with hybrid logical clocks (string-comparable: time, counter, node id), a Merkle trie to find missing messages, 1 minute max clock drift, Protobuf on the wire, Node.js server on better-sqlite3. The message log is stored beside the data (data stored twice, accepted because the DB is small). TypeScript, MIT, v26.10.0 on 2026-10-02. This is the design E13 in [[research-sync-engines]] proposes, and it runs in production in a non-Rust, non-CRDT-library stack. https://jlongster.com/using-crdts-in-the-wild (secondary), https://github.com/actualbudget/actual
- L2. **Linear is server-authoritative, not peer to peer.** Client keeps an in-memory object graph (MobX) persisted to IndexedDB. Writes are queued transactions sent to the server, which assigns a global monotonic sync ID and broadcasts deltas. Rejected transactions roll back locally. Not our model (multi-user, server owns order). Source is a third-party reverse-engineering, not Linear docs. https://github.com/wzhudev/reverse-linear-sync-engine (secondary)
- L3. **Anytype (any-sync) is Go.** Protocol components are Go, data is encrypted DAGs per space, changes are CRDT-based and signed, and sync, file, consensus and coordinator nodes exist. MIT. Multi-device and multi-user. A real Go sync stack exists, built custom rather than on a CRDT library. https://github.com/anyproto/any-sync
- L4. **Obsidian Sync uses per-file strategies.** Markdown merges with diff-match-patch (or writes a conflict file, since 1.9.7), other files are last-modified-wins, settings JSON merges by key with local on top. Single-owner multi-device, file-based, not record-based. https://obsidian.md/help/sync/troubleshoot
- L5. **Logseq's DB version is built on datascript (datalog) with a db.sqlite file.** Clojure, AGPL-3.0, release 2.0.2 on 2026-10-07. The page read does not state its sync mechanism. https://github.com/logseq/logseq/blob/master/docs/dev-practices.md
- L6. **Zed uses CRDTs only for live collaborative text buffers.** Rust, Lamport timestamps, tombstones, per-user undo. Collaboration is a different problem from single-owner sync. v1.25.3 on 2026-10-23. https://zed.dev/blog/crdts
- L7. **Standard Notes is TypeScript, AGPL-3.0, release 2026-10-17.** Sync model not confirmed this session (help page returned 404). Treat as unknown. https://github.com/standardnotes/app
- L8. **Tailscale is Go (BSD-3-Clause, v1.104.2, 2026-10-21).** Relevant only as evidence that a large, long-lived networked product ships in Go. It is not a local-first data app. https://github.com/tailscale/tailscale
- L9. **Replicache is archived and Zero needs Postgres plus a cache server** (see E6 in [[research-sync-engines]]). Both target multi-user web apps with a server of record.
- L10. **Conference coverage.** No Local-First Conf 2026 program found. Sync Conf 2025 listed Iroh (Rust device-to-device sync) and PowerSync (SQLite in the browser). FOSDEM 2026 had a local-first devroom with ElectricSQL and Loro talks. Language mix is TypeScript heavy with Rust in the CRDT and networking libraries. No Go sync talk found. https://buttondown.com/local-first/archive/cfp-talks-for-sync-conf , https://fosdem.org/2026/schedule/track/local-first , https://archive.fosdem.org/2026/schedule/event/TDGH7X-local-first/

Single-owner multi-device like ours: Actual Budget (L1), Obsidian Sync (L4), Logseq (L5, unconfirmed). Multi-user or server-authoritative: Linear (L2), Anytype (L3), Zed (L6), Zero (L9).

### What Rust would unlock

- L11. **rusqlite 0.41.0 (2026-10-24), 135M downloads.** Mature and fast moving. It loads C extensions, so cr-sqlite becomes loadable. The `bundled` feature compiles SQLite with a C compiler at build time, so the "no C toolchain" property we have with modernc is lost (from rusqlite docs knowledge, not re-read today). https://crates.io/crates/rusqlite
- L12. **cr-sqlite is not unlocked in practice.** Its repo language is Rust, but last release is v0.16.3 on 2024-01-17, commits through 2026-08-10, open "is this project dead?" issue (see E1). Rust makes loading it easy, maintenance remains the blocker. https://github.com/vlcn-io/cr-sqlite
- L13. **Loro (loro-crdt 1.17.1, 2026-10-22, MIT) and Automerge (core JS 3.5.0, 2026-09-16, MIT) are native or first-class in Rust** (Loro is Rust). Both are document CRDTs. Using them means storing contacts as CRDT documents beside or instead of relational rows, which conflicts with [[sync-safe-data]] and the positional migrations. A rewrite alone does not remove that mismatch. https://github.com/loro-dev/loro , https://github.com/automerge/automerge
- L14. **Turso (Rust SQLite rewrite) 0.8.3 on 2026-10-12, MIT, 25k stars.** Pure Rust, no C. Sync still pre-1.0 with unverified conflict semantics (E10). Its Go driver `tursogo` exists, so Rust is not required to try it. https://github.com/tursodatabase/turso
- L15. **ratatui 0.30.2 (2026-06-19, 64M downloads) plus ratatui-image 12.0.0-rc.3 (2026-10-18, 1.2M downloads).** ratatui-image supports Sixel, Kitty, iTerm2 and a halfblocks fallback, with a terminal matrix. It reports Kitty, WezTerm, iTerm2, Ghostty, Foot, xterm working and Alacritty, Konsole, Warp not working. Windows is not mentioned on the page. It is a release candidate, not stable. Bubble Tea has no image widget, so images in Go need go-termimg or similar ([[research-terminal-images]] F5). Same terminal limits apply to both stacks, so the gain is library convenience, not new terminal reach. https://github.com/benjajaja/ratatui-image
- L16. **MCP SDK maturity is equal.** Rust: official `rmcp` 3.7.0 (2026-10-22), 40M downloads, repo modelcontextprotocol/rust-sdk. Go: official `modelcontextprotocol/go-sdk` v1.8.0 (2026-09-14), plus community `mark3labs/mcp-go` v1.3.0 (2026-10-14, MIT), which we use at v0.45.0 in go.mod (behind by one major, a separate upgrade task). Both active. https://github.com/modelcontextprotocol/rust-sdk , https://github.com/modelcontextprotocol/go-sdk , https://github.com/mark3labs/mcp-go
- L17. **vCard and CardDAV crates are thinner in Rust.** Found: `vcard4` 0.7.3 (2026-02-07, 56k downloads), `calcard` 0.3.14 (2026-09-15, 75k downloads), `ical` 0.11.0 (last update 2024-03-13). We use `emersion/go-vcard` and `emersion/go-webdav` (client and server). No Rust CardDAV server library of equivalent scope was found (not verified). Our `internal/dav` is 2,306 source lines on top of go-webdav. https://crates.io/crates/vcard4 , https://crates.io/crates/calcard

### What it would cost

- L18. **Size of crm-cli Go code (read-only count on 2026-10-25, excluding .claude worktrees):** 296 files, 68,643 lines, of which 24,522 are tests (44,121 non-test). Largest: `internal/tui` 13,018 + 8,099 test, `internal/db/repo` 8,065 + 4,504 test, `internal/cli` 7,736 + 4,184 test, `internal/quick` 2,601 + 1,108, `internal/mcp` 2,644 + 1,363, `internal/dav` 2,306 + 1,124, `internal/shortsyntax` 1,664 + 1,140, `internal/sync` 1,150 + 422, `internal/model` 1,191 + 128. See table below.
- L19. **Rewrite effort estimate (judgment, not measured).** 68k lines is the scope to port, with 24k of tests to port or regenerate for equivalence. The TUI (21k lines with tests) is the largest piece and ratatui is immediate-mode, not Elm style, so it is a redesign, not a translation.
- L20. **No direct Rust equivalent, or a changed property:** Bubble Tea and Bubbles (to ratatui, different architecture), Cobra (clap, similar), go-webdav server plus client (L17), go-pretty tables, modernc pure-Go SQLite (rusqlite bundled needs a C compiler, Turso is pure Rust but pre-1.0), mcp-go API (rmcp differs), go-vcard (vcard4, different API), Go cross-compile to Windows and ARM Pi with `GOOS`/`GOARCH` and no toolchain (Rust needs targets and linkers, doable but heavier), existing agent workflow and tests the owner already has in Go.

### Hybrid

- L21. **Rust library via FFI into Go needs CGO**, which breaks the modernc no-CGO property. Loro has no maintained Go binding (E3).
- L22. **Rust sidecar over a pipe or local HTTP avoids CGO.** The Go process spawns or talks to a separate binary. Candidates: a Loro or Turso sync daemon, or an image renderer. Cost: two toolchains, two release artifacts for Windows and Pi, an IPC contract. Image rendering gains little because Go libraries cover Kitty and Sixel and the problem is terminal behavior, not language ([[research-terminal-images]] F5, F6).
- L23. **Sync as a separate Go package stays inside the one binary.** Actual Budget (L1) and Anytype (L3) both show op-log or DAG sync built in the app's own language.

## Comparison

| Option | Sync approach | Language and build | Effort | Images in TUI | MCP and DAV | Main risk |
|---|---|---|---|---|---|---|
| A. Stay Go, op-log plus HLC | Hand-rolled, per-field LWW (L1, L23) | Pure Go, modernc, no CGO | Smallest: add `changes` table, extend 1,150-line `internal/sync` | go-termimg or similar on Bubble Tea, spike needed | mcp-go and go-webdav as today | We own convergence tests |
| B. Rewrite in Rust | Same op-log, or Loro, or cr-sqlite (L12, L13) | Rust, rusqlite bundled (C compiler) or Turso (pre-1.0) | Port 68k lines incl. 21k TUI redesign | ratatui-image, RC, Windows unlisted | rmcp ok, no CardDAV server crate found | Months of no feature progress, thin DAV crates |
| C1. Go plus Turso sync (tursogo) | Vendor push/pull | Go driver, CGO unknown | Medium, driver swap | as A | as A | Pre-1.0, conflicts unverified, acquisition pending |
| C2. Go plus Rust sidecar for sync | Loro or Turso in sidecar | Go + Rust binaries | Medium-high, IPC contract | as A | as A | Two toolchains, document model mismatch |
| C3. Go plus Rust sidecar for images | Sidecar renders images | Go + Rust | Medium | ratatui-image via process | as A | Complexity for a feature with a fallback |
| C4. Go plus Litestream backup | None (backup only) | Separate binary | Small | as A | as A | Not a sync answer |

Package size table (lines, files):

| Package | Source | Tests |
|---|---|---|
| internal/tui | 13,018 (38f) | 8,099 (29f) |
| internal/db/repo | 8,065 (28f) | 4,504 (22f) |
| internal/cli | 7,736 (41f) | 4,184 (30f) |
| internal/mcp | 2,644 (14f) | 1,363 (14f) |
| internal/quick | 2,601 (5f) | 1,108 (1f) |
| internal/dav | 2,306 (7f) | 1,124 (2f) |
| internal/shortsyntax | 1,664 (4f) | 1,140 (3f) |
| internal/model | 1,191 (10f) | 128 (2f) |
| internal/sync | 1,150 (6f) | 422 (1f) |
| internal/dedupe | 723 (2f) | 236 (1f) |
| internal/theme | 614 (3f) | 300 (2f) |
| internal/importer | 516 (1f) | 178 (1f) |
| others (db, doctor, help, keys, modules, schema, format, agentdoc, cmd) | 1,905 | 2,000 approx |
| Total | 44,121 (296 files all) | 24,522 |

## Recommended

Ranked answer.

1. **Stay in Go with the op-log plus HLC (A).** The closest real example, Actual Budget (L1), runs this design for single-owner multi-device in production, in a non-Rust stack, with a local SQLite copy. Nothing in Rust unlocks a sync approach we cannot reach in Go: cr-sqlite is stale (L12), Loro and Automerge need a document model (L13), Turso sync is pre-1.0 (L14). MCP parity (L16) removes a second argument. The rewrite cost is 68k lines (L18, L19).
2. **Add a Go spike for Turso sync (C1) only as a watched alternative**, per [[research-sync-engines]] E10. Not now.
3. **Keep images in Go (A).** Run the A2 and A3 spikes in [[research-terminal-images]] first. A Rust sidecar (C3) is not justified before that.
4. **Rust rewrite (B) last.**

Conditions under which a Rust rewrite becomes justified (any two):

1. Turso reaches 1.0 with documented field-level conflict handling and a production self-hosted sync server, and `tursogo` needs CGO or lacks features, so only Rust embeds it cleanly.
2. Loro or Automerge becomes the chosen data model (for example for rich text notes or collaborative editing) and no Go binding exists.
3. The Bubble Tea image spikes fail on the owner's terminals and ratatui-image demonstrably works there, including Windows Terminal, as a stable release.
4. A Rust vCard plus CardDAV server library of go-webdav's scope exists and is maintained.
5. The owner adds a client that needs a Rust core, such as a mobile app sharing the sync engine through FFI.
6. Go's modernc SQLite becomes a measured performance or correctness blocker.

One condition alone (for example images) is cheaper to meet with a sidecar (C2, C3) than a rewrite.
