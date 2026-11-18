# Simplicity and Dead Code (SD)

Lens: does every added line earn its place, or is the diff carrying constants, wrappers, guards, and abstractions that a reader must unwind to see the three lines of real logic underneath.

Boundaries: "we already have X in components/" and duplicated code across files is RO; comment text is CD; effect, hook, and render-helper mechanics are RP; optional-vs-required props, casts, and identifier names are TN; Tailwind class content and layout are SA; story and test structure is TS; storage and query semantics are DF.

Reading order: open new files first (a new utils, hooks, or storage module is where whole-module over-engineering hides), then the largest hunks in existing components. For every new top-level const, helper, or export, grep the PR for its usages before reading further; one usage is the signal for SD1, SD2, and SD7. Read each new file whole, not just the hunk, so unreachable branches and unused exports are visible. Open the existing component or helper a wrapper delegates to and ask what the wrapper adds beyond its name.

## Checkpoints

### SD1: Inline single-use constants and variables

Default severity: should-fix
FE parent: FE3

Look for:

- A module-level or function-level const (className string, copy string, tooltip text, ternary result, fixture id) referenced exactly once. Grep the identifier: one hit besides the declaration means inline it, unless the name carries meaning the literal does not (a limit, a unit, a protocol value), which site/AGENTS.md allows. A module-level array, object, or function also stays when its stable reference matters: outside the React Compiler directories, inlining `[]` creates a new value on every render (`emptyPresets` in `CreateWorkspacePage.tsx` feeds a `useMemo` dependency list).
- Tailwind class strings hoisted into `const xClassName = "..."`, `*_CLASS_NAME`, or `*Classes` constants, especially when a comment or the identifier is the only justification.
- A variable that names a sub-expression (`const foo = a ? b : c`) used once on the next few lines, or destructured aliases (`const agentId = chat.id`) that replace a single property access.
- Exported constants (copy strings, flags) consumed in exactly one file: inline the value when the name adds nothing; when the name carries meaning, RO4 moves the constant next to its consumer instead.

Fix: inline the value at the point of use. If the constant exists to avoid duplicating a class string across elements, the reviewers want a component, not a shared string.

### SD2: Delete wrappers and helpers that add nothing

Default severity: should-fix
FE parent: FE3

Look for:

- One-line functions whose body is a single call, property access, or comparison (`const getX = (c) => c.getQueryData(key(id))`, `const stop = (e) => e.stopPropagation()`, `(v) => v === "a" || v === "b"`). Ask what the name adds over the expression.
- A helper left behind after a refactor whose body collapsed to `return option.displayName` or `return stored`.
- Custom hooks that only call another hook and return its result, unless the hook owns arguments every caller would otherwise repeat (the module-private subscribe and snapshot functions `usePreferredColorScheme.ts` passes to `useSyncExternalStore`), and helpers exported solely so a test can import them. A reset export that clears module-private state for tests stays, because no other file can reach that state (`_resetForTesting` in `chime.ts`).
- Fragment or `<div className="flex">` wrappers around a single child, and `asChild` applied to an element the primitive would already render.
- A single-purpose re-export or rename (`const fooKey = (id) => getFooKey(id)`), or a family of local helpers that a new shared primitive already covers.

Fix: delete the helper and call the underlying expression, primitive, or hook directly at each site. If the wrapper is exported only for a test, inline it and drop the test that exercised the library it wrapped.

### SD3: Simplify expressions in place

Default severity: should-fix
FE parent: none

Look for:

- Nested ternaries, especially two branches yielding the same value (`a ? X : b ? X : Y` is `a || b ? X : Y`), and ternaries inside template literals inside ternaries.
- Regexes a reader cannot decode at a glance, regex built from regex, and `Array.from(new Set([...])).join(" ")` style cleverness where a plain literal or hard-coded list reads better.
- IIFEs used to compute a conditional value, negated hook calls (`!useX()`), template literals wrapped in JSX braces where `{a}` works, and `window.location` where `location` suffices.
- Boolean expressions with three or more operators and a double negative; API surfaces where two entry points do the same thing (`set(null)` and `remove()`, a magic `RESET` sentinel).
- Long functions with no blank lines, or hard-to-follow blocks (deeply nested callbacks, an async IIFE inside a handler).

Fix: rewrite the expression for the reader: flatten the ternary, hard-code the list, name the regex intent or replace it with string methods, use an early return, and keep one way to do each operation. Add blank lines between logical steps.

### SD4: Remove dead code, unreachable guards, and unused surface

Default severity: should-fix
FE parent: FE3

Look for:

- `typeof window === "undefined"` checks anywhere under `site/src/`: the app is a SPA and the branch never runs.
- Guards whose false branch is already the natural result: `if (value.length === 0) return value` before `.map` when no caller relies on getting the same reference back (cache updaters return their input to mean no change, as `upsertMessagesAcrossPages` in `api/queries/chats.ts` does), a `String(x)` on a value already typed `string | undefined`, a cleanup that cannot register because of an earlier return, `process.exit(0)` as the last statement of a script that leaves no timer, socket, watcher, or other handle open (while one is open, the exit is what ends the process).
- Props accepted but never read (`condensed: _condensed`), props added with no caller, empty story parameters (`queries: []` under a meta that seeds no queries; see TS6), aliased imports (`x as xPath`) that only rename, and knip exclusions that hide unused exports.
- Backward-compat redirects and legacy-format branches for routes or data that are not stable or are about to be dropped; try/catch around reads that cannot throw in supported browsers (storage access can: `localStorage` throws a `SecurityError` when the browser denies storage, as `reasoningEffort.test.ts` simulates); lint suppressions where the fix is smaller than the comment.

Fix: delete it. Fix the lint rule instead of suppressing it, remove the prop from the type, and drop compat aliases for unstable routes.

### SD5: Use the built-in or existing primitive instead of hand-rolled logic

Default severity: should-fix
FE parent: none

Look for:

- Deep copies written by hand or as `JSON.parse(JSON.stringify(x))` where `structuredClone` does the job; `[...arr].sort()` where `toSorted` exists. An object spread copies only the top level, so it cannot replace a deep copy whose nested fields are written later (`applyKnownModelDefaults.ts`).
- `typeof error === "object" && error !== null && "name" in error` style guards instead of catching and narrowing on the error type (`instanceof`, `isAxiosError`). Structural guards are correct for parsed data that was never thrown, such as the response body `isApiErrorResponse` checks in `api/errors.ts`.
- Several regexes or `replace` calls (`replace(/^0+/, "")`) that hand-parse one value where a single helper taking the string and returning the parsed value or `undefined` would read better; a hand-written tokenizer or matcher that recreates regex. Parsing is not validation: `Number.parseInt("12abc", 10)` is 12 and `Number("0x50")` is 80, so a replacement must still reject every input the old check rejected (keep `/^\d+$/` when the whole string must be digits).
- npm scripts chaining `cd .. && ... && cd site` where a tool flag (`make -C ..`) exists. Do not suggest `fetch(dataUrl)` in place of manual data-URL decoding: the production CSP's `connect-src` excludes `data:`, so the fetch fails.

Fix: replace the hand-rolled block with the platform or library call, keeping any full-string check the old code performed, and narrow errors by type rather than by structural checks.

### SD6: Remove stray JSX whitespace and blank lines

Default severity: nit
FE parent: none

Look for:

- `{" "}` immediately after a closing tag, a closing `)}` of a conditional, or as the first child of a container. Grep the diff for `{" "}` on added lines and ask whether each one is intended: most are leftovers, but one between inline text and an element is a visible space.
- A blank line inserted between a file header comment and the first import, or in the middle of an import block.
- Whitespace-only hunks in files the PR otherwise did not need to touch.

Fix: delete the unintended `{" "}` or blank line. Keep a `{" "}` that separates inline words, and space flex or grid items with a gap class on the parent.

### SD7: Question premature extraction and abstraction

Default severity: should-fix
FE parent: none

Look for:

- A new file of 200 to 1200 lines (utils, hooks, storage, "API" layers) introduced by a PR whose user-facing change is small. Ask whether it is the simplest design for what the product needs now. A generated file (a `Code generated ... DO NOT EDIT` header, such as `api/rbacresourcesGenerated.ts`) is exempt, because its fix belongs in the generator input.
- Logic pulled out of a component into a helper file or a React context with one consumer, a second `cva` definition for classes applied to the same element, or a generic version of a function with exactly one specialization.
- Module-level mutable state or builders in stories that exist only to share setup between two stories.
- A refactor of working code in the same PR as a feature, with no second caller motivating it.

Fix: revert the extraction and keep the logic where it is used. Cut a large new module down to what the product needs now, or replace it with an existing dependency that covers it.

## Not this role

- "Is a helper already defined somewhere" and "do we have any existing packages we depend on" questions are RO when the answer is an existing module rather than deletion.
- `useState(() => chatId)` mirroring a prop into state and needless async functions are RP: effect and state mechanics, not dead code.
- Parsing stored JSON with a hand-written array check instead of `yup` is DF: persisted-data validation strategy.
- Hoisted Tailwind class strings (SD1) become SA when the reviewer objects to the class content or asks for a variant instead of a className prop.
- "Comment is a bit verbose" alongside a simplification request is CD; only the `toSorted` half is SD5.
- A prop that should not exist because the caller never needs it sits between SD4 and RP; if the reasoning is component API design rather than dead surface, hand it to RP.
