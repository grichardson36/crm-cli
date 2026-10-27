# Tags

Free-form labels. A tag can go on a person, organization, deal or interaction.

## Apply, show, remove

The entity type is one of: person, organization, deal, interaction.

```sh crm-example
crm tag apply person 1 vip
crm tag apply person 2 reactconf-2026
crm tag show person 1
crm tag remove person 1 vip
crm tag list
```

Tags are created the first time they are applied. Applying a tag twice does nothing.

## Find by tag

```sh crm-example
crm person list --tag vip
crm person list --tag vip -q
```

`-q` prints only IDs, which is handy for scripts.

## Delete a tag everywhere

A tag is a name on records, so deleting it untags every record that carries it, as one step, and the tag drops out of the tag list. Each tagging is soft deleted: tagging a record with the name again brings its old tagging back, and in the terminal UI `ctrl+z` restores all of them at once. It still needs `--yes`. Preview first.

```sh crm-example
crm tag delete vip --dry-run
crm tag delete vip --yes
```

Rename with `crm tag rename old new`. A name another tag already has, in any case, is refused and the error says to merge. `crm tag merge source target` previews and `--apply` moves every record of `source` to `target` (a record with both counts once) and removes `source`. In the terminal UI find the tag in the bar, `tab`, then `r` renames and `m` merges, one undo step each. MCP: `crm_tag_rename`, `crm_tag_merge`.

See also: people, duplicates (merging people combines their tags), tui-keys (`T` adds a tag, `-name` removes it, `/#tag` searches by tag).
