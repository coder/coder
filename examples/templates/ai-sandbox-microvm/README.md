---
display_name: AI sandbox (microVM)
description: A Terraform-declared AI agent running inside an embedded microVM
icon: ../../../site/static/icon/coder.svg
maintainer_github: coder
tags: [docker, ai, security, microvm, demo]
---

# AI sandbox in an embedded microVM

This template declares two Coder agents on one Docker workspace resource:

- `main` is the ordinary host agent. It runs workspace scripts and boots the
  embedded microVM.
- `ai` is declared with `ai_bound = true` and
  `egress_enforcement = "forced"`. The Coder agent process for this declaration
  runs inside the microVM.

Both agents exist as soon as Terraform completes the build, so the workspace UI
shows the intended topology even when KVM access or guest boot fails. A
non-blocking `coder_script` starts `coder agent sandbox` in the background and
writes its foreground output to `/tmp/coder-agent-sandbox.log`.

The workspace image does not need `coder-sandbox`, a microVM daemon, or helper
scripts. The host Coder binary contains the embedded runtime integration and is
also mounted into the guest as its agent binary.

## Provider development override

The `ai_bound` and `egress_enforcement` arguments require commit `4791eb1` from
the `ai-agent-identity` branch of `github.com/coder/terraform-provider-coder`
until a provider release contains them.

On the provisioner host, including `jon-kvm`, build the provider as the same user
that runs the Coder provisioner:

```bash
git clone https://github.com/coder/terraform-provider-coder.git
cd terraform-provider-coder
git checkout ai-agent-identity
git checkout 4791eb1
go build -o terraform-provider-coder
```

Add a development override to that user's `~/.terraformrc`. Replace
`/home/YOU/terraform-provider-coder` with the absolute path printed by `pwd` in
the provider checkout:

```hcl
provider_installation {
  dev_overrides {
    "coder/coder" = "/home/YOU/terraform-provider-coder"
  }
  direct {}
}
```

For example, if the checkout is `/home/coder/terraform-provider-coder`, the
exact override is:

```hcl
provider_installation {
  dev_overrides {
    "coder/coder" = "/home/coder/terraform-provider-coder"
  }
  direct {}
}
```

Restart the provisioner after changing `~/.terraformrc`. Terraform prints a
warning when a development override is active. That warning is expected.

## Prerequisites

- Coder server code from the `ais` branch.
- The provider development override described above.
- A Linux amd64 Docker provisioner host with hardware virtualization enabled.
- `/dev/kvm` available to Docker and accessible to the workspace user. Find the
  device group ID on the Docker host:

  ```bash
  stat -c %g /dev/kvm
  ```

  Supply that value through the `kvm_gid` workspace parameter. The template
  mounts `/dev/kvm` and adds the numeric GID as a supplemental group. The
  default `108` is common on Debian and Ubuntu, but it is not portable.
- Outbound network access from the workspace container on first boot. The
  embedded runtime and configured OCI guest image are downloaded on demand.

The default `codercom/enterprise-base:ubuntu` image already provides the shell
and `nohup` used by the startup script. No additional sandbox executable is
required.

## How startup works

The Docker container references both agent tokens:

```text
CODER_AGENT_TOKEN             -> coder_agent.main.token
CODER_SANDBOX_AGENT_TOKEN     -> coder_agent.ai.token
```

These references associate both Terraform agents with the same container
resource. The container starts `coder_agent.main`. Its startup script inherits:

- `CODER_SANDBOX_AGENT_TOKEN`, the token passed to the guest agent,
- `CODER_AGENT_URL`, the normal agent control-plane URL, and
- `CODER_AGENT_TOKEN`, which the running host agent replaces with its current
  session token for every workspace command.

`coder agent sandbox` uses the host token only to fetch the initial egress policy
and watch policy revisions over SSE. It uses the sandbox token only for the
agent launched inside the guest.

The startup script first resolves `/proc/$PPID/exe`. Agent scripts with a
shebang are executed as a direct child shell of the host agent, so this path
selects the exact version-matched binary already running the workspace. If that
path does not look like a Coder executable, the script falls back to
`command -v coder`. The default workspace image provides `coder` on `PATH`.

The script uses `nohup` so it can return without blocking login. It records the
process ID in `/tmp/coder-agent-sandbox.pid` and appends output to:

```text
/tmp/coder-agent-sandbox.log
```

Inspect the process from the host agent:

```bash
coder ssh ai-microvm.main
cat /tmp/coder-agent-sandbox.pid
ps -fp "$(cat /tmp/coder-agent-sandbox.pid)"
tail -f /tmp/coder-agent-sandbox.log
```

Expected startup messages include the initial policy rule count, runtime and
image provisioning, microVM boot, and guest-agent launch. A missing KVM device,
unsupported platform, or guest boot failure appears directly in this log.

## What appears in the workspace UI

After the build completes, the container resource lists two agents:

- `main` connects as the host workspace agent.
- `ai` is visible immediately as the AI-bound agent and connects after the
  microVM launches its guest process.

The build-time binding performed by Coder attaches `ai` to the workspace-origin
AI identity. It does not designate the workspace, and `main` remains an ordinary
human-owned agent.

## Persistent first-boot cache

The template mounts a persistent volume at `/home/coder`. The sandbox command
uses these default locations:

```text
~/.config/coder-ai/microvm/cache
~/.config/coder-ai/microvm/state
```

A cold start can take several minutes while runtime artifacts and the guest OCI
image are downloaded. Later workspace starts reuse successful downloads from
the persistent home volume.

## Configuration

| Input                         |                           Default | Purpose                                                      |
|-------------------------------|----------------------------------:|--------------------------------------------------------------|
| `kvm_gid` parameter           |                             `108` | Numeric group ID that owns `/dev/kvm` on the Docker host     |
| `workspace_image` variable    | `codercom/enterprise-base:ubuntu` | Container image that runs the host agent and sandbox command |
| `sandbox_image` variable      |                    `ubuntu:24.04` | Linux amd64 OCI image booted as the guest                    |
| `sandbox_memory_mib` variable |                            `1024` | Guest memory in MiB                                          |
| `sandbox_cpus` variable       |                               `1` | Guest virtual CPU count                                      |

## Live policy flow

```text
Coder UI or API policy edit
  -> coderd policy revision
  -> host agent token authenticates policy fetch and SSE
  -> shared in-memory PolicyEngine
  -> in-process microVM gateway proxy
  -> immediate guest enforcement
```

The policy engine starts deny-default. If the initial fetch fails, the command
logs the error loudly and still boots the guest with only the implicit
control-plane destination allowed. A later successful SSE revision atomically
replaces the active policy.

There is no exported policy file, daemon reload hook, or polling loop. The
embedded gateway reads the same `PolicyEngine` updated by the SSE watcher.

## Create and observe a workspace

Push the template and provide the KVM group ID from the Docker host:

```bash
KVM_GID=$(stat -c %g /dev/kvm)
coder templates push ai-sandbox-microvm \
  -d examples/templates/ai-sandbox-microvm
coder create ai-microvm \
  --template ai-sandbox-microvm \
  --parameter "kvm_gid=${KVM_GID}"
```

Immediately after the build, `coder show ai-microvm` should list both `main` and
`ai`. The `ai` agent remains disconnected until the sandbox log reports that the
guest agent launched and the guest completes its control-plane connection.

## Operator validation checklist

1. **Both declarations are visible.** Run `coder show ai-microvm` immediately
   after the build and confirm that `main` and `ai` both appear on the container
   resource. Confirm that `ai` is AI-bound in the UI.
2. **KVM is accessible.** Connect to `ai-microvm.main` and confirm that the
   workspace user can open `/dev/kvm` read-write:

   ```bash
   test -r /dev/kvm && test -w /dev/kvm && echo "KVM accessible"
   ```

3. **The sandbox command is running.** Inspect
   `/tmp/coder-agent-sandbox.log` and the PID file. Confirm that the log shows
   runtime provisioning, VM boot, and guest-agent launch.
4. **The AI agent connects.** Run `coder show ai-microvm` again and confirm that
   `ai` is connected. Then connect with:

   ```bash
   coder ssh ai-microvm.ai
   ```

5. **An allowed request succeeds.** Allow a test host and port in the Coder
   egress policy UI, then request it from the AI agent terminal:

   ```bash
   curl -I --max-time 10 https://allowed.example
   ```

6. **A denied request fails.** Request a host or port that is not allowed and
   confirm the proxy rejects it:

   ```bash
   curl -I --max-time 10 https://denied.example
   ```

7. **A live policy flip applies.** Change the allowlist in the UI without
   restarting the workspace or guest. Repeat both requests and confirm that the
   new decision applies immediately.

The default Ubuntu guest may not include `curl`. For the HTTP checks, select a
Linux amd64 guest image that already contains it. This is a validation tool in
the guest image, not a host runtime dependency.

## Current limitations and validation status

- Egress allow and deny decisions are structured entries in
  `/tmp/coder-agent-sandbox.log`. This declared sibling-agent mode has no
  server-side event retention path yet.
- Managed create-script, proxy-only, and coderd-reconciled embedded microVM modes
  remain supported. This example intentionally uses the declared two-agent
  model instead.
- Unit tests cover flag and environment parsing, fail-closed policy startup,
  live policy updates, lifecycle shutdown, and command wiring. Real boot smoke
  tests skip when `/dev/kvm` is unavailable.
- Embedded microVM boot was first validated end to end on user-provided KVM
  hardware. Guest boot, agent connection, and live allow or deny changes remain
  operator checks for each KVM host and guest image combination.
