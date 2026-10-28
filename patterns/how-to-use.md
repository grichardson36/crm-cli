---
type: guide
---
# How to use the pattern language

Direction from the owner, 2026-10-16. Modeled on Christopher Alexander's A Pattern Language: small named rules that link from large to small.

1. Before any design decision (a new object, view, key, command, prompt, link, or action) open [[index]] and read the patterns that apply. Follow links to larger and smaller patterns. Follow [[decision-cascade]] to find the answer to or properly frame your question.
2. Follow the pattern. Never build a per-object copy of an action. See [[objects-first]] and [[one-verb-set]].
3. When researching a convention, start from [[inspirations]] and check what we already know about each source. If no pattern covers the question, or code disagrees with a pattern, follow [[surface-gaps-before-deciding]]: research the convention in our inspirations and in software practice, then report the gap to the user with a recommendation. Do not decide alone.
4. After the user decides, add or amend one atomic note, link it to its larger and smaller patterns, add it to [[index]], log research in DESIGN-PATTERNS.md, and update [[open-discrepancies]].
5. One note holds one idea. Split a note if it needs an "and". Use wikilinks. Note names are the idea in plain words.
6. Note format: Problem, Therefore, Because, Larger patterns, Smaller patterns, Sources.
7. Status `decided` means the owner accepted it. `decided, owner to confirm` means adopted by an agent and not yet confirmed.
8. "%%{NOTE}%%" are notes provided by the user when they are directly interacting with this pattern language. Please address their requests and clarify with the user in chat if needed.
9. You are responsible for maintaining this pattern language as current and correct to spec. Make formatting corrections and fill material gaps as you find them.