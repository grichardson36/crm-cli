# Deal stages

A deal is in one stage of a pipeline. The stages are a list you manage. A new database starts with lead, prospect, proposal, negotiation, won and lost. Each stage has an outcome: `open` (the deal is in the pipeline), `won` or `lost` (the deal is closed). The outcome decides what closes a deal, the name does not.

```sh crm-example
crm deal stages list
crm deal stages add qualified
crm deal stages outcome negotiation lost
crm deal stages rename proposal "quote sent"
crm deal stages reorder lead proposal
crm deal stages remove prospect --replace-with lead
```

- `list` shows pipeline order, outcome and how many deals are in each stage. `--all` adds removed stages.
- `add NAME` puts an open stage at the end. `outcome NAME open|won|lost` changes what it means.
- `rename OLD NEW` rewrites every deal in the stage.
- `reorder STAGE...` puts the named stages first, the rest follow. `--position N` moves one stage. `crm deal pipeline` and the status summary follow this order.
- `remove STAGE` with deals in it needs `--replace-with OTHER`, they move there. There is no `--keep`: a deal always holds a listed stage. Otherwise `E_VOCAB_IN_USE`.

Rules, checked on remove and outcome: at least one stage stays open (`E_STAGE_NO_OPEN`), and at least one is won (`E_STAGE_NO_WON`) and one lost (`E_STAGE_NO_LOST`).

The stage of a deal must be on the list, any case, the listed spelling is stored. `crm deal add` without `--stage` uses the first open stage. An unlisted stage fails with `E_VOCAB_UNKNOWN` and a fix naming `crm deal stages add`. `--stage` completes from the list in shell completion.

Closed date: moving a deal into a won or lost stage stamps `closed_at` with now, again on every such move (won to lost too). Moving it into an open stage clears it. Open deal counts, the pipeline and the won and lost meaning in reports follow the outcome. A stage you rename to signed still closes if it is won. Making a stage won or lost stamps the deals already in it that have no close date. Removing a stage with `--replace-with` keeps the close date when both stages have the same outcome.

Terminal UI: the stage row and cell of a deal use the choice editor (type to filter, a new name offers `Create stage X`, an open stage). Set field on selected offers `stage`. The palette entry Manage deal stages adds, renames, sets the outcome, moves and removes stages, undoable except remove. MCP: `crm_deal_stage_list`, `_add`, `_rename`, `_remove` (`replace_with`), `_reorder`, `_outcome`.

See also: deal-roles, validation, links, tui-keys, errors.
