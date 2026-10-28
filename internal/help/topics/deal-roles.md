# Deal roles

A person's role on a deal is a sales role: champion, decision maker, economic buyer, influencer, blocker, end user, signer, billing contact. The role at a company (a membership: CTO, Advisor) is a different thing and stays free text. Deal roles are a list you manage. A new database starts with the eight above.

```sh crm-example
crm deal roles list
crm deal roles add "technical evaluator"
crm deal roles rename "decision maker" "final decision maker"
crm deal roles reorder champion "economic buyer" signer
crm deal roles reorder signer --position 2
crm deal roles remove blocker
```

- `list` shows pick order and how many deal links use each role. `--all` adds removed roles.
- `rename OLD NEW` rewrites every deal link that uses the role.
- `reorder ROLE...` puts the named roles first, the rest follow. `--position N` moves one role.
- `remove ROLE` with links still using it needs `--replace-with OTHER` (they move) or `--keep` (the text stays on the link as plain text). Otherwise `E_VOCAB_IN_USE`.

The role of a link must be on the list. The match ignores case and the list spelling is stored. An unlisted role fails with `E_VOCAB_UNKNOWN` and a fix naming `crm deal roles add`. `--create` (`create` in MCP) adds it while saving the link: `crm deal link person 1 2 --role champion`, `crm person link deal 2 1 --role "technical evaluator" --create`.


## Roles you already have

Free-text roles on deal links from before the list stay as they are and show as written. `backfill` lists the distinct ones so they appear in the picker. It reports only unless `--apply`, never changes a link, merges spellings that differ in case, skips roles you removed on purpose and texts that cannot be names (over 80 characters, or with `|`). Running it again adds nothing.

```sh crm-example
crm deal roles backfill
crm deal roles backfill --apply
```

## Terminal UI and MCP

On a deal page the `deal role` cell opens the choice editor: type to filter, up and down pick, `enter` saves. A name not on the list adds the row `Create role X`, and `enter` adds it to the list and sets it as one undo step. Set field on selected (palette) offers `deal role` for selected people and asks for the deal.

MCP: `crm_deal_role_list`, `_add`, `_rename`, `_remove` (`replace_with`, `keep`), `_reorder`, `_backfill` (`apply`), and `create` on the link tools. Deal stages are fixed today. This list is a general vocabulary table that can hold customizable stages later.

See also: links, relations, validation, tui-keys, errors.
