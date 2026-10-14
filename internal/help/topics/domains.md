# Domains: multiple databases

A domain is one database file. Keep personal and work contacts apart by giving each its own file. Its name is the file name without `.db`.

## Which database is active

crm picks the database in this order:

1. `--db PATH` on the command line
2. the `CRM_DB` environment variable
3. `~/.crm/crm.db` (on Windows `%USERPROFILE%\.crm\crm.db`)

```sh crm-example
crm where
crm where -f json
```

`crm where` shows the path, which of the three chose it, the domain name, schema version, device id, size and record counts. It opens the file read-only and never creates it.

## Use another domain

```sh crm-example-skip
crm --db ~/.crm/work.db person list
crm --db ~/.crm/work.db where
```

To keep a domain for a whole shell session, set the variable: `export CRM_DB=~/.crm/work.db` (bash, zsh) or `$env:CRM_DB = "$HOME\.crm\work.db"` (PowerShell). The variable stays in force for every command in that session, so check `crm where` when a result looks wrong.

## List domains

```sh crm-example
crm domains
crm domains -f json
crm domains --dir ~/.crm
```

`crm domains` scans one directory (default `~/.crm`) for `*.db` files that are CRM databases and shows each one's schema version and people count. The active one is marked.

## Create a domain

```sh crm-example
crm init --domain work --yes
```

This creates `~/.crm/work.db`. It refuses to overwrite an existing file. See `crm help install` for first-time setup.

## Keep the database out of synced folders

Do not put a database inside OneDrive, Dropbox or iCloud. Those clients copy the file and its `-wal` companion at different moments, which can corrupt SQLite. Keep the database local and write backups into the synced folder with `crm backup`. `crm doctor` warns when it finds this.

## Related topics

`install`, `agents`, `sync`.
