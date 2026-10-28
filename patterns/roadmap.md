---
type: roadmap
---
# Roadmap

Items here are not built. Decisions behind them live in this vault, start at [[index]].

## Next

- Phone number normalization to the E.164 standard. Today `PhoneKey` (`internal/dedupe/normalize.go`) only reduces a number to its last 10 digits for matching, and the stored value stays as typed. Plan: parse with a phone number library using a default region, store the original and the E.164 form, display in national format, match on E.164, and keep the original text for the vCard round trip (see [[research-sync-standard]]).

## Someday / maybe

- User-defined commands (shell templates like `lazygit` custom commands). The verb set stays fixed for now ([[research-tui-building-blocks]], Q30).
