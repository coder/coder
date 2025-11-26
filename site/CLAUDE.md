# Frontend Development Guidelines

## 🚨 Critical Pattern Migrations (MUST FOLLOW)

The following patterns are actively being migrated and have **STRICT policies**:

1. **Emotion → Tailwind**: "No new emotion styles, full stop" - Always use Tailwind CSS
2. **MUI Components → Custom/Radix Components**: Replace MUI components (Tooltips, Tables, Buttons) with custom/shadcn equivalents
3. **MUI Icons → lucide-react**: All icons must use lucide-react, never MUI icons
4. **spyOn → queries parameter**: Use `queries` in story parameters for GET endpoint mocks
5. **localStorage → user_configs**: Store user preferences in backend, not browser storage

When touching existing code, **"leave the campsite better than you found it"** - refactor old patterns to new ones even if not directly related to your changes.

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

- **MUI components are deprecated** - migrate away from these when encountered
  - Replace `@mui/material/Tooltip` with custom `Tooltip` component (Radix-based)
  - Default 100ms delay via global tooltip provider
  - Use `delayDuration={0}` when immediate tooltip needed
  - Replace MUI Tables with custom table components
  - Replace MUI Buttons with shadcn Button components
  - Systematically replace MUI components with custom/shadcn equivalents
- Use shadcn/ui components first - check `site/src/components` for existing implementations
- Do not use shadcn CLI - manually add components to maintain consistency
- The modules folder should contain components with business logic specific to the codebase
- Create custom components only when shadcn alternatives don't exist

### Icon Migration: MUI Icons → lucide-react

**STRICT POLICY**: All icons must use `lucide-react`, not MUI icons.

```tsx
// OLD - MUI Icons (DO NOT USE)
import BusinessIcon from "@mui/icons-material/Business";
import GroupOutlinedIcon from "@mui/icons-material/GroupOutlined";
import PublicOutlinedIcon from "@mui/icons-material/PublicOutlined";

// NEW - lucide-react
import {
  Building2Icon,
  UsersIcon,
  GlobeIcon,
} from "lucide-react";
```

**Common icon mappings:**

- `BusinessIcon` → `Building2Icon`
- `GroupOutlinedIcon` / `GroupIcon` → `UsersIcon`
- `PublicOutlinedIcon` / `PublicIcon` → `GlobeIcon`
- `PersonIcon` → `UserIcon`
- Always use descriptive lucide-react icons over generic MUI icons

### MUI → Radix Component Prop Naming

When migrating from MUI to Radix components, prop names change:

```tsx
// MUI Tooltip props
<Tooltip placement="top" PopperProps={...}>

// Radix Tooltip props
<Tooltip side="top">  // placement → side
// PopperProps is removed (internal implementation detail)
```

**Common prop name changes:**

- `placement` → `side` (for positioning)
- Remove `PopperProps` (internal implementation, not needed)
- MUI's `title` prop → Radix uses children pattern with `TooltipContent`

## Styling

- **Emotion CSS is STRICTLY DEPRECATED: "no new emotion styles, full stop"**
  - Never use `@emotion/react`, `css` prop, `useTheme()`, or emotion styled components
  - Always use Tailwind CSS utility classes instead
  - When touching code with emotion styles, refactor to Tailwind ("leave the campsite better than you found it")
- Use custom Tailwind classes in tailwind.config.js
- Tailwind CSS reset is currently not used to maintain compatibility with MUI
- Responsive design - use Tailwind's responsive prefixes (sm:, md:, lg:, xl:)
- Do not use `dark:` prefix for dark mode

### Common Emotion → Tailwind Migrations

```tsx
// OLD - Emotion (DO NOT USE)
import { type Interpolation, type Theme, useTheme } from "@emotion/react";
<div css={styles.container}>
<Stack direction="row" spacing={3}>
<span css={{ fontWeight: 500, color: theme.experimental.l1.text }}>

// NEW - Tailwind
<div className="flex flex-col gap-2">
<div className="flex items-center gap-6">
<span className="font-medium text-content-primary">
```

**Common replacements:**

- `css={visuallyHidden}` → `className="sr-only"`
- `Stack` component → flex with Tailwind classes (`flex`, `flex-col`, `flex-row`, `gap-*`)
- Theme colors → Tailwind semantic tokens (`text-content-primary`, `bg-surface-secondary`, `border-border-default`)
- Icons: use lucide-react with `size-icon-sm`, `size-icon-xs` classes

## Tailwind Best Practices

- Group related classes
- Use semantic color names from the theme inside `tailwind.config.js` including `content`, `surface`, `border`, `highlight` semantic tokens
- Prefer Tailwind utilities over custom CSS when possible
- For conditional classes, use the `cn()` utility (from `utils/cn`) which combines `clsx` and `tailwind-merge`

  ```tsx
  import { cn } from "utils/cn";
  
  <div className={cn("base-classes", condition && "conditional-classes", className)} />
  ```

## General Code style

- Use ES modules (import/export) syntax, not CommonJS (require)
- Destructure imports when possible (eg. import { foo } from 'bar')
- Prefer `for...of` over `forEach` for iteration
- **Biome** handles both linting and formatting (not ESLint/Prettier)

## Testing Patterns

### Storybook: spyOn → queries parameter (for GET endpoint mocks)

**PREFERRED PATTERN**: Use `queries` parameter in story parameters instead of `spyOn` for GET endpoint mocks.

```tsx
// OLD - spyOn pattern (AVOID for GET mocks)
beforeEach: () => {
  spyOn(API, "getUsers").mockResolvedValue({
    users: MockUsers,
    count: MockUsers.length,
  });
  spyOn(API, "getTemplates").mockResolvedValue([MockTemplate]);
}

// NEW - queries parameter pattern (PREFERRED)
parameters: {
  queries: [
    {
      key: usersKey({ q: "" }),
      data: {
        users: MockUsers,
        count: MockUsers.length,
      },
    },
    {
      key: getTemplatesQueryKey({ q: "has-ai-task:true" }),
      data: [MockTemplate],
    },
  ],
}
```

**Important notes:**

- This applies specifically to GET endpoint mocks in Storybook stories
- `spyOn` is still used for other mock types (POST, PUT, DELETE, non-GET endpoints)
- Must import the correct query key functions (e.g., `usersKey`, `getTemplatesQueryKey`)

### Chromatic/Storybook Testing Best Practices

- **Prefer visual validation through snapshots** over programmatic assertions
- Chromatic snapshots catch visual changes during review
- Avoid programmatic assertions in stories that duplicate what snapshots show
- Programmatic assertions can introduce flakiness - remove when redundant
- Stories are snapshot tests - rely on the screenshot to verify correctness

## State Storage

### localStorage vs user_configs table

**IMPORTANT**: For user preferences that should persist across devices and browsers, use the `user_configs` table in the backend, NOT `localStorage`.

- **localStorage is browser-specific**, not user-specific
- **User preferences should persist** across devices/browsers
- Follow the plumbing for `theme_preference` as a reference example
- localStorage may be acceptable only for truly transient UI state that doesn't need to follow the user

**Key principle**: If a user dismisses something or sets a preference, it should be tied to their account, not their browser.

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
