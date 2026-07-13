# Frontend Development Guidelines

Repo-specific rules for `site/`. Generic React and TypeScript knowledge
is assumed; this file lists the deltas this repo enforces.

Use the TypeScript LSP tools first when investigating code (definition,
references, hover, diagnostics, rename_symbol); do not manually search
files for symbols.

## Commands

- `pnpm dev` - Vite dev server
- `pnpm storybook --no-open` - Storybook dev server. Required before the
  Storybook MCP server (`http://localhost:6006/mcp`, configured in the
  repo root `.mcp.json`) can connect.
- `pnpm test` - Vitest unit tests (this repo uses Vitest, not Jest)
- `pnpm test -- path/to/specific.test.ts` - single unit test file
- `pnpm test:storybook` - story play-function tests via Vitest + Playwright
- `pnpm test:storybook src/path/to/component.stories.tsx` - single story file
- `pnpm lint` - full suite: Biome lint, TypeScript (`lint:types`),
  circular deps, React Compiler check, knip
- `pnpm check` - Biome lint and format checks only; it does NOT type
  check. Type checking runs through `pnpm lint`.
- `pnpm format` - Biome format; run before creating a PR
- `pnpm playwright:test` - e2e tests; remind the user a license is
  required to run all of them

Playwright failure artifacts land in `site/test-results/` (HTML report in
`site/playwright-report/`, coderd debug log in
`site/e2e/test-results/debug.log`). In CI, the `test-e2e` job uploads
artifacts named `playwright-artifacts-<matrix job>-<commit SHA>`.

## Components and styling

- Search for existing implementations before creating UI: shared
  primitives in `site/src/components/`, business-logic components in
  `site/src/modules/`, and sibling files for local helpers. Keep one-off
  variants of shared primitives local to your feature folder; promote
  them only when reuse and design justify it.
- Changing `site/src/components/` is cross-cutting: it affects every
  consumer, so coordinate with design before extending shared primitives.
- MUI is deprecated; use shadcn/ui-style components from
  `site/src/components` (add them manually, not via the shadcn CLI).
- Emotion CSS is deprecated; use Tailwind with the semantic tokens from
  `tailwind.config.js` (`content`, `surface`, `border`, `highlight`). No
  `dark:` prefix. The Tailwind reset stays disabled for MUI compatibility.
- Keep component files under ~500 lines; extract sub-components beyond that.

## Code style

- Biome handles linting and formatting (not ESLint/Prettier).
- ES modules, destructured imports, `for...of` over `forEach`.
- Access browser globals directly (`location.href`, not
  `window.location.href`); Coder is a pure SPA, so never add
  `typeof window` style runtime checks.
- Match existing patterns in the file you are editing before introducing
  new conventions (shared API helpers, state initialization style).
- Match errors by error code or HTTP status, never by message string.
- Use the JSX boolean shorthand (`<Foo prop />`, not `<Foo prop={true} />`).
- Do not add helpers, aliases, wrapper hooks, or components that only
  rename or forward a single operation; inline them at the call site.
  After refactors, delete helpers that collapsed to pass-throughs.
- Comments explain non-obvious behavior or invariants. Remove narration,
  restated signatures, and change-history notes.

## TypeScript

- Never use `as unknown as X` double assertions; fix types at the source.
- Prefer type guards and annotations over `as` casts; avoid the non-null
  assertion operator (`!.`).
- Use generated types from `src/api/typesGenerated.ts` for all API types;
  never re-declare them.
- If a component's implementation depends on a prop, make it required.
- Avoid `@ts-ignore` and lint suppressions; document why when unavoidable.

## Data and state

- Use React Query for all server state. Never call an `API` function
  directly from a rendered component.
- Query keys must nest under established parent hierarchies (for example
  `["chats", "costSummary", ...]`, not `["chatCostSummary"]`) so parent
  invalidation works.
- When you do not need to await a mutation, use `mutate()` with
  `onSuccess`/`onError`, not `mutateAsync()` in a try/catch that swallows
  errors.
- Use effects only to synchronize with external systems. Compute derived
  values during render; never `useEffect` + `setState` to derive state.
- List every reactive value in dependency arrays; use refs only for a
  documented non-reactive boundary.

## React Compiler and performance

- The React Compiler scope is defined by the include filter in
  `site/vite.config.mts` (currently `src/pages/AgentsPage/` and
  `src/pages/AIBridgePage/`) and enforced by
  `site/scripts/check-compiler.mjs`. Inside that scope, do not add
  `useMemo`, `useCallback`, or `memo()`. The one exception is `memo()` on
  list-item components rendered in a `.map()`, because the compiler does
  not add `React.memo()` behavior across component boundaries.
- Outside the compiled scope, add manual memoization only for a measured
  or structurally necessary reason, documented near the code.
- Extract frequently changing state (scroll, hover, animation) into a
  child component instead of re-rendering a large parent subtree, and
  throttle high-frequency handlers with `requestAnimationFrame`.

## Testing

- Storybook stories and `play` functions are the only place for rendered
  component and page behavior: visual states, interactions, keyboard
  navigation, focus management, accessibility. Do not create standalone
  vitest/RTL test files for components or pages. Reserve plain Vitest for
  pure logic: utilities, data transformations, `renderHook()` hooks
  without DOM assertions, and query/cache operations.
- Assert observable behavior via roles, accessible names, and visibility
  (`queryByRole`, `toBeVisible()`), never CSS class names. Use
  `data-testid` only when an element has no semantic role.
- Do not depend on `behavior: "smooth"` scrolling in tests; use
  `behavior: "instant"` or set scroll position directly.
- Update or add stories whenever you change a component's appearance or
  behavior; stale stories hide regressions.

## Accessibility

- Every table needs an `aria-label` or `<caption>`.
- Every element with `tabIndex={0}` needs a semantic `role`.
- When visually hiding an interactive element (`opacity-0`,
  `pointer-events-none`), also remove it from the tab order and
  accessibility tree (`tabIndex={-1}` plus `aria-hidden`, or better,
  conditionally render it). `pointer-events: none` only suppresses
  mouse and touch input.
- Use `React.useId()` for element IDs; hard-coded IDs collide when a
  component renders twice.

## Robustness

- Render a visible fallback ("Untitled", "N/A") for nullable user-facing
  text; never a blank cell.
- Guard `Number(...)` conversions against `NaN` before formatting.
- Pass an explicit locale to `toLocaleString()` for deterministic output.
- Accept dynamic values like the current time as props so components stay
  deterministic and testable in Storybook.

## Pre-PR checklist

1. `pnpm lint` - Biome, type check, circular deps, compiler check, knip
2. `pnpm format` - format code consistently
3. `pnpm test` - run affected unit tests
4. Visual check in Storybook if components changed
