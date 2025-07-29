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
- `pnpm storybook --no-open` - Run storybook tests
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

- **Don’t call component functions directly; render them via JSX.** This keeps Hook rules intact and lets React optimize reconciliation.
- **Never pass Hooks around as values or mutate them dynamically.** Keep Hook usage static and local to each component.

### 4. State Management

- After calling a setter you’ll still read the **previous** state during the same event; updates are queued and batched.
- Use **functional updates** (setX(prev ⇒ …)) whenever next state depends on previous state.
- Pass a function to useState(initialFn) for **lazy initialization**—it runs only on the first render.
- If the next state is Object.is-equal to the current one, React skips the re-render.

### 5. Effects

- An Effect takes a **setup** function and optional **cleanup**; React runs setup after commit, cleanup before the next setup or on unmount.
- The **dependency array must list every reactive value** referenced inside the Effect, and its length must stay constant.
- Effects run **only on the client**, never during server rendering.
- Use Effects solely to **synchronize with external systems**; if you’re not “escaping React,” you probably don’t need one.

### 6. Lists & Keys

- Every sibling element in a list **needs a stable, unique key prop**. Never use array indexes or Math.random(); prefer data-driven IDs.
- Keys aren’t passed to children and **must not change between renders**; if you return multiple nodes per item, use `<Fragment key={id}>`

### 7. Refs & DOM Access

- useRef stores a mutable .current **without causing re-renders**.
- **Don’t call Hooks (including useRef) inside loops, conditions, or map().** Extract a child component instead.
- **Avoid reading or mutating refs during render;** access them in event handlers or Effects after commit.
