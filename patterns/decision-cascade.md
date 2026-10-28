(ADAPT TO PROPER NOTATION FORMATTING)

Problem
Coding agents don't seek out resources that could answer an implementation decision before they approach the user. This means the coding agent can bring a poor set of general choices to the user, wasting their time and creating a sub-par product. A pitfall of just picking one resource is there are resources that are closer to the spirit of the individual project. 

Therefore
Research things in relation to distance from this particular project. Starting with further, general best practices, and if those don't answer the question move closer until you approach the user with the highest-impact, most particular needs for clarification. If a more generic implementation further away does not solve the problem adequately, we must approach closer to the user:

[FURTHEST]
1. General coding best practices/patterns (these are probably embedded in the coding agent's training already)
2. Product software type best practices and standardized data structures (ISO, schema.org, HTML, JSON, Python, etc)
3. [[inspirations]] directly labeled for this project. You can research their online documentation, use '/REA' skill for reverse engineering the feature's code to study their implementation, or research feature lists on their website. Because the user is not going to spell out every single particularity of feature implementation, we can stand on the quality work of past projects.
4. [This pattern language](how-to-use) is the atomic specification of past decisions the user decided to encode into this particular project's design direction.
5. [The user] is the final arbiter to bring discrepancies and implementation issues to when all other resources fail. Responses from the user that are long-standing and generalizable should be encoded back into this pattern language.
[CLOSEST]

Because
The best implementation is one appropriately scoped for the level of problem. A routine implementation of a for loop is well documented enough to not need the user's time to instantiate, while larger wide-swinging decisions like decision direction, user experience, platform decisions, and business direction require more input from the user. 

Over time as discrepancies are resolved and the pattern language is built out, less decisions should need direct arbitration from the user until 99.9% of decisions are made by well-instantiated specifications.

Larger patterns
- [[borrow-before-inventing]]
Smaller patterns

Sources