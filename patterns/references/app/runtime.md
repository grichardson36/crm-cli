---
type: reference
---
# App runtime

| Concern |Current choice |
|---|---|
| Language | Go 1.26 |
| Executable | One `crm` binary from `cmd/crm` |
| Database | SQLite through pure-Go `modernc.org/sqlite` |
| CLI | Cobra |
| TUI | Bubble Tea, Bubbles and Lip Gloss |
| Agent protocol | MCP through `mcp-go` |
| Contact protocol | CardDAV/WebDAV through `go-vcard` and `go-webdav` |
| Search | SQLite FTS5 |

See `go.mod` for the authoritative dependency versions.
