---
type: reference
---
# App architecture

```text
CLI ─┐
TUI ─┼─> application commands ─> model + validation ─> repositories ─> SQLite
MCP ─┘              │
                    ├─> CardDAV adapter ─> phone clients
                    └─> replica adapter ─> optional HTTP relay
```

`internal/model` owns shared object and link declarations. `internal/cli`, `internal/tui` and `internal/mcp` are interface adapters. `internal/db/repo` owns persistence operations. `internal/dav` and `internal/replica` are external adapters.
