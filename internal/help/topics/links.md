# Links between objects

People, organizations, deals and tasks link to each other. Every link is made, shown and removed from either side, with the same words in the CLI, MCP and terminal UI. Notes (interactions) attach to people, organizations and deals too.

## Relations

- person and organization: many each way, with role, dates and a primary flag
- person and person, organization and organization: typed (friend, parent, manager). See relations
- deal and person (a role per link), deal and organization (one primary)
- task and person, organization or deal, many each. Notes attach the same way
- tag and any of them

## CLI

`crm <object> link <other> <id> <other id>` and `crm <object> unlink ...`. The ids follow the object order in the command. Linking what is linked changes nothing. Unlinking soft deletes the link and keeps its role, dates, kind and notes: linking the same pair (and kind) again brings that row back with them unless you give new ones. Tags work the same way. `--new` is the only way to add a second row. Flags carry the attributes: `--role --from --to --primary --kind --notes`. `--new` adds a second link for a pair (a second stay). `unlink --id N` removes one of several.

```sh crm-example
crm person link org 2 1 --role CTO --from 2019-03
crm org link person 1 3
crm person link person 1 2 --kind parent
crm deal link org 1 1 --primary
crm deal link person 1 2 --role champion
crm task link org 1 1
crm person link tag 1 vip
crm person show 1 -f json
```

```sh crm-example-skip
crm person unlink org 2 1
crm person unlink org 2 1 --id 4
```

`show` on a person, org, deal or task lists its links with attributes. Deleting an object hides its links and restoring it brings them back.

## MCP and the terminal UI

MCP: `crm_<object>_link` and `_unlink` (same attributes), `crm_links`, `crm_link_roles`. In the terminal UI press `l` on an object, pick the relation, pick or create the other object, then answer the kind picker or the one-line prompt (role | from | to | primary | notes). `E` edits a link row, `u` unlinks, `ctrl+z` undoes each step.

See also: relations, orgs, tags, tasks, tui-keys, agents.
