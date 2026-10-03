# Data Flow (DF)

Lens: how server and persisted data enters and leaves the UI: every read goes through a query defined in `api/queries/`, every write goes through `useMutation`, keys are imported not retyped, every view handles loading, error, empty, and refetch without discarding what the user typed or selected, and browser storage is a last resort with explicit keys and lifecycle.

Boundaries: effect mechanics, refs, hook shape, and form libraries belong to RP; `??`/`||` coercions and optional-versus-required fields belong to TN; story and test structure, play functions, and Mock* fixtures belong to TS; "use the existing helper" and file placement belong to RO; wrapper removal and inlining belong to SD.

Reading order: open changed files under `site/src/api/queries/` and `site/src/api/api.ts` first, then the pages, hooks, and contexts that consume them. Read the whole query file, not the hunk, to learn which key constants and `*Key()` helpers already exist and what neighbouring mutations invalidate. Then open every `.stories.tsx` and `.test.ts` touched and read its `parameters.queries` block. Grep the diff for `API.`, `useQuery(`, `useMutation(`, `mutateAsync`, `onSuccess`, `invalidateQueries`, `useState(false)`, `useRef(false)`, `localStorage`, `sessionStorage`, `defineStorageKey`, and `useStorage`. For storage changes also read `site/src/storage/index.ts` and `site/src/utils/storage/keys.ts`.

## Checkpoints

### DF1: Import and export query keys, never rebuild them

Default severity: should-fix
FE parent: FE7
Evidence: 10 comments across 9 PRs

Look for:
- A string array literal used as a query key anywhere outside `api/queries/` (component, context, story, test): `queryKey: ["get-proxies", ...]`, `const myKey = ["me", "appearance"]`, `key: ["chat-model-configs"]`.
- A story or test calling a query options builder only to read `.queryKey` (`mcpServerConfigs(id).queryKey`, `preferenceSettings().queryKey`) when a key constant or `*Key(arg)` helper exists or could be exported.
- A key constant or key helper that the diff makes non-exported, or that is exported without `as const` while its siblings in the same file use it.
- Two key definition styles inside one PR (with and without `as const`, constant versus helper) for the same shape of key.

Fix: define the key once next to the query options in `api/queries/`, export it (constant or `*Key(arg)` helper), and import that everywhere including stories and tests. Match the `as const` convention already used in the file.

### DF2: Writes go through useMutation, not hand-managed flags

Default severity: should-fix
FE parent: FE7
Evidence: 9 comments across 6 PRs

Look for:
- An `async` handler that calls `API.*` directly and brackets it with `setIsSubmitting(true/false)`, `setIsExportingAll`, or a `useRef(false)` in-flight guard, usually inside a `try/catch/finally`.
- A component that receives or derives its own pending boolean when the `useMutation` it wraps already exposes `isPending`.
- Request ordering or dedupe built by hand: a `useRef<Promise>` chain, `.then(() => mutate(...))`, or a queue of pending submits sitting next to a mutation.
- `AbortController` plumbing around an `API.*` call inside a component or hook that is not a query function.

Fix: move the call into a `useMutation` (defined in `api/queries/` when it is reusable), read `isPending` and `error` from the mutation, and delete the local flags, refs, and promise chains. For forms, let the form library own submitting state (RP owns the form choice).

### DF3: Handle the mutation result in one place and invalidate precisely

Default severity: should-fix
FE parent: FE7
Evidence: 9 comments across 4 PRs

Look for:
- Per-call `mutate(id, { onSuccess, onError })` blocks inside JSX or click handlers that close dialogs, toast, and navigate; especially several copies in one file.
- `try/catch` around `mutateAsync` written inline in markup, or a `catch` that only toasts, when the `useMutation` definition already has (or should have) `onSuccess`/`onError`.
- Hand-written `MutationOptions<Awaited<ReturnType<typeof API.x>>, unknown, {...}>` generics instead of `mutationOptions(...)`.
- `invalidateQueries` after a mutation that hits a broader key than the entity changed (the whole list for a single-item update) with no stated reason, or that is missing on one of two chained requests.

Fix: pick one style per file: either callbacks defined once on the `useMutation` (adding to, not overwriting, existing `onSuccess`/`onError`), or `await mutateAsync()` followed by the success work with a real `catch`. Wrap options in `mutationOptions`. Invalidate the narrowest key that covers what changed and say why when it must be wider.

### DF4: Queries live in api/queries and React Query drives the fetching

Default severity: should-fix
FE parent: FE7
Evidence: 5 comments across 4 PRs

Look for:
- `useQuery({ queryKey, queryFn: () => API.x(...) })` spelled out inside a component, context, or feature hook instead of spreading an options object from `api/queries/`.
- A `use*` hook whose body is a single `useQuery` call plus a return; it should be query options, not a hook.
- `await API.*` reads inside event handlers or helper functions in `pages/` or `modules/` (lookups before a write, existence checks) with no query or mutation around them.
- A `useEffect` that calls `refetch()` or `fetchNextPage()` to drain or re-run a query based on other query state.

Fix: define the query options (key, `queryFn`, `select`, `refetchInterval`, `enabled`) in the matching `api/queries/*.ts` file and consume them with `useQuery(options)`. Reads needed before a write belong in the mutation function or in a query the component already holds; ask whether the endpoint should return the full set instead of paging in an effect.

### DF5: Every UI state is handled and decided from the real data

Default severity: should-fix
FE parent: FE5
Evidence: 4 comments across 3 PRs

Look for:
- A view with more than one empty state (no permission, no results, filter matched nothing): check which one wins and whether "you have items but the search matched none" is distinguishable from "you have none".
- `query.data?.field ?? ""` or `?? fallbackProp` feeding a child that needs a real value: ask what renders while `data` is undefined and whether the undefined case is a loading, error, or absent state that needs its own branch.
- Show/hide logic for banners and hints derived by comparing a query or filter string to a hardcoded literal; ask what happens for equivalent inputs written differently.
- A story matrix that covers loading and error but not the empty variants the code branches on.

Fix: branch explicitly on `isLoading`, `error`, and each empty condition with deliberate copy, derive visibility from parsed data rather than string equality, and stop coalescing missing server data into placeholder values (TN owns the coercion style, DF owns the missing branch).

### DF6: Never clobber user state on refetch or reconciliation

Default severity: should-fix
FE parent: FE5
Evidence: 3 comments across 2 PRs

Look for:
- `queryClient.setQueryData` on a list key that merges local, queued, or optimistic entries with server pages; trace what happens to an entry the user just added or edited when the next refetch or stream event lands.
- Suppression or promotion bookkeeping (`suppressQueuedMessageID`, "promoted head", pending id sets) spread across a store and a page: ask where the single source of truth for in-progress user input is.
- Storage or cache expiry that deletes drafts, selections, or half-finished edits on a timer or sweep rather than on user action.
- Forms or selections initialised from `query.data` without a key or guard, so a background refetch re-initialises them.

Fix: keep in-progress user input in one owner (form state or a store) that the server merge reads but never overwrites, reconcile server data against it in one obvious function, and do not expire user drafts without a user-facing reason.

### DF7: Browser storage is a last resort with explicit keys and lifecycle

Default severity: should-fix
FE parent: none
Evidence: 10 comments across 5 PRs

Look for:
- A new `localStorage`/`sessionStorage` key or `defineStorageKey` for a user preference (button variant, layout width, full-width toggle, flags): ask whether it should be a persisted user setting in the database like existing preference settings.
- Keys that are not scoped by user id, organization id, or workspace id when the value differs per user or resource; and legacy-key migration or removal done inline in a component (`removeItem(legacyKey)`) or via a registered "legacy keys" sweep with no owner.
- Read paths that ignore the expiry the write path records, expiry on data the user expects to keep (drafts), and helpers that subscribe to both a custom change event and the `storage` event for the same key.
- `useEffect` that mirrors React state into storage with a "last persisted" ref, and one-off `use*Flag` hooks that wrap a single storage key instead of the shared storage hook.

Fix: prefer a database-backed user setting for anything users expect to follow them; when storage is right, declare the key once through the shared storage module with scoping in the key, write it in the event handler, make `get` honour expiry, and put migrations and legacy cleanup in the storage layer rather than in components.

## Not this role

- Manual submit handling in a form component that should use formik or the form library (`setIsSubmitting` inside a form, "should we be using `useFormik`"): RP owns the form choice; DF only owns the mutation behind it.
- `useEffect` plus `useRef` used to persist width to storage, and effects that call `refetch()` or drain `fetchNextPage()`: RP owns the effect misuse; DF flags the data consequence.
- Whether a `use*` hook should exist at all (single-purpose flag hooks, hooks that should be plain functions): RP.
- `?? ""` and `?? organization` fallbacks on nullable query data: TN owns the coercion; DF owns the missing-data branch.
- Story query wiring that pre-builds a query object instead of sharing a `Mock*` entity constant, and stories that read `.queryKey` from options builders: TS owns the fixture shape; DF owns the key export.
- An API client function that drops fields of an expanded generated filter type, and URL search param handling in a page: DF-adjacent one-offs; TN for the type-widening miss, RP for URL-as-state placement.
