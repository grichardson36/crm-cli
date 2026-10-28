---
type: research
---
# HubSpot export: how real data uses the relation schema

Source: owner's HubSpot export dated 2025-09-24 (Capstone Aero, Sales Ops). Read 2026-10-16 by headers, counts and aggregate statistics only. No names, emails or company data are recorded here. Feeds [[link-from-either-side]], [[objects-first]] and the schema ER review. Companion: `research-relationship-models.md`.

## Sizes
3,082 contacts, 3,735 companies, 16 deals, 746 emails, 151 notes, 163 calls.

## Findings

- H1. **Many companies per contact is normal.** 856 of 3,082 contacts (28%) have two or more companies (max 10). 4,013 contact-company links in total. Only 11 contacts have none.
- H2. **Primary flag is on the link.** Exactly 0 or 1 primary company per contact, always one of the contact's companies. 942 links are non-primary.
- H3. **No per-link role, title or dates.** HubSpot carries job title once on the person, so a person with two jobs has one title. Our membership with role, start and end dates goes beyond HubSpot (Salesforce AccountContactRelation is the only product found that does it).
- H4. **Labeled links are rare and named by role.** Champion, Economic Buyer, Contact Signer, Billing Contact, Partners, Sibling, Referral. 6 deals have a champion contact, always exactly one and always among the deal's contacts. HubSpot's Buying Role property is empty in all rows. Roles do not need a fixed list. Free text with suggestions fits ([[pick-or-create]]).
- H5. **Company hierarchy is single-parent, shallow and sparse.** 11 of 3,735 companies have a parent (single id each), 6 have children (max 4), 4 are both child and parent so two levels exist. Parent and child links are exactly reciprocal. Our decision allows several parents (owner decision 2026-10-16), which HubSpot does not.
- H6. **Other company links exist:** sibling (4 companies), generic company-to-company (29), partners (2).
- H7. **Activities attach to several object types at once.** Notes: 115 of 151 company-only, 14 contact plus company, 10 contact only. Calls: 108 contact plus company, 24 contact plus company plus deal. Emails: 631 contact plus company, 73 plus deal. Activities link to the company far more often than to the person.
- H8. **Links are stored on both sides, exactly reciprocal.** One link, two views, as in [[link-from-either-side]]. All ids resolve.
- H9. **Merges leave alias ids.** 1,007 contacts and 194 companies carry merged-in old ids. A merge needs an alias record so old references still resolve.
- H10. **Person-to-person links are almost unused** (2 of 3,082 contacts), with no label. Dex Related is similarly untyped.

## What this means for the schema

1. Memberships (person to many orgs) with a primary flag are required: 28% of real contacts need them.
2. Notes and interactions must attach to organizations and deals as first-class targets, not only to people.
3. Role labels on deal-person links are sales roles. First read as free text with suggestions, changed 2026-10-29 to a user-managed list seeded from this export (champion, economic buyer, signer, billing contact and the usual buying committee), see [[deal-roles]]. Membership roles stay free text.
4. Org hierarchy needs reciprocal parent and child views and at least two levels. Cycle checks cost nothing at this size.
5. Merge must keep aliases of merged ids, in addition to `merged_into`.
6. Import from HubSpot is a future adapter. Join tables are built by splitting the `* IDs` columns on `;` and using ids, never names.
