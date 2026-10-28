---
display_name: KubeVirt workspace prototype
description: A Linux VM with local Docker, persistent storage, and a Coder agent
icon: /icon/kubernetes.svg
maintainer_github: david-fraley
tags: [kubernetes, vm, docker]
---

# KubeVirt workspace prototype

**Experimental, not a supported production template.** The workspace, Docker
Engine, and Coder agent run together inside a full Linux VM managed by KubeVirt.
This tests ordinary Docker behavior without a remote Docker daemon, file-sync
helper, host Docker socket, or Sysbox. See [the evidence](EVIDENCE.md) for what
actually ran and what remains untested.

## Lifecycle and trust boundaries

A standalone CDI DataVolume owns the persistent root disk, including
`/home/coder` and `/var/lib/docker`. Starting creates a VM and fresh agent
credentials. Stopping deletes the VM and its Secrets, but retains the disk.
Deleting the Coder workspace deletes the disk too.

The guest's firmware UUID and cloud-init identity stay stable. Cloud-init installs
packages and a small bootstrap loader once. Each new VM receives the current
bootstrap script, agent initialization script, and token on a read-only Secret
disk. The bootstrap writes the agent service and credentials under `/run`, so
updates do not depend on cloud-init re-running or a token cached on the root disk.
Changing a Secret does not update a running VM's disk: stop and start to apply it.
Base-image and first-boot loader updates still need an explicit disk migration.

The workspace owner has sudo and Docker access inside their VM. This is not an
agent-task sandbox. Cluster administrators and trusted template administrators
can read credentials and attach disks in the organization namespace. This
prototype adds no tenant NetworkPolicy, metadata-service filtering, or admission
policy restricting volume references. A VM boundary alone does not make those
policies unnecessary.

## Prerequisites

Use a dedicated, disposable Linux test machine or VM, not a customer or shared
cluster. KubeVirt installs privileged, cluster-wide components. Platform and
security teams must approve those components and the node/storage configuration
before any production pilot. This is not a Pod Security Restricted template.

The lab uses these explicit versions:

| Component                     | Version                               |
|-------------------------------|---------------------------------------|
| kind                          | v0.33.0                               |
| Kubernetes node               | v1.36.4                               |
| KubeVirt and virtctl          | v1.9.1                                |
| CDI                           | v1.66.1                               |
| Terraform Kubernetes provider | 3.4.0                                 |
| Terraform Coder provider      | 2.19.0                                |
| Guest image                   | Ubuntu 24.04, release-20261017, amd64 |

KubeVirt 1.9's [compatibility matrix](https://github.com/kubevirt/sig-release/blob/main/releases/k8s-support-matrix.md)
includes Kubernetes 1.36. The template's explicit `pci-topology-version: v3`
annotation matches KubeVirt v1.9.1 and prevents an observed Kubernetes provider
state-type mismatch. Recheck creation and drift when upgrading either component.
Do not use kind's unpinned default node image. A real
KVM-capable runner is preferable; software-emulated runs cannot establish
production performance or cost. Record the actual accelerator, not just a
configuration flag.

The provisioner needs access to the cluster during Terraform planning, including
read-only CRD discovery. [provisioner-role.yaml](provisioner-role.yaml) grants
workload writes only in `coder-kubevirt-prototype`, plus that schema discovery.
Use a separate namespace and provisioner identity for each Coder organization.
The example Role and RoleBinding names must match the `namespace` variable.
Keep kubeconfigs, tokens, Terraform state, and private keys outside this directory.

## Create the disposable cluster

Install Docker, kind, kubectl, Terraform, and Python 3.10 or later on the test
machine first. Keep
administrative credentials in a private directory and use the explicit kubeconfig
for every command. The following creates a new cluster; it does not install
operators into your current Kubernetes context.

```bash
umask 077
LAB=$(mktemp -d /tmp/coder-kubevirt.XXXXXX)
CLUSTER="coder-kv-$(openssl rand -hex 6)"
printf '%s\n' "$CLUSTER" > "$LAB/cluster-name"
kind create cluster --name "$CLUSTER" --kubeconfig "$LAB/admin.kubeconfig" \
  --image kindest/node:v1.36.4@sha256:099e049362a1526b2db71494e1947aae99bd16290d7c895f2b7ea312e3cbfaed \
  --wait 15m
```

Download the versioned operator manifests from the upstream releases, inspect
them, and verify the hashes before applying them:

```bash
curl -fL -o "$LAB/kubevirt-operator.yaml" https://github.com/kubevirt/kubevirt/releases/download/v1.9.1/kubevirt-operator.yaml
curl -fL -o "$LAB/kubevirt-cr.yaml" https://github.com/kubevirt/kubevirt/releases/download/v1.9.1/kubevirt-cr.yaml
curl -fL -o "$LAB/cdi-operator.yaml" https://github.com/kubevirt/containerized-data-importer/releases/download/v1.66.1/cdi-operator.yaml
curl -fL -o "$LAB/cdi-cr.yaml" https://github.com/kubevirt/containerized-data-importer/releases/download/v1.66.1/cdi-cr.yaml
(cd "$LAB" && sha256sum -c - <<'SUMS'
9ac45a436c49d32e6fc21d542bcb7600b970705297b9e6752f3ef9b0a1d292da  kubevirt-operator.yaml
43106136dbce3312bdbfdeae612aacafc6c12da518d233f90645b4685d84a2af  kubevirt-cr.yaml
c7d92bd1b2ee1a3952a5990000437c08d58109f31c0bd48ab6188ec27a13e224  cdi-operator.yaml
a497f90de608c1df26f9ee4095289373f17132d74a2042ca03e00c17c964f8a7  cdi-cr.yaml
SUMS
)
kubectl --kubeconfig "$LAB/admin.kubeconfig" apply -f "$LAB/kubevirt-operator.yaml"
kubectl --kubeconfig "$LAB/admin.kubeconfig" -n kubevirt rollout status deployment/virt-operator --timeout=20m
kubectl --kubeconfig "$LAB/admin.kubeconfig" apply -f "$LAB/kubevirt-cr.yaml"
kubectl --kubeconfig "$LAB/admin.kubeconfig" apply -f "$LAB/cdi-operator.yaml"
kubectl --kubeconfig "$LAB/admin.kubeconfig" -n cdi rollout status deployment/cdi-operator --timeout=20m
kubectl --kubeconfig "$LAB/admin.kubeconfig" apply -f "$LAB/cdi-cr.yaml"
kubectl --kubeconfig "$LAB/admin.kubeconfig" -n kubevirt wait kubevirt/kubevirt --for=condition=Available --timeout=45m
kubectl --kubeconfig "$LAB/admin.kubeconfig" wait cdi/cdi --for=condition=Available --timeout=45m
kubectl --kubeconfig "$LAB/admin.kubeconfig" create namespace coder-kubevirt-prototype
kubectl --kubeconfig "$LAB/admin.kubeconfig" label namespace coder-kubevirt-prototype pod-security.kubernetes.io/enforce=privileged
kubectl --kubeconfig "$LAB/admin.kubeconfig" apply -f provisioner-role.yaml
```

Use the ServiceAccount from that file to create a private provisioner kubeconfig
with a short-lived token. Do not give the Coder provisioner the lab's admin
kubeconfig. The test harness separately needs admin access for VM inspection.

The storage class must support filesystem ReadWriteOnce PVCs. The lab uses kind's
`standard` local-path class, which is not durable across node/cluster deletion and
is not a production storage recommendation. Keep CDI DataVolume TTL cleanup off
for these persistent workspace disks. The VM consumes the DataVolume without
waiting for import in Terraform, allowing WaitForFirstConsumer storage to bind.

## Create a Coder workspace

For a local Coder checkout, start the server with `./scripts/develop.sh` using a
free port and a guest-reachable access URL. The default loopback access URL is not
reachable from a guest. The guest's `guest_network_cidr` must not overlap that URL,
the Kubernetes networks, or other destinations it needs. For a nested QEMU lab,
KubeVirt's usual `10.0.2.0/24` guest network can collide with QEMU's host gateway;
this template uses a configurable, separate guest subnet.

Create a private variables file outside the template directory. Set
`kubeconfig_path` to the provisioner kubeconfig on the provisioner machine:

```yaml
namespace: coder-kubevirt-prototype
storage_class: standard
kubeconfig_path: /absolute/private/path/provisioner.kubeconfig
image_url: https://cloud-images.ubuntu.com/releases/noble/release-20261017/ubuntu-24.04-server-cloudimg-amd64.img
image_sha256: 04e8f2461a16a44e78a370284471f29616cdd9baf1893df25807b71f985b6c94
```

CDI verifies the image checksum before import. Docker, Compose, and buildx come
from the Ubuntu archive; this prototype does not freeze their package versions.
For an emulated CPU, select a `cpu_model` advertised by the node rather than
inventing a node label. The attempted lab run used the advertised `Nehalem`
model. Leave `ssh_public_key` empty unless recovery access is needed.
A supplied first-boot recovery key persists on the root disk; clearing the
variable does not revoke it. Delete lab disks rather than promoting them to
production.

With a CLI logged into the isolated Coder deployment:

```bash
terraform init -backend=false
terraform validate
coder templates push kubevirt-prototype --directory . --variables-file /absolute/private/path/variables.yaml
coder create kv-workspace --template kubevirt-prototype --parameter memory=2 --parameter cpu=2 --parameter disk_size=24
coder ssh kv-workspace -- docker info
```

A successful Terraform apply alone is not a working workspace. Wait for the Coder
agent to connect and finish its blocking Docker startup check. For a failed boot,
inspect VM/VMI conditions, importer logs, and the guest console. The optional
recovery key can be used through `virtctl port-forward` over the authenticated
Kubernetes API; no public SSH Service is created.

## Run the live checks

`tests/workloads.sh` runs Docker builds, live bind mounts, generated files,
Compose networking, a named volume, Docker restart continuity, and explicit
negative controls inside the VM through Coder. `tests/lifecycle.py` adds actual
KubeVirt and Coder checks around stop/start, credential replacement, source
provenance, and disk deletion. It does not accept manifest rendering as success.

The lifecycle harness requires a loopback Coder URL and a dedicated kind context
whose name starts with `kind-coder-kv-`. Run the phases once, in order, against the
same workspace, source tree, and private evidence directory. The final phase
**deletes the workspace and its disk**.

```bash
for phase in prepare stop restart delete; do
  python3 tests/lifecycle.py "$phase" \
    --state-dir "$LAB/evidence" \
    --coder-config /absolute/private/path/coder-config \
    --coder-binary /absolute/path/to/coder \
    --url http://127.0.0.1:38623 \
    --kubeconfig "$LAB/admin.kubeconfig" || break
done
```

Only `complete: true` with all four successful phases is a completed run. A failed
run remains failed; preserve it and use a fresh fixture for a rerun. Review and
sanitize evidence before sharing. Never publish the whole lab directory, a
kubeconfig, Terraform state, or VM disk.

After verifying that the recorded name still identifies only this experiment,
remove the disposable cluster with `kind delete cluster --name "$CLUSTER"
--kubeconfig "$LAB/admin.kubeconfig"`. Stop any local Coder server and SSH forwards
you started, then remove only your generated lab credentials and disks.

Generated by Coder Agents. A
[community KubeVirt template](https://github.com/sulo1337/coder-kubevirt-template)
listed in this repository was consulted as a reference; this prototype uses current providers, independent
persistent storage, and a per-start bootstrap disk.
