# React Patterns (RP)

Lens: does each hook, effect, ref, memo call, and component boundary in the diff exist because React needs it, or because the author reached for the first tool that worked?

Boundaries: react-query usage, mutation callbacks, and cache invalidation belong to DF; reuse of existing components and hooks to RO; dead code and zero-value wrappers to SD; CSS and layout measurement to SA; types and prop optionality to TN; stories and tests to TS; comment text to CD.

Reading order: open every changed file under hooks/ and any file whose diff adds useEffect, useLayoutEffect, useRef, useCallback, useMemo, or memo first. Read the whole component, not the hunk: you need to see where the effect's inputs come from and who reads the ref or state it writes. Then open the sibling hooks folder and site/src/hooks to check whether an existing hook (useStorage, useEffectEvent patterns) already covers the case, and one existing formik form in site/src/pages to compare against any new form.

## Checkpoints

### RP1: Derive it in render instead of mirroring it through an effect

Default severity: should-fix
FE parent: FE8
Evidence: 9 comments across 7 PRs

Look for:
- An effect whose body is only setState calls, keyed on a prop or another piece of state (open, chatId, selected key, org id). Ask: can the value be read directly, or computed inline, on each render?
- A "reset when X changes" effect, often paired with a previousXRef to detect the change. That is the setter-during-render pattern or a keyed remount, not an effect.
- useState initialised from a prop and never set again. Use the prop.
- An effect that runs to "sync" one state into another with an inequality guard. Delete one of the two.

Fix: read the source value directly or compute the derived value in render; when a prop change must reset local state, compare against the previous value during render or remount the subtree with a key.

### RP2: Move side effects into the event handler or query callback that causes them

Default severity: should-fix
FE parent: FE8
Evidence: 8 comments across 4 PRs

Look for:
- An effect that persists state (storage.set, save*) whenever the state changes, usually guarded by a lastPersistedRef. The write belongs in the setter or the existing storage hook.
- An effect that navigates, updates storage, or opens an editor as soon as a query result or route param is truthy. Ask which user action or query callback should own that step.
- An effect that runs a migration or one-off transform on every render where query data exists, with a ref to make it run once.
- Any effect the author could not explain in one sentence. The reviewers asked for the justification in the PR or in a comment, and mostly got a rewrite.

Fix: call the side effect from the handler that changes the value, or from the query or mutation callback that produces it; if the effect stays, document why the external-system case applies.

### RP3: Audit dependencies and refs on effects that own listeners, observers, fetches, or DOM

Default severity: should-fix
FE parent: FE8
Evidence: 19 comments across 14 PRs

Look for:
- biome-ignore on useExhaustiveDependencies. If the value is stable, add it; if it is not, the effect is wrong.
- An effect that only copies state or a prop into a ref (widthRef.current = width, formValuesRef, dragged.current = true). Ask who reads the ref; usually it exists to dodge a dependency array. Use useEffectEvent or read the value directly.
- addEventListener, ResizeObserver, popup windows, or fetchNextPage set up outside an effect, or inside one with no cleanup, or with cleanup in a different effect than the one that created the resource.
- Helper functions declared inside the effect body and DOM found via querySelector instead of a ref. Pull the function out; attach a ref or ref callback instead.

Fix: make the dependency list honest, delete ref-sync effects, keep setup and cleanup in the same effect, and use a ref callback for measurements that only need the mounted element.

### RP4: Drop manual memoization; React Compiler owns it

Default severity: nit
FE parent: none
Evidence: 19 comments across 12 PRs

Look for:
- Every new useCallback, useMemo, or memo(). site/src is compiled by React Compiler, so each one needs a stated reason, and the reviewers rejected almost all of them.
- memo() on a child whose parent passes an inline arrow: the memo cannot hit.
- useMemo(() => generateId(), []) used for a stable identity. useMemo is a performance hint React may discard; use useState with an initialiser.
- try/catch/finally inside a component body or handler that the compiler must optimise; the compiler bails out on it.
- Functions that capture no component variables defined inside the component. Hoist them to module level.

Fix: remove the memoization and let the compiler do it; keep useMemo only for an external, provably expensive computation and say so in a short comment; use useState for values that must never change.

### RP5: Do not add a hook that a plain function or an existing hook already covers

Default severity: should-fix
FE parent: none
Evidence: 13 comments across 9 PRs

Look for:
- A new use* file with one call site. Ask what breaks if the body is inlined.
- A hook that wraps a single useQuery or useMutation, or that re-fetches data the caller already holds.
- A hook that returns JSX or a render function. That is a component.
- A hook or context provider introduced to avoid passing one value through a few props. Ask whether the value can be computed once at a higher level instead.
- A hook whose only job is a storage flag when a generic storage hook exists.

Fix: replace with a function that takes the already-fetched data or mutations as arguments, or inline it at the call site; keep a hook only when it owns React state or an effect that several components share.

### RP6: Split components that have grown past one screen of logic or markup

Default severity: should-fix
FE parent: none
Evidence: 9 comments across 7 PRs

Look for:
- A diff that adds state, handlers, or derived values to a component that is already several hundred lines. The reviewers flagged the growth even when the file was large before the PR.
- Indentation deeper than roughly six levels of JSX, or a .map callback that builds labels, tooltips, and conditionals inline.
- Nested div.flex trees where no element has a name that says what it is.
- Large page components accumulating request queues, refs, and editing logic that belong in a hook or child component.

Fix: extract named sub-components for each visual block and move non-render logic into helpers or a hook; do it in the PR that adds the growth, or file the split as an explicit follow-up.

### RP7: Forms use formik and mutations, not hand-rolled draft and submitting state

Default severity: should-fix
FE parent: none
Evidence: 8 comments across 4 PRs

Look for:
- Several useState calls holding a draft, a toggled flag, and a "local edit" per field, with a save handler that clears them on success. That is a form; use formik.
- setIsSubmitting(true/false) or an inFlight ref around an API call. useMutation already tracks pending state.
- Submit logic that conditionally calls submitForm or setFieldValue from an onChange handler with no comment. Ask what the branch is for.
- A settings panel with one input and one save button. Still a form; the reviewers accepted skipping formik only for the smallest toggles.

Fix: model the fields with useFormik and drive the submit through a useMutation; read isPending from the mutation and remove the manual flags.

### RP8: Anything that returns JSX is a component, so name and define it as one

Default severity: should-fix
FE parent: none
Evidence: 6 comments across 5 PRs

Look for:
- const renderX = (...) => (<...>) declared inside a component body, or any lowercase function returning ReactNode.
- The same markup block repeated twice with a shared className constant instead of a small component.
- Helpers that take a "tab" or "message" argument and return a fragment; these are components with props.

Fix: promote it to a capitalised component with typed props, at module level or in its own file, and render it as JSX.

## Not this role

- Manual loading flags (setIsExportingAll) around API calls outside a form, and hooks that only wrap useQuery: DF owns the react-query side of these, RP only flags the hook or form shape.
- Measuring panel or menu width with ResizeObserver or clientWidth to set inline sizes that CSS (w-full) could express: SA.
- Component API consistency, such as exporting Title and Message sub-components as children instead of props, or a dialog type prop whose values are used inconsistently: TN.
- Whether a component may be written as an implicit-return arrow expression: TN, as a codebase convention question.
- Mutating a caller-owned object (URLSearchParams) inside a state setter: a React purity rule no role owns yet; flag it under "Outside my role".
