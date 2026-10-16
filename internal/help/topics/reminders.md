# Reminders

crm never sends anything. It tells you which reminders are due, once, and you or an agent deliver them.

## Event reminders

Set them on an event with `--remind 15m` (repeat the flag, `1d:email` marks one as mail).

```sh crm-example
crm cal add "Standup" --start "fri 09:15" --duration 15m --remind 10m
crm cal due-reminders --dry-run
crm cal due-reminders --within 15m --dry-run -f json
```

A reminder is due when the event start minus the reminder has passed and the event has not started. `due-reminders` lists the due ones and marks them, so the next call lists only new ones. `--dry-run` looks without marking. A repeating event reminds once per instance, and a moved event reminds again.

## Booking messages

A link can carry `--remind 1d` and `--followup 2h` (`crm help book`). `crm book remind` drafts the messages that are due: a reminder before the meeting with how to cancel, and a follow-up after it. Each draft has `to`, `reply_to`, `subject` and `body`, and is drafted once.

```sh crm-example-skip
crm book remind --dry-run
crm book remind -f json
```

## Recipe for an agent

Run this every 5 minutes from cron or a scheduler:

1. `crm cal due-reminders -f json`. Tell the owner about each row. For `method` `email`, write to the `guests` addresses.
2. `crm book remind -f json`. Send each draft with a mail tool (the imap-smtp skill sends mail) to `to`, from the owner's address.
3. If a send fails, the item is already marked. Report it to the owner instead of retrying silently.

In the TUI the Home view shows "Reminders due". The palette has "Reminders: due now" and "Reminders: mark due as sent".

See also: calendar, book, agents, tasks.
