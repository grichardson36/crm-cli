---
type: pattern
scale: interaction
status: decided
---

# Bar results act like views

**Problem.** Finding a person in the bar and then leaving the bar to mark them contacted, snooze them or add a note costs a screen change for a one-key job. Plain letters type into the query, so the bar cannot also take verb hotkeys.

**Therefore.** Tab moves the focus from the input to the results. With the focus on the results a plain registry hotkey does to the highlighted row exactly what it does in that object's own view ([[same-key-same-verb]]): c marks a person contacted, s snoozes, f favorites, n adds a note, t a task, L links, d deletes (asks y/n), x completes a task, e edits an organization. The registry decides which verbs apply to which kind ([[objects-first]], [[one-verb-set]]): the key runs in a context that looks like the object's view with that object selected, so a verb the kind does not have does nothing and says so. Esc or Tab returns to the input, and the footer lists the highlighted object's verbs with their keys ([[show-the-key-beside-the-action]]). The bar stays open: the result refreshes, a toast names the action and the target, ctrl+z and ctrl+y undo and redo ([[undo-appears-in-the-bar]]). A verb that needs a prompt, a confirm or a picker takes the screen and the bar comes back when it is done. Read-only mode hides and blocks write verbs. A tag is an object in the bar: its name in the query lists it without #, an exact name leads the objects, enter opens its tagged people, and d, L and U delete the tag, give it to a person and take it off.

**Because.** One bar is the place to find and to do ([[one-bar]]). Chords were not required: ctrl+k then tab keeps the hands on the home row, and no key table is repeated, so rebinding a key in [keys] changes the bar too.

**Larger patterns.** [[one-bar]], [[same-key-same-verb]]

**Smaller patterns.** [[show-the-key-beside-the-action]], [[undo-appears-in-the-bar]], [[keys-are-rebindable]]

**Sources.** Owner request 2026-10-21. Superhuman and Linear act on the highlighted row from their command lists

**Inspired by.** [[superhuman]], [[linear]]
