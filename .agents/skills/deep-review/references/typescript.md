# TypeScript Review Checks for Coder

[`site/AGENTS.md`](../../../../site/AGENTS.md) is the canonical frontend
contract. Apply normal TypeScript expertise without repeating it here. Check
only these repository-enforced rules.

## Type safety

- Reject `as unknown as X` double assertions. They bypass structural checking
  and conceal contract mismatches.
- Flag non-null assertions such as `value!.field`. Require explicit narrowing,
  an early return, a default, or a proven invariant expressed in the type.
- Prefer type guards, discriminated unions, schema validation, and control-flow
  narrowing over `as` casts.
- Treat a single `as` cast as a review target. Keep it only when the runtime
  invariant is established and TypeScript cannot express the narrowing.
- Reject `// @ts-ignore`. Fix the type or use a narrowly scoped, explained
  `// @ts-expect-error` only when the error is intentional and unavoidable.

## Contracts

- Make props required when the implementation depends on them. Flag optional
  props that are dereferenced, rendered, or used for control flow without a
  real fallback.
- Require generated API types from
  `site/src/api/typesGenerated.ts`. Do not re-declare API requests, responses,
  resources, enums, or error shapes.
- Preserve generated optionality and nullability. Do not cast generated values
  into a more convenient local shape.
- Match API errors by stable error code or HTTP status, never by message text.

## Review discipline

- Do not propose generic TypeScript modernization unless it fixes a concrete
  issue in the changed code.
- Verify proposed types against nearby call sites, generated contracts, and the
  repository's existing patterns.
- Keep suppressions and casts local, rare, and justified by a runtime invariant.
