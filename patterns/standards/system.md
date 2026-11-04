---
type: standard
domains:
  - architecture
  - runtime
  - application
---
# System standard

## Scope

Application boundaries, package ownership, runtime choices and dependency direction.

## Current shape

```text
CLI ─┐
TUI ─┼─> application behavior ─> model + validation ─> repositories ─> SQLite
MCP ─┘              │
                    ├─> CardDAV adapter
                    └─> replica adapter
```

| Concern | Current choice |
|---|---|
| Language | Go 1.26 |
| Executable | One `crm` binary from `cmd/crm` |
| Database | SQLite through `modernc.org/sqlite` |
| CLI | Cobra |
| TUI | Bubble Tea, Bubbles and Lip Gloss |
| Agent protocol | MCP through `mcp-go` |
| Contact protocol | CardDAV/WebDAV through `go-vcard` and `go-webdav` |
| Search | SQLite FTS5 |

`go.mod` is the dependency-version source.

## Rules

- `internal/model` owns shared object and link declarations.
- CLI, TUI and MCP are adapters. They do not become separate domain or persistence implementations.
- Repositories own SQLite operations. External adapters translate at their boundary.
- The default system remains local-first: no account, server or network is required for core behavior.
- A module boundary exists to simplify a caller or protect a domain invariant, not to mirror a feature name.

## Checks

- A new package has one owner, one direction of dependency and no duplicate write path.
- A library or runtime addition solves a present constraint that the existing stack cannot meet.
- A new service is optional unless the owner explicitly changes the local-first boundary.

## Evidence

Feature work is in [[features/system]].
