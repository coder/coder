# Data Flow (DF)

Lens: how server and persisted data enters and leaves the UI: every read goes through a query defined in `api/queries/`, every write goes through `useMutation`, keys are imported not retyped, every view handles loading, error, empty, and refetch without discarding what the user typed or selected, and browser storage is a last resort with explicit keys and lifecycle.

Boundaries: effect mechanics, refs, hook shape, and form libraries belong to RP; `??`/`||` coercions and optional-versus-required fields belong to TN; story and test structure, play functions, and Mock* fixtures belong to TS; "use the existing helper" and file placement belong to RO; wrapper removal and inlining belong to SD.

Reading order: open changed files under `site/src/api/queries/` and `site/src/api/api.ts` first, then the pages, hooks, and contexts that consume them. Read the whole query file, not the hunk, to learn which key constants and `*Key()` helpers already exist and what neighbouring mutations invalidate. Then open every `.stories.tsx` and `.test.ts` touched and read its `parameters.queries` block. Grep the review target's diff for `API.`, `useQuery(`, `useMutation(`, `mutateAsync`, `onSuccess`, `invalidateQueries`, `useState(false)`, `useRef(false)`, `localStorage`, and `sessionStorage`. Browser storage has no shared wrapper: features call `localStorage` directly, usually from a small module next to their code (for example `site/src/pages/AgentsPage/utils/draftStorage.ts`). For storage changes, read the feature's existing storage code to see how its keys are prefixed and scoped.

## Checkpoints

### DF1: Import and export query keys, never rebuild them

Default severity: should-fix
FE parent: FE7

Look for:

- A string array literal used as a query key anywhere outside `api/queries/` (component, context, story, test): `queryKey: ["get-proxies", ...]`, `const myKey = ["me", "appearance"]`, `key: ["chat-model-configs"]`.
- A story or test calling a query options builder only to read `.queryKey` (`mcpServerConfigs(id).queryKey`, `preferenceSettings().queryKey`) when a key constant or `*Key(arg)` helper exists or could be exported.
- A key constant or key helper that the diff makes non-exported, or that is exported without `as const` while its siblings in the same file use it.
- Two key definition styles inside one PR (with and without `as const`, constant versus helper) for the same shape of key.

Fix: define the key once next to the query options in `api/queries/`, export it (constant or `*Key(arg)` helper), and import that everywhere including stories and tests. Match the `as const` convention already used in the file.

### DF2: Writes go through useMutation, not hand-managed flags

Default severity: should-fix
FE parent: FE7

Look for:

- An `async` handler that calls `API.*` directly and brackets it with `setIsSubmitting(true/false)`, `setIsExportingAll`, or a `useRef(false)` in-flight guard, usually inside a `try/catch/finally`.
- A component that receives or derives its own pending boolean when the `useMutation` it wraps already exposes `isPending`.
- Request ordering built by hand: a `useRef<Promise>` chain, `.then(() => mutate(...))`, or a queue of pending submits sitting next to a mutation. Mutations that share a `scope` id already run one at a time. A ref guard next to a mutation that ignores a repeated event while a request is in flight is correct, because every `mutate` call starts a new request (`inFlightBootstrapRef` in `AgentEmbedPage.tsx` ignores repeated bootstrap messages).
- `AbortController` plumbing around a read that a query could own: a query function receives a `signal` and React Query cancels it. A controller that lets a mutation be cancelled on a repeat click or unmount is correct, because `useMutation` cancels nothing (`ExportAllDebugRunsButton` in `DebugPanel.tsx`).

Fix: move the call into a `useMutation` (defined in `api/queries/` when it is reusable), read `isPending` and `error` from the mutation, and delete the local flags, refs, and promise chains that duplicate them; give calls that must run in order a shared `scope` id. Move a read with its own `AbortController` into a `useQuery` keyed by what it reads, whose query function passes React Query's `signal` (DF4), so React Query cancels the old read when the key changes or the component unmounts; do not move it into a mutation. For forms, let the form library own submitting state (RP owns the form choice).

### DF3: Handle the mutation result in one place and invalidate precisely

Default severity: should-fix
FE parent: FE7

Look for:

- The same per-call `mutate(id, { onSuccess, onError })` callbacks repeated at several call sites. One call site that toasts and closes its dialog in a per-call `onSuccess` is the default FE7 asks for (`MCPServerSharingDialog.tsx`).
- `await mutateAsync()` where no later step needs the result for control flow (FE7 asks for `mutate()` with callbacks there), `try/catch` around it written inline in markup, or a `catch` that only toasts when the `useMutation` definition already has (or should have) `onSuccess`/`onError`.
- Hand-written `MutationOptions<Awaited<ReturnType<typeof API.x>>, unknown, {...}>` generics instead of `mutationOptions(...)`.
- `invalidateQueries` on a key that also covers queries the mutation cannot have changed, with no stated reason, or a missing invalidation for a query that holds the changed data (the list that shows an updated item, the second of two chained requests).

Fix: use `mutate()` with `onSuccess`/`onError`. Define a callback the call sites share once on the `useMutation` when it must run whatever the caller does, such as a cache update (add to, do not overwrite, existing callbacks). Keep UI steps such as navigating or closing a dialog in per-call callbacks, which TanStack Query skips once the calling component unmounts, and share them through a helper when several call sites repeat them. Keep `await mutateAsync()` only where the caller needs the result for control flow, with a `catch` that handles the error. Wrap options in `mutationOptions`. Invalidate every query that holds data the mutation changed, as FE7 requires (`updateTemplateMeta` in `api/queries/templates.ts` invalidates both the list and the detail keys), and nothing broader without a stated reason.

### DF4: Queries live in api/queries and React Query drives the fetching

Default severity: should-fix
FE parent: FE7

Look for:

- `useQuery({ queryKey, queryFn: () => API.x(...) })` spelled out inside a component, context, or feature hook instead of spreading an options object from `api/queries/`.
- A `use*` hook whose body is a single `useQuery` call plus a return; it should be query options, not a hook.
- `await API.*` reads inside event handlers or helper functions in `pages/` or `modules/` (lookups before a write, existence checks) with no query or mutation around them.
- A `useEffect` that calls `refetch()` or `fetchNextPage()` to drain or re-run a query based on other query state.

Fix: define the query options (key, `queryFn`, `select`, `refetchInterval`, `enabled`) in the matching `api/queries/*.ts` file and consume them with `useQuery(options)`. Reads needed before a write belong in the mutation function or in a query the component already holds; ask whether the endpoint should return the full set instead of paging in an effect.

### DF5: Every UI state is handled and decided from the real data

Default severity: should-fix
FE parent: FE5

Look for:

- A view with more than one empty state (no permission, no results, filter matched nothing): check which one wins and whether "you have items but the search matched none" is distinguishable from "you have none".
- `query.data?.field ?? ""` or `?? fallbackProp` feeding a child that needs a real value: ask what renders while `data` is undefined and whether the undefined case is a loading, error, or absent state that needs its own branch.
- Show/hide logic for banners and hints derived by comparing a query or filter string to a hardcoded literal; ask what happens for equivalent inputs written differently.
- A story matrix that covers loading and error but not the empty variants the code branches on.

Fix: branch explicitly on `isLoading`, on `error` when there is no usable data, and on each empty condition with deliberate copy. When a background refetch fails, keep the cached data on screen and show the error beside it, because FE5 keeps valid data visible on refetch. Derive visibility from parsed data rather than string equality, and stop coalescing data that has not loaded yet into placeholder values (TN owns the coercion style, DF owns the missing branch). A nullable field the server did return still renders a visible fallback such as "N/A" (FE5).

### DF6: Never clobber user state on refetch or reconciliation

Default severity: should-fix
FE parent: FE5

Look for:

- `queryClient.setQueryData` on a list key that merges local, queued, or optimistic entries with server pages; trace what happens to an entry the user just added or edited when the next refetch or stream event lands.
- Suppression or promotion bookkeeping (`suppressQueuedMessageID`, "promoted head", pending id sets) spread across a store and a page: ask where the single source of truth for in-progress user input is.
- Storage or cache expiry that deletes drafts, selections, or half-finished edits on a timer or sweep rather than on user action.
- Forms or selections that an effect or formik's `enableReinitialize` re-seeds from `query.data`, so a background refetch overwrites in-progress edits. A plain `useState(query.data)` initialiser runs once and does not re-seed (`DefaultModelSettings.tsx` keeps its unsaved selection in such state for this reason).

Fix: keep in-progress user input in one owner (form state or a store) that the server merge reads but never overwrites, reconcile server data against it in one obvious function, and do not expire user drafts without a user-facing reason.

### DF7: Browser storage is a last resort with explicit keys and lifecycle

Default severity: should-fix
FE parent: none

Look for:

- A new `localStorage` or `sessionStorage` key for a user preference (button variant, layout width, full-width toggle, flags): ask whether it should be a persisted user setting in the database like existing preference settings.
- Keys that are not scoped by user id, organization id, or workspace id when the value differs per user or resource; and legacy-key migration or removal done inline in a component (`removeItem(legacyKey)`) or via a registered "legacy keys" sweep with no owner.
- Read paths that ignore the expiry the write path records, and expiry on data the user expects to keep (drafts). A custom change event next to the native `storage` event is not a duplicate: the native event fires only in other tabs.
- `useEffect` that mirrors React state into storage with a "last persisted" ref when one event handler is the only thing that changes the state and could write it instead.

Fix: prefer a database-backed user setting for anything users expect to follow them; when storage is right, define the key once next to the code that reads it, with the user or resource scope in the key, write it in the event handler when one handler changes the value (an effect stays the write point when several code paths do, per RP2), make reads honour any expiry the write records, and keep migrations and legacy cleanup with the key definition rather than in components.

## Not this role

- Manual submit handling in a form component that should use formik or the form library (`setIsSubmitting` inside a form, "should we be using `useFormik`"): RP owns the form choice; DF only owns the mutation behind it.
- A `useEffect` plus `useRef` guard that persists state to storage, and effects that call `refetch()` or drain `fetchNextPage()`: RP owns the effect misuse; DF flags the data consequence.
- Whether a `use*` hook should exist at all (single-purpose flag hooks, hooks that should be plain functions): RP.
- `?? ""` and `?? organization` fallbacks on nullable query data: TN owns the coercion; DF owns the missing-data branch.
- Story query wiring that pre-builds a query object instead of sharing a `Mock*` entity constant, and stories that read `.queryKey` from options builders: TS owns the fixture shape; DF owns the key export.
- An API client function that drops fields of an expanded generated filter type, and URL search param handling in a page: DF-adjacent one-offs; TN for the type-widening miss, RP for URL-as-state placement.
