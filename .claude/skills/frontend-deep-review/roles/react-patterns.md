# React Patterns (RP)

Lens: does each hook, effect, ref, memo call, and component boundary in the diff exist because React needs it, or because the author reached for the first tool that worked?

Boundaries: react-query usage, mutation callbacks, and cache invalidation belong to DF; reuse of existing components and hooks to RO; dead code and zero-value wrappers to SD; CSS and layout measurement to SA; types and prop optionality to TN; stories and tests to TS; comment text to CD.

Reading order: open every changed file under hooks/ and any file whose diff adds useEffect, useLayoutEffect, useRef, useCallback, useMemo, or memo first. Read the whole component, not the hunk: you need to see where the effect's inputs come from and who reads the ref or state it writes. Check whether each file sits in a directory the React Compiler `include` filter in site/vite.config.mts lists; RP4 depends on it. Then open the sibling hooks folder and site/src/hooks to check whether an existing hook already covers the case, and one existing formik form in site/src/pages to compare against any new form.

## Checkpoints

### RP1: Derive it in render instead of mirroring it through an effect

Default severity: should-fix
FE parent: FE8

Look for:

- An effect whose body is only setState calls, keyed on a prop or another piece of state (open, chatId, selected key, org id). Ask: can the value be read directly, or computed inline, on each render?
- A "reset when X changes" effect, often paired with a previousXRef to detect the change. That is the setter-during-render pattern or a keyed remount, not an effect.
- useState initialised from a prop and never set again, in a component that should follow later changes to the prop. Use the prop. Keeping only the first value on purpose is fine when the prop name says so (`initialX`, `defaultX`).
- An effect that runs to "sync" one state into another with an inequality guard. Delete one of the two.

Fix: read the source value directly or compute the derived value in render; when a prop change must reset local state, compare against the previous value during render or remount the subtree with a key derived from the changed id (FE5 forbids `key={String(booleanState)}`).

### RP2: Move side effects into the event handler or mutation callback that causes them

Default severity: should-fix
FE parent: FE8

Look for:

- An effect that persists state (localStorage.setItem, save*) whenever the state changes, usually guarded by a lastPersistedRef, when one event handler or setter call is the only thing that changes that state: the write belongs there. An effect that is the single sync point for state several code paths change is synchronizing with an external system and stays (`RightPanel.tsx` persists a width that dragging, snapping, and resize clamping all set; `AgentCreateForm.tsx` persists an organization that the user and render-time reconciliation both change).
- An effect that navigates, updates storage, or opens an editor as soon as a query result or route param is truthy. Ask which user action or mutation callback should own that step. An effect that consumes a deep-link parameter on arrival and replaces the URL is synchronizing with browser history, which no handler owns (`AgentCreatePage.tsx` strips its prompt and debug parameters with `navigate(..., { replace: true })`). `useQuery` has no `onSuccess` (TanStack Query v5 removed query callbacks), so never suggest one.
- An effect that runs a migration or one-off transform on every render where query data exists, with a ref to make it run once.
- Any effect the author could not explain in one sentence: ask which user action or mutation callback should own it.

Fix: call the side effect from the handler that changes the value, or from the mutation callback that produces it; derive values from query data in render or in the query's `select`. An effect that only synchronizes with an external system is FE8's one allowed case, so it is not an RP2 finding.

### RP3: Audit dependencies and refs on effects that own listeners, observers, fetches, or DOM

Default severity: should-fix
FE parent: FE8

Look for:

- A suppression of the exhaustive-deps lint rule, whichever linter the checkout enables it in: oxlint's `react/exhaustive-deps` (`site/.oxlintrc.jsonc`) or Biome's `lint/correctness/useExhaustiveDependencies`. If the value is stable, add it; if it is not, the effect is wrong.
- An effect that only copies state or a prop into a ref (widthRef.current = width, formValuesRef, dragged.current = true). Ask who reads the ref; usually it exists to dodge a dependency array. Use useEffectEvent or read the value directly.
- addEventListener, ResizeObserver, or a popup's message listener or close polling set up outside an effect, or inside one with no cleanup, or with cleanup in a different effect than the one that created the resource. The `window.open` call itself belongs in the click handler: browsers block popups opened outside a user gesture (`useMCPOAuthFlow.ts`).
- Helper functions declared inside the effect body that capture nothing from the component or the effect: hoist them to module level. A helper that reads props, state, or a resource the effect creates (a socket, observer, or timer) stays inside the effect, as React recommends.
- An element the component renders itself found via querySelector instead of a ref: attach a ref or ref callback. Nodes a library creates (cmdk group headings, a noVNC canvas) cannot take a ref, so querying them from a ref on the nearest element the component owns is correct.

Fix: make the dependency list honest, delete ref-sync effects, keep setup and cleanup in the same effect, and use a ref callback for measurements that only need the mounted element.

### RP4: Leave memoization to React Compiler where it runs, and justify it elsewhere

Default severity: nit
FE parent: none

Look for:

- A new useCallback, useMemo, or memo() in a directory the React Compiler `include` filter in site/vite.config.mts lists. The compiler already memoizes those files. Exception: memo() on a list-item component rendered in a map stays, because compiler memoization does not cross component boundaries (site/AGENTS.md).
- A new useCallback, useMemo, or memo() outside those directories with no stated reason: ask for one. Nothing memoizes these files automatically, so do not ask to remove memoization that has a reason.
- memo() on a child whose parent passes a new function or object on every render: the memo never hits.
- useMemo(() => generateId(), []) used for a stable identity. useMemo is a performance hint React may discard; use useState with an initialiser, or React.useId() for DOM and ARIA ids (FE6).
- Functions that capture no component variables defined inside the component. Hoist them to module level.

Fix: in a compiled directory, remove the memoization (except memo() on list items); elsewhere, keep memoization only with a short comment saying why. Anywhere, use useState for values that must never change and hoist functions that capture nothing. `pnpm lint` already fails when the compiler skips a function in a compiled directory, for example over a `try` with a `finally` clause or with a `? :`, `&&`, `||`, `??`, or `?.` expression inside the `try` block; restructure such a `try` as `.then().catch().finally()`.

### RP5: Do not add a hook that a plain function or an existing hook already covers

Default severity: should-fix
FE parent: FE3

Look for:

- A new use* file with one call site that owns no React state or effect. Ask what breaks if the body is inlined.
- A hook that wraps a single useQuery or useMutation, or that re-fetches data the caller already holds.
- A hook that returns JSX or a render function. That is a component.
- A hook or context provider introduced to avoid passing one value through a few props. Ask whether the value can be computed once at a higher level instead.
- A hook whose only job is one storage flag that one component reads. Ask whether the component can hold the state and call plain get and set functions from a module next to the feature code (as site/src/pages/AgentsPage/utils/*Storage.ts do). A storage hook that keeps several components or tabs in sync through a subscription stays (`useChatFullWidth.ts` uses `useSyncExternalStore`).

Fix: replace with a function that takes the already-fetched data or mutations as arguments, or inline it at the call site. Keep a hook that owns React state or an effect even when one component calls it (`useMCPOAuthFlow.ts` keeps its message listener out of `AgentChatInput.tsx`), except the single-reader storage flag above.

### RP6: Split components that have grown past one screen of logic or markup

Default severity: should-fix
FE parent: none

Look for:

- A diff that adds state, handlers, or derived values to a component that is already several hundred lines. The reviewers flagged the growth even when the file was large before the PR.
- Indentation deeper than roughly six levels of JSX, or a .map callback that builds labels, tooltips, and conditionals inline.
- Nested div.flex trees where no element has a name that says what it is.
- Large page components accumulating request queues, refs, and editing logic that belong in a hook or child component.

Fix: put what the PR adds into named sub-components for each visual block and move its non-render logic into helpers or a hook, in this PR. Splitting the code that was already there is a separate refactor (RO5), not part of this finding.

### RP7: Forms use formik and mutations, not hand-rolled draft and submitting state

Default severity: should-fix
FE parent: none

Look for:

- Several useState calls holding a draft, a toggled flag, and a "local edit" per field, with a save handler that clears them on success. That is a form; use formik.
- setIsSubmitting(true/false) or an inFlight ref around a direct API call. useMutation already tracks pending state. A ref that ignores repeated events while a mutation is in flight is correct (DF2).
- Submit logic that conditionally calls submitForm or setFieldValue from an onChange handler. Ask what the branch is for, and flag it only when the answer shows the form library already covers it.
- A settings panel with one input and one save button. Still a form; the reviewers accepted skipping formik only for the smallest toggles.

Fix: model the fields with useFormik and drive the submit through a useMutation; read isPending from the mutation and remove the manual flags.

### RP8: Anything that returns JSX is a component, so name and define it as one

Default severity: should-fix
FE parent: none

Look for:

- const renderX = (...) => (<...>) declared inside a component body, or any other lowercase function returning ReactNode. Exception: a render callback that a component prop expects, such as `renderOption` on `Autocomplete`.
- The same markup block repeated twice with a shared className constant instead of a small component.
- Helpers that take a "tab" or "message" argument and return a fragment; these are components with props.

Fix: promote it to a capitalised component with typed props, at module level or in its own file, and render it as JSX.

## Not this role

- Manual loading flags (setIsExportingAll) around API calls outside a form, and hooks that only wrap useQuery: DF owns the react-query side of these, RP only flags the hook or form shape.
- Measuring panel or menu width with ResizeObserver or clientWidth to set inline sizes that CSS (w-full) could express: SA.
- Component API consistency, such as exporting Title and Message sub-components as children instead of props, or a dialog type prop whose values are used inconsistently: TN.
- Whether a component may be written as an implicit-return arrow expression: TN, as a codebase convention question.
- Mutating a caller-owned object (URLSearchParams) inside a state setter: a React purity rule no role owns yet; flag it under "Outside my role".
