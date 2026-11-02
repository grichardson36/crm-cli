---
type: roadmap
---
# Roadmap

Items here are not built. Decisions behind them live in this vault, start at [[index]].

## Next

- Phone number normalization to the E.164 standard. Today `PhoneKey` (`internal/dedupe/normalize.go`) only reduces a number to its last 10 digits for matching, and the stored value stays as typed. Plan: parse with a phone number library using a default region, store the original and the E.164 form, display in national format, match on E.164, and keep the original text for the vCard round trip (see [[research-sync-standard]]).

## Someday / maybe

- User-defined commands (shell templates like `lazygit` custom commands). The verb set stays fixed for now ([[research-tui-building-blocks]], Q30).
- Smart linking suggestions (kind of how we want to suggest via enrichment) but these link suggestions come from 1. links you've made (Joey and Hannah Osgood are spouses, Jude Osgood is child of Joey Osgood, so Hannah Osgood may also be parent of Jude Osgood, so Hannah Osgood's link section in her note grayscale-suggests Jude Osgood as a child link) 2. Corroborated from link evidence found via social media networks (Joey lists Hannah as his wife on his profile). Never automatically make a link - suggest it in grayscale and let the user accept or reject the suggestion. We should collect the link suggestions in a place the user can go through them (kind of like Merge view) and further deepen their node graph of relationships. Related connections (orgs, tags, mutual friends, previous related school or work) can help strengthen the suggestion algorithm but should really be left to really strong links typically within family of origin and super close friends.
- Node graph view of object nodes and their link edges like [[obsidian]]
- Have a way to edit an activity's date inline
- 