---
type: pattern
scale: interaction
status: decided
---

# Fetched data is a suggestion until the user accepts it

**Problem.** Scraped or purchased data can be wrong, stale or conflicting. Writing it straight into the record corrupts the user's own data and hides where a value came from.

**Therefore.** A fetch never writes to a record. Results appear as suggestions beside the current value, dimmed or in grayscale, with their source and date. The user highlights one and presses enter to accept it as canon. When several sources disagree, the user arrows through the alternatives and picks one. Accepted values are cached with their source. Rejected ones are remembered so they do not return. A fetch may run automatically or on demand, and either way only changes the suggestion list.

**Because.** Owner decision 2026-10-19. Keeps price and load low: fetch only what is requested or stale, cache results, and spend credits on accepted data.

**Larger patterns.** [[local-first]], [[sync-maps-structure-to-flat-fields]]

**Smaller patterns.** [[unresolved-names-stay-text]], [[conflicts-keep-server-store-local]], [[pick-or-create]]

**Sources.** Owner decision 2026-10-19, [[features-enrichment]]

**Inspired by.** [[dex]], [[hubspot]], [[clay-me-sh]]
