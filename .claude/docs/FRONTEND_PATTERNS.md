# Frontend Patterns (FE rules)

The canonical rule contract for changes under `site/src/`. Each rule has a
stable ID (FE1 to FE10) so code review comments, agent guidance, and tooling
can all reference the same rule. The rules are ordered by how often reviewers
flagged violations in past frontend PRs.

How to use this document:

- Writing code: treat every rule as a default requirement, not a suggestion.
- Reviewing: cite rule IDs in comments (for example, "FE7: re-typed query key").
- Disagreeing: propose a change to this file instead of silently deviating.

`site/AGENTS.md` holds the one-line summary of each rule plus general frontend
workflow guidance. This file is the authoritative version with examples.

## FE1: UI behavior ships with Storybook interaction coverage

Every user-visible behavior change needs a Storybook story whose `play`
function actually exercises the interaction. Jest/RTL tests are for pure logic
(helpers, hooks without DOM interaction), not for UI interactions.

- The story must perform the real interaction: open the dropdown, submit the
  form, pin the mobile viewport. A story that renders a closed popover tests
  nothing.
- Cover the meaningful branches: error, empty, disabled, and mobile states,
  not only the happy path.
- Assert both sides of an invariant: the item that changed and a neighboring
  item that must not change.

**Incorrect (interaction test in Jest/RTL):**

```tsx
// ModelSelector.test.tsx
it("selects a model", async () => {
  render(<ModelSelector {...props} />);
  await userEvent.click(screen.getByRole("button"));
  // ...
});
```

**Correct (Storybook story with a play function):**

```tsx
// ModelSelector.stories.tsx
export const SelectModel: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: /model/i }));
    await expect(canvas.getByRole("listbox")).toBeVisible();
  },
};
```

## FE2: No loose types

- Never use `any`, `as unknown as X`, or non-null assertions in any form
  (`x!.y`, `items[0]!`, `fn()!`, `value! as T`).
- Prefer type annotations and narrowing over `as` casts. If types do not
  align, fix them at the source.
- Use generated types from `api/typesGenerated.ts` for all API data. Never
  re-declare a type that the backend already generates.
- If a component requires a prop to function, make the prop required.

**Incorrect:**

```tsx
const config = data as unknown as ChatModelConfig;
```

**Correct:**

```tsx
import type { ChatModelConfig } from "api/typesGenerated";

const config: ChatModelConfig = parseConfig(data);
```

## FE3: Reuse before building, and keep PRs single-purpose

- Before writing a component, hook, or helper, search `site/src/components/`
  and sibling feature folders for an existing implementation. Reviewers have
  repeatedly found new files that were near-copies (or byte-identical copies)
  of existing ones.
- Use existing wrapped primitives (Combobox, dialogs, tables) instead of
  hand-assembling the underlying pieces they already wrap.
- Delete dead code and unreachable branches instead of carrying them along.
- Keep the PR scoped to one change. Move unrelated cleanups, renames, and
  drive-by refactors to separate PRs.

## FE4: Comments must earn their place

- Do not write comments that restate the identifier, the assertion, or the
  control flow below them. Reviewers flag these in nearly every AI-authored
  PR.
- Keep only comments that capture a non-obvious invariant, external
  constraint, or deliberate tradeoff, in 1 to 3 lines.
- Before pushing, re-read every comment your diff adds and delete the ones a
  reader would not need.

**Incorrect:**

```tsx
// Track whether the panel is open
const [isOpen, setIsOpen] = useState(false);
```

## FE5: Handle every UI state, and never clobber user state

Every view that renders server data must handle this matrix:

| State   | Requirement                                                  |
|---------|--------------------------------------------------------------|
| Loading | Show a skeleton or spinner, never a blank or half-valid view |
| Error   | Surface the actionable server error, not a generic message   |
| Empty   | Deliberate empty state with copy, never a blank region       |
| Refetch | Keep showing valid data; never reset forms or selections     |

- A background refetch must not reinitialize form state or discard in-progress
  edits.
- When a mutation partially fails, the UI must reflect what succeeded and what
  did not (see FE7 for cache invalidation).
- Render a visible fallback ("Untitled", "N/A") for nullable display data.

## FE6: Accessibility is behavior, not decoration

- Every interactive element must be keyboard-reachable, including the way to
  discover why a control is disabled.
- The accessible name must contain the visible label text. Do not replace a
  trigger's name with an unrelated `aria-label` (a "Label in Name" violation).
- Check what the primitive does with `aria-*` props before setting them; some
  (for example cmdk) silently overwrite `role` and `aria-selected`.
- Preserve focus position across dialogs and route transitions.
- When visually hiding an interactive element, also remove it from the tab
  order and accessibility tree, or conditionally render it out of the DOM.

## FE7: React Query discipline

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

**Incorrect (re-typed query key in a story):**

```tsx
parameters: {
  queries: [{ key: ["chat-model-configs"], data: [MockChatModelConfig] }],
},
```

**Correct (imported constant):**

```tsx
import { chatModelConfigsKey } from "api/queries/chats";

parameters: {
  queries: [{ key: chatModelConfigsKey, data: [MockChatModelConfig] }],
},
```

## FE8: Effects are a last resort

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

## FE9: Fixtures and mocks follow repo conventions

- Represent entities with shared `Mock*` constants in `site/src/testHelpers/`
  (for example `MockChatModelConfig` in `testHelpers/chatModels.ts`). When a
  story needs a variant, spread the base fixture into a named local constant.
- Compose story query wiring (`{ key, data }`) inline per story so each story
  is readable on its own. Share the entity fixture, not a pre-wired query
  object.
- Query keys in mocks follow FE7: import the constant.

## FE10: Tests assert observable behavior

- Query by semantic role and accessible name (`getByRole`, `getByLabelText`).
  This tests accessibility (FE6) for free.
- Never use `querySelector`, class-name substring matches
  (`[class*='flex-col']`), or DOM-geometry assertions. They break silently on
  refactors without any user-visible regression.
- Use `data-testid` only when an element has no semantic role or name.
- Keep tests deterministic: no `behavior: "smooth"` scrolling, explicit
  locales for `toLocaleString()`, and time passed in as a prop or mock.
