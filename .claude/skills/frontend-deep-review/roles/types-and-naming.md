# Types and Naming (TN)

Lens: Does every type, identifier, and user-facing string in the diff say exactly what the code does, with nothing left optional, coerced, re-declared, or loosely worded without a stated reason?

Boundaries: Whether a prop, variable, or helper should exist at all is SD or RP; where a shared type or component should live, and file naming, is RO; the query and storage semantics behind a fallback are DF; comments and JSDoc are CD; text overflow and truncation are SA; test fixture content is TS.

Reading order: Open new and changed component files (`*.tsx`) first and read each props type against every call site, not only the hunks, to judge optionality. Then read `*.ts` helpers and `api/queries/*` for coercions and hand-written shapes, and grep `site/src/api/typesGenerated.ts` for any field names you see re-declared. Open one sibling component in the same folder to learn the local `React.FC`, `type`, and naming conventions before flagging a deviation. Finally read every changed JSX string literal and `name:` label as a user would see it.

## Checkpoints

### TN1: Make props and parameters required unless a caller really omits them

Default severity: should-fix
FE parent: FE2

Look for:

- Every new `?:` in a props type and every `| undefined` or `| null` in a function signature: grep the call sites; if every caller passes the value, drop the `?` or the union.
- Signatures that accept `| null | undefined` on several parameters at once; this usually means the caller has not narrowed its data, so ask for the narrowing upstream.
- Doc comments that promise behaviour the type does not allow (for example "passing null removes the key" on `set: (value: T)`).
- Optional booleans (`showX?: boolean`) added to an existing component with no caller relying on the default.

Fix: Make the prop or parameter required, or narrow at the call site and pass a definite value. A prop that some caller really omits stays optional and is not a finding.

### TN2: Remove coercions and fallbacks that hide the real type

Default severity: should-fix
FE parent: FE2

Look for:

- `!!x` in new code: the codebase preference is `Boolean(x)`.
- `String(x)` or `x === undefined ? undefined : String(x)` where `x` is already `string | undefined`; check the declared type before accepting the cast.
- `?? ""`, `|| "fallback"`, `?? otherValue`, and `"" satisfies T` sentinels: ask under what scenario the left side is missing and whether the fallback is reachable. A visible fallback such as "N/A" or "Unknown" for nullable display text is required by FE5, so keep it. A hardcoded literal that duplicates an existing default constant must reference the constant.
- `x === false` or `x !== false` on a non-optional `boolean`: write `!x` or `x`. On an optional field, `x !== false` treats a missing value as true, so it stays (`info.enabled !== false` in `modelOptions.ts`).

Fix: Delete the coercion when the declared type already rules out the missing case. When the fallback is reachable, keep the visible fallback FE5 requires for display text; otherwise make the source non-nullable, or name the default as a constant and reference it.

### TN3: Reuse generated and library types instead of re-declaring them

Default severity: should-fix
FE parent: FE2

Look for:

- New `type X = { id: string; name: string; ... }` or `interface` blocks describing API data: search `api/typesGenerated.ts` for the same fields.
- Hand-written callback or option shapes (`onSuccess?: () => void; onError?: () => void`) that mirror a React Query, React, or other library type.
- Inline unions (`"info" | "warning"`) that repeat a union already declared in a sibling file: export the existing type and import it.
- `React.HTMLAttributes<HTMLParagraphElement>` where `React.ComponentProps<"p">` is the convention, and types that carry both `snake_case` and `camelCase` spellings of the same field.

Fix: Import the generated or library type; when a local union already exists, export it and use it in both places.

### TN4: Follow the codebase type conventions: React.FC, named type props, plain generics

Default severity: should-fix
FE parent: none

Look for:

- Components declared as `({ a, b }: Props) =>` with no `React.FC`, or as `FC<{ inline: string }>` with the props shape inlined. The codebase convention is a named `XProps` type and `React.FC<XProps>`; in a file whose existing components are bare typed arrows (`site/src/components/Tabs/Tabs.tsx`), match the file instead, as site/AGENTS.md asks. A generic component such as `Timeline = <TData,>({ ... }: TimelineProps<TData>)` (`Timeline.tsx`) stays a typed arrow, because `React.FC` cannot declare type parameters.
- `interface XProps` for props, and diffs that swap `React.FC` for a bare `FC` import: the convention is `type` and the `React` namespace; treat a change away from it as a regression.
- Generics narrowed to `string` (or another concrete type) when the implementation does not depend on it, and multi-line `NonNullable<ComponentPropsWithRef<...>[...]>` gymnastics to express a simple requirement.
- Two styles for the same thing inside one PR (one query key `as const`, another not): pick the existing convention and apply it everywhere. Fixture names are exempt: they are TS3's checkpoint, and a new `mockX` fixture beside pre-existing unprefixed fixtures is the convention arriving, not a second style.

Fix: Declare `type XProps = { ... }` and `export const X: React.FC<XProps> = (...)` unless the file's existing components use another form; keep generics generic; match the pattern in the same file, then the surrounding folder, rather than introducing a second one. Never recommend removing a `mock` prefix or renaming existing unprefixed fixtures.

### TN5: Name identifiers for what they mean to the reader

Default severity: should-fix
FE parent: none

Look for:

- Names that describe one case of the value (`hasFileReferences` for any chip content, `NON_LIGHT_THEME_CLASSES` that still includes light colorblind themes, `showClassicParameterFlow` for a compatibility-mode alert).
- `is*` and `has*` functions that do not return `boolean`.
- Props, helpers, and constants a consumer cannot read at a glance (`text` and `query`, `cacheKeyFor`, `host`): ask what it is and how it is used; the answer should be the name.
- Positional arguments of primitive lists whose purpose is only guessable from the values, and `data-testid` values that contradict the component name (a reusable frame with a `left-sidebar` test id).

Fix: Rename to the general meaning, return a boolean from `is*` functions, and replace opaque positional arguments with an options object whose keys say what they do.

### TN6: Keep user-facing copy deliberate: wording, case, and punctuation

Default severity: should-fix
FE parent: none

Look for:

- Diff lines that change only the case of existing UI text ("AI Gateway Keys" to "AI Gateway keys"): confirm it is intentional and matches neighbouring labels.
- Rewritten descriptions that now mention a feature or concept the original never did, especially in a removal PR.
- Count-based messages that drop the noun ("1 template" becoming a bare display name): keep the entity word in the sentence.
- New filter and label names that do not use the product term already used elsewhere in the diff ("compatibility mode" versus "classic parameters"), and escaped characters (`"\u2014"`) that put punctuation the lint rule bans into copy. An escaped dash shown alone as the placeholder for a missing value is not punctuation and is fine (`CoderAgentsProductCard.tsx`).

Fix: Revert unintended copy changes, keep the entity noun in messages, align new labels with the vocabulary used nearby, and rewrite sentences instead of encoding banned punctuation.

## Not this role

- Whether a derived local should exist at all (`const agentId = chat.id` pulled out for a single use): SD. TN only keeps the "overloaded term" half.
- Hand parsing of tool result payloads with `asString` helpers when no typed schema exists: TN flags the loose narrowing; designing a typed parsing layer for streamed data is DF.
- Real domains in test fixtures (`"*.coder.com"` instead of `"*.example.com"`): TS. TN only keeps the `HOST` naming suggestion.
- The `mock` prefix on story and test fixtures: TS3, even when older fixtures in the same file lack it.
- A `?? organization` fallback that exists because the query is gated on a possibly missing organization: the query shape and enablement belong to DF.
- Where a shared type such as `MutationCallbacks` should live: RO.
- `as const` consistency on query keys across `api/queries/*` overlaps with DF's query key discipline.
