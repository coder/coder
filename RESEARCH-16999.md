# Research: Field-level drift logging and destructive change protection

**Issue:** [#16999](https://github.com/coder/coder/issues/16999)
**Branch:** [`fix/log-drift-all-builds`](https://github.com/coder/coder/compare/fix/log-drift-all-builds)
**Commit:** [`8eac0ba`](https://github.com/coder/coder/commit/8eac0ba7314351a2d02f9d301c6dc98eda40a64e)

## Problem

Coder's terraform build logs don't show which fields changed or drifted.
When a workspace update destroys a resource (e.g. `azurerm_managed_disk`),
the admin sees "will be replaced" but not *why*. A Wayve customer has lost
data multiple times because of this.

## Root cause

`terraform plan -json` (what Coder uses) has `Before`/`After` blobs per
resource but Coder never diffs them. The only human-readable diff comes from
`terraform show <planfile>` (non-JSON), which dumps full HCL for every
resource, far too verbose for build logs.

## Prior work in-repo

| PR | Status | What it did | Why it's not enough |
|----|--------|-------------|---------------------|
| [#17571](https://github.com/coder/coder/pull/17571) | Merged | Added `logDrift()`/`showPlan()` infra to run `terraform show` and stream to build logs | Gated to **prebuild claims only** (`isPrebuildClaimAttempt` in `executor.go`) |
| [#18355](https://github.com/coder/coder/pull/18355) | Closed | Tried to remove the gate | Blocked: (1) WARN level too noisy for non-prebuild, (2) `provisioner_job_logs` table volume from full `terraform show` output |

`coder/preview` is not relevant here. It's a static HCL analysis engine for
parameter preview; it doesn't run terraform plan/apply.

## Prototype: what the branch does

Two independent features, 4 files changed (+404 lines).

### Feature 1: Compact field-level diffs in build logs

**File:** [`resource_change_summary.go`](https://github.com/coder/coder/blob/fix/log-drift-all-builds/provisioner/terraform/resource_change_summary.go) (233 lines, new)

Extracts `Before`/`After` JSON from `plan.ResourceDrift` and
`plan.ResourceChanges`, diffs each field, and logs compact lines:

```
WARN: Drift detected: azurerm_managed_disk.data
  ~ disk_size_gb: 128 → 256

INFO: Update: docker_container.workspace
  ~ image: ubuntu:22.04 → fedora:40

WARN: Destroy: docker_volume.home (delete and re-create)
  ~ name: "home-v1" → "home-v2" (forces replacement)
```

Key design choices:

- Filters zero/null noise (`isZeroish()`), skips complex nested values
  (`isComplex()`) to keep output minimal.
- Drift logged at WARN, normal changes at INFO, destroys/replacements at
  WARN. Addresses the review feedback from #18355 about log levels.
- Much smaller output than `terraform show`, addressing the table volume
  concern.

### Feature 2: Destructive change blocker with confirmation dialog

**File:** [`destructive_check.go`](https://github.com/coder/coder/blob/fix/log-drift-all-builds/provisioner/terraform/destructive_check.go) (96 lines, new)

Checks if any resource in a hardcoded list of persistent types would be
destroyed or replaced:

- `docker_volume`
- `azurerm_managed_disk`
- `aws_ebs_volume`
- `google_compute_disk`
- `kubernetes_persistent_volume_claim`

If so, the build fails with a `CODER_BLOCK_DESTROY:` sentinel in
`job.error`.

**File:** [`WorkspaceReadyPage.tsx`](https://github.com/coder/coder/blob/fix/log-drift-all-builds/site/src/pages/WorkspacePage/WorkspaceReadyPage.tsx) (+51 lines, modified)

Detects the sentinel and **auto-opens** a confirmation dialog (no need to
click Retry first). "Continue anyway" retries the build with a
`__confirm_persistent_resource_destruction__=true` bypass parameter.

### Integration

**File:** [`executor.go`](https://github.com/coder/coder/blob/fix/log-drift-all-builds/provisioner/terraform/executor.go) (+31/-7 lines, modified)

- `logResourceChangeSummary(plan, logr)` called after every plan.
- Destructive check runs between plan and apply (skipped for destroy
  transitions or when bypass param is present).
- Existing prebuild `logDrift()` path from #17571 is unchanged.

## Complexity assessment

| Area | Complexity | Notes |
|------|-----------|-------|
| `resource_change_summary.go` | Medium | JSON diffing with edge cases (nulls, complex values, type coercion). Needs tests. |
| `destructive_check.go` | Low | Simple type check + parameter lookup. Needs tests. |
| `executor.go` changes | Low | Wiring only, 31 new lines. |
| `WorkspaceReadyPage.tsx` | Low | Uses existing `ConfirmDialog`, 51 new lines. Needs Storybook story. |
| **Total** | **Medium** | 404 new lines across 4 files. No DB changes, no new API endpoints. |

For production, the main complexity increase would be:

- Tests for both new Go files.
- A template-level opt-in/opt-out mechanism instead of the hardcoded
  blocklist.
- A proper API for the bypass (instead of a magic template parameter).
- Storybook story for the dialog state.

## What this does NOT do

- No changes to `coder/preview`.
- No changes to the existing prebuild `logDrift()` path.
- No database schema changes.
- No new API endpoints.

## Open design questions

1. **Blocklist vs template metadata.** The hardcoded persistent resource
   list doesn't scale. Options: a `coder_persistent_resource` data source,
   a template-level setting, or relying on Terraform's `prevent_destroy`
   lifecycle and reading it from plan JSON.

2. **Log volume.** The compact diff is much smaller than `terraform show`
   but still adds rows to `provisioner_job_logs`. Is this acceptable at
   scale?

3. **Bypass mechanism.** The magic template parameter works for a prototype
   but production needs a first-class "confirm destructive changes" API,
   likely a new build status or a separate confirmation endpoint.

4. **Scope of blocking.** Currently blocks any destroy/replace of listed
   types. Should it only block when the resource has data? Should it be
   opt-in per template?

## Demo templates

Two templates were created for testing (not committed):

- **`drift-demo`**: `docker_volume` with name keyed on a parameter.
  Changing the parameter forces volume replacement, triggers the blocker,
  and shows the confirmation dialog.
- **`drift-safe`**: Stable `docker_volume`, changing `base_image` parameter
  replaces the container but preserves the volume. Shows compact
  drift/change logs without triggering the blocker.
