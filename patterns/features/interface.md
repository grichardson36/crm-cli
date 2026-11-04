---
type: feature-worklist
domains:
  - interface
---
# Interface work

Active work governed by [[standards/interface]]. `TODO` means not built; `PART` requires fresh code inspection before work starts.

## Shared doors and discovery

- [ ] `I01` (PART, P1): Keep help topics, TUI help and command discovery aligned from one source.
- [ ] `I05` (PART, P1): Keep MCP descriptions, help resources and schema aligned with the CLI.
- [ ] `I10` (PART, P2): Complete the install-by-prompt path and its verification behavior.
- [ ] `K07` (PART, P2): Keep help concise and derive agent-facing summaries from the same behavior.

## Input, actions and feedback

- [ ] `M09` (PART, P1): Complete one object/action registry across CLI, TUI and MCP.
- [ ] `K01` (PART, P1): Complete the shared short-syntax parser.
- [ ] `K02` (PART, P1): Apply short syntax consistently wherever typed text creates a record.
- [ ] `K03` (PART, P2): Complete live input preview chips.
- [ ] `K04` (PART, P2): Complete context-aware autocomplete and pickers.
- [ ] `K05` (PART, P2): Reuse those pickers for edits to existing records.
- [ ] `K06` (TODO, P3): Add concise, dismissible inline help only where it reduces ambiguity.
- [ ] `I09` (TODO, P3): Design first-run and contextual learning without a second help system.

## TUI and platform

- [ ] `M14` (PART, P1): Verify the unmerged user-arranged modules branch and merge only behavior that still fits the interface standard.
- [ ] `M17` (PART, P1): Verify command-bar result actions reuse the same object actions.
- [ ] `M22` (PART, P1): Verify the unmerged keymap layer and universal verbs before treating it as current behavior.
- [ ] Legacy `M23` Settings polish (PART, P2): Verify the branch before assigning a canonical ID. See [[open-discrepancies]] X49.
- [ ] `M16` (PART, P3): Decide whether terminal image support provides enough value over initials and text fallbacks.
- [ ] `F01` (PART, P3): Define the concrete user need for a web client before choosing a platform.
- [ ] `F02` (PART, P3): Define the concrete user need for a mobile client before choosing a platform.
- [ ] `F03` (PART, P3): Define the concrete user need for a desktop client beyond the TUI.
- [ ] `F14` (PART, P3): Evaluate phone-friendly terminal behavior only against identified mobile workflows.
- [ ] `F15` (PART, P1): Complete themes and custom theming. Current roadmap items supply the next bounded work.
