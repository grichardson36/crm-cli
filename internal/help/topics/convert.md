# Company text to organizations

`people.company` is free text and stays the source text. Wherever it is set, the text creates or reuses an organization and the person gets a membership. There is no review screen.

## The rule

The text is matched to an existing organization by its normalized name (case, spacing, punctuation, `&` and `and`, a leading "the" and endings like Inc, LLC, Ltd, Corp and Co ignored) or by the name of an organization that was merged away (merge alias). No match creates the organization. The person's primary membership then points to it (the title becomes the role of a new membership).

Close but different names never merge on their own: a typo, `Acme Holdings` next to `Acme`, or a spelling beyond the normalization makes its own organization. They show up as merge candidates in `crm org dupes` (MCP `crm_org_dupes`, and "similar name" on the organization page in the terminal UI), and you merge them with `crm org merge`.

It applies at every door that sets a company: `crm person add` and `crm person edit --company` (not when `--org` is given), `crm import dex`, MCP `crm_person_create` and `crm_person_update`, the command bar and `crm do` (`set X company=Acme`), device sync, the terminal UI person form, and an ORG edited on a phone over CardDAV (see relations: the primary membership is repointed or created). In the terminal UI the creation and the edit undo together as one step.

## Junk strings

Strings such as `N/A`, `none`, `self-employed`, `retired`, `unemployed` and `-` never create an organization. The list is data: built-in defaults plus your additions, editable.

```sh crm-example
crm org ignore list
crm org ignore add "stay at home parent"
crm org ignore remove retired
```

MCP: `crm_org_ignore_list`, `crm_org_ignore_add`, `crm_org_ignore_remove`.

## Backfill existing people

`crm org backfill` is a dry run. It prints how many people have company text, how many distinct groups, how many organizations it would create, how many exist and are reused, how many strings are ignored and how many people would get a membership. Groups of 3 or more people are listed first. `--apply` runs it as one batch (one undo step in the terminal UI, existing memberships stay, primary only when the person has none) and is safe to run again.

```sh crm-example
crm org backfill
crm org backfill --limit 20 -f json
crm org backfill --apply
crm org dupes
```

MCP: `crm_org_backfill` (dry run unless `apply` is true). `crm org convert` and `crm_org_convert_list` / `crm_org_convert_apply` are deprecated aliases of the backfill.

See also: orgs, relations, links, duplicates, org-merge, dav.
