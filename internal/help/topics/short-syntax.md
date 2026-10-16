# Short syntax

Type meaning inline. A symbol marks a named thing, plain English marks time. The CLI, the TUI command bar and agents read it the same way.

## Tokens

A token counts only when it parses, so `jane@example.com`, `#42` and `every time` stay text. A backslash (`\#42`) or "double quotes" keep text as typed.

| You type | Sets |
|---|---|
| `#deal` | tag, unknown tags are created |
| `@Sam`, `@Sam_Lee` | link a person (several matches or none: you choose) |
| `fri 3pm`, `tomorrow`, `next week`, `in 2 weeks`, `mar 4` | due date or time (tasks, logs, events) |
| `by fri` | deadline |
| `remind fri` | reminder date in a note, where bare dates are not read |
| `~30m`, `for 2 hours` | duration |
| `!high`, `!!!` (`!!` is medium) | priority |
| `every 3m`, `every 2 weeks`, `every fri` | repeat. A cadence crm knows (1w 2w 6w 1m 3m 6m 1y) also sets keep in touch |
| `at Cafe Rio` | place (events) |

A time with no date means today, or tomorrow when it has passed. `sat` or `mon` alone is a day only with a helper (`on sat`, `sat 3pm`).

## Where it works

```sh crm-example
crm do "n Jane called about the deal #deal remind fri" --dry-run
crm do "t Jane send the contract fri 3pm by mon !high"
crm do "set Jane: company=Acme title=CTO"
crm task add "Call back fri 3pm for 30m @Jane" --dry-run
crm log add --type call 1 "Talked about the offsite #trip yesterday" --dry-run
```

`crm do` verbs: `n`, `t`, `log <type>`, `set`, `tag`, `snooze`. A name after the verb picks the person, or use `--person ID`. Flags win over tokens. `--dry-run` shows what was read, `--no-syntax` turns it off.

A name that matches nobody or several people is never guessed. The TUI lists them all on one screen before saving: `1`-`9` pick a person, `c` creates a contact, `x` keeps the text. Nothing is created by default. `crm do` exits 2 or 3 with the candidates in JSON, or take `--keep-unresolved` / `--link-new`.

In the TUI, `/` or `ctrl+k` then `n Sam called #deal remind fri` previews the note. `tab` completes, `enter` runs, `alt+d` turns the chip at the cursor back into text, `ctrl+z` undoes.

See also: tasks, notes, tags, tui, keys, agents.
