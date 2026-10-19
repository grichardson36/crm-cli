# Organizations and relationships

An organization is a company or group. A person can belong to several, with a role and dates. People and organizations can also be related to each other (see relations).

## Organizations

```sh crm-example
crm org add "Initech" --domain initech.com --industry Software
crm org list
crm org show 1 --with-people
crm person edit 2 --org 1
```

`person edit --org ID` makes that organization the primary one. `crm person link org 2 1 --role CTO` or `crm org link person 1 2` add one with a role and dates, see links. `org show --with-people` lists the people linked to it. `crm org delete 1` is a soft delete and `crm org restore 1` brings it back.

## In the terminal UI

Organizations and deals are advanced views, not tabs. Open them from the palette (`ctrl+k`): "Organizations view", "Deals view".

- The standard keys: `a` adds (`+` also), `enter` or `e` edits, `d` deletes (asks y/n), `u` restores, `L` links, `ctrl+z` undoes. "Restore organization" and "Restore deal" are in the palette.
- An organization is one line: `name | domain | industry | notes`. Its page lists members, parent and child organizations, deals, tasks and notes. `enter` on a linked row opens it, `n` adds a note.
- A deal is `title | value | stage | organization | person | notes`. The organization (primary) and person are an id or a name. `s` picks the stage, a deal gets a close date each time it moves into won or lost.
- Editing opens the same line filled in. A field you empty is cleared.

## Relationships between people

```sh crm-example
crm person relate 1 2 --type colleague --notes "Same team"
crm person relationships 1
```

`relate` is the older name of `crm person link person --kind`. Kinds: see `crm relation-kind list`.

`crm context ID` includes the organizations and relationships in the meeting briefing.

See also: people, links, relations, tags, duplicates.
