# Coder Kit

Coder Kit is Coder's design system. It documents the foundations, tokens,
and components that make up the Coder UI.

## Foundations

| Topic | Description |
|---|---|
| [Color](./color.md) | Semantic color tokens for content, surfaces, borders, and highlights |
| [Typography](./typography.md) | Font families, sizes, and weights |
| [Spacing & Layout](./spacing.md) | Border radius, icon sizes, and layout primitives |
| [Theming](./theming.md) | Light, dark, and colorblind-accessible theme variants |
| [Icons](./icons.md) | Icon library and usage guidelines |

## Tech Stack

- **Component library:** [shadcn/ui](https://ui.shadcn.com/) (New York style)
- **Styling:** [Tailwind CSS](https://tailwindcss.com/) with CSS custom properties
- **Fonts:** Geist Variable (sans), Geist Mono Variable (mono)
- **Icons:** [Lucide](https://lucide.dev/)
- **Legacy layer:** MUI (being migrated to shadcn/ui + Tailwind)

## Color System

All colors are defined as HSL values in CSS custom properties and consumed
through Tailwind utility classes. This allows opacity modifiers to work
natively (e.g., `bg-surface-primary/50`).

### Semantic Scales

| Scale | Purpose | Examples |
|---|---|---|
| `content-*` | Text and icon foreground | `content-primary`, `content-link`, `content-destructive` |
| `surface-*` | Backgrounds | `surface-primary`, `surface-secondary`, `surface-green` |
| `border-*` | Borders and dividers | `border-default`, `border-success`, `border-destructive` |
| `highlight-*` | Emphasis and accents | `highlight-purple`, `highlight-green`, `highlight-sky` |
| `syntax-*` | Code syntax highlighting | `syntax-key`, `syntax-string`, `syntax-number` |
| `git-*` | Version control status | `git-added`, `git-deleted`, `git-merged` |

## Typography

| Token | Size | Line Height | Weight |
|---|---|---|---|
| `text-2xs` | 0.625rem | 0.875rem | - |
| `text-xs` | 0.75rem | 1rem | 500 |
| `text-sm` | 0.875rem | 1.5rem | 500 |
| `text-base` | 1rem | 1.5rem | 400 |
| `text-3xl` | 2rem | 2.5rem | - |

## Components

Components live in `site/src/components/`. Each component has its own
directory. Storybook stories serve as the primary documentation and test
surface for all components.

### Primitives

Abbr, Alert, Avatar, Badge, Breadcrumb, Button, Calendar, Checkbox,
Collapsible, Combobox, Command, ContextMenu, Dialog, DropdownMenu, Input,
Kbd, Label, Link, Popover, RadioGroup, ScrollArea, Select, Separator,
Skeleton, Slider, Spinner, Switch, Tabs, Textarea, Tooltip

### Data Display

Chart, CodeExample, CopyableValue, Logs, Markdown, Stats,
StatusIndicator, StatusPill, SyntaxHighlighter, Table, Timeline

### Forms & Input

Autocomplete, DateRangePicker, DurationField, FileUpload, Filter, Form,
FormField, IconField, InputGroup, MultiSelectCombobox, MultiUserSelect,
PasswordField, RichParameterInput, Search, SearchField, SelectMenu,
TagInput, UserAutocomplete, OrganizationAutocomplete

### Layout

EmptyState, Expander, FullPageForm, FullPageLayout, LinearProgress,
Loader, Margins, Menu, OverflowY, PageHeader, PaginationWidget, Paywall,
SettingsHeader, Sidebar, SignInLayout, Welcome

### Feedback

Dialogs, HelpPopover, InfoTooltip, Pill, Toaster

## Theme Variants

Coder ships six theme variants defined in `site/src/theme/`:

| Variant | Directory |
|---|---|
| Light | `theme/light` |
| Dark | `theme/dark` |
| Dark Protanopia/Deuteranopia | `theme/darkProtanDeuter` |
| Dark Tritanopia | `theme/darkTritan` |
| Light Protanopia/Deuteranopia | `theme/lightProtanDeuter` |
| Light Tritanopia | `theme/lightTritan` |

## Directory Structure

```
site/coder-kit/
  index.md          # This file
  color.md          # Color tokens reference
  typography.md     # Type scale and font usage
  spacing.md        # Spacing, radius, and sizing tokens
  theming.md        # Theme architecture and customization
  icons.md          # Icon usage and guidelines
```

## Contributing

When adding or changing design tokens:

1. Update the CSS custom properties in `site/src/index.css`.
2. Update the Tailwind config in `site/tailwind.config.js`.
3. Update the relevant doc in this directory.
4. Verify all six theme variants have consistent token coverage.
