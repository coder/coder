# Template-scoped Terraform resources

A template has two Terraform configurations:

- **Template resources:** managed once per template, with their own state and
  explicit plan, apply, and destroy runs.
- **Workspace resources:** managed separately for each workspace through normal
  workspace builds.

The minimal example below uses only `terraform_data`, so it creates no cluster or
cloud resources. For the real Serpent demo with a template-owned Kubernetes
namespace and network policy, see [Kubernetes template resources](kubernetes/README.md).
Both examples have two separate Terraform roots. Do not combine or apply them in
the same directory/state.

## Try it

Run the development server with the `template-resources` experiment (the default
in `../start-native-dev.py`) and the companion provider override described below.
Create a normal template with a bootstrap workspace configuration first. For this
example, `resource "terraform_data" "bootstrap" { input = "bootstrap" }` is enough.
Open **Edit files > Template resources**, add `template/main.tf` as `main.tf`,
then **Build** to save and plan a revision and **Publish** to apply it.
Then upload `workspace/main.tf` as a new workspace configuration version
associated with that template and create a workspace.

The workspace reads the template's latest successful outputs through
`coder_template_outputs`. Each build receives a fixed snapshot rather than
fetching changing values while Terraform runs. `outputs` exposes a typed object,
preserving nested objects, collections, booleans, numbers, and nulls without
requiring `jsondecode`:

```hcl
data "coder_template_outputs" "current" {}

locals {
  template = data.coder_template_outputs.current.outputs
}
```

The `private_setting` output is sensitive and is deliberately excluded from
workspace snapshots. Terraform state and logs still require trusted operator
access; `sensitive` is not encryption.

Change the greeting in the template configuration and apply again. Existing
workspace builds are not changed. The next workspace start reads the new
snapshot. Rolling back the workspace configuration does not roll back template
resources. Delete the test workspace before explicitly destroying template
resources.

## Companion Terraform provider

The POC introduces `coder_template_outputs` and `coder_template` data sources in
the companion `terraform-provider-coder` repository. Released version 2.19.0
alone does not contain them. The included `terraform-provider-coder.patch` applies
to provider commit `e44b8cc9304e107f07bb4b34dd9df059c230a04d`. Apply it in a
separate provider checkout with `git apply --unidiff-zero`, then run
`go build -o /absolute/path/to/provider-binary-directory/terraform-provider-coder .`.
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
Pass this file to the development launcher with `--terraform-cli-config`.
The version constraint in the example permits initialization against the
registry; Terraform uses the explicit local binary for plan/apply. This is a
local development arrangement, not a published provider release.

## Verification

The native end-to-end check passed using these two roots: initial startup gating,
plan/apply/logs, sensitive-output exclusion, nested output types, independent
second template apply and rollback without workspace changes, updated outputs
on the next workspace start, guarded destroy using the applied revision, and
native workspace/template cleanup.
The test created no cluster or cloud resources. Backend/race tests, frontend
checks, and generation also passed.

## Scope and limitations

Both editor scopes use a file tree, file operations, a full-screen code editor,
and build output. Template-resource Build saves an immutable multi-file archive
and plans it; Publish applies that built revision without publishing a workspace
version. Publish runs a fresh plan before applying, not a previously approved plan
artifact. Local modules can be bundled in the archive, preserving their bytes.
Pin provider and remote-module versions: the source revision is immutable, but an
unpinned external dependency is not. Coder owns the template resource state, so
external Terraform backends are not supported.

Template Terraform can use arbitrary providers and modules with the existing
provisioner's credentials. Coder does not interpret networking, namespaces, or
relationships between the resources. For example, the Serpent workspace can
replace its namespace variable with the `namespace` output, while a
Cilium module in template configuration selects the template UUID from
`data.coder_template.current.id`.

A module reference shares code, not Terraform state ownership. Resources shared
across templates must have one owner and be referenced by the other
configurations. There is no cross-template resource-association database model.

Use template outputs for workspace resources, not dynamic parameter definitions,
presets, or provisioner routing tags in this POC. Those use a separate preview
evaluator and are not part of the new output-snapshot integration.

There is no background reconciliation or claim of verified network enforcement.
Acquired interrupted mutations can have an UNKNOWN outcome; supported UNKNOWN
recovery is not implemented. The earlier policy-specific POC and its history
remain separate. Do not adopt its managed resources into another Terraform state
or delete its existing resources as part of this example.
