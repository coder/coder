# Frontend Development Guidelines

## TypeScript LSP Navigation (USE FIRST)

When investigating or editing TypeScript/React code, always use the TypeScript language server tools for accurate navigation:

- **Find component/function definitions**: `mcp__typescript-language-server__definition ComponentName`
  - Example: `mcp__typescript-language-server__definition LoginPage`
- **Find all usages**: `mcp__typescript-language-server__references ComponentName`
  - Example: `mcp__typescript-language-server__references useAuthenticate`
- **Get type information**: `mcp__typescript-language-server__hover site/src/pages/LoginPage.tsx 42 15`
- **Check for errors**: `mcp__typescript-language-server__diagnostics site/src/pages/LoginPage.tsx`
- **Rename symbols**: `mcp__typescript-language-server__rename_symbol site/src/components/Button.tsx 10 5 PrimaryButton`
- **Edit files**: `mcp__typescript-language-server__edit_file` for multi-line edits

## Bash commands

- `pnpm dev` - Start Vite development server
- `pnpm storybook --no-open` - Start Storybook dev server
- `pnpm test:storybook` - Run storybook story tests (play functions) via Vitest + Playwright
- `pnpm test:storybook src/path/to/component.stories.tsx` - Run a single story file
- `pnpm test` - Run jest unit tests
- `pnpm test -- path/to/specific.test.ts` - Run a single test file
- `pnpm lint` - Run complete linting suite (Biome + TypeScript + circular deps + knip)
- `pnpm lint:fix` - Auto-fix linting issues where possible
- `pnpm playwright:test` - Run playwright e2e tests. When running e2e tests, remind the user that a license is required to run all the tests
- `pnpm format` - Format frontend code. Always run before creating a PR

## Components

- MUI components are deprecated - migrate away from these when encountered
- Use shadcn/ui components first - check `site/src/components` for existing implementations.
- Do not use shadcn CLI - manually add components to maintain consistency
- The modules folder should contain components with business logic specific to the codebase.
- Create custom components only when shadcn alternatives don't exist
- **Before creating any new component**, search the codebase for existing
  implementations. Check `site/src/components/` for shared primitives
  (Table, Badge, icons, error handlers) and sibling files for local
  helpers. Duplicating existing components wastes effort and creates
  maintenance burden.
- Keep component files under ~500 lines. When a file grows beyond that,
  extract logical sections into sub-components or a folder with an
  index file.

## Styling

- Emotion CSS is deprecated. Use Tailwind CSS instead.
- Use custom Tailwind classes in tailwind.config.js.
- Tailwind CSS reset is currently not used to maintain compatibility with MUI
- Responsive design - use Tailwind's responsive prefixes (sm:, md:, lg:, xl:)
- Do not use `dark:` prefix for dark mode

## Tailwind Best Practices

- Group related classes
- Use semantic color names from the theme inside `tailwind.config.js` including `content`, `surface`, `border`, `highlight` semantic tokens
- Prefer Tailwind utilities over custom CSS when possible

## General Code style

- Use ES modules (import/export) syntax, not CommonJS (require)
- Destructure imports when possible (eg. import { foo } from 'bar')
- Prefer `for...of` over `forEach` for iteration
- **Biome** handles both linting and formatting (not ESLint/Prettier)
- Always use react-query for data fetching. Do not attempt to manage any
  data life cycle manually. Do not ever call an `API` function directly
  within a component.
- **Match existing patterns** in the same file before introducing new
  conventions. For example, if sibling API methods use a shared helper
  like `getURLWithSearchParams`, do not manually build `URLSearchParams`.
  If sibling components initialize state with `useMemo`, don't switch to
  `useState(initialFn)` in the same file without reason.
- Match errors by error code or HTTP status, never by comparing error
  message strings. String matching is brittle — messages change, get
  localized, or get reformatted.

## TypeScript Type Safety

- **Never use `as unknown as X`** double assertions. They bypass
  TypeScript's type system entirely and hide real type incompatibilities.
  If types don't align, fix the types at the source.
- **Prefer type annotations over `as` casts.** When narrowing is needed,
  use type guards or conditional checks instead of assertions.
- **Avoid the non-null assertion operator (`!.`)**. If a value could be
  null/undefined, add a proper guard or narrow the type. If it can never
  be null, fix the upstream type definition to reflect that.
- **Use generated types from `api/typesGenerated.ts`** for all
  API/server types. Never manually re-declare types that already exist in
  generated code — duplicated types drift out of sync with the backend.
- If a component's implementation depends on a prop being present, make
  that prop **required** in the type definition. Optional props that are
  actually required create a false sense of flexibility and hide bugs.
- Avoid `// @ts-ignore` and `// eslint-disable`. If they seem necessary,
  document why and seek a better-typed alternative first.

## React Query Patterns

- **Query keys must nest** under established parent key hierarchies. For
  example, use `["chats", "costSummary", ...]` not `["chatCostSummary"]`.
  Flat keys that break hierarchy prevent
  `queryClient.invalidateQueries(parentKey)` from correctly invalidating
  related queries.
- When you don't need to `await` a mutation result, use **`mutate()`**
  with `onSuccess`/`onError` callbacks — not `mutateAsync()` wrapped in
  `try/catch` with an empty catch block. Empty catch blocks silently
  swallow errors. `mutate()` automatically surfaces errors through
  react-query's error state.

## Accessibility

- Every `<table>` / `<Table>` must have an **`aria-label`** or
  `<caption>` so screen readers can distinguish between multiple tables
  on a page.
- Every element with `tabIndex={0}` must have a semantic **`role`**
  attribute (e.g., `role="button"`, `role="row"`) so assistive technology
  can communicate what the element is.
- When hiding an interactive element visually (e.g., `opacity-0`,
  `pointer-events-none`), you **must also** remove it from the keyboard
  tab order and accessibility tree. Add `tabIndex={-1}` and
  `aria-hidden="true"`, or better yet, conditionally render the element
  so it's not in the DOM at all. `pointer-events: none` only suppresses
  mouse/touch — keyboard and screen readers still reach the element.

## Testing Patterns

- **Assert observable behavior, not CSS class names.** In Storybook play
  functions and tests, use queries like `queryByRole`, `toBeVisible()`,
  or `not.toBeVisible()` — not assertions on class names like
  `opacity-0`. Asserting class names couples tests to the specific
  Tailwind/CSS technique and breaks when the styling mechanism changes
  without user-visible regression.
- **Use `data-testid`** for test element lookup when an element has no
  semantic role or accessible name (e.g., scroll containers, wrapper
  divs). Never use CSS class substring matches like
  `querySelector("[class*='flex-col-reverse']")` — these break silently
  on class renames or Tailwind output changes.
- **Don't depend on `behavior: "smooth"` scroll** in tests. Smooth
  scrolling is async and implementation-defined — in test environments,
  `scrollTo` may not produce native scroll events at all. Use
  `behavior: "instant"` in test contexts or mock the scroll position
  directly.
- When modifying a component's visual appearance or behavior, **update or
  add Storybook stories** to capture the change. Stories must stay
  current as components evolve — stale stories hide regressions.

## Robustness

- When rendering user-facing text from nullable/optional data, always
  provide a **visible fallback** (e.g., "Untitled", "N/A", em-dash).
  Never render a blank cell or element.
- When converting strings to numbers (e.g., `Number(apiValue)`), **guard
  against `NaN`** and non-finite results before formatting. For example,
  `Number("abc").toFixed(2)` produces `"NaN"`.
- When using `toLocaleString()`, always pass an **explicit locale**
  (e.g., `"en-US"`) for deterministic output across environments. Without
  a locale, `1234` formats as `"1.234"` in `de-DE` but `"1,234"` in
  `en-US`.

## Performance

- `src/pages/AgentsPage/` and `src/components/ai-elements/` are opted
  into React Compiler via `babel-plugin-react-compiler`. The compiler
  automatically memoizes values, callbacks, and JSX at build time. Do
  not add `useMemo`, `useCallback`, or `memo()` in these directories
  — the compiler handles it. The only exception is `memo()` on
  list-item components rendered in a `.map()` (e.g. `ChatMessageItem`,
  `Tool`, `ChatTreeNode`, `LazyFileDiff`) because the compiler does
  not add `React.memo()` behavior across component boundaries.
- When adding state that changes frequently (scroll position, hover,
  animation frame), **extract the state and its dependent UI into a child
  component** rather than keeping it in a parent that renders a large
  subtree. This prevents React from re-rendering the entire subtree on
  every state change.
- **Throttle high-frequency event handlers** (scroll, resize, mousemove)
  that call `setState`. Use `requestAnimationFrame` or a throttle
  utility. Even when React skips re-renders for identical state, the
  handler itself still runs on every frame (60Hz+).

## Workflow

- Be sure to typecheck when you're done making a series of code changes
- Prefer running single tests, and not the whole test suite, for performance
- Some e2e tests require a license from the user to execute
- Use pnpm format before creating a PR
- **ALWAYS use TypeScript LSP tools first** when investigating code - don't manually search files

## Pre-PR Checklist

1. `pnpm check` - Ensure no TypeScript errors
2. `pnpm lint` - Fix linting issues
3. `pnpm format` - Format code consistently
4. `pnpm test` - Run affected unit tests
5. Visual check in Storybook if component changes

## Migration (MUI → shadcn) (Emotion → Tailwind)

### Migration Strategy

- Identify MUI components in current feature
- Find shadcn equivalent in existing components
- Create wrapper if needed for missing functionality
- Update tests to reflect new component structure
- Remove MUI imports once migration complete

### Migration Guidelines

- Use Tailwind classes for all new styling
- Replace Emotion `css` prop with Tailwind classes
- Leverage custom color tokens: `content-primary`, `surface-secondary`, etc.
- Use `className` with `clsx` for conditional styling

## React Rules

### 1. Purity & Immutability

- **Components and custom Hooks must be pure and idempotent**—same inputs → same output; move side-effects to event handlers or Effects.
- **Never mutate props, state, or values returned by Hooks.** Always create new objects or use the setter from useState.

### 2. Rules of Hooks

- **Only call Hooks at the top level** of a function component or another custom Hook—never in loops, conditions, nested functions, or try / catch.
- **Only call Hooks from React functions.** Regular JS functions, classes, event handlers, useMemo, etc. are off-limits.

### 3. React orchestrates execution

- **Don't call component functions directly; render them via JSX.** This keeps Hook rules intact and lets React optimize reconciliation.
- **Never pass Hooks around as values or mutate them dynamically.** Keep Hook usage static and local to each component.

### 4. State Management

- After calling a setter you'll still read the **previous** state during the same event; updates are queued and batched.
- Use **functional updates** (setX(prev ⇒ …)) whenever next state depends on previous state.
- Pass a function to useState(initialFn) for **lazy initialization**—it runs only on the first render.
- If the next state is Object.is-equal to the current one, React skips the re-render.

### 5. Effects

- An Effect takes a **setup** function and optional **cleanup**; React runs setup after commit, cleanup before the next setup or on unmount.
- The **dependency array must list every reactive value** referenced inside the Effect, and its length must stay constant.
- Effects run **only on the client**, never during server rendering.
- Use Effects solely to **synchronize with external systems**; if you're not "escaping React," you probably don't need one.
- **Never use `useEffect` to derive state from props or other state.** If
  a value can be computed during render, use `useMemo` or a plain
  variable. A `useEffect` that reads state A and calls `setState(B)` on
  every change is a code smell — it causes an extra render cycle and adds
  unnecessary complexity.

### 6. Lists & Keys

- Every sibling element in a list **needs a stable, unique key prop**. Never use array indexes or Math.random(); prefer data-driven IDs.
- Keys aren't passed to children and **must not change between renders**; if you return multiple nodes per item, use `<Fragment key={id}>`
- **Never use `key={String(booleanState)}`** to force remounts. When the
  boolean flips, React unmounts and remounts the component synchronously,
  killing exit animations (e.g., dialog close transitions) and wasting
  renders. Use a monotonically increasing counter or avoid `key` for
  this pattern entirely.

### 7. Refs & DOM Access

- useRef stores a mutable .current **without causing re-renders**.
- **Don't call Hooks (including useRef) inside loops, conditions, or map().** Extract a child component instead.
- **Avoid reading or mutating refs during render;** access them in event handlers or Effects after commit.

### 8. Element IDs

- **Use `React.useId()`** to generate unique IDs for form elements,
  labels, and ARIA attributes. Never hard-code string IDs — they collide
  when a component is rendered multiple times on the same page.

### 9. Component Testability

- When a component depends on a dynamic value like the current time or
  date, **accept it as a prop** (or via context) rather than reading it
  internally (e.g., `new Date()`, `Date.now()`). This makes the
  component deterministic and testable in Storybook without mocking
  globals.
