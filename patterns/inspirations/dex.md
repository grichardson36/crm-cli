---
type: inspiration
kind: crm
research: mined
---
# Dex

**What it is.** Local-first personal CRM and our migration target. Entities seen in the app: contacts, emails, phones, custom fields, hierarchical groups, tags, timeline items, reminders, birthdays, saved views and filters, shared workspaces. Routes include keep-in-touch (board and list by frequency), birthdays, merge-and-fix, network insights and priority map.

**Research state.** mined. See [[inspirations]].

**What we take.**
- Keep-in-touch cadence per person with an enum (7 days, 14 days, 1 month, 42 days, 3 months, 6 months, 1 year, never) and priority tiers (Top, High, Medium, Low, None). [[relationship-loop-first]] [[home-is-an-action-queue]]
- Merge-and-fix duplicates with a compare screen. [[soft-delete-restorable]]
- User-defined note types, timeline items that carry meeting type and calendar event id. [[one-record-page]] [[timeline-upcoming-first]]
- CSV import with column mapping that creates custom fields from unknown columns. [[import-is-a-snapshot-sync-is-a-relationship]]
- Dex already exposes API, MCP, CLI and Zapier, which validates the agent-surface direction. [[cli-json-is-the-baseline]]

**What we leave.** AI copilot and nightly summaries, SMS and WhatsApp reminders, team workspaces, paid-only API access. Dex has no documented command palette.

**Where we beat it.** Faster to reach (keyboard first), local files, agent-readable, no paid tier for API.

**Gaps to research.** Confirm field shapes through the Dex REST API (needs the Professional plan, not held). LinkedIn feed_items are not imported.

**Sources.** DEX-GAP.md (REA capture 2026-10-13 plus public JS, field names observed, types inferred). FEATURES.md has 94 mined rows. Dex CSV exports 2026-10-13 are imported (4445 contacts, 93 notes). github.com/getdex/agent-skills for the CLI and MCP docs.
