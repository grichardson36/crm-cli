---
type: pattern
scale: object
status: decided
---

# One verb set for every object

**Problem.** Objects offer different verbs with different names and behaviour.

**Therefore.** The verbs are fixed: Add, Open, Edit, Link, Unlink, Delete, Restore, Mark, Undo, Redo. Palette labels read Verb object (Add organization). CLI reads crm object verb. A verb an object cannot support is absent, never renamed. Side effects (for example a deal marked won setting closed_at) are identical in CLI, MCP and TUI.

**Larger patterns.** [[objects-first]]

**Smaller patterns.** [[same-key-same-verb]], [[same-input-style]], [[soft-delete-restorable]], [[delete-asks-yes-no]]

**Sources.** CRUD convention. HubSpot and Pipedrive use one add and edit flow across record types
