# Import

Bring contacts in from other tools. Dex CSV is the supported format today.

## Import a Dex export

In Dex, export from Settings, Import/Export. You get a contacts CSV and optionally a notes CSV. Always preview first: the dry run counts what would be imported and writes nothing.

```sh crm-example-skip
crm import dex --contacts dex_contacts.csv --dry-run
crm import dex --contacts dex_contacts.csv --notes dex_notes.csv
crm import dex --contacts dex_contacts.csv --skip-tag "Old clients"
```

The report is JSON on stdout: contacts imported and skipped, emails, phones, links, tags applied, custom fields, notes imported and skipped, and note links. Notes attach to imported contacts, so import both files in one run.

## Safe to repeat

Each Dex record is remembered by its Dex id in the external ids table. Running the same import again skips records already imported. Extra columns Dex exports (education, business card URL, and similar) become custom fields.

## Import while creating a database

```sh crm-example-skip
crm init --import-dex-contacts dex_contacts.csv --import-dex-notes dex_notes.csv --yes
```

## After importing

```sh crm-example
crm status
crm dupes
```

Imports often create duplicates. Review them with `crm help duplicates`.

## Other sources

Pulling from live services (contacts, calendars, mail) goes through the sync runner. See `crm help sync`.
