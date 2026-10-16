# Organizations and relationships

An organization is a company or group. A person can belong to one. People can also be related to each other.

## Organizations

```sh crm-example
crm org add "Initech" --domain initech.com --industry Software
crm org list
crm org show 1 --with-people
crm person edit 2 --org 1
```

`person edit --org ID` links a person to an organization. So does `crm person link org 2 1` or `crm org link person 1 2`, see links. `org show --with-people` lists the people linked to it. `crm org delete 1` is a soft delete and `crm org restore 1` brings it back.

## In the terminal UI

Organizations and deals are advanced views, not tabs. Open them from the palette (`ctrl+k`): "Organizations view", "Deals view".

- The standard keys: `a` adds (`+` also), `enter` or `e` edits, `d` deletes (asks y/n), `u` restores, `L` links, `ctrl+z` undoes. "Restore organization" and "Restore deal" are in the palette.
- An organization is one line: `name | domain | industry | notes`.
- A deal is `title | value | stage | organization | person | notes`. The organization and person are an id or a name. `s` picks the stage.
- Editing opens the same line filled in. A field you empty is cleared.

## Relationships between people

```sh crm-example
crm person relate 1 2 --type colleague --notes "Same team"
crm person relationships 1
```

Relationship types: colleague, friend, manager, mentor, referred-by.

`crm context ID` includes the organization and relationships in the meeting briefing.

See also: people, links, tags, duplicates.
