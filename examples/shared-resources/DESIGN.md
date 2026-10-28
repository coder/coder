# Foundations: design and implementation boundaries

## Product model

A template defines each workspace. A **Foundation** owns the infrastructure those
workspaces share. It is a live Terraform deployment, not a blueprint or another
workspace template.

Each Foundation belongs to an organization and has its own stable identity, name,
immutable source revisions, Terraform state, output snapshots, provisioner routing,
and run history. There is one live deployment per Foundation, not a deployment
for every revision. A template can have at most one Foundation, and a Foundation
can serve at most one template in the same organization.

The association is persistent: there is no detach or rebind operation.
Destroy retains the Foundation object and association.
Replacing the Foundation requires deletion of the Foundation and creation and association of a new one.
Destroy provisioned infrastructure before deleting the Foundation.
An unconfigured Foundation with no retained state and no active run can be deleted directly.
Cross-template sharing and multi-consumer migration are outside this iteration.
Moving resource blocks does not migrate Terraform state.

## Template administrator experience

**Foundations** appears between **Templates** and **Agents** in the top-level
navigation, with a template-style list. Creation, source editing, and runs are
standalone workflows. The template page shows its association or lets an
administrator select an available Foundation in the same organization. Its
**Create Foundation** link carries the template's organization to the creation
page; creation does not automatically attach the Foundation. Neither creation
nor association executes Terraform. There is no **Shared / Per-workspace** toggle
in the template editor.

The Foundation editor supports a multi-file root, including bundled local modules.
Its source history and Terraform lifecycle are independent of workspace versions:

- **Save files** saves an immutable source revision without running Terraform.
- **Plan** previews Terraform changes for the saved revision.
  It does not apply infrastructure or publish new outputs.
- **Apply** intentionally changes live infrastructure. It runs a fresh plan,
  not a previously approved immutable plan artifact. Review warnings and confirm
  the operation; the final changes may differ from the earlier preview.
- **Destroy** uses the state-producing source and retained state to remove managed
  infrastructure. It does not delete the Foundation or its association.
- **Delete** removes the Foundation object after cleanup. It is separate from
  Destroy and from template deletion.

The Foundation editor has separate **Save files**, **Plan**, and **Apply** controls.
These actions do not publish workspace versions.
The workspace template editor uses **Build** and **Promote**.

Workspace source can be prepared before Foundation Apply: the workspace template
editor's **Build** saves an inactive version and its source even when import fails
because required Foundation outputs are missing. After Foundation Apply, reopen
that failed version and select **Build** again, with unchanged source if appropriate.
This creates a fresh version and import; after success, select **Promote**. It does
not resume the failed version. There is no first-class awaiting-outputs state or
automatic retry. This workflow was checked read-only, not exercised end to end here.
A bootstrap root is a practical way to create the template and association. Do not
invent fallback outputs merely to hide a required dependency.

## Metadata, outputs, and snapshots

The combined data source is the normal consumption interface:

```tf
data "coder_foundation" "current" {}

locals {
  shared           = data.coder_foundation.current.outputs
  shared_sensitive = data.coder_foundation.current.sensitive_outputs
}
```

`id`, `name`, and `template_ids` describe the contextual Foundation. Metadata is
available in the Foundation's own Plan, Apply, and Destroy contexts, including
before its first Apply. It is not dependent on having published outputs.
`snapshot_id` identifies the run that produced the supplied outputs, not a source
revision selector. In a
workspace context it describes the associated Foundation; IDs are computed
metadata, not selectors for arbitrary Foundations. An unbound Foundation has no
attached template IDs. `coder_template` continues to describe an attached template,
not the Foundation. Preserve existing template UUIDs, labels, resource addresses,
and names when adopting this ownership model.

`outputs` contains only public Terraform root outputs and is not marked sensitive.
`sensitive_outputs` contains only sensitive roots and is always marked sensitive, including when empty.
Both typed objects come from the same pinned snapshot and use its exact root type descriptors.
Nested objects, collection kinds and element types, numbers, booleans, and typed nulls are preserved without stringification or JSON type inference.
Each root appears in only one object, based on the snapshot's root sensitivity metadata.
A sensitive root's public-looking children remain inside `sensitive_outputs`, not duplicated into `outputs`.
Ordinary management responses omit sensitive values but retain per-output metadata.
The Foundation detail page lists each output's name, type, and sensitivity, showing actual public values and a fixed `********` mask for sensitive values.

Public sets and maps work directly with `for_each` without `nonsensitive(...)` for unrelated secret roots.
Keep public identity keys in separate root outputs from secret values.
Consume secrets through `data.coder_foundation.current.sensitive_outputs`, and mark downstream root outputs containing them as sensitive.
Do not merge both objects and remove their sensitivity marks.
Sensitivity is not encryption or a sandbox: trusted Terraform authors, state readers, and provisioner operators can access secrets.
State and logs require appropriate access controls.

Each workspace build pins its selected output snapshot and Foundation provenance.
A later Foundation Apply does not rewrite existing builds.
A workspace start uses the available published snapshot even while a Foundation run is active or after a failed run.
Before the first publication, both output objects are empty, and `sensitive_outputs` remains sensitive.
There is no blanket startup gate based on association, run status, or Foundation version.
Terraform decides whether missing outputs make that workspace configuration invalid.

Stop and delete use the snapshot associated with the retained workspace Terraform
state, not simply the latest attempted build. A failed plan does not replace that
state-associated snapshot. A partial workspace Apply that returns a state checkpoint
must retain the corresponding snapshot for subsequent cleanup. Snapshot pinning
preserves inputs; it cannot preserve infrastructure removed by a later Foundation run.

## Permissions and execution

Foundation read, source editing/Plan, Apply/Destroy, and creation are distinct Coder
capabilities. Applying requires an authorized operator's explicit confirmation,
not mandatory independent approval. Template and Foundation administration can be
granted together or separately.

These are logical permission boundaries, not isolation from malicious Terraform.
Operators own provisioner credentials. Prefer separate pools with narrow workspace
credentials and only the shared-resource permissions needed in the Foundation pool.
Foundation routing is independent of workspace routing; queued jobs retain their
selected routing. Both Foundation and consuming workspace/validation pools need
workers supporting provisionerd API v1.25 or later within major version 1, plus the
`coder.template_resources=1` capability. The protocol requirement prevents older
workers from silently discarding output sensitivity metadata. Upgrade both pools
before use; a protocol version alone does not opt a worker into the capability.

Runs use native Terraform provisioner jobs with persisted logs, cancellation,
checkpoint fencing, and one reserved Foundation run at a time. Source archives
preserve local module bytes. Pin external providers and remote modules separately;
an immutable archive does not freeze an unpinned external dependency. Coder owns
state, so external Terraform backends are unsupported.

Migration from template-owned shared state must preserve Terraform bytes, revision
IDs, run history, snapshots, and resource identities without applying infrastructure.
Organization-scoped constraints prevent mismatched owners; association constraints
prevent multiple templates from independently managing the same Foundation state.

## Compatibility and rollout

Rollout is operator-owned. There is no minimum workspace-version floor, inferred
compatibility gate, automatic rebuild, or requirement to upgrade workspaces after
every Foundation change. Operators can update incrementally, schedule maintenance,
or accept temporary incompatibility. A workspace-version rollback does not roll
back the Foundation, and a Foundation-only rollback is not necessarily safe.

Source revisions, output snapshots, and compatibility are different concepts.
Removing an output or changing its type can invalidate an older workspace root.
Changing a root's sensitivity moves its key between `outputs` and `sensitive_outputs`, requiring an update to consuming references.
An unchanged output does not prove unchanged application behavior. With only one
live deployment, selecting historical output values would not restore historical
infrastructure. Automated semantic compatibility checking remains out of scope.

Impact warnings are advisory, not guarantees of safety or network enforcement.
Optional assessment may cover no workspaces, selected representatives, or all
workspaces. Any assessment must identify its sample and assessed revision/snapshot,
and distinguish **not assessed** from **assessed with no planned changes**. Do not
claim that unimplemented assessment runs have occurred. One plan per workspace
version is only a sample: parameters, state, and pinned snapshots can differ.

Before Apply, Terraform may know some output changes and leave others unknown.
Unknown is not proof of a change. Actual post-Apply outputs provide more information,
not an application compatibility guarantee. No exhaustive dependency graph,
background fleet reconciliation, or bulk upgrade scheduler is promised.

## Destruction, deletion, and recovery

`prevent_destroy_with_workspaces` defaults to `true` and is configurable by an
authorized operator. With protection enabled, Destroy is blocked while consuming
workspaces remain, including stopped, hidden, and historical-snapshot consumers.
Disabling it intentionally permits destruction with consumers; it does not make
that operation safe. Confirm the impact and plan cleanup before changing protection.
The protection is not a guarantee against destructive changes in an ordinary Apply.

Successful Destroy retains the Foundation object, source/run history, and template
association so the same Foundation can be applied again. Delete is a separate
operation after infrastructure cleanup. Template deletion does not implicitly
destroy infrastructure. Do not discard state or use `terraform state rm` to make
cleanup appear successful.

A completed failed mutation with a valid returned checkpoint is retryable from that
checkpoint. Retain its state-producing revision, which may differ from the last
successfully published output revision. An acquired interrupted mutation with an
uncertain outcome is **UNKNOWN**, not an ordinary retryable failure. It remains
fenced until manual infrastructure/state reconciliation; a supported automated
UNKNOWN recovery workflow is not implemented. Cancellation is a request, not proof
that no infrastructure changed. Independent Foundation management keeps cleanup
accessible even when a workspace template version is broken.

## Verification focus

Check independent IDs and unchanged migrated state, organization isolation,
persistent one-to-one association, permissions and routing, Plan/Apply/Destroy,
metadata in self contexts, sensitive output marks and management redaction,
state-associated cleanup snapshots, configurable destruction protection, and
checkpoint retry versus UNKNOWN fencing. Exercise the minimal Terraform-only root
before any separately authorized Kubernetes validation. Keep observed results in
[RESULTS.md](RESULTS.md), distinct from this design and from unimplemented workflows.
