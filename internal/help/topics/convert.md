# Company text to organizations

`people.company` is free text. The review turns it into organizations in passes, with you choosing for every group. Nothing is created by looking and nothing is created without a decision.

## How groups form

People are grouped by the company text with case, spacing, punctuation and endings like Inc, LLC, Ltd, Corp and Co ignored (`ACME`, `Acme, Inc.` and `acme` are one group). The text itself is never rewritten and stays after conversion. Each group shows the existing organization it matches: exact (same normalized name), alias (the name of an organization that was merged away) or close (a suggestion only). People who already have a primary organization are left out unless their text differs. A group also shows the email domain most of its people share (free mail such as gmail is ignored, a group of 2 or more needs 2 people on the same domain). When an existing organization has that domain it is shown as a domain match, apart from the name match. A domain match is a suggestion only, `--accept-exact` never takes it, link it with `--link`. People tagged `org-review` (a phone ORG that matched nothing) are flagged.

```sh crm-example
crm org convert --stats
crm org convert --limit 20 --people
crm org convert --review
```

## Decisions

`--create KEY` or `KEY=Name` makes the organization (its domain is the group's email domain unless the decision file gives `"domain"`) and gives each person a membership: primary when they have none, their title as the role, no dates. `--link KEY=ORG_ID` links to an existing one. `--skip KEY` hides the group until its text changes, `--unskip KEY` brings it back. `--accept-exact` links every exact or alias match. `--decisions file.json` takes `[{"key":"acme","action":"create","name":"Acme"}]`. Without `--apply` they are only previewed.

```sh crm-example
crm org convert --accept-exact
crm org convert --create acme="Acme Corp" --skip globex
crm org convert --accept-exact --apply
```

A converted group is gone from the next pass and the `org-review` tag is removed, so a re-run shows only what is left.

## MCP and the terminal UI

MCP: `crm_org_convert_list` and `crm_org_convert_apply` (decisions, `accept_exact`, `dry_run`). In the terminal UI open "Company review view" from the palette, or press `C` in the Organizations view. `c` create, `l` link, `e` rename then create, `s` skip, `x` clear, `A` accept all exact matches, `enter` applies everything staged. Each apply is one undo step.

See also: orgs, relations, links, duplicates, dav.
