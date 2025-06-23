# Frontend Development Guidelines

## Bash commands

- `pnpm storybook --no-open` - Run storybook tests
- `pnpm test` - Run jest unit tests
- `pnpm lint` - Lint frontend code using biome
- `pnpm playwright:test` - Run playwright e2e tests. When running e2e tests, remind the user that a license is required to run all the tests
- `pnpm format` - Format frontend code. Always run before creating a PR

## Components

- MUI components are deprecated in this codebase
- Prefer shadcn components, check for existing components in `site/src/components` before adding new components. Do not use shadcn cli.
- The modules folder should contain components with business logic specific to the codebase.

## Styling

- Emotion CSS is deprecated in this codebase. Use Tailwind instead.
- Use custom Tailwind classes in tailwind.config.js

## General Code style

- Use ES modules (import/export) syntax, not CommonJS (require)
- Destructure imports when possible (eg. import { foo } from 'bar')
- Do not use forEach to iterate, prefer using for..of

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

## Workflow

- Be sure to typecheck when you’re done making a series of code changes
- Prefer running single tests, and not the whole test suite, for performance
- Some e2e tests require a license from the user to execute
- Use pnpm format before creating a PR
