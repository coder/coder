---
name: frontend
description: Canonical frontend guidelines and the FE1-FE10 pattern rules for Coder's web UI. Load before reading, editing, creating, or reviewing any file under site/ (React, TypeScript, Storybook, Tailwind, Biome). Detailed code examples and the full workflow guide load on demand from references/.
---

# Frontend Development

Canonical guidance for Coder's web UI under `site/`. Load this skill as soon as
a task touches any file under `site/`, before reading or editing code, so the
frontend rules are in context while you write.

This skill is the source of truth for frontend rules and conventions.
`site/AGENTS.md` is a stub that points here.

How to use these rules:

- Writing code: treat every rule as a default requirement, not a suggestion.
- Reviewing: cite rule IDs in comments (for example, "FE7: re-typed query key").
- Disagreeing: propose a change to this skill instead of silently deviating.

## When to load

- Before reading, editing, or creating any file under `site/` (including
  `site/src/`, `site/e2e/`, stories, and config).
- Before reviewing a diff that touches `site/`.
- Skip only when no `site/` files are involved.

## On-demand references

Load these with `read_skill_file` when you need them:

- `references/patterns.md` - correct/incorrect code examples for each FE rule.
- `references/self-review.md` - the deterministic FE1 to FE10 diff-audit gate to
  run before opening a PR or when asked to review frontend changes.
- `references/frontend-guide.md` - commands, TypeScript LSP navigation,
  components, styling, robustness, performance, workflow, and the full React
  rules.

## Non-negotiables (most important first)

These are the rules reviewers flag most. The full contract is below; internalize
these first.

1. **Reuse before building (FE3).** This is the most important rule. Before
   writing any component, hook, or helper, search `site/src/components/` and
   sibling feature folders for an existing implementation, and use existing
   wrapped primitives (Combobox, dialogs, tables) instead of hand-assembling
   what they already wrap. Duplicating existing code is the single most common
   review rejection.
2. **No loose types (FE2).** No `any`, no `as unknown as X`, no non-null
   assertions. Use generated types from `api/typesGenerated.ts`.
3. **React Query for all server data (FE7).** Never call an `API` function or
   `fetch` in a component; never manage server-data lifecycle with
   `useState` + `useEffect`. Import query key constants; never re-type them.
4. **Effects are a last resort (FE8).** `useEffect` only synchronizes with an
   external system. Never derive state or chain fetches in effects.
5. **Handle every UI state (FE5).** Loading, error, empty, and refetch, without
   clobbering user state.
6. **Accessibility is behavior (FE6).** Keyboard-reachable, correct accessible
   names.
7. **UI behavior ships with Storybook interaction coverage (FE1).** Vitest/RTL is
   for pure logic only.

## The FE rules (FE1 to FE10)

The rules are ordered by how often reviewers flagged violations. Each has a
stable ID so review comments, agent guidance, and tooling reference the same
rule. Code examples live in `references/patterns.md`.

### FE1: UI behavior ships with Storybook interaction coverage

Every user-visible behavior change needs a Storybook story whose `play`
function actually exercises the interaction. Vitest/RTL tests are for pure logic
(helpers, hooks without DOM interaction), not for UI interactions.

- The story must perform the real interaction: open the dropdown, submit the
  form, pin the mobile viewport. A story that renders a closed popover tests
  nothing.
- Cover the meaningful branches: error, empty, disabled, and mobile states,
  not only the happy path.
- Assert both sides of an invariant: the item that changed and a neighboring
  item that must not change.
- When a component depends on the current time or date, accept it as a prop or
  via context instead of reading `new Date()` or `Date.now()` internally, so
  stories render deterministically without mocking globals.

### FE2: No loose types

- Never use `any`, `as unknown as X`, or non-null assertions in any form
  (`x!.y`, `items[0]!`, `fn()!`, `value! as T`).
- Prefer type annotations and narrowing over `as` casts. If types do not
  align, fix them at the source.
- Use generated types from `api/typesGenerated.ts` for all API data. Never
  re-declare a type that the backend already generates.
- If a component requires a prop to function, make the prop required.
- Avoid `@ts-ignore` and `biome-ignore` suppression comments. Seek a
  better-typed alternative first, and document why when one is unavoidable.

### FE3: Reuse before building, and keep PRs single-purpose

- Before writing a component, hook, or helper, search `site/src/components/`
  and sibling feature folders for an existing implementation. Reviewers have
  repeatedly found new files that were near-copies (or byte-identical copies)
  of existing ones.
- Use existing wrapped primitives (Combobox, dialogs, tables) instead of
  hand-assembling the underlying pieces they already wrap.
- Delete dead code and unreachable branches instead of carrying them along.
- Keep the PR scoped to one change. Move unrelated cleanups, renames, and
  drive-by refactors to separate PRs.

### FE4: Comments must earn their place

- Do not write comments that restate the identifier, the assertion, or the
  control flow below them. Reviewers flag these in nearly every AI-authored PR.
- Keep only comments that capture a non-obvious invariant, external
  constraint, or deliberate tradeoff, in 1 to 3 lines.
- Before pushing, re-read every comment your diff adds and delete the ones a
  reader would not need.

### FE5: Handle every UI state, and never clobber user state

Every view that renders server data must handle four states:

- Loading: show a skeleton or spinner, never a blank or half-valid view.
- Error: surface the actionable server error, not a generic message.
- Empty: a deliberate empty state with copy, never a blank region.
- Refetch: keep showing valid data; never reset forms or selections.

- A background refetch must not reinitialize form state or discard in-progress
  edits.
- When a mutation partially fails, the UI must reflect what succeeded and what
  did not (see FE7 for cache invalidation).
- Render a visible fallback ("Untitled", "N/A") for nullable display data.
- Never use `key={String(booleanState)}` to force a remount. When the boolean
  flips, React synchronously unmounts and remounts the subtree, discarding its
  state and killing exit animations.

### FE6: Accessibility is behavior, not decoration

- Every interactive element must be keyboard-reachable, including the way to
  discover why a control is disabled.
- The accessible name must contain the visible label text. Do not replace a
  trigger's name with an unrelated `aria-label` (a "Label in Name" violation).
- Check what the primitive does with `aria-*` props before setting them; some
  (for example cmdk) silently overwrite `role` and `aria-selected`.
- Preserve focus position across dialogs and route transitions.
- When visually hiding an interactive element, also remove it from the tab
  order and accessibility tree, or conditionally render it out of the DOM.
- Generate IDs for form elements, labels, and ARIA attributes with
  `React.useId()`. Hard-coded string IDs collide when a component renders more
  than once on a page.

### FE7: React Query discipline

All server data goes through react-query. Never call an `API` function or
`fetch` directly inside a component, and never manage server-data lifecycle
with `useState` + `useEffect`.

- Import query key constants from `api/queries/`. Never re-type a key as a
  string literal in a component, story, or test. If the constant is not
  exported, export it; do not copy the string.
- `isLoading` means no cached data yet; `isFetching` includes background
  refetches. Do not gate on both (`isLoading || isFetching` is just
  `isFetching`) and do not blank valid data during a background refetch.
- After a mutation, invalidate every affected query, including on partial
  failure paths (for example, when the second of two chained mutations fails).
- Use `mutate()` with `onSuccess`/`onError` callbacks unless you need the
  result for control flow. Never wrap `mutateAsync()` in a `try/catch` with an
  empty catch block.

### FE8: Effects are a last resort

Decide where logic goes before reaching for `useEffect`:

1. Can it be computed from props/state during render? Compute it in render
   (or `useMemo` if expensive). Do not mirror it into state.
2. Does it respond to a user action? Put it in the event handler.
3. Is it server data? Use a query or mutation (FE7).
4. Is it synchronizing with an external system (WebSocket, DOM API,
   subscription)? This is the only case for `useEffect`.

- Never write an effect that reads state A and calls `setStateB`; derive the
  value instead.
- Audit every dependency you add to an effect that owns a connection or
  triggers fetches. Past regressions include a dependency change that
  disconnected and reconnected the chat WebSocket on every message, and an
  effect on `isFetching` that caused an infinite fetch loop.
- Delete effects that only synchronize a ref nobody reads.
- **`useCallback` is an antipattern by default.** Do not memoize callbacks
  preemptively. Reach for `useCallback` only when a specific memoized child or
  hook dependency requires a stable reference, and confirm that need first. An
  unnecessary `useCallback` adds noise and a dependency array to maintain
  without a measured benefit. The same caution applies to `useMemo`.

### FE9: Fixtures and mocks follow repo conventions

- Represent entities with shared `Mock*` constants in `site/src/testHelpers/`
  (for example `MockChatModelConfig` in `testHelpers/chatModels.ts`). When a
  story needs a variant, spread the base fixture into a named local constant.
- Compose story query wiring (`{ key, data }`) inline per story so each story
  is readable on its own. Share the entity fixture, not a pre-wired query
  object.
- Query keys in mocks follow FE7: import the constant.

### FE10: Tests assert observable behavior

- Query by semantic role and accessible name (`getByRole`, `getByLabelText`).
  This tests accessibility (FE6) for free.
- Never use `querySelector`, class-name substring matches
  (`[class*='flex-col']`), or DOM-geometry assertions. They break silently on
  refactors without any user-visible regression.
- Use `data-testid` only when an element has no semantic role or name.
- Keep tests deterministic: no `behavior: "smooth"` scrolling, explicit
  locales for `toLocaleString()`, and time passed in as a prop or mock.

## Self-review before opening a PR

Run the standard checks: `pnpm check`, `pnpm lint`, `pnpm format`, and
`pnpm test` on affected tests (full pre-PR checklist in
`references/frontend-guide.md`).

When the diff touches `site/src/` (or `site/e2e/`), or when you are asked to
review frontend changes, load `references/self-review.md` with `read_skill_file`
and follow it exactly. It is a deterministic FE1 to FE10 diff audit that pins
the merge-base diff, emits a `PASS`/`FAIL file:line` verdict per rule, and
requires a rerun after fixes. Do not substitute a general code-review pass for
it.
