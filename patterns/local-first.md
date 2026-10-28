---
type: pattern
scale: product
status: decided
---

# The program works with no account and no network

**Problem.** Hosting, sync and accounts creep in until the tool stops working offline.

**Therefore.** Everything works from one binary and local SQLite files. Hosting and sync are optional conveniences.

**Because.** The buyer is a technical person with agents. Self-hosting stays free (BUSINESS.md).

**Smaller patterns.** [[sync-safe-data]], [[never-test-on-real-data]], [[extension-ladder]], [[import-is-a-snapshot-sync-is-a-relationship]], [[sync-maps-structure-to-flat-fields]]

**Sources.** BUSINESS.md decisions 2026-10-14

**Inspired by.** [[obsidian-sync-and-publish]], [[simplefin-and-exe-dev]], [[calendly]], [[us]]
