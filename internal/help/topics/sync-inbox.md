# The sync inbox

Records the sync runner could not place wait here for a decision. Nothing in the inbox changes a person until you accept or attach it.

```sh crm-example
crm sync inbox list
crm sync inbox list --status all
```

```sh crm-example-skip
crm sync inbox accept 1
crm sync inbox attach 1 2
crm sync inbox ignore 1
```

- `accept ID` creates a person from a person record.
- `attach ID PERSON_ID` fills that person's blank fields from a person record and links the external id, or logs an interaction record against that person.
- `ignore ID` dismisses it. Later runs leave it dismissed.

## Reasons

Each inbox row has a reason:

- `unmatched`: nobody matched. Accept it or attach it.
- `ambiguous`: several candidates matched.
- `no_name`: the record has no name.
- `already_linked`: the person is already linked to another record of this source.
- `name_conflict`: creating the person clashed with an existing one.
- `no_person`: an interaction whose person is unknown.

`crm doctor` warns about a non-empty inbox. The terminal UI Sync view (hidden by default: Settings, "Open Sync", or `v`) has the same actions: `a` accept, `p` attach, `i` ignore.

See also: sync, import, tui-keys.
