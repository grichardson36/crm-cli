---
type: pattern
status: decided by owner 2026-10-28
---
# Grouped lists with dividers

A list module can group its rows under divider headings, declared in the view definition as the grouping field. Birthdays groups by day with a relative label. Merge candidates group by confidence level (high, medium, low, fuzzy). Other views can group by stage, date bucket or type with the same component.

The default shows everything grouped. The user narrows it with the view's sort and filter overlay, which is the same overlay in every view ([[one-abstraction-many-contexts]]). A group divider is not a row you can act on, but selection and bulk verbs apply across groups ([[bulk-actions-always]]).

The grouping field is part of the list module's declaration, so a new program gets the behavior without UI code ([[navigation-units-and-homes]]).
