# Organizations and relationships

An organization is a company or group. A person can belong to one. People can also be related to each other.

## Organizations

```sh crm-example
crm org add "Initech" --domain initech.com --industry Software
crm org list
crm org show 1 --with-people
crm person edit 2 --org 1
```

`person edit --org ID` links a person to an organization. `org show --with-people` lists the people linked to it.

## In the terminal UI

Organizations and deals are advanced views, not tabs. Open them from the palette (`ctrl+k`): "Organizations view", "Deals view".

- `+` adds, `enter` or `e` edits, `d` deletes (asks y/n), `ctrl+z` undoes. "Restore deleted organization" and "Restore deleted deal" are in the palette.
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

See also: people, tags, duplicates.
