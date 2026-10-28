---
type: pattern
scale: object
status: decided, narrowed 2026-10-29 (deal roles moved to [[deal-roles]])
---

# Roles are free text with suggestions

**Problem.** A fixed list of roles never fits: CTO, board observer, advisor, founder. A list the user cannot extend gets ignored.

**Therefore.** The role on a **membership** (a person at a company) is free text. The Link form and the CLI suggest the values already in use, most used first and case folded: `crm roles`, the status line of the Link prompt, Tab to complete, `crm_link_roles` for agents. Nothing is validated against the suggestions. Job title stays one headline field on the person (`people.title`). Task and note links keep a free text role as well.

**Not for deal roles.** The role of a person on a deal is a sales role (champion, decision maker, economic buyer, blocker, signer), a different fact from the role at a company. It is a list the user manages, seeded and fully editable, and an unlisted role is created on purpose, not by accident ([[deal-roles]]). A person can be the CTO of a customer (membership role, free text) and the economic buyer of a deal (deal role, listed).

**Because.** Labeled links are rare and named by role in real data: champion, economic buyer, signer, billing contact. HubSpot's Buying Role property is empty in every row of the export ([[research-hubspot-export]] H4), so a fixed list shipped with no way to change it would not be used. Memberships stay free text because company roles have no useful common vocabulary. Deal roles have one, so they get a list the user can reshape.

**Larger patterns.** [[pick-or-create]]

**Smaller patterns.** [[person-belongs-to-many-organizations]], [[deal-roles]]

**Sources.** [[schema-er]] (Q25), [[research-hubspot-export]] H4

**Inspired by.** [[hubspot]], [[salesforce]]
