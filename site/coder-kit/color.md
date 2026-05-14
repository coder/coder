# Colors

All color tokens are stored as HSL triplets (shadcn convention) so they compose with alpha:

```css
color: hsl(var(--content-primary));
background: hsl(var(--surface-sky) / 0.5);   /* with alpha */
```

Dark is the default theme. Switch with `data-theme="light"` or class `light` on any ancestor.

Source of truth: `site/src/index.css`

---

## Primitive Scales

Raw color ramps imported from `site/src/theme/tailwindColors.ts`.
Semantic tokens reference these indirectly. Avoid using primitives directly
in UI code.

### Zinc (neutral)

The primary neutral scale. Includes a non-standard step 650.

| Step | Hex       |
|------|-----------|
| 50   | `#fafafa` |
| 100  | `#f4f4f5` |
| 200  | `#e4e4e7` |
| 300  | `#d4d4d8` |
| 400  | `#a1a1aa` |
| 500  | `#71717a` |
| 600  | `#52525b` |
| 650  | `#4b4b52` |
| 700  | `#3f3f46` |
| 800  | `#27272a` |
| 900  | `#18181b` |
| 950  | `#09090b` |

### Blue (primary accent)

Matches the standard Tailwind blue scale.

| Step | Hex       |
|------|-----------|
| 50   | `#eff6ff` |
| 100  | `#dbeafe` |
| 200  | `#bfdbfe` |
| 300  | `#93c5fd` |
| 400  | `#60a5fa` |
| 500  | `#3b82f6` |
| 600  | `#2563eb` |
| 700  | `#1d4ed8` |
| 800  | `#1e40af` |
| 900  | `#1e3a8a` |
| 950  | `#172554` |

All standard Tailwind color scales (slate, gray, neutral, stone, red,
orange, amber, yellow, lime, green, emerald, teal, cyan, sky, indigo,
violet, purple, fuchsia, pink, rose) are also available via
`tailwindColors.ts`. The semantic tokens below for magenta and purple were
defined directly as HSL values; no named primitive ramp exists for them.

---

## Content

Text and icon foreground colors.

| Token                   | Dark HSL         | Dark Hex  | Light HSL        | Light Hex | Usage                     |
|-------------------------|------------------|-----------|------------------|-----------|---------------------------|
| `--content-primary`     | `0 0% 100%`     | `#ffffff` | `240 10% 4%`    | `#09090b` | Default text              |
| `--content-secondary`   | `240 5% 65%`    | `#a1a1aa` | `240 5% 34%`    | `#52525b` | Meta, labels, captions    |
| `--content-link`        | `213 94% 68%`   | `#61a6fa` | `221 83% 53%`   | `#2463eb` | Links, focus rings        |
| `--content-invert`      | `240 10% 4%`    | `#09090b` | `0 0% 98%`      | `#fafafa` | Text on inverted surfaces |
| `--content-disabled`    | `240 5% 26%`    | `#3f3f46` | `240 5% 65%`    | `#a1a1aa` | Disabled text             |
| `--content-success`     | `142 76% 36%`   | `#16a249` | `142 72% 29%`   | `#157f3c` | Running, success states   |
| `--content-warning`     | `31 97% 72%`    | `#fdba72` | `27 96% 61%`    | `#fb923c` | Warnings                  |
| `--content-destructive` | `0 91% 71%`     | `#f87272` | `0 84% 60%`     | `#ef4343` | Errors, failed states     |

---

## Surface

Background fills for containers, cards, and tinted regions.

| Token                        | Dark HSL          | Dark Hex  | Light HSL         | Light Hex | Usage                     |
|------------------------------|-------------------|-----------|-------------------|-----------|---------------------------|
| `--surface-primary`          | `240 10% 4%`     | `#09090b` | `0 0% 100%`      | `#ffffff` | Page background           |
| `--surface-secondary`        | `240 6% 10%`     | `#18181b` | `240 5% 96%`     | `#f4f4f5` | Cards, sidebar            |
| `--surface-tertiary`         | `240 4% 16%`     | `#27272a` | `240 6% 90%`     | `#e4e4e7` | Hover, active tab         |
| `--surface-quaternary`       | `240 5% 26%`     | `#3f3f46` | `240 5% 84%`     | `#d4d4d8` | Selected, pressed         |
| `--surface-invert-primary`   | `240 6% 90%`     | `#e4e4e7` | `240 4% 16%`     | `#27272a` | Inverted background       |
| `--surface-invert-secondary` | `240 5% 65%`     | `#a1a1aa` | `240 5% 26%`     | `#3f3f46` | Inverted secondary        |
| `--surface-destructive`      | `0 75% 15%`      | `#430a0a` | `0 93% 94%`      | `#fee1e1` | Error background          |
| `--surface-green`            | `145 80% 10%`    | `#052e16` | `141 79% 85%`    | `#bbf7d0` | Success background        |
| `--surface-grey`             | `240 6% 10%`     | `#18181b` | `240 5% 96%`     | `#f4f4f5` | Neutral tint              |
| `--surface-orange`           | `13 81% 15%`     | `#451507` | `34 100% 92%`    | `#ffedd6` | Warning background        |
| `--surface-sky`              | `196 67% 12%`    | `#0a2833` | `191 67% 89%`    | `#d0eff6` | Pending / info background |
| `--surface-red`              | `0 75% 15%`      | `#430a0a` | `0 93% 94%`      | `#fee1e1` | Red tint (alias)          |
| `--surface-purple`           | `268 68% 17%`    | `#290e49` | `259 100% 95%`   | `#eee5ff` | Purple tint               |
| `--surface-magenta`          | `291 74% 15%`    | `#3a0a43` | `289 100% 98%`   | `#fdf5ff` | Magenta tint              |
| `--surface-git-added`        | `145 80% 10%`    | `#052e16` | `141 84% 93%`    | `#defce9` | Diff addition background  |
| `--surface-git-deleted`      | `0 75% 15%`      | `#430a0a` | `0 93% 94%`      | `#fee1e1` | Diff deletion background  |
| `--surface-git-merged`       | `274 87% 21%`    | `#3c0764` | `269 100% 95%`   | `#f2e6ff` | Diff merge background     |

---

## Border

Stroke colors for dividers, inputs, and status badges.

| Token                  | Dark HSL               | Dark Hex  | Light HSL              | Light Hex | Usage                   |
|------------------------|------------------------|-----------|------------------------|-----------|-------------------------|
| `--border-default`     | `240 4% 16%`          | `#27272a` | `240 6% 90%`          | `#e4e4e7` | Default border          |
| `--border-secondary`   | `240 5% 26%`          | `#3f3f46` | `240 5% 65%`          | `#a1a1aa` | Stronger / hover border |
| `--border-success`     | `142 76% 36%`         | `#16a249` | `142 76% 36%`         | `#16a249` | Success badge outline   |
| `--border-destructive` | `0 91% 71%`           | `#f87272` | `0 84% 60%`           | `#ef4343` | Error badge outline     |
| `--border-warning`     | `30.66 97.16% 72.35%` | `#fdba74` | `30.66 97.16% 72.35%` | `#fdba74` | Warning badge outline   |
| `--border-sky`         | `194 90% 62%`         | `#47cdf5` | `203 90% 40%`         | `#0a7bc2` | Pending / starting      |
| `--border-green`       | `143 77% 87%`         | `#c4f7d8` | `138 82% 82%`         | `#abf7c2` | Green badge outline     |
| `--border-purple`      | `255 92% 76%`         | `#a689fa` | `255 92% 76%`         | `#a689fa` | Purple badge outline    |
| `--border-magenta`     | `292 100% 78%`        | `#f08fff` | `295 68% 46%`         | `#b826c5` | Magenta badge outline   |

> **Note:** `--border-warning` currently uses the same value in both light
> and dark themes. This is inconsistent with other border tokens that
> differentiate by theme and may be a bug.

---

## Highlight

High-contrast foreground colors used inside tinted badges and chips.
Pair with the matching `--surface-*` token for accessible contrast.

| Token                  | Dark HSL          | Dark Hex  | Light HSL         | Light Hex | Usage               |
|------------------------|-------------------|-----------|-------------------|-----------|---------------------|
| `--highlight-purple`   | `269 100% 74%`   | `#ba7aff` | `271 61% 35%`    | `#5b2390` | Purple badge text   |
| `--highlight-green`    | `141 79% 85%`    | `#bbf7d0` | `143 64% 24%`    | `#166434` | Green badge text    |
| `--highlight-orange`   | `31 100% 70%`    | `#ffb566` | `30 100% 32%`    | `#a35200` | Orange badge text   |
| `--highlight-sky`      | `188 75% 80%`    | `#a6e8f2` | `195 61% 22%`    | `#16495a` | Sky / info badge text |
| `--highlight-red`      | `0 91% 71%`      | `#f87272` | `0 74% 42%`      | `#ba1c1c` | Red badge text      |
| `--highlight-magenta`  | `292 100% 78%`   | `#f08fff` | `295 68% 40%`    | `#a021ab` | Magenta badge text  |

> **Known issue:** The Tailwind config references `highlight-grey` via
> `hsl(var(--highlight-grey))`, but `--highlight-grey` is never defined
> in `index.css`. Using `highlight-grey` utility classes produces invalid
> CSS.

### Badge pairing recipe

Badges combine a `--highlight-*` foreground with a `--surface-*` background and a `--border-*` outline:

| Badge   | Foreground           | Background         | Border                 | Usage                    |
|---------|----------------------|--------------------|------------------------|--------------------------|
| Sky     | `--highlight-sky`    | `--surface-sky`    | `--border-sky`         | Beta, pending, starting  |
| Purple  | `--highlight-purple` | `--surface-purple` | `--border-purple`      | Enterprise, graphs       |
| Red     | `--highlight-red`    | `--surface-red`    | `--border-destructive` | Errors, failed           |
| Orange  | `--highlight-orange` | `--surface-orange` | `--border-warning`     | Warnings, build timeline |
| Green   | `--highlight-green`  | `--surface-green`  | `--border-green`       | Success, running         |
| Magenta | `--highlight-magenta`| `--surface-magenta`| `--border-magenta`     | Build timeline           |

---

## Overlay

| Token               | Dark                | Light                | Usage          |
|----------------------|---------------------|----------------------|----------------|
| `--overlay-default`  | `240 10% 4% / 80%` | `240 5% 84% / 80%`  | Modal backdrop |

Use with `hsla()`: `background: hsla(var(--overlay-default));`

---

## Syntax Highlighting

Tokens for code blocks and inline code. Values differ between light and
dark themes.

| Token              | Dark HSL         | Dark Hex  | Light HSL        | Light Hex | Usage           |
|--------------------|------------------|-----------|------------------|-----------|-----------------|
| `--syntax-key`     | `201 98% 80%`   | `#9adbfe` | `211 95% 33%`   | `#0451a4` | Keywords        |
| `--syntax-string`  | `18 47% 64%`    | `#ce9278` | `0 77% 36%`     | `#a21515` | String literals |
| `--syntax-number`  | `100 28% 73%`   | `#b4cda7` | `158 88% 28%`   | `#098658` | Numbers         |
| `--syntax-boolean` | `207 61% 59%`   | `#579dd6` | `240 100% 50%`  | `#0000ff` | Booleans        |

---

## Git Diff

Foreground colors for diff indicators. Values differ between light and
dark themes.

| Token                  | Dark HSL         | Dark Hex  | Light HSL        | Light Hex | Usage           |
|------------------------|------------------|-----------|------------------|-----------|-----------------|
| `--git-added`          | `142 77% 73%`   | `#85efac` | `142 72% 29%`   | `#157f3c` | Additions       |
| `--git-deleted`        | `0 94% 82%`     | `#fca6a6` | `0 74% 42%`     | `#ba1c1c` | Deletions       |
| `--git-modified`       | `31 97% 72%`    | `#fdba72` | `17 88% 40%`    | `#c04a0c` | Modifications   |
| `--git-merged`         | `271 91% 65%`   | `#a855f7` | `271 91% 65%`   | `#a855f7` | Merges          |
| `--git-added-bright`   | `142 71% 45%`   | `#21c45d` | `142 72% 29%`   | `#157f3c` | Bright addition |
| `--git-deleted-bright`  | `0 91% 71%`     | `#f87272` | `0 74% 42%`     | `#ba1c1c` | Bright deletion |
| `--git-merged-bright`  | `270 95% 75%`   | `#bf7afc` | `272 72% 47%`   | `#7e22ce` | Bright merge    |

> **Note:** `--git-merged` is the only git token with the same value in
> both themes. All others differ.

---

## shadcn Aliases

For compatibility with shadcn/ui components. These are defined using
`var()` references so they resolve automatically per theme.

| Alias                  | Maps to               |
|------------------------|-----------------------|
| `--background`         | `var(--surface-primary)` |
| `--foreground`         | `var(--content-primary)` |
| `--muted`              | `var(--surface-secondary)` |
| `--muted-foreground`   | `var(--content-secondary)` |
| `--primary`            | `var(--content-link)` |
| `--primary-foreground` | `var(--surface-primary)` |

These standalone shadcn tokens use raw HSL values (not references):

| Token     | Dark HSL           | Light HSL         |
|-----------|--------------------|-------------------|
| `--border`| `240 3.7% 15.9%`  | `240 5.9% 90%`   |
| `--input` | `240 3.7% 15.9%`  | `240 5.9% 90%`   |
| `--ring`  | `240 4.9% 83.9%`  | `240 10% 3.9%`   |
