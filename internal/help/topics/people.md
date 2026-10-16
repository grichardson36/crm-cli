# People

Add, find, show and edit people. A person is identified by an integer ID.

## Add and list

```sh crm-example
crm person add "Ada Lovelace" --email ada@example.com --company "Analytical Engines" --title Founder
crm person add "Grace Hopper" --location Arlington --notes "Met at a conference" -f json
crm person list --limit 10
crm person list --tag vip
```

The name is split at the first space into first and last name. Use `--first-name` and `--last-name` with `person edit` to correct it.

## Show and find

```sh crm-example
crm person show 1
crm search jane
crm search acme --type person
crm context 1
```

`crm context` is the briefing before a meeting: profile, organization, recent interactions, open deals, pending tasks, relationships and tags.

## Edit

Only the flags you pass change. An empty value clears a field.

```sh crm-example
crm person edit 2 --title CTO --company "Acme Corp"
crm person edit 1 --favorite
crm person edit 1 --tier 1
crm person edit 2 --location ""
```

See also: contact-methods, orgs (organizations, relationships), keep-in-touch, tags, duplicates, archive.
