# Styling and Accessibility (SA)

Lens: does every added class, style, theme value, and interactive element use the Tailwind and semantic-HTML tools the codebase already has, at the layer that owns them, and does the result render and read correctly for every user.

Boundaries: "use the existing component X" and file placement belong to RO; single-use constants that should be inlined belong to SD; the mechanics of effects, refs, and observers belong to RP (SA only asks whether CSS could replace them); user-facing wording belongs to TN; stories and tests belong to TS.

Reading order: open changes under site/src/index.css and site/src/theme/ first (they cascade everywhere), then changed primitives in site/src/components/, then pages and modules. For each className hunk read the whole class string, not just the changed tokens, and open the primitive it is applied to (DialogContent, PopoverContent, DropdownMenuContent) to see what it already provides. Open sibling components for the existing cva or variant convention, site/src/theme/externalImages.ts when icons.json changes, and the `@theme` blocks in site/src/index.css, which extend Tailwind's default scale, when a value looks off-scale.

## Checkpoints

### SA1: Keep Tailwind classes on-scale, non-redundant, and readable

Default severity: should-fix
FE parent: none

Look for:

- Arbitrary values (`gap-[0.15rem]`, `py-[0.35rem]`, `md:w-[18rem]`, `[overflow-wrap:anywhere]`, `[filter:drop-shadow(...)]`) where a scale step or named utility exists (`gap-0.5`, `py-1.5`, `md:w-72`, `wrap-anywhere`, `drop-shadow-md`).
- Classes added "everywhere" with no effect on that element (`min-w-0` on an element that is neither a flex item nor a grid item, a second `w-3.5 h-3.5` pair that `size-3.5` covers) and `!` modifiers that only existed to beat MUI styles.
- Variant order that changes meaning (`after:hover:` vs `hover:after:`), and paired `data-[state=open]`/`data-[state=closed]` classes out of order.
- Codemod or upgrade diffs that replace a composable class with a longer arbitrary one.

Fix: use the nearest scale value or named utility, delete classes after confirming they have no effect on that element (`scroll-mt-44` on the `MultiSelectCombobox.tsx` list changes where its `scrollIntoView` call lands), drop leftover `!`, and restore the shorter composable form. A class whose purpose you cannot state is a question, not a FAIL: in report mode ask the author, and in self-review find out what it does before deleting it. An off-scale value derived from a fixed size the layout must match (a card height, an image) is not a finding; the PR description should say what it is derived from.

### SA2: Put shared styling in a component or variant, not a className string

Default severity: should-fix
FE parent: none

Look for:

- `const xClassName = "..."` or exported className constants, and the same class string pasted onto several `PopoverContent`/`DropdownMenuContent` call sites in one PR.
- Ternary or `&&` chains selecting classes per variant (`variant === "complete" && "border-..."`) or a `variantClasses` lookup object where the codebase uses `cva`; a second `cva` whose classes land on the same element as the first.
- Consumers of a primitive passing layout it should own (`max-h-[90vh]`, `grid-rows-[...]` on `DialogContent`), and helpers in `utils/` that return class names.

Fix: inline the classes at the use site, or if two places must stay in sync, make a shared component or add a `cva` variant on the primitive so consumers do not remember the classes. Delete layout a consumer repeats from the primitive (`DialogContent` already gets `max-h-[90vh]` from `dialogVariants` in `Dialog.tsx`).

### SA3: Style with Tailwind, not inline style, emotion, MUI, theme objects, or global CSS

Default severity: should-fix
FE parent: none

Look for:

- New `style={{ ... }}` props whose values are static and have a utility equivalent, including `style` used only to read a CSS variable (`height: "var(--header-height)"`). Inline `style` stays for values computed at runtime (a percentage from data, a user-chosen color, a virtualizer's row position), because Tailwind only generates classes it finds in the source.
- `css={...}` or `@mui` imports in changed files, and `useTheme` or `theme.palette.*` reads whose value ends up in a class or style a Tailwind token covers. The in-repo `useTheme` from `#/theme/context` is correct for a library that takes a JS value, such as Mermaid's `theme` option (`MermaidDiagram.tsx`); `site/src/theme/palette.ts` names others (Monaco, charts, confetti, color pickers).
- New entries in `site/src/theme/roles.ts` or other theme color objects.
- Edits to `site/src/index.css`, new global class names consumed from components, and JS that writes static values to `document.documentElement.style`. A value measured at runtime stays in JS (`AgentChatInput.tsx` sets `--mobile-dropdown-bottom` from the composer's position).

Fix: express static styles as Tailwind classes on the element (arbitrary-value classes are acceptable for CSS variables) and leave runtime values in `style`; keep colors in Tailwind tokens; keep global CSS untouched unless the change is meant to cascade site-wide, and say so.

### SA4: Let CSS and the browser do layout; check spacing and text rendering

Default severity: should-fix
FE parent: none

Look for:

- `ResizeObserver`, `clientWidth`, `scrollIntoView`, or computed `calc()` strings that set a width, height, or scroll position a class (`w-full`, `last-of-type:`, native listbox scrolling) would handle. Measurement stays when code counts or compares sizes (how many blocks fit in `WorkspaceTiming/Chart/Blocks.tsx`, which tabs overflow in `useKebabMenu.ts`), and `scrollIntoView` stays when it reveals the item JS marks active (`SkillsTriggerMenu.tsx`).
- An index-based flag (`!isLast && <div className="h-4" />`) that a CSS selector expresses.
- Conditional padding or margin added to a shared wrapper (`pb-5`) and whether it shifts neighboring items.
- Prose that is wrapped in `<code>`, `font-mono`, or `text-xs` when it should read as a sentence.

Fix: replace the measurement with the equivalent class or selector, or name the primitive that should own the behavior; move spacing to the element that needs it; drop code styling from inline sentence text.

### SA5: Stay theme-aware: tokens and theme files, not hardcoded colors, fonts, or filters

Default severity: should-fix
FE parent: none

Look for:

- `invert`, color, or filter classes that suit only one theme, especially on `ExternalImage`, which already adapts icons to the theme through `site/src/theme/externalImages.ts`. Ask for a semantic theme token or that mechanism; site/AGENTS.md forbids the `dark:` prefix, so never ask for one.
- A new icon in `site/src/theme/icons.json` that does not look right in every theme in the default full-color mode (a black or single-color logo) and has no entry in `defaultParametersForBuiltinIcons` in `site/src/theme/externalImages.ts`. Icons that look right everywhere need no entry; most existing icons have none.
- New theme variants that redefine every variable instead of overriding only the values that differ, and any change that removes or renames the legacy `auto` values in `LEGACY_AUTO_SYNC` (`site/src/theme/index.ts`), which map stored preferences to the sync option.
- Hardcoded `fontFamily` or color strings passed to third-party renderers (Mermaid, terminals) when a Tailwind or CSS variable could be inherited.

Fix: use the semantic token or the theme-aware primitive instead of a fixed color or filter; register icons that need a non-default mode in externalImages.ts; base new themes on an existing one; read fonts and colors from CSS variables where the library allows.

### SA6: Use semantic elements and correct accessible names, keyboard paths, and focus

Default severity: should-fix
FE parent: FE6

Look for:

- `<div role="button" tabIndex={0} onKeyDown=...>` where `<button>` gives the role, name, and keyboard handling for free, and a hand-built `role="radiogroup"` where the `RadioGroup` primitive (`site/src/components/RadioGroup/`) or a `<fieldset>` with `<legend>` around native radio inputs does. A control inside another interactive element, such as the `role="button"` clear control inside the `Autocomplete.tsx` trigger, is invalid either way: a `<button>` may contain neither interactive content nor an element with `tabindex`. Ask for the two controls to become siblings instead of swapping one element for another.
- `aria-label` values that replace rather than contain the visible label; `aria-*` props on primitives (cmdk) that overwrite them.
- Interactive elements reachable only by pointer, and dialogs or route changes that drop focus.
- Visually hidden interactive elements still in the tab order.

Fix: use the native element (add the Tailwind reset classes if the browser default looks wrong), move a control nested inside another interactive element out beside it, keep the visible label inside the accessible name, and verify the keys the element or its ARIA pattern defines: Tab reaches it, Enter and Space press a button, Enter follows a link, arrow keys move within a radio group or menu, and Escape closes a dialog, popover, or menu.

## Not this role

- Design approval and PR scope when a core component outside the feature area is modified (Select separator restyle): RO.
- Effects that write `--mobile-dropdown-bottom` to `document.documentElement`, ResizeObserver and `scrollIntoView` hooks: RP owns the effect and ref mechanics; SA only asks whether CSS replaces them.
- Where a class-name helper file lives (`site/src/utils/` or the feature folder): RO owns the file placement.
- A second `cva` or an exported className constant that adds nothing: SD shares the inlining call.
- Reviewer requests for the reason behind an off-scale value or a global CSS edit to be recorded: CD.
