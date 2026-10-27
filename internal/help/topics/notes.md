# Notes and the interaction log

Everything that happened with a person is an interaction: a call, email, meeting, message, note, or a type you define. A note is an object with the same verbs as every other: add, edit, delete, restore, link and unlink. `crm note` is the plain name and `crm log` keeps working.

## Add, edit, link, delete

```sh crm-example
crm note add "Talked about the offsite" --person 1 --org 1
crm note edit 1 --content "Corrected text"
crm note link deal 1 1
crm note delete 1
```

`crm note restore <id>` brings a deleted note back and `crm note unlink` removes a link. A note can exist before it is attached: `crm note link person|org|deal <note id> <id>` attaches it, and `crm person|org|deal link note <id> <note id>` does the same from the other side. `--direction` is inbound or outbound and `--at` sets when it happened (default now). MCP has `crm_note_create`, `crm_note_update`, `crm_note_delete`, `crm_note_restore`, `crm_note_link` and `crm_note_unlink`.

## Log and read

```sh crm-example
crm log call 1 --subject "Intro call" --content "Agreed on the Q4 timeline"
crm log meeting 1 2 --subject "Product demo" --at "2026-10-20 14:00"
crm log list --person 1 --limit 5
crm note list --type call -f json
```

`crm log <type> <person id>...` records one for several people. `crm log edit` and `crm log rm` are the older names of `note edit` and `note delete`. A deleted note leaves lists and stays in the database.

Types are data, not a fixed list, and each says whether logging it resets the keep-in-touch clock. See `crm help note-types`.

See also: short-syntax, note-types, keep-in-touch, tags, tui-keys.
