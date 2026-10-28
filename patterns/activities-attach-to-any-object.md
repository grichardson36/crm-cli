---
type: pattern
scale: object
status: decided
---

# Activities attach to any object

**Problem.** Notes and tasks tied to one person miss how work happens: a call is about a company and a deal as much as about the contact. Per-person join tables force a new table for every new target.

**Therefore.** Tasks and interactions (notes, calls, meetings) point at people, organizations and deals through one table, activity_target (source kind and id, target kind and id). A task or note can have several targets of several kinds. The CLI, MCP and TUI attach with the one Link verb and with `--org` and `--deal` on `log add`. Organization and deal pages show their tasks and a Recent timeline, and `n` adds a note there. The polymorphic columns carry no foreign keys, so triggers refuse an orphan row and `crm doctor` counts any that remain.

**Because.** Activities link to the company far more often than to the person in the owner's export: 115 of 151 notes are company only, calls and emails mostly carry person, company and deal together ([[research-hubspot-export]] H7). HubSpot and Salesforce both associate activities with several record types.

**Larger patterns.** [[link-from-either-side]], [[objects-first]]

**Smaller patterns.** [[hidden-not-removed-links]]

**Sources.** [[schema-er]] (activity_target, E2), [[research-hubspot-export]], [[research-relationship-models]] section 4

**Inspired by.** [[hubspot]], [[salesforce]]
