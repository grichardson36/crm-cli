# Sync

The sync runner pulls records from external sources into the database. No adapters are bundled yet, so a fresh install has no sources. This topic describes the framework that adapters plug into.

```sh crm-example
crm sync list
crm sync status
crm sync inbox list
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

## External ids

Each synced record keeps the source name and the source's own id for it. That is how a second run recognizes the same record, and how Dex imports stay repeatable.

## Developers

A test source named `fake` registers only when `CRM_SYNC_FAKE_FILE` points to a JSON file. Real adapters register in `internal/sync`.

Records the runner cannot place wait in the inbox for a decision. See `crm help sync-inbox`.

See also: sync-inbox, import, dav, agents.
