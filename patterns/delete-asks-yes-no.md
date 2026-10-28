---
type: pattern
scale: object
status: decided
---

# Delete asks y or n

**Problem.** Delete either fires without warning or buries the user in dialogs.

**Therefore.** d on a selected row asks y/n in the footer. Soft deletes then support undo. Bulk deletes name what they will touch first.

**Larger patterns.** [[one-verb-set]]

**Sources.** Task delete (commit 71fb2da), orgs and deals (commit 84aa84d)
