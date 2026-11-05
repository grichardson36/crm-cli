---
type: standard
domains:
  - interface
  - application
---
# Interface standard

## Scope

The user and agent surfaces: TUI, CLI, MCP and generated help.

## Current shape

CLI, TUI and MCP are adapters over the same CRM behavior. The TUI exposes views, object pages, prompts, overlays and a command bar. Help and machine-readable schema are the public discovery surfaces.

## Rules

- One behavior has one validation path and equivalent effects in every applicable door.
- Define an action once. Derive its availability, label and feedback from that definition rather than creating per-screen copies.
- The same key has one meaning in a scope. Hints use the same verb words as the action they invoke.
- A visible write gives clear feedback and preserves the path to undo when the behavior supports undo.
- A screen detail is a component or convention only when it recurs. One screen layout does not justify a new pattern.
- UI choices must remain operable by keyboard and usable in a narrow terminal. Visual enhancement cannot be the only path.

## Checks

- A changed behavior names its applicable doors and any intentional omission.
- A new key is checked against its scope, text-entry mode and visible hint.
- A new TUI component has a reused behavior, a focus/escape path and a narrow-width result.

## Evidence

[[research-tui-building-blocks]] and the Stitch design guidance are evidence, not CRM policy. Current work is in the [[core#Roadmap|roadmap]].
