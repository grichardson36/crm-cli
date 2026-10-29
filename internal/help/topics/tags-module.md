# Tags module

The Tags view lists every tag with how many objects carry it. A tag named `parent/child` is a nested tag (nested-tags topic), shown in the tree layout.

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
- Palette: Tags: merge case duplicates, Tags: delete unused tags, Tags: nest under..., Tags: unnest.
- `ctrl+o` Layout switches to a tree, remembered on this device. `right` opens a node or goes to its first child, `left` closes it or goes to the parent, `ctrl+up` and `ctrl+down` jump between siblings. Each node shows its objects and, in brackets, those with its children.
- In the tree `d` on a parent deletes the whole branch after you type the count, `m` takes the children along, a rename renames them. Tag page in `ctrl+o` includes the children's objects.

See also: tags, nested-tags, modules, tui-selection
