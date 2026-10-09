# Tests and Stories (TS)

Lens: does the diff prove the changed UI behavior the way this repo proves it: a Storybook story per state whose play function drives the UI into that state for the visual snapshot, Vitest for pure logic and callback behavior, shared Mock* fixtures, and no test scaffolding leaking into production code.

Boundaries: exporting or renaming query key constants in api/queries belongs to DF; near-copies of existing components inside story files belong to RO; single-use constants and needless helpers outside test files belong to SD; comments inside tests and stories belong to CD.

Reading order: open every *.stories.tsx and *.test.tsx in the diff first, and read each story file whole (not just the hunks) so module-level state, helper components, and local fixtures are visible. Then open the component under test to list its real states, check site/src/testHelpers for an existing Mock* fixture before accepting a new local one, and open the matching api/queries module to confirm the key constant a story wires against.

## Checkpoints

### TS1: Play sets up one state for the snapshot and stops there

Default severity: should-fix
FE parent: FE1
Evidence: 31 comments across 13 PRs

Look for:
- A play function whose last statements are `expect(...).toBeVisible()`, `toBeInTheDocument()`, `not.toBeInTheDocument()`, `toBeNull()`, or `toHaveAttribute` on rendered content: the snapshot already captures this, so the assertions are noise.
- A play that drives the UI into a state, asserts it, then drives it into a second state: split it into one story per state.
- A story whose play only checks presence and performs no interaction, or a `findBy*` used as a disguised content assertion after the setup step.
- The changed behavior has no story at all, or the story renders the closed or default state without opening the popover, submitting the form, or paginating.

Fix: keep the interactions (click, type, open) that put the component in the state to snapshot, delete the assertions, and add a separate story for each additional state. If the play only asserted presence, remove the play entirely.

### TS2: Assertions target observable behavior through standard queries

Default severity: should-fix
FE parent: FE10
Evidence: 11 comments across 7 PRs

Look for:
- `tagName`, `getBoundingClientRect`, `scrollWidth`/`clientWidth`, `getAllByRole(...).toHaveLength`, or a storage key exported from the component so a test can read it: these test the implementation, not what the user sees.
- `within(canvasElement.ownerDocument.body)` or `within(document.body)` to reach portal content: import and use `screen`.
- `waitFor(() => expect(x.getByText(...)).toBeVisible())` replacing an existing `await findByText(...)`: same assertion dressed up, no behavior change.
- A bare `await findByRole(...)` whose purpose (waiting versus asserting existence) is not clear from the surrounding steps.

Fix: query by role and accessible name with `screen` or `canvas`, use `findBy*` when the intent is to wait, and drop assertions on DOM structure or geometry; if the underlying layout matters, it belongs in the snapshot.

### TS3: Fixtures are shared Mock* constants, variants spread the base

Default severity: should-fix
FE parent: FE9
Evidence: 23 comments across 13 PRs

Look for:
- A local entity constant in a story or test named without the `mock` prefix (`defaultChat`, `parentChat`, `secondTemplate`, `slackMCPAlwaysOnNeedingAuth`): rename to `mockX`. Pre-existing unprefixed neighbours in the same file (`sentryMCP`, `linearMCP`) do not excuse the new one; flag only the new fixture and do not ask for the old ones to be renamed.
- A factory function (`makeCostControl(overrides)`, `buildMCPServer(...)`, `memberWithSpend(...)`) where a `mockX` constant plus `{ ...mockX, field: value }` at the call site would do.
- A local fixture that duplicates an existing `Mock*` from site/src/testHelpers with no meaningful difference: use the shared one.
- Real production domains or hostnames in fixture values: use `*.example.com`.

Fix: define one `mockX` constant per entity (spreading the shared `MockX` when one exists), override fields inline at each call site, and delete the factory.

### TS4: Keep test and story files plain, and keep test code out of production

Default severity: should-fix
FE parent: FE1
Evidence: 17 comments across 10 PRs

Look for:
- Helper or stateful wrapper components defined inside a *.stories.tsx, especially ones that re-create an existing Input, Select, or Label.
- Test helpers built from regexes, derived class lists, or `JSON.parse(JSON.stringify(...))` where a hard-coded value or `structuredClone` reads plainly.
- Single-use constants in a story file (`const chatId = MockChat.id`) and tests that only move in the diff without changing.
- `_resetXForTesting` or a tiny wrapper exported from the implementation module solely for tests, and tests that exercise that helper or a library (DOMPurify, backend filtering) rather than frontend behavior.

Fix: render the real components with args, hard-code test inputs, inline single-use values, move test-only helpers into a separate test-support file or delete them, and delete tests that have no frontend behavior to check.

### TS5: Callback and logic checks go to Vitest, rendered UI goes to Storybook

Default severity: should-fix
FE parent: FE1
Evidence: 9 comments across 6 PRs

Look for:
- `expect(onX).toHaveBeenCalledWith(...)`, `mockClear()`, or `navigator.clipboard.writeText` spies inside a story play: this is a behavior test, not a visual baseline.
- A story that exists only to check an attribute or a non-visual side effect (`data-bwignore`, a persisted value).
- A *.test.tsx that renders a component and asserts on rendered text or presence: the visual state belongs in a story.

Fix: move callback and side-effect assertions into a Vitest test of the handler or hook, and move rendered-state checks into a story without assertions.

### TS6: Mock at the query boundary with exported keys, wired inline per story

Default severity: should-fix
FE parent: FE9
Evidence: 15 comments across 11 PRs

Look for:
- A shared pre-wired query object or helper (`spawnModelConfigsQuery`, `aiSpendQuery(overrides)`) instead of a `mockX` data constant plus an inline `{ key, data }` in each story's `parameters.queries`.
- A key built as a string literal (`["chat-model-configs"]`) or by constructing query options only to read `.queryKey` (`mcpServerConfigs(id).queryKey`): import the exported key constant.
- `spyOn(API, ...)` where `parameters.queries` covers the same data, and a module-level `let capturedQueryClient` or similar used to reach into the cache from a story.
- Empty wiring left behind (`queries: []`).

Fix: share the entity fixture, wire `{ key: exportedKey, data: mockX }` inline per story, delete cache-capturing module state, and prefer `parameters.queries` over spies unless the story needs a pending or failing request.

### TS7: Stories model the real state space, one story per reachable state

Default severity: should-fix
FE parent: FE1
Evidence: 4 comments across 3 PRs

Look for:
- A story whose mocked data cannot occur in production (an authorization combination the backend never returns).
- A new story that adds no state the existing stories do not already show.
- A missing story for a reachable branch the component handles (filtered results empty while data exists, error, disabled, mobile).
- Stories for a code path the PR series is about to delete.

Fix: add one story per reachable meaningful state, remove impossible and duplicate stories, and skip stories for behavior scheduled for removal.

## Not this role

- Exporting a query key constant or key helper from api/queries so tests can import it: DF owns the export; TS only flags the story that re-types or rebuilds the key.
- Story files that re-create Input, Select, or Label components the repo already has: RO owns the "we already have this" finding.
- Single-use constants in story files: SD owns inlining; TS keeps them only as story hygiene evidence.
- Whether the no-permission empty state should take priority over the filter empty state: DF owns the state-matrix decision; TS only asks for the story.
- A tiny production wrapper exported for tests: SD owns removing the wrapper; TS owns the pointless test around it.
