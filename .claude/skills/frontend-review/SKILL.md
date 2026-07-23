---
name: frontend-review
description: Diff-scoped self-review of frontend changes under site/src against the FE pattern rules (FE1-FE10) before creating or updating a PR. Use whenever a branch diff touches site/src files.
---

# Frontend Review

Audit the current branch diff against the frontend rule contract in
[`.claude/docs/FRONTEND_PATTERNS.md`](../../docs/FRONTEND_PATTERNS.md) and fix
every violation before the PR is created or updated. This is a self-review
gate: its purpose is to catch the findings reviewers would otherwise post,
before they see the PR.

## When to run

- Before creating a PR whose diff touches `site/src/`.
- Before pushing significant new commits to an existing frontend PR.
- Skip only when the diff touches no files under `site/src/`.

## Workflow

1. Collect the diff: `git diff --merge-base main -- site/src` (add
   `site/e2e` if touched). Use `origin/main` instead when the checkout has an
   `origin` remote and the local `main` may be stale. List the changed files.
2. For each changed file, audit against each FE rule using the checklist
   below. Read the full file when the diff alone cannot answer a check (for
   example, whether a story's `play` function exercises the new behavior).
3. Report results as a per-rule verdict table (see Output format). Every FAIL
   must carry `file:line` and a one-line reason.
4. Fix all FAIL findings with the smallest safe diff. Re-run the audit until
   every rule passes or a remaining finding is explicitly justified.
5. Include unresolved justifications in the PR description so reviewers see
   them up front.

## Per-rule diff checklist

- **FE1 (Storybook coverage)**: Does any changed component or page alter
  user-visible behavior? Then a changed or added `.stories.tsx` must exist,
  and its `play` function must perform the new interaction (open the menu,
  submit the form), not merely render. Interaction tests added to `.test.tsx`
  files are a FAIL unless they cover pure logic.
- **FE2 (types)**: Search the diff for `any`, `as unknown as`, non-null
  assertions in any form (`x!.y`, `items[0]!`, `fn()!`, `value! as T`), and
  new `as` casts. Check that API data uses types from `api/typesGenerated.ts`.
- **FE3 (reuse/scope)**: For each new component, hook, or helper, search
  `site/src/components/` and sibling folders for an existing equivalent.
  Flag near-duplicates, hand-assembled versions of wrapped primitives, dead
  branches, and unrelated changes bundled into the diff.
- **FE4 (comments)**: Read every comment line the diff adds or edits. Flag
  any comment that restates the identifier, assertion, or control flow.
  Verify surviving comments are factually correct.
- **FE5 (UI states)**: For each view rendering server data, confirm loading,
  error, empty, and refetch handling. Flag form or selection state that a
  background refetch would reset.
- **FE6 (a11y)**: Flag interactive elements that are keyboard-unreachable,
  `aria-label`s that replace visible label text, `aria-*` props that the
  underlying primitive overwrites, and visually-hidden elements still in the
  tab order.
- **FE7 (react-query)**: Flag direct `API.*`/`fetch` calls in components,
  string-literal query keys (must import the constant from `api/queries/`),
  `isLoading || isFetching` patterns, missing invalidation on mutation paths
  (including partial failure), and `mutateAsync` in `try/catch` with an empty
  catch.
- **FE8 (effects)**: For every added or modified `useEffect`, apply the
  decision tree in FRONTEND_PATTERNS.md. Flag derived state via
  `setState`-in-effect, fetches triggered by effects, new dependencies on
  effects that own connections, and effects that only write refs nobody
  reads.
- **FE9 (fixtures)**: Flag inline entity literals that duplicate or deviate
  from `Mock*` fixtures in `site/src/testHelpers/`, and shared pre-wired
  query objects instead of per-story inline `{ key, data }` wiring.
- **FE10 (test queries)**: Flag `querySelector`, class-name substring
  matches, geometry assertions, `behavior: "smooth"` dependence, and
  locale-less `toLocaleString()` in changed tests and stories.

## Output format

```
FE1 PASS
FE2 FAIL  site/src/pages/FooPage/FooPage.tsx:42  `as unknown as Workspace` cast
FE3 PASS
...
```

One line per rule. FAIL lines carry every finding (repeat the rule ID for
multiple findings). After fixes, print the re-run table. The audit is done
when all rules PASS or remaining FAILs have a written justification.

## Notes

- This audit does not replace `pnpm check`, `pnpm lint`, `pnpm format`, or
  tests; run those too (see site/AGENTS.md Pre-PR Checklist).
- Report findings in the current diff only. Do not refactor pre-existing
  violations in untouched code; note them at most.
