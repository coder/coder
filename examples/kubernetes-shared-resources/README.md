# Kubernetes provisioning scopes: Serpent

See [validation results and limitations](RESULTS.md) for completed checks.

A template has two provisioning scopes with independent Terraform roots:

- **Shared resources:** `template/main.tf` creates `workspaces` once for the
  template and calls our `coder_network_policy` module with the template UUID and
  owned namespace.
- **Per-workspace resources:** `workspace/main.tf` creates each pod and 10 GiB PVC.
  Both read `data.coder_template_outputs.current.outputs.namespace`
  without a fallback. `startup.sh` is unchanged from the earlier Serpent demo.

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
  Released Coder 2.19.0 alone lacks the new data sources. Set `TF_CLI_CONFIG_FILE`
  to the explicit Terraform CLI development override.
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

The fixed `workspaces` namespace must have only one template owner. If an earlier
shared namespace exists, first delete its workspaces and policies through their
native Coder lifecycles. Inspect anything remaining before separately authorizing
namespace deletion. Do not import/adopt unrelated resources or manage this
namespace outside its Terraform root. Never bypass UNKNOWN quarantine.

1. Retain the existing `serpent-dev` template. On a fresh installation, create a
   template using a minimal bootstrap workspace root to obtain its UUID, but do
   not create a workspace yet.
2. From the repository root, package the shared root and canonical module:

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

3. Upload through `POST /api/v2/files`, then save a shared-resource revision
   through `POST /api/v2/templates/{template_id}/resources/versions` with
   `{"file_id":"<uploaded hash>"}`. These are authenticated Coder APIs; retain
   credentials outside the repository. Alternatively, create the same file tree
   directly in **Edit files > Shared**.
4. In that editor, **Build** saves an immutable archive and runs Terraform plan.
   **Publish** applies the successfully built revision independently of workspace
   versions. Wait for success before importing the consuming workspace root.
   Output `namespace` must be `workspaces`; `template_id` must match this template.

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
Build and publish from **Shared**. Verify the changed network behavior
separately: Terraform success is not proof of enforcement. Workspace builds, pods,
and PVCs must remain unchanged. Build/publish the earlier restricted source to
roll back independently; workspace-version rollback never rolls back this root.

Delete all consuming workspaces through Coder before explicitly destroying shared
resources. Workspace deletion removes its pod/PVC but leaves the namespace and
policy. Shared-resource destroy removes the policy before its namespace because
of the Terraform reference. Do not delete a namespace to bypass a failed destroy.

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
