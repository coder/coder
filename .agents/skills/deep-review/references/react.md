# Modern React (18–19.2) + Compiler 1.0 — Reference

Reference for writing idiomatic React. Covers what changed, what it replaced, and what to reach for. Includes React Compiler patterns — what the compiler handles automatically, what it changes semantically, and how to verify its behavior empirically. Scope: client-side SPA patterns only. Server Components, `use server`, and `use client` directives are framework-specific and omitted. Check the project's React version and compiler config before reaching for newer APIs.

## How modern React thinks differently

**Concurrent rendering** (18): React can now pause, interrupt, and resume renders. This is the foundation everything else builds on. Most existing code "just works," but components that produce side effects during render (mutations, subscriptions, network calls in the render body) are unsafe and will misbehave. Concurrent features are opt-in — they only activate when you use a concurrent API like `startTransition` or `useDeferredValue`.

**Urgent vs. non-urgent updates** (18): The `startTransition` / `useTransition` API introduces a formal split between updates that must feel immediate (typing, clicking) and updates that can be interrupted (filtering a large list, navigating to a new screen). Non-urgent updates yield to urgent ones mid-render. Use this instead of `setTimeout` or manual debounce when you want the UI to stay responsive during expensive re-renders.

**Actions** (19): Async functions passed to `startTransition` are called "Actions." They automatically manage pending state, error handling, and optimistic updates as a unit. The `useActionState` hook and `<form action={fn}>` prop are built on this. The pattern replaces the hand-rolled `isPending/setIsPending` + `try/catch` + `setError` boilerplate that was previously necessary for every data mutation.

**Automatic batching** (18): State updates are now batched everywhere — inside `setTimeout`, `Promise.then`, native event handlers, etc. Previously batching only happened inside React-managed event handlers. If you genuinely need a synchronous flush, use `flushSync`.

**Automatic memoization** (Compiler 1.0): React Compiler is a build-time Babel plugin that automatically inserts memoization into components and hooks. It replaces manual `useMemo`, `useCallback`, and `React.memo` — including conditional memoization and memoization after early returns, which manual APIs cannot express. The compiler only processes components and hooks, not standalone functions. It understands data flow and mutability through its own HIR (High-level Intermediate Representation), so it can memoize more granularly than a human would. Projects adopt it incrementally — typically via path-based Babel overrides or the `"use memo"` directive. Components that violate the Rules of React are silently skipped (no build error), so the automated lint tools that check compiler compatibility matter.

## Replace these patterns

The left column reflects patterns common before React 18/19. Write the right column instead. The "Since" column tells you the minimum React version required.

| Old pattern                                                       | Modern replacement                                                             | Since |
| ----------------------------------------------------------------- | ------------------------------------------------------------------------------ | ----- |
| `ReactDOM.render(<App />, el)`                                    | `createRoot(el).render(<App />)`                                               | 18    |
| `ReactDOM.hydrate(<App />, el)`                                   | `hydrateRoot(el, <App />)`                                                     | 18    |
| `ReactDOM.unmountComponentAtNode(el)`                             | `root.unmount()`                                                               | 18    |
| `ReactDOM.findDOMNode(this)`                                      | DOM ref: `const ref = useRef(); ref.current`                                   | 18    |
| `<Context.Provider value={v}>`                                    | `<Context value={v}>`                                                          | 19    |
| `React.forwardRef((props, ref) => ...)`                           | `function Comp({ ref, ...props }) { ... }` (ref as a regular prop)             | 19    |
| String ref `ref="input"` in class components                      | Callback ref or `createRef()`                                                  | 19    |
| `Heading.propTypes = { ... }`                                     | TypeScript / ES6 type annotations                                              | 19    |
| `Component.defaultProps = { ... }` on function components         | ES6 default parameters `({ text = 'Hi' })`                                     | 19    |
| Legacy Context: `contextTypes` + `getChildContext`                | `React.createContext()` + `contextType`                                        | 19    |
| `import { act } from 'react-dom/test-utils'`                      | `import { act } from 'react'`                                                  | 19    |
| `import ShallowRenderer from 'react-test-renderer/shallow'`       | `import ShallowRenderer from 'react-shallow-renderer'`                         | 19    |
| Manual `isPending` state around async calls                       | `const [isPending, startTransition] = useTransition()`                         | 18    |
| Manual optimistic state + revert logic                            | `useOptimistic(currentValue)`                                                  | 19    |
| `useEffect` to subscribe to external stores                       | `useSyncExternalStore(subscribe, getSnapshot)`                                 | 18    |
| Hand-rolled unique ID (counter, random, index)                    | `useId()` — SSR-safe, hydration-safe                                           | 18    |
| `useEffect` to inject `<title>` or `<meta>` / `react-helmet`      | Render `<title>`, `<meta>`, `<link>` directly in components; React hoists them | 19    |
| `ReactDOM.useFormState(action, initial)` (Canary name)            | `useActionState(action, initial)`                                              | 19    |
| `useReducer<React.Reducer<State, Action>>(reducer)`               | `useReducer(reducer)` — infers from the reducer function                       | 19    |
| `<div ref={current => (instance = current)} />` (implicit return) | `<div ref={current => { instance = current }} />` (explicit block body)        | 19    |
| `useRef<T>()` with no argument                                    | `useRef<T>(undefined)` or `useRef<T \| null>(null)` — argument is now required | 19    |
| `MutableRefObject<T>` type annotation                             | `RefObject<T>` — all refs are mutable now; `MutableRefObject` is deprecated    | 19    |
| `React.createFactory('button')`                                   | `<button />` JSX                                                               | 19    |
| `useMemo(() => expr, [deps])` in compiled components              | `const val = expr;` — compiler memoizes automatically                          | C 1.0 |
| `useCallback(fn, [deps])` in compiled components                  | `const fn = () => { ... };` — compiler memoizes automatically                  | C 1.0 |
| `React.memo(Component)` in compiled components                    | Plain component — compiler skips re-render when props are unchanged            | C 1.0 |
| `eslint-plugin-react-compiler` (standalone)                       | `eslint-plugin-react-hooks@latest` (compiler rules merged into recommended)    | C 1.0 |
| `useRef` + `useLayoutEffect` for stable callbacks                 | `useEffectEvent(fn)` — compiler handles both, but `useEffectEvent` is clearer  | 19.2  |

## New capabilities

These enable things that weren't practical before. Reach for them in the described situations.

| What                                                                 | Since   | When to use it                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| -------------------------------------------------------------------- | ------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `useTransition()` / `startTransition()`                              | 18      | Mark a state update as non-urgent so React can interrupt it to handle clicks or keystrokes. The `isPending` boolean lets you show a loading indicator without blocking the UI.                                                                                                                                                                                                                                                                                              |
| `useDeferredValue(value, initialValue?)`                             | 18 / 19 | Defer re-rendering a slow subtree: pass the deferred value as a prop, wrap the expensive child in `memo`. Unlike debounce, uses no fixed timeout — renders as soon as the browser is idle. The `initialValue` arg (19) avoids a flash on first render.                                                                                                                                                                                                                      |
| `useId()`                                                            | 18      | Generate a stable, SSR-consistent ID for accessibility attributes (`htmlFor`, `aria-describedby`). Do not use for list keys.                                                                                                                                                                                                                                                                                                                                                |
| `useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot?)`   | 18      | Subscribe to external (non-React) state stores safely under concurrent rendering. Preferred over `useEffect`-based subscriptions in libraries.                                                                                                                                                                                                                                                                                                                              |
| `useActionState(action, initialState)`                               | 19      | Manage an async mutation: returns `[state, wrappedAction, isPending]`. Handles pending, result, and error state as a unit. Replaces the manual `isPending` + `try/catch` + `setError` pattern.                                                                                                                                                                                                                                                                              |
| `useOptimistic(currentValue)`                                        | 19      | Show a speculative value while an async Action is in flight. Returns `[optimisticValue, setOptimistic]`. React automatically reverts to `currentValue` when the transition settles.                                                                                                                                                                                                                                                                                         |
| `use(promiseOrContext)`                                              | 19      | Read a promise or Context value inside a component or custom hook. Unlike hooks, `use` can be called conditionally (after early returns). Promises must come from a cache — do not create them during render.                                                                                                                                                                                                                                                               |
| `useFormStatus()` (from `react-dom`)                                 | 19      | Read `{ pending, data, method, action }` of the nearest parent `<form>` Action. Works across component boundaries without prop drilling — useful for submit buttons inside design-system components.                                                                                                                                                                                                                                                                        |
| `useEffectEvent(fn)`                                                 | 19.2    | Extract a non-reactive callback from an effect. The function sees the latest props/state without being listed in deps, and is never stale. Replaces the `useRef`-and-mutate-in-layout-effect workaround for stable event-like callbacks. The compiler has built-in knowledge of this hook and correctly prunes its return value from effect dependency arrays. Both `useEffectEvent` and the old ref workaround compile cleanly; `useEffectEvent` is preferred for clarity. |
| `<Activity>`                                                         | 19.2    | Hide part of the UI while preserving its state and DOM. React deprioritizes updates to hidden content. Use via framework APIs for route prerendering or tab preservation — not a direct replacement for CSS `visibility`.                                                                                                                                                                                                                                                   |
| `captureOwnerStack()`                                                | 19.1    | Dev-only API that returns a string showing which components are responsible for rendering the current component (owner stack, not call stack). Useful for custom error overlays. Returns `null` in production.                                                                                                                                                                                                                                                              |
| `<form action={fn}>`                                                 | 19      | Pass an async function as a form's `action` prop. React handles submission, pending state, and automatic form reset on success. Works with `useActionState` and `useFormStatus`.                                                                                                                                                                                                                                                                                            |
| Ref cleanup function                                                 | 19      | Return a cleanup function from a ref callback: `ref={el => { ...; return () => cleanup(); }}`. React calls it on unmount. Replaces the pattern of checking `el === null` in the callback.                                                                                                                                                                                                                                                                                   |
| `<link rel="stylesheet" precedence="default">`                       | 19      | Declare a stylesheet next to the component that needs it. React deduplicates and inserts it in the correct order before revealing Suspense content.                                                                                                                                                                                                                                                                                                                         |
| `preinit`, `preload`, `prefetchDNS`, `preconnect` (from `react-dom`) | 19      | Imperatively hint the browser to load resources early. Call from render or event handlers. React deduplicates hints across the component tree.                                                                                                                                                                                                                                                                                                                              |
| React Compiler (`babel-plugin-react-compiler`)                       | C 1.0   | Build-time automatic memoization for components and hooks. Install, add to Babel/Vite pipeline. Projects typically start with path-based overrides to compile a subset of files.                                                                                                                                                                                                                                                                                            |
| `"use memo"` directive                                               | C 1.0   | Opt a single function into compilation when using `compilationMode: 'annotation'`. Place at the start of the function body. Module-level `"use memo"` at the top of a file compiles all functions in that file.                                                                                                                                                                                                                                                             |
| `"use no memo"` directive                                            | C 1.0   | Temporary escape hatch — skip compilation for a specific component or hook that causes a runtime regression. Not a permanent solution. Place at the start of the function body.                                                                                                                                                                                                                                                                                             |
| Compiler-powered ESLint rules                                        | C 1.0   | Rules for purity, refs, set-state-in-render, immutability, etc. now ship in `eslint-plugin-react-hooks` recommended preset. Surface Rules-of-React violations even without the compiler installed. Note: some projects use Biome instead — check project lint config.                                                                                                                                                                                                       |

## Key APIs

### `useTransition` and `startTransition` (18)

`useTransition` returns `[isPending, startTransition]`. Wrap any state update that is not directly tied to the user's current gesture inside `startTransition`. React will render the old UI while computing the new one, and `isPending` is `true` during that window.

In React 19, `startTransition` can accept an async function (an "Action"). React sets `isPending` to `true` for the entire duration of the async work, not just during the synchronous part.

```tsx
// 18: synchronous transition
const [isPending, startTransition] = useTransition();
startTransition(() => setQuery(input));

// 19: async Action — isPending stays true until the await settles
startTransition(async () => {
	const err = await updateName(name);
	if (err) setError(err);
});
```

Use `startTransition` (the module-level export) when you cannot use the hook (outside a component, in a router callback, etc.).

### `useDeferredValue` (18 / 19)

Creates a "lagging" copy of a value. Pass it to a memoized, expensive component so that React can render the stale UI while computing the updated one.

```tsx
// 19: initialValue shows '' on first render; avoids loading flash
const deferred = useDeferredValue(searchQuery, "");
return <Results query={deferred} />; // Results wrapped in memo
```

`deferred !== searchQuery` while the deferred render is in progress — use this to show a "stale" indicator.

### `useActionState` (19)

Replaces the `useState` + `isPending` + `try/catch` + `setError` boilerplate for any async operation that can be retried or submitted as a form.

```tsx
const [error, submitAction, isPending] = useActionState(
	async (prevState, formData) => {
		const err = await updateName(formData.get("name"));
		if (err) return err; // returned value becomes next state
		redirect("/profile");
		return null;
	},
	null, // initialState
);

// Use submitAction as the form's action prop or call it directly
<form action={submitAction}>
	<input name="name" />
	<button disabled={isPending}>Save</button>
	{error && <p>{error}</p>}
</form>;
```

### `useOptimistic` (19)

Shows a speculative value immediately while an async Action is in progress. React automatically reverts to the server-confirmed value when the Action resolves or rejects.

```tsx
const [optimisticName, setOptimisticName] = useOptimistic(currentName);

const submit = async (formData) => {
	const newName = formData.get("name");
	setOptimisticName(newName); // shows immediately
	await updateName(newName); // reverts if this throws
};
```

### `use()` (19)

Unlike hooks, `use` can appear after conditional statements. Two primary uses:

**Reading a promise** (must be stable — from a cache, not created inline):

```tsx
function Comments({ commentsPromise }) {
	const comments = use(commentsPromise); // suspends until resolved
	return comments.map((c) => <p key={c.id}>{c.text}</p>);
}
```

**Reading context after an early return** (hooks cannot appear after `return`):

```tsx
function Heading({ children }) {
	if (!children) return null;
	const theme = use(ThemeContext); // valid here; hooks would not be
	return <h1 style={{ color: theme.color }}>{children}</h1>;
}
```

### `useSyncExternalStore` (18)

The correct way for libraries (and app code) to subscribe to non-React state. Prevents tearing under concurrent rendering.

```tsx
const value = useSyncExternalStore(
	store.subscribe, // called when store changes
	store.getSnapshot, // returns current value (must be stable reference if unchanged)
	store.getServerSnapshot, // optional: for SSR
);
```

## Verifying compiler behavior

The compiler is a black box unless you inspect its output. When reviewing code in compiled paths, run the compiler on the specific code to see what it actually does. Do not guess — verify.

**Run the compiler on a code snippet:**

```sh
cd site && node -e "
const {transformSync} = require('@babel/core');
const code = \`<paste component here>\`;
const diagnostics = [];
const result = transformSync(code, {
  plugins: [
    ['@babel/plugin-syntax-typescript', {isTSX: true}],
    ['babel-plugin-react-compiler', {
      logger: {
        logEvent(_, event) {
          if (event.kind === 'CompileError' || event.kind === 'CompileSkip') {
            diagnostics.push(event.detail?.toString?.()?.substring(0, 200));
          }
        },
      },
    }],
  ],
  filename: 'test.tsx',
});
console.log('Compiled:', result.code.includes('_c('));
if (diagnostics.length) console.log('Diagnostics:', diagnostics);
console.log(result.code);
"
```

**Reading compiled output:**

- `const $ = _c(N)` — allocates N memoization cache slots.
- `if ($[n] !== dep)` — cache invalidation guard. Re-computes when `dep` changes (referential equality).
- `if ($[n] === Symbol.for("react.memo_cache_sentinel"))` — one-time initialization. Runs once on first render, cached forever after. This is how the compiler handles expressions with no reactive dependencies.
- `_temp` functions — pure callbacks the compiler hoisted out of the component body.

**Check all compiled files at once:**

```sh
cd site && pnpm run lint:compiler
```

This runs the compiler on every file in the compiled paths and reports CompileError / CompileSkip diagnostics. Zero diagnostics means all functions compiled cleanly.

**What the compiler catches vs. what it does not:**

The compiler emits `CompileError` for mutations of props, state, or hook arguments during render, and for `ref.current` access during render. The project's lint pipeline catches these automatically — do not flag them in review.

The compiler does **not** flag impure function calls during render (`Math.random()`, `Date.now()`, `new Date()`). Instead it silently memoizes them with a sentinel guard, freezing the value after first render. This changes semantics without any diagnostic. Verify suspicious calls by running the compiler and checking for sentinel guards in the output.

## Pitfalls

Things that are easy to get wrong even when you know the modern API exists. Check your output against these.

**Effects run twice in development with StrictMode.** React 18 intentionally mounts → unmounts → remounts every component in dev to surface effects that are not resilient to remounting. This is not a bug. If an effect breaks on the second mount, it is missing a cleanup function. Write `return () => cleanup()` from every effect that sets up a subscription, timer, or external resource.

**Concurrent rendering can call render multiple times.** The render function (component body) may be called more than once before React commits to the DOM. Side effects (mutations, subscriptions, logging) in the render body will run multiple times. Move them into `useEffect` or event handlers.

**Do not create promises during render and pass them to `use()`.** A new promise is created every render, causing an infinite suspend-retry loop. Create the promise outside the component (module level), or use a caching library (SWR, React Query, `cache()` from React) to stabilize it.

**`useOptimistic` reverts automatically — do not fight it.** The optimistic value is a presentation layer only. When the Action settles, React replaces it with the real `currentValue` you passed in. Do not try to sync optimistic state back to your real state; let React handle the revert.

**`flushSync` opts out of automatic batching.** If third-party code or a browser API (e.g. `ResizeObserver`) calls `setState` and you need synchronous DOM flushing, wrap with `flushSync(() => setState(...))`. This is a last resort; prefer letting React batch.

**`forwardRef` still works in React 19 but will be deprecated.** Function components accept `ref` as a plain prop now. New code should use the prop directly. Existing `forwardRef` wrappers continue to work without changes; migrate when convenient.

**`<Activity>` does not unmount.** Content inside a hidden `<Activity>` boundary stays mounted. Effects keep running. Use it for preserving scroll position or form state, not for preventing expensive mounts — use lazy loading for that.

**TypeScript: implicit returns from ref callbacks are now type errors.** In React 19, returning anything other than a cleanup function (or nothing) from a ref callback is rejected by the TypeScript types. The most common case is arrow-function refs that implicitly return the DOM node:

```tsx
// Error in React 19 types:
<div ref={el => (instance = el)} />

// Fix — use a block body:
<div ref={el => { instance = el; }} />
```

**TypeScript: `useRef` now requires an argument.** `useRef<T>()` with no argument is a type error. Pass `undefined` for mutable refs or `null` for DOM refs you initialize on mount: `useRef<T>(undefined)` / `useRef<HTMLDivElement | null>(null)`.

**`useId` output format changed across versions.** React 18 produced `:r0:`. React 19.1 changed it to `«r0»`. React 19.2 changed it again to `_r0`. Do not parse or depend on the specific format — treat it as an opaque string.

**`useFormStatus` reads the nearest parent `<form>` with a function `action`.** It does not reflect native HTML form submissions — only React Actions. A submit button that is a sibling of `<form>` (rather than a descendant) will not see the form's status.

**Context as a provider (`<Context>`) requires React 19; `<Context.Provider>` still works.** Do not use `<Context>` shorthand in a codebase that needs to support React 18. The two forms can coexist during migration.

**Compiler freezes impure expressions silently.** `Math.random()`, `Date.now()`, `new Date()`, and `window.innerWidth` in a component body all compile without diagnostics. The compiler wraps them in a sentinel guard (`Symbol.for("react.memo_cache_sentinel")`) that runs the expression once and caches the result forever. The value never updates on re-render. Fix: move to a `useState` initializer (`useState(() => Math.random())`), `useEffect`, or event handler.

**Component granularity affects compiler optimization.** When one pattern in a component causes a `CompileError` (e.g., a necessary `ref.current` read during render), the compiler skips the **entire** component. If the rest of the component would benefit from compilation, extract the non-compilable pattern into a small child component. This keeps the parent compiled.

**The compiler only memoizes components and hooks.** Standalone utility functions (even expensive ones called during render) are not compiled. If a utility function is truly expensive, it still needs its own caching strategy outside of React (e.g., a module-level cache, `WeakMap`, etc.).

**Changing memoization can shift `useEffect` firing.** A value that was unstable before compilation may become stable after, causing an effect that depended on it to fire less often. Conversely, future compiler changes may alter memoization granularity. Effects that use memoized values as dependencies should be resilient to these changes — they should be true synchronization effects, not "run this when X changes" hacks.

## Behavioral changes that affect code

- **Automatic batching** (18): State updates in `setTimeout`, `Promise.then`, `addEventListener` callbacks, etc. are now batched into a single re-render. Previously only React synthetic event handlers were batched. Code that relied on unbatched updates (reading DOM synchronously after each `setState`) must use `flushSync`.

- **StrictMode double-invoke** (18): In development, every component is mounted → unmounted → remounted with the previous state. Every effect runs cleanup → setup twice on initial mount. `useMemo` and `useCallback` also double-invoke their functions. Production behavior is unchanged. If a test or component breaks under this, the component had a latent cleanup bug.

- **StrictMode ref double-invoke** (19): In development, ref callbacks are also invoked twice on mount (attach → detach → attach). Return a cleanup function from the ref callback to handle detach correctly.

- **StrictMode memoization reuse** (19): During the second pass of double-rendering, `useMemo` and `useCallback` now reuse the cached result from the first pass instead of calling the function again. Components that are already StrictMode-compatible should not notice a difference.

- **Suspense fallback commits immediately** (19): When a component suspends, React now commits the nearest `<Suspense>` fallback without waiting for sibling trees to finish rendering. After the fallback is shown, React "pre-warms" suspended siblings in the background. This makes fallbacks appear faster but changes the order of rendering work.

- **Error re-throwing removed** (19): Errors that are not caught by an Error Boundary are now reported to `window.reportError` (not re-thrown). Errors caught by an Error Boundary go to `console.error` once. If your production monitoring relied on the re-thrown error, add handlers to `createRoot`: `createRoot(el, { onUncaughtError, onCaughtError })`.

- **Transitions in `popstate` are synchronous** (19): Browser back/forward navigation triggers synchronous transition flushing. This ensures the URL and UI update together atomically during history navigation.

- **`useEffect` from discrete events flushes synchronously** (18): Effects triggered by a click or keydown (discrete events) are now flushed synchronously before the browser paints, consistent with `useLayoutEffect` for those cases.

- **Hydration mismatches treated as errors** (18 / improved in 19): Text content mismatches between server HTML and client render revert to client rendering up to the nearest `<Suspense>` boundary. React 19 logs a single diff instead of multiple warnings, making mismatches much easier to diagnose.

- **New JSX transform required** (19): The automatic JSX runtime introduced in 2020 (`react/jsx-runtime`) is now mandatory. The classic transform (which required `import React from 'react'` in every file) is no longer supported. Most toolchains have already shipped the new transform; check your Babel or TypeScript config if you see warnings.

- **UMD builds removed** (19): React no longer ships UMD bundles. Load via npm and a bundler, or use an ESM CDN (`import React from "https://esm.sh/react@19"`).

- **React Compiler automatic memoization** (Compiler 1.0): Build-time Babel plugin that inserts memoization into components and hooks. Components that follow the Rules of React are automatically memoized; components that violate them are silently skipped (no build error, no runtime change). The compiler can memoize conditionally and after early returns — things impossible with manual `useMemo`/`useCallback`. Works with React 17+ via `react-compiler-runtime`; best with React 19+. Projects adopt incrementally via path-based Babel overrides, `compilationMode: 'annotation'`, or the `"use memo"` / `"use no memo"` directives. Check the project's Vite/Babel config to know which paths are compiled. Compiled components show a "Memo ✨" badge in React DevTools.
