# Checkbox

A toggle control for binary or indeterminate selections. Built on
[Radix UI Checkbox](https://www.radix-ui.com/primitives/docs/components/checkbox)
with [Lucide](https://lucide.dev/) icons.

Source: `site/src/components/Checkbox/Checkbox.tsx`

---

## Anatomy

```
┌──────────────────────────────────────┐
│ ┌──────┐                             │
│ │ 18×18│  Label text                  │
│ │  box │  Description (optional)      │
│ └──────┘                             │
│  4 px margin (m-1) on all sides      │
└──────────────────────────────────────┘
```

- **Box size**: 18×18 px (`size-[18px]`)
- **Margin**: 4 px on all sides (`m-1`)
- **Border radius**: 2 px (`rounded-xs`, computed from `calc(var(--radius) - 6px)`)
- **Gap** between checkbox and label: 10 px (`gap-2.5`) in the canonical
  `WithLabel` story. Usage patterns vary (see [Usage in context](#usage-in-context)).
- **Checkbox vertical alignment**: `pt-0.5` (2 px) in the `WithLabel` story;
  `mt-1` (4 px) in `RoleSelector`

---

## Styles

### Box (unchecked)

```
size-[18px]
rounded-xs
border border-border border-solid
bg-surface-primary
```

The border token `border-border` resolves to `hsl(var(--border-default))`
through the Tailwind color config.

### Box (checked)

```
bg-surface-invert-primary
border-surface-invert-primary
text-content-invert
```

### Box (indeterminate)

Same tokens as checked.

### Icons

Both icons come from `lucide-react` and render inside a centered
indicator (`absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2`).

| State         | Icon        | Size   | Stroke width |
|---------------|-------------|--------|--------------|
| Checked       | `CheckIcon` | 16×16 (`size-4`) | 2.5 |
| Indeterminate | `MinusIcon` | 16×16 (`size-4`) | 2.5 |

Icon color is inherited from the parent's `text-content-invert` class.

### Label (from `WithLabel` story)

```
text-sm font-medium
→ 14px / 24px, weight 500
color: content-primary (via inheritance)
```

When the checkbox is disabled, the label uses the `peer-disabled:` modifier:

```
peer-disabled:cursor-not-allowed
peer-disabled:text-content-disabled
```

### Description (from `WithLabel` story)

```
text-sm text-content-secondary mt-1
→ 14px / 24px, weight 500, color: content-secondary
```

---

## States

| State              | Box background             | Box border                 | Icon | Notes |
|--------------------|----------------------------|----------------------------|------|-------|
| Unchecked          | `surface-primary`          | `border-default`           | none | |
| Unchecked, hover   | `surface-primary`          | `border-secondary`         | none | `hover:enabled:` |
| Checked            | `surface-invert-primary`   | `surface-invert-primary`   | ✓    | |
| Checked, hover     | `surface-invert-secondary` | `surface-invert-secondary` | ✓    | |
| Indeterminate      | `surface-invert-primary`   | `surface-invert-primary`   | —    | |
| Indeterminate, hover | `surface-invert-secondary` | `surface-invert-secondary` | —  | |
| Disabled, unchecked | `surface-primary`         | `border-default`           | none | `cursor-not-allowed` |
| Disabled, checked  | `surface-tertiary`         | `surface-tertiary`         | ✓    | `cursor-not-allowed` |

> **Note:** There is no explicit disabled+indeterminate style. The
> `disabled:bg-surface-primary` class applies but may conflict with the
> `data-[state=indeterminate]:bg-surface-invert-primary` class depending
> on CSS specificity. This could be a gap worth addressing.

No opacity values are used on the Checkbox component itself. Disabled
states are communicated through background and border color changes.
Consumers may add opacity externally (e.g., `RoleSelector` uses
`opacity-50` on the wrapper for non-assignable roles).

### Focus ring

```
ring-2
ring-content-link
ring-offset-[3px]
ring-offset-surface-primary
```

This uses Tailwind's ring utilities (box-shadow based), not CSS `outline`.

### Transition

No transition is defined on the component.

---

## Usage in context

The Checkbox component is used with varying layouts depending on context.
Gap, alignment, and disabled patterns are not standardized across usages.

### WithLabel story (canonical)

The simplest label pattern from the Storybook story.

```
Container:  flex gap-2.5           (10px gap)
Checkbox:   wrapped in div.pt-0.5  (2px top padding for alignment)
Label:      text-sm font-medium    (14px/24px, weight 500)
Desc:       text-sm text-content-secondary mt-1
```

### Table rows (WorkspacesTable, TasksTable)

Checkboxes in data table rows alongside `AvatarData`.

```
Layout:     flex items-center gap-5    (20px gap)
Row height: 72px
Avatar:     size="lg" (40px, rounded-[6px])
Name:       text-sm font-semibold text-content-primary    (14px, weight 600)
Subtitle:   text-content-secondary text-xs font-medium    (12px/16px, weight 500)
Checked bg: bg-surface-secondary
```

### MultiUserSelect (member picker)

Checkbox rows in a popover picker with avatars.

```
Layout:     flex items-center gap-3    (12px gap)
Row:        flex min-h-[64px] items-center px-4 py-3
Hover:      ring-1 ring-inset ring-border-secondary
Checked bg: bg-surface-secondary
Avatar:     size="lg" (40px, rounded-[6px])
Name:       text-sm font-semibold text-content-primary    (14px, weight 600)
Subtitle:   text-content-secondary text-xs font-medium    (12px/16px, weight 500)
```

### RoleSelector (permissions)

Checkbox rows for role assignment in a scrollable container.

```
Layout:      flex items-start gap-2    (8px gap)
Checkbox:    mt-1 shrink-0             (4px top margin for alignment)
Container:   border border-border rounded-md p-3 flex flex-col gap-2
             overflow-y-auto max-h-72
Label:       text-sm font-medium       (14px/24px, weight 500)
Description: text-sm text-content-secondary
Non-assignable: cursor-not-allowed opacity-50
```

### Form checkboxes (settings, setup)

Simple inline checkbox + label pairs.

```
Layout:     flex items-center gap-2    (8px gap)
            or flex items-start gap-2
Checkbox:   mt-0.5 (2px) for multi-line alignment
Label:      text-sm font-medium
```

---

## Behavior

- **Click** toggles between unchecked and checked.
- **Indeterminate** is a programmatic third state (e.g., parent group with
  mixed children). Clicking an indeterminate checkbox resolves it per app
  logic.
- **Disabled** checkboxes use `cursor-not-allowed` and color changes rather
  than opacity.
- The checkbox must be **controlled** (pass `checked` prop) to support the
  indeterminate state.

---

## Theme support

All colors reference semantic tokens. The component renders correctly in
both dark and light themes via CSS custom properties. No theme-specific
logic exists in the component.
