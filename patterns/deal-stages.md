---
type: pattern
scale: object
status: decided (owner decision 2026-10-29), built in feat/deal-stages (not merged, schema head 14)
---

# Deal stages are a list the user manages, each with an outcome

**Problem.** The stages of a deal were hardcoded (lead, prospect, proposal, negotiation, won, lost). Every team names its pipeline differently, and the code treated the two names `won` and `lost` as the meaning of a closed deal. Renaming a stage, or adding "closed won" and "no budget", would have broken the close date and every report.

**Therefore.** The deal stages are a second kind on the same vocabulary as the deal roles ([[deal-roles]], [[one-abstraction-many-contexts]]): add, rename, reorder, remove, with the six defaults seeded in order. A stage carries an **outcome**: `open`, `won` or `lost`. The outcome is what the program reads. The name is only a label.

- `closed_at` follows the outcome of the stage. A move into a won or lost stage stamps now, again on every such move (won to lost too, per the earlier owner decision), and a move into an open stage clears it. A stage renamed `signed` still closes a deal if it is won. A stage named `won` that is marked open does not.
- Open deal counts, the pipeline and the won and lost reports group by outcome, never by name. The pipeline lists stages in the order of the list.
- The stage of a deal is a choice field backed by the vocabulary (`FieldSpec.Vocabulary`), so the record page, the cell of a linked deal, Set field on selected, the pipe line and the CLI and MCP checks all read one list ([[field-types-drive-editors]], [[validate-and-correct-at-every-boundary]]). An unlisted stage is refused with `E_VOCAB_UNKNOWN` and a fix. The editor offers `Create stage X`, which adds an open stage.

**Rules, enforced in one place** (`validate.StageRules`, codes in `internal/validate/codes.go`).

- At least one open stage stays (`E_STAGE_NO_OPEN`).
- At least one won stage (`E_STAGE_NO_WON`) and one lost stage (`E_STAGE_NO_LOST`) exist.
- They are checked on every change that could break them: removing a stage and setting an outcome.

**Difference from deal roles.** A role on a link may keep free text as plain text, a deal must always hold a listed stage. So removing a stage that deals use needs `--replace-with`, there is no keep, and the editor has no clear row (`VocabSpec.Strict`). Rename rewrites the deals, as it does for roles.

**Storage.** `vocabulary` gains the column `outcome` (migration 014, `open` by default and only meaningful for kind `deal_stage`). The same migration rebuilds `deals` without the old CHECK that fixed the six names. Existing deals keep their stage text, which matches a seeded name.

**Changing an outcome or removing a stage that deals use.**

- Making a stage won or lost stamps the deals already in it that have no close date. Making it open leaves their dates alone.
- Removing a stage with a replacement keeps the close date when both stages have the same outcome, stamps it when an open deal moves into a closed stage or won becomes lost, and clears it when a deal moves into an open stage.

**Doors.** `crm deal stages list|add|rename|remove|reorder|outcome`, MCP `crm_deal_stage_*`, help topic `deal-stages`, palette action Manage deal stages (no key). The stage flags of `crm deal add|edit|list` name the list command and complete from it.

**Because.** Owner decision 2026-10-29. One vocabulary mechanism serves roles and stages, and an outcome flag, not a name, carries the closing meaning, so renaming never changes what a deal is.

**Larger patterns.** [[deal-roles]], [[field-types-drive-editors]], [[one-abstraction-many-contexts]]

**Smaller patterns.** [[validate-and-correct-at-every-boundary]], [[pick-or-create]], [[one-validation-framework]], [[bulk-actions-always]]

**Sources.** `internal/model/vocabulary.go`, `internal/db/repo/vocabulary.go`, `internal/db/repo/deal.go`, `internal/validate/vocab.go`, `internal/tui/vocabstage.go`

**Inspired by.** [[hubspot]] (pipeline stages with a closed won and closed lost probability), [[salesforce]] (opportunity stages with an IsClosed and IsWon flag)
