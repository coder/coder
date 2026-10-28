# Kubernetes provisioning scopes: Serpent

As a template administrator, use this example to manage a Serpent workspace and its shared Kubernetes infrastructure independently.
The [validation results and limitations](RESULTS.md) record pre-Foundation checks, not a live Foundation verification.

Manage Foundations from the template-style list under **Foundations**, between **Templates** and **Agents** in the top-level navigation.
Foundation creation, editing, and publication are standalone workflows, using the same full file editor as templates.
The template editor has no **Shared / Per-workspace** toggle.
The Terraform roots remain independent:

- **Shared:** a Foundation owns `template/main.tf`, which creates `workspaces` once and calls `coder_network_policy` with the attached template UUID and owned namespace.
- **Per-workspace:** `workspace/main.tf` creates each pod and 10 GiB PVC.
  Both read `local.shared.namespace`, an alias for `data.coder_foundation.current.outputs.namespace`, without a fallback.
  `startup.sh` is unchanged from the earlier Serpent demo.

A Foundation has its own organization-scoped ID, source revisions, state, routing, and run history.
Its association with a template is one-to-one within the same organization, with no detach or rebind operation.
Destroy retains the Foundation and association.
Replacement requires deletion and creation/association of a new Foundation.
Destroy provisioned infrastructure before deleting the Foundation.
An unconfigured Foundation with no retained state and no active run can be deleted directly.
The source path remains `template/` for continuity.
`data.coder_foundation.current` combines Foundation metadata and outputs.
Metadata is available in the Foundation's own Plan, Apply, and Destroy contexts, even before its first Apply; this root exports `foundation_id` to demonstrate it.
`data.coder_template.current.id` still identifies the attached template, not the Foundation.
Do not replace the policy UUID, selectors, labels, namespace, or other resource identity inputs with a Foundation ID.
That substitution could replace or retarget existing infrastructure.

The canonical module source is included directly in
`template/modules/coder-network-policy`. The upload command below includes its
actual Terraform files, with no symlinks or dependencies on `scratch/`. Each native
revision therefore preserves its own module bytes for later publication and destroy.
There is no policy-specific API, target, attachment, or module injection.

## Prerequisites

- The repository's development dependencies, Terraform >= 1.9.0 and < 2.0.0,
  `kubectl`, GNU tar, curl, and jq.
- The [companion Coder provider](../shared-resources/README.md#companion-terraform-provider)
  and a Coder server with the `template-resources` experiment enabled.
  Released Coder provider 2.19.0 alone lacks these data sources. Set `TF_CLI_CONFIG_FILE`
  to the explicit Terraform CLI development override.
- Foundation and consuming workspace/validation provisioner pools supporting
  provisionerd API v1.25 or later within major version 1 and the
  `coder.template_resources=1` capability. Upgrade both pools to preserve output
  sensitivity metadata.
- The existing local K3s cluster and kubeconfig, context `default`, healthy Cilium
  with its CNP CRD, DNS/L7 proxy and Envoy. CoreDNS uses `k8s-app=kube-dns` in
  `kube-system`. A default dynamic storage class must support 10 GiB PVCs.
- The built-in provisioner needs namespace, pod, PVC, and Cilium policy management
  plus schema discovery. No separate provisioner pod, namespace, or service
  account is needed. Workspace pods do not mount Kubernetes credentials.
- Coder and its embedded relay must be reachable from pods at `172.20.0.3:3100`.
  Replace this demo-specific address in Terraform and the development command when needed;
  do not broaden the `/32`. Keep direct P2P and external STUN disabled.

Use only the existing local cluster. Do not bootstrap networking, use AWS
credentials, or recreate `coder-policy-poc-a` or `coder-policy-poc-b`.

## Start the development server

From the repository root, set the paths to your existing local kubeconfig and
[provider override](../shared-resources/README.md#companion-terraform-provider):

```bash
export DEMO_KUBECONFIG="/absolute/path/to/local-kubeconfig"
export TF_CLI_CONFIG_FILE="/absolute/path/to/coder-provider.tfrc"
kubectl --kubeconfig "$DEMO_KUBECONFIG" --context default config view --minify
```

Confirm that `default` selects the intended HTTPS loopback Kubernetes API, with no
proxy or credential plugin. Then check the cluster and start Coder directly:

```bash
kubectl --kubeconfig "$DEMO_KUBECONFIG" --context default get nodes

env -i PATH="$PATH" HOME="$HOME" DEVELOP_IN_CODER=1 \
  AWS_CONFIG_FILE=/dev/null AWS_SHARED_CREDENTIALS_FILE=/dev/null \
  AWS_EC2_METADATA_DISABLED=true \
  TF_CLI_CONFIG_FILE="$TF_CLI_CONFIG_FILE" \
  KUBE_CONFIG_PATH="$DEMO_KUBECONFIG" KUBECONFIG="$DEMO_KUBECONFIG" \
  ./scripts/develop.sh \
  --port 3100 --web-port 8180 --prometheus-port 0 \
  --access-url http://172.20.0.3:3100 --skip-setup -- \
  --experiments template-resources --telemetry=false \
  --derp-config-url '' --derp-server-stun-addresses '' \
  --block-direct-connections=true
```

This uses the built-in provisioners and excludes inherited deployment, cloud, and
proxy credentials. Replace the pod-reachable access address and the matching
Terraform `/32` allowance together if needed. Open port 8180 for the UI. Do not
start another instance on these ports or reset an existing demo database.

## Create the shared resources

The fixed `workspaces` namespace must have only one Terraform state owner.
If a migrated Foundation already owns these resources, retain that Foundation and its template association.
Do not create a second Foundation to adopt the same resources.
Before any separately authorized replacement, delete consumers and destroy the original infrastructure through its native Coder lifecycle.
Do not import/adopt unrelated resources or manage this namespace outside its Terraform root.
Never bypass UNKNOWN quarantine.

1. Retain the existing `serpent-dev` template and its UUID.

   On a fresh installation, create a template using a minimal bootstrap workspace root, but do not create a workspace yet.

2. Read the template's association using `GET /api/v2/templates/{template_id}/foundation`.

   Retain a returned Foundation's `id` and `organization_id` for subsequent requests.
   If the response is `null`, create a Foundation with `POST /api/v2/organizations/{organization_id}/foundations` and body `{"name":"serpent-foundation","provisioner_tags":{}}`.
   Bind its returned `id` using `PUT /api/v2/templates/{template_id}/foundation` with `{"foundation_id":"<Foundation UUID>"}`.
   Creation and binding do not execute Terraform.
   There is no blanket startup gate or workspace-version floor.
   Before the first Apply, outputs are empty: this workspace root needs `namespace` and cannot successfully plan until that output exists.
   Starts pin available published outputs even during an active or failed Foundation run.
   Stop and delete use the snapshot and Foundation provenance associated with retained workspace state, not merely the latest attempted build.
   The [minimal API workflow](../shared-resources/README.md#foundation-api-workflow) provides curl examples.
   In the UI, the template's **Foundation** page only manages the association and selection of an unbound Foundation in the same organization.
   Its **Create Foundation** link navigates to `/foundations/new?organization=<id>` using the template's organization ID.
   Creation does not automatically attach the Foundation. Return to the template and select it explicitly before building this shared root.

3. You can prepare the workspace source before Foundation Apply.
   Add `workspace/main.tf` as `main.tf` and `workspace/startup.sh` as `startup.sh` in the workspace template editor, then select **Build**.
   An import failure on missing `namespace` still retains the inactive version and its source.
   After Foundation Apply, reopen that failed version and select **Build** again, then **Promote** the new successful version.
   Unchanged source is allowed; this creates a fresh version/import, not a same-version resume.
   There is no first-class awaiting-outputs state or automatic retry.
   This flow was checked read-only, not verified end to end here.

4. From the repository root, package the shared root and canonical module:

   ```bash
   tar --format=ustar --no-recursion \
     -C examples/kubernetes-shared-resources/template \
     -cf /tmp/opencode/serpent-template-resources.tar \
     main.tf outputs.tf modules modules/coder-network-policy \
     modules/coder-network-policy/main.tf \
     modules/coder-network-policy/outputs.tf \
     modules/coder-network-policy/variables.tf \
     modules/coder-network-policy/versions.tf
   ```

5. Upload the archive through `POST /api/v2/files`.

   These are authenticated Coder APIs.
   Retain credentials outside the repository.

6. Save a revision through `POST /api/v2/organizations/{organization_id}/foundations/{foundation_id}/versions` with `{"file_id":"<uploaded hash>"}`.

   Alternatively, create the same file tree directly in the Foundation's standalone file editor and select **Save files**.
   Saving creates an immutable archive without running Terraform.

7. In the Foundation UI, select **Plan** to preview the saved revision without applying infrastructure.
8. Select **Apply** and confirm the live infrastructure change.

   Apply runs a fresh plan, not a previously approved plan artifact, independently of workspace versions.
   The API equivalent is `POST /api/v2/organizations/{organization_id}/foundations/{foundation_id}/runs` with `{"revision_id":"<revision UUID>","action":"apply"}`.
   Use `"action":"plan"` for a plan-only run.

9. After success, rebuild the retained failed workspace version, or import the consuming root for the first time.

   Output `namespace` must be `workspaces`, and `template_id` must match the attached template, not the Foundation.
   The separate `foundation_id` output must match the Foundation.

The module allows exact DNS queries for `github.com`, `proxy.golang.org`, and
`sum.golang.org` to CoreDNS on TCP/UDP 53; HTTPS to those names on TCP 443 with
matching TLS SNI; and the local Coder `/32` on TCP 3100. Both policy directions
are default deny. Direct P2P is explicitly disabled.

## Publish the per-workspace configuration

Use your Coder CLI login against the local development server:

```bash
tar --format=ustar \
  -C examples/kubernetes-shared-resources/workspace \
  -cf - main.tf startup.sh |
  coder templates push serpent-dev --directory - --ignore-lockfile
```

This creates a version linked to the existing template. An unlinked import has
no shared outputs and intentionally fails on the missing namespace. Do not add
a fallback to hide that dependency. Create a workspace through Coder after the
version is active.

The public Go 1.21.13 image digest, Serpent revision
`40b60bf4c1495caddc0292f862de275da292e6aa`, Makefile, and linter v1.53.3 are unchanged.

## Verify the workspace

Wait for startup, then connect through Coder SSH or the web terminal. Startup can
take several minutes on the first cold build. From `/workspace/serpent`, run:

```bash
make lint
go build ./...
curl --fail https://github.com
curl --connect-timeout 4 --max-time 6 https://example.com
curl --connect-timeout 4 --max-time 6 http://github.com
curl --connect-timeout 4 --max-time 6 https://1.1.1.1
```

The unchanged `make lint` exits 2 with nine findings: five errcheck, three
loopclosure, and one SA1019. This is not a clean-lint claim. `go build ./...` and
GitHub HTTPS should succeed; the other network probes should fail. Correlate
failures with Hubble policy-denied or DNS-rejection evidence rather than assuming
every timeout proves enforcement. Node image pulls are outside pod policy.

To verify persistence, create a marker under `/workspace`, stop and start through
Coder, and confirm the marker, checkout, linter, and caches survive while the pod
UID changes. Delete the temporary workspace through Coder when done; retain the
shared resources unless intentionally testing their separate cleanup lifecycle.

## Independent policy updates and cleanup

Edit the module's `allowed_https_hosts` argument, for example adding `example.com`.
Plan and Apply from the Foundation's standalone editor. Verify changed network behavior
separately: Terraform success is not proof of enforcement. A policy-only change should
not itself rebuild workspaces or replace their pods/PVCs. Apply the earlier restricted
source to roll back independently, after reviewing its consumer impact;
workspace-version rollback never rolls back this root. Rollout is operator-owned,
with no minimum workspace-version floor or automatic rebuild. Impact warnings and
sampled assessments are advisory, not guarantees of compatibility or safety.

Manage the Foundation independently through these authenticated API routes:

| Method   | Route                                                                                      | Purpose                                                                       |
|----------|--------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------|
| `GET`    | `/api/v2/organizations/{organization_id}/foundations`                                      | List accessible Foundations.                                                  |
| `GET`    | `/api/v2/organizations/{organization_id}/foundations/{foundation_id}`                      | Read settings, status, revisions, runs, and outputs.                          |
| `PATCH`  | `/api/v2/organizations/{organization_id}/foundations/{foundation_id}`                      | Update name, routing, and destruction protection without executing Terraform. |
| `GET`    | `/api/v2/organizations/{organization_id}/foundations/{foundation_id}/runs/{run_id}/logs`   | Read persisted logs.                                                          |
| `PATCH`  | `/api/v2/organizations/{organization_id}/foundations/{foundation_id}/runs/{run_id}/cancel` | Request cancellation.                                                         |
| `DELETE` | `/api/v2/organizations/{organization_id}/foundations/{foundation_id}`                      | Remove the Foundation after cleanup.                                          |

`prevent_destroy_with_workspaces` defaults to `true` and blocks Destroy while consumers remain, including stopped, hidden, and historical-snapshot consumers.
An authorized operator can set it to `false` in a Foundation settings update to accept destruction with consumers.
Omitting it preserves its current value on update.
This protection does not prevent destructive changes in an ordinary Apply.
For this example, leave protection enabled and delete all consuming workspaces through Coder first.
Workspace deletion removes its pod/PVC but leaves the namespace and policy.
Use `POST /api/v2/organizations/{organization_id}/foundations/{foundation_id}/runs` with `action:"destroy"` and the state-producing revision as `revision_id`.
After a successful Apply this is `applied_revision_id`; after a completed failed mutation, use the revision associated with the retained checkpoint, not an arbitrary draft.
Foundation Destroy removes the policy before its namespace because of the Terraform reference.
Successful Destroy retains the Foundation object and template association, allowing Apply again.
Only a separate Delete removes that object after cleanup; replacement then requires a new Foundation and association.

A completed failed mutation with a valid returned checkpoint is retryable from that checkpoint.
An interrupted acquired mutation with an UNKNOWN outcome remains fenced pending manual infrastructure/state reconciliation.
Do not delete a namespace or discard state to bypass a failed Destroy or UNKNOWN quarantine.
Cancellation does not prove that an acquired mutation left infrastructure unchanged.

The [Foundation design](../shared-resources/DESIGN.md#compatibility-and-rollout) describes operator-owned compatibility and rollout.
A pinned snapshot does not preserve an older live policy or restore deleted infrastructure.
There is no inferred compatibility gate, automatic workspace rebuild, or guarantee that a Foundation-only rollback is safe.
Sensitive root values are supported, although this example's outputs are public.
Management APIs omit sensitive values while retaining per-output metadata.
The Foundation detail page lists each output's name, type, and sensitivity, showing actual public values and a fixed `********` mask for sensitive values.
The provider exposes public roots only in `outputs` and sensitive roots only in `sensitive_outputs`, which is always sensitive, even when empty.
Both objects preserve exact Terraform types from the same pinned snapshot.
Public sets and maps in `outputs` work directly with `for_each`, without `nonsensitive(...)` for unrelated secrets.
Keep public identity keys in separate root outputs from secret values.
Changing a root's sensitivity moves its key between `outputs` and `sensitive_outputs`, requiring an update to consuming references.
Refer to the [minimal example](../shared-resources/README.md) for sensitive value consumption.

## Local checks

With `TF_CLI_CONFIG_FILE` set to the companion override, initialize and validate
both roots and run their mocked tests:

```bash
terraform -chdir=examples/kubernetes-shared-resources/template init -backend=false -input=false
terraform -chdir=examples/kubernetes-shared-resources/template validate
terraform -chdir=examples/kubernetes-shared-resources/template test
terraform -chdir=examples/kubernetes-shared-resources/workspace init -backend=false -input=false
terraform -chdir=examples/kubernetes-shared-resources/workspace validate
terraform -chdir=examples/kubernetes-shared-resources/workspace test
terraform -chdir=examples/kubernetes-shared-resources/template/modules/coder-network-policy init -backend=false -input=false
terraform -chdir=examples/kubernetes-shared-resources/template/modules/coder-network-policy test
```

These tests do not mutate the cluster. Coder owns the live state; never run a
separate local apply/destroy against the same resources.

The namespace output is an explicit contract, not an automatic cross-root Terraform
graph or migration mechanism. Changing it can replace pods/PVCs on later starts and
lose data. Cilium allows compose additively; payload inspection, misuse of approved
services, node image pulls, and host/container escapes are outside these controls.
Supported UNKNOWN recovery remains unimplemented.
