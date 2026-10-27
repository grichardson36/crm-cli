# Merging organizations

The same company often exists twice (`BLUE ORIGIN` and `Blue Origin`). Creating an organization is refused when a live one has the same name or domain, then `org merge` folds the duplicates into one.

## Duplicate check

Names compare with case, punctuation, `&` and `and`, a leading "the" and endings like Inc, LLC and Ltd ignored, the same as the company text rule (see convert). Domains compare without `www.` and scheme. The error names the existing organization and its id. `crm org add Name --allow-duplicate` creates it anyway (MCP: `allow_duplicate`). In the terminal UI a match asks: use the existing one, create anyway, or cancel. Company text that normalizes to an existing name reuses that organization. Close names stay separate: `crm org dupes` lists them as merge candidates.

```sh crm-example
crm org add "Acme Corp" --allow-duplicate
```

## Merge

```sh crm-example-skip
crm org merge 1 2 3
crm org merge 1 2 3 --apply
```

`org merge SURVIVOR DUPLICATE...` previews, `--apply` writes. MCP: `crm_org_merge` with `dry_run`.

- The survivor keeps its own fields. An empty domain, industry, notes or summary takes the first value from the duplicates.
- Memberships all move. Only a row identical to one the survivor has (same person, role, start and end) is dropped, and its primary flag moves to the row that stays.
- Parent links move. A link that becomes a self link or a duplicate is dropped. A merge that would make an organization its own parent is refused.
- Deal links, tasks, notes and tags move.
- Duplicates are archived and their ids keep resolving to the survivor. Company text with the old name matches the survivor.

In the terminal UI open the Organizations view, mark rows with `space`, press `M`, pick the survivor and confirm. `ctrl+z` undoes the whole merge in one step. The merge is also in the merge log, see merge-log.

See also: merge-log, orgs, convert, duplicates, links.
