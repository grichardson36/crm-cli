# Archive, restore and purge

Removing a person is reversible. Only `purge` is permanent.

## Archive and restore

An archived person disappears from lists, search and the due view. Their history is kept.

```sh crm-example
crm person archive 2
crm person archive 2 3
crm person list --archived
crm person restore 4
```

You can pass several IDs. `crm person delete` is a deprecated alias of `archive`. Organizations, deals, tasks and interactions are archived by their own `delete` or `rm` commands.

## Purge

Purge permanently deletes people who are already archived, with their contact details, relationships, tags, custom fields, and any interactions or tasks that involve only them. A tombstone is recorded so sync can propagate the delete. It cannot be undone, so it needs `--yes`.

```sh crm-example
crm person purge 4 --dry-run
crm person purge 4 --yes
```

Purge refuses people who are not archived.

## Safe order for an agent

1. `crm person archive ID --dry-run` shows who will be archived.
2. `crm person archive ID` archives.
3. Ask the owner before any purge, then run `--dry-run`, then `--yes`.

## Related topics

`duplicates` (merge archives the dropped people), `agents`, `tui` (the Archived view, key 6: `u` restores, `X` purges after you type the name).
