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

## Relationships between people

```sh crm-example
crm person relate 1 2 --type colleague --notes "Same team"
crm person relationships 1
```

Relationship types: colleague, friend, manager, mentor, referred-by.

`crm context ID` includes the organization and relationships in the meeting briefing.

See also: people, tags, duplicates.
