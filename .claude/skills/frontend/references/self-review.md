# Frontend Self-Review (FE1 to FE10 gate)

Deterministic, diff-scoped audit of frontend changes against the FE1 to FE10
contract in `../SKILL.md`. This is a verification gate, not a general code
review. Do not delegate it to the broader `code-review` workflow, and do not
substitute a prose "I checked the rules" summary for the per-rule verdict.

Its purpose is to catch the findings reviewers would otherwise post, before
they see the PR.

## When to run

- Before creating a PR whose diff touches `site/src/`.
- Before pushing significant new commits to an existing frontend PR.
- Whenever you are explicitly asked to review a frontend diff or PR.
- Skip only when the diff touches no files under `site/src/`.

## Workflow

1. Collect the diff: `git diff --merge-base origin/main -- site/src` (add
   `site/e2e` if touched). Fall back to `main` when there is no `origin`
   remote. List the changed files.
2. For each changed file, audit against every FE rule using the checklist
   below. Read the full file when the diff alone cannot answer a check (for
   example, whether a story's `play` function exercises the new behavior).
3. Report a per-rule verdict (see Output format). Every FAIL carries
   `file:line` and a one-line reason.
4. Fix all FAIL findings with the smallest safe diff. Re-run the audit until
   every rule passes or a remaining finding is explicitly justified.
5. Include unresolved justifications in the PR description.

Audit the current diff only. Do not refactor pre-existing violations in
untouched code; note them at most.

## Per-rule diff checklist

- **FE1 (Storybook coverage)**: Does a changed component or page alter
  user-visible behavior? Then a changed or added `.stories.tsx` must exist and
  its `play` function must perform the new interaction, not merely render.
  Interaction tests in `.test.tsx` are a FAIL unless they cover pure logic;
  `renderHook` suites for stateful UI hooks count as interaction tests and
  belong in the consuming component's story.
- **FE2 (types)**: Search the diff for `any`, `as unknown as`, non-null
  assertions (`x!.y`, `items[0]!`, `fn()!`, `value! as T`), and new `as` casts.
  Confirm API data uses `api/typesGenerated.ts`.
- **FE3 (reuse/scope)**: For each new component, hook, or helper, search
  `site/src/components/` and sibling folders for an existing equivalent. Flag
  near-duplicates, hand-assembled versions of wrapped primitives, dead
  branches, and unrelated changes bundled into the diff. Flag new React hooks
  that an existing hook, a plain function, or component state could replace;
  several new single-use hooks in one diff is a FAIL.
- **FE4 (comments)**: Read every comment the diff adds or edits. Flag any that
  restate the identifier, assertion, or control flow, and verify the rest are
  correct.
- **FE5 (UI states)**: For each view rendering server data, confirm loading,
  error, empty, and refetch handling. Flag form or selection state a
  background refetch would reset.
- **FE6 (a11y)**: Flag keyboard-unreachable interactive elements, `aria-label`s
  that replace visible label text, `aria-*` props the primitive overwrites, and
  visually hidden elements still in the tab order.
- **FE7 (react-query)**: Flag direct `API.*`/`fetch` in components,
  string-literal query keys (import the constant from `api/queries/`),
  `isLoading || isFetching`, missing invalidation on mutation paths (including
  partial failure), and `mutateAsync` in a `try/catch` with an empty catch.
- **FE8 (effects)**: For every added or modified `useEffect`, apply the FE8
  decision tree. Flag derived state via `setState`-in-effect, effect-triggered
  fetches, new dependencies on effects that own connections, and effects that
  only write refs nobody reads. Flag preemptive `useCallback`/`useMemo`.
- **FE9 (fixtures)**: Flag inline entity literals that duplicate or deviate
  from `Mock*` fixtures in `site/src/testHelpers/`, and shared pre-wired query
  objects instead of per-story inline `{ key, data }` wiring. Flag any
  `Object.defineProperty` replacement of a browser global in tests or stories:
  unit tests stub with `vi.stubGlobal`, stories mock existing globals with
  `spyOn` from `storybook/test`.
- **FE10 (test queries)**: Flag `querySelector`, class-name substring matches,
  geometry assertions, `behavior: "smooth"` dependence, and locale-less
  `toLocaleString()` in changed tests and stories.

## Output format

```
FE1 PASS
FE2 FAIL  site/src/pages/FooPage/FooPage.tsx:42  `as unknown as Workspace` cast
FE3 PASS
...
```

One line per rule. FAIL lines carry every finding (repeat the rule ID for
multiple findings). After fixes, print the re-run table. The audit is done when
all rules PASS or remaining FAILs have a written justification.

## Notes

- This audit does not replace `pnpm check`, `pnpm lint`, `pnpm format`, or the
  tests; run those too (see the pre-PR checklist in `frontend-guide.md`).
