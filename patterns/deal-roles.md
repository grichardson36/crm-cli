---
type: pattern
scale: object
status: decided (owner request 2026-10-29), built in feat/deal-roles (not merged, schema head 13)
---

# Deal roles are a list the user manages

**Problem.** A person's role on a deal is a sales role: champion, decision maker, economic buyer, influencer, blocker, end user, signer, billing contact. As free text it drifts (Champion, champion, Internal champion), cannot be filtered or reported, and nothing suggests the standard roles. A fixed list would be wrong the other way, because every team names its buying committee differently.

**Therefore.** The deal roles are a vocabulary the user owns: add, rename, reorder, remove, with the eight roles above seeded. The role of a person on a deal (`deal_person.role`) is a choice field backed by that vocabulary ([[field-types-drive-editors]]). It is edited in the cell of the people table on the deal page with the choice editor and [[pick-or-create]]: typing a name that is not listed adds the row `Create role X`. The CLI and MCP refuse an unlisted role with `E_VOCAB_UNKNOWN` and a fix, unless `--create` (`create`) is given ([[one-validation-framework]]). Set field on selected works for it like every choice field ([[bulk-actions-always]]).

**Deal role versus membership role.** These are two different facts.

- The **deal role** says what the person does in the buying decision of this deal. It comes from the list.
- The **membership role** says what the person does at a company (CTO, Advisor). It stays free text with suggestions ([[roles-are-free-text]]). Task and note links keep free text roles too.

A person can be the CTO of the customer (membership) and the economic buyer of a deal (deal role).

**Storage.** One table, `vocabulary` (migration 013): `kind`, `name` unique per kind ignoring case, `position`, `archived`. The kind is `deal_role`. The registry `model.Vocabularies` names the columns that hold a kind's values, so rename can rewrite them and removal can count them. Nothing in the repository, the validation or the editors is specific to deal roles: a field names its vocabulary (`FieldSpec.Vocabulary`) and gets the editor, the check and the CLI shape.

**Customizing.**

- Rename rewrites every deal link that uses the role.
- Remove with links still using it asks for a decision: `--replace-with OTHER` moves them, `--keep` leaves the text on the links as plain text. Nothing is lost.
- Reorder puts named roles first, or one role at a position.

**Existing free text is preserved.** Links written before the list hold any text. The page shows it as written, a cancelled edit keeps it, and a link keeps its role while its other attributes change. A one-time backfill (`crm deal roles backfill`, dry run with counts, `--apply`, one undo step in the repository) adds the distinct roles in use to the list, merging spellings that differ in case and skipping roles the user removed on purpose.

**Stages use the same table.** Deal stages are a second kind of the vocabulary, with an outcome flag for the closing meaning. See [[deal-stages]]. Note types, which have their own table, can move onto it the same way, a separate decision.

**Because.** Owner request 2026-10-29. Real data names roles ([[research-hubspot-export]] H4: champion, economic buyer, signer, billing contact, Buying Role empty everywhere), but a list the user cannot extend is ignored, and free text cannot be reported on. A list that is seeded and fully editable keeps both.

**Larger patterns.** [[field-types-drive-editors]], [[pick-or-create]], [[one-abstraction-many-contexts]], [[deal-stages]]

**Smaller patterns.** [[roles-are-free-text]], [[one-validation-framework]], [[bulk-actions-always]]

**Sources.** [[research-hubspot-export]] H4, [[schema-er]] (deal_person). `internal/model/vocabulary.go`, `internal/db/repo/vocabulary.go`, `internal/validate/vocab.go`, `internal/tui/vocabedit.go`

**Inspired by.** [[hubspot]], [[salesforce]] (opportunity contact roles are a managed picklist)
