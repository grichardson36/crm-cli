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

This removes the tag and every use of it. It cannot be undone, so it needs `--yes`. Preview first.

```sh crm-example
crm tag delete vip --dry-run
crm tag delete vip --yes
```

See also: people, duplicates (merging people combines their tags), tui-keys (`T` adds a tag, `-name` removes it, `/#tag` searches by tag).
