# Problem: Preset Deeplinks for Workspace Creation

**Discussion**: https://github.com/coder/coder/discussions/24029
**Author**: rodmk
**Date**: 2026-04-04

---

## Background

Coder supports "Open in Coder" deeplinks that navigate users directly to the workspace creation page with pre-filled parameters. These URLs follow the pattern:

```
https://coder.example.com/templates/<org>/<template>/workspace?mode=auto&param.region=us-east&param.cpu=4&param.memory=8
```

The deeplink system supports several query parameters:

| Parameter | Purpose |
|---|---|
| `param.<name>=<value>` | Pre-fill individual template parameters |
| `mode=auto\|duplicate` | Control creation behavior (automatic creation or workspace duplication; defaults to interactive form) |
| `name=<workspace-name>` | Pre-fill the workspace name |
| `version=<id>` | Target a specific template version |
| `disable_params=<csv>` | Make specific parameters read-only |
| `match=<query>` | Find an existing workspace instead of creating (used with `mode=auto`) |

Separately, Coder has a **presets** system that bundles a named set of parameter values together. Presets are defined in templates and are selectable via a dropdown in the workspace creation UI. They allow template authors to define curated configurations (e.g., "Small - 2 CPU / 4 GB", "Large - 8 CPU / 32 GB") that users can choose instead of manually configuring each parameter.

Presets are scoped to a template version and have a database uniqueness constraint on `(name, template_version_id)`, meaning each preset name is unique within a given template version.

## User Personas

Three distinct personas experience this problem differently:

- **Template authors** — define presets, construct deeplinks, and embed them in documentation. They feel the maintenance burden most directly when preset definitions change but deeplinks lag behind.
- **Platform engineers / DevOps** — embed deeplinks in CI pipelines, internal developer portals, onboarding docs, and IDP dashboards. They care about URL stability, compactness, and correctness at scale across many templates.
- **Developers (end users)** — click the links. They do not construct URLs, but they experience the consequences when a deeplink produces a misconfigured workspace because its hardcoded parameter values have drifted from the current preset definition.

## Problem Statement

There is no way to reference a preset by name in a deeplink URL. To share a link that creates a workspace with a specific preset's configuration, users must manually encode every individual parameter value into the URL using `param.*` query parameters.

This creates four concrete problems:

### 1. URL Maintenance Burden

When a template author updates a preset (e.g., changes the default region, bumps the CPU count, adds a new parameter), every deeplink URL that was manually constructed to mirror that preset must also be updated. There is no single source of truth — the preset definition lives in the template, but its values are duplicated across potentially many deeplink URLs embedded in documentation, READMEs, CI configs, and internal wikis.

This problem compounds with template version pinning. Presets are scoped to template versions. If a deeplink omits the `version` parameter, the parameters resolve against the active template version — which may have different parameter names or values than when the deeplink was constructed. If the deeplink pins a `version`, the parameters are frozen to that version's schema. Either way, manual parameter encoding creates a maintenance liability.

### 2. URL Complexity

A preset with 5-10 parameters produces an unwieldy URL:

```
https://coder.example.com/templates/devops/k8s-workspace/workspace?mode=auto&param.region=us-east-1&param.cpu=8&param.memory=32&param.disk=100&param.gpu_type=nvidia-t4&param.gpu_count=1&param.image=ubuntu-22.04-cuda&param.namespace=ml-team&param.priority_class=high
```

Such URLs are error-prone to construct, difficult to read, and impractical to embed in places with character limits (Slack messages, GitHub badge URLs, etc.). A name-based reference to the preset would reduce this to a single query parameter.

### 3. Semantic Disconnect

Presets exist to give meaningful names to parameter bundles. When a deeplink encodes the raw parameters instead of referencing the preset, the intent is lost. A reviewer of a README cannot tell at a glance that `param.cpu=8&param.memory=32&...` corresponds to the "ML Large" preset — they see an opaque list of key-value pairs.

### 4. Unreliable Prebuild Matching

The backend's prebuild system uses presets to determine which workspaces to pre-provision. Presets have `desired_instances` and scheduling configuration. When `mode=auto` creates a workspace using individual `param.*` values, the backend must heuristically match those values against a preset to claim a prebuilt workspace. If a single parameter is slightly off or a new parameter was added to the preset but not the deeplink, the match fails and the user loses the prebuild speed benefit — even though they intended to use that preset. A direct preset reference would make prebuild matching deterministic rather than heuristic.

## Edge Cases in the Problem Space

The following interactions represent ambiguities that any solution must address. They are stated here as open questions, not design decisions:

- **`preset` + `param.*` interaction**: The frontend disables (locks) parameters controlled by a preset. If a deeplink specifies both a preset name and `param.*` values for the same parameters, the deeplink must mirror the UI behavior — preset values take precedence and `param.*` values for preset-controlled parameters are ignored. `param.*` values for non-preset parameters are still applicable, since those remain editable in the UI.
- **`preset` + `version` interaction**: Presets are scoped to a template version. If a deeplink provides a preset name without a `version`, it must resolve against the active version. If the active version is updated and the preset name is removed or renamed, the deeplink breaks silently.
- **Default presets**: The data model supports `is_default` presets that auto-select in the UI. How does a deeplink-provided preset interact with the default? Does omitting a `preset` param still apply the default?
- **Preset name encoding**: Preset names are free-form text in the database (e.g., "ML Large — 8 CPU / 32 GB"). URL query parameters require percent-encoding for spaces and special characters. Whether the solution should require URL-safe names or handle encoding transparently is a design question.
- **Case sensitivity**: The database uniqueness constraint is case-sensitive by default in PostgreSQL. Whether preset name lookup from a URL should be case-sensitive or case-insensitive affects usability.
- **Preset not found**: What happens when a `preset` parameter references a name that doesn't exist on the resolved template version? Silent fallback to no preset, a user-visible error, or blocking workspace creation are all options.
- **Auto-create path gap**: The `autoCreateWorkspace` mutation currently sends only `rich_parameter_values` and never a `template_version_preset_id`. Even with a `preset` URL param, the auto-create codepath would need to be updated to either resolve the preset client-side or pass the preset ID to the backend.

## Impact

| Workflow | Severity | Frequency | Impact |
|---|---|---|---|
| **README "Open in Coder" badges** | High | Every template with presets | URLs break when preset changes; manual sync required |
| **CI/CD pipeline links** | High | Every CI-integrated deployment | Hardcoded param values drift from preset definitions |
| **`mode=auto` + prebuilds** | High | Every auto-create with prebuilds | Heuristic preset matching fails on parameter drift |
| **Internal documentation** | Medium | Ongoing maintenance | Long URLs are unreadable and hard to maintain |
| **Template embed page** | Medium | Template configuration | Cannot generate a compact deeplink for a preset config |
| **Self-service onboarding** | High | Enterprise onboarding flows | Broken deeplinks delay developer onboarding |

## Business Impact

- **Enterprise scale**: Large deployments with dozens of templates and hundreds of deeplinks across documentation systems amplify the maintenance burden. A single preset update can invalidate links across multiple systems.
- **Onboarding reliability**: Many enterprise customers use "Open in Coder" badges as the primary developer onboarding path. Drifted deeplinks produce misconfigured workspaces, creating support tickets and eroding trust.
- **Prebuild ROI**: Organizations invest in prebuilds to reduce workspace start times. When deeplinks fail to match prebuilds due to parameter drift, the prebuild investment is wasted.

## Scope Boundary

This document focuses on the deeplink URL problem. It does not cover:

- Changes to preset creation, storage, or the Terraform provider
- Preset management UI beyond what's needed for deeplink generation
- Prebuild scheduling or provisioning logic
- Backend preset matching heuristics (beyond identifying the gap)
