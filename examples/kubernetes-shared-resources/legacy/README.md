# Native workspace network-policy demo

For the current Serpent demo, see
[Kubernetes provisioning scopes](../README.md).
The shared configuration owns the namespace and policy, while the per-workspace
configuration consumes the namespace output. This guide preserves the
earlier policy-specific setup; use `--experiments network-policies` to enable its API
explicitly. Do not apply this guide's shared-namespace setup or upload its legacy
workspace configuration over the current provisioning-scopes demo.

This directory contains the earlier `serpent dev` workspace template. The trusted
Cilium Terraform module is in
`examples/kubernetes-shared-resources/template/modules/coder-network-policy`.
The older Helm deployment and cluster-bootstrap experiments are not required.
Credentials, Terraform state, VM images, caches, and execution logs are excluded.
Use an existing local K3s cluster; do not bootstrap or replace its networking.

## Prerequisites

- Repository development dependencies for `scripts/develop.sh`, plus
  kubectl, and Terraform >= 1.9.0, < 2.0.0. Module tests were run with 1.16.6.
- An explicit local kubeconfig with context `default` and a loopback HTTPS API.
  The original demo API is `https://127.0.0.1:22643`.
- Healthy Cilium with the CNP CRD, DNS/L7 proxy, and Envoy. The module tests target
  Cilium 1.20.3. CoreDNS uses `k8s-app=kube-dns` in `kube-system`.
- A default dynamically provisioned storage class with room for a 10 GiB PVC.
- A pod-reachable IPv4 address for the development server on TCP 3100. The
  original demo uses `172.20.0.3`; this is not a portable address.

The provisioner runs inside the development Coder server. Its existing local
kubeconfig must permit pod, PVC, and Cilium policy management in `workspaces`,
as well as provider schema discovery. No separate provisioner namespace, pod,
or service account is required. This is a trusted local demo arrangement, not
production least-privilege hardening. Workspace pods receive no Kubernetes token.
Do not use remote clusters or AWS credentials.

## Start the development server

From the repository root, inspect the explicit context:

```bash
export DEMO_KUBECONFIG="/absolute/path/to/local-kubeconfig"
kubectl --kubeconfig "$DEMO_KUBECONFIG" --context default config view --minify
```

Confirm that `default` selects the intended HTTPS loopback API, with no proxy or
credential plugin. Supply your existing kubeconfig; it is not included in Git.
Only for this earlier policy-specific setup, inspect and create its namespace:

```bash
kubectl --kubeconfig "$DEMO_KUBECONFIG" --context default get nodes
kubectl --kubeconfig "$DEMO_KUBECONFIG" --context default apply \
  -f examples/kubernetes-shared-resources/legacy/workspaces-namespace.yaml
```

Do not apply that manifest over the current shared-root example or recreate the
retired `coder-policy-poc-a` or `coder-policy-poc-b` namespaces. Start Coder directly:

```bash
env -i PATH="$PATH" HOME="$HOME" DEVELOP_IN_CODER=1 \
  AWS_CONFIG_FILE=/dev/null AWS_SHARED_CREDENTIALS_FILE=/dev/null \
  AWS_EC2_METADATA_DISABLED=true \
  TF_CLI_CONFIG_FILE="${TF_CLI_CONFIG_FILE:-/dev/null}" \
  KUBE_CONFIG_PATH="$DEMO_KUBECONFIG" KUBECONFIG="$DEMO_KUBECONFIG" \
  ./scripts/develop.sh \
  --port 3100 --web-port 8180 --prometheus-port 0 \
  --access-url http://172.20.0.3:3100 --skip-setup -- \
  --experiments template-resources --experiments network-policies \
  --network-policy-module-path "$PWD/examples/kubernetes-shared-resources/template/modules/coder-network-policy" \
  --telemetry=false --derp-config-url '' --derp-server-stun-addresses '' \
  --block-direct-connections=true
```

Replace the access address if needed. This uses the trusted module and built-in
provisioners, with API port 3100 and UI port 8180. It excludes inherited deployment,
cloud, and proxy credentials and disables direct P2P and external STUN. It does not
start a cluster or log into AWS. Set `TF_CLI_CONFIG_FILE` to an explicit provider
mirror or development override if needed; otherwise it defaults to `/dev/null`.
Do not run a second instance on these ports or reset an existing demo database.

## Import the workspace template

Create an archive containing only the four template files:

```bash
mkdir -p examples/kubernetes-shared-resources/.local
tar -C examples/kubernetes-shared-resources/legacy/serpent-dev-template \
  -cf examples/kubernetes-shared-resources/.local/serpent-dev.tar \
  main.tf startup.sh .terraform.lock.hcl README.md
```

Upload it through Coder's template creation UI. Use slug `serpent-dev`, display
name `serpent dev`, and icon `/emojis/1f40d.png`. Keep template variables
`namespace=workspaces` and `kubeconfig_context=default`. The template owns each
workspace's pod and PVC, not the shared namespace. Stop removes the pod; delete
removes the PVC too. Clone, Go dependency setup, and linter installation are
explicitly declared in the uploaded startup script. No custom image or manual
image import is required.

## Create and publish the policy

In **Templates > Network Policies**, create `serpent-dev` and attach the template.
Paste `serpent-dev-policy.hcl` into the HCL editor. Before saving, replace its
control-plane CIDR with the exact pod-reachable Coder IPv4 address used above.
The local embedded relay uses that same address and port. Do not broaden the
CIDR to compensate for an incorrect route.

Add a target with context `default`, namespace `workspaces`, and no custom
provisioner tags. For this demo, use the `kube-system` namespace UID as the
cluster identifier:

```bash
kubectl --kubeconfig "$DEMO_KUBECONFIG" --context default \
  get namespace kube-system -o jsonpath='{.metadata.uid}{"\n"}'
```

This is an operator-supplied identity, not K3s cluster discovery. The current
implementation does not verify the declared identity or persist custom target
names; the UI displays cluster/context/namespace. Publish the policy and wait
for its native Terraform job to succeed before creating a workspace. State,
immutable versions, publication, drift plans, and deletion belong to Coder, not
an operator-run Terraform directory. Do not directly apply a Cilium manifest.

## Verify and demonstrate

Create a temporary workspace, wait for startup, and connect through Coder SSH or
the web terminal. Startup can take several minutes on the first cold build.
From `/workspace/serpent`:

```bash
make lint
go build ./...
curl --fail https://github.com
curl --connect-timeout 4 --max-time 6 https://example.com
curl --connect-timeout 4 --max-time 6 http://github.com
curl --connect-timeout 4 --max-time 6 https://1.1.1.1
```

The unchanged pinned Serpent source has nine known lint findings: five errcheck,
three loopclosure, and one SA1019. `make lint` exits 2; this is not a clean-lint
claim. `go build ./...` and approved GitHub HTTPS should succeed. The other
network probes should fail. Correlate failures with Hubble policy-denied/DNS
rejection evidence rather than assuming every timeout proves enforcement.
Only GitHub, the Go proxy, and checksum service are allowed on HTTPS, plus trusted
DNS and the explicit Coder exception. Node image pulls are outside pod policy.

To verify persistence, create a marker under `/workspace`, stop and start through
Coder, and confirm the marker, checkout, linter, and caches survive while the pod
UID changes. Delete the temporary workspace through Coder when done; retain the
template, policy, and shared namespace for recording.

Publication and rollback use native policy jobs independently of workspace
builds. Terraform success means Applied, not verified dataplane enforcement.
Allow policies compose additively; preserve and inspect unrelated cluster rules.
Destination/port/DNS restrictions do not inspect payloads or prevent misuse of
approved services. Container escapes and host-equivalent privileges are outside
this demo's trust boundary.

## Offline validation

```bash
terraform -chdir=examples/kubernetes-shared-resources/template/modules/coder-network-policy init -backend=false -input=false -lockfile=readonly
terraform -chdir=examples/kubernetes-shared-resources/template/modules/coder-network-policy fmt -check -recursive
terraform -chdir=examples/kubernetes-shared-resources/template/modules/coder-network-policy validate
terraform -chdir=examples/kubernetes-shared-resources/template/modules/coder-network-policy test
terraform -chdir=examples/kubernetes-shared-resources/legacy/serpent-dev-template init -backend=false -input=false -lockfile=readonly
terraform -chdir=examples/kubernetes-shared-resources/legacy/serpent-dev-template validate
bash -n examples/kubernetes-shared-resources/legacy/serpent-dev-template/startup.sh
shellcheck examples/kubernetes-shared-resources/legacy/serpent-dev-template/startup.sh
```

Module tests mock the provider and do not access a cluster. The native Go and
frontend tests live alongside the implementation.

## Known limitations

UNKNOWN recovery is not implemented. Native deletion refuses quarantined
policies; a clean plan, namespace deletion, or returned checkpoint bytes are not
proof of recovery. The stale original demo policy was forcibly soft-deleted from
the local database at the user's request after its namespaces were removed.
That unsafe demo-only bypass is not part of this setup or a supported workflow.
Other remaining gaps include template/target eligibility, cluster-identity
verification, retry budgets/alerts, complete audit integration, and production
hardening.
