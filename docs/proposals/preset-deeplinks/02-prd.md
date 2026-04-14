# PRD: Preset Deeplinks for Workspace Creation

**Discussion**: https://github.com/coder/coder/discussions/24029
**Problem Document**: [01-problem.md](./01-problem.md)
**Date**: 2026-04-04

---

## Overview

Add support for a `preset` query parameter in workspace creation deeplink URLs, allowing users to reference a template preset by name instead of manually encoding every parameter value.

**Target URL**:

```text
https://coder.example.com/templates/<org>/<template>/workspace?preset=<preset-name>
```

## Goals

1. Allow deeplink URLs to reference a preset by name.
2. Compose with existing query parameters (`mode`, `name`, `version`, `disable_params`).
3. `preset` and `param.*` are mutually exclusive, matching the UI where preset parameters are disabled when a preset is selected.
4. Work with `mode=auto` to enable zero-click preset-based workspace creation.
5. Pass `template_version_preset_id` to the backend when a preset is selected via URL, enabling deterministic prebuild matching.

## Non-Goals

- Changes to the preset data model (schema, Terraform provider, storage).
- New backend API endpoints (the existing `GET /templateversions/{id}/presets` endpoint already returns all data needed).
- Preset management UI changes beyond the embed page.
- Enforcing URL-safe preset naming conventions (handled by standard URL encoding; guidance provided in docs).
- Embed page updates — deferred to a fast-follow (see R7).

## User Stories

### Template Author

> As a template author, I want to embed a deeplink in my README that references
> my "GPU Large" preset by name, so that when I update the preset's parameters
> in the template, all existing deeplinks automatically pick up the new values.

### Platform Engineer

> As a platform engineer, I want to configure CI pipelines with compact
> preset-based deeplinks, so that our workspace creation URLs are short,
> readable, and don't require updates when templates evolve.

### Developer

> As a developer, I want to click an "Open in Coder" badge and get a workspace
> with the correct preset configuration, including claiming a prebuild if one
> is available for that preset.

### DevOps (Auto-Create)

> As a DevOps engineer, I want `mode=auto&preset=my-preset` to automatically
> create a workspace with the named preset and deterministically match a
> prebuild, without requiring user interaction beyond the consent dialog.

## Requirements

### R1: `preset` Query Parameter

The workspace creation page (`/templates/<org>/<template>/workspace`) must accept an optional `preset` query parameter whose value is the preset name.

**Behavior**:

- When `preset=<name>` is present, the `CreateWorkspacePage` component resolves the preset by matching the URL value against the `Name` field of presets returned by `templateVersionPresetsQuery`.
- Resolution must wait for `templateVersionPresetsQuery` to settle before proceeding.
- The preset's parameters are applied as autofill values, identical to selecting the preset from the UI dropdown.
- The preset dropdown in the UI reflects the URL-selected preset.
- The `template_version_preset_id` is set on the form, so that workspace creation sends it to the backend.
- URL-based preset selection takes precedence over `useEffect`-based default preset selection. The implementation must ensure the URL preset is applied after the default preset effect runs.

**Prerequisite fix**: The presets query currently uses `templateQuery.data?.active_version_id` (line 80), ignoring the `version` URL param. It must be updated to use `realizedVersionId` so that `version=<id>` correctly scopes preset resolution.

### R2: `preset` and `param.*` Are Mutually Exclusive

`preset` and `param.*` cannot be combined in the same deeplink. This matches the frontend UI behavior: when a preset is selected, its parameters are **disabled** (read-only) and **hidden** by default. Users cannot override individual preset parameter values — they must either use the preset as-is or not use a preset at all.

**Behavior when both are present**:

- If `preset=<name>` and any `param.*` values are both present in the URL, `preset` takes precedence and all `param.*` values are **ignored**.
- The form displays an inline notice: "Preset selected — `param.*` URL parameters have been ignored. Use either `preset` or `param.*`, not both."
- `template_version_preset_id` is preserved on submission (the preset is not compromised).

**Rationale**: In the UI, selecting a preset disables its parameter inputs. Allowing `param.*` overrides in deeplinks but not in the UI would create an inconsistency between the two creation paths. Keeping them mutually exclusive is simpler, safer for prebuild matching, and avoids a class of subtle bugs where a deeplink silently produces a workspace that doesn't match any preset.

### R3: `mode=auto` Support

When `mode=auto` is combined with `preset=<name>`:

- `autoCreateReady` must also require `templateVersionPresetsQuery` to be settled and the preset to be successfully resolved before firing.
- The consent dialog displays the resolved preset name above the parameter list (add an optional `presetName` prop to `AutoCreateConsentDialog`).
- Auto-creation sends `template_version_preset_id` in the `CreateWorkspaceRequest`.
- The `autoCreateWorkspace` mutation (`AutoCreateWorkspaceOptions` type) is updated to accept an optional `templateVersionPresetId` field, and the mutation passes it through to `CreateWorkspaceRequest`.

### R4: Version Interaction

- If `version=<id>` is specified, the preset is resolved against that version.
- If `version` is omitted, the preset is resolved against the template's active version.
- If the named preset does not exist on the resolved version, an error message is displayed and the form falls back to no preset selected. In `mode=auto`, creation does not proceed; the mode falls back to `form`.
- The error message must include the template version context: "Preset '\<name\>' not found on template version \<version\>. Check that the preset name matches exactly (names are case-sensitive)."

### R5: Default Preset Interaction

- If `preset=<name>` is specified, it overrides any `is_default` preset.
- If `preset` is omitted and the template version has a default preset, the existing behavior (auto-select default) is preserved.
- No changes to default preset behavior are required.

### R6: Query Parameter Interactions

| Condition                            | Behavior                                                                                                                                                            |
|--------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Preset name not found**            | Show inline error with version context and case-sensitivity hint; fall back to no preset; `mode=auto` falls back to `form`                                          |
| **Preset name is empty (`preset=`)** | Ignored; treated as if `preset` param is absent                                                                                                                     |
| **Multiple `preset` params**         | Use the first value                                                                                                                                                 |
| **`preset` + `param.*`**             | `preset` takes precedence; all `param.*` values are ignored; show inline notice explaining mutual exclusivity                                                       |
| **`preset` + `disable_params`**      | Preset parameters are applied first; `disable_params` disables the named parameters (locking them to the preset's values)                                           |
| **`preset` + `match`**               | If `match` finds an existing workspace, the user is navigated to it and `preset` is ignored. If no match is found, `preset` applies to the create-new fallback path |
| **`preset` + `mode=duplicate`**      | `preset` is ignored; `mode=duplicate` copies parameters from the source workspace. The preset dropdown shows no selection                                           |
| **`preset` + `name`**                | Both apply independently; `preset` sets parameters, `name` sets the workspace name                                                                                  |

### R7: Embed Page Update (Deferred — Fast-Follow)

The "Open in Coder" embed page update is deferred to a follow-up PR. Rationale:

- The core value prop (preset deeplinks in READMEs, CI, docs) is achieved by R1-R6 alone — users can construct `?preset=<name>` URLs directly.
- Two embed page implementations exist (`TemplateEmbedPage.tsx` and `TemplateEmbedPageExperimental.tsx`), doubling the implementation surface.
- The experimental embed page uses a WebSocket-based dynamic parameter system with no preset awareness, making integration nontrivial.

When implemented, the embed page should:

- Show a preset selector.
- When a preset is selected, generate the URL with `preset=<name>` instead of individual `param.*` values.
- Show a warning for preset names that require heavy URL encoding.

### R8: Case Sensitivity

Preset name matching from the URL is **case-sensitive**, matching the database constraint. The URL value must exactly match the preset name as defined in the template.

If an exact match fails, the error message should include a case-sensitivity hint (see R4). Test coverage must include a case-sensitivity regression test to prevent future introduction of case-insensitive matching.

### R9: URL Encoding

Preset names with spaces or special characters are handled via standard URL percent-encoding. No restrictions are imposed on preset naming. The frontend's `URLSearchParams` API handles encoding/decoding transparently.

**Note**: Preset names containing `+` characters require explicit percent-encoding (`%2B`) since `+` is interpreted as a space in query strings. `URLSearchParams` handles this correctly.

## Success Metrics

| Metric                         | How to Measure                                                                                                                                                                       |
|--------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Adoption**                   | % of deeplink workspace creations using `preset=` vs. `param.*` after 30 days (requires telemetry on `CreateWorkspaceRequest.template_version_preset_id` presence + referrer/source) |
| **Prebuild match improvement** | Reduction in prebuild match failures for deeplink-originated workspace creations (instrument the create path)                                                                        |
| **URL compactness**            | Single `preset=` param replaces N `param.*` params (design validation, not runtime metric)                                                                                           |
| **Zero regressions**           | Existing `param.*`, `mode=auto`, and `disable_params` behavior unchanged (test coverage)                                                                                             |

## Known Risks

### Preset Name Instability (High)

Preset names are free-form text, not slugs, and are scoped to template versions. If a template author renames "ML Large" to "ML - Large" in a new version, every deeplink with `preset=ML%20Large` silently breaks. Mitigation:

- Documentation guidance recommending stable, slug-like preset names for deeplink use.
- Clear error messages when preset names are not found (R4).
- Future work: optional slug/alias field on presets for URL-stable references.

### Case Sensitivity Support Tickets (Medium)

Users constructing URLs by hand will type `preset=gpu-large` when the preset is `GPU-Large`. Mitigation: error messages include case-sensitivity hints (R4, R8).

### Forward Compatibility

The `preset` parameter means "by name." If a future change introduces ID-based lookup, it should use a distinct parameter name (e.g., `preset_id`).

## Out of Scope / Future Work

- **Preset name validation**: Enforcing slug-friendly preset names at template creation time.
- **Preset-aware `match` param**: Extending `match` to find existing workspaces by preset name.
- **CLI support**: `coder create --preset=<name>`.
- **Audit logging**: Tracking that a workspace was created via a preset deeplink vs. manual selection.
- **Case-insensitive fallback**: Attempting case-insensitive matching when exact match fails.
- **Embed page preset support**: Covered by deferred R7.
