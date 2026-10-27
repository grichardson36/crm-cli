# Organizations and relationships

An organization is a company or group. A person can belong to several, with a role and dates. People and organizations can also be related to each other (see relations).

## Organizations

```sh crm-example
crm org add "Initech" --domain initech.com --industry Software --size 51-200 --city Austin --country US
crm org list
crm org show 1 --with-people
crm person edit 2 --org 1
```

`person edit --org ID` makes that organization the primary one. `crm person link org 2 1 --role CTO` or `crm org link person 1 2` add one with a role and dates, see links. `org show --with-people` lists the people linked to it. `crm org delete 1` is a soft delete and `crm org restore 1` brings it back.

## In the terminal UI

Organizations is tab 5 and Deals tab 6 by default. Hide a view you do not use with `crm modules off deals` or the palette entry "Modules" (see modules).

- The standard keys: `a` adds (`+` also), `enter` or `e` edits, `d` deletes (asks y/n), `u` restores, `L` links, `ctrl+z` undoes. "Restore organization" and "Restore deal" are in the palette.
- An organization is one line: `name | domain | industry | size | location | website | linkedin | description | notes` (the old `name | domain | industry | notes` still works). Location is `city, region, country`. Its page lists members, parent and child organizations, deals, tasks and notes. `enter` on a linked row opens it, `n` adds a note.
- A deal is `title | value | stage | organization | person | notes`. The organization (primary) and person are an id or a name. `s` picks the stage, a deal gets a close date each time it moves into won or lost.
- Editing opens the same line filled in. A field you empty is cleared. `space` marks rows and `M` merges them, see org-merge.

## Relationships between people

```sh crm-example
crm person link person 1 2 --kind colleague --notes "Same team"
crm person show 1
```

`relate` is the older name of `crm person link person --kind`. Kinds: see `crm relation-kind list`.

`crm context ID` includes the organizations and relationships in the meeting briefing.

`crm org add` refuses a name or domain that matches a live organization (`--allow-duplicate` overrides), `crm org merge` folds duplicates, see org-merge.

See also: people, links, relations, convert, org-merge, tags, duplicates.
