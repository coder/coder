# RFC: Unified FilterSearchField Component

> 🪐 RFC Process:
>
> 1. Start an RFC with a Problem Statement.
> 2. Collaborate on a draft RFC. Customer-facing issues should include Ben. Most should include backend and frontend input.
> 3. Announce the RFC in #dev and put an RFC Review meeting on the calendar. Invite Sprints & Releases so anyone can opt in, and invite any stakeholders individually.
> 4. During review meetings, show reviewers what the RFC currently says and field questions and comments.
> 5. Modify the RFC as needed based on feedback and schedule another review meeting. Repeat until stakeholders approve.
> 6. Announce the RFC is approved in #dev.
> 7. Write epics and tickets as needed for the work the RFC prescribes and put them in the Backlog.

---

# Stakeholders

Who needs to approve this? Feel free to add others!

- [ ] Product Lead:
- [ ] Engineering DRI:
- [ ] CTO:

---

# Problem Statement

The current filtering system across Coder's dashboard (`<Filter>` + `<SelectFilter>` + page-specific wrappers like `<WorkspacesFilter>`) has several problems:

1. **Fragmented UX** — Filters are split between a free-text search input and a row of separate dropdown buttons (Owner, Template, Status, etc.). Users must mentally map between the raw query string (`owner:me status:running`) shown in the text input and the dropdown selections. There's no unified interaction model.

2. **High boilerplate per page** — Each filtered page (Workspaces, Audit, Connection Logs, AI Bridge) requires its own filter wrapper component, menu components, menu hook wiring, and skeleton states. The Workspaces filter alone spans 5 files and ~850 lines of glue code (`WorkspacesFilter.tsx`, `menus.tsx`, `Filter.tsx`, `SelectFilter.tsx`, `UserFilter.tsx`).

3. **No typeahead or resource preview** — Typing `test` in the search bar gives no indication of what the query will match. Users can't see that `test` would match a workspace named `test-dev` or a user named `testuser01` until they press Enter and wait for results. There's no inline suggestion of which filter categories are relevant.

4. **Discoverability** — New users don't know what filter keys are available. The dropdown buttons help, but they're visually disconnected from the text input and the `key:value` syntax is not taught anywhere in the UI.

5. **Accessibility gaps** — The current system uses separate Radix popovers for each dropdown, resulting in multiple focus traps, inconsistent keyboard navigation, and no unified `combobox` ARIA pattern.

We need a single, reusable filter component that combines chip-based structured filters with freeform search and live typeahead — reducing per-page boilerplate while improving discoverability, speed, and accessibility.

---

# UX & Design



---

# User Stories

**As a workspace user**, I want to type partial text and immediately see matching filter categories, filter values, and workspaces so I can find what I need without memorizing `key:value` syntax.

**As a workspace user**, I want selected filters to appear as removable chips in the search bar so I can see my active filters at a glance and modify them without editing raw text.

**As a workspace user**, I want to press Tab to autocomplete a filter category (e.g., typing `ow` → Tab → `owner:`) so filtering is fast for keyboard-heavy workflows.

**As a platform admin**, I want the same filter component on every filtered page (Workspaces, Audit, Connection Logs) so the interaction model is consistent across the dashboard.

**As a frontend developer**, I want to add filtering to a new page by defining an array of `FilterCategory` objects and passing them as props — no menu hooks, skeleton wrappers, or per-page filter components required.

**As a user relying on assistive technology**, I want the filter to follow the WCAG combobox pattern (`role="combobox"` with `aria-activedescendant`) so my screen reader correctly announces available options as I navigate.

---

# Requirements

## Initial Functional Requirements

- **Chip-based filter display** — Active filters render as `key:value` chips inside the search input. Each chip has an X button to remove it. Chips serialize to/from a standard query string (`owner:me status:running workspace-name`).
- **Category picker** — On focus (empty input), a dropdown shows all available filter categories as pill buttons. Clicking or pressing Enter on a category starts a chip for that category and shows its available values.
- **Options dropdown** — After selecting a category, the dropdown shows that category's options (fetched async via `getOptions`). The user can type to filter options, then click or press Enter to complete the chip.
- **Typeahead mode** — When the user types freeform text, the dropdown shows three sections:
    - **Matching categories** — categories whose name contains the typed text (e.g., `ow` → Owner).
    - **Filter suggestions** — options across all categories that match the text (e.g., `test` → Owner: testuser01). When a category name matches, ALL its options are shown (fetched via `getOptions("")`).
    - **Resource results** — actual matching resources (e.g., workspaces) so the user can preview what the query will return.
- **Keyboard navigation** — Full arrow key navigation across all dropdown items. Tab completes the top matching category. Enter with no item highlighted submits the current search. Escape closes the dropdown.
- **Single-select by default, opt-in multiSelect** — Each `FilterCategory` defaults to single-select (new value replaces existing chip of the same key). Categories can opt into `multiSelect: true` to allow multiple chips with the same key. This is a frontend-only toggle; backend support for multi-value filters is a separate concern.
- **Controlled component** — Accepts `value` (query string) and `onChange` props. Syncs internal chip state from the external value. Compatible with the existing `useFilter` hook.
- **300ms debounce** — API calls for global search and resource search are debounced. Local category matching is immediate.

## Initial Non-functional Requirements

- **WCAG 2.1 AA compliance** — `role="combobox"` on the input, single `role="listbox"` on the dropdown, `aria-activedescendant` for keyboard navigation, one focus indicator at a time, `role="presentation"` on non-interactive headings inside the listbox.
- **No new dependencies** — Uses existing project primitives (`Badge`, `Spinner`, `Avatar`, `cn()`, `lucide-react` icons). No Radix Popover — the dropdown is a plain absolutely-positioned `<div>`.
- **Performance** — Stale async responses are discarded via request ID counters. Debounce prevents excessive API calls during typing. `Promise.allSettled` ensures one slow category doesn't block others.
- **Responsive** — Chips wrap (not scroll) when the input overflows. The component grows vertically. Width is consumer-controlled via `className`.

## Eventual Requirements

- **Multi-value backend support** — See "Backend Work Required for Multi-Select Filters" section below for full details. The frontend `multiSelect` toggle is already implemented and ready; the backend needs migration from single-value to multi-value parsing and SQL `ANY()` clauses.
- **Preset filters** — The current `WorkspacesFilter` supports preset quick-filters ("My workspaces", "Running workspaces", "Failed workspaces", "Dormant workspaces"). These should be re-integrated as a companion feature — likely as a small button row or dropdown alongside `FilterSearchField`, not embedded inside it.
- **Adoption on other pages** — Audit, Connection Logs, and AI Bridge pages all use the current `<Filter>` + `<SelectFilter>` system and are candidates for migration.
- **Error display** — The current filter shows API validation errors inline. `FilterSearchField` does not yet handle error display; this should be added before full adoption.
- **Dormant/outdated/shared filters** — These are boolean filters (`dormant:true`, `outdated:true`, `shared:true`) that don't fit the category → options model. They need a design decision: category with `true`/`false` options, toggle chips, or something else.

---

# Scope

## In scope

- `FilterSearchField` component implementation (complete — prototype exists at `site/src/components/FilterSearchField/FilterSearchField.tsx`, ~1450 lines)
- Workspaces page integration (complete — prototype wired into `WorkspacesPageView.tsx`)
- Storybook stories for the component
- WCAG combobox compliance
- Replacement of `<WorkspacesFilter>` on the Workspaces page

## Not in scope

- Migration of Audit, Connection Log, or AI Bridge pages (eventual)
- Removal of the existing `<Filter>` / `<SelectFilter>` / `<UserFilter>` components (kept until all pages migrate)
- Backend changes to support multi-value filters
- Preset filter buttons (re-integration is a follow-up)
- Mobile-specific design considerations
- Unit/integration test coverage (follow-up)

---

# Phases

## Phase 1: Component + Workspaces page (current prototype)

Ship the `FilterSearchField` component and wire it into the Workspaces page, replacing `<WorkspacesFilter>`. This delivers the new UX for the highest-traffic filtered page.

Deliverables:
- `FilterSearchField.tsx` component
- `FilterSearchField.stories.tsx` with Empty, WithDefaultValue, MultipleFilters, WithFreeformText, AutoFocused stories
- `WorkspacesPageView.tsx` updated to use `FilterSearchField`
- Categories: Owner, Template, Status, Organization
- Resource search: live workspace preview in typeahead

## Phase 2: Backend multi-select + preset filters + error handling

Migrate workspace filter backend fields from single-value to multi-value parsing (see backend section below). Re-integrate preset quick-filters ("My workspaces", "Running workspaces", etc.) as a companion UI element. Add inline error display for invalid filter queries.

## Phase 3: Migrate remaining pages

Adopt `FilterSearchField` on Audit, Connection Logs, and AI Bridge pages. Each page defines its own `FilterCategory[]` array and optional `getSearchResults`. Remove per-page filter wrapper components as they're replaced.

## Phase 4: Deprecate old filter system

Once all pages are migrated, remove `Filter.tsx`, `SelectFilter.tsx`, `UserFilter.tsx`, and all page-specific menu/filter wrappers. Remove associated storybook stories and test helpers.

---

# Backend Work Required for Multi-Select Filters

The frontend `FilterCategory` type supports an optional `multiSelect: boolean` toggle. When enabled, users can add multiple chips for the same category (e.g., `status:running status:stopped`). **No workspace category currently uses this because the backend doesn't support it.** Here's what would need to change.

## Current state

Workspace filter parsing lives in `coderd/searchquery/search.go`, function `Workspaces()`. The relevant fields in `GetWorkspacesParams` are all single-value:

| Filter | Query param parser | Go struct field | SQL clause |
|---|---|---|---|
| Owner | `parser.String(values, "", "owner")` | `OwnerUsername string` | `WHERE owner_id = (SELECT id FROM users WHERE username = @owner_username)` |
| Template | `parser.String(values, "", "template")` | `TemplateName string` | `WHERE template_id = ANY(SELECT id FROM templates WHERE name = @template_name)` |
| Status | `httpapi.ParseCustom(... ParseEnum[WorkspaceStatus])` | `Status string` | Large `CASE/WHEN` block matching a single status |

When the frontend sends `owner:admin owner:member`, the Go `url.Values` parser creates `{"owner": ["admin", "member"]}`, but `parser.String` silently takes only the first value. The second value is dropped.

Note: `has-agent` already uses `parser.Strings` (multi-value), and the Users filter uses `ParseCustomList` for `status` and `login_type` — so the patterns exist in the codebase.

## Changes needed per field

### Owner (highest value for multi-select)

1. **`coderd/searchquery/search.go`**: Change `parser.String(values, "", "owner")` → `parser.Strings(values, []string{}, "owner")`
2. **`coderd/database/queries.sql.go`** / `GetWorkspacesParams`: Change `OwnerUsername string` → `OwnerUsernames []string`
3. **`coderd/database/queries/workspaces.sql`**: Change the `WHERE` clause from single-user subquery to `owner_id = ANY(SELECT id FROM users WHERE lower(username) = ANY(@owner_usernames))`
4. **`coderd/database/dbmem/dbmem.go`**: Update in-memory filter to iterate over the slice
5. Run `make gen` to regenerate

### Status

1. Change `ParseCustom` → `ParseCustomList` (pattern already exists for user status filtering)
2. Change `Status string` → `Statuses []database.WorkspaceStatus` in the params struct
3. Update the SQL `CASE/WHEN` block to check membership in an array rather than equality to a single value
4. This is the most complex change due to the large status `CASE/WHEN` in `workspaces.sql`

### Template

1. Change `parser.String` → `parser.Strings`
2. Change `TemplateName string` → `TemplateNames []string`
3. SQL already uses `ANY()` for `template_ids` — similar pattern for names

## Recommended migration order

Each field can be migrated independently:

1. **Status** — highest user value for OR filtering ("show me running OR stopped"), and `ParseCustomList` pattern already exists for user status
2. **Owner** — useful for admins viewing multiple users' workspaces
3. **Template** — useful but lower priority

Each migration requires: query parser change, struct field change, SQL change, in-memory DB change, `make gen`, and audit table update if the field touches audit logging.

This same pattern applies to any future category added to `FilterSearchField` on other pages (e.g., AI governance filters for provider, model, client). If the backend field uses `parser.String` today and the UI wants multi-select, the migration is the same: `parser.String` → `parser.Strings`, scalar struct field → slice, single-match SQL → `ANY()` clause.

## Frontend readiness

Once a backend field supports multi-value, the only frontend change is adding `multiSelect: true` to that category's definition in `WorkspacesPageView.tsx`. The `FilterSearchField` component already handles:

- Allowing multiple chips with the same key
- Replace-on-select for single-select, append-on-select for multi-select
- Correct serialization (`status:running status:stopped`)
