---
type: feature-area
---
# Domain model and boundaries DDD review intake, 2026-10-29. These are proposed changes, not implementation decisions. Read [[adding-features]] before claiming a row. **Follows patterns:** [[core]], [[object-registry]], , [[interface-conventions]], [[link-registry]], [[edge-adapters]] **See also:** [[features-organizations]], [[features-tasks-and-notes]], [[features-cli-mcp-and-agents]], [[features-sync-and-dav]], [[open-discrepancies]]

## Rows | ID | State |

Current evidence | Decision | Next work | P |
|---|---|---|---|---|---|
| R01 | TODO | `people.company` and Person–Organization links represent the same fact. | [[organization-links-are-authoritative]] | Remove company and migrate it to primary Person–Organization links. | P1 |
| R02 | TODO | Stored interactions are presented inconsistently as notes. | open | Decide the public Activity name, then rename all doors together. | P1 |
| R03 | TODO | Person updates and organization automation are separate writes. | open | Define the smallest shared write path for R01. | P1 |
| R04 | TODO | Object knowledge is split between model, TUI, CLI, MCP and views. | [[object-registry]] | Decide the smallest shared metadata seam. | P1 |
| R05 | TODO | CLI context and the TUI person page assemble related data separately. | [[interface-conventions]] | Define one reusable person briefing query. | P1 |
| R06 | TODO | CardDAV state and mappings still refer to `people.company`. | [[organization-links-are-authoritative]] | Map ORG to the primary Person–Organization link after R01. | P2 |
