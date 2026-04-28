# FilterSearchField Component — Reproduction Prompt

Build a `FilterSearchField` component for the Coder frontend and wire it
into the Workspaces page to replace the existing `<WorkspacesFilter>` +
`<SelectFilter>` dropdown system. The component is an inline filter bar
with chip tokens, a dropdown for selecting filter categories/values, and
freeform text search that also searches actual resources (workspaces).

## Environment & Tooling

- Workspace is inside the Coder monorepo at `/home/coder/coder`
- Frontend lives in `site/`; package manager is `pnpm`
- Linter: `pnpm biome check --write <files>`
- Type check: `npx tsc --noEmit --pretty`
- Dev server: `./scripts/develop.sh` — backend on port 3000, Vite HMR on
  port 8080
- Use the project's existing UI primitives: `Badge`, `Spinner`, `Avatar`,
  `StatusIndicatorDot`, `cn()` utility. Do NOT add new dependencies.
- Icons come from `lucide-react`: `SearchIcon`, `ListFilter`, `XIcon`,
  `User`, `LayoutPanelTop`, `CircleDot`, `Building2`.
- Do NOT use Radix Popover. The dropdown is a plain `<div>` with absolute
  positioning — see Part 7 for layout details.

## Files to Create/Modify

1. **`site/src/components/FilterSearchField/FilterSearchField.tsx`** — the
   main component (~1450 lines)
2. **`site/src/pages/WorkspacesPage/WorkspacesPageView.tsx`** — replace
   `<WorkspacesFilter>` usage with `<FilterSearchField>`

---

## Part 1: Component Types (exported)

```ts
export type FilterOption = {
  label: string;
  value: string;
  startIcon?: React.ReactNode;  // avatar/icon before the label
  subtitle?: string;            // secondary text below label (e.g. email)
};

export type FilterCategory = {
  key: string;                  // machine key used in query string (e.g. "owner")
  label: string;                // human label shown in UI (e.g. "Owner")
  getOptions: (query: string) => Promise<FilterOption[]>;
  icon?: React.ReactNode;       // icon next to category button
  /**
   * When true, multiple values can be selected for this category
   * (e.g. `status:running status:stopped`). Defaults to false
   * (selecting a new value replaces the existing chip).
   */
  multiSelect?: boolean;
};

export type FilterChip = {
  key: string;                  // the category key
  value: string;                // selected value; empty string = incomplete
};

export type SearchResult = {
  label: string;
  value: string;
  startIcon?: React.ReactNode;
  subtitle?: string;
};

export type FilterSearchFieldProps = {
  value: string;                // controlled: current query string "owner:me status:running"
  onChange: (query: string) => void;
  categories: FilterCategory[];
  getSearchResults?: (query: string) => Promise<SearchResult[]>;
  placeholder?: string;
  className?: string;
  autoFocus?: boolean;
  ref?: Ref<HTMLInputElement>;
  "aria-label"?: string;
};
```

## Part 2: Helper Functions

### `parseQuery(query, categories) → { chips: FilterChip[], freeform: string }`

- Split on spaces. For each token, check if it matches `key:value` where
  key is a known category.
- Known `key:value` tokens become chips. Everything else becomes freeform
  text.
- Handles duplicate keys correctly (e.g. `"owner:me status:stopped"` → two
  chips).

### `serializeQuery(chips, freeform) → string`

- Joins `key:value` pairs (only complete chips with non-empty value) then
  appends freeform text.
- Chips come first, freeform at the end.

## Part 3: Sub-components

### `ChipBadge` — renders a filter chip as a Badge

- Complete chips show `CategoryLabel:value [X]` with an X button to remove.
- Incomplete chips (value = "") show a dashed-border Badge with just
  `CategoryLabel:`.
- X button uses `onMouseDown={e.preventDefault()}` to prevent input blur,
  then `onClick` to remove.

### `CategoryButton` — pill-shaped div for "Filter by" mode

- `<div role="option" tabIndex={-1}>` — NOT a `<button>` (nested inside a
  listbox).
- Rounded border, icon + label, hover/focus styles.
- Uses `onMouseDown={e.preventDefault()}` to keep focus on the input.
- `aria-selected` reflects highlighted state.

### `OptionItem` — a single selectable option row

- `<div role="option" tabIndex={-1}>` with `aria-selected`.
- Icon (in a 24px container), label, optional subtitle, optional
  `categoryLabel:` prefix.
- Highlighted state: `bg-surface-secondary text-content-primary`.
- Keyboard-highlighted state (when `showFocusRing`): adds `ring-2
  ring-content-link`.
- Uses `onMouseDown={e.preventDefault()}` to prevent blur.

### `SearchResultsSection` — renders the "Workspaces" resource results section

- Shows a `<Spinner>` while loading with no results yet.
- Returns null if not ready or no results.
- Section heading: `"Workspaces"` (`role="presentation"`, text-xs,
  font-medium, text-content-secondary).

## Part 4: Main Component State & Architecture

### Dropdown modes (discriminated union)

```ts
type DropdownMode =
  | { type: "categories" }
  | { type: "options"; categoryKey: string }
  | { type: "typeahead" }
```

### State variables

- `chips: FilterChip[]` — local chip state synced from `value` prop via
  useEffect
- `freeformText: string` — the current free text in the input
- `isOpen: boolean` — whether the dropdown is open
- `dropdownMode: DropdownMode`
- `highlightedIndex: number` — keyboard nav index, starts at **-1** (no
  item highlighted)
- `isKeyboardNav: boolean` — tracks whether the user is navigating via
  keyboard (true) or mouse (false)
- `categoryOptions: FilterOption[]` / `isLoadingOptions` — for options mode
- `categorySearchText: string` — text typed while in options mode (filters
  options)
- `typeaheadMatches: FilterCategory[]` — categories whose key `startsWith`
  input (for Tab completion, no visual indicator shown)
- `globalResults: GlobalSearchResult[]` / `isLoadingGlobal` — suggestions
  from all categories
- `searchResults: SearchResult[]` / `isLoadingSearch` — resource search
  results
- `globalSearchReady` / `searchResultsReady` — booleans to prevent "no
  results" flash while first request is in-flight
- Three `useRef(0)` request ID counters (`categorySearchIdRef`,
  `globalSearchIdRef`, `searchIdRef`) to discard stale async responses
- `debounceRef` — `useRef<ReturnType<typeof setTimeout>>` for 300ms
  debounce on global/search API calls

### `highlightedIndex` semantics

- **-1**: No item highlighted. Focus ring stays on the search box input.
- **≥ 0**: An item in the dropdown is highlighted. `aria-activedescendant`
  on the input points to `filter-option-${highlightedIndex}`.
- ArrowDown from -1 → 0. ArrowUp from -1 → last item.
- Typing resets to -1.
- Mouse enter on items sets to their index.

### `isKeyboardNav` semantics

- ArrowDown/ArrowUp/ArrowRight/ArrowLeft set `isKeyboardNav = true`.
- `onMouseEnter` handlers set `isKeyboardNav = false`.
- Blue `ring-2 ring-content-link` only shows on highlighted items when
  `isKeyboardNav === true`.
- Mouse hover shows only `bg-surface-secondary` (no ring).
- When `isKeyboardNav && highlightedIndex >= 0`, the input group's focus
  ring is suppressed (one focus indicator at a time per WCAG).

### Sync from external value

```ts
useEffect(() => {
  const newParsed = parseQuery(value, categories);
  setChips(newParsed.chips);
  setFreeformText(newParsed.freeform);
}, [value, categories]);
```

### `emitChange(nextChips, nextFreeform)`

- Filters out incomplete chips (value = ""), serializes, calls `onChange`.

### `showDropdown` and `hasDropdownContent`

```ts
const hasDropdownContent = useMemo(() => {
  if (dropdownMode.type === "categories") return true;
  if (dropdownMode.type === "options") return true;
  return (
    filteredCategories.length > 0 ||
    globalResults.length > 0 ||
    searchResults.length > 0 ||
    isLoadingGlobal ||
    isLoadingSearch
  );
}, [/* deps */]);

const showDropdown = isOpen && hasDropdownContent;
```

`showDropdown` controls rendering of the dropdown div.

## Part 5: Core Callbacks

### `loadCategoryOptions(categoryKey, query)`

- Looks up category from a `categoryMap` (useMemo Map), calls
  `getOptions(query)`, sets `categoryOptions`. Uses request ID pattern to
  discard stale responses.

### `loadSearchResults(query)`

- Calls `getSearchResults(query)` if provided. Sets `searchResults`. Uses
  request ID pattern.

### `loadGlobalResults(query)` — the "Filter suggestions" search

- Queries **ALL** categories.
- For each category, checks `categoryNameMatches` **first**:
  - If the category name (key or label) contains the query → calls
    `getOptions("")` (empty string) to fetch ALL options, includes all of
    them.
  - If the category name doesn't match → calls `getOptions(query)` and
    filters locally by label/value containing the query.
- This ensures: "ow" → owner category matches by name → shows ALL users
  (including "member"); "test" → owner doesn't match by name, but
  "testuser01" matches by value → shows that user.
- Uses request ID + `Promise.allSettled` pattern.

### Debounce (300ms)

- `loadGlobalResults` and `loadSearchResults` in typeahead mode are
  debounced 300ms via `debounceRef`.
- `computeTypeahead` (local category name matching) remains immediate.
- The debounce timer is cleared on dropdown close and on unmount.

### `closeDropdown()`

- Cancels pending debounce timer.
- Removes incomplete chips, resets ALL state (dropdownMode to "categories",
  clears all results/loading flags, bumps all request ID refs to cancel
  in-flight requests).
- Calls `emitChange` with the complete chips.

### `selectCategory(categoryKey)`

- For single-select categories: strips incomplete chips AND removes
  existing chips with the same key. For multiSelect categories: only
  strips incomplete chips.
- Adds an incomplete chip `{key, value: ""}`.
- Switches to options mode, loads options for that category.
- Resets global/search results and bumps their request IDs.

### `selectOption(option, categoryKey?)`

- Two paths:
  - **With `categoryKey`** (from global search suggestions): For
    single-select, replaces existing chip of same key. For multiSelect,
    appends. Creates complete chip.
  - **Without `categoryKey`** (from options mode): Finds the incomplete
    chip, fills in its value. For single-select, also removes any other
    chip with the same key.
- Resets ALL loading state, bumps all request ID refs.
- Calls `emitChange`, keeps dropdown open (in categories mode), focuses
  input.

### `selectSearchResult(result)`

- Removes incomplete chips, sets freeform text to `result.value`, closes
  dropdown, emits change.

### `removeChip(chipIndex)`

- Removes by original array index, emits change, focuses input.

### `computeTypeahead(text)`

- Filters categories where key or label `startsWith` the trimmed lowercase
  input (stricter than `includes` — this is for Tab completion).

### `filteredCategories` (useMemo)

- Categories where key or label `includes` the freeform text
  (case-insensitive). Used for rendering the category list in typeahead
  mode. Falls back to all categories when input is empty.

### `checkForKeyColonPattern(text)`

- Regex `/(\\S+):$/` — detects `"owner:"` → starts an incomplete chip,
  switches to options mode. Respects `multiSelect` per category.
- Regex `/(\\S+):(\\S+)\\s$/` — detects `"owner:me "` (trailing space) →
  creates a complete chip immediately. Respects `multiSelect` per category.

### `handleInputChange(e)`

- In options mode: updates `categorySearchText`, reloads options.
- Otherwise: checks for key:colon pattern, updates freeformText.
- If empty: switches to categories mode, clears results.
- If non-empty: computes typeahead (immediate), switches to typeahead mode,
  debounces (300ms) global results + search results.
- Clears globalResults before loading (`setGlobalResults([])`) and sets
  `isLoadingGlobal(true)`.

## Part 6: Keyboard Navigation

### `navItems` (useMemo) — flat list for arrow key navigation

- Categories mode: all categories.
- Options mode: `categoryOptions`.
- Typeahead mode: `filteredCategories` (as category items) + `globalResults`
  (as global items) + `searchResults` (as search items, only if
  `searchResultsReady`).

### `handleKeyDown`

- **Escape**: close dropdown, blur input.
- **Backspace** (empty input, not in options mode): remove last chip.
- **Backspace** (options mode, empty search text): cancel incomplete chip,
  go back to categories.
- **Tab** (typeahead mode, has typeahead matches): complete top match via
  `selectCategory`. No visual Tab kbd badge in the dropdown.
- **ArrowDown/ArrowUp**: cycle `highlightedIndex` through `navItems`. Sets
  `isKeyboardNav = true`.
- **ArrowRight/ArrowLeft** (categories mode only): also cycle
  `highlightedIndex` for horizontal button layout. Sets `isKeyboardNav =
  true`.
- **Enter** (highlightedIndex < 0): closes dropdown, blurs input (submits
  current freeform search).
- **Enter** (highlightedIndex ≥ 0): activates the highlighted navItem
  based on its `kind` (category/option/global/search).

### Scroll into view

```ts
useEffect(() => {
  if (!isOpen || highlightedIndex < 0) return;
  const el = document.getElementById(`filter-option-${highlightedIndex}`);
  if (el) el.scrollIntoView({ block: "nearest" });
}, [highlightedIndex, isOpen]);
```

## Part 7: Layout & Render

### Overall structure (NO Radix Popover)

```
<div ref={containerRef} className="relative rounded-md">
  {/* Input group — has border, focus ring via has-[:focus] */}
  <div className={cn(
    "...border border-border...",
    !(isKeyboardNav && highlightedIndex >= 0) &&
      "has-[:focus]:ring-2 has-[:focus]:ring-content-link"
  )}>
    [SearchIcon] [Chips + Input (role="combobox")] [ListFilter icon]
  </div>
  {/* Dropdown — absolutely positioned, floats over page content */}
  {showDropdown && (
    <div id="filter-search-listbox" role="listbox"
         className="absolute left-0 right-0 top-full z-50 max-h-80
                    overflow-y-auto rounded-md border border-border
                    bg-surface-primary shadow-lg mt-1.5">
      {categories mode | options mode | typeahead mode}
    </div>
  )}
</div>
```

Key layout decisions:
- Dropdown is `absolute top-full mt-1.5` — floats over content, does NOT
  push content down.
- Dropdown has its own `border` + `shadow-lg` + `rounded-md` — visually
  separate from search box.
- Search box gets standard `has-[:focus]:ring-2 ring-content-link` focus
  ring.
- NO unified ring around both search + dropdown (too complex with absolute
  positioning).
- Focus ring on search box is suppressed when keyboard is highlighting a
  dropdown item (one focus indicator at a time per WCAG).
- Grey separator between search and dropdown comes from the dropdown's own
  border-top.

### Input group (the search bar)

- `group/filter-search flex items-start w-full min-w-0 min-h-10 rounded-md
  border border-solid border-border bg-transparent transition-colors`
- Focus ring: `has-[:focus]:ring-2 has-[:focus]:ring-content-link` —
  suppressed when `isKeyboardNav && highlightedIndex >= 0`.
- onClick focuses the input and opens the dropdown.

### Search icon (left)

- `flex shrink-0 items-center justify-center h-10 pl-3 pr-2
  text-content-secondary`

### Chips + input area (center)

- `flex flex-1 flex-wrap items-center gap-1.5 min-w-0 py-1.5 cursor-text`
- Renders complete chips via `completeChipEntries` (a useMemo that maps
  chips to `{chip, originalIndex}` — needed because `removeChip` needs the
  original index, not the rendered index).
- Renders incomplete chip indicator (dashed border Badge with
  `CategoryLabel:`).
- Text input: `flex-1 min-w-[40px] h-7 bg-transparent border-none
  outline-none text-sm`
  - Placeholder only shown when no chips exist.
  - Value is `categorySearchText` in options mode, `freeformText`
    otherwise.
  - onFocus opens dropdown if not already open.
  - onBlur closes dropdown unless focus moved within `containerRef`.

### WCAG attributes on `<input>`

```tsx
role="combobox"
aria-expanded={isOpen}
aria-haspopup="listbox"
aria-autocomplete="list"
aria-controls={isOpen ? "filter-search-listbox" : undefined}
aria-activedescendant={
  isOpen && highlightedIndex >= 0 && navItems.length > 0
    ? `filter-option-${highlightedIndex}`
    : undefined
}
aria-label={ariaLabel ?? "Filter search"}
```

### ListFilter icon (right)

- `flex shrink-0 items-center justify-center self-stretch pl-2 pr-3
  border-0 border-l border-solid border-border text-content-secondary
  hover:text-content-primary transition-colors cursor-pointer`
- `self-stretch` so the left border extends full height when chips wrap.

### Dropdown `<div>` (the listbox)

- `id="filter-search-listbox"` with `role="listbox"` and
  `aria-label="Filter options"`.
- Single `role="listbox"` — no nested listbox elements.
- Heading `<p>` tags inside ("Filter by", "Filter suggestions",
  "Workspaces") use `role="presentation"` to avoid WCAG violations inside a
  listbox.

### Categories mode

- Heading: "Filter by" (`role="presentation"`, text-xs, font-medium,
  text-content-secondary, mb-2).
- Flex-wrap grid of `CategoryButton` components (div role="option").

### Options mode

- Loading: centered `<Spinner>`.
- Empty: "No results found" centered text.
- Results: list of `OptionItem` components.

### Typeahead mode (three sections)

1. **Matching categories** (only if `filteredCategories.length > 0`):
   - List of category rows with icon and label (div role="option").
   - Highlighted state same as OptionItem.

2. **Filter suggestions** (`globalResults`):
   - Shows single spinner while `isLoadingGlobal && isLoadingSearch` and
     both result arrays are empty (consolidated — no double spinner).
   - Section heading: `"Filter suggestions"` (role="presentation").
   - Each result rendered as `OptionItem` with `categoryLabel` prefix
     (e.g. "Owner: testuser01").
   - Nav index offset = `filteredCategories.length`.

3. **Workspaces** (resource search results, only if `getSearchResults`
   provided):
   - Rendered by `SearchResultsSection` sub-component.
   - Section heading: `"Workspaces"` (role="presentation").
   - Nav index offset = `filteredCategories.length + globalResults.length`.

### Section spacing

- Categories mode container: `p-3`
- Categories section in typeahead: `p-2`
- Suggestions section: `px-2 pb-2 pt-1` with heading `px-2 pb-1`
- Results section: `px-2 pb-2 pt-1` with heading `px-2 pb-1`
- No `border-t` separators between sections.

---

## Part 8: Workspaces Page Integration

### In `WorkspacesPageView.tsx`

**Replace** the existing `<WorkspacesFilter>` (and its `<SelectFilter>`
dropdowns) with a single `<FilterSearchField>`.

### Categories (built via `useMemo` with deps `[me.username, me.avatar_url]`)

1. **Owner** — `key: "owner"`, `icon: <User>`, `getOptions`: calls
   `API.getUsers({q: query, limit: 25})`, maps to FilterOption with
   avatar. Pins current user (`me`) first in the list. **No multiSelect**
   — backend uses `parser.String` (single value only).

2. **Template** — `key: "template"`, `icon: <LayoutPanelTop>`,
   `getOptions`: calls `API.getTemplates()`, client-side filters by
   name/display_name containing query. Uses template icon avatar. **No
   multiSelect** — backend uses `parser.String`.

3. **Status** — `key: "status"`, `icon: <CircleDot>`, `getOptions`:
   returns static list `["running", "stopped", "failed", "pending"]`
   mapped through `getDisplayWorkspaceStatus` for labels and
   `StatusIndicatorDot` for icons. Create a `getStatusVariant` helper that
   maps workspace status display types to StatusIndicatorDot variants:
   `{active: "pending", inactive: "inactive", success: "success", error:
   "failed", danger: "warning", warning: "warning"}`. **No multiSelect** —
   backend uses `ParseCustom` (single enum).

4. **Organization** — `key: "organization"`, `icon: <Building2>`,
   `getOptions`: calls `API.getOrganizations()`, maps to FilterOption with
   avatar. Always shown (no `showOrganizations` gate). **No multiSelect**.

**Backend limitation:** All workspace filter fields (`owner`, `template`,
`status`, `name`) use `parser.String` or single-enum parsing. None support
multiple values. The `multiSelect` toggle exists on the `FilterCategory`
type for future use, but no workspace category should set it to `true`
until the backend is updated to use `parser.Strings` or `ParseCustomList`.

### `getSearchResults` callback (via `useCallback`)

- Calls `API.getWorkspaces({ q: query, limit: 5 })`.
- Maps each workspace to `SearchResult` with template icon avatar,
  workspace name, subtitle `"ownerName · statusText"`.

### FilterSearchField usage

```tsx
<FilterSearchField
  value={filterState.filter.query}
  onChange={filterState.filter.update}
  categories={categories}
  getSearchResults={getSearchResults}
  placeholder="Search workspaces..."
  aria-label="Filter workspaces"
  className="w-fit min-w-[min(550px,100%)] max-w-full"
/>
```

### Critical: Use `filter.update` not `filter.debounceUpdate`

The existing `useFilter` hook has a `debounceUpdate` (500ms delay). Using
it causes a race condition: the sync effect (which re-parses the `value`
prop) resets internal chips before the debounce fires, causing the table to
get stuck in a loading state. Use `filter.update` (immediate) instead.

---

## Critical Implementation Details (Bug Prevention)

These are subtle issues discovered during development. Getting them wrong
causes visible bugs:

### 1. Single-select vs multiSelect per category

`FilterCategory` has an optional `multiSelect?: boolean` (defaults to
false). When false, selecting a new value for the same category key
**replaces** the existing chip. When true, multiple chips with the same key
are allowed. All four chip-creation paths (`selectCategory`,
`selectOption` with/without `categoryKey`, `checkForKeyColonPattern`)
check `categoryMap.get(key)?.multiSelect` before deciding whether to strip
existing chips of the same key.

### 2. WCAG: `role="combobox"` on the `<input>`, not a wrapper

The `<input>` element itself has `role="combobox"`, `aria-expanded`,
`aria-haspopup`, `aria-controls`, and `aria-activedescendant`. NOT a
wrapper `<div>`.

### 3. WCAG: Single `role="listbox"` on the dropdown

One `role="listbox"` on the outer dropdown `<div>`. No nested listbox
elements. `CategoryButton` is a `<div role="option">`, not a `<button>`.
Heading `<p>` tags inside the listbox use `role="presentation"`.

### 4. WCAG: One focus indicator at a time

When `isKeyboardNav && highlightedIndex >= 0`, the search box's focus ring
is suppressed. Blue `ring-2 ring-content-link` shows only on the
highlighted dropdown item. When `highlightedIndex < 0`, the search box
shows its normal focus ring. Mouse hover only shows `bg-surface-secondary`
on items (no blue ring).

### 5. WCAG: `aria-controls` only when open

`aria-controls="filter-search-listbox"` is set only when `isOpen` is true.
When closed, it's `undefined`.

### 6. Click focus management with `onMouseDown={e.preventDefault()}`

Every interactive element inside the dropdown (X buttons on chips, category
buttons, option items, search results) must use
`onMouseDown={e.preventDefault()}` to prevent the input from losing focus.
Without this, clicking triggers onBlur → closeDropdown before the click
event fires.

### 7. State cleanup on every transition

`selectOption`, `selectCategory`, and `closeDropdown` must ALL: reset
every loading flag to false, set `searchResultsReady`/`globalSearchReady`
to true, and bump all three request ID refs to cancel in-flight requests.
Without this, stale async responses can set loading states that never
clear, causing permanent spinners.

### 8. `loadGlobalResults` must call `getOptions("")` for matching categories

When a category name matches the query (e.g. "ow" → "owner"), call
`getOptions("")` to fetch ALL options — not `getOptions(query)` which
would only return API results matching the partial text. Check
`categoryNameMatches` BEFORE calling `getOptions`.

### 9. Debounce only on API calls, not local matching

`loadGlobalResults` and `loadSearchResults` are debounced 300ms via
`debounceRef` in the `handleInputChange` handler. `computeTypeahead`
(local category name matching) runs immediately — no debounce. The
debounce timer is cancelled on close and on unmount.

### 10. `completeChipEntries` index mapping

The rendered chip list only shows complete chips, but `removeChip` needs
the original index in the `chips` array (which includes incomplete chips).
Use a useMemo to build `{chip, originalIndex}[]`.

### 11. Chips wrapping behavior

Use `flex-wrap` on the chips+input container, NOT horizontal scroll. Use
`min-h-10` on the input group (not fixed `h-10`) so it grows when chips
wrap. Use `items-start` so icons align to the top row. The ListFilter
icon uses `self-stretch` so its left border extends the full height.

### 12. Width behavior

The component root has `className={cn("relative rounded-md", className)}`
— no default `w-full`. The consumer controls width. The Workspaces page
passes `className="w-fit min-w-[min(550px,100%)] max-w-full"` — prefers
550px, grows with chip content, caps at page width, no horizontal scroll
on narrow viewports.

### 13. Dropdown positioning

The dropdown is an inline `<div>` with `absolute left-0 right-0 top-full
z-50 mt-1.5`. It is NOT rendered in a Radix portal — it lives inside
`containerRef`. This means `onBlur` only needs to check
`containerRef.current?.contains(related)` to decide whether to close.

### 14. Enter key behavior

When `highlightedIndex < 0` (no item highlighted), Enter closes the
dropdown and blurs the input (submits the current freeform search). When
`highlightedIndex >= 0`, Enter activates the highlighted item.
