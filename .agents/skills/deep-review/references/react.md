# React Review Checks for Coder

[`site/AGENTS.md`](../../../../site/AGENTS.md) is the canonical frontend
contract. Apply normal React expertise without repeating it here. Check only
these repository-specific rules.

## Data and API boundaries

- Require React Query for all server state, including reads, mutations, cache
  invalidation, loading, and error state.
- Flag components that call API client functions directly. API calls belong in
  query or mutation definitions under the established API layer.
- Require generated API types from
  `site/src/api/typesGenerated.ts`. Flag locally re-declared response, request,
  enum, or resource types.
- Match API errors by stable error code or HTTP status. Flag comparisons against
  human-readable message strings.

## Components and styling

- Prefer existing shadcn/ui components from `site/src/components`. Flag new MUI
  usage because MUI is deprecated.
- Require Tailwind for new styling. Flag new Emotion usage because Emotion is
  deprecated.
- Check accessibility through semantic roles, accessible names, keyboard
  behavior, and focus behavior. Do not accept CSS class assertions as evidence
  of accessible or observable behavior.
- Require Biome-compatible code and configuration. Do not suggest ESLint or
  Prettier fixes.

## Tests

- Require Storybook stories for rendered components and pages.
- Require Storybook `play` functions for interactions, keyboard behavior, focus,
  accessibility, loading, error, and state-transition coverage.
- Flag standalone Vitest or React Testing Library files that render components
  or pages. Plain Vitest is reserved for pure logic, data transformations,
  non-DOM hooks, and query or cache operations.
- Refer to the frontend test runner as Vitest, never Jest.
- Assert behavior through roles, accessible names, visible state, and user
  interactions. Flag tests coupled to CSS classes or Tailwind output.

## React Compiler

- Read `site/vite.config.mts` before judging compiler behavior. Its include
  filter is the source of truth for compiled paths.
- The current compiled directories are `site/src/pages/AgentsPage/` and
  `site/src/pages/AIBridgePage/`.
- Inside compiled directories, flag manual `useMemo`, `useCallback`, and
  `memo()` because the compiler supplies memoization.
- Allow the repository exception: `memo()` on list-item components rendered
  inside `.map()`, where React Compiler does not add cross-component memoization.
- Do not apply compiler-specific restrictions outside the configured include
  paths.
