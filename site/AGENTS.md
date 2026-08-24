# Frontend Development Guidelines

Read [Frontend Patterns](../.claude/docs/FRONTEND_PATTERNS.md) before changing `site/src/`. It is the canonical contract for FE1 through FE10.

## Frontend contract

- **FE1:** UI behavior changes ship with Storybook stories whose `play` function exercises the real interaction. Vitest or RTL is for pure logic only.
- **FE2:** No `any`, `as unknown as`, or avoidable casts. Use generated API types from `api/typesGenerated.ts`.
- **FE3:** Search for an existing component or helper before writing one. Keep changes single-purpose.
- **FE4:** Do not add comments that restate identifiers, assertions, or control flow.
- **FE5:** Views handle loading, error, empty, and refetch states without clobbering user state.
- **FE6:** Interactive elements remain keyboard reachable and have correct accessible names.
- **FE7:** Use React Query for server data. Import query key constants instead of retyping string literals.
- **FE8:** Use `useEffect` only to synchronize with external systems. Do not derive state or chain fetches in effects.
- **FE9:** Share entity fixtures as `Mock*` constants. Compose story query wiring inline per story.
- **FE10:** Tests query semantic roles and names. Do not use `querySelector` or class-name assertions.

## Navigation and commands

Use the TypeScript language server when available for definitions, references, type information, diagnostics, and renames.

| Task             | Command                                                 |
|------------------|---------------------------------------------------------|
| Develop          | `pnpm dev`                                              |
| Storybook        | `pnpm storybook --no-open`                              |
| Story tests      | `pnpm test:storybook`                                   |
| One story file   | `pnpm test:storybook src/path/to/component.stories.tsx` |
| Unit tests       | `pnpm test`                                             |
| One unit file    | `pnpm test path/to/file.test.ts`                        |
| Typecheck        | `pnpm lint:types`                                       |
| Biome check      | `pnpm check`                                            |
| Lint             | `pnpm lint`                                             |
| Fix lint         | `pnpm lint:fix`                                         |
| Format           | `pnpm format`                                           |
| End-to-end tests | `pnpm playwright:test`                                  |

Some end-to-end tests require a license. The Storybook MCP at `http://localhost:6006/mcp` requires Storybook to be running.

## Components and styling

- Use existing shadcn components and Tailwind CSS. MUI and Emotion have been removed; do not reintroduce them.
- Search `site/src/components/` and nearby feature code before creating a component or helper.
- Add shadcn components manually. Do not use the shadcn CLI.
- Keep business-specific components in feature modules. Create shared components only when reuse is established.
- Changes to core components are cross-cutting. Coordinate visual or API expansion with design when needed.
- Keep component files near 500 lines or less. Extract coherent sections when a file becomes difficult to navigate.
- Use semantic theme colors and existing Tailwind tokens. Do not use the `dark:` prefix.

## TypeScript and data flow

- Use ES modules and Biome. Prefer `for...of` over `forEach`.
- Access browser globals directly. This is a client-only SPA, so do not guard them with `typeof window` or similar checks.
- Components must not call API functions directly. Use established React Query definitions and key hierarchies.
- Use `mutate()` with callbacks when the result does not need to be awaited. Do not swallow mutation failures in empty catches.
- Prefer generated types, annotations, guards, and upstream type fixes over assertions or non-null assertions.
- Match errors by code or HTTP status, not message text.
- Match patterns in the same file before introducing a new convention.
- Avoid single-use wrappers, aliases, constants, and hooks that add navigation without adding meaning.
- Use JSX shorthand for boolean props whose value is `true`.

## Accessibility and robustness

- Every table has an `aria-label` or caption.
- Elements with `tabIndex={0}` have an appropriate semantic role.
- Visually hidden interactive elements must also leave the tab order and accessibility tree. Prefer conditional rendering.
- Render a visible fallback such as `Unknown` or `N/A` for missing user-facing text.
- Guard number conversions against `NaN` and non-finite values.
- Pass an explicit locale to `toLocaleString()` for deterministic output.
- Do not use em dashes, en dashes, or spaced double hyphens as punctuation.

## Testing

- Add or update Storybook stories for component and page behavior, visual states, keyboard interaction, focus, and accessibility.
- Assert observable behavior with semantic queries. Do not assert Tailwind classes or implementation details.
- Use `data-testid` only when an element has no suitable role or accessible name.
- Do not depend on smooth scrolling in tests. Use instant behavior or control the scroll position directly.
- Keep stories current when components change.

## Performance

- `src/pages/AgentsPage/`, including `components/ChatElements/`, uses React Compiler. Do not add `useMemo`, `useCallback`, or `memo()` there.
- `memo()` remains valid for list-item components rendered in a map because compiler memoization does not cross component boundaries.
- Isolate frequently changing state in a small child component instead of rerendering a large parent subtree.
- Throttle high-frequency handlers that set state with `requestAnimationFrame` or an established throttle utility.

## Completion

- Run targeted story or unit tests during iteration.
- Visually inspect affected component stories before handoff.
- Before handoff, run `pnpm check`, `pnpm lint`, and `pnpm format`, plus affected tests.
- For changes under `site/src/`, run the repository `frontend-review` skill at `.claude/skills/frontend-review/SKILL.md` and fix each applicable FE1 through FE10 failure.
