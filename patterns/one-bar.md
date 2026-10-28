---
type: pattern
scale: screen
status: decided
---

# One bar finds objects and runs verbs

**Problem.** Search and commands live in separate boxes.

**Therefore.** ctrl+k, and its aliases / and :, open one bar. Typing finds objects of every type, verbs and settings. With an object selected, the bar lists that object's verbs first. Ranking is fuzzy, then boosted by recency and by the current view. Ranking never reorders sections. A tag whose name matches the query is a result row too, without the # prefix, and an exact name leads the objects. Tab moves the focus into the results where the view hotkeys act on the highlighted object ([[bar-results-act-like-views]]).

**Because.** Every verb has a bar entry. A verb without one does not exist.

**Larger patterns.** [[objects-first]]

**Smaller patterns.** [[bar-results-act-like-views]], [[bar-modes-by-first-word]], [[bar-scope-prefixes]], [[empty-bar-shows-recents]], [[create-on-miss]], [[show-the-key-beside-the-action]], [[undo-appears-in-the-bar]]

**Sources.** Superhuman, Raycast and Linear single list. Obsidian uses two boxes, VS Code uses prefix modes (DESIGN-PATTERNS.md C)

**Inspired by.** [[hubspot]], [[obsidian]], [[vscode]], [[superhuman]], [[linear]]
