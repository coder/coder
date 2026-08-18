---
display_name: AI sandbox (microVM)
description: An AI child agent confined in an embedded microVM
icon: ../../../site/static/icon/coder.svg
maintainer_github: coder
tags: [docker, ai, security, microvm, demo]
---

# AI sandbox in an embedded microVM

This template boots a managed AI sandbox directly from the Coder agent binary.
It does not install or invoke `coder-sandbox`, a microVM daemon, helper scripts,
or any other runtime binary. The workspace image can remain the plain
`codercom/enterprise-base:ubuntu` image.

Terraform creates one workspace container and one `coder_agent.main`. When the
agent sees `CODER_AI_SANDBOX_MICROVM=true`, it asks coderd to reconcile the
managed sandbox and child agent, downloads the microVM runtime and guest image
when needed, boots the guest, mounts its own executable at `/opt/coder`, and
starts the child agent inside the guest.

## Prerequisites

- A Linux amd64 Docker provisioner host with hardware virtualization enabled.
- `/dev/kvm` available to the Docker daemon and accessible to the workspace
  user. Find the device group ID on the Docker host:

  ```bash
  stat -c %g /dev/kvm
  ```

  Supply that value through the `kvm_gid` workspace parameter. The template
  mounts the device and adds the numeric GID as a supplemental group. The
  default `108` is common on Debian and Ubuntu, but it is not portable.
- Outbound network access from the workspace container on first boot. The agent
  downloads the embedded microVM runtime and the configured OCI guest image.

No additional executable is required in the workspace image. The Coder agent
binary carries the sandbox integration and is also the binary launched inside
the guest.

## Persistent first-boot cache

The template mounts a persistent volume at `/home/coder`. The agent stores
runtime artifacts and OCI image data below:

```text
~/.config/coder-ai/microvm/cache
~/.config/coder-ai/microvm/state
```

A cold workspace start needs network access and can take several minutes. Once
an artifact or image is cached successfully, later workspace restarts reuse it
from the home volume.

## Configuration

| Input | Default | Purpose |
|---|---:|---|
| `kvm_gid` parameter | `108` | Numeric group ID that owns `/dev/kvm` on the Docker host |
| `workspace_image` variable | `codercom/enterprise-base:ubuntu` | Container image that runs the host Coder agent |
| `sandbox_image` variable | `ubuntu:24.04` | Linux amd64 OCI image booted as the guest |
| `sandbox_memory_mib` variable | `1024` | Guest memory in MiB |
| `sandbox_cpus` variable | `1` | Guest virtual CPU count |

The template declares `CODER_AI_SANDBOX_EGRESS_ENFORCEMENT=forced`. This is an
administrator attestation recorded by Coder. Embedded mode makes that claim
defensible because the platform starts the microVM and installs its gateway
proxy instead of trusting an external runtime script to do so.

## Live policy flow

```text
Coder UI or API policy edit
  -> coderd policy revision
  -> SSE to coder_agent.main
  -> shared in-memory PolicyEngine
  -> in-process microVM gateway proxy
  -> immediate guest enforcement
```

There is no exported policy file, descriptor rendering, reload hook, daemon
watch loop, or 500 millisecond polling interval. The SSE watcher updates the
same `PolicyEngine` read by the embedded proxy, so a complete policy revision is
used immediately for new requests.

## Enforcement model

The gateway proxy runs inside the host Coder agent process and is attached to
the private network created for the embedded microVM. The guest does not use the
parent loopback proxy listener used by create-script and proxy-only modes. It
can reach the dedicated gateway listener supplied by the embedded runtime.

This differs from the former external-daemon example. In that arrangement,
Coder exported policy to a file and trusted `coder-sandbox` plus template
scripts to create the boundary and reload policy. In embedded mode, the Coder
agent owns the VM boot configuration, proxy evaluator, event recorder, child
agent launch, and shutdown. Network events flow through the normal Coder event
batcher and can be attributed to the managed child agent.

The controller does not automatically change an administrator's egress
attestation. This template explicitly declares `forced`; custom templates must
choose their own value.

## Try it

Push the template and provide the KVM group ID from the Docker host:

```bash
KVM_GID=$(stat -c %g /dev/kvm)
coder templates push ai-sandbox-microvm \
  -d examples/templates/ai-sandbox-microvm
coder create ai-microvm \
  --template ai-sandbox-microvm \
  --parameter "kvm_gid=${KVM_GID}"
```

The managed child agent is named `microvm`, so it is addressable as
`ai-microvm.microvm` after it connects.

## Operator validation checklist

MicroVM boot is gated on KVM and is not exercised by the ordinary CI suite. An
operator with usable `/dev/kvm` access should validate all of the following:

1. **KVM access.** Connect to `ai-microvm.main` and confirm that the workspace
   user can open `/dev/kvm` read-write.

   ```bash
   coder ssh ai-microvm.main
   test -r /dev/kvm && test -w /dev/kvm && echo "KVM accessible"
   ```

2. **Guest boot.** Inspect the host agent logs for `embedded AI sandbox microVM
   started`. A failure is reported as degraded while the host workspace remains
   available.
3. **Child agent connection.** Run `coder show ai-microvm` and confirm that the
   `microvm` child agent is connected. Then connect with:

   ```bash
   coder ssh ai-microvm.microvm
   ```

4. **Allowed request.** In the Coder egress policy UI, allow a test host and
   port. From the child-agent terminal, run a request such as:

   ```bash
   curl -I --max-time 10 https://allowed.example
   ```

5. **Denied request.** From the same terminal, request a host or port that is
   not allowed and confirm the proxy rejects it:

   ```bash
   curl -I --max-time 10 https://denied.example
   ```

6. **Live policy flip.** Change the allowlist in the UI without restarting the
   workspace or guest. Repeat both requests and confirm that the new decision
   applies immediately.
7. **Shutdown.** Stop the workspace and confirm the controller closes the
   embedded VM without leaving runtime state for a running guest.

The default Ubuntu guest may not include `curl`. For the HTTP checks, select a
Linux amd64 guest image that already contains it. This is a validation tool in
the guest image, not a host runtime dependency.

## Validation status

Terraform formatting and provider validation are exercised without KVM. The
embedded controller and policy wiring have unit coverage with a fake VM, while
the real boot smoke test skips unless `/dev/kvm` can be opened read-write. An
end-to-end guest boot, child connection, and live allow or deny flip therefore
remain operator-validated on a KVM-capable host.
