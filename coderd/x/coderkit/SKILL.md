---
name: coder-kit
description: "Build HTML artifacts in the Coder product look with the Coder Kit: ck- component classes, Geist fonts, the blue link accent, light and dark themes. Read it before you write or change an HTML artifact, unless the user asks for another look."
---

# Coder Kit for HTML artifacts

HTML artifacts use the Coder product look by default: the zinc surfaces, the blue link color and the Geist fonts of the Coder dashboard. The Coder Kit gives you that look as `ck-` classes. Use another look only when the user asks for it.

## Start every page from this skeleton

```html
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Release readiness</title>
<link rel="stylesheet" href="coder-kit.css">
<script src="coder-kit.js" defer></script>
<style>
  .cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 16px; }
</style>
</head>
<body class="ck-page">
<main class="ck-main">
  <div class="ck-page-header">
    <hgroup><h1 class="ck-h1">Release readiness</h1><p class="ck-secondary">v2.31, cut on Oct 26</p></hgroup>
  </div>
  ...
</main>
</body>
</html>
```

1. Link `coder-kit.css` by that name, with no folder. When Coder shows the page, it replaces the link with the kit: the stylesheet, the Geist fonts and the Lucide icons. Do not copy the kit into the page, and do not link a CDN copy of Geist.
2. Link `coder-kit.js` only when the page needs tabs, menus, dialogs or toasts. Coder inlines it too.
3. Put `ck-page` on `<body>`. It sets the background, the text color and Geist 14/24.
4. The page follows the theme of the viewer: Coder sets `data-theme="light"` or `data-theme="dark"` on `<html>`. `data-theme` on an element forces a theme on that subtree. Do not write `prefers-color-scheme` rules. Style your own parts with the role tokens below, and they change with the theme.
5. The page runs in a sandbox with no network. Scripts load only from cdnjs, unpkg, jsDelivr `/npm/`, and the Tailwind and jQuery CDNs. Put images inline as `<svg>` or `data:` URIs.
6. A local copy of the page in the workspace shows no kit, because the kit is not in the workspace. Judge the look in the artifact viewer.
7. publish_artifact lists the `ck-` classes that the kit does not have. A misspelled class draws nothing, and a misspelled icon draws a solid square. Fix them and publish again.

## Rules

1. One accent: the link blue, `--coder-accent`. Use the other colors for status, data and categories only.
2. Geist for text, Geist Mono for labels, metadata, IDs, timestamps and code. The kit loads both.
3. Depth comes from surface steps and 1 px borders. Shadows only on floating things, such as menus and dialogs.
4. Radius: 8 px on large containers, 6 px on buttons and inputs. The kit classes already have them.
5. Text contrast at least 4.5:1. The role tokens pass on every surface. Do not write raw hex colors.
6. No emoji. Use the kit icons.
7. Do not draw logos or illustrations in SVG or CSS. `ck-logo` is the Coder mark.
8. The product name is Coder, with a capital C.

## Words

1. Sentence case everywhere: buttons, headings, tabs, table headers, dialog titles. "Create workspace", not "Create Workspace".
2. American English. Second person ("your workspace"). Imperative verbs on actions ("Stop", "Rebuild"). No "we".
3. Keep platform nouns literal: workspace, template, agent, provisioner, organization.
4. Short status words: Running, Stopped, Starting, Failed, Pending, Canceled, Deleting.
5. No period on short labels or one-sentence toasts. An ellipsis on loading text and search placeholders ("Search workspaces…").
6. Lead with the point. No marketing voice, no exclamation marks.

## Page and type

| Class | Use |
| --- | --- |
| `ck-page` | On `<body>`: background, text color, Geist 14/24 |
| `ck-main` | The page column: at most 1380 px wide, centered, 24 px at the sides |
| `ck-h1` | Page title, 32/40 semibold |
| `ck-h2` | 24/32 semibold |
| `ck-h3` | Section or dialog title, 20/28 semibold |
| `ck-body`, `ck-body-bold` | 14/24 body text; bold for row titles |
| `ck-small`, `ck-small-bold` | 12/18 secondary text |
| `ck-mono` | 12/18 Geist Mono: IDs, timestamps, commands |
| `ck-footnote` | 10/14 labels and captions |
| `ck-secondary` | Secondary text color |
| `ck-link` | Link color, underline on hover |
| `ck-code` | Inline code chip: `<code class="ck-code">coder ssh dev</code>` |
| `ck-sr-only` | Text for screen readers only. Its nearest positioned ancestor must hold it (`ck-table-wrap` does) |

## Page layout

```html
<main class="ck-main">
  <div class="ck-page-header">
    <hgroup><h1 class="ck-h1">Workspaces</h1><p class="ck-secondary">Create a workspace from a template.</p></hgroup>
    <div class="ck-page-header-actions"><button class="ck-btn"><i class="ck-icon ck-icon-plus" aria-hidden="true"></i>New workspace</button></div>
  </div>
  <div class="ck-toolbar">
    <div class="ck-input"><i class="ck-icon ck-icon-search" aria-hidden="true"></i><input aria-label="Search workspaces" placeholder="Search workspaces…"></div>
    <button class="ck-btn ck-btn-outline">Filters<i class="ck-icon ck-icon-chevron-down" aria-hidden="true"></i></button>
  </div>
  <div class="ck-table-wrap">...</div>
</main>
```

- `ck-page-header` puts the title and the subtitle in its `<hgroup>`, and the buttons in `ck-page-header-actions`. From 768 px up, the buttons sit at the right.
- `ck-toolbar` is the row above a table: a search `ck-input` that grows, then buttons and filter badges.
- For a report, put each section in a `ck-panel`, or under a `ck-h3`. Lay out cards with your own grid and the role tokens.

## Button

```html
<button class="ck-btn"><i class="ck-icon ck-icon-plus" aria-hidden="true"></i>New workspace</button>
<button class="ck-btn ck-btn-outline">Cancel</button>
<button class="ck-btn ck-btn-outline ck-btn-icon" aria-label="More"><i class="ck-icon ck-icon-ellipsis" aria-hidden="true"></i></button>
```

- `ck-btn` alone is the primary button, 40 px high and at least 80 px wide.
- Variants: `ck-btn-outline`, `ck-btn-subtle` (text only), `ck-btn-destructive` (only for an action that deletes or cannot be undone).
- Sizes: `ck-btn-sm` (32 px), `ck-btn-xs` (24 px), `ck-btn-icon` (32 × 32), `ck-btn-icon-lg` (40 × 40). An icon-only button needs `aria-label`.
- `disabled` or `aria-disabled="true"` greys it out. Works on `<button>` and `<a>`.

## Badge and status

```html
<span class="ck-badge ck-badge-green"><i class="ck-icon ck-icon-check" aria-hidden="true"></i>Healthy</span>
<span class="ck-badge ck-badge-hoverable">status:running<button class="ck-badge-x" aria-label="Remove filter"><i class="ck-icon ck-icon-x" aria-hidden="true"></i></button></span>
<span class="ck-status"><span class="ck-dot ck-dot-running"></span>Running</span>
```

- `ck-badge` alone is the grey default, 24 px. Sizes: `ck-badge-sm` (22 px), `ck-badge-xs` (18 px, no border).
- Tones: `ck-badge-info`, `ck-badge-green` (versions and healthy states only), `ck-badge-warning`, `ck-badge-destructive`, `ck-badge-purple`, `ck-badge-magenta`.
- `ck-badge-hoverable` is a clickable filter tag. `ck-badge-x` is its remove button.
- `ck-status` is the status pill. Pick the dot from the status word: Running `ck-dot-running`, Starting `ck-dot-starting`, Pending `ck-dot-pending`, Stopping, Stopped, Canceling and Canceled `ck-dot-stopped`, Failed, Deleting and Deleted `ck-dot-failed`. Starting and pending pulse.

## Table

```html
<div class="ck-table-wrap">
  <table class="ck-table ck-table-lg">
    <thead><tr><th>Name</th><th>Template</th><th>Status</th><th class="ck-cell-actions"><span class="ck-sr-only">Actions</span></th></tr></thead>
    <tbody>
      <tr class="ck-row-hover" tabindex="0">
        <td><div class="ck-avatar-data"><span class="ck-avatar">ML</span><div><div class="ck-avatar-title">prod-dashboard</div><div class="ck-avatar-subtitle">marcus</div></div></div></td>
        <td>kubernetes</td>
        <td><span class="ck-status"><span class="ck-dot ck-dot-running"></span>Running</span></td>
        <td class="ck-cell-actions"><button class="ck-btn ck-btn-outline ck-btn-icon" aria-label="Workspace actions"><i class="ck-icon ck-icon-ellipsis-vertical" aria-hidden="true"></i></button></td>
      </tr>
    </tbody>
  </table>
</div>
```

- Always wrap the table in `ck-table-wrap`. It scrolls the table sideways on a narrow page.
- The text is 12 px in the secondary color. Header cells are semibold. The body rows get the outer border and the rounded corners.
- `ck-table-lg` makes each body cell 72 px high. `ck-row-hover` outlines a clickable row. `aria-selected="true"` on a `<tr>` fills a selected row.
- `ck-cell-actions` keeps the last column as narrow as its buttons and aligns them to the right.

## Panel

```html
<div class="ck-panel">
  <div class="ck-panel-header"><div class="ck-panel-title">Add members</div><div class="ck-panel-desc">They get the Member role.</div></div>
  <div class="ck-panel-body"><label class="ck-panel-row"><input type="checkbox" class="ck-checkbox"> Marcus Lee</label></div>
  <div class="ck-panel-footer"><button class="ck-btn ck-btn-sm">Add</button></div>
</div>
```

A bordered box for a settings card, a member list or a report section. A list with columns is a `ck-table`.

## Field and input

```html
<label class="ck-field"><span class="ck-label">Workspace name</span><input class="ck-input" value="dev"><span class="ck-hint">Lowercase letters, numbers and dashes.</span></label>
<label class="ck-field"><span class="ck-label">Repository</span><input class="ck-input ck-input-error" value="git@"><span class="ck-error">Not a valid Git URL.</span></label>
```

- `ck-input` styles an `<input>`, a `<textarea>`, or a wrapper that holds an icon and an `<input>`. It is 40 px high, like a default `ck-btn`.
- `ck-input-error` gives the red border, with the message in `ck-error`. `ck-field` stacks `ck-label`, the input, and `ck-hint` or `ck-error`.

## Checkbox, radio and switch

```html
<label class="ck-choice"><input type="checkbox" class="ck-checkbox" checked><span class="ck-choice-text"><span>Notify me</span><span class="ck-choice-desc">Send an email when a build fails.</span></span></label>
<label class="ck-choice"><input type="radio" class="ck-radio" name="stop" checked><span>Always on</span></label>
<label class="ck-choice"><input type="checkbox" class="ck-switch" role="switch" checked><span>Auto-stop after 1 hour idle</span></label>
```

## Tabs

```html
<div class="ck-tabs" role="tablist">
  <button class="ck-tab" role="tab" aria-selected="true" aria-controls="p1">Workspaces <span class="ck-tab-count">12</span></button>
  <button class="ck-tab" role="tab" aria-selected="false" aria-controls="p2" tabindex="-1">Templates</button>
</div>
<div id="p1" role="tabpanel">...</div><div id="p2" role="tabpanel" hidden>...</div>
```

- `ck-tabs` and `ck-tab` are the underline tabs. `ck-segmented` and `ck-segment` are the segmented control.
- With `coder-kit.js`, a click or an arrow key selects a tab and shows the panel that `aria-controls` names.

## Dialog, menu and toast

These need `coder-kit.js`.

```html
<button class="ck-btn ck-btn-outline" data-ck-open="stop">Stop</button>
<dialog class="ck-dialog" id="stop">
  <h2 class="ck-dialog-title">Stop workspace?</h2>
  <p class="ck-dialog-body">Running processes stop. Files in the home folder stay.</p>
  <div class="ck-dialog-actions"><button class="ck-btn ck-btn-outline ck-btn-sm" data-ck-close>Cancel</button><button class="ck-btn ck-btn-sm" data-ck-close>Stop</button></div>
</dialog>

<button class="ck-btn ck-btn-outline ck-btn-icon" data-ck-menu="row-1" aria-haspopup="menu" aria-expanded="false" aria-label="Workspace actions"><i class="ck-icon ck-icon-ellipsis-vertical" aria-hidden="true"></i></button>
<div class="ck-menu" id="row-1" role="menu" hidden>
  <button class="ck-menu-item" role="menuitem"><i class="ck-icon ck-icon-settings" aria-hidden="true"></i>Settings</button>
  <div class="ck-menu-divider" role="separator"></div>
  <button class="ck-menu-item" role="menuitem" data-ck-open="stop">Stop…</button>
</div>
```

- Dialog buttons: the safe action first, the main action last. Escape closes the dialog.
- `CoderKit.menuButton(menu)` returns the button that opened a shared menu last, so an item's click handler can find its row.
- `CoderKit.toast("Workspace ready", { body: "dev", tone: "success", timeout: 5000 })` shows a toast at the bottom right. Tones: `success`, `warning`, `error`, `info`. `timeout: 0` keeps it until your script removes it.
- A static toast: `<div class="ck-toast ck-toast-success"><i class="ck-icon ck-icon-circle-check" aria-hidden="true"></i><div><div class="ck-toast-title">Workspace ready</div><div class="ck-toast-body">dev</div></div></div>`.

## Navbar and avatar

```html
<header class="ck-navbar">
  <i class="ck-logo" role="img" aria-label="Coder"></i>
  <nav class="ck-nav" aria-label="Main"><a class="ck-nav-link" href="#" aria-current="page">Workspaces</a><a class="ck-nav-link" href="#">Templates</a></nav>
  <div class="ck-navbar-end"><span class="ck-avatar">TR</span></div>
</header>
```

- Use `ck-navbar` only for a mockup of a dashboard screen, not for a report.
- `ck-avatar` is 32 px and square: two initials, an `<img>` or an icon. Sizes: `ck-avatar-sm` (24 px), `ck-avatar-lg` (40 px). `ck-avatar-service` has no fill. `ck-avatar-data` puts an avatar next to `ck-avatar-title` and `ck-avatar-subtitle`.

## Icons

`<i class="ck-icon ck-icon-play" aria-hidden="true"></i>` draws a 16 px Lucide icon in the text color. Size it with `width` and `height`. In a button, badge, input or toast it gets the right size. `ck-spin` turns an icon, such as `ck-icon-loader-circle`.

The icons: `activity`, `arrow-left`, `arrow-right`, `bell`, `box`, `calendar`, `check`, `chevron-down`, `chevron-left`, `chevron-right`, `chevron-up`, `circle-alert`, `circle-check`, `circle-x`, `clock`, `cloud`, `code`, `copy`, `cpu`, `download`, `ellipsis`, `ellipsis-vertical`, `external-link`, `eye`, `eye-off`, `file`, `file-text`, `filter`, `folder`, `git-branch`, `git-commit-horizontal`, `git-pull-request`, `globe`, `hard-drive`, `history`, `info`, `key-round`, `layers`, `link`, `loader-circle`, `lock`, `log-out`, `minus`, `pause`, `pencil`, `play`, `plus`, `refresh-cw`, `search`, `server`, `settings`, `square`, `star`, `terminal`, `trash-2`, `triangle-alert`, `upload`, `user`, `users`, `x`. Any other name draws a solid square.

## Style your own parts

Use the kit classes first. For anything else, use the role tokens. They are full colors: `border: 1px solid var(--coder-line)`.

| Role token | Use |
| --- | --- |
| `--coder-bg`, `--coder-surface` | Page and card background |
| `--coder-surface-raised` | Hover fill, selected row, a raised strip |
| `--coder-fg` | Text |
| `--coder-muted` | Secondary text |
| `--coder-line`, `--coder-line-strong` | Lines and borders |
| `--coder-accent`, `--coder-on-accent` | Link blue, and text on it |
| `--coder-ok`, `--coder-warn`, `--coder-bad`, `--coder-info` | Status text |
| `--coder-series-1` to `--coder-series-6` | Data and category colors, in this order |
| `--coder-font-sans`, `--coder-font-mono` | Geist and Geist Mono |

The product tokens of the dashboard are also there as bare HSL numbers, so wrap them: `hsl(var(--surface-secondary))`, `hsl(var(--content-secondary))`, `hsl(var(--border-default))`.

For a chart library from a CDN, read the colors from the tokens, so the chart follows the theme:

```js
const token = (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();
const colors = [1, 2, 3, 4, 5, 6].map((n) => token(`--coder-series-${n}`));
```
