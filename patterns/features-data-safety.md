---
type: feature-area
---
# Data safety: backup, doctor, migrations

Backup, install health, dry runs, schema migrations and where the data lives.

Moved from FEATURES.md on 2026-10-29. Every row keeps its ID and text. Update a row's State and Now in the same session you change the feature ([[keep-the-tracker-accurate]]). Column meanings and the row lifecycle are in [[feature-rules]].

**Follows patterns:** [[never-test-on-real-data]], [[soft-delete-restorable]], [[sync-safe-data]], [[agents-never-get-prompts]], [[delete-asks-yes-no]]

**See also:** [[index]]

## Rows

| ID | Feature (evidence) | From | State | Now | Open questions | Action plan | Better by | P |
|---|---|---|---|---|---|---|---|---|
| G10 | Backup and snapshot publish from the TUI | Us | TODO | `crm backup`, `deploy/publish-snapshot.sh` | Allow publishing from the TUI? | Palette action "Backup now" only. Publishing stays a script | safe | P3 |
| I06 | Safe-by-default for agents: `--dry-run` on every mutating command, `--yes` for destructive ones, stable exit codes, JSON errors on stderr with `error.code` | Us | DONE | Generic `--dry-run` on mutating commands, `--yes` on destructive ones, JSON errors on stderr with code and exit code. | - | Audit all mutating commands; add `--dry-run`; document in I01 `agents` topic | safe | P1 |
| I07 | Data discoverability: `crm where` prints the active database path, domain name, schema version, device id, and other known domains so an agent never guesses | Us | DONE | `crm where`, `crm domains`. | Naming domains: file name or config? | `crm where -f json`, `crm domains list` (scans `~/.crm/*.db`) | simple | P1 |
| I11 | `crm init` guided setup: creates a domain database (name, path), optional import (Dex CSV, vCard), sets cadence defaults, prints next steps. Non-interactive flags and `-f json` so agents can drive it; interactive prompts when run by a person | Us | DONE | `crm init [--domain] [--path] [--import-dex-contacts] --yes -f json`. | Default location `~/.crm/<domain>.db`? | `crm init [--domain personal] [--path ...] [--import-dex FILE] --yes`; refuses to overwrite | fast, simple | P1 |
| I12 | `crm doctor`: checks install health (binary version, data dirs, DB integrity, schema version, backups, DAV server, sync sources) and prints fixes. JSON for agents | Us | DONE | `crm doctor -f json`: integrity, schema, backups, cloud-folder warning, sync, with fixes. | - | Each check returns `ok | warn | fail` with a one-line fix; agents use it to verify setup after I10 | safe | P1 |

## Schema migrations named in the rows

Collected from the rows of this vault. The row is the source for each fact.

| Migration | Schema head | What it did | Row |
|---|---|---|---|
| 004 | | Never-contacted people first come due at the cadence-set date plus cadence | D02 in [[features-keep-in-touch-and-birthdays]] |
| 006 | | CardDAV server (`crm dav`) | H06 in [[features-sync-and-dav]] |
| 007 | | Task tags, after migrations 007 to 014 of the calendar were removed (section J intro in [[features-calendar-archived]]). The M03 row cites it as migration 015 | M03 in [[features-tasks-and-notes]] |
| 008 | 8 | Moves people.org_id, deals.person/org, tasks.person/deal, task_people, interaction_people, relationships and Dex related_contacts into links | M11, A15 in [[features-links-and-relations]] |
| 009 | 9 | Native organization fields | M15 in [[features-organizations]] |
| 010 | 10 | `company_ignore` | M12 in [[features-organizations]] |
| 011 | 11 | `taggings` rebuilt with uuid and soft delete, `merge_log`, `org_dupe_dismissals` | M19 in [[features-merge-archive-and-undo]] |
| 012 | 12 | `name_aliases` | M23 in [[features-validation-and-field-types]] |
| 013 | 13 | `vocabulary` (deal roles) | M27 in [[features-deals]] |
| 014 | 14 | `vocabulary.outcome`, deal stages seeded, `deals` rebuilt without the stage CHECK | M28 in [[features-deals]] |
