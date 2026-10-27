# Shared and per-workspace resources

A template defines the resources shared by its workspaces and the resources each
workspace gets. It has two provisioning scopes, each with its own Terraform
configuration, state, and publishing lifecycle:

- **Shared resources:** provisioned once for all workspaces using the template,
  with independent plan, apply, and destroy runs.
- **Per-workspace resources:** provisioned separately for each workspace through
  normal workspace builds.

The minimal example below uses only `terraform_data`, so it creates no cluster or
cloud resources. For the real Serpent demo with a template-owned Kubernetes
namespace and network policy, see
[Kubernetes provisioning scopes](../kubernetes-shared-resources/README.md).
Both examples have two separate Terraform roots. Do not combine or apply them in
the same directory/state.

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

The experiment, API, and data-source identifiers retain their existing names;
**Shared** and **Per-workspace** name the customer-facing provisioning scopes.
Create a normal template with a bootstrap workspace configuration first. For this
example, `resource "terraform_data" "bootstrap" { input = "bootstrap" }` is enough.
Open **Edit files > Shared**, add `template/main.tf` as `main.tf`,
then **Build** to save and plan a revision and **Publish** to apply it.
Then upload `workspace/main.tf` as a new workspace configuration version
associated with that template and create a workspace.

The workspace reads the shared scope's latest successful outputs through
`coder_template_outputs`. Each build receives a fixed snapshot rather than
fetching changing values while Terraform runs. `outputs` exposes a typed object,
preserving nested objects, collections, booleans, numbers, and nulls without
requiring `jsondecode`:

```hcl
data "coder_template_outputs" "current" {}

locals {
  shared = data.coder_template_outputs.current.outputs
}
```

The `private_setting` output is sensitive and is deliberately excluded from
workspace snapshots. Terraform state and logs still require trusted operator
access; `sensitive` is not encryption.

Change the greeting in the shared configuration and apply again. Existing
workspace builds are not changed. The next workspace start reads the new
snapshot. Rolling back the per-workspace configuration does not roll back shared
resources. Delete the test workspace before explicitly destroying shared resources.

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
Set `TF_CLI_CONFIG_FILE` to this file when running `./scripts/develop.sh` or
Terraform locally.
The version constraint in the example permits initialization against the
registry; Terraform uses the explicit local binary for plan/apply. This is a
local development arrangement, not a published provider release.

## Verification

The native end-to-end check passed using these two roots: initial startup gating,
plan/apply/logs, sensitive-output exclusion, nested output types, independent
second shared-resource apply and rollback without workspace changes, updated outputs
on the next workspace start, guarded destroy using the applied revision, and
native workspace/template cleanup.
The test created no cluster or cloud resources. Backend/race tests, frontend
checks, and generation also passed.

## Scope and limitations

Both provisioning scopes use a file tree, file operations, a full-screen code
editor, and build output. In **Shared**, Build saves an immutable multi-file
archive and plans it; Publish applies that built revision without publishing a
per-workspace version. Publish runs a fresh plan before applying, not a previously
approved plan artifact. Local modules can be bundled in the archive, preserving
their bytes. Pin provider and remote-module versions: the source revision is
immutable, but an unpinned external dependency is not. Coder owns the shared
resource state, so external Terraform backends are not supported.

The shared configuration can use arbitrary providers and modules with the
existing provisioner's credentials. Coder does not interpret networking,
namespaces, or relationships between the resources. For example, the Serpent
workspace can replace its namespace variable with the `namespace` output, while
a Cilium module in the shared configuration selects the template UUID from
`data.coder_template.current.id`.

A module reference shares code, not Terraform state ownership. Resources shared
across templates must have one owner and be referenced by the other
configurations. There is no cross-template resource-association database model.

Use shared outputs for per-workspace resources, not dynamic parameter definitions,
presets, or provisioner routing tags in this POC. Those use a separate preview
evaluator and are not part of the new output-snapshot integration.

There is no background reconciliation or claim of verified network enforcement.
Acquired interrupted mutations can have an UNKNOWN outcome; supported UNKNOWN
recovery is not implemented. The earlier policy-specific POC and its history
remain separate. Do not adopt its managed resources into another Terraform state
or delete its existing resources as part of this example.
