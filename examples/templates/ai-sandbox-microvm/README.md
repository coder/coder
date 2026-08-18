---
display_name: AI sandbox (microVM)
description: An AI child agent confined in a coder/sandbox microVM
icon: ../../../site/static/icon/coder.svg
maintainer_github: coder
tags: [docker, ai, security, microvm, demo]
---

# AI sandbox in a coder/sandbox microVM

This template runs one ordinary Coder agent in a Docker workspace. That host
agent asks coderd to create a managed AI sandbox and child agent, then boots the
child inside a `coder/sandbox` microVM. Terraform declares only
`coder_agent.main`; coderd creates the child agent with the correct parent and
AI identity.

The template demonstrates live policy delivery to an external sandbox runtime.
It does not claim that Coder can attest the microVM boundary or force all guest
egress itself.

## Requirements

- A Linux amd64 Docker provisioner host with hardware virtualization enabled.
- `/dev/kvm` available to the Docker daemon.
- The numeric group ID that owns `/dev/kvm`. Find it on the Docker host:

  ```bash
  stat -c %g /dev/kvm
  ```

  Enter that value for the `kvm_gid` workspace parameter. The default `108` is
  common on Debian and Ubuntu, but it must not be assumed correct for another
  host.
- A workspace image containing `coder-sandbox`, `bash`, and a static Linux
  amd64 `coder` binary on `PATH`. The default image documents the expected base
  image but does not include `coder-sandbox`; build a derived image before using
  the template:

  ```dockerfile
  FROM codercom/enterprise-base:ubuntu
  COPY coder-sandbox /usr/local/bin/coder-sandbox
  RUN sudo chmod 0755 /usr/local/bin/coder-sandbox
  ```

- An OCI guest image for `linux/amd64` with `/bin/sh`. The default is
  `ubuntu:24.04`.

The Docker container mounts `/dev/kvm` and adds the supplied group ID as a
supplemental group. The persistent `/home/coder` volume retains
`~/.config/coder-sandbox`, including daemon state, runtime artifacts, and image
cache.

### First boot network access

A cold `coder-sandbox up` may download its VM runtime and the OCI rootfs. The
sandbox controller allows five minutes for the create script. Slow or restricted
networks can exceed that limit. Preinstall or prewarm the runtime and guest image
when possible. The persistent home volume avoids repeating successful downloads
on later workspace starts.

These downloads are made by the host workspace, before guest policy is relevant.
The host workspace therefore needs ordinary network access to the configured
artifact and image registries.

## Live policy flow

```text
coderd policy revision
  -> SSE to coder_agent.main
  -> atomic runtime-network.yaml replacement
  -> sandbox-reload-policy.sh rebuilds the full descriptor
  -> coder/sandbox watches that descriptor every 500ms
  -> valid runtime policy is atomically applied to the guest proxy
```

`coder/sandbox up NAME -f descriptor.yaml` registers the descriptor path itself.
With `runtime.network.reload: watch`, the daemon polls that descriptor, parses
only its `runtime` section, and applies the shared network and MCP policy
snapshot. Changes under `sandbox` do not alter a running VM.

The descriptor format cannot include an external `runtime.network` file. The
reload hook therefore indents the controller-owned policy document into a newly
rendered descriptor. The initial policy file and descriptor are written before
the create script boots the VM.

## Policy translation

The host agent exports this `runtime.network` shape:

```yaml
reload: watch
default: deny
mode: enforce
rules:
  - host: api.example.com
    ports: [443]
    action: allow
    tls: passthrough
```

Translation differences are security relevant:

| Coder egress policy | coder/sandbox document |
|---|---|
| Empty port list | Explicit ports `80` and `443` |
| `*.example.com` | Passed through unchanged, but coder/sandbox matches arbitrary subdomain depth rather than exactly one label |
| TLS handling | Every rule uses `tls: passthrough`; the guest keeps end-to-end TLS |
| IP literal | Exact `/32` or `/128` CIDR rule |
| Coder access URL | Host rule on the access URL port, plus exact CIDRs for private or loopback resolutions |
| Default and mode | Always `deny` and `enforce` |

The access URL CIDRs are required because the coder/sandbox proxy performs a
second policy decision when a hostname resolves to a private or loopback
address.

## Lifecycle

The controller invokes the policy reload hook before the create script. The
create script then:

1. Verifies `coder-sandbox`, the static `coder` binary, and read-write KVM
   access.
2. Runs `coder-sandbox down` to discard stale state. The daemon does not resume
   VMs after daemon restarts.
3. Renders the descriptor again and runs `coder-sandbox up NAME -f ...`.
4. Bind-mounts the static `coder` binary read-only at `/opt/coder`.
5. Uses `coder-sandbox ssh NAME -- ...` and `setsid` to launch the child agent
   with the server-minted child token.

On workspace stop, the destroy script runs `coder-sandbox down NAME`.

## What is and is not attested

This template deliberately does not set Terraform `ai_bound` or
`egress_enforcement` fields. Managed-sandbox mode creates the AI child in coderd,
and the session reports `egress_enforcement: none`. The network boundary is
implemented externally by `coder/sandbox`, not forced or verified by the Coder
platform.

`coder/sandbox` prevents ordinary guest IPv4 egress from bypassing its proxy,
but its VM firewall forwards IPv6 and other non-IPv4 EtherTypes without the same
policy parsing. This is a known enforcement gap, so the template must not claim
platform-forced confinement.

The Coder confine proxy and event batcher still run on the host, but the microVM
guest cannot use that proxy. Guest requests are recorded by `coder/sandbox`.
They are not yet retained as Coder AI sandbox network events.

## Validation status

The template was not booted end to end in the development workspace where it
was added. On that host, `/dev/kvm` is mode `0660`, owned by group ID `65534`,
and UID `1000` cannot open it read-write. Terraform validation, shell checks,
and descriptor rendering were exercised without starting a VM.

An operator with KVM access must complete the guest boot and live-policy checks
below before relying on the demo.

## Try it

Push the template and supply the actual KVM group ID:

```bash
KVM_GID=$(stat -c %g /dev/kvm)
coder templates push ai-sandbox-microvm \
  -d examples/templates/ai-sandbox-microvm
coder create ai-microvm \
  --template ai-sandbox-microvm \
  --parameter "kvm_gid=${KVM_GID}"
```

Check the host-side daemon and guest:

```bash
coder ssh ai-microvm -- 'coder-sandbox ls'
coder ssh ai-microvm -- 'coder-sandbox troubleshoot'
```

Change the template AI egress policy without rebuilding the workspace, then
inspect the exported policy and descriptor:

```bash
coder ssh ai-microvm -- \
  'cat ~/.config/coder-sandbox/coder-ai/runtime-network.yaml'
coder ssh ai-microvm -- \
  'grep -A30 "^runtime:" ~/.config/coder-sandbox/coder-ai/coder-*.yaml'
```

For an end-to-end host validation, confirm all of the following:

1. The workspace container can open `/dev/kvm` read-write as the `coder` user.
2. `coder-sandbox up` boots the guest and `coder-sandbox ls` reports it running.
3. The managed child agent connects and appears under the workspace.
4. An allowed HTTPS request succeeds from `coder-sandbox ssh NAME -- ...`.
5. A denied host or port fails.
6. Editing the Coder egress policy changes the descriptor and guest behavior
   without rebuilding or rebooting the workspace.
