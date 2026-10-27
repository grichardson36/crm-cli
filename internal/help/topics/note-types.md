# Note types

Every logged interaction has a type: call, email, meeting, message, note, or one you define. Types are data, not a fixed list. Each type says whether logging it resets the keep-in-touch clock.

```sh crm-example
crm log types
crm log types add standup --counts-as-contact=false
crm log types rm note
crm log add --type call 1 "Talked about the offsite"
```

- `log types` lists the active types.
- `log types add NAME` defines a type. `--counts-as-contact=false` keeps it from resetting the clock.
- `log types rm NAME` archives a type. Existing interactions keep it.
- `log add --type NAME PERSON_ID TEXT` logs any active type. Text after the person ID becomes the content.

Logging a call, email, meeting or message counts as contact. Notes count only for types defined with `--counts-as-contact`.

In the terminal UI, `a n` adds a note of the last used type (press `tab` in the prompt to change the type) and "Add note (pick type)" in the palette picks the type first. The palette action "Manage note types" adds and archives types.

Tag an interaction with `crm tag apply interaction ID NAME`.

See also: notes, keep-in-touch, tags, tui-keys.
