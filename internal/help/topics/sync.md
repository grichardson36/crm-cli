# Sync

The sync runner pulls records from external sources into the database. No adapters are bundled yet, so a fresh install has no sources. This topic describes the framework that adapters plug into.

## Commands

```sh crm-example
crm sync list
crm sync status
crm sync inbox list
crm sync inbox list --status all
```

`sync list` shows the registered sources. `sync status` shows each source's last run and the inbox size.

```sh crm-example-skip
crm sync run SOURCE --dry-run
crm sync run SOURCE
crm sync run SOURCE --allow-create
```

`SOURCE` is a name from `crm sync list`. Run with `--dry-run` first: it reports counts and writes nothing.

## How a run places a record

1. Match by external id: the record was seen before from this source. The person is refreshed in place: fields the source sets are updated, emails, phones and links are only added. An archived person is never brought back.
2. Otherwise match by exact email, phone number or social link. Only blank fields are filled, and the external id is stored for next time.
3. If nobody matches, the record goes to the inbox. With `--allow-create` it creates a new person instead.

Sync never deletes anything.

## The inbox

Records the runner could not place wait for a decision. Each has a reason: `unmatched`, `ambiguous` (several candidates), `no_name`, `already_linked`, `name_conflict`, or `no_person` (an interaction whose person is unknown).

```sh crm-example-skip
crm sync inbox list
crm sync inbox accept 1
crm sync inbox attach 1 2
crm sync inbox ignore 1
```

- `accept ID` creates a person from a person record.
- `attach ID PERSON_ID` fills that person's blank fields from a person record and links the external id, or logs an interaction record against that person.
- `ignore ID` dismisses it. Later runs leave it dismissed.

## External ids

Each synced record keeps the source name and the source's own id for it. That is how a second run recognizes the same record, and how Dex imports stay repeatable.

## Developers

A test source named `fake` registers only when the environment variable `CRM_SYNC_FAKE_FILE` points to a JSON file. Real adapters register in `internal/sync`.

## Related topics

`import`, `doctor` (`crm doctor` warns about a non-empty inbox), `agents`.
