# Comments and Docs (CD)

Lens: every comment, JSDoc block, and doc line the diff adds or touches must tell a reader something the code beneath it cannot, be true, be short, and sit where its subject lives.

Boundaries: whether the code itself is right belongs to other roles: effect misuse to RP, type contracts and identifier naming to TN, dead code and simplification to SD, query invalidation and storage to DF, story and test structure to TS, file placement and PR scope to RO, styling to SA. CD judges only the prose.

Reading order: grep the diff for added comment lines (`^\+\s*//`, `^\+\s*\*`, `{/*`) and open the files with the most hits first; api/queries, hooks, storage, and *.stories.tsx files carry the worst offenders. For each hit read the whole enclosing declaration, not just the hunk, so you can tell whether the comment restates it. When a comment cites a server limit, a caller, a backend endpoint, or another file, open that target and check the claim. Read the PR description once: rationale that lives there does not need to live in the code.

## Checkpoints

### CD1: Delete comments that restate the code below them

Default severity: should-fix
FE parent: FE4
Evidence: 24 comments across 15 PRs

Look for:
- A JSDoc line on a prop, field, or method that paraphrases its name: `/** Read the stored value */ get`, `/** Remove the persisted value */ remove`, `/** Accessible label for the icon */ iconLabel`.
- A comment directly above a condition, ternary, or JSX branch that narrates what the condition or branch renders ("Single-item trigger: flat text, no button chrome").
- A doc block on a function or story whose sentences map one to one onto the statements or the export name beneath it (numbered "1. 2. 3." step lists, "Splits X into Y and Z" above a function named for exactly that).
- Section labels above a group of identifiers ("// Queue editing state.") that name what the identifiers already name.

Fix: delete the comment. If one clause carried a non-obvious constraint, keep only that clause.

### CD2: Delete rationale comments that do not earn their place

Default severity: should-fix
FE parent: FE4
Evidence: 46 comments across 25 PRs

Look for:
- A "why" comment above an ordinary call: a cache invalidation, a useState, a className, a navigate, an event listener, a story export. Ask whether a reader of the surrounding code would have wondered; if not, the comment is noise even when every word is true.
- Comments that record PR history or authoring context: "moved here because", "intentionally not ported", "matching the behavior of the Provider form", "Generated once per mount via lazy state init". That belongs in the PR description or commit message.
- Prop and field comments that explain one caller's situation or a backend condition ("This can be true even when providers/models exist in the DB catalog"), and banner or divider comments (`// -----`) between declarations.
- Pre-existing comments the diff touches: if the diff edits a comment that adds nothing, the reviewers ask for it to go rather than be extended.

Fix: delete the comment outright; do not shorten it. Move any genuinely needed context to the PR description.

### CD3: Cut over-long comments to the one constraint they carry

Default severity: should-fix
FE parent: FE4
Evidence: 28 comments across 18 PRs

Look for:
- Any added comment of four or more lines, or a JSDoc block longer than the declaration it documents. Count lines per hunk; reviewers react to length before content.
- Comments that explain second-order effects, harmlessness of an edge case, or how another module will react ("the resulting over-invalidation is harmless", "anchors the in-flight cycle so a refetch can't shift").
- The same fact stated twice in different words inside one comment, or two adjacent comments that overlap.
- Lines that wrap well before the column limit, and prose references to external files or repos that should be a link.
- A diff that grows an existing short comment with extra sentences (React Compiler notes, scope explanations); the original was usually right.

Fix: rewrite to 1 to 3 lines holding only the invariant, external constraint, or tradeoff; if nothing survives the cut, delete it. Restore the previous wording when the diff only lengthened an existing comment.

### CD4: Reject comments that are wrong, contradict the code, or will go stale

Default severity: should-fix
FE parent: FE4
Evidence: 12 comments across 11 PRs

Look for:
- A comment that states a fact the adjacent code disproves: "doesn't query the story canvas" above `within(canvasElement).getByRole`, a doc block above a predicate whose first branch does the opposite, "passing null removes the key" on a setter typed `(value: T)`.
- Sentences that make no sense when read literally ("Reused as the confirm handler when omitted"), or that label a live option as "legacy".
- Comments that pin a number, an experiment name, a Tailwind class arithmetic, or a backend rule ("The server enforces a maximum of 2000", "when the experiment goes stable", "h-7 matches one input row") that another PR can change without touching this file.
- TODO comments the diff touches or sits beside, especially ones addressed to a person: ask whether the TODO is still relevant and delete it if nobody can say.

Fix: correct the sentence so it matches the code, or delete it; for stale-prone facts, delete the comment and let the code or the referenced constant speak. Resolve or remove the TODO.

### CD5: Put documentation on the thing it describes

Default severity: should-fix
FE parent: FE4
Evidence: 10 comments across 5 PRs

Look for:
- JSDoc tags with no consumer in this repo (`@public`, "Consumed by ... in a higher stack layer"); the reviewers drop them on sight.
- A type or interface doc block that describes what each method does instead of the method's own JSDoc doing it; a call-site comment in a test or page that explains a helper's behavior instead of sitting on the helper's definition.
- Frontend doc comments that describe callers ("used on the org members editor") or coderd internals (which endpoint populates which field); both belong with the code they describe, or nowhere.
- Module-level or config-file prose placed away from the section it governs (a cleanup note at the top of a file whose cleanup section sits at the bottom, a one-line policy at line 1 of a 300-line module).

Fix: move the sentence to the declaration it describes and delete the original; drop stack-layer and visibility annotations entirely.

### CD6: Explain the non-obvious decision, or make the existing explanation readable

Default severity: should-fix
FE parent: FE4
Evidence: 4 comments across 4 PRs

Look for:
- A useEffect, manual useMemo, migration call, or other pattern the FE rules discourage that ships with no comment, or with a one-liner a reader cannot connect to the code ("Generated X is alphabetical, not display order").
- A justification that names a mechanism without the reason ("external to the compiler's static analysis"); if a reviewer would have to ask "why does that matter here?", it is not a justification yet.
- A component or helper whose props (`text`, `query`) cannot be understood from the signature and whose comment does not say how they are used.

Fix: write one or two sentences stating the constraint and the consequence of removing the code; if you cannot, the code probably should not exist (hand that to RP or SD).

## Not this role

- The useEffect that runs a legacy-selection migration whenever query data arrives is an effect-misuse question first and a documentation question second: RP.
- `[...messages].sort(...)` where `toSorted` would do: SD.
- A setter typed `(value: T)` whose doc says null removes the key, and props named `text` and `query` that a consumer cannot read: the type and naming fixes are TN; CD only owns the mismatched prose.
- A predicate that returns true for the empty string: the comment lied, but the code behavior is SD's call.
- Requests to add `auto-protan-deuter` and `auto-tritan` theme options are feature scope, not comment quality: RO.
