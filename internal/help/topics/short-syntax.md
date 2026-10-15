# Short syntax

Type meaning inline: tags, dates, people and more inside the text. The CLI, the TUI command bar and agents read it the same way.

## Tokens

A token starts a word and counts only when it parses, so `jane@example.com`, `+1 555 0100` and `#42` stay plain text. A backslash keeps a token as text: `\#42`.

| Token | Sets | Examples |
|---|---|---|
| `#tag` | tag, unknown tags are created | `#deal` |
| `@date` | due date or time | `@fri`, `@tomorrow 3pm`, `@next week`, `@+2w`, `@mar 4`, `@15:00` |
| `!date` | deadline | `!mon`, `!2026-12-31` |
| `~30m` | duration | `~30m`, `~1h30m` |
| `*3m` | keep-in-touch cadence | `*1w *2w *6w *1m *3m *6m *1y *never` |
| `+Name` | link a person | `+Sam`, `+Sam_Lee` |
| `^high` | priority | `^top ^high ^medium ^low` |

## Where it works

```sh crm-example
crm do "n Jane called about the deal #deal @fri" --dry-run
crm do "set Jane: company=Acme title=CTO"
crm task add "Call back @fri 3pm ~30m +Jane" --dry-run
crm log add --type call 1 "Talked about the offsite #trip @yesterday" --dry-run
```

`crm do` verbs: `n`/`note`, `t`/`task`, `log <type>`, `set`, `tag`, `snooze`. A name after the verb picks the person. `--person ID` names them instead. A name that matches nobody or several people is an error that lists the candidates.

In `n`, `#tag` tags the note, `+Name` links more people, `@date` creates a reminder and `*3m` sets the cadence. In `log`, `@date` is when it happened.

Flags win over tokens. `--dry-run` shows what was read. `--no-syntax` turns it off. Tasks cannot carry tags yet, so `#tag` stays in a task title.

## In the TUI

`ctrl+k` opens the command bar. Type `n Sam called #deal @fri` and a preview line shows what will happen. `tab` completes verbs, names, tags, dates and cadences. `enter` runs it. A `set` with two or more fields shows old and new values and waits for another `enter`. `ctrl+z` undoes and `ctrl+r` redoes (session only). With a person selected no name is needed. The note and task prompts (`n`, `t`) read the same tokens. In the People search, `#friend ^high *3m` filter the list. The palette action "Toggle short syntax" turns all of it off.

See also: tasks, notes, tags, tui, agents.
