---
type: pattern
status: decided by owner 2026-10-28
---
# Bulk actions are always available

Every verb works on a selection as well as on one object. When rows are selected, the verb acts on all of them. When none are selected, it acts on the highlighted row. This is mandatory for every object type and every verb in the framework ([[universal-verbs]], [[one-verb-set]]).

Selection: `space` toggles a row, `shift+up` and `shift+down` extend it, `ctrl+a` selects everything visible. The view's sort and filter fine-tune what is visible, so there is no "select all matching" verb. Destructive bulk verbs show the count and ask for a typed confirmation.

A generic "set field on selected" action covers every field type that has a fixed set of values (choice fields such as tier and deal stage, dates, numbers) so no field needs its own bulk action ([[field-types-drive-editors]]).

Why: one function in different contexts lowers what the user has to remember ([[one-abstraction-many-contexts]]).
