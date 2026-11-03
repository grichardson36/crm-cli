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

The Notes row of Settings shows the list in its right pane (`enter` enters it, `esc` leaves it): `a` adds, `e` renames (notes follow), `tab` toggles mark contacted, `shift+up`/`shift+down` order, `d` removes (notes move to another type or keep the type, one undo step), `shift+r` resets to the built-in types. In the terminal UI, `a n` adds a note of the last used type (press `tab` in the prompt to change the type) and "Add note (pick type)" in the palette picks the type first.

Tag an interaction with `crm tag apply interaction ID NAME`.

See also: notes, keep-in-touch, tags, tui-keys.
