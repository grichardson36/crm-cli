---
type: guide
---
# Core design gates

Read these gates in order before changing CRM behavior or design. Core gates define the questions every change must answer. Supporting patterns supply reusable, owner-decided answers when a gate is not specific enough.

1. **Requirement:** Does the requirement have a traceable decision and purpose? Can it be deleted? Simplify what survives, shorten its feedback loop, and automate only when stable.
2. **Relationship loop:** Does it help the user find a person, understand the relationship, capture what changed or complete the next follow-up? If not, get an owner decision.
3. **Local boundary:** Does it work without an account or network, with data owned locally?
4. **Concept and name:** Can an existing object, field, link or activity express the fact? What is its canonical term and class naming convention? Use it in the model, storage and every CRM door.
5. **Source of truth:** Is every representation canonical, a derived projection or an approved boundary translation? Anything else is a duplicate. Delete it or record a discrepancy before extending the concept.
6. **Behavior:** Can every applicable CLI, TUI, MCP and agent door use one behavior with the same validation and effects? State any omitted door in the feature record.
7. **Safety and boundaries:** Is the change validated, reversible, archive-safe and sync-safe? Do external inputs and outputs map into the canonical CRM model rather than define it?
8. **Evidence and escalation:** If a gate does not answer the question, use relevant standards, project research and inspirations. If still unresolved, record the gap and recommendation for the owner. Do not invent a local exception.

## Add a supporting pattern

A supporting pattern is a reusable, owner-decided answer to one core gate. Add one only when all are true:

1. A core gate leaves a stable decision unanswered.
2. It will guide at least three future features or doors.
3. It has a default and meaningful exception.
4. It has an observable check.

Otherwise use a feature record, reference, research note or discrepancy. A supporting pattern links directly to one core gate and cannot create child patterns.

A supporting pattern contains one current decision. A replacement uses `status: superseded` and a `replaced-by` link to its current note; it contains no history. Commands, schema, migrations, UI keys, test paths and research belong in references or evidence.

## Add a core gate

Only an owner decision adds or changes a core gate. Add one only when all are true:

1. It defines a durable product or engineering invariant.
2. It applies across most domains and future work, not one object, door or integration.
3. It changes how agents evaluate a proposal, not merely how they implement it.
4. No existing core gate can answer it without becoming unclear.
5. It can be stated as one short question with a clear pass or fail.

Otherwise use a supporting pattern, feature record, reference, research note or discrepancy.

## This app

Current app facts live outside the core gates: [[references/app/domain-model|app domain model]], [[references/app/architecture|app architecture]], [[references/app/runtime|app runtime]] and [[references/app/operations|app operations]].
