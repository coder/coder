# MUI replacement primitives

This is the slice 1 foundation reference for replacing MUI usage in
`site/` with the existing Radix and shadcn-style component layer. This slice is
intentionally documentation-only. It does not migrate existing callsites,
remove MUI packages, remove Emotion packages, or change providers and themes.

Use this file when planning later migration slices. Prefer the existing shared
components in `site/src/components/` before adding a new primitive.

## Existing primitive inventory

The following shared primitives already exist and are the default starting
points for MUI replacement work.

### Inputs and fields

- `Input` provides the base Tailwind input styling.
- `Textarea` provides multiline text input styling.
- `FormField` combines `Label`, `Input`, helper text, error text,
  `aria-invalid`, and `aria-describedby` for form-backed fields.
- `PasswordField` provides the existing password visibility pattern.
- `SearchField` provides the existing search input pattern.

### Selection controls

- `Checkbox` provides the Radix checkbox primitive, including indeterminate
  state support for controlled checkboxes.
- `Label` provides the Radix label primitive.
- `RadioGroup` provides grouped radio selection.
- `Switch` provides binary toggle controls.

### Selectors

- `Select`, `SelectTrigger`, `SelectValue`, `SelectContent`, `SelectItem`,
  `SelectGroup`, and `SelectLabel` provide the closed-list select pattern.
- `Combobox`, `ComboboxButton`, `ComboboxInput`, `ComboboxContent`,
  `ComboboxList`, `ComboboxItem`, and `ComboboxEmpty` provide the searchable
  option-list pattern.

### Feedback

- `Spinner` provides the loading indicator pattern. It supports `size`,
  `loading`, and `children` props.

### Navigation and text

- `Link` provides the text-link pattern. It supports `asChild`, `size`, and
  `showExternalIcon`.

### Overlays

- `Dialog` provides Radix dialog primitives, including `DialogFooter`.
- `DropdownMenu`, `Popover`, `Tooltip`, and `ContextMenu` provide the current
  overlay and menu primitive layer.

## Canonical mapping

| MUI component or API                   | Replacement pattern                                                         | Notes                                                                               |
|----------------------------------------|-----------------------------------------------------------------------------|-------------------------------------------------------------------------------------|
| `TextField`                            | `FormField` for form-backed labeled inputs, or `Input` for bare inputs      | `FormField` already wires label, helper text, errors, and accessible descriptions.  |
| `Link`                                 | `#/components/Link/Link`                                                    | Use `asChild` when composing with router links or another anchor-like child.        |
| `Select` plus `MenuItem`               | `Select`, `SelectTrigger`, `SelectValue`, `SelectContent`, and `SelectItem` | This is the default closed-list replacement for finite options.                     |
| `MenuItem` in action menus             | Menu primitives in a later slice                                            | Do not map action-menu items to `SelectItem`. Slice 1 documents the direction only. |
| `FormControlLabel`                     | `Checkbox` or `Switch` plus `Label`                                         | Keep explicit `id` and `htmlFor` wiring so the label activates the control.         |
| `Checkbox`                             | `#/components/Checkbox/Checkbox`                                            | Use controlled state when indeterminate behavior is required.                       |
| `CircularProgress`                     | `#/components/Spinner/Spinner`                                              | Use `loading` when wrapping fallback children.                                      |
| `DialogActions`                        | `DialogFooter` when dialog wrappers are migrated                            | Existing dialog wrappers remain slice 2 scope.                                      |
| `sx`, MUI `styled`, and MUI `useTheme` | Tailwind classes, design tokens, and CVA variants                           | Emotion cleanup remains final-cleanup scope.                                        |

## Examples

### TextField

Use `FormField` when the field comes from the existing form helpers and needs a
label, helper text, or error text.

```tsx
import { FormField } from "#/components/FormField/FormField";

<FormField label="Name" field={nameField} />;
```

Use `Input` directly when the surrounding UI already owns the label and helper
text.

```tsx
import { Input } from "#/components/Input/Input";

<Input aria-label="Workspace name" value={name} onChange={onNameChange} />;
```

### Link

Use the shared `Link` for visual link styling. Use `asChild` for router links
or other components that should receive the link classes.

```tsx
import { Link } from "#/components/Link/Link";

<Link href="https://example.com">Documentation</Link>;
```

### Select and MenuItem

Use the Radix `Select` family for closed-list options.

```tsx
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "#/components/Select/Select";

<Select value={unit} onValueChange={setUnit}>
  <SelectTrigger>
    <SelectValue placeholder="Unit" />
  </SelectTrigger>
  <SelectContent>
    <SelectItem value="hours">Hours</SelectItem>
    <SelectItem value="days">Days</SelectItem>
  </SelectContent>
</Select>;
```

### FormControlLabel and Checkbox

Use `Checkbox` with `Label`, and wire the `id` to `htmlFor` explicitly.

```tsx
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { Label } from "#/components/Label/Label";

<div className="flex items-center gap-2">
  <Checkbox id={enabledId} checked={enabled} onCheckedChange={setEnabled} />
  <Label htmlFor={enabledId}>Enabled</Label>
</div>;
```

### CircularProgress

Use `Spinner` for loading states. When `loading` is false, `Spinner` renders
its children.

```tsx
import { Spinner } from "#/components/Spinner/Spinner";

<Spinner loading={isLoading} size="sm">
  <span>Ready</span>
</Spinner>;
```

### DialogActions

Use `DialogFooter` when dialog wrappers are migrated in slice 2.

```tsx
import { DialogFooter } from "#/components/Dialog/Dialog";

<DialogFooter>
  <Button variant="outline">Cancel</Button>
  <Button>Save</Button>
</DialogFooter>;
```

## Select versus Combobox

Default to the Radix `Select` family when replacing a MUI select with finite
options. It preserves the existing closed-list user experience and supports a
mechanical migration from `Select` plus `MenuItem` callsites.

Use `Combobox` only when the existing UI is already searchable, filterable,
async, server-filtered, or allows custom values. Do not choose `Combobox` just
because a list is long unless the slice has design approval to change the user
experience.

## Deferred work

- Dialog and menu wrapper migrations are slice 2 scope.
- Existing MUI callsite replacement is later slice scope.
- MUI provider, theme, package, and Emotion cleanup remain final-cleanup scope.
- Add new primitives only when a later slice proves the existing component
  inventory cannot cover the target migration.
