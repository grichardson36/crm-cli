---
type: pattern
scale: object
status: decided (owner 2026-10-30), built in feat/notes-feed (not merged)
---

# Notes and interactions are a readable feed

**Problem.** Notes on a record page were one cut-off line each (three lines of the rest with an ellipsis), so a note could not be read in the page and an email or meeting summary was lost.

**Therefore.** The Recent section of every record page (person, organization, deal) is a feed that continues down the page. It is one component (`internal/tui/feed.go`), not a copy per page. Notes and every interaction (email, call, meeting, message, user types) are entries, newest first.

```
Recent
 2026-10-27 note   Note text, wrapped to the
  Acme Corp        width of the right column
  Bob Smith        ...
 2026-08-01 email  Re: pricing (bold subject)
  Globex           Email text
```

**Layout.** Two columns. The left column holds the date (once per day, later entries of that day leave it blank), the note type to the right of the date, and under them the linked objects, one per line, in their kind color (`m.st.KindStyle`, the theme roles of [[related-segment]]). Without color the same prefixes as the related segment name the kind: `p:` person, `o:` organization, `d:` deal, `#` tag. The object whose page it is stays out, every entry is linked to it. The right column holds the text wrapped to the pane. A blank line separates days. Below 20 cells of text width the entry stacks: heading line, objects, then the text indented. More than 50 entries show a dim "older entries are not shown" row.

**Wrapping.** Line breaks of the text are kept, blank lines collapse to one, a list marker gives a hanging indent, escape sequences and control characters are dropped. A word wider than the column (a URL, a CJK run) is cut at the column and a wide character is never split. It re-wraps on every resize.

**Expand rule.** A collapsed entry shows at most 5 lines. If text is hidden the 5th line ends with a dim `...`, and only then. `enter` or `e` on such an entry expands it in place, `enter` again collapses it, and the page keeps scrolling. An entry that shows everything is edited by `enter` or `e` as before, so a long entry is edited from the palette (Edit note) while it is the cursor row. An expanded entry taller than the pane scrolls inside the page: `up`, `down`, `pgup`, `pgdn` move the viewport through it, and at its end the cursor moves to the next entry.

**Keys (none new).** Entries are rows of the page list, so the universal verbs apply ([[universal-verbs]]): `d` delete (y/n), restore from the palette or `ctrl+z`, `l` and `u` link and unlink, `right` the type cell, `space` selects the entry and moves down, then `d` and the other verbs act on all selected entries with the count typed back ([[bulk-actions-always]]). `ctrl+up` and `ctrl+down` jump by entry ([[navigation-units-and-homes]]). Editing uses the long text editor: `alt+enter` is a new line ([[field-types-drive-editors]]).

**Scrolling.** The page is one document. The cursor moves entry by entry and the viewport follows it.

**Because.** Owner request 2026-10-30. Reading a note must not need a second screen.

**Larger patterns.** [[one-record-page]], [[timeline-upcoming-first]]

**Smaller patterns.** [[universal-verbs]], [[related-segment]], [[grouped-lists-with-dividers]]

**Sources.** `internal/tui/feed.go`, `internal/tui/pages.go` (`timelineRows`), `internal/tui/feed_test.go`
