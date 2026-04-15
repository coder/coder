# Modern TypeScript (5.0–6.0 RC) — Reference

Reference for writing idiomatic TypeScript. Covers what changed, what it replaced, and what to reach for. Respect the project's minimum TypeScript version: don't emit features from a version newer than what the project targets. Check `package.json` and `tsconfig.json` before writing code.

## How modern TypeScript thinks differently

The 5.x era resolves years of module system ambiguity and cleans house on legacy options. Three themes dominate:

**Module semantics are explicit.** `--verbatimModuleSyntax` (5.0) makes import/export intent visible in source: type imports must carry `type`, value imports stay. Combined with `--module preserve` or `--moduleResolution bundler`, the compiler now accurately models what bundlers and modern runtimes actually do. `import defer` (5.9) extends the model to deferred evaluation.

**Resource lifetimes are first-class.** `using` and `await using` (5.2) provide deterministic cleanup without `try/finally`. Any object implementing `Symbol.dispose` participates. `DisposableStack` handles ad-hoc multi-resource cleanup in functions where creating a full class is overkill.

**Inference is smarter about what it knows.** Inferred type predicates (5.5) let `.filter(x => x !== undefined)` produce `T[]` instead of `(T | undefined)[]` automatically. `NoInfer<T>` (5.4) gives library authors precise control over which parameters drive inference. Narrowing now survives closures after last assignment, constant indexed accesses, and `switch (true)` patterns.

**TypeScript 6.0 is a transition release toward 7.0** (the Go-native port). It turns years of soft deprecations into errors and changes several defaults. Most impactful: `types` defaults to `[]` (must list `@types` packages explicitly), `rootDir` defaults to `.`, `strict` defaults to `true`, `module` defaults to `esnext`. Projects relying on implicit behavior need explicit config. Check the deprecations section before upgrading.

## Replace these patterns

The left column reflects patterns still common before TypeScript 5.x. Write the right column instead. The "Since" column tells you the minimum TypeScript version required.

| Old pattern                                                                                                      | Modern replacement                                                                                                           | Since  |
| ---------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- | ------ |
| `--experimentalDecorators` + legacy decorator signatures                                                         | Standard decorators (TC39): `function dec(target, context: ClassMethodDecoratorContext)` — no flag needed                    | 5.0    |
| Requiring callers to add `as const` at call sites                                                                | `<const T extends HasNames>(arg: T)` — `const` modifier on type parameter                                                    | 5.0    |
| `--importsNotUsedAsValues` + `--preserveValueImports`                                                            | `--verbatimModuleSyntax`                                                                                                     | 5.0    |
| `import { Foo } from "..."` when `Foo` is only used as a type                                                    | `import { type Foo } from "..."` or `import type { Foo } from "..."`                                                         | 5.0    |
| `"extends": "@tsconfig/strictest/tsconfig.json"` chain                                                           | `"extends": ["@tsconfig/strictest/tsconfig.json", "./tsconfig.base.json"]` (array form)                                      | 5.0    |
| `try { ... } finally { resource.close(); resource.delete(); }`                                                   | `using resource = acquireResource()` — calls `[Symbol.dispose]()` automatically                                              | 5.2    |
| `try { ... } finally { await resource.close() }`                                                                 | `await using resource = acquireAsyncResource()`                                                                              | 5.2    |
| Ad-hoc cleanup with multiple `try/finally` blocks                                                                | `using cleanup = new DisposableStack(); cleanup.defer(() => ...)`                                                            | 5.2    |
| `import data from "./data.json" assert { type: "json" }`                                                         | `import data from "./data.json" with { type: "json" }`                                                                       | 5.3    |
| `.filter(Boolean)` or `.filter(x => !!x)` to remove nulls                                                        | `.filter(x => x !== undefined)` or `.filter(x => x !== null)` (infers type predicate)                                        | 5.5    |
| Extra phantom type param to block inference bleed: `<C extends string, D extends C>`                             | `NoInfer<C>` on the parameter you don't want to drive inference                                                              | 5.4    |
| `/** @typedef {import("./types").Foo} Foo */` in JS files                                                        | `/** @import { Foo } from "./types" */` (JSDoc `@import` tag)                                                                | 5.5    |
| `myArray.reverse()` mutating in place                                                                            | `myArray.toReversed()` (returns new array)                                                                                   | 5.2    |
| `myArray.sort(cmp)` mutating in place                                                                            | `myArray.toSorted(cmp)` (returns new array)                                                                                  | 5.2    |
| `const copy = [...arr]; copy[i] = v`                                                                             | `arr.with(i, v)` (returns new array)                                                                                         | 5.2    |
| Manual `has`/`get`/`set` pattern on `Map`                                                                        | `map.getOrInsert(key, defaultValue)` or `getOrInsertComputed(key, fn)`                                                       | 6.0 RC |
| `new RegExp(str.replace(/[.\*+?^${}()\[\]\\]/g, '\\$&'))`                                                        | `new RegExp(RegExp.escape(str))`                                                                                             | 6.0 RC |
| `--moduleResolution node` (node10)                                                                               | `--moduleResolution nodenext` (Node.js) or `--moduleResolution bundler` (bundlers/Bun)                                       | 6.0 RC |
| `"baseUrl": "./src"` + `"@app/*": ["app/*"]` in paths                                                            | Remove `baseUrl`; use `"@app/*": ["./src/app/*"]` in paths directly                                                          | 6.0 RC |
| `module Foo { export const x = 1; }`                                                                             | `namespace Foo { export const x = 1; }`                                                                                      | 6.0 RC |
| `export * from "..."` when all re-exported members are types                                                     | `export type * from "..."` (or `export type * as ns from "..."`)                                                             | 5.0    |
| `function f(): undefined { return undefined; }` — explicit return required in `: undefined`-returning function   | Remove the `return` entirely; `undefined`-returning functions no longer require any return statement                         | 5.1    |
| Manual type predicate annotation on a simple arrow: `(x: T \| undefined): x is T => x !== undefined`             | Remove the annotation; TypeScript infers `x is T` from `!== null/undefined` and `instanceof` checks automatically            | 5.5    |
| `const val = obj[key]; if (typeof val === "string") { use(val); }` — extract to const to narrow indexed access   | `if (typeof obj[key] === "string") { obj[key].toUpperCase(); }` directly — both `obj` and `key` must be effectively constant | 5.5    |
| Copy narrowed `let`/param to a `const`, or restructure code to escape stale closure narrowing after reassignment | Remove the copy; narrowing survives into closures created after the last assignment to the variable                          | 5.4    |
| `(arr as string[]).filter(...)` or restructure to avoid "not callable" errors on `string[] \| number[]`          | Call `.filter`, `.find`, `.some`, `.every`, `.reduce` directly on union-of-array types                                       | 5.2    |
| `if`/`else` chain used to work around lack of narrowing inside a `switch (true)` body                            | `switch (true)` — each `case` condition now narrows the tested variable in its clause                                        | 5.3    |

## New capabilities

These enable things that weren't practical before. Reach for them in the described situations.

| What                                            | Since  | When to use it                                                                                                                                                                                                                                |
| ----------------------------------------------- | ------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `using` / `await using` declarations            | 5.2    | Any resource needing deterministic cleanup (file handles, DB connections, locks, event listeners). Object must implement `Symbol.dispose` / `Symbol.asyncDispose`.                                                                            |
| `DisposableStack` / `AsyncDisposableStack`      | 5.2    | Ad-hoc multi-resource cleanup without creating a class. Call `.defer(fn)` right after acquiring each resource. Stack disposes in LIFO order.                                                                                                  |
| `const` modifier on type parameters             | 5.0    | Force `const`-like (literal/readonly tuple) inference at call sites without requiring callers to write `as const`. Constraint must use `readonly` arrays.                                                                                     |
| Decorator metadata (`Symbol.metadata`)          | 5.2    | Attach and read per-class metadata from decorators via `context.metadata`. Retrieved as `MyClass[Symbol.metadata]`. Requires `Symbol.metadata ??= Symbol(...)` polyfill.                                                                      |
| `NoInfer<T>` utility type                       | 5.4    | Prevent a parameter from contributing inference candidates for `T`. Use when one argument should be the "source of truth" and others should only be checked against it.                                                                       |
| Inferred type predicates                        | 5.5    | Filter callbacks that test for `!== null` or `instanceof` now automatically produce a type predicate. `Array.prototype.filter` then narrows the result array type.                                                                            |
| `--isolatedDeclarations`                        | 5.5    | Require explicit return types on exported declarations. Unlocks parallel declaration emit by external tooling (esbuild, oxc, etc.) without needing a full type-checker pass.                                                                  |
| `${configDir}` in tsconfig paths                | 5.5    | Anchor `typeRoots`, `paths`, `outDir`, etc. in a shared base tsconfig to the _consuming_ project's directory, not the shared file's location.                                                                                                 |
| Always-truthy/nullish check errors              | 5.6    | Catches regex literals in `if`, arrow functions as comparators, `?? 100` on non-nullable left side, misplaced parentheses. No API to call; existing bugs now surface as errors.                                                               |
| Iterator helper methods (`IteratorObject`)      | 5.6    | Built-in iterators from `Map`, `Set`, generators, etc. now have `.map()`, `.filter()`, `.take()`, `.drop()`, `.flatMap()`, `.toArray()`, `.reduce()`, etc. Use `Iterator.from(iterable)` to wrap any iterable.                                |
| `--noUncheckedSideEffectImports`                | 5.6    | Error when a side-effect import (`import "..."`) resolves to nothing. Catches typos in polyfill or CSS imports.                                                                                                                               |
| `--noCheck`                                     | 5.6    | Skip type checking entirely during emit. Useful for separating "fast emit" from "thorough check" pipeline stages, especially with `--isolatedDeclarations`.                                                                                   |
| `--rewriteRelativeImportExtensions`             | 5.7    | Rewrite `.ts`→`.js`, `.tsx`→`.jsx`, `.mts`→`.mjs`, `.cts`→`.cjs` in relative imports during emit. Required when writing `.ts` imports for Node.js strip-types mode and still needing `.js` output for library distribution.                   |
| `--erasableSyntaxOnly`                          | 5.8    | Error on constructs that can't be type-stripped by Node.js `--experimental-strip-types`: `enum`, `namespace` with code, parameter properties, `import =` aliases.                                                                             |
| `require()` of ESM under `--module nodenext`    | 5.8    | Node.js 22+ allows CJS to `require()` ESM files (no top-level `await`). TypeScript now allows this under `nodenext` without error.                                                                                                            |
| `import defer * as ns from "..."`               | 5.9    | Defer module _evaluation_ (not loading) until first property access. Module is loaded and verified at import time; side-effects are delayed. Only works with `--module preserve` or `esnext`.                                                 |
| `Set` algebra methods                           | 5.5    | Non-mutating: `union`, `intersection`, `difference`, `symmetricDifference` → new `Set`. Predicate: `isSubsetOf`, `isSupersetOf`, `isDisjointFrom` → `boolean`. Requires `esnext` or `es2025` lib.                                             |
| `Object.groupBy` / `Map.groupBy`                | 5.4    | Group an iterable into buckets by key function. Return type has all keys as optional (not every key is guaranteed present). Requires `esnext` or `es2024`+ lib.                                                                               |
| `Temporal` API types                            | 6.0 RC | `Temporal.Now`, `Temporal.Instant`, `Temporal.PlainDate`, etc. Available under `esnext` or `esnext.temporal` lib. Usable in runtimes that already ship it (V8 118+, SpiderMonkey, etc.).                                                      |
| `@satisfies` in JSDoc                           | 5.0    | Validates that a JS expression satisfies a type without widening it — the TS `satisfies` operator for `.js` files. Write `/** @satisfies {MyType} */` above the declaration or inline on a parenthesized expression.                          |
| `@overload` in JSDoc                            | 5.0    | Declare multiple call signatures for a JS function. Each JSDoc comment tagged `@overload` is treated as a distinct overload; the final JSDoc comment (without `@overload`) describes the implementation signature.                            |
| Getter/setter with completely unrelated types   | 5.1    | `get style(): CSSStyleDeclaration` and `set style(v: string)` can now have fully unrelated types, provided both have explicit type annotations. Previously the getter type was required to be a subtype of the setter type.                   |
| `instanceof` narrowing via `Symbol.hasInstance` | 5.3    | When a class defines `static [Symbol.hasInstance](val: unknown): val is T`, the `instanceof` operator now narrows to the predicate type `T`, not the class type itself. Useful when the runtime check and the structural type differ.         |
| Regex literal syntax checking                   | 5.5    | TypeScript validates regex literal syntax: malformed groups, nonexistent backreferences, named capture mismatches, and features not available at the current `--target`. No API needed; existing latent bugs surface as errors automatically. |
| `--build` continues past intermediate errors    | 5.6    | `tsc --build` no longer stops at the first failing project. All projects are built and errors reported together. Use `--stopOnBuildErrors` to restore the old stop-on-first-error behavior. Useful for monorepos during upgrades.             |
| `--module node18`                               | 5.8    | Stable `--module` flag for Node.js 18 semantics: disallows `require()` of ESM (unlike `nodenext`) and still allows import assertions. Use when pinned to Node 18 and not ready for `nodenext` behavior changes.                               |
| `--module node20`                               | 5.9    | Stable `--module` flag for Node.js 20 semantics: permits `require()` of ESM, rejects import assertions. Implies `--target es2023` (unlike `nodenext`, which floats to `esnext`).                                                              |

## Key APIs

### `Disposable` / `AsyncDisposable` / stacks (5.2)

Global types provided by TypeScript's lib (requires `esnext.disposable` or `esnext` in `lib`):

- `Disposable` — `{ [Symbol.dispose](): void }`
- `AsyncDisposable` — `{ [Symbol.asyncDispose](): PromiseLike<void> }`
- `DisposableStack` — `defer(fn)`, `use(resource)`, `adopt(value, disposeFn)`, `move()`. Is itself `Disposable`.
- `AsyncDisposableStack` — async equivalent. Is itself `AsyncDisposable`.
- `SuppressedError` — thrown when both the scope body and a `[Symbol.dispose]` throw. `.error` holds the dispose-phase error; `.suppressed` holds the original error.

Polyfill the symbols in older runtimes:

```ts
Symbol.dispose ??= Symbol("Symbol.dispose");
Symbol.asyncDispose ??= Symbol("Symbol.asyncDispose");
```

### Decorator context types (5.0)

Each decorator kind receives a typed context object as its second parameter:

- `ClassDecoratorContext`
- `ClassMethodDecoratorContext`
- `ClassGetterDecoratorContext`
- `ClassSetterDecoratorContext`
- `ClassFieldDecoratorContext`
- `ClassAccessorDecoratorContext`

All context objects have `.name`, `.kind`, `.static`, `.private`, and `.metadata`. Method/getter/setter/accessor contexts also have `.addInitializer(fn)` for running code at construction time.

### `IteratorObject` (5.6)

`IteratorObject<T, TReturn, TNext>` is the new type for built-in iterable iterators. Key methods: `map`, `filter`, `take`, `drop`, `flatMap`, `forEach`, `reduce`, `some`, `every`, `find`, `toArray`. Not the same as the pre-existing structural `Iterator<T>` protocol.

- Generators produce `Generator<T>` which extends `IteratorObject`.
- `Map.prototype.entries()` returns `MapIterator<[K, V]>`, `Set.prototype.values()` returns `SetIterator<T>`, etc.
- `Iterator.from(iterable)` converts any `Iterable` to an `IteratorObject`.
- `AsyncIteratorObject` exists for async parity.
- `--strictBuiltinIteratorReturn` (new `--strict`-mode flag in 5.6) makes the return type of `BuiltinIteratorReturn` be `undefined` instead of `any`, catching unchecked `done` access.

### Array copying methods (5.2)

Declared on `Array`, `ReadonlyArray`, and all `TypedArray` types. Use these instead of the mutating variants when you need to preserve the original:

| Mutating                           | Non-mutating copy                     |
| ---------------------------------- | ------------------------------------- |
| `arr.sort(cmp)`                    | `arr.toSorted(cmp)`                   |
| `arr.reverse()`                    | `arr.toReversed()`                    |
| `arr.splice(start, del, ...items)` | `arr.toSpliced(start, del, ...items)` |
| `arr[i] = v`                       | `arr.with(i, v)`                      |

## Pitfalls

Things easy to get wrong even when you know the modern API exists. Check your output against these.

**tsconfig defaults changed hard in 6.0.** `types: []` means no `@types/*` packages load implicitly. If you see floods of "cannot find name 'process'" or "cannot find module 'fs'" after upgrading to 6.0, add `"types": ["node"]` (or whatever you need) to `compilerOptions`. `rootDir: "."` means a project with source in `src/` will emit to `dist/src/` instead of `dist/` — add `"rootDir": "./src"` explicitly. `strict: true` by default means projects with loose code see new errors.

**`using` requires a runtime polyfill on older runtimes.** `Symbol.dispose` and `Symbol.asyncDispose` don't exist before Node.js 18.x / Chrome 120. Add the two-line polyfill at your entry point. `DisposableStack` and `AsyncDisposableStack` need a more substantial polyfill (e.g. from `@microsoft/using-polyfill`).

**`using` disposes in LIFO order.** Resources declared later in a scope are disposed first. Declare in the order you want reversed cleanup (acquisition order). `DisposableStack.defer` also runs in LIFO order.

**Inferred type predicates have if-and-only-if semantics.** `x => !!x` does NOT infer `x is NonNullable<T>` because `0`, `""`, and `false` are falsy but not absent. TypeScript correctly refuses the predicate. Use `x => x !== undefined` or `x => x !== null` for precise null/undefined filters. If a predicate isn't being inferred, the false branch is probably ambiguous.

**`--verbatimModuleSyntax` breaks CJS `require` emit.** Under this flag ESM `import`/`export` is emitted verbatim. You cannot produce `require()` calls from standard `import` syntax. For CJS output you must use `import foo = require("foo")` and `export = { ... }` syntax explicitly.

**`NoInfer<T>` doesn't prevent `T` from being resolved, only from being contributed at that position.** Other parameters can still infer `T`. It means "don't use me as an inference candidate", not "block `T` from being resolved".

**`--isolatedDeclarations` requires explicit return types on all exports.** Exported arrow functions, function declarations, and class methods all need annotations if their return type isn't trivially inferrable from a literal or type assertion. Editor quick-fixes can add them automatically.

**Standard decorators are incompatible with `--experimentalDecorators`.** Different type signatures, metadata model, and emit. A decorator written for one will not work with the other. `--emitDecoratorMetadata` is not supported with standard decorators. Don't mix the two systems in one project.

**`import defer` does not downlevel.** TypeScript does not transform `import defer` to polyfill-compatible code. The module is still _loaded_ eagerly (must exist); only _evaluation_ is deferred. Only use it under `--module preserve` or `esnext` with a runtime or bundler that supports it.

**`--erasableSyntaxOnly` prohibits parameter properties.** `constructor(public x: number)` is not allowed. Expand to an explicit field declaration plus assignment in the constructor body.

**Closure narrowing is invalidated if the variable is assigned anywhere in a nested function.** TypeScript cannot know when a nested function will run, so any assignment to a `let`/param inside a nested function — even a no-op like `value = value` — invalidates narrowing for all closures in the outer scope. Only the outer "no further assignments after this point" pattern is safe.

**Constant indexed access narrowing requires both `obj` and `key` to be unmodified between the check and the use.** If either is a `let` that could be reassigned, TypeScript will not narrow `obj[key]`. Extract the value to a `const` in that case.

**`switch (true)` narrowing does not carry across fall-through cases.** In a `switch (true)`, each `case` condition narrows independently. A variable narrowed in `case typeof x === "string":` that falls through to the next case will have its narrowing widened by the next condition, not accumulated from the previous one.

**`const` type parameter modifier falls back when constraint is mutable.** `<const T extends string[]>(args: T)` falls back to `string[]` because `readonly ["a", "b"]` isn't assignable to `string[]`. Use `<const T extends readonly string[]>` for arrays.

**`assert` import syntax errors under `--module nodenext` since 5.8.** Any remaining `import x from "..." assert { ... }` must be updated to `import x from "..." with { ... }`.

**`Array.prototype.filter(x => x !== null)` now narrows to non-null (5.5).** This is almost always correct, but if you intentionally needed the nullable type downstream, add an explicit annotation: `const items: (T | null)[] = arr.filter(x => x !== null)`.

## Behavioral changes that affect code

- **All enums are union enums** (5.0): Every enum member gets its own literal type. Out-of-domain literal assignment to an enum type now errors. Cross-enum assignment between enums with identical names but differing values now errors.
- **Relational operators no longer allow implicit string/number coercions** (5.0): `ns > 4` where `ns: number | string` is a type error. Use `+ns > 4` to explicitly coerce.
- **`--module`/`--moduleResolution` must agree on node flavor** (5.2): Mixing `--module nodenext` with `--moduleResolution bundler` is an error. Use `--module nodenext` alone or `--module esnext --moduleResolution bundler`.
- **Deprecations from 5.0 become hard errors in 5.5**: `--importsNotUsedAsValues`, `--preserveValueImports`, `--target ES3`, `--out`, and several others are fully removed in 5.5. They can no longer be specified, even with `"ignoreDeprecations": "5.0"`. Migrate to `--verbatimModuleSyntax` for the import flags.
- **Type-only imports conflicting with local values** (5.4): Under `--isolatedModules`, `import { Foo } from "..."` where a local `let Foo` also exists now errors. Use `import type { Foo }` or `import { type Foo }`.
- **Reference directives no longer synthesized or preserved in declaration emit** (5.5): `/// <reference types="node" />` TypeScript used to add automatically is no longer emitted. User-written directives are dropped unless they carry `preserve="true"`. Update library `tsconfig.json` if you relied on this.
- **`.mts` files never emit CJS; `.cts` files never emit ESM** (5.6): Regardless of `--module` setting. Previously the extension was ignored in some modes.
- **JSON imports under `--module nodenext` require `with { type: "json" }`** (5.7): `import data from "./config.json"` without the attribute is now a type error.
- **`TypedArray`s are now generic** (5.7): `Uint8Array` is `Uint8Array<TArrayBuffer extends ArrayBufferLike = ArrayBufferLike>`. Code passing `Buffer` (from `@types/node`) to typed-array parameters may see new errors. Update `@types/node` to a version that matches.
- **`import assert { ... }` is an error under `--module nodenext`** (5.8): Node.js 22 dropped support for the old syntax. Use `with { ... }`.
- **`types` defaults to `[]` in 6.0**: All implicit `@types/*` loading stops. Add an explicit `"types": ["node"]` or the array will remain empty. Using `"types": ["*"]` restores the 5.x behavior.
- **`rootDir` defaults to `.` (the tsconfig directory) in 6.0**: Previously inferred from the common ancestor of all source files. Projects with `"include": ["./src"]` and no explicit `rootDir` will now emit into `dist/src/` instead of `dist/`. Add `"rootDir": "./src"` to fix.
- **`strict` defaults to `true` in 6.0**: Projects that were implicitly not strict will see new errors. Set `"strict": false` explicitly if you're not ready to fix them.
- **`--baseUrl` deprecated in 6.0** and no longer acts as a module resolution root. Add explicit prefixes to your `paths` entries instead.
- **`--moduleResolution node` (node10) deprecated in 6.0**: Removed in 7.0. Migrate to `nodenext` or `bundler`.
- **`amd`, `umd`, `systemjs`, `none` module targets deprecated in 6.0**: Removed in 7.0. Migrate to a bundler.
- **`--outFile` removed in 6.0**: Use a bundler (esbuild, Rollup, Webpack, etc.).
- **`module Foo { }` syntax removed in 6.0**: Rename all such declarations to `namespace Foo { }`.
- **`--esModuleInterop false` and `--allowSyntheticDefaultImports false` removed in 6.0**: Safe interop is now always on. Default imports from CJS modules (`import express from "express"`) are always valid.
- **Explicit `typeRoots` disables upward `node_modules/@types` fallback** (5.1): When `typeRoots` is specified and a lookup fails in those directories, TypeScript no longer walks parent directories for `@types`. If you relied on the fallback, add `"./node_modules/@types"` explicitly to your `typeRoots` array.
- **`super.` on instance field properties is a type error** (5.3): Calling `super.foo()` where `foo` is a class field (arrow function assigned in the constructor) rather than a prototype method now errors. Instance fields don't exist on the prototype; `super.field` is `undefined` at runtime.
- **`--build` always emits `.tsbuildinfo`** (5.6): Previously only written when `--incremental` or `--composite` was set. Now written unconditionally in any `--build` invocation. Update `.gitignore` or CI artifact management if needed.
- **`.mts`/`.cts` extensions and `package.json` `"type"` respected in all module modes** (5.6): Format-specific extensions and the `"type"` field inside `node_modules` are now honored regardless of `--module` setting (except `amd`, `umd`, `system`). A `.mts` file will never emit CJS output even under `--module commonjs`.
- **Granular return expression checking** (5.8): Each branch of a conditional expression (`cond ? a : b`) directly inside a `return` statement is now checked individually against the declared return type. Previously an `any`-typed branch could silently suppress type errors in the other branch.
