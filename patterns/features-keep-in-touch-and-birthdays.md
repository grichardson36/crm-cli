---
type: feature-area
---
# Keep in touch, birthdays and Home

Cadences, due dates, snooze, birthdays, digests and the Home action queue. D18 (calendar smart reminder) is in [[features-calendar-archived]].

Moved from FEATURES.md on 2026-10-29. Every row keeps its ID and text. Update a row's State and Now in the same session you change the feature ([[keep-the-tracker-accurate]]). Column meanings and the row lifecycle are in [[feature-rules]].

**Follows patterns:** [[home-is-an-action-queue]], [[relationship-loop-first]], [[grouped-lists-with-dividers]], [[navigation-units-and-homes]], [[date-safety-rules]]

**See also:** [[roadmap]], [[index]]

## Rows

| ID | Feature (evidence) | From | State | Now | Open questions | Action plan | Better by | P |
|---|---|---|---|---|---|---|---|---|
| D01 | KIT cadences: 7 days, 14 days, 1 mon, 42 days, 3 mons, 6 mons, 1 year, never (docs, API) | Dex | DONE | `--keep-in-touch`, view `keep_in_touch_status`, MCP | - | none | simple |  |
| D02 | Next due = last interaction + cadence; no interaction = today + cadence (docs) | Dex | DONE | Migration 004: never-contacted people are due at the cadence-set date + cadence. Existing cadences were stamped with the migration date. | - | Differs from Dex when never contacted: Dex uses today + cadence, we use created date + cadence (imported contacts all come due at once). Decide and spread. DECIDED: never-contacted contacts first come due at (date the cadence was set, else today) + cadence, not created date. Needs `keep_in_touch_set_at` column set when cadence changes. Backfill existing cadences to the migration date. | - | P1 |
| D03 | Interactions in both directions reset the timer (docs) | Dex | PART | Any non-note interaction resets it (notes do not); no inbound/outbound flag | - | Fine as is unless message sync (E08) needs direction | - |  |
| D04 | KIT page: Upcoming tab, overdue first in red, board by frequency (docs, REA) | Dex | PART | `crm due`; TUI Due view flat list | Group by Overdue / This week / Later, or by cadence? | TUI Due view groups: Overdue, This week, Later; red for overdue; cadence board via B10 | tactile | P1 |
| D05 | Mark done with E (docs) | Dex | DONE | TUI `c` and `e`; CLI `person contacted`. | - | Add `e` alias | fast |  |
| D06 | Snooze with H, pick date (docs) | Dex | DONE | TUI `s`, `--snooze-until` | - | none | - |  |
| D07 | Set frequency with F; bulk; set by group (docs) | Dex | DONE | TUI `F` cycles cadence (single or marked); CLI and MCP. | Set by group needs B03. | TUI `F` cycle/pick; bulk via B11; by group needs B03 | fast | P1 |
| D08 | Ignored reminder repeats after one full period (docs) | Dex | TODO | none | - | Snooze auto-advance rule in `due`; document | - | P3 |
| D09 | Quick Actions: step through cards with shortcut keys (docs) | Dex | TODO | none | - | TUI Review mode: one contact per screen, keys c/s/F/n/skip, `n` next. DEFAULT: Review mode keys c contacted, s snooze, F frequency, n note, t task, space skip, b back, esc exit. | fast, tactile | P1 |
| D10 | Max 5 KIT notifications per day (docs) | Dex | TODO | none | - | Only matters for D14 digest; cap there | - | P3 |
| D13 | Birthdays page grouped by month; month/day/year each optional (docs) | Dex | DONE | `crm birthdays`; TUI Birthdays view groups the upcoming list by month and shows age when year is known. | - | - | intuitive | P2 |
| D14 | Daily digest 8:30, weekly digest Sunday, resurface email, KIT email (docs) | Dex | TODO | none | Delivery: email via `imap-smtp`, Beeper message, or terminal only? Schedule on which host? | `crm digest [--daily or --weekly]` prints text/markdown. Deliver through agent/cron to email or Beeper. Not built into the TUI | smart | P2 |
| D15 | Resurface: relevant past contacts without a cadence; resurface time and number settings (docs) | Dex | TODO | none | Weighting: tier, favorite, time since contact, and past note volume? | `crm resurface --n 5 --since 1y` (no cadence, not contacted for X, weight favorites/tier); TUI Home section | smart | P2 |
| D16 | Dex Shuffle: 3 random contacts with draft messages (docs) | Dex | TODO | none | - | TUI `S` shuffle key shows context and last note; drafting stays with the agent | tactile | P3 |
| D17 | Home page: events, Resurface, birthdays, KIT, reminders, network updates, recent interactions/notes (docs) | Dex | SKIP | DECIDED 2026-10-19: Home is an action queue only. It shows people due for follow-up and person-linked tasks due today or overdue. | - | Birthdays, sync, recent notes and other context stay in their dedicated views or the person workspace. Calendar and resurface remain outside the core loop. | fast, simple | P1 |
| D21 | Birthdays view as a calendar strip (owner 2026-10-21): month heading, a divider per day (date left, relative time right: today, in 1w 1d, in 1mo 1w, 3d ago), people below with the age they turn on that day at the right edge (blank without a birth year). Opens at today, 90 days of past birthdays above, 365 ahead. `ctrl+t` or `ctrl+up` snaps back to today (registry action `go-today`, rebindable). | Dex | DONE | BRANCH birthdays-ux. `model.RelativeDate` (calendar months, at most three parts, "ago" for the past) used by the TUI, `crm birthdays` and `crm_birthdays` (new JSON field `relative`, days_until stays and is negative in the past). `PersonRepo.BirthdaysWindow(back, ahead)`. Window constants `birthdayDaysBack` 90 and `birthdayDaysAhead` 365. Help topic `birthdays`. | CLI has no look-back flag | A look-back flag for the CLI and MCP (owner) | tactile, intuitive | P2 |
| D19 | LinkedIn title change alerts (docs, csv) | Dex | PART | `feed_items` CSV (1389 rows) exists, not imported | Import the 1,389 rows now or wait for a refresh path? | Import into a `network_updates` table; show in Home; refresh by LinkedIn export later | - | P3 |
| D20 | SMS/WhatsApp text reminders and digest (docs) | Dex | SKIP | none | - | Agent can message via Beeper if wanted | - |  |
| G09 | Status dashboard and context briefing in the TUI | Dex | DONE | Home view (first tab) in `internal/tui/home.go`: status, due, birthdays, tasks, recent notes, sync line. Loads in about 11 ms on 4,446 people. | - | Home view (D17) holds the status counts | connected | P1 |
