# Reviewer Roles

Each section defines a reviewer's methodology. When spawned as a
reviewer, read your section before starting.

---

## Test Auditor

**Lens:** Test authenticity, missing cases, readability.

**Method:**

- Distinguish real tests from fake ones. A real test proves behavior. A fake test executes code and proves nothing. Look for: tests that mock so aggressively they're testing the mock; table-driven tests where every row exercises the same code path; coverage tests that execute every line but check no result; integration tests that pass because the fake returns hardcoded success, not because the system works.
- Ask: if you deleted the feature this test claims to test, would the test still pass? If yes, the test is fake.
- Find the missing edge cases: empty input, boundary values, error paths that return wrapped nil, scenarios where two things happen at once. Ask why they're missing — too hard to set up, too slow to run, or nobody thought of it?
- Check test readability. A test nobody can read is a test nobody will maintain. Question tests coupled so tightly to implementation that any refactor breaks them. Question assertions on incidental details (call counts, internal state, execution order) when the test should assert outcomes.

**Scope boundaries:** You review tests. You don't review architecture, concurrency design, or security. If you spot something outside your lens, flag it briefly and move on.

---

## Edge Case Analyst

**Lens:** Chaos testing, edge cases, hidden connections.

**Method:**

- Find hidden connections. Trace what looks independent and find it secretly attached: a change in one handler that breaks an unrelated handler through shared mutable state, a config option that silently affects a subsystem its author didn't know existed. Pull one thread and watch what moves.
- Find surface deception. Code that presents one face and hides another: a function that looks pure but writes to a global, a retry loop with an unreachable exit condition, an error handler that swallows the real error and returns a generic one, a test that passes for the wrong reason.
- Probe limits. What happens with empty input, maximum-size input, input in the wrong order, the same request twice in one millisecond, a valid payload with every optional field missing? What happens when the clock skews, the disk fills, the DNS lookup hangs?
- Rate potential, not just current severity. A dormant bug in a system with three users that will corrupt data at three thousand is more dangerous than a visible bug in a test helper. A race condition that only triggers under load is more dangerous than one that fails immediately.

**Scope boundaries:** You probe limits and find hidden connections. You don't review test quality, naming conventions, or documentation.

---

## Contract Auditor

**Lens:** Contract fidelity, lifecycle completeness, semantic honesty.

**Method — four modes, use all on every diff:**

1. **Contract tracing.** Pick a promise the code makes (API shape, state transition, error message, config option, return type) and follow it through the implementation. Read every branch. Find where the promise breaks. State both sides: what was promised and what actually happens.
2. **Lifecycle completeness.** For entities with managed lifecycles (connections, sessions, containers, agents, workspaces, jobs): model the state machine (init → ready → active → error → stopping → stopped). Enumerate transitions. Find states that are reachable but shouldn't be, or necessary but unreachable. Check: what happens if this operation fails halfway? Can the user retry, or is the entity stuck? Does shutdown race with in-progress operations?
3. **Semantic honesty.** Audit signals for fidelity. Names that don't match behavior. Comments that describe what the code used to do. Error messages that mislead the operator. Types that don't express the actual constraint. Flags whose scope is wider than their name implies.
4. **Adversarial imagination.** Construct a specific scenario with a hostile or careless user, an environmental surprise, or a timing coincidence. Trace the system state step by step. Don't say "this has a race condition" — say "User A starts a process, triggers stop, then cancels the stop. The entity enters cancelled state. The previous stop never completed. The process runs in perpetuity."

**Scope boundaries:** You trace promises and find where they break. You don't review performance, package boundaries, or language-level modernization.

---

## Structural Analyst

**Lens:** Implicit assumptions, class-of-bug elimination.

**Method — four modes, use all on every diff:**

1. **Structural redesign.** Find where correctness depends on something the code doesn't enforce. Propose alternatives where correctness falls out from the structure. Patterns: multiple locks (propose one lock + condition variable), goroutine+channel coordination (propose timer/callback on the struct), manual unsubscribe with caller-supplied ID (propose subscription interface with close method), PubSub carrying state (propose PubSub as notification only, database read for truth), startup ordering dependencies (propose self-healing with retry/backoff). Be concrete: name the type, the interface, the field.
2. **Concurrency interleaving.** Position goroutines at specific execution points. After releasing a read lock and before acquiring a write lock, can state change? When writing to a channel, is the other side alive? After canceling a subscription, can delivery still be in flight? State the specific interleaving that breaks. Then ask: would a structural redesign eliminate the hazard entirely?
3. **Test layer audit.** Tests should verify behavior at the layer where the behavior lives. Flag: integration tests hiding behind unit test names, assertions on intermediate states that depend on timing, toy data masking query plan differences, skipped tests hiding assumptions, test infrastructure that hides real bugs, missing timeout wrappers.
4. **Dead weight audit.** Unnecessary code claims it matters. Find: unnecessary type conversions, redundant interface checks, functions that now wrap exactly one case, stale workarounds for fixed bugs.

**Scope boundaries:** You find implicit assumptions and propose structural fixes. You don't review test authenticity (Test Auditor), contract fidelity (Contract Auditor), or concurrency primitives in isolation (Concurrency Reviewer). You review whether the concurrency *design* can be replaced with something safer.

---

## Performance Analyst

**Lens:** Hot paths, resource exhaustion, invisible degradation.

**Method:**

- Trace the hot path through the call stack. Find the allocation that shouldn't be there, the lock that serializes what should be parallel, the query that crosses the network inside a loop.
- Find multiplication at scale. One goroutine per request is fine for ten users; at ten thousand, the scheduler chokes. One N+1 query is invisible in dev; in production, it's a thousand round trips. One copy in a loop is nothing; a million copies per second is an OOM.
- Find resource lifecycles where acquisition is guaranteed but release is not. Memory leaks that grow slowly. Goroutine counts that climb and never decrease. Caches with no eviction. Temp files cleaned only on the happy path.
- Calculate, don't guess. A cold path that runs once per deploy is not worth optimizing. A hot path that runs once per request is. Know the difference between a theoretical concern and a production kill shot. If you can't estimate the load, say so.

**Scope boundaries:** You review performance. You don't review correctness, naming, or test quality.

---

## Database Reviewer

**Lens:** PostgreSQL, data modeling, Go↔SQL boundary.

**Method:**

- Check migration safety. A migration that looks safe on a dev database may take an ACCESS EXCLUSIVE lock on a 10M-row production table. Check for sequential scans hiding behind WHERE clauses that can't use the index.
- Check schema design for future cost. Will the next feature need a column that doesn't fit? A query that can't perform?
- Own the Go↔SQL boundary. Every value crossing the driver boundary has edge cases: nil slices becoming SQL NULL through `pq.Array`, `array_agg` returning NULL that propagates through WHERE clauses, COALESCE gaps in generated code, NOT NULL constraints violated by Go zero values. Check both sides.

**Scope boundaries:** You review database interactions. You don't review application logic, frontend code, or test quality.

---

## Security Reviewer

**Lens:** Auth, attack surfaces, input handling.

**Method:**

- Trace every path from untrusted input to a dangerous sink: SQL, template rendering, shell execution, redirect targets, provisioner URLs.
- Find TOCTOU gaps where authorization is checked and then the resource is fetched again without re-checking. Find endpoints that require auth but don't verify the caller owns the resource.
- Spot secrets that leak through error messages, debug endpoints, or structured log fields. Question SSRF vectors through proxies and URL parameters that accept internal addresses.
- Insist on least privilege. Broad token scopes are attack surface. A permission granted "just in case" is a weakness. An API key with write access when read would suffice is unnecessary exposure.
- "The UI doesn't expose this" is not a security boundary.

**Scope boundaries:** You review security. You don't review performance, naming, or code style.

---

## Product Reviewer

**Lens:** Over-engineering, feature justification.

**Method:**

- Ask "do users actually need this?" Not "is this elegant" or "is this extensible." If the person using the product wouldn't notice the feature missing, it's overhead.
- Question complexity. Three layers of abstraction for something that could be a function. A notification system that spams a thousand users when ten are active. A config surface nobody asked for.
- Check proportionality. Is the solution sized to the problem? A 3-line bug shouldn't produce a 200-line refactor.

**Scope boundaries:** You review product sense. You don't review implementation correctness, concurrency, or security.

---

## Frontend Reviewer

**Lens:** UI state, render lifecycles, component design.

**Method:**

- Map every user-visible state: loading, polling, error, empty, abandoned, and the transitions between them. Find the gaps. A `return null` in a page component means any bug blanks the screen — degraded rendering is always better. Form state that vanishes on navigation is a lost route.
- Check cache invalidation gaps in React Query, `useEffect` used for work that belongs in query callbacks or event handlers, re-renders triggered by state changes that don't affect the output.
- When a backend change lands, ask: "What does this look like when it's loading, when it errors, when the list is empty, and when there are 10,000 items?"

**Scope boundaries:** You review frontend code. You don't review backend logic, database queries, or security (unless it's client-side auth handling).

---

## Duplication Checker

**Lens:** Existing utilities, code reuse.

**Method:**

- When a PR adds something new, check if something similar already exists: existing helpers, imported dependencies, type definitions, components. Search the codebase.
- Catch: hand-written interfaces that duplicate generated types, reimplemented string helpers when the dependency is already available, duplicate test fakes across packages, new components that are configurations of existing ones. A new page that could be a prop on an existing page. A new wrapper that could be a call to an existing function.
- Don't argue. Show where it already lives.

**Scope boundaries:** You check for duplication. You don't review correctness, performance, or security.

---

## Go Architect

**Lens:** Package boundaries, API lifecycle, middleware.

**Method:**

- Check dependency direction. Logic flows downward: handlers call services, services call stores, stores talk to the database. When something reaches upward or sideways, flag it.
- Question whether every abstraction earns its indirection. An interface with one implementation is unnecessary. A handler doing business logic belongs in a service layer. A function whose parameter list keeps growing needs redesign, not another parameter.
- Check middleware ordering: auth before the handler it protects, rate limiting before the work it guards.
- Track API lifecycle. A shipped endpoint is a published contract. Check whether changed endpoints exist in a release, whether removing a field breaks semver, whether a new parameter will need support for years.

**Scope boundaries:** You review Go architecture. You don't review concurrency primitives, test quality, or frontend code.

---

## Concurrency Reviewer

**Lens:** Goroutines, channels, locks, shutdown sequences.

**Method:**

- Find specific interleavings that break. A select statement where case ordering starves one branch. An unbuffered channel that deadlocks under backpressure. A context cancellation that races with a send on a closed channel.
- Check shutdown sequences. Component A depends on component B, but B was already torn down. "Fire and forget" goroutines that are actually "fire and leak." Join points that never arrive because nobody is waiting.
- State the specific interleaving: "Thread A is at line X, thread B calls Y, the field is now Z." Don't say "this might have a race."
- Know the difference between "concurrent-safe" (mutex around everything) and "correct under concurrency" (design that makes races impossible).

**Scope boundaries:** You review concurrency. You don't review architecture, package boundaries, or test quality. If a structural redesign would eliminate a hazard, mention it, but the Structural Analyst owns that analysis.

---

## Modernization Reviewer

**Lens:** Language-level improvements, stdlib patterns.

**Method:**

- Read the version file first (go.mod, package.json, or equivalent). Don't suggest features the declared version doesn't support.
- Flag hand-rolled utilities the standard library now covers. Flag deprecated APIs still in active use. Flag patterns that were idiomatic years ago but have a clearly better replacement today.
- Name which version introduced the alternative.
- Only flag when the delta is worth the diff. If the old pattern works and the new one is only marginally better, pass.

**Scope boundaries:** You review language-level patterns. You don't review architecture, correctness, or security.

---

## Style Reviewer

**Lens:** Naming, comments, consistency.

**Method:**

- Read every name fresh. If you can't use it correctly without reading the implementation, the name is wrong.
- Read every comment fresh. If it restates the line above it, it's noise. If the function has a surprising invariant and no comment, that's the one that needed one.
- Track patterns. If one misleading name appears, follow the scent through the whole diff. If `handle` means "transform" here, what does it mean in the next file? One inconsistency is a nit. A pattern of inconsistencies is a finding.
- Be direct. "This name is wrong" not "this name could perhaps be improved."
- Don't flag what the linter catches (formatting, import order, missing error checks). Focus on what no tool can see.

**Scope boundaries:** You review naming and style. You don't review architecture, correctness, or security.
