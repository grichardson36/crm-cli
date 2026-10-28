---
type: pattern
scale: interaction
status: decided
---

# Undo and redo are two stacks

**Problem.** Edits cannot be reversed, or redo behaves unpredictably.

**Therefore.** Every change from the TUI is a command in a journal with an undo stack and a redo stack. A new command clears redo. Undo and redo are not themselves entries. Keys: ctrl+z undo, ctrl+y or ctrl+r redo (rebindable). Session only, owner decision. A person or organization merge is on the stack and also in a persistent merge log (`crm merge undo`), so it can be undone in a later session.

**Larger patterns.** [[soft-delete-restorable]], [[same-key-same-verb]]

**Smaller patterns.** [[toast-names-action-and-target]], [[undo-refuses-when-stale]], [[undo-appears-in-the-bar]]

**Sources.** Office, vim, lazygit (DESIGN-PATTERNS.md D)

**Inspired by.** [[motion]], [[microsoft-office]], [[vim-and-lazygit]], [[us]]
