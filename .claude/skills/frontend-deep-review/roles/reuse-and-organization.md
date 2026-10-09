# Reuse and Organization (RO)

Lens: does this diff build, copy, or misplace something the codebase already has, and is the PR a single reviewable change?

Boundaries: type shape and casts belong to TN, dead code and needless wrappers to SD, comment quality to CD, query and storage handling to DF, effects, rendering, and DOM behavior to RP, stories and tests to TS, styling and accessibility to SA. RO judges only whether code should exist here at all, whether it exists somewhere already, and whether it belongs in this PR.

Reading order: start with the PR description and `git diff --stat` to see size and spread. Open added files first (`git diff --diff-filter=A --name-only site/src`): for each new component, hook, util, constant, or regex, grep `site/src/components/`, `site/src/hooks/`, `site/src/utils/`, and the sibling feature folder for the same name or behavior before reading the implementation. Then read every changed file that lives outside the feature folder the PR is about (`site/src/components/`, `site/src/utils/`, `site/src/testHelpers/`, `site/.storybook/`, `site/vite.config.mts`, `site/package.json`) in full and ask what in the PR needs it. For placement calls, list the destination folder and read the sibling files, not just the hunk.

## Checkpoints

### RO1: Reuse the existing component, hook, util, or dependency

Default severity: should-fix
FE parent: FE3
Evidence: 25 comments across 15 PRs

Look for:
- Hand-rolled snippets whose job an existing helper already does: `error instanceof Error ? error.message : ...` (`getErrorMessage`), `crypto.randomUUID()` (in-tree uuid util), `navigator.clipboard.*` (`useClipboard`), `localStorage.getItem` or `storage.get()` paired with `useState` (`useStorage`), `typeof x === "string" && ...` type guards and manual `JSON.parse`/`validateSync` (yup schemas, `yupCodec`).
- New local constants that already exist elsewhere (`nilUUID`, `"me"`, page limits); grep the literal value across `site/src` before accepting the definition.
- JSX that assembles a dialog, copy button, lightbox, alert, or list scrolling by hand when `ConfirmDialog`, `CopyButton`, `Lightbox`, `ErrorAlert`, or the primitive's native behavior already covers it.
- New entries in `site/package.json`: ask whether an existing dependency already serves the purpose.

Fix: delete the local version and import the existing one; if the existing helper is missing a capability, extend it (add `readFromClipboard` to `useClipboard`) rather than writing a second one. A helper used in one place is inlined, not exported.

### RO2: No near-copies of existing components or modules

Default severity: should-fix
FE parent: FE3
Evidence: 8 comments across 6 PRs

Look for:
- A new file or exported component whose name is an existing name plus a prefix or qualifier (`NewPasswordField` next to `PasswordField`, `SelectField`, `AppFormField`, `Field`): open the original and diff behavior, not just names. RO1 covers a hand-rolled snippet inside otherwise new code; RO2 covers a whole second implementation of something that already has a home.
- Story files that define their own `*Field`, `SelectField`, or input wrappers instead of importing the real components.
- A second parser, validator, or regex for a format the repo already handles (front matter, integer strings); grep for the regex literal and for the format name.
- Any `Field`-style form wrapper outside `FormField`: the reviewers reject new ones on sight.

Fix: delete the copy and import the original. If the original lacks an option, add the option to the original in the same PR and explain it, instead of forking it.

### RO3: Extract what the PR repeats

Default severity: should-fix
FE parent: FE3
Evidence: 12 comments across 4 PRs

Look for:
- The same `className` string, or the same added class prefix (`mobile-full-width-dropdown ...`), appearing in more than two hunks of the diff; grep the diff for the added literal and count.
- The same expression or selection logic pasted into two components (`organizations.find((o) => o.is_default) ?? organizations[0]`).
- Repeated JSX structure with a hardcoded style list (`<p className="m-0 mb-1 font-semibold ...">` title plus message) in several tooltips or popovers.
- An exported `className` constant, or a re-typed union (`"info" | "warning"`) that mirrors a type another component already declares.

Fix: create one shared component (or a variant prop on the existing primitive) and use it everywhere the PR repeated it; export and import the type instead of retyping it. Sharing a `className` constant is not the fix; either accept two maintained copies or build the component.

### RO4: Put code next to its consumer, in a module that matches its name

Default severity: should-fix
FE parent: none
Evidence: 12 comments across 10 PRs

Look for:
- New `types.ts`, `*Constants.ts`, or const-only modules: who imports each export, and is there more than one consumer? A single consumer means co-locate; no functions alongside the consts means the module has no reason to exist.
- New files under `site/src/utils/`: does the file name describe every export? Functions that return a `className`, rendering predicates, or agent lookups do not belong in `utils/` or in a module named for something else (`workspaceApps`, `portForward`, `budget`).
- Central registries (every storage key for the whole app in one file) instead of definitions next to the component that reads the state.
- New top-level or config files: do they match the format and location of their siblings (`.jsonc`), and could the contents go in an existing file (`theme/index.ts`, `Badges.tsx`)?

Fix: move the code next to the component that uses it, or into the existing module whose name already covers it; delete the new file when that empties it. When the move would bloat the PR, reviewers accept a stated follow-up.

### RO5: Keep unrelated changes out of the PR

Default severity: should-fix
FE parent: none
Evidence: 13 comments across 12 PRs

Look for:
- Hunks in files the PR title does not explain: `site/.storybook/preview.tsx`, `site/vite.config.mts`, `site/src/testHelpers/handlers.ts`, `site/src/theme/index.ts`, import rewrites (`import React from "react"` replacing `import { StrictMode }`), removed dependencies. Ask of each: what in this PR breaks without it?
- Whole-file deletions and blocks moved within a file with no mention in the description.
- Edits to shared components under `site/src/components/` from a feature PR: separate additive props (`ariaLabel`, `disabled`) from logic or visual changes, and require a stated justification for the latter.
- New pages or routes that render nothing yet, added ahead of the feature that needs them.

Fix: revert the hunk (reviewers post an empty suggestion to do it) or move it to its own PR; explain any deletion or move that stays. Shared component behavior changes get their own PR and a design check.

### RO6: Split PRs that are too large to review

Default severity: should-fix
FE parent: FE3
Evidence: 3 comments across 2 PRs

Look for:
- A new module in the hundreds of lines (`@@ -0,0 +1,678 @@`) landing with its first consumers and a bug fix in one diff; ask whether the design is the simplest that meets the stated need and whether an existing dependency covers part of it.
- Bug fixes or behavior changes buried inside a large refactor diff (see RO5); on a big diff the reviewers refuse to review them in place.
- Split stacks that leave exports unused in the current PR and suppress the lint (`site/.knip.jsonc` exclusions with "staged in PR n" comments): each slice should be self-contained or the suppression called out to the human reviewer.

Fix: split by concern (infrastructure, consumers, fixes) into a stack of PRs each reviewable on its own, with no lint exclusions carrying dead exports between slices.

## Not this role

- Hand-rolled runtime type guards (`typeof record.x === "string" && ...`) versus yup schemas: RO owns "use the existing library", but the narrowing and type-predicate correctness belongs to TN.
- `useState` plus a separate `storage.set()` call instead of the hook that does both: the reuse call is RO, the effect and storage mechanics are RP and DF.
- Manual `scrollIntoView` where the primitive scrolls natively: RP owns the effect, SA asks whether CSS replaces it.
- A story file defining a dozen helper components: RO flags the copies of real components, but story structure and what a story should contain is TS.
- Knip exclusions carrying unused exports between stacked PRs: dead-code ownership is SD.
- Whether a visual change to a shared `Select` or `OrganizationAutocomplete` looks right: SA; RO5 only asks for the justification of touching the shared component.
