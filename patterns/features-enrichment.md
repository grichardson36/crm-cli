---
type: feature-area
---
# Enrichment and data connections

Fetched data is a suggestion until the user accepts it.

Moved from FEATURES.md on 2026-10-29. Every row keeps its ID and text. Update a row's State and Now in the same session you change the feature ([[keep-the-tracker-accurate]]). Column meanings and the row lifecycle are in [[feature-rules]].

**Follows patterns:** [[enrichment-is-suggested-then-accepted]], [[field-sources-are-ranked]], [[extension-ladder]]

**Research:** [[research-hubspot-export]]

**See also:** [[index]]

Rule: fetched data is a suggestion until the user accepts it (pattern enrichment-is-suggested-then-accepted). Suggestions show dimmed or grayscale beside the current value. The user highlights one and presses enter to cache it as canon. Conflicting results from several sources can be arrowed through before choosing. Fetching may run automatically or on demand, with low price and low load: fetch what was asked or is stale, cache, remember rejections.

## Rows

| ID | Feature (evidence) | From | State | Now | Open questions | Action plan | Better by | P |
|---|---|---|---|---|---|---|---|---|
| A16 | Enhance from LinkedIn: title, company, location, photo (docs) | Dex | TODO | none | Which source: LinkedIn page via agent, or manual only? | Agent task (browser/REA), not built into CLI. Leave a `crm enrich` hook later | - | P3 |
| M10 | LinkedIn connection to fetch organization and person data quickly (Dex has one). Scope out: official APIs and terms, what data may be fetched, consent, and an adapter on the H02 framework (owner idea 2026-10-16) | Dex | MOVED to N02 | None | What does LinkedIn allow (API, export, browser extension)? Which fields? Terms-of-service limits? | Research first, then scope as an adapter that proposes updates into the sync inbox for confirmation | fast, smart | P3 |
| N01 | Suggestion store and accept flow: per-field suggestions with source, fetched_at and confidence; TUI highlight and enter to accept, arrow through alternatives, reject memory; CLI `crm suggestions list\|accept\|reject`; MCP; accepted values keep their source | Us, Dex | TODO | `sync_inbox` exists as groundwork (H02) | Reuse sync_inbox or a new table? Scope of fields? | Build on the H02 inbox, one record page section "Suggestions", dimmed values | smart, safe | P2 |
| N02 | LinkedIn connection to fetch organization and person data (moved from M10): scope official APIs, terms, allowed fields, consent | Dex | TODO | None | What does LinkedIn allow (API, export, browser extension)? | Research, then adapter on the H02 framework that only creates suggestions (N01) | fast, smart | P3 |
| N03 | Enrichment providers as pluggable connections: Clay, Anymailfinder and similar APIs, with keys held locally; per-provider field maps | Clay, Us | TODO | None | Which provider first? Pricing, rate limits, terms? | H02 adapter per provider, fields flow into N01 suggestions, credits spent only on requested or stale records | fast | P3 |
| N04 | Small scraper or script for organization data from a domain (site, description, logo, location), user initiated or scheduled | Us | TODO | None | Where does it run, how often, what is allowed to be fetched? | Local script producing N01 suggestions, logo to the image store (M16) | fast | P3 |
| N05 | Refresh policy: auto or manual per object type, staleness window, budget cap per provider, dry run showing cost before spending | Us | TODO | None | Defaults? | Config plus `crm enrich --dry-run`, estimates shown before any paid call | safe | P3 |
