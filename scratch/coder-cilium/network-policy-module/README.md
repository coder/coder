# Native network policy module POC

This trusted module owns exactly one namespace-scoped `cilium.io/v2`
`CiliumNetworkPolicy`, via `hashicorp/kubernetes` `kubernetes_manifest`.
It creates no namespaces, workspace resources, credentials, API, or UI.
Terraform is the only writer of the policy. Never apply an exported manifest
with kubectl alongside Terraform.

## Module contract

Terraform CLI is pinned by `.terraform-version` to **1.16.6** (the installed
version); supported constraint is `>= 1.9.0, < 2.0.0`. Kubernetes provider is
pinned to **2.38.0**, with checksums in `.terraform.lock.hcl`. The module has no
provider configuration or backend: the trusted runner supplies credentials,
provider configuration, isolated state, and locking. It works as a root module
with injected variables, or as the bundled child at `./module`.

Coder owns and injects these required inputs:

| Input | Type | Meaning |
| --- | --- | --- |
| `policy_id` | `string` | Immutable lowercase DNS label, at most 47 characters; UUIDs work. |
| `namespace` | `string` | Existing target namespace, not chosen by the author. |
| `template_ids` | `set(string)` | Stable template IDs attached to the policy. |

Static user inputs:

| Input | Type | Default |
| --- | --- | --- |
| `allowed_https_hosts` | `set(string)` | `[]` |
| `dns_query_names` | `set(string)` | `[]`, additional exact dependency DNS queries only |
| `control_plane_allowances` | `list(allowance)` | `[]` |
| `relay_allowances` | `list(allowance)` | `[]` |
| `direct_p2p_enabled` | `bool` | **`true`** |
| `direct_p2p_cidrs` | `set(string)` | `["0.0.0.0/0", "::/0"]` |

An `allowance` is an object with `endpoint_selectors = list(map(string))`
(default `[]`), `cidrs = set(string)` (default `[]`), and required nonempty
`ports = list(object({ port = number, protocol = optional(string, "TCP") }))`.
Exactly one of selectors/CIDRs must be nonempty. Each selector must include
`k8s:io.kubernetes.pod.namespace` and at least one workload label. Selector
entries are ORed; labels within an entry are ANDed. Ports are integers
1..65535, TCP or UDP; port 53 is reserved for the trusted DNS rule and rejected.
CIDR rules are for external addresses, not a replacement for endpoint selectors
for Cilium-managed cluster pods. No service, entity, wildcard, arbitrary
manifest, or arbitrary selector-expression input is supported.

Outputs: `policy_name` and `manifest`, both null when no templates are attached.
Resource address: `kubernetes_manifest.policy["active"]` (prefix with the module
address when bundled). Name: `coder-policy-${policy_id}`. Namespace changes
move the target resource and must be controlled by Coder, not policy authors.

### Pod identity and detachment

The selector uses `In` on **`k8s:coder.template_id`** with the attached IDs.
The `k8s:` prefix is Cilium's label source; Kubernetes pods must carry the actual
label **`coder.template_id=<stable-template-id>`**. Existing
`examples/templates/kubernetes/main.tf` uses `com.coder.workspace.id` and
`com.coder.workspace.name`, not a stable template label. This module therefore
requires template pod-label integration; it does not assume those labels
already exist. Label the pod template, not just Deployment metadata. A workload
able to change its identity label can escape selection; admission/RBAC and
label ownership are prerequisites outside this module.

An empty `template_ids` produces an empty resource `for_each`. Applying this
transition deletes the previously managed policy, never creates an empty
selector and never selects every pod. Removing the policy also removes its
protection, so detach is not a quarantine operation. Coder must retain and reuse
the same state to destroy it, including during policy deletion. Do not discard
state or use `terraform state rm` as deletion.

### Supported HCL subset for the native command

The policy author supplies exactly one `module "coder_network_policy"` block with
literal `source = "./module"` and only the static inputs listed above. Values
must be static strings, numbers, booleans, lists, maps, and objects; reject
references, functions, interpolation, unknown/computed values, `count`,
`for_each`, `depends_on`, `providers`, injected input names, and other blocks.
No user-authored root `terraform`, `provider`, `resource`, `data`, `variable`,
`locals`, `output`, import, backend, provisioner, or external module source is
accepted. The native parser must validate against an explicit allowlist before
Terraform sees author content. Terraform variable validation is not an HCL
sandbox, and rejecting unknown object fields requires the parser (Terraform
object conversion can discard them).

The bundled runner supplies the trusted module bytes and provider pin, injects
identity/namespace/attachments, configures the provider from trusted target
credentials, and keeps state per organization/policy/target. The native validator
and immutable archive builder are implemented in `coderd/networkpolicy/`.
See `../serpent-dev-policy.hcl` for author HCL and `../NATIVE-DEMO.md` for setup.
Organization authorization and target ownership belong to Coder; they cannot
be delegated to arbitrary HCL.

## Network semantics

- Default-deny **both ingress and egress**, with established reply traffic
  handled by Cilium's connection tracking. No ingress permits with P2P disabled.
- DNS only to endpoints labeled `k8s-app=kube-dns` in `kube-system`, TCP/UDP 53.
  L7 `matchName` queries are the union of exact HTTPS hosts and
  `dns_query_names`. No wildcard query. Empty union means no DNS rule.
- HTTPS only TCP 443 with `toFQDNs.matchName` **and** the same `serverNames` TLS
  SNI list. Hosts must be lowercase exact DNS names without scheme, path,
  wildcard, IP literal, or trailing dot. Redirect hosts need separate entries.
  `google.com` does not allow `www.google.com`. DNS search-path expansions need
  their own exact entries; use absolute DNS queries when testing.
- Explicit dependency allowances are L3/L4 exceptions without SNI inspection.
  Restrict them to trusted server/relay endpoints and ports. Broad CIDRs or
  selectors can undermine HTTPS restrictions; do not allow arbitrary
  untrusted destinations here.
- **Direct P2P defaults enabled**, intentionally permitting external UDP
  1024..65535 ingress and egress, including STUN 3478, within
  `direct_p2p_cidrs`. This is a broad UDP exception, not WireGuard recognition.
  Arbitrary UDP protocols and DNS on nonstandard ports can pass there. Narrow
  peer CIDRs or explicitly disable it for the strict HTTPS subset, as the
  example does. UDP 53 and QUIC 443 are not included. Peers on privileged ports,
  in-cluster peers, UPnP/NAT-PMP discovery, and TCP direct transports are outside
  this subset. NAT and agent listen-port configuration still determine whether
  P2P actually works; this module does not configure Coder's `block_direct` or
  STUN settings. Disabling P2P here should be paired with deployment-side direct
  connection blocking and reachable DERP relays.

Requires a healthy Cilium installation with its CNP CRD, DNS/L7 proxy and Envoy;
CoreDNS must have the assumed labels. NodeLocal DNS needs a different module,
not a wider wildcard. Schema validation targets Cilium **1.20.3**, matching the
local bootstrap pin. Terraform `kubernetes_manifest` queries the live CRD schema
at plan time; offline mocked tests do not prove enforcement.

Policies are additive. Existing broad Kubernetes/Cilium allows can reopen
traffic, including the local cluster's existing `coder-restricted-egress` policy,
which matches `app.kubernetes.io/name=coder-workspace` and permits wildcard DNS.
Audit overlaps before relying on this new restriction; this POC
does not edit or remove that existing policy. Policies do not control node image
pulls, host-network workloads, or privileged escape. SNI is not encrypted HTTP
inspection: allowed services can tunnel traffic, shared allowed IPs/names are
not URL authorization, and ECH may not expose usable SNI.

## Offline checks

```bash
terraform -chdir=scratch/coder-cilium/network-policy-module init -backend=false -input=false
terraform -chdir=scratch/coder-cilium/network-policy-module fmt -check -recursive
terraform -chdir=scratch/coder-cilium/network-policy-module validate
terraform -chdir=scratch/coder-cilium/network-policy-module test
```

Tests mock the provider and never use cluster credentials. Live apply and
network probes require separate authorization and are not part of this task.
