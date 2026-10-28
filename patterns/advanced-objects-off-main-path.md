---
type: pattern
scale: product
status: decided (revised 2026-10-20)
---

# The default navigation is loose and the user arranges it

**Problem.** A prescriptive default (a short fixed tab list, with everything else hidden) decides for the user what matters. A cluttered default hides nothing and helps no one find the daily path.

**Therefore.** The program ships a loose default: every module is visible and in a sensible order, so the options are discoverable. The user hides, shows and reorders from there ([[modules-are-user-arranged]]). The daily path (Home, People, Follow-up, Tasks) comes first in the default order. Nothing is removed from the program because it is not on the daily path. Every object follows the same verbs and keys whether its module is shown or hidden.

**Because.** Owner decision 2026-10-20: the earlier fixed list was "too prescriptive". Acceptance: a user can clear the weekly follow-up queue without visiting Deals, Orgs, Sync or Settings, whatever they have hidden.

**Larger patterns.** [[relationship-loop-first]], [[extension-ladder]]

**Smaller patterns.** [[modules-are-user-arranged]], [[tui-parity-for-daily-actions]]

**Sources.** CRM-UX-RESEARCH.md O3, R4, owner decisions 2026-10-19 and 2026-10-20

**Inspired by.** [[hubspot]], [[salesforce]], [[notion]]

**Note (2026-10-16).** Archived with calendar: events are not an object on main.

**History.** Until 2026-10-20 this pattern was "Advanced objects stay off the main path" and fixed navigation to Home, People, Follow-up and Tasks. The file name is kept so links still resolve.
