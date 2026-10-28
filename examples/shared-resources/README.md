# Shared and per-workspace resources

As a template administrator, use this minimal example to publish a Foundation and consume its outputs in a workspace.
A **Foundation** owns shared infrastructure, with its own organization-scoped ID, name, Terraform state, revisions, provisioner routing, and run history.
A template defines each workspace and refers to a Foundation through an association.
Manage Foundations from **Foundations** in the top-level navigation, between **Templates** and **Agents**.
The list follows the template list layout, with standalone creation, editing, and publication.
The template page only manages the Foundation association; its file editor has no **Shared / Per-workspace** toggle.

- **Foundation:** shared Terraform configuration, with independent plan, apply, and destroy runs.
- **Template:** resources provisioned separately through normal workspace builds.

Initially, a template has at most one Foundation, and a Foundation serves at most one template in the same organization.
Creating or binding a Foundation does not execute Terraform.
The association has no detach or rebind operation: replacement requires deletion and creation/association of a new Foundation.
Destroy provisioned infrastructure before deleting the Foundation.
An unconfigured Foundation with no retained state and no active run can be deleted directly.
There is no blanket startup gate or minimum workspace-version floor.
Before the first Apply, outputs are empty; Terraform determines whether a workspace can proceed without them.
Starts pin available published outputs even during an active or failed Foundation run.
Stop and delete retain the snapshot and Foundation provenance associated with the retained workspace state, not merely the latest attempted build.
Refer to the [Foundation design and boundaries](DESIGN.md) for the accepted design and implementation limitations.

This example uses only `terraform_data`, so it creates no cluster or cloud resources.
For the Serpent demo with a shared Kubernetes namespace and network policy, refer to [Kubernetes provisioning scopes](../kubernetes-shared-resources/README.md).
Both examples have two separate Terraform roots.
Do not combine or apply them in the same directory/state.
The shared source directory remains named `template/` for continuity, not because the template owns its state.

## Try it

Run the development server from the repository root with the `template-resources`
experiment and the [companion provider override](#companion-terraform-provider):

```bash
env -i PATH="$PATH" HOME="$HOME" DEVELOP_IN_CODER=1 \
  AWS_CONFIG_FILE=/dev/null AWS_SHARED_CREDENTIALS_FILE=/dev/null \
  AWS_EC2_METADATA_DISABLED=true \
  TF_CLI_CONFIG_FILE=/absolute/path/to/coder-provider.tfrc \
  ./scripts/develop.sh -- --experiments template-resources
```

This minimal example uses no cluster or cloud credentials. For the Kubernetes
example, use its [development command](../kubernetes-shared-resources/README.md#start-the-development-server)
instead.

The experiment retains the technical identifier `template-resources`.
The API uses organization-scoped Foundations rather than template-owned resource routes.
Both Foundation and consuming workspace/validation provisioner pools require provisionerd API v1.25 or later within major version 1 and the `coder.template_resources=1` capability.
Older workers can discard output sensitivity metadata; upgrade both pools before use.

1. Create a normal template with a bootstrap workspace configuration.

   For this example, `resource "terraform_data" "bootstrap" { input = "bootstrap" }` is enough.

2. On the template's **Foundation** page, select **Create Foundation** or choose an unbound Foundation from the same organization.

   **Create Foundation** navigates to `/foundations/new?organization=<id>` using the template's organization ID.
   Creation does not automatically attach the Foundation. Return to the template and select it explicitly.

3. Prepare `workspace/main.tf` as `main.tf` in the workspace template editor and select **Build**.
   The inactive version retains its source even if import fails on missing Foundation outputs.
4. Open the Foundation, choose **Edit files**, and add `template/main.tf` as `main.tf`.
5. Select **Save files** to save an immutable revision without running Terraform.
6. Select **Plan** to preview the saved revision without applying infrastructure.
7. Select **Apply** and confirm the live infrastructure change.
   Apply runs a fresh plan, not the earlier preview.
8. After Apply succeeds, reopen the failed workspace version and select **Build** again.
   Unchanged source is allowed: this creates a fresh version/import, not a resume of the failed version.
   After successful import, select **Promote**.
9. Create a workspace from the promoted version.

This source-preparation flow was checked read-only; it is not an end-to-end validation result.
There is no first-class awaiting-outputs state or automatic import retry.
Alternatively, upload the consuming workspace source after Foundation Apply.

The [API workflow](#foundation-api-workflow) below also creates and binds a Foundation explicitly.

The workspace reads the Foundation's published outputs and metadata through one data source.
Each build receives a fixed snapshot rather than fetching changing values while Terraform runs.
`outputs` contains only public root outputs.
`sensitive_outputs` contains only sensitive root outputs and is always marked sensitive, even when empty.
Both objects preserve exact Terraform types from the same pinned snapshot, including collection kinds and nested values, without requiring `jsondecode`:

```tf
data "coder_foundation" "current" {}

locals {
  shared           = data.coder_foundation.current.outputs
  shared_sensitive = data.coder_foundation.current.sensitive_outputs
}
```

`data.coder_foundation.current.id` identifies the contextual Foundation, not an arbitrary lookup target.
The same data source exposes `name`, `template_ids`, and `snapshot_id`, the producing Foundation run UUID, not a source revision selector.
Foundation metadata is also available in the Foundation's own Plan, Apply, and Destroy contexts, before any successful Apply.
An unbound Foundation has no attached template IDs.
The minimal workspace stores metadata and values in the existing `terraform_data.workspace_settings` resource.
A Foundation ID is not a template ID, workspace ID, or revision ID.

The sensitive root `private_setting` is available only through `local.shared_sensitive.private_setting`, not through `local.shared`.
Ordinary management responses omit sensitive values while retaining per-output type and sensitivity metadata.
On the Foundation detail page, the **Outputs** table lists each output's name, type, and sensitivity.
It displays actual public values and a fixed `********` mask for sensitive values, not the secrets omitted by the API.
The combined `workspace_settings` root output is sensitive because it contains `private_setting`.
The separate `namespace` output reads `local.shared.namespace` directly and remains public.
The `terraform_data.public_setting` resource uses `for_each = local.shared.public_set` without `nonsensitive(...)`, even though the same snapshot contains a secret.

Keep public identity keys in separate root outputs from secret values.
A sensitive root's public-looking children remain inside `sensitive_outputs`.
Changing a root's sensitivity moves its key between `outputs` and `sensitive_outputs`, requiring an update to consuming references.
Do not merge the objects and remove their sensitivity marks.
Terraform state and logs still require trusted operator access.
[`sensitive`](https://developer.hashicorp.com/terraform/language/manage-sensitive-data) is not encryption and cannot prevent a trusted Terraform author from accessing a value.

Change the greeting in the Foundation configuration and Apply again. Existing
workspace builds are not changed. The next workspace start pins the published
snapshot. Rollout is operator-owned: there is no automatic rebuild or version floor.
Rolling back workspace configuration does not roll back shared resources, and a
Foundation-only rollback is not guaranteed safe. Impact warnings are advisory,
not compatibility or network-enforcement guarantees.
Delete the test workspace before explicitly destroying shared resources.

## Foundation API workflow

From the repository root, set `CODER_URL`, `CODER_SESSION_TOKEN`, `ORG_ID`, and `TEMPLATE_ID` for your development instance and bootstrap template.
Keep credentials outside the repository.
`ORG_ID` must be the template's organization UUID, not a template UUID.

1. Create an unbound Foundation:

   ```bash
   FOUNDATION_ID=$(curl --fail --silent --show-error \
     -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
     -H 'Content-Type: application/json' \
     -d '{"name":"example-foundation","provisioner_tags":{}}' \
     "$CODER_URL/api/v2/organizations/$ORG_ID/foundations" | jq -er '.id')
   FOUNDATION_URL="$CODER_URL/api/v2/organizations/$ORG_ID/foundations/$FOUNDATION_ID"
   ```

2. Bind the Foundation to your template:

   ```bash
   jq -n --arg id "$FOUNDATION_ID" '{foundation_id:$id}' |
     curl --fail --silent --show-error -X PUT \
       -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
       -H 'Content-Type: application/json' --data-binary @- \
       "$CODER_URL/api/v2/templates/$TEMPLATE_ID/foundation"
   ```

3. Save the minimal shared source as an immutable Foundation revision:

   ```bash
   REVISION_ID=$(jq -n --rawfile source examples/shared-resources/template/main.tf '{source:$source}' |
     curl --fail --silent --show-error \
       -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
       -H 'Content-Type: application/json' --data-binary @- \
       "$FOUNDATION_URL/versions" | jq -er '.id')
   ```

   For multi-file roots, upload a complete TAR archive through `POST /api/v2/files` instead.
   Pass its returned hash as `{"file_id":"<uploaded hash>"}` to the same `/versions` route.
   Do not send both `file_id` and `source`.

4. Queue a plan:

   ```bash
   RUN_ID=$(jq -n --arg revision "$REVISION_ID" '{revision_id:$revision,action:"plan"}' |
     curl --fail --silent --show-error \
       -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
       -H 'Content-Type: application/json' --data-binary @- \
       "$FOUNDATION_URL/runs" | jq -er '.id')
   ```

5. Inspect the run's status and persisted logs:

   ```bash
   curl --fail --silent --show-error \
     -H "Coder-Session-Token: $CODER_SESSION_TOKEN" "$FOUNDATION_URL"
   curl --fail --silent --show-error \
     -H "Coder-Session-Token: $CODER_SESSION_TOKEN" "$FOUNDATION_URL/runs/$RUN_ID/logs?after=0"
   ```

6. After the plan succeeds, publish intentionally by repeating step 4 with `action:"apply"`.

   Apply runs a fresh plan and can change live infrastructure.
   After Apply succeeds, rebuild any retained workspace version whose import failed on missing outputs, then Promote the new successful version.
   To request cancellation, send `PATCH` to `$FOUNDATION_URL/runs/$RUN_ID/cancel`.
   Cancellation does not prove that an acquired mutation made no changes.

### Independent settings and cleanup

`GET /api/v2/templates/$TEMPLATE_ID/foundation` returns the associated Foundation, or `null` for an unbound template.
Independent management does not require the workspace template version to validate.
The standalone UI uses `/organizations/{organization}/foundations/{foundation}`.
The API provides these independent operations:

| Method   | Route                                       | Purpose                                           |
|----------|---------------------------------------------|---------------------------------------------------|
| `GET`    | `/api/v2/organizations/$ORG_ID/foundations` | List accessible Foundations.                      |
| `GET`    | `$FOUNDATION_URL`                           | Read settings, revisions, runs, and outputs.      |
| `PATCH`  | `$FOUNDATION_URL`                           | Update name, routing, and destruction protection. |
| `DELETE` | `$FOUNDATION_URL`                           | Remove a Foundation after infrastructure cleanup. |

For example, this settings update does not execute Terraform:

```bash
curl --fail --silent --show-error -X PATCH \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"example-foundation","provisioner_tags":{}}' "$FOUNDATION_URL"
```

Foundation provisioner routing is independent of workspace routing.
The update replaces name and tags; queued jobs retain their selected routing.
The optional `prevent_destroy_with_workspaces` setting defaults to `true` on creation and is preserved when omitted from an update.
An authorized operator can explicitly set it to `false` to allow Destroy with consumers, accepting the risk.
With protection enabled, Destroy counts all consuming workspaces, including stopped, hidden, and historical-snapshot consumers.
This setting does not prevent an ordinary Apply from making destructive changes.

For this example, delete all consuming workspaces before intentionally destroying the Foundation infrastructure.
Queue `action:"destroy"` on `/runs` with the state-producing revision, not an unapplied draft.
Use `checkpoint_revision_id`. After a successful Apply this matches `applied_revision_id`; a completed failed mutation's retained checkpoint can have a different state-producing revision.
Do not guess the revision when recovering a failure.
Successful Destroy retains the Foundation object and template association, allowing Apply again.
For a provisioned Foundation, wait for successful destruction before sending `DELETE` to `$FOUNDATION_URL` to remove the object and association.
An unconfigured Foundation with no retained state and no active run can be deleted directly.
After deletion, create and associate a replacement Foundation.
There is no detach/rebind shortcut.

A completed failed mutation with a valid returned state checkpoint can be retried from that checkpoint.
An acquired interrupted mutation with an UNKNOWN outcome remains fenced and requires manual infrastructure/state reconciliation, not an automatic retry.
Supported automated UNKNOWN recovery is not implemented.
Never discard state to bypass a failure.
Deleting the Foundation is separate from deleting its template.

## Companion Terraform provider

The POC provides the combined `coder_foundation` data source and `coder_template` metadata in the companion `terraform-provider-coder` repository.
Use `coder_foundation.current.outputs` for public roots and `coder_foundation.current.sensitive_outputs` for sensitive roots.
Do not use the separate `coder_template_outputs` data source.
Released provider version 2.19.0 alone does not contain them.
The included `terraform-provider-coder.patch` applies to provider commit `e44b8cc9304e107f07bb4b34dd9df059c230a04d`.
Apply it in a separate provider checkout with `git apply --unidiff-zero`, then run `go build -o /absolute/path/to/provider-binary-directory/terraform-provider-coder .`.
Configure an explicit Terraform CLI development override for `coder/coder`:

```hcl
provider_installation {
  dev_overrides {
    "coder/coder" = "/absolute/path/to/provider-binary-directory"
  }
  direct {}
}
```

That directory must contain the built `terraform-provider-coder` executable.
Set `TF_CLI_CONFIG_FILE` to this file when running `./scripts/develop.sh` or
Terraform locally.
The version constraint in the example permits initialization against the
registry; Terraform uses the explicit local binary for plan/apply. This is a
local development arrangement, not a published provider release.

## Verification

Refer to the [phase-1 validation report](VALIDATION-20261028.md) for recorded checks and integration findings.
The earlier [Foundation validation report](RESULTS.md) records historical native lifecycle results.

The phase-1 report's [split-output follow-up](VALIDATION-20261028.md#split-output-follow-up) records native lifecycle checks of the public and sensitive output objects.
Earlier whole-object sensitivity observations remain historical evidence, not current guidance.
Use only the explicitly recorded checks and their stated scope; do not interpret these instructions as test results.

For local checks with the companion override:

```bash
terraform fmt -check -recursive examples/shared-resources
terraform -chdir=examples/shared-resources/template init -backend=false -input=false
terraform -chdir=examples/shared-resources/template validate
terraform -chdir=examples/shared-resources/workspace init -backend=false -input=false
terraform -chdir=examples/shared-resources/workspace validate
terraform -chdir=examples/shared-resources/workspace test
```

The workspace mock test covers public and sensitive value consumption, direct public `for_each`, the namespace output, snapshot run ID, and Foundation metadata.
Mocks exercise the supplied value shapes, not the provider's exact type transport or sensitivity marks.
They do not verify live HTTP or the provisioning lifecycle.
Initialization may download dependencies; it is not required for formatting and is not an offline check.

## Scope and limitations

The standalone Foundation editor provides a file tree, file operations, a full-screen code editor, and run logs.
**Save files** saves an immutable multi-file archive without running Terraform.
**Plan** previews the saved revision without applying infrastructure.
**Apply** runs a fresh plan before applying, not a previously approved plan artifact, and does not publish a per-workspace version.
The workspace template editor uses **Build** and **Promote** separately.
Local modules can be bundled in the archive, preserving their bytes.
Pin provider and remote-module versions: the source revision is immutable, but an unpinned external dependency is not.
Coder owns the shared resource state, so external Terraform backends are not supported.

The shared configuration can use arbitrary providers and modules with the
existing provisioner's credentials. Coder does not interpret networking,
namespaces, or relationships between the resources. For example, the Serpent
workspace can replace its namespace variable with the `namespace` output, while
a Cilium module in the shared configuration selects the template UUID from
`data.coder_template.current.id`.

A module reference shares code, not Terraform state ownership.
Foundation association is one-to-one and has no detach/rebind in this iteration.
Cross-template sharing is not implemented.

Compatibility and rollout are operator-owned, with no workspace-version floor.
Immutable revisions and pinned snapshots do not establish a compatibility contract or restore infrastructure deleted by a later Apply.
No inferred compatibility gating, automatic workspace upgrade, or safe Foundation-only rollback is promised.
Warnings and any sampled impact assessment are advisory; semantic compatibility checking remains out of scope.

Use shared outputs for per-workspace resources, not dynamic parameter definitions,
presets, or provisioner routing tags in this POC. Those use a separate preview
evaluator and are not part of the new output-snapshot integration.

There is no background reconciliation or claim of verified network enforcement.
Acquired interrupted mutations can have an UNKNOWN outcome; supported UNKNOWN
recovery is not implemented. The earlier policy-specific POC and its history
remain separate. Do not adopt its managed resources into another Terraform state
or delete its existing resources as part of this example.
