---
type: feature-area
---
# Calendar and booking links (archived)

Everything about calendar, scheduling and booking links. Archived 2026-10-16, code on branch `archive/calendar` and tag `archive-calendar-2026-10-16`.

Moved from FEATURES.md on 2026-10-29. Every row keeps its ID and text. Update a row's State and Now in the same session you change the feature ([[keep-the-tracker-accurate]]). Column meanings and the row lifecycle are in [[feature-rules]].

**Follows patterns:** [[at-place-means-location]], [[idempotent-by-uid]], [[import-is-a-snapshot-sync-is-a-relationship]]

**Research:** [[caldav-and-icalendar-standards]], [[motion]], [[calendly]], [[outlook]], [[thunderbird]], [[fantastical]]

**See also:** [[index]]

Owner decision 2026-10-16: all of section J is archived until the core objects are right. The code is on branch `archive/calendar` and tag `archive-calendar-2026-10-16` (fork origin), removed from main 2026-10-16. Migrations 007 to 014 were removed and task tags became migration 007. To bring any of it back, start from the tag and rebase onto the new schema.

Inspiration: Motion, Outlook, Thunderbird (calendar view, tasks on the calendar, smart scheduling), Calendly (booking links). Builds on E05 (calendar sync adapter), H06 (CardDAV server, same CalDAV/WebDAV stack) and the tasks that already exist. Contacts stay the center: every event and booking links to people.

## Rows

| ID | Feature (evidence) | From | State | Now | Open questions | Action plan | Better by | P |
|---|---|---|---|---|---|---|---|---|
| E05 | Calendar sync: Google, Outlook; meetings on timeline (docs) | Dex | ARCHIVED | branch archive/calendar, tag archive-calendar-2026-10-16, removed from main 2026-10-16 (no code existed; calendar sync adapter was J02) | Calendar source: Radicale only, or also Google? Attendee matching by email only? | Pull from Radicale/CalDAV via `contacts-calendar` skill into interactions (type meeting), match attendee emails | connected | P2 |
| D18 | Calendar smart reminder (anniversary of an event) (docs) | Dex | SKIP | none | - | Needs calendar sync (E05) | - |  |
| J01 | Events: title, start/end, all-day, timezone, location, attendees (linked people), recurrence, reminders, notes | Outlook/Thunderbird | ARCHIVED | branch archive/calendar, tag archive-calendar-2026-10-16, removed from main 2026-10-16 | Store in the CRM DB or only as CalDAV? | Table `events` (uuid, title, start_utc, end_utc, tz, all_day, location, rrule, status) + `event_people`; CLI `crm cal add/list/show/edit/rm`, MCP, ICS import/export | connected | P1 |
| J02 | Calendar sync both ways via CalDAV (Radicale first), Google/Outlook later; event <-> interaction (meeting) link | Dex E05, Thunderbird | ARCHIVED | branch archive/calendar, tag archive-calendar-2026-10-16, removed from main 2026-10-16 | Which calendars to include? | H02 adapter `caldav`; `external_ids` maps events; attendee emails match people via H04; past meetings create `meeting` interactions | connected | P1 |
| J03 | TUI calendar views: Day, Week (columns by day), Month, Agenda list; jump keys (t today, [ ] prev/next, d/w/m view), mouse click on a slot | Outlook, Motion | ARCHIVED | branch archive/calendar, tag archive-calendar-2026-10-16, removed from main 2026-10-16 | - | New view "Calendar" in the registry; events, tasks and birthdays on one grid; palette actions for every view; keep the screen clean (agenda by default on narrow terminals) | fast, tactile | P1 |
| J04 | Create and move events in the TUI by keyboard (quick add "lunch with Sam fri 12:30 1h"), drag by mouse optional | Motion | ARCHIVED | branch archive/calendar, tag archive-calendar-2026-10-16, removed from main 2026-10-16 | Parser library? | Quick-add prompt parses text, matches people by name, shows a preview before saving | fast | P1 |
| J05 | Tasks on the calendar: each task has duration, deadline, priority, optional earliest start; shown as blocks | Motion | ARCHIVED | branch archive/calendar, tag archive-calendar-2026-10-16, removed from main 2026-10-16 | - | Add `duration_min`, `earliest_start`, `priority` to tasks; blocks rendered in J03 | connected | P2 |
| J06 | Smart scheduling: auto-place unscheduled tasks into free time before their deadlines, respecting working hours, existing events, buffers, task priority; reschedule when events change; explain each placement | Motion | ARCHIVED | branch archive/calendar, tag archive-calendar-2026-10-16, removed from main 2026-10-16 | Working hours and buffers config? Auto-apply or propose? | Deterministic scheduler in `internal/schedule` (greedy by deadline then priority with buffers, no AI required); `crm cal plan [--apply]` proposes a plan, `--apply` writes `scheduled_start` on tasks; TUI shows the proposal for accept/tweak. Agents can add judgment on top | smart | P2 |
| J07 | Find a time with people: `crm cal free --with <person...> --within 7d --len 45m` using their calendars where known (CalDAV free-busy) or only yours | Outlook | ARCHIVED | branch archive/calendar, tag archive-calendar-2026-10-16, removed from main 2026-10-16 | - | Free/busy from your calendars; others via FREEBUSY if available; palette "Find a time" | smart | P2 |
| J08 | Keep-in-touch and birthdays appear on the calendar; "schedule a catch-up" from a Due contact proposes slots | Dex, Us | ARCHIVED | branch archive/calendar, tag archive-calendar-2026-10-16, removed from main 2026-10-16 | - | Calendar overlay layers (toggle); action "Schedule catch-up" in Due view uses J07 | connected, smart | P2 |
| J09 | Booking links (Calendly/Motion style): you define event types (name, length, buffers, hours, days), share `https://book.andersonvdc.com/<slug>`; the invitee picks a slot, enters name and email; the CRM creates the event, finds or creates the person, logs an interaction, emails both a confirmation with an ICS file | Calendly, Motion | ARCHIVED | branch archive/calendar, tag archive-calendar-2026-10-16, removed from main 2026-10-16 | Stretch goal. Host on pibrain behind relayd like crm.andersonvdc.com? Public form needs rate limit and spam protection; invitee emails via `imap-smtp`? | `crm book serve` (small public HTTP server separate from the TUI web terminal, read-only on availability), availability computed from J01/J02 and event-type rules, double-booking check in a transaction, per-IP rate limit, no login, confirmation and cancel links with signed tokens; palette "Booking links" manages types and shows bookings | connected, simple | P3 |
| J10 | Booking and calendar actions for agents: MCP and `-f json` for events, plans, free slots, bookings, with `--dry-run` | Us | ARCHIVED | branch archive/calendar, tag archive-calendar-2026-10-16, removed from main 2026-10-16 | - | Each J feature ships CLI + MCP + help topic (rules 1 and 3) | extensible | P1 |
| M07 | Calendar: keep `[` `]` as global view switching; step period with Tab / Shift+Tab. ICS import is one-shot as-is by UID; two-way CalDAV is the sync path. Scheduling follows Motion, lean | Motion, Thunderbird | ARCHIVED | branch archive/calendar, tag archive-calendar-2026-10-16, removed from main 2026-10-16 | - | See CALENDAR-PLAN.md | simple | P1 |
