# Tags module

The Tags view lists every tag with how many objects carry it. Tag names are plain text, so `parent/child` shows as written.

## Clean up from the command line

`crm tag unused` lists the tags no object carries. `crm tag duplicates` lists tags that differ only by case, spaces, hyphens or underscores, with the most used spelling to merge into. Both only preview. `--apply` deletes the unused names or merges every group into its most used spelling.

```sh crm-example
crm tag unused
crm tag duplicates -f json
```

MCP: `crm_tag_unused`, `crm_tag_duplicates` (`apply`).

## In the terminal UI

The module is after Deals (key `7` with every module on, or the palette entry Tags view). It has no tag-specific key, the universal verbs act on the selected tags.

- `enter` opens the tag page: the objects that carry the tag, grouped by kind. `enter` on one opens it, `esc` goes back.
- `tab`, then `enter` or `e` on the name renames it everywhere. A name another tag has asks once to merge into it.
- `d` deletes, `m` merges the selected tags into the highlighted one, `l` gives the tag to an object, `u` takes it off. Several at once ask for the count typed back, and one `ctrl+z` undoes the whole step.
- `ctrl+o` sorts (name, number of objects) and filters (tags in use, unused, possible case duplicates). `/` finds a tag.
- Palette: Tags: merge case duplicates, Tags: delete unused tags.

See also: tags, modules, tui-selection
