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

Write a Terraform CLI configuration with a development override and point the
provisioner at that exact file with `TF_CLI_CONFIG_FILE`. The `dev_overrides`
value must be the directory containing the built `terraform-provider-coder`
binary; Terraform does not accept a file path there. Replace
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

Save the configuration to a file, for example `/home/coder/coder-dev.tfrc`,
and set `TF_CLI_CONFIG_FILE` on the process that runs the provisioner. With
built-in provisioners that is the `coder server` process:

```bash
TF_CLI_CONFIG_FILE=/home/coder/coder-dev.tfrc coder server ...
```

The provisioner passes `TF_*` variables through to Terraform, so the exact
file is used regardless of the provisioner user's home directory. Writing the
same content to that user's `~/.terraformrc` also works. Terraform prints a
warning when a development override is active. That warning is expected.

## Prerequisites

- Coder server code from the `ais` branch.
- The provider development override described above.
- A Linux amd64 Docker provisioner host with hardware virtualization enabled.
- Passwordless sudo for the workspace user (true for
  `codercom/enterprise-base` images). The sandbox host process serves the
  guest's virtio-fs filesystem, so guest `chown` calls execute with that
  process's host privileges. Without root, guest package managers fail with
  ownership errors (the CAP_CHOWN preflight warning). The startup script
  elevates with `sudo -E` when available; the microVM remains the security
  boundary.
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

The command automatically allows the exact `CODER_AGENT_URL` hostname at its
effective port, including private or loopback resolutions for on-premises
installations. Operators do not need to add the Coder access URL to the template
egress policy. Policy edits cannot remove this platform control-channel
allowance.

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

Expected startup messages include the injected platform control-channel
allowance, initial policy rule count, runtime and image provisioning, microVM
boot, and guest-agent launch. A missing KVM device, unsupported platform, or
guest boot failure appears directly in this log.

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
logs the error loudly and still boots the guest with only the platform
control-channel destination allowed. A later successful SSE revision atomically
replaces the active administrator policy. The evaluator-level control-channel
allowance remains in place across every revision.

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

## Claude Code in the sandbox

The `ai` agent installs Claude Code at startup and exposes a `Claude Code`
app on the workspace page that opens it in the guest. Model traffic routes
through the Coder AI gateway (`ANTHROPIC_BASE_URL`) authenticated as the AI
agent identity (`ANTHROPIC_AUTH_TOKEN`), so no provider API key enters the
guest and every request is metered per identity. An Anthropic provider must
be configured under Admin settings, AI, Providers.

The installer downloads through the sandbox egress proxy, so the template's
AI egress policy must allow the download and package hosts:

| Host                       | Ports | Purpose                    |
|----------------------------|-------|----------------------------|
| `claude.ai`                | 443   | Claude Code installer      |
| `storage.googleapis.com`   | 443   | Claude Code binary         |
| `archive.ubuntu.com`       | 80    | curl package (first boot)  |
| `security.ubuntu.com`      | 80    | curl package (first boot)  |

If installation fails (policy denials appear in the sandbox log), the agent
stays usable; rerun by restarting the workspace after fixing the policy.

## MCP gateway access

Sandboxed agents reach MCP servers through the Coder MCP gateway instead of
connecting to them directly, so no new egress rules are needed and the
sponsoring human's OAuth credentials never enter the guest. See
`AI_AGENT_MCP_GATEWAY_SPEC.md` for the full contract.

- Endpoint: `<access URL>/api/v2/ai-gateway/mcp/{server-slug}` over
  streamable HTTP.
- Credential: the scoped AI identity session token as a bearer token. Inside
  the guest it is exposed as `CODER_SESSION_TOKEN` when delivered.
- Administrators configure servers under Admin settings, AI, MCP Servers:
  choose the `External auth provider` authentication method, select a
  configured provider, and set tool rules (a server default plus per-tool
  overrides).
- When the sponsoring human has not authenticated with the provider, tool
  calls return a JSON-RPC error whose data carries a `reauth_url`; opening
  that URL in a browser completes the provider login and unblocks the agent.

### Session token delivery

The template declares `data.coder_workspace_ai_agent.me`, which opts the
workspace into an AI agent identity: coderd detects the data source at
template import and mints a scoped session token at every build, sponsored
by the workspace owner. The template passes that token to
`coder agent sandbox` as `CODER_SANDBOX_SESSION_TOKEN`, and the guest agent
exposes it as `CODER_SESSION_TOKEN`. The token carries the AI identity's
restricted scopes, including AI/MCP gateway access, and never the owner's
full permissions. The managed embedded mode
(`CODER_AI_SANDBOX_MICROVM=true`) delivers an equivalent token
automatically.

### MCP validation checklist

1. Configure an MCP server with external auth and at least one disabled
   tool rule.
2. From the guest, list tools:
   `curl -X POST -H "Authorization: Bearer $CODER_SESSION_TOKEN" ...`
   against the gateway URL and confirm disabled tools are absent.
3. Call an allowed tool and confirm it succeeds using the sponsor's
   provider identity.
4. Call the disabled tool directly and confirm a JSON-RPC policy denial.
5. Revoke the provider link at `<access URL>/external-auth/{provider}` and
   confirm the next call returns the structured re-auth error.

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
