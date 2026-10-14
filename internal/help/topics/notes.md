# Notes and the interaction log

Everything that happened with a person is an interaction: a call, email, meeting, message, note, or a type you define. `crm log` records them.

## Log something

```sh crm-example
crm log call 1 --subject "Intro call" --content "Agreed on the Q4 timeline"
crm log email 1 --subject "Follow-up" --direction outbound
crm log meeting 1 2 --subject "Product demo" --at "2026-10-20 14:00"
crm log note 1 --subject "Quick check-in" --content "Seemed interested"
crm log message 1 --content "Sent the deck on WhatsApp"
```

List person IDs after the type to link several people. `--direction` is inbound or outbound. `--at` sets when it happened (default now).

## Read the log

```sh crm-example
crm log list
crm log list --person 1 --limit 5
crm log list --type call -f json
```

## Edit and remove

```sh crm-example
crm log edit 1 --content "Corrected text"
crm log edit 1 --date 2026-10-01
crm log rm 1
```

`log rm` archives the interaction: it leaves lists and cannot be edited, but stays in the database.

## Note types

Types are data, not a fixed list. Each type says whether logging it resets the keep-in-touch clock.

```sh crm-example
crm log types
crm log types add standup --counts-as-contact=false
crm log types rm note
crm log add --type call 1 "Talked about the offsite"
```

`log types rm` archives a type. Existing interactions keep it. `log add --type NAME PERSON_ID TEXT` logs any active type. Text after the person ID becomes the content.

## Related topics

`keep-in-touch`, `tags` (tag an interaction with `crm tag apply interaction ID NAME`), `tui` (`n` adds a note, `N` picks the type).
