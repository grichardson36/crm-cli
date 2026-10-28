---
type: pattern
scale: object
status: decided
---

# Organizations have many parents

**Problem.** Real groups are not trees: a joint venture has two owners, a brand sits under a holding and a distributor. A single parent column cannot say that, and unchecked parents can loop.

**Therefore.** Organization to organization is a typed link (org_relation) of a kind from [[relation-kinds-have-inverses]]. The shipped kind is parent, read as "a is parent b", and from b the label is child. An organization may have several parents and several children. A link that would make an organization its own ancestor is refused in every door with a plain message. The organization page groups Parent organizations, Child organizations and Related organizations. The kind column is ready for partner, client and vendor.

**Because.** HubSpot has exactly reciprocal parent and child links, shallow and sparse: 11 of 3,735 companies have a parent, two levels exist ([[research-hubspot-export]] H5). Salesforce keeps one ParentId, HubSpot allows labels that permit more. The owner chose several parents and a cycle check, which costs nothing at this size.

**Larger patterns.** [[relation-kinds-have-inverses]], [[link-from-either-side]]

**Smaller patterns.** [[one-record-page]]

**Sources.** [[schema-er]] (org_relation, Q23), [[research-relationship-models]] section 2, [[research-hubspot-export]]

**Inspired by.** [[hubspot]], [[salesforce]]
