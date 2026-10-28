---
type: pattern
scale: object
status: decided
---

# Core objects carry native fields, not custom ones

**Problem.** Fields every user needs, such as an organization's industry, size or website, buried in custom fields are hard to filter, sort and show.

**Therefore.** Fields that HubSpot and Salesforce both treat as standard on a record are real columns with a place in the record page header, the CLI, MCP and bulk edit. Custom fields stay for what is personal. For an organization: name, domain, website, industry, size, location, LinkedIn, description, logo, parents and children, people with roles, deals and an activity timeline.

**Because.** Owner decision 2026-10-19: "I like HubSpot and Salesforce's implementation better as native."

**Larger patterns.** [[objects-first]], [[one-record-page]]

**Smaller patterns.** [[roles-are-free-text]]

**Sources.** Owner decision 2026-10-19

**Inspired by.** [[hubspot]], [[salesforce]]
