# Terraform State Management

This document provides a technical reference for how Coder manages Terraform state across workspace lifecycle operations. Understanding this behavior is essential for template authors, platform administrators, and anyone debugging workspace provisioning issues.

## Overview

Coder orchestrates workspaces through Terraform, maintaining state in the database and passing it to the provisioner for each build operation. The state flows through several components:

1. **coderd** - The API server that handles workspace build requests
2. **wsbuilder** - The workspace builder that constructs provisioner jobs
3. **provisionerd** - The provisioner daemon that executes Terraform
4. **terraform-provider-coder** - The Terraform provider that exposes workspace metadata

## Workspace Transitions

Coder defines three core workspace transitions in [`codersdk/workspacebuilds.go`](https://github.com/coder/coder/blob/v2.29.1/codersdk/workspacebuilds.go#L14-L20):

| Transition | Terraform Operation | `start_count` Value | Description |
|------------|---------------------|---------------------|-------------|
| `start` | `terraform apply` | 1 | Creates/starts workspace resources |
| `stop` | `terraform apply` | 0 | Stops workspace resources, preserves state |
| `delete` | `terraform destroy` | N/A | Destroys workspace and all resources |

The `start_count` value is exposed to templates via the [`coder_workspace`](https://github.com/coder/terraform-provider-coder/blob/main/provider/workspace.go#L26-L29) data source, allowing conditional resource creation:

```hcl
resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  # ... resource configuration
}
```

## State Flow by Operation

### State Source Resolution

When a workspace build is initiated, the [`wsbuilder.getState()`](https://github.com/coder/coder/blob/v2.29.1/coderd/wsbuilder/wsbuilder.go#L744-L763) function determines which Terraform state to use:

```go
func (b *Builder) getState() ([]byte, error) {
    if b.state.orphan {
        return nil, nil  // Orphan deletes skip state
    }
    if b.state.explicit != nil {
        return *b.state.explicit, nil  // Explicitly provided state
    }
    // Default: use state from prior build
    bld, err := b.getLastBuild()
    // ...
    return bld.ProvisionerState, nil
}
```

### State Update on Completion

When a provisioner job completes successfully, the state is updated via [`completeWorkspaceBuildJob()`](https://github.com/coder/coder/blob/v2.29.1/coderd/provisionerdserver/provisionerdserver.go#L1903-L2000):

```go
err = db.UpdateWorkspaceBuildProvisionerStateByID(ctx, database.UpdateWorkspaceBuildProvisionerStateByIDParams{
    ID:               workspaceBuild.ID,
    ProvisionerState: jobType.WorkspaceBuild.State,
    UpdatedAt:        now,
})
```

### State Update on Failure

When a provisioner job fails, state is conditionally updated in [`FailJob()`](https://github.com/coder/coder/blob/v2.29.1/coderd/provisionerdserver/provisionerdserver.go#L1146-L1262):

```go
if jobType.WorkspaceBuild.State != nil {
    err = db.UpdateWorkspaceBuildProvisionerStateByID(ctx, database.UpdateWorkspaceBuildProvisionerStateByIDParams{
        ID:               input.WorkspaceBuildID,
        ProvisionerState: jobType.WorkspaceBuild.State,
    })
}
```

> **Important:** State is only updated on failure if Terraform provides a state. If the failure occurs before Terraform generates state, the previous build's state is preserved.

## Operations Reference Table

### API Operations

| Endpoint | Transition | Terraform Op | State Source | State Updated |
|----------|------------|--------------|--------------|---------------|
| `POST /workspaces` | start | `apply` | Empty (nil) | ✅ On completion |
| `POST /workspaces/{id}/builds` (start) | start | `apply` | Last build | ✅ On completion |
| `POST /workspaces/{id}/builds` (stop) | stop | `apply` | Last build | ✅ On completion |
| `POST /workspaces/{id}/builds` (delete) | delete | `destroy` | Last build | N/A (deleted) |
| `POST /workspaces/{id}/builds` (delete + orphan) | delete | None | Ignored | ❌ Intentional |
| `PATCH /workspacebuilds/{id}/cancel` | N/A | None | N/A | ⚠️ **Stale** |
| `PUT /workspacebuilds/{id}/state` | N/A | None | Explicit | ✅ Direct update |

### CLI Operations

| Command | API Calls | Terraform Op | State Source | State Updated |
|---------|-----------|--------------|--------------|---------------|
| `coder create` | POST /workspaces | `apply` | Empty | ✅ On completion |
| `coder start` | POST builds (start) | `apply` | Last build | ✅ On completion |
| `coder stop` | POST builds (stop) | `apply` | Last build | ✅ On completion |
| `coder delete` | POST builds (delete) | `destroy` | Last build | N/A |
| `coder delete --orphan` | POST builds (delete+orphan) | None | Ignored | ❌ Intentional |
| `coder restart` | POST builds (stop), POST builds (start) | `apply` x2 | Last build | ✅ On completion |
| `coder update` | POST builds (stop)*, POST builds (start) | `apply` x2 | Last build | ✅ On completion |
| `coder state push` | POST builds | `apply` | Explicit | ✅ On completion |
| `coder state push --no-build` | PUT state | None | Explicit | ✅ Direct update |

\* Stop only occurs if workspace is currently running

### Autobuild Operations

The [lifecycle executor](https://github.com/coder/coder/blob/v2.29.1/coderd/autobuild/lifecycle_executor.go) handles automated workspace transitions:

| Trigger | Transition | Terraform Op | State Source | State Updated |
|---------|------------|--------------|--------------|---------------|
| Autostart schedule | start | `apply` | Last build | ✅ On completion |
| Autostop (deadline/TTL) | stop | `apply` | Last build | ✅ On completion |
| Failure TTL | stop | `apply` | Last build | ✅ On completion |
| Dormancy (inactivity) | stop | `apply` | Last build | ✅ On completion |
| Auto-delete (dormant) | delete | `destroy` | Last build | N/A |

### Prebuild Operations

The [prebuilds reconciler](https://github.com/coder/coder/blob/v2.29.1/enterprise/coderd/prebuilds/reconcile.go) manages prebuilt workspace pools:

| Operation | Transition | Terraform Op | State Source | State Updated | Special Env Vars |
|-----------|------------|--------------|--------------|---------------|------------------|
| Create prebuild | start | `apply` | Empty | ✅ On completion | `CODER_WORKSPACE_IS_PREBUILD=true` |
| Claim prebuild | start | `apply` | Prebuild state | ✅ On completion | `CODER_WORKSPACE_IS_PREBUILD_CLAIM=true` |
| Delete prebuild | delete | `destroy` | Last build | N/A | - |
| Cancel pending + orphan | delete | None | Ignored | ❌ Intentional | - |

## The Stale State Condition

### Overview

When a workspace build is **canceled**, Terraform state may become **stale** — meaning the stored state no longer accurately reflects the actual infrastructure. This is a known condition that occurs by design.

### Technical Details

Build cancellation is handled in [`patchCancelWorkspaceBuild()`](https://github.com/coder/coder/blob/v2.29.1/coderd/workspacebuilds.go#L650-L770). The code includes this comment:

```go
// If the jobStatus is pending, we always allow cancellation regardless of
// the template setting as it's non-destructive to Terraform resources.
if jobStatus == database.ProvisionerJobStatusPending {
    return true, nil
}
```

When a job is canceled:

1. The job is marked as canceled via `UpdateProvisionerJobWithCancelByID`
2. **No call to `UpdateWorkspaceBuildProvisionerStateByID` is made**
3. The build's `ProvisionerState` remains unchanged from initialization

### Why This Is Intentional

The decision not to update state on cancellation is a **safety measure**:

1. **Unknown state**: When a running job is canceled, Terraform may be in any phase:
   - Planning (no changes made)
   - Early apply (partial changes)
   - Late apply (most changes complete)
   - Post-apply (changes complete, finalizing)

2. **State integrity**: The Terraform state at cancellation time is undefined. Writing potentially corrupt state could make the workspace unrecoverable.

3. **Conservative approach**: By preserving the last known good state, Coder ensures:
   - The next build starts from a known baseline
   - Terraform can detect and reconcile any drift
   - No data corruption from partial state writes

### Stale State Scenarios

| Cancel Timing | Terraform Phase | Actual State | Stored State | Result |
|---------------|-----------------|--------------|--------------|--------|
| Pending | Not started | No changes | Previous build | ✅ Correct |
| Early running | Planning | No changes | Previous build | ✅ Correct |
| Mid running | Applying | Partial | Previous build | ⚠️ Stale |
| Late running | Apply complete | Complete | Previous build | ⚠️ Stale |

### Symptoms of Stale State

When state becomes stale, you may observe:

- Terraform attempting to recreate existing resources
- "Resource already exists" errors
- Drift detection showing unexpected changes
- Resource replacement on subsequent builds
- Orphaned cloud resources not tracked by Coder

### Recovery Procedures

#### Option 1: Let Terraform Reconcile

In many cases, simply starting a new build allows Terraform to detect drift and reconcile:

```bash
coder start <workspace>
```

Terraform's refresh phase will detect existing resources and update the state accordingly.

#### Option 2: Manual State Push

For complex situations, manually update the state:

```bash
# Pull the current (stale) state
coder state pull <workspace> > state.json

# Edit state.json or obtain correct state from cloud provider
# ...

# Push corrected state without triggering a build
coder state push --no-build <workspace> state.json

# Or push and trigger a build to verify
coder state push <workspace> state.json
```

#### Option 3: Orphan Delete and Recreate

If state is irrecoverably corrupt:

```bash
# Delete workspace without destroying resources
coder delete --orphan <workspace>

# Manually clean up cloud resources
# ...

# Create fresh workspace
coder create <workspace> --template <template>
```

> **Warning:** Orphan delete leaves cloud resources running. Ensure you clean them up manually to avoid unexpected charges.

## Special Cases

### Empty State Handling

When destroying a workspace with no state, the provisioner exits early ([`provision.go`](https://github.com/coder/coder/blob/v2.29.1/provisioner/terraform/provision.go#L161-L165)):

```go
// If we're destroying, exit early if there's no state. This is necessary to
// avoid any cases where a workspace is "locked out" of terraform due to
// e.g. bad template param values and cannot be deleted.
if request.Metadata.GetWorkspaceTransition() == proto.WorkspaceTransition_DESTROY && len(request.GetState()) == 0 {
    sess.ProvisionLog(proto.LogLevel_INFO, "The terraform state does not exist, there is nothing to do")
    return &proto.PlanComplete{}
}
```

This prevents workspaces from becoming "locked out" due to template issues.

### Orphan Delete Mode

Orphan delete ([`wsbuilder.Orphan()`](https://github.com/coder/coder/blob/v2.29.1/coderd/wsbuilder/wsbuilder.go#L162-L166)) explicitly skips Terraform:

```go
func (b Builder) Orphan() Builder {
    b.state = stateTarget{orphan: true}
    return b
}
```

Use cases:
- Workspace stuck in broken state
- No provisioners available
- Template has been deleted
- State is corrupt beyond repair

### Prebuild State Isolation

Prebuilt workspaces use environment variables to control template behavior:

- `CODER_WORKSPACE_IS_PREBUILD=true` — Initial prebuild creation
- `CODER_WORKSPACE_IS_PREBUILD_CLAIM=true` — User claiming a prebuild

This allows templates to defer user-specific configuration until claim time. See the [terraform-provider-coder workspace data source](https://github.com/coder/terraform-provider-coder/blob/main/provider/workspace.go#L159-L206) for implementation details.

## Monitoring and Debugging

### Relevant Metrics

| Metric | Description |
|--------|-------------|
| `coderd_provisionerd_job_timings_seconds` | Duration of provisioner jobs |
| `coderd_provisionerd_jobs_current` | Currently running provisioner jobs |
| `coderd_workspace_builds_total` | Total workspace builds by status |

### Audit Log Fields

Workspace build events include:
- `transition` — The workspace transition type
- `provisioner_state` — Not tracked (sensitive)
- `template_version_id` — Tracked for version changes

### Debug Logging

Enable provisioner debug logging for detailed Terraform output:

```bash
coder start <workspace> --log-level debug
```

Or via API:

```json
{
  "transition": "start",
  "log_level": "debug"
}
```

## Related Documentation

- [Workspace Lifecycle](../../user-guides/workspace-lifecycle.md)
- [Workspace Management](../../user-guides/workspace-management.md)
- [Provisioner Jobs](../../admin/provisioners/manage-provisioner-jobs.md)
- [Prebuilt Workspaces](../../admin/templates/extending-templates/prebuilt-workspaces.md)

## Source Code References

| Component | File | Description |
|-----------|------|-------------|
| Workspace Builder | [`coderd/wsbuilder/wsbuilder.go`](https://github.com/coder/coder/blob/v2.29.1/coderd/wsbuilder/wsbuilder.go) | Build orchestration and state resolution |
| Provisioner Server | [`coderd/provisionerdserver/provisionerdserver.go`](https://github.com/coder/coder/blob/v2.29.1/coderd/provisionerdserver/provisionerdserver.go) | Job completion and state persistence |
| Terraform Provisioner | [`provisioner/terraform/provision.go`](https://github.com/coder/coder/blob/v2.29.1/provisioner/terraform/provision.go) | Terraform execution and state handling |
| Workspace Builds API | [`coderd/workspacebuilds.go`](https://github.com/coder/coder/blob/v2.29.1/coderd/workspacebuilds.go) | API handlers for build operations |
| Lifecycle Executor | [`coderd/autobuild/lifecycle_executor.go`](https://github.com/coder/coder/blob/v2.29.1/coderd/autobuild/lifecycle_executor.go) | Automated workspace transitions |
| Prebuilds Reconciler | [`enterprise/coderd/prebuilds/reconcile.go`](https://github.com/coder/coder/blob/v2.29.1/enterprise/coderd/prebuilds/reconcile.go) | Prebuild pool management |
| Provider Workspace DS | [`provider/workspace.go`](https://github.com/coder/terraform-provider-coder/blob/main/provider/workspace.go) | Terraform provider workspace data source |
