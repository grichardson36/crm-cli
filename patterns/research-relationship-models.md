# Research: how mature CRMs model relationships between objects

Date: 2026-10-16. Read-only research. No decision is made here.

Tags: [doc] = read from first-party documentation or API reference. [search] = from a search result snippet or third-party page, not verified at source. [general] = background knowledge, not verified this session. "Not documented" means I looked and found nothing, not that the feature is absent.

Coverage gaps: Dex has no public page for "Related" semantics beyond the API. Monica's help pages did not load (404), only API relationship-type pages surfaced via search. Attio and Folk help pages gave partial answers. Salesforce object pages returned 403, so Salesforce comes from search snippets of the official reference.

## 1. Person to organization

| Product | Several orgs at once | Past vs current | Role/title per membership | Primary flag | Start/end dates |
|---|---|---|---|---|---|
| Dex | Unclear. Company is a contact field, filters `has_company`, `has_linkedin_companies` [doc]. Work history is imported from LinkedIn [doc]. No company object found | Work history imported, not a modelled past-role record [doc] | Contact-level `job_title` filter [doc]. Per-membership not documented | Not documented | Not documented |
| HubSpot | Yes, many companies per contact [doc] | Not a built-in concept. Use custom labels such as "Former employee" [general] | Labels on the association (up to 10 per object pair) [doc]. Job title is a contact property [general] | Yes. Exactly one primary company per record, own type IDs (contact to primary company = 1) [doc] | None on the association [general] |
| Attio | Person has a `company` record reference. Company `team` is its inverse and accepts multiple values [doc]. Whether a person may hold several companies is not stated on the page | No "past companies" attribute in standard objects [doc] | `job_title` is a text attribute on the person, not on the link [doc] | Not documented | Not documented. Docs do not say a link can carry data [doc] |
| Folk | Yes, many companies per person [doc] | Not documented | Job title shown in company People section. Whether per-link is not stated [doc] | Yes, one "favorite" company [doc] | Not documented |
| Monica | Not confirmed (help pages did not load) | Not confirmed | Not confirmed | Not confirmed | Not confirmed |
| Notion | Yes, a relation property is many-to-many by default, limit can be set to 1 page [doc] | Not applicable, no built-in model | Only if you add an intermediate database | No | Only via intermediate database |
| Salesforce | Yes via AccountContactRelation. A contact has one direct account plus indirect accounts [doc via search] | Yes. `IsActive`, `StartDate`, `EndDate` [doc via search] | Yes. `Roles` multipicklist (Decision Maker, Executive Sponsor, Influencer, etc.) [doc via search] | Yes. `IsDirect` marks the primary account [doc via search] | Yes [doc via search] |

## 2. Organization to organization

- HubSpot: company to company is supported. Parent to child is type 13, child to parent is 14, so the pair is directional with two type IDs [doc]. Other kinds come from custom paired labels (for example partner, vendor) [doc]. Whether a company can have multiple parents is not stated. Labels allow it [general].
- Attio: Company page lists no parent/child attribute [doc]. Custom relationship attributes (one-to-one, one-to-many, many-to-one, many-to-many, all bidirectional) can be added on paid plans [doc].
- Folk: not documented in what I read.
- Dex, Monica: not documented for organizations.
- Notion: a self-relation on one database. One property gives a symmetric link. Turning on two-way gives two properties such as "Next" and "Previous" [doc].
- Salesforce: `ParentId` hierarchy on Account [general]. Partner links between accounts create two records, forward and reverse, with a `PartnerRole` table mapping each role to its `ReverseRole` [doc via search]. Cardinality: ParentId is a single parent [general].

## 3. Person to person

- Dex "Related": API has `GET /v2/contact-relations/{contactId}`, `POST .../add` and `.../remove`, each taking `contactIds` (array of UUID). The list response returns only id and name fields [doc]. So it is an untyped list of other contacts with no label, no role, no dates. Whether adding A to B also makes B show A is not documented. Treat symmetry as unknown.
- Monica: typed. Relationship types sit in groups (love, family, friend, work). Every type has an opposite, because creating a link creates two relationships, one per direction. Partner/spouse/date/lover use the same name both ways, father/child do not. Types and groups are read-only through the API [search]. The exact API page is monicahq.com/api/relationshiptypes.
- HubSpot: same-object associations support paired labels with `inverseLabel`, for example Manager/Employee between contacts [doc]. A single (unpaired) label is the symmetric case [doc]. Each direction has its own typeId [doc].
- Attio: custom two-way relationship attribute on People pointing to People would work [doc inferred, not tested]. No typed inverse label beyond naming the two attributes.
- Salesforce: Contact to Contact relationships are not standard. Account-Contact and Partner records exist [general].
- Notion: self-relation, symmetric or two properties [doc].
- Folk: not documented.

Typed kinds with inverses, as products define them:
- Directional pairs: parent/child, manager/report (HubSpot paired label, Monica, Salesforce PartnerRole/ReverseRole) [doc, search].
- Symmetric kinds: friend, spouse, partner, colleague, sibling [search, Monica; Virtuous snippet says most types are symmetric].

## 4. Tasks, deals, notes, meetings across objects

- HubSpot: tasks, notes, and meetings each associate to contact, company, deal, ticket. Type IDs: task to contact 204, company 192, deal 216. Note to contact 202, company 190, deal 214. Meeting to contact 200, company 188, deal 212 [doc]. Deal to contact 3, contact to deal 4 [doc]. Deal-contact labels exist (limit one "Point of contact" example) [doc].
- Dex: reminders link to contacts through a `reminders_contacts` array of `contact_id` (many contacts per reminder) [doc]. Notes are timeline items with a `contacts` array [search]. Meetings are note types (Meeting, Call, Coffee, Note) [doc]. Calendar events are not stored in Dex, read live from Google/Microsoft [doc]. No deals found.
- Attio: Companies have `associated_deals`, an inverse of the Deal `companies` relationship, which is multi-valued. People also have associated deals when Deals is activated [doc]. Notes and tasks attach to records. Multi-record attachment not confirmed here.
- Folk: groups hold people and companies, and a contact can be in many groups [doc]. Deals/notes/reminders not covered in what I read.
- Monica: journal posts and activities link to contacts [doc]. Activities with several contacts not confirmed.
- Salesforce: OpportunityContactRole carries a role on the link [general]. Tasks and events use polymorphic WhoId (contact or lead) and WhatId (account, opportunity, other) [general]. With Shared Activities, an event can have multiple contacts [general].
- Notion: any database can relate to any other. A task database with a relation to people and a relation to companies is the idiom [doc inferred].

## 5. How linking is exposed from either side, and what the link carries

| Product | From either side | Link record carries |
|---|---|---|
| Dex | Related list on a contact [doc] | Nothing found. IDs only |
| HubSpot | Both directions, each its own typeId. Labels via `associationTypeId` [doc] | Labels (up to 10 per object pair), primary flag, per-type count limits. No dates or free fields [doc] |
| Attio | Bidirectional attribute pair, edit either side updates both [doc] | Docs do not say the link carries data. Fields from the linked record can be shown in views [doc] |
| Folk | Person's companies and company's People section [doc] | Favorite flag only, as far as documented |
| Monica | Two rows per link, one per direction [search] | Relationship type with group and reverse name |
| Notion | One-way by default. "Show on" toggle makes it two-way [doc] | Nothing. Rollups aggregate from the related page |
| Salesforce | Related lists on both Account and Contact [general] | Roles, IsDirect, IsActive, StartDate, EndDate [doc via search] |

## 6. Storage as documented

- HubSpot: association is a first-class edge between two records. Each direction has its own typeId, and a label is a type definition, `HUBSPOT_DEFINED` or `USER_DEFINED`. Deleting the unlabeled type removes all labeled links between the pair [doc]. Internal table layout not published.
- Salesforce: AccountContactRelation is a real junction object with fields. Partner stores two rows per link linked by `ReversePartnerId` [doc via search].
- Monica: two relationship rows per link, per the API docs [search]. A third-party schema shows a `relationship_types` table with forward and reverse names [search, unofficial].
- Attio: each relationship is a pair of attributes, one per object [doc]. Internal storage not published.
- Notion: relation property. Storage not published.
- Dex, Folk: storage not published.

## Comparison table

| Capability | Dex | HubSpot | Attio | Folk | Monica | Notion | Salesforce |
|---|---|---|---|---|---|---|---|
| Person in many orgs | unclear | yes | unclear | yes | unconfirmed | yes | yes |
| Primary org flag | no | yes | no | yes (favorite) | unconfirmed | no | yes (IsDirect) |
| Role per membership | no | label | no (title on person) | unclear | unconfirmed | DIY | yes (Roles) |
| Start/end dates | no | no | no | no | unconfirmed | DIY | yes |
| Past vs current | LinkedIn import | via label | no | no | unconfirmed | DIY | IsActive |
| Org to org | not found | yes, parent/child + labels | custom attribute | not found | not found | self-relation | ParentId + Partner |
| Person to person | untyped list | paired labels | custom attribute | not found | typed, group, reverse | self-relation | not standard |
| Typed with inverse | no | yes | by attribute naming | no | yes | by two properties | yes (PartnerRole) |
| Tasks/notes to many objects | contacts array | yes, 4 object types | yes (records) | partial | partial | yes | polymorphic |
| Link carries data | no | labels, primary | no | favorite | type | no | yes, rich |

## Convention most products agree on

1. Person to organization is many-to-many.
2. One membership can be marked primary (HubSpot primary, Folk favorite, Salesforce IsDirect).
3. Links are visible and editable from both sides.
4. Tasks, notes, and meetings attach to several records of several object types.
5. Directional person-to-person or org-to-org kinds are stored as a pair with an inverse. Symmetric kinds use one name for both directions.

## Where they diverge

- Whether the link itself carries data. Salesforce: roles, dates, active flag. HubSpot: labels only. Attio, Notion, Dex: nothing documented.
- Past roles. Only Salesforce has start/end/active. Others rely on labels or imports.
- Typed person-to-person. Monica and HubSpot yes, Dex untyped, others DIY.
- Inverse handling. HubSpot: two type IDs. Monica: two rows. Salesforce: two rows plus role map. Attio and Notion: two paired properties.
- Org hierarchy. Salesforce single ParentId. HubSpot labeled associations. Attio none standard.
- Activities. HubSpot uses typed associations per object pair. Salesforce uses polymorphic IDs. Dex uses contact ID arrays.

## Candidate schema shapes for local SQLite, single user

### S1. Specific join tables

`person_org(person_id, org_id, title, is_primary, start_date, end_date, note)`, `org_org(parent_id, child_id, kind)`, `person_person(a_id, b_id, kind_id)`, plus `task_person`, `task_org`, `task_deal`, `note_person`, `note_org`, `deal_person(deal_id, person_id, role)`, and so on.

Pros: real foreign keys, simple SQL, each table has exactly the columns it needs, easy for a CLI to validate.
Cons: many tables, each new pairing needs a table, activity queries across types need UNIONs.

### S2. One generic link table

`link(id, from_type, from_id, to_type, to_id, kind_id, role, is_primary, start_date, end_date, note)`, with `link_kind(id, name, inverse_id, symmetric, from_type, to_type)`.

Pros: one table and one code path for all pairs. Inverse kinds live in data (HubSpot and Monica style), so new kinds are rows. Tasks, notes, and meetings use the same table.
Cons: no foreign keys on polymorphic columns, so orphans need triggers or app checks. Constraints like one primary per person need partial indexes per type. Queries need type filters.

### S3. Hybrid

Specific tables where the link has a rich shape (`membership` for person to org with title, dates, primary, a Salesforce AccountContactRelation analog; `deal_contact` with role) and one generic `relation(kind_id, a_id, b_id)` table with an inverse/symmetric kind table for person-to-person and org-to-org. Activities use one `activity_target(activity_id, target_type, target_id, role)` table.

Pros: foreign keys where data is richest, flexibility where it is not, matches the pattern Salesforce uses (junction objects for rich links) and HubSpot (typed edges).
Cons: three mechanisms to learn and document. Two places to look when asking "who is related to X".

Notes for the owner, not recommendations: symmetric kinds can be stored once with a canonical ordering (a_id less than b_id) or twice. Directional kinds need either an inverse kind id or two rows. Dex's own model is the simplest case (untyped ID list), which S2 or S3 can represent with a single default kind.
