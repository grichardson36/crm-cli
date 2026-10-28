---
type: pattern
scale: product
status: planned, not built
---

# Saved views (planned, not built)

**Problem.** A user who often looks at the same slice of data (open deals above a value, people tagged client who are due, tasks due this week grouped by organization) rebuilds it each time with the view options. The built-in views are fixed and the only way to add one is code.

**Therefore (planned).** A saved view is a module the user creates. It has a name, an object kind (any kind: person, organization, deal, task, note, tag), a filter, a sort and a grouping field. It appears in the Views screen next to the built-in views ([[views-screen]]), can be shown, hidden and ordered like them, and takes a number key by its place. Grouping uses the grouped-list component ([[grouped-lists-with-dividers]]) and the same view options overlay as every list view ([[one-abstraction-many-contexts]]).

- **Stored in the database**, in a table of their own, so they sync between devices ([[sync-safe-data]]). The arrangement (which are shown, in what order) stays per device in `config.toml` like the built-in modules ([[modules-are-user-arranged]]).
- **Shareable as TOML.** A saved view exports to a small TOML file and imports from one, so a view can be sent to another person or kept in a repository. The file names the kind, filter, sort and grouping and nothing personal.
- **Built on the existing ideas.** The view-file shape and the idea of a query-bound view module come from [[research-view-module-standard]]. The grouping comes from the grouped list. The filter language is the one listed in [[features-tags-groups-and-views]].
- **Bulk actions apply** to a saved view like any list ([[bulk-actions-always]]).

**Status.** Planned. Nothing here is built. The open questions are the filter syntax (one language for the bar, the CLI and the file), whether a saved view can nest another, and how a view whose kind is later removed behaves. They are for the owner ([[surface-gaps-before-deciding]]).

**Larger patterns.** [[modules-are-user-arranged]], [[views-screen]]

**Smaller patterns.** [[grouped-lists-with-dividers]], [[bulk-actions-always]]

**Sources.** Owner request 2026-10-29. [[research-view-module-standard]]
