# Styling and Accessibility (SA)

Lens: does every added class, style, theme value, and interactive element use the Tailwind and semantic-HTML tools the codebase already has, at the layer that owns them, and does the result render and read correctly for every user.

Boundaries: "use the existing component X" and file placement belong to RO; single-use constants that should be inlined belong to SD; the mechanics of effects, refs, and observers belong to RP (SA only asks whether CSS could replace them); user-facing wording belongs to TN; stories and tests belong to TS.

Reading order: open changes under site/src/index.css and site/src/theme/ first (they cascade everywhere), then changed primitives in site/src/components/, then pages and modules. For each className hunk read the whole class string, not just the changed tokens, and open the primitive it is applied to (DialogContent, PopoverContent, DropdownMenuContent) to see what it already provides. Open sibling components for the existing cva or variant convention, site/src/theme/externalImages.ts when icons.json changes, and the Tailwind config when a value looks off-scale.

## Checkpoints

### SA1: Keep Tailwind classes on-scale, non-redundant, and readable

Default severity: should-fix
FE parent: none
Evidence: 16 comments across 5 PRs

Look for:
- Arbitrary values (`gap-[0.15rem]`, `py-[0.35rem]`, `md:w-[18rem]`, `[overflow-wrap:anywhere]`, `[filter:drop-shadow(...)]`) where a scale step or named utility exists (`gap-0.5`, `py-1.5`, `md:w-72`, `wrap-anywhere`, `drop-shadow-md`).
- Classes added "everywhere" with no effect on that element (`min-w-0` on a non-flex-child, a second `w-3.5 h-3.5` pair that `size-3.5` covers) and `!` modifiers that only existed to beat MUI styles.
- Variant order that changes meaning (`after:hover:` vs `hover:after:`), paired `data-[state=open]`/`data-[state=closed]` classes out of order, and classes whose purpose the reviewer cannot state (`scroll-mt-44`).
- Codemod or upgrade diffs that replace a composable class with a longer arbitrary one.

Fix: use the nearest scale value or named utility, delete classes that do nothing on that element, drop leftover `!`, and restore the shorter composable form; if an off-scale value is intentional, say why in the PR.

### SA2: Put shared styling in a component or variant, not a className string

Default severity: should-fix
FE parent: none
Evidence: 16 comments across 10 PRs

Look for:
- `const xClassName = "..."` or exported className constants, and the same class string pasted onto several `PopoverContent`/`DropdownMenuContent` call sites in one PR.
- Ternary or `&&` chains selecting classes per variant (`variant === "complete" && "border-..."`) or a `variantClasses` lookup object where the codebase uses `cva`; a second `cva` whose classes land on the same element as the first.
- Consumers of a primitive passing layout it should own (`max-h-[90vh]`, `grid-rows-[...]` on `DialogContent`), and helpers in `utils/` that return class names.

Fix: inline the classes at the use site, or if two places must stay in sync, make a shared component or add a `cva` variant on the primitive so consumers do not remember the classes.

### SA3: Style with Tailwind, not inline style, emotion, MUI, theme objects, or global CSS

Default severity: should-fix
FE parent: none
Evidence: 12 comments across 5 PRs

Look for:
- New `style={{ ... }}` props, `css={...}`, `theme.palette.*`, `useTheme`, or `@mui` imports in changed files, including `style` used only to read a CSS variable (`height: "var(--header-height)"`).
- New entries in `site/src/theme/roles.ts` or other theme color objects.
- Edits to `site/src/index.css`, new global class names consumed from components, and JS that writes `document.documentElement.style` properties.

Fix: express it as Tailwind classes on the element (arbitrary-value classes are acceptable for CSS variables); keep colors in Tailwind tokens; keep global CSS untouched unless the change is meant to cascade site-wide, and say so.

### SA4: Let CSS and the browser do layout; check spacing and text rendering

Default severity: should-fix
FE parent: none
Evidence: 9 comments across 6 PRs

Look for:
- `ResizeObserver`, `clientWidth`, `scrollIntoView`, or computed `calc()` strings that set a width, height, or scroll position a class (`w-full`, `last-of-type:`, native listbox scrolling) would handle.
- An index-based flag (`!isLast && <div className="h-4" />`) that a CSS selector expresses.
- Conditional padding or margin added to a shared wrapper (`pb-5`) and whether it shifts neighboring items.
- Prose that is wrapped in `<code>`, `font-mono`, or `text-xs` when it should read as a sentence.

Fix: replace the measurement with the equivalent class or selector, or name the primitive that should own the behavior; move spacing to the element that needs it; drop code styling from inline sentence text.

### SA5: Stay theme-aware: tokens and theme files, not hardcoded colors, fonts, or filters

Default severity: should-fix
FE parent: none
Evidence: 5 comments across 4 PRs

Look for:
- `invert`, color, or filter classes with no `dark:` qualifier, especially on `ExternalImage`.
- New icons in `site/src/theme/icons.json` without a matching entry in `site/src/theme/externalImages.ts`.
- New theme variants that redefine every variable instead of overriding only the values that differ, and any change that removes or renames the `auto` theme option.
- Hardcoded `fontFamily` or color strings passed to third-party renderers (Mermaid, terminals) when a Tailwind or CSS variable could be inherited.

Fix: qualify by theme or use the token; register icons in externalImages.ts; base new themes on an existing one; read fonts and colors from CSS variables where the library allows.

### SA6: Use semantic elements and correct accessible names, keyboard paths, and focus

Default severity: should-fix
FE parent: FE6
Evidence: 5 comments across 3 PRs

Look for:
- `<div role="button" tabIndex={0} onKeyDown=...>` or `<div role="radiogroup" aria-label=...>` where `<button>`, `<fieldset>` with `<legend>`, or an existing primitive gives the role, name, and keyboard handling for free.
- `aria-label` values that replace rather than contain the visible label; `aria-*` props on primitives (cmdk) that overwrite them.
- Interactive elements reachable only by pointer, and dialogs or route changes that drop focus.
- Visually hidden interactive elements still in the tab order.

Fix: use the native element (add the Tailwind reset classes if the browser default looks wrong), keep the visible label inside the accessible name, and verify Tab, Enter, Space, and Escape reach and operate the control.

## Not this role

- Design approval and PR scope when a core component outside the feature area is modified (Select separator restyle): RO.
- Effects that write `--mobile-dropdown-bottom` to `document.documentElement`, ResizeObserver and `scrollIntoView` hooks: RP owns the effect and ref mechanics; SA only asks whether CSS replaces them.
- Class-name helper functions living in `site/src/utils/budget.ts`: RO owns the file placement.
- A second `cva` or an exported className constant that adds nothing: SD shares the inlining call.
- Reviewer requests for the reason behind an off-scale value or a global CSS edit to be recorded: CD.
