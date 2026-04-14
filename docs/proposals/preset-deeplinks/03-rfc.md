# RFC: Preset Deeplinks for Workspace Creation

**Discussion**: https://github.com/coder/coder/discussions/24029
**Problem Document**: [01-problem.md](./01-problem.md)
**PRD**: [02-prd.md](./02-prd.md)
**Date**: 2026-04-04

---

## Summary

Add a `preset` query parameter to workspace creation deeplink URLs. The
parameter accepts a preset name and resolves it against the target template
version's presets, applying the preset's parameters and ID to the workspace
creation form. The change is frontend-only — no new backend endpoints are
needed.

## Design

### URL Syntax

```
/templates/<org>/<template>/workspace?preset=<preset-name>
```

Composes with existing parameters:

```
?preset=ml-large&mode=auto&name=my-ws
?preset=ml-large&version=<uuid>
```

`preset` and `param.*` are **mutually exclusive** (see Key Design
Decisions below). If both are present, `preset` wins and `param.*`
values are ignored.

### Resolution Flow

```
URL parsed
  ├─ preset param present and non-empty?
  │   ├─ mode=duplicate? → ignore preset, proceed with duplicate flow
  │   ├─ YES → wait for templateVersionPresetsQuery to settle
  │   │         ├─ query succeeded?
  │   │         │   ├─ YES → find preset by exact Name match (case-sensitive)
  │   │         │   │         ├─ FOUND → apply preset params as autofill
  │   │         │   │         │           ├─ param.* also present?
  │   │         │   │         │           │   ├─ YES → ignore param.*, show notice, keep preset ID
  │   │         │   │         │           │   └─ NO  → proceed normally with preset ID
  │   │         │   │         │           └─ proceed (form or auto-create)
  │   │         │   │         └─ NOT FOUND → show error with version context, fallback mode=auto→form
  │   │         │   └─ NO (query error) → show query error, fallback mode=auto→form
  │   │         └─ loading → show loader, block auto-create
  │   └─ NO  → existing behavior (default preset if is_default, else no preset)
  └─ continue with mode handling
```

### Key Design Decisions

**1. `preset` and `param.*` are mutually exclusive.**

In the UI, selecting a preset **disables** (locks) its parameter inputs
and **hides** them by default. Users cannot override individual preset
parameter values — they must either use the preset as-is or select
"None." The deeplink behavior mirrors this:

- If `preset=<name>` is present and any `param.*` values are also
  present, `preset` takes precedence and all `param.*` values are
  **ignored**.
- An inline notice is shown: "Preset selected — `param.*` URL
  parameters have been ignored."
- `template_version_preset_id` is always preserved when `preset` is
  specified.

This avoids an inconsistency where deeplinks could produce workspace
configurations that are impossible to create through the UI.

**2. Preset params merge into `autofillParameters`.**

When a URL preset is resolved, its parameters are merged into the
`autofillParameters` array (with `source: "url"`) before being passed to
the view. This ensures:
- Preset values are included in the initial `sendInitialParameters`
  WebSocket call, avoiding a flash of default values.
- The data flow matches the existing `param.*` pipeline.

**3. Backend receives both preset ID and parameter values.**

When `template_version_preset_id` is sent in `CreateWorkspaceRequest`,
the backend already uses it for prebuild matching. The frontend also
sends `rich_parameter_values` populated from the preset's parameters (via
the autofill merge). This means the backend receives both the preset ID
and the actual parameter values — no backend changes are needed.

## Implementation Plan

The implementation is split into 3 PRs.

### PR 1: Fix presets query version scoping (prerequisite)

**File**: `site/src/pages/CreateWorkspacePage/CreateWorkspacePage.tsx`

The presets query (line 79-82) currently hardcodes `active_version_id`:

```ts
const templateVersionPresetsQuery = useQuery({
    ...templateVersionPresets(templateQuery.data?.active_version_id ?? ""),
    enabled: Boolean(templateQuery.data),
});
```

It must use `realizedVersionId` (currently at line 92-93), which accounts
for the `version` URL param. This requires reordering the declarations —
`realizedVersionId` must be moved above the presets query:

```ts
// Move this ABOVE the presets query:
const realizedVersionId =
    customVersionId ?? templateQuery.data?.active_version_id ?? "";

const templateVersionPresetsQuery = useQuery({
    ...templateVersionPresets(realizedVersionId),
    enabled: realizedVersionId !== "",
});
```

**Behavioral change note**: Users currently using `?version=<id>` see
presets from the active version (incorrect). After this fix, they see
presets from the pinned version (correct). If the pinned version has
different presets, the dropdown changes. This should be documented in the
PR description.

**Tests**: Story with `version=<non-active-id>` verifying presets match
the pinned version.

### PR 2: Core `preset` query parameter + form mode

#### 2a. Parse `preset` from URL

**File**: `site/src/pages/CreateWorkspacePage/CreateWorkspacePage.tsx`

Add at line ~61, alongside existing param extraction:

```ts
const customVersionId = searchParams.get("version") ?? undefined;
const defaultName = searchParams.get("name");
const disabledParams = searchParams.get("disable_params")?.split(",");
const presetName = searchParams.get("preset") || undefined;  // NEW — use || not ?? to treat "" as absent
const [mode, setMode] = useState(() => getWorkspaceMode(searchParams));
```

Note: `||` is used instead of `??` because `searchParams.get("preset")`
returns `""` for `?preset=` (empty value), and per PRD R6, empty preset
should be treated as absent.

#### 2b. Handle `mode=duplicate`

Skip preset resolution entirely when duplicating:

```ts
const effectivePresetName = mode === "duplicate" ? undefined : presetName;
```

Use `effectivePresetName` everywhere downstream instead of `presetName`.

#### 2c. Resolve preset

After `templateVersionPresetsQuery` resolves, find the matching preset:

```ts
const presets = templateVersionPresetsQuery.data ?? [];

const urlPresetResult = useMemo(() => {
    if (!effectivePresetName) return { preset: undefined, error: undefined };

    if (templateVersionPresetsQuery.isError) {
        return {
            preset: undefined,
            error: "Failed to load presets. Please try refreshing the page.",
        };
    }

    if (!templateVersionPresetsQuery.isSuccess) {
        return { preset: undefined, error: undefined }; // Still loading.
    }

    const found = presets.find((p) => p.Name === effectivePresetName);
    if (!found) {
        return {
            preset: undefined,
            error: `Preset "${effectivePresetName}" not found on template version `
                 + `${realizedVersionId}. Check that the preset name matches `
                 + "exactly (names are case-sensitive).",
        };
    }
    return { preset: found, error: undefined };
}, [effectivePresetName, presets, templateVersionPresetsQuery.isSuccess,
    templateVersionPresetsQuery.isError, realizedVersionId]);
```

#### 2d. Merge preset params into autofill

Merge preset parameters into `autofillParameters` so they flow through
the existing data pipeline (including `sendInitialParameters`):

```ts
const urlAutofillParameters = getAutofillParameters(searchParams);

const autofillParameters = useMemo(() => {
    if (!urlPresetResult.preset) return urlAutofillParameters;

    // When preset is specified, use only preset params.
    // param.* values are ignored (preset and param.* are mutually exclusive).
    const presetParams: AutofillBuildParameter[] =
        urlPresetResult.preset.Parameters.map((p) => ({
            name: p.Name,
            value: p.Value,
            source: "url" as const,
        }));

    return presetParams;
}, [urlPresetResult.preset, urlAutofillParameters]);

const hasIgnoredUrlParams = urlAutofillParameters.length > 0 && !!effectivePresetName;
```

#### 2e. Update view component props

**File**: `site/src/pages/CreateWorkspacePage/CreateWorkspacePageView.tsx`

Add to `CreateWorkspacePageViewProps`:

```ts
interface CreateWorkspacePageViewProps {
    // ...existing props
    urlPreset?: TypesGen.Preset;
    urlPresetError?: string;
    hasIgnoredUrlParams?: boolean;
}
```

#### 2f. Apply URL preset in view (override default preset effect)

Modify the existing default preset effect (lines 191-211):

```ts
useEffect(() => {
    const options = [
        { label: "None", value: "undefined", icon: "", description: "" },
        ...presets.map((preset) => ({
            label: preset.Default ? `${preset.Name} (Default)` : preset.Name,
            value: preset.ID,
            icon: preset.Icon,
            description: preset.Description,
        })),
    ];
    setPresetOptions(options);

    // URL preset takes precedence over default preset.
    if (urlPreset) {
        const idx = presets.findIndex((p) => p.ID === urlPreset.ID) + 1;
        setSelectedPresetIndex(idx);
        form.setFieldValue("template_version_preset_id", urlPreset.ID);
        return;
    }

    const defaultPreset = presets.find((p) => p.Default);
    if (defaultPreset) {
        const idx = presets.indexOf(defaultPreset) + 1;
        setSelectedPresetIndex(idx);
        form.setFieldValue("template_version_preset_id", defaultPreset.ID);
    } else {
        setSelectedPresetIndex(0);
        form.setFieldValue("template_version_preset_id", undefined);
    }
}, [presets, form.setFieldValue, urlPreset]);
```

The existing preset parameter application effect (lines 259-332) fires
when `selectedPresetIndex` changes, applying preset parameter values to
the form. By setting the correct index above, parameter application
happens automatically through the existing code path. React 18+ batches
the `setPresetOptions` and `setSelectedPresetIndex` state updates, so
the parameter application effect sees both in the same render cycle.

#### 2g. Display errors and notices

```tsx
{urlPresetError && (
    <Alert severity="warning" sx={{ mb: 2 }}>
        {urlPresetError}
    </Alert>
)}

{hasIgnoredUrlParams && urlPreset && (
    <Alert severity="info" sx={{ mb: 2 }}>
        Preset selected — <code>param.*</code> URL parameters have been
        ignored. Use either <code>preset</code> or <code>param.*</code>,
        not both.
    </Alert>
)}
```

#### 2h. Update `shouldShowLoader`

The loader should account for pending preset resolution:

```ts
// Add to shouldShowLoader conditions:
const shouldShowLoader =
    // ...existing conditions
    || (effectivePresetName && !templateVersionPresetsQuery.isSuccess
                            && !templateVersionPresetsQuery.isError);
```

**Tests** (Storybook stories for `CreateWorkspacePageView`):
- `preset=<valid-name>` → preset selected in dropdown, parameters applied
- `preset=<invalid-name>` → error shown with version ID, no preset selected
- `preset=<name>&param.cpu=16` → preset applied, `param.*` ignored, notice shown
- `preset=<name>` with `is_default` preset → URL preset wins
- `preset=` (empty string) → treated as absent, no error
- `preset=GPU` does not match preset named `gpu` (case sensitivity)
- `preset=ML%20Large` matches preset named `ML Large` (URL encoding)
- `preset=ML+Large` does NOT match `ML Large` (+ is literal, not space)
- `mode=duplicate&preset=X` → preset ignored
- `preset=X&disable_params=region` → region locked to preset value
- Presets query failure → error message shown

### PR 3: `mode=auto` + preset support

#### 3a. Gate auto-create on preset resolution

**File**: `site/src/pages/CreateWorkspacePage/CreateWorkspacePage.tsx`

Add preset resolution to the `autoCreateReady` condition:

```ts
const presetResolved =
    !effectivePresetName ||  // No preset requested — resolved trivially.
    (templateVersionPresetsQuery.isSuccess && urlPresetResult.preset !== undefined);

// In the existing autoCreateReady computation:
let autoCreateReady =
    mode === "auto" && autoCreateConsented && presetResolved /* NEW */ && ...;
```

Integrate the preset-not-found fallback into the existing mode-fallback
block (lines 252-275) rather than adding a separate `useEffect`:

```ts
// In the existing fallback block:
if (
    mode === "auto" && (
        externalAuthError ||
        (effectivePresetName && templateVersionPresetsQuery.isSuccess && !urlPresetResult.preset)
    )
) {
    setMode("form");
}
```

This avoids conflicts between separate effects that both call
`setMode("form")`.

#### 3b. Update consent dialog

**File**: `site/src/pages/CreateWorkspacePage/AutoCreateConsentDialog.tsx`

Add optional `presetName` prop:

```ts
interface AutoCreateConsentDialogProps {
    open: boolean;
    presetName?: string;              // NEW
    autofillParameters: AutofillBuildParameter[];
    onConfirm: () => void;
    onDeny: () => void;
}
```

Render preset name when present:

```tsx
{presetName && (
    <Box sx={{ mb: 2 }}>
        <strong>Preset:</strong> {presetName}
    </Box>
)}
```

#### 3c. Pass preset ID through auto-create mutation

**File**: `site/src/api/queries/workspaces.ts`

Add `templateVersionPresetId` to `AutoCreateWorkspaceOptions`:

```ts
type AutoCreateWorkspaceOptions = {
    organizationId: string;
    templateName: string;
    workspaceName: string;
    match: string | null;
    templateVersionId?: string;
    buildParameters?: WorkspaceBuildParameter[];
    templateVersionPresetId?: string;  // NEW
};
```

In the mutation function, pass it to `CreateWorkspaceRequest`:

```ts
const newWorkspace = await API.createWorkspace(organizationId, {
    name: workspaceName,
    template_version_id: templateVersionId,
    rich_parameter_values: buildParameters,
    template_version_preset_id: templateVersionPresetId,  // NEW
});
```

Note: When `match` finds an existing workspace (workspaces.ts line
160-167), the mutation returns it directly and `preset` is irrelevant.
No guard is needed — the early return naturally bypasses preset handling.

**File**: `site/src/pages/CreateWorkspacePage/CreateWorkspacePage.tsx`

In the auto-create call (~line 226), pass the preset ID:

```ts
const newWorkspace = await autoCreateWorkspaceMutation.mutateAsync({
    organizationId,
    templateName,
    buildParameters: autofillParameters,
    workspaceName: defaultName ?? generateWorkspaceName(),
    templateVersionId: realizedVersionId,
    match: searchParams.get("match"),
    templateVersionPresetId:                                // NEW
        effectivePresetName
            ? urlPresetResult.preset?.ID
            : undefined,
});
```

Note: `URLSearchParams.get()` returns the first value when multiple
`preset` params are present, satisfying PRD R6 "use the first value"
without additional code.

**Tests** (stories/integration tests for `CreateWorkspacePage`):
- `mode=auto&preset=<valid>` → consent dialog shows preset name,
  auto-creates with preset ID in request
- `mode=auto&preset=<invalid>` → falls back to form mode with error
- `mode=auto&preset=<valid>&param.cpu=16` → preset wins, `param.*`
  ignored, consent shows preset name, auto-creates with preset ID
- `mode=auto&preset=<valid>&match=<query>` → match takes precedence,
  navigates to existing workspace

## Files Changed

| File                                  | Change                                                    | PR      |
|---------------------------------------|-----------------------------------------------------------|---------|
| `CreateWorkspacePage.tsx`             | Reorder `realizedVersionId` above presets query           | PR 1    |
| `CreateWorkspacePage.tsx`             | Parse `preset`, resolve, merge autofill, gate auto-create | PR 2, 3 |
| `CreateWorkspacePageView.tsx`         | New props, URL preset effect, error/warning display       | PR 2    |
| `AutoCreateConsentDialog.tsx`         | Add `presetName` prop                                     | PR 3    |
| `workspaces.ts`                       | Add `templateVersionPresetId` to auto-create options      | PR 3    |
| `CreateWorkspacePageView.stories.tsx` | New stories for preset deeplink scenarios                 | PR 2    |
| `CreateWorkspacePage.test.tsx`        | Auto-create + preset integration tests                    | PR 3    |

## Files NOT Changed

| File                  | Reason                                                                                                                             |
|-----------------------|------------------------------------------------------------------------------------------------------------------------------------|
| Backend API endpoints | Existing `GET /templateversions/{id}/presets` is sufficient; `CreateWorkspaceRequest` already accepts `template_version_preset_id` |
| Database schema       | No model changes                                                                                                                   |
| `typesGenerated.ts`   | Already has `Preset`, `PresetParameter`, `CreateWorkspaceRequest.template_version_preset_id`                                       |
| Embed pages           | Deferred to fast-follow (PRD R7)                                                                                                   |

## Alternatives Considered

### 1. Backend preset resolution endpoint

Add a `GET /templateversions/{id}/presets?name=<name>` endpoint. Rejected
because the existing endpoint already returns all presets and the list is
small (typically 3-10 per version). Client-side filtering is simpler and
avoids a new API surface.

### 2. Preset ID in URL instead of name

Use `preset_id=<uuid>` instead of `preset=<name>`. Rejected because:
- UUIDs are opaque and not human-readable in URLs.
- Preset IDs change across template versions (new version = new preset
  rows).
- Names are the user-facing identifier and match the UI dropdown.

### 3. Case-insensitive matching

Match preset names case-insensitively. Deferred because:
- The database constraint is case-sensitive.
- Introducing case-insensitive matching in the frontend but not the
  backend creates inconsistency.
- A case-insensitive fallback could be added later as a UX enhancement.

### 4. Apply preset params only via effect (no autofill merge)

Apply preset parameters only through the existing `useEffect` that fires
when `selectedPresetIndex` changes. Rejected because:
- Preset values would not be included in `sendInitialParameters`, causing
  a flash of default values before the effect runs.
- The auto-create path would send empty `buildParameters` for
  `mode=auto&preset=X` (no `param.*`), relying entirely on the backend
  to resolve from preset ID.
- Merging into `autofillParameters` keeps all parameters flowing through
  a single pipeline.

### 5. Allow `param.*` overrides on top of presets

Allow `param.*` URL values to override individual preset parameter values,
clearing the preset ID from submission. Rejected because:
- The UI does not allow this — when a preset is selected, its parameters
  are disabled (read-only) and hidden by default.
- Allowing overrides in deeplinks but not in the UI creates an
  inconsistency between the two creation paths.
- Clearing the preset ID on override silently disables prebuild matching,
  which is a confusing failure mode.
- Keeping `preset` and `param.*` mutually exclusive is simpler to
  implement, test, and explain.

## Rollout

This is a frontend-only change with no feature flag required. The `preset`
query parameter is additive — existing URLs without it work unchanged.

**Rollout sequence**:
1. **PR 1**: Fix presets query version scoping. Land independently. Note
   behavioral change for `?version=<id>` users in PR description.
2. **PR 2**: Core `preset` param + form mode. Covers R1, R2, R4, R5, R6,
   R8, R9.
3. **PR 3**: `mode=auto` + preset support. Covers R3, auto-create
   mutation, consent dialog.
4. **Fast-follow**: Embed page preset support (separate PR, PRD R7).
5. **Fast-follow**: Documentation for stable preset naming guidance.

## Open Questions

1. **Telemetry**: Should we instrument the create workspace path to log
   whether `template_version_preset_id` was set from a URL param vs. UI
   selection? Low-effort but requires a product decision on what to track.

2. **URL persistence after dropdown change**: If a user navigates to
   `?preset=X`, then manually changes the preset dropdown to Y, the URL
   still shows `preset=X`. Refreshing or hitting back re-applies X. This
   is standard SPA behavior. Clearing the URL param on dropdown change is
   possible but adds complexity. Recommend accepting this as-is for MVP.

3. **WebSocket settlement**: After preset parameters are applied, the
   dynamic parameters WebSocket may return modified values (server-side
   validation). Should `autoCreateReady` wait for the WebSocket response
   to settle? The existing `mode=auto` flow already has this issue with
   `param.*` — it doesn't wait for WebSocket validation. Recommend
   maintaining parity: don't add a new wait.
