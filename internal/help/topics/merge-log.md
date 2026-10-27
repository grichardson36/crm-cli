# The merge log, undo and "not a duplicate"

Every applied merge of people (`crm person merge`) or organizations (`crm org merge --apply`) is written to a merge log that lasts across sessions. The log records the survivor, the merged-in records, what moved and every row the merge changed.

```sh crm-example
crm merge log
crm merge log --kind person --limit 5
```

## Undo and redo

`crm merge undo ID` reverses one merge, in any later session: the merged records come back, their links move back, the alias is removed and fields the merge overwrote get their old values. A value someone edited after the merge stays as it is and is counted as skipped. Undo is refused when the merged record was restored or merged again by hand. `crm merge redo ID` repeats an undone merge. The merge commands print the id, and MCP merge results carry a `log_id`.

MCP: `crm_merge_log`, `crm_merge_undo`, `crm_merge_redo`. In the terminal UI a merge is one `ctrl+z` step, and it goes through the same log.

## Not a duplicate

An organization pair that looks alike but is different can be dismissed. It stops showing in `crm org dupes`, and the choice is stored as data and can be reversed.

```sh crm-example-skip
crm org dupes dismiss 1 2
crm org dupes undismiss 1 2
crm org dupes --dismissed
```

MCP: `crm_org_dupes_dismiss` (`undo` to bring the pair back). Terminal UI palette: "Not a duplicate (organization)" in the Organizations view and "Show dismissed organization pairs". People pairs use `crm dupes dismiss` (see duplicates).

See also: duplicates, org-merge, archive.
