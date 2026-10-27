# Purge any archived type

Purge is a verb of every object type that can be archived: people, organizations, deals, tasks and notes. It permanently deletes the object and its dependent rows (links, tags, memberships) and records a tombstone for each deleted synced row, so sync can propagate it. It cannot be undone, and it only takes archived objects.

## Command line

`crm purge --archived` is a dry run that lists what would go. Pass `--apply` to delete. `--type` limits it to one type (person, organization, deal, task, note). `--ids` needs a type and defaults to person.

```sh crm-example
crm purge --archived
crm purge --archived --type organization --ids 3,4 --apply
```

`crm person purge ID --yes` still purges one person.

## MCP

`crm_purge_archived` takes `type`, `ids` and `apply` (default false, a dry run).

## Terminal UI

In the Archived view `d` purges the highlighted object after you type its name, or the selected objects after you type the count back (`purge 5`). The palette has Purge selected forever and Purge all remaining forever, which takes every archived object in the list (all types, or the type set with `ctrl+o`).

See also: archive, agents, tui-keys.
