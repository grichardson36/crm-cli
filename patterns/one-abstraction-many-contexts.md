---
type: pattern
status: decided by owner 2026-10-28
---
# One abstraction, many contexts

Do not build the same behavior twice. When one abstraction, function or interface serves several object types and several interfaces (CLI, MCP, TUI, import, sync), build it once and reuse it. A feature that needs a special version for one context is a sign the abstraction is wrong.

Examples decided so far:
- One verb set for every object ([[one-verb-set]], [[universal-verbs]]).
- Sort and filter are one overlay in every view, and a filter also reduces what a bulk action selects ([[bulk-actions-always]]).
- Select all is `ctrl+a` everywhere. There is no "mark all visible" action.
- Editing a link's role or dates is editing a field on the linked row, not a separate "edit link" verb ([[field-types-drive-editors]]).
- Validation lives in one framework used by every interface ([[one-validation-framework]]).
- Purge is a generic verb across object types.
- One matcher and one popup serve the bar completions, the relation typeahead and every pipe line ([[autocomplete-component]]).

Test before adding an action: can an existing verb in a different context do this? If yes, do not add it.
