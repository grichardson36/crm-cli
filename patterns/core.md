---
type: guide
---
# Core design gates

This pattern language and the program are enforced by these core gates. Violation of these gates across the repository must be flagged with the user and removed. Supporting patterns supply reusable, owner-decided answers when a gate is not specific enough.

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

## Workflow & instructions

`core.md` is the active source of truth for requirements, decisions, workflow state and the roadmap. Agents update it in place. Completed work moves to [[completed-roadmap]] to preserve working context.

**Task format**

Use one checklist under `# Roadmap`, with `next` or `someday` priority. Preserve the original request. Expand a task's record only when work begins.

```
- [ ] 🧭 🔨 🧪 CRM-001: Feature description (next)
  - Intent: ...
  - Decisions: ...
  - Scenarios: Given ... When ... Then ...
  - Test seam: ...
  - Evidence: ...
  - Exceptions: ...
```

**Badges are earned, not assigned.** Append each badge only after its gate passes:

🧭 Planned > 🔨 Built > 🧪 Tested > 🔍 Reviewed > 📚 Documented > ✅ Implemented

The first missing badge identifies the next gate. A failed gate retains its previous badges. Skip work only when its gate is already satisfied with evidence. Never invent a passing result.

### 1. Plan 🧭

**INPUTS:** `core.md`, `index.md`, request or conversation, relevant code, existing patterns and ADRs.

**INSTRUCTIONS:**

- Synthesize existing decisions before asking questions. Ask the owner only about unresolved decisions that block progress.
- Apply all eight core design gates. Prefer existing concepts, behaviors and test seams.
- Describe expected behavior, not an implementation invented to satisfy the request.

**WORKFLOW:**

1. Select or capture a roadmap request. Preserve its intent and assign a stable ID.
2. Inspect relevant code, tests, domain vocabulary, patterns and architectural decisions.
3. Identify applicable rules, boundaries, assumptions and existing test seams. Prefer the highest reliable observable seam.
4. Load `/grilling` and `/ascii-wireframe` skills.
5. Challenge ambiguity, contradictions, missing behavior, edge cases and failure conditions through `/grilling`, ATDD/BDD discovery, and wireframing.
6. Formulate agreed behavior as concise, testable Given/When/Then scenarios. Record implementation decisions, exclusions and applicable doors.
7. Resolve blocking questions with the owner. Do not silently turn assumptions into requirements.

**REPORT:** Update the roadmap item with intent, decisions, scenarios, test seam and unresolved exceptions.

**GATE:** Required behavior is unambiguous, scenarios are testable, and decisions are traceable to the request or owner approval. Add 🧭.

### 2. Build 🔨

**INPUTS:** Approved scenarios, relevant code, existing tests and architecture.

**INSTRUCTIONS:**

- Use git worktrees for parallel execution, one sub-agent per branch. The coordinating agent owns integration, final merge and push.
- Tests derive from approved scenarios, never from implementation behavior.
- Implementation agents may not weaken tests or alter approved scenarios to obtain a pass.
- Keep changes small and within approved scope.

**WORKFLOW:**

1. Select one unimplemented scenario.
2. **Red:** Write its test and prove it fails for the intended behavioral reason. If existing code already satisfies the scenario, record the passing evidence instead.
3. **Green:** Write the minimum code necessary to pass the test. Run relevant regression tests.
4. **Refactor:** Improve the code without changing behavior. Keep tests green.
5. Repeat steps 1–4 until all approved scenarios are implemented.

**REPORT:** Record scenario-to-test references, implementation changes and red/green evidence in the task.

**GATE:** Every approved scenario has a passing test and implementation, with no unauthorized changes to requirements or tests. Add 🔨.

### 3. Test 🧪

**INPUTS:** Approved scenarios, implementation, tests and build evidence.

**INSTRUCTIONS:**

- Verify observable behavior, not internal implementation details.
- Prefer existing test infrastructure and seams.
- Introduce additional test types only where scenarios or engineering risks justify them.

**WORKFLOW:**

1. Verify every approved scenario maps to passing tests across applicable interfaces.
2. Run relevant integration, contract, regression and invariant checks. During the work run only the affected packages and tests (`go test ./internal/tui -run <regex>`). The full `go vet ./...` and `go test ./...` run once per deploy, inside `deploy/deploy.ps1`, so do not repeat it by hand before a deploy.
3. Perform a mutation sweep. Investigate surviving meaningful mutations and strengthen inadequate tests.
4. Prune redundant tests without losing scenario coverage or demonstrated defect detection.
5. Rerun affected tests and regression checks.

**REPORT:** Record coverage, test results, meaningful mutation survivors and exceptions.

**GATE:** All required tests pass, every scenario is verified, and no unexplained meaningful mutation or coverage gap remains. Add 🧪.

### 4. Review 🔍

**INPUTS:** `core.md`, approved scenarios, code diff, tests, evidence and relevant patterns.

**INSTRUCTIONS:**

- Review independently of the implementing agent where practical.
- Apply all eight core design gates.
- Challenge unnecessary complexity, duplicated concepts, scope expansion and deviations from approved behavior.

**WORKFLOW:**

1. Compare implementation against the original request and approved scenarios.
2. Check architecture, naming, canonical data, applicable doors, safety, reversibility and sync behavior.
3. Verify test integrity, evidence and compliance with existing patterns.
4. Resolve findings. Return failed work to the appropriate earlier gate and invalidate affected badges.
5. Obtain owner decisions for unresolved deviations or changes to requirements and core gates.

**REPORT:** Record findings, resolutions and any remaining approved exceptions.

**GATE:** The implementation satisfies the approved behavior and core design gates without unresolved blocking findings. Add 🔍.

### 5. Document 📚

**INPUTS:** Reviewed changes, `core.md`, `index.md` and relevant project documentation.

**INSTRUCTIONS:**

- Make de minimis updates to domain-model, architecture, runtime, operations or none, as appropriate.
- Do not duplicate information already defined elsewhere.
- The coordinating agent owns final integration, merge and push to main.

**WORKFLOW:**

1. Update only documentation affected by the implementation.
2. Confirm the integrated changes pass required checks.
3. Merge and push to main. Verify the resulting commit.
4. Deploy a code change to the running program with `deploy/deploy.ps1` ([[references/app/operations|app operations]]). Never build, test, copy or swap `bin/crm.exe` by hand.
5. Add 📚 and mark the task `[x] ✅` only after the implemented result is verified.
6. Move a concise completion record to [[completed-roadmap]], preserving its ID, outcome, important decisions, exceptions and commit reference.
7. Remove the completed task and its working notes from the active roadmap.

**REPORT:** Final implementation reference and archived completion record.

**GATE:** Code is integrated, required checks pass, documentation is current and the completion record exists. The task no longer appears in the active roadmap.

---

**Global enforcement:** No gate advances without evidence. Approved behavior cannot be silently changed by an implementation or testing agent. Failure returns work to the earliest affected gate. The roadmap records current state; [[completed-roadmap]] preserves completed decisions and implementation references.

## This app

Current app facts live outside the core gates: [[references/app/domain-model|app domain model]], [[references/app/architecture|app architecture]], [[references/app/runtime|app runtime]] and [[references/app/operations|app operations]].

# Roadmap

Items below here are not built.

- ## Next
	- [ ] Interactive color picker for the Theme pane (today a hex field with a swatch): pick a color on a grid or sliders, keep the hex field as the typed path ([[custom-themes]]).
	- [ ] Custom themes ([[custom-themes]]): snippets in `~/.crm/themes/snippets`, theme checks in `internal/validate` with contrast warnings (`crm theme check`), a file watcher for live reload, `crm theme export` and `import`, a picker detail pane with a rendered sample.
	- [ ] Theme generator in the style of the theme-factory skill: `crm theme generate` takes a name, a mood or a few seed colors (accent, background) and writes both variants with roles derived from the seeds, checks contrast (4.5:1 text, 3:1 kinds) and the 16-color mapping, then shows the sample in the picker before saving ([[custom-themes]]).
	- [ ] Phone number normalization to the E.164 standard. Today `PhoneKey` (`internal/dedupe/normalize.go`) only reduces a number to its last 10 digits for matching, and the stored value stays as typed. Plan: parse with a phone number library using a default region, store the original and the E.164 form, display in national format, match on E.164, and keep the original text for the vCard round trip (see [[research-sync-standard]]).
	- [ ] Smart linking suggestions (kind of how we want to suggest via enrichment) but these link suggestions come from 1. links you've made (Joey and Hannah Osgood are spouses, Jude Osgood is child of Joey Osgood, so Hannah Osgood may also be parent of Jude Osgood, so Hannah Osgood's link section in her note grayscale-suggests Jude Osgood as a child link) 2. Corroborated from link evidence found via social media networks (Joey lists Hannah as his wife on his profile). Never automatically make a link - suggest it in grayscale and let the user accept or reject the suggestion. We should collect the link suggestions in a place the user can go through them (kind of like Merge view) and further deepen their node graph of relationships. Related connections (orgs, tags, mutual friends, previous related school or work) can help strengthen the suggestion algorithm but should really be left to really strong links typically within family of origin and super close friends. If A is sibling with B and B sibling with C then A is probably sibling with C. We need to think through relationships like that to build a strong semantic model we can convert into code.
	- [ ] Merge candidates for other object types (Orgs, Tags) to keep the data clean (&=and for instance) all put in the Merge list screen and ctrl+o sortable/filterable/groupable. Base merge screen shows all objects grouped by confidence with object type the user's object type colors. Need to think through merge behavior for these.
	- [ ] nicknames/aliases variable for People ("Bob" would also show in a search, their official name on social is Robert Borer but I call them Bob Borer), organizations, and tags
		- [ ] Incorporate maiden names
	- [ ] Sorting/Filtering/Grouping improvements
		- [ ] Add full functionality to all views - make this a modular component
		- [ ] Be able to sort and group by other variables - maybe the user can pick from all our tracked variables
	- [ ] Like Dex, track changes to LinkedIn title/subscript and aggregate them to a list. Look and function very similar to Dex. This can go in Home screen just like Dex.
	- [ ] Home screen
		- [ ] Customizable - user can see what modules they'd like
	- [ ] Locations as a view w/ standard sort/filter/group - research Dex implementation
	- [ ] Fix the flaky `TestConcurrentOpenMigratesOnce` in `internal/db` on Windows (fails about 6 of 20 runs with "Access is denied" on the migrate lock and "database is locked"), so the deploy test step stops failing at random.
	- [ ] Speed up the TUI test suite: 637 tests average 0.46s each with no slow outliers, so look at the per-test setup in `newTestModel` (a seeded database copied per test instead of migrated per test).
- ## Someday / maybe
	- [ ] Node graph view of object nodes and their link edges like [[obsidian]]
	- [ ] Deep enrichment capabilities - sync to LinkedIn, Facebook, email servers (Gmail, etc), Beeper/messaging apps, phone call history
		- [ ] Enrichment view screen
			- [ ] User approves linking and data suggestions
			- [ ] If it sees a phone call, the program could ask what the call was about if note is missing
			- [ ] Facebook and LinkedIn information merges/blends with user data (we should really work hard to model this out for all possible scenarios so we have a rock solid sync model). For instance, seeing University of Florida on their LinkedIn profile would use the user's University of Florida (UF) tag
			- [ ] Web searches can confirm general tags for people/organizations (i.e. "construction", "engineering", "architecture", "law", "nonprofit" etc.)
			- [ ] Having a link following that platform's pattern creates a sync job in the sync engine (we still need to build)
			- [ ] Facebook relationships show up as object links (Rylan Mort and Emily Edwards are spouses)
	- [ ] Peer sharing of syncing contact data - shares contacts of a group/tag with another person - they merge and sync to the best version of "Nathan Walter" - some vars will need to be private and some synced
	- [ ] import-export
		- [ ] Bulk import, sanitization, validation and dedupe from clipboard
		- [ ] CSV, XLSX, etc
	- [ ] Link select have the inverse available as an option (for example could select parent or child)
	- [ ] Tony/Anthony Ricordati merge candidate  has two companies - he may have worked for one beforehand so it should show up in his history. Not sure how the user gets to pick how that history works via the merge screen.
	- [ ] User-defined commands (shell templates like `lazygit` custom commands). The verb set stays fixed for now ([[research-tui-building-blocks]], Q30).
	- [ ] Custom fonts integrated into themes
	- [ ] "Keys" and "Framework verbs" in Settings seems extremely redundant and confusing for the user. Grill with user before settling on how best to configure these controls
