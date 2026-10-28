---
type: pattern
scale: interaction
status: decided
---

# The same key means the same verb

**Problem.** Keys differ per view and muscle memory breaks.

**Therefore.** One key per verb, identical in every object view: add, enter to open or edit, d delete, restore, link, space to mark, ctrl+z undo, ctrl+y or ctrl+r redo. A key that cannot apply does nothing and never means something else. Keys specific to one object (stage on deals) are allowed and listed in the footer.

**Because.** Direct keys remain for high-frequency work but not one new key per feature. Keys in use: a add (+ alias), enter edit, d delete, u restore, L link, U unlink, space mark. L and U were chosen by an agent, owner to confirm.

**Larger patterns.** [[one-verb-set]]

**Smaller patterns.** [[keys-are-rebindable]], [[show-the-key-beside-the-action]], [[undo-redo-two-stacks]]

**Sources.** Pipedrive shortcuts (CRM-UX-RESEARCH.md O2), Obsidian hotkeys

**Inspired by.** [[pipedrive]]

**Update 2026-10-27 (feat/keymap-a).** Keys in use are now the table in [[universal-verbs]]: `a` add (picker), `enter` and `e` activate, `d` delete, `r` restore, `l` link, `u` unlink, `x` mark, `m` merge, `space` select, `ctrl+z`, `ctrl+y`, `ctrl+r`. `L`, `U`, `u` restore, `+`, `#`, `M` are retired. `TestKeyAudit` fails when a key gets a second verb ([[keymap-layers-and-scopes]]).
