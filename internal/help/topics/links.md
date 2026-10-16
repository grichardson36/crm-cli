# Links between objects

People, organizations, deals and tasks link to each other. Every link is made, shown and removed from either side, with the same words in the CLI, MCP and terminal UI.

## Relations

- person and organization (a person has one organization)
- person and deal, deal and organization (a deal has one person and one organization)
- task and person (many people per task), task and deal
- tag and any of them

## CLI

`crm <object> link <other> <id> <other id>` and `crm <object> unlink ...`. The order of the two ids follows the object order in the command. Linking what is linked changes nothing. A single-valued side moves to the new object.

```sh crm-example
crm person link org 2 1
crm org link person 1 3
crm deal link org 1 1
crm task link person 1 2
crm task link deal 1 1
crm person link tag 1 vip
```

```sh crm-example-skip
crm person unlink org 2 1
crm task unlink person 1 2
```

`crm org restore 1` and `crm deal restore 1` undo a delete, like `crm task restore`.

## MCP and the terminal UI

MCP tools are `crm_<object>_link` and `crm_<object>_unlink` with `id`, `other` and `other_id`, plus `crm_org_restore`, `crm_deal_restore` and `crm_task_restore`.

In the terminal UI press `L` on a person, task, organization or deal (Organizations and Deals views are in the palette), pick the relation, type to filter and press enter. The last row, Create, makes the object from your text and links it in one step. `U` unlinks. `ctrl+z` undoes a link, and a create plus link is one step. On a person, the organization row opens the organization on `enter`.

See also: orgs, tags, tasks, tui-keys, agents.
