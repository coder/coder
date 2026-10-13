# Reuse and Organization (RO)

Lens: does this diff build, copy, or misplace something the codebase already has, and is the PR a single reviewable change?

Boundaries: type shape and casts belong to TN, dead code and needless wrappers to SD, comment quality to CD, query and storage handling to DF, effects, rendering, and DOM behavior to RP, stories and tests to TS, styling and accessibility to SA. RO judges only whether code should exist here at all, whether it exists somewhere already, and whether it belongs in this PR.

Reading order: start with the PR description and the whole PR's diff stat and file list, without the `-- site/` filter, so RO5 and RO6 also see changes outside site/ (`git diff --stat <commits>` and `git diff --name-only <commits>`, where `<commits>` is the one SHA or the two SHAs the target passes). In self-review, also run `git ls-files --others --exclude-standard` for new files anywhere in the repository, which `git diff` omits. Open added files first: the ones `git diff --diff-filter=A --name-only <commits> -- site/src` prints, and every file the list marks `(new, untracked)`. A bare `git diff` shows only uncommitted edits, so always pass the target's commits. For each new component, hook, util, constant, or regex, grep `site/src/components/`, `site/src/hooks/`, `site/src/utils/`, and the sibling feature folder for the same name or behavior before reading the implementation. Then read every changed file that lives outside the feature folder the PR is about (`site/src/components/`, `site/src/utils/`, `site/src/testHelpers/`, `site/.storybook/`, `site/vite.config.mts`, `site/package.json`) in full and ask what in the PR needs it. For placement calls, list the destination folder and read the sibling files, not just the hunk.

## Checkpoints

### RO1: Reuse the existing component, hook, util, or dependency

Default severity: should-fix
FE parent: FE3

Look for:

- Hand-rolled snippets whose job an existing helper already does: `error instanceof Error ? error.message : ...` (`getErrorMessage`), `crypto.randomUUID()` (`generateUUID` in `site/src/utils/random.ts`), `navigator.clipboard.writeText` (`useClipboard`), and `typeof x === "string" && ...` type guards where a yup schema already describes the shape.
- A direct `navigator.clipboard.readText()` call: suggest `readFromClipboard` from `useClipboard` only when its fallback to the last in-app copy on insecure origins is wanted. A read with its own permission handling stays direct (`useDesktopConnection.ts`).
- New local constants that already exist elsewhere (`NIL_UUID`, `"me"`, page limits); grep the literal value across `site/src` before accepting the definition.
- JSX that assembles a dialog, copy button, lightbox, alert, or list scrolling by hand when `ConfirmDialog`, `CopyButton`, `Lightbox` (in `pages/AgentsPage/components/`), `ErrorAlert`, or the primitive's native behavior already covers it.
- New entries in `site/package.json`: ask whether an existing dependency already serves the purpose.

Fix: delete the local version and import the existing one; if the existing helper is missing a capability, extend it (the way `readFromClipboard` was added to `useClipboard`) rather than writing a second one. A helper used in one place is inlined, not exported.

### RO2: No near-copies of existing components or modules

Default severity: should-fix
FE parent: FE3

Look for:

- A new file or exported component whose name is an existing name plus a prefix or qualifier (`NewPasswordField` next to `PasswordField`, a second `SelectField` or `FormField`): open the original and diff behavior, not just names. RO1 covers a hand-rolled snippet inside otherwise new code; RO2 covers a whole second implementation of something that already has a home.
- Story files that define their own `*Field`, `SelectField`, or input wrappers instead of importing the real components.
- A second parser, validator, or regex for a format the repo already handles (front matter, integer strings); grep for the regex literal and for the format name.
- A new `Field`-style wrapper that re-implements `FormField`'s label, helper text, or error layout: the reviewers reject those on sight. A wrapper that renders `FormField` and adds behavior is fine (`PasswordField.tsx` adds a debounced password check).

Fix: delete the copy and import the original. If the original lacks an option, add it to the original instead of forking it: in this PR when the option leaves current callers unchanged, and in its own PR when it changes what current callers get (RO5).

### RO3: Extract what the PR repeats

Default severity: should-fix
FE parent: FE3

Look for:

- The same `className` string, or the same added class prefix (`mobile-full-width-dropdown ...`), appearing in more than two hunks of the diff; grep the diff for the added literal and count.
- The same expression or selection logic pasted into two components (`organizations.find((o) => o.is_default) ?? organizations[0]`).
- Repeated JSX structure with a hardcoded style list (`<p className="m-0 mb-1 font-semibold ...">` title plus message) in several tooltips or popovers.
- An exported `className` constant, or a re-typed union (`"info" | "warning"`) that mirrors a type another component already declares.

Fix: for repeated JSX, create one shared component (or a variant prop on the existing primitive); for repeated logic such as a selection expression, one shared helper function. Use it everywhere the PR repeated it, and export and import the type instead of retyping it. Sharing a `className` constant is not the fix: build the component, or leave the class string alone when it appears in only two places.

### RO4: Put code next to its consumer, in a module that matches its name

Default severity: should-fix
FE parent: none

Look for:

- New `types.ts`, `*Constants.ts`, or const-only modules: who imports each export, and is there more than one consumer? A single consumer means co-locate, except `Mock*` entity fixtures, which FE9 keeps in `site/src/testHelpers/` even with one consumer. A const-only module that several files import is a fine shared home (`ChatElements/tools/previewConstants.ts`).
- New files under `site/src/utils/`: does the file name describe every export? Functions that return a `className`, rendering predicates, or agent lookups do not belong in `utils/` or in a module named for something else (`workspaceApps`, `portForward`, `budget`).
- Central registries (every storage key for the whole app in one file) instead of definitions next to the component that reads the state.
- New top-level or config files: do they match the format and location of their siblings (`.jsonc`), and could the contents go in an existing file (`theme/index.ts`, `components/Badge/Badge.tsx`)?

Fix: move the code next to the component that uses it, or into the existing module whose name already covers it; delete the new file when that empties it.

### RO5: Keep unrelated changes out of the PR

Default severity: should-fix
FE parent: FE3

Look for:

- Hunks in files the PR title does not explain: `site/.storybook/preview.tsx`, `site/vite.config.mts`, `site/src/testHelpers/handlers.ts`, `site/src/theme/index.ts`, import rewrites (`import React from "react"` replacing `import { StrictMode }`), removed dependencies. Ask of each: what in this PR breaks without it?
- Whole-file deletions and blocks moved within a file with no mention in the description.
- Edits to shared components under `site/src/components/` from a feature PR. A backward-compatible extension the feature needs, such as an additive prop (`ariaLabel`, `disabled`) or a new variant that leaves current callers unchanged, ships with the feature. A change that alters what current callers render or do, or that the feature does not need, belongs in its own PR.
- New pages or routes that render nothing yet, added ahead of the feature that needs them.

Fix: revert the hunk (reviewers post an empty suggestion to do it) or move it to its own PR; a shared component change that alters current callers gets its own PR and a design check. When the PR does need a deletion or move, add the missing line to the PR description saying why.

### RO6: Split PRs that are too large to review

Default severity: should-fix
FE parent: FE3

Look for:

- A new module in the hundreds of lines (`@@ -0,0 +1,678 @@`) landing with its first consumers and a bug fix in one diff; ask whether the design is the simplest that meets the stated need and whether an existing dependency covers part of it.
- Bug fixes or behavior changes buried inside a large refactor diff (see RO5); on a big diff the reviewers refuse to review them in place.
- Split stacks that leave exports unused in the current PR and suppress the lint (`site/.knip.jsonc` exclusions with "staged in PR n" comments): each slice should be self-contained, so move the export into the slice that first uses it instead of suppressing the lint.

Fix: split by concern (infrastructure, consumers, fixes) into a stack of PRs each reviewable on its own, with no lint exclusions carrying dead exports between slices.

## Not this role

- Hand-rolled runtime type guards (`typeof record.x === "string" && ...`) versus yup schemas: RO owns "use the existing library", but the narrowing and type-predicate correctness belongs to TN.
- Manual `scrollIntoView` where the primitive scrolls natively: RP owns the effect, SA asks whether CSS replaces it.
- A story file defining a dozen helper components: RO flags the copies of real components, but story structure and what a story should contain is TS.
- Knip exclusions carrying unused exports between stacked PRs: dead-code ownership is SD.
- Whether a visual change to a shared `Select` or `OrganizationAutocomplete` looks right: SA; RO5 only asks that a shared component change that alters current callers move to its own PR.
