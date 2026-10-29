# Nested tags

A tag named `work/clients/acme` is a child of `work/clients`, which is a child of `work`. The hierarchy is the name, there is nothing else to set up. A parent that no tag is named after still shows in the tree as a virtual parent.

## Use them

```sh crm-example
crm tag apply person 1 work/clients/acme
crm tag tree
crm tag tree -f json
```

In short syntax `#work/clients` is one tag. Names are checked: no empty level (`a//b`), no leading or trailing `/`, at most 8 levels and 200 characters. A name that breaks a rule exits 2 with `E_PATH_EMPTY_SEGMENT`, `E_PATH_EDGE_SLASH`, `E_PATH_TOO_DEEP` or `E_PATH_TOO_LONG`.

## Move and delete a branch

The tags below a tag always go with it.

```sh crm-example-skip
crm tag rename work job
crm tag nest home hobbies --under area/life
crm tag unnest area/life/home
crm tag merge work job --apply
crm tag delete work --yes
```

`rename` turns `work/clients` into `job/clients`. A name that exists is refused, `--merge` merges into it and children with the same names merge too. `nest` puts the tags under a parent (an empty `--under ""` is the top level, `--merge` merges a taken name, `--dry-run` previews). `unnest` moves them up one level. `merge` makes the children of the source children of the target. `delete` untags the whole branch, `--only-this` keeps the children. `crm tag unused` does not list a parent whose children are in use, and `crm tag duplicates` compares whole paths.

MCP: `crm_tag_tree`, `crm_tag_nest`, `crm_tag_unnest`, `crm_tag_delete`, `crm_tag_rename`, `crm_tag_merge`. In the terminal UI the Tags view has a tree layout: tags-module topic.

See also: tags, tags-module, validation, short-syntax
