---
display_name: AI sandbox (declarative)
description: An AI agent confined to a coder/sandbox microVM, declared in Terraform
tags: [local, docker, ai]
icon: /icon/docker.png
---

# AI sandbox, declared in Terraform

A human-owned workspace containing two agents: an ordinary one holding the
owner's credentials, and an AI-bound one confined to a `coder/sandbox`
microVM with deny-by-default egress. The AI agent is declared in Terraform
with a single attribute, `ai_bound = true`.

This template demonstrates the Vertical 1 and Vertical 2 work described in
`AI_AGENT_IDENTITY_SPEC.md` and `AI_AGENT_SANDBOX_SPEC.md`.

## What the demo shows

| Claim | How it is visible |
|---|---|
| The AI acts as a distinct principal | The bound agent's API calls audit as an `ai_agent` user, on behalf of the owner |
| The AI cannot reach the owner's credentials | No user secrets, no external auth token, no Git SSH key, no owner session token |
| The AI cannot escape into the owner's other workspaces | Every workspace action except read and create requires a designation match |
| The AI's network egress is confined and recorded | `coder-sandbox` denies by default and writes `requests.log` |
| The workspace remains the human's | The host agent keeps full credentials; `workspaces.ai_agent_id` stays NULL |

## Prerequisites

This template depends on provider and server changes that are on a branch,
not in a release.

1. **A Coder server built from the `ais` branch.** The `ai_bound` handling,
   identity binding, and script environment injection all live server side
   and in the agent binary.

2. **The `coder/coder` Terraform provider built from its `ai-agent-identity`
   branch**, because `ai_bound` and `egress_enforcement` do not exist in any
   published provider release. Build it and add a dev override:

   ```bash
   cd terraform-provider-coder
   go build -o ~/.terraform.d/plugins/coder-dev
   ```

   ```hcl
   # ~/.terraformrc
   provider_installation {
     dev_overrides {
       "coder/coder" = "/home/YOU/.terraform.d/plugins"
     }
     direct {}
   }
   ```

3. **Hardware virtualization.** `/dev/kvm` must exist on the Docker host and
   is mapped into the workspace container by this template. Without it the
   microVM backend cannot boot.

4. **A workspace image carrying the tooling.** The default
   `codercom/example-base:ubuntu` does **not** include `coder-sandbox` or
   the microsandbox runtime. Build an image that has:
   - the `coder-sandbox` binary on `PATH` (`make build` in `coder/sandbox`),
   - the `msb` and `libkrunfw` runtime preseeded under `~/.microsandbox`,
     since the first boot otherwise needs outbound network access that the
     workspace may not have.

   Set `workspace_image` to that image when creating the workspace.

## Setup

```bash
coder templates push ai-sandbox-declarative \
  -d examples/templates/ai-sandbox-declarative

coder create ai-demo --template ai-sandbox-declarative
```

## Demo walkthrough

### 1. Two agents, one workspace, different privilege

Open the workspace page. Two agents appear: `main` and `ai`. The `ai` row
carries the AI-bound badge.

```bash
# The host agent has the owner's credentials.
coder ssh ai-demo.main -- 'echo $CODER_SESSION_TOKEN | head -c 12'

# The bound agent does not. This returns empty.
coder ssh ai-demo.ai -- 'echo "[$CODER_SESSION_TOKEN]"'
```

The point to make out loud: nothing in the template asked for this. The
only declaration was `ai_bound = true`; withholding credentials is a server
decision that a template cannot override.

### 2. The credential sources are individually denied

Starvation is enforced at each source, not by stripping one env var:

```bash
# All three fail for the bound agent and succeed for the host agent.
coder ssh ai-demo.ai   -- 'coder external-auth access-token github'
coder ssh ai-demo.main -- 'coder external-auth access-token github'

coder ssh ai-demo.ai   -- 'git ls-remote git@github.com:coder/coder.git'
coder ssh ai-demo.main -- 'git ls-remote git@github.com:coder/coder.git'
```

### 3. The AI is a distinct principal in the audit log

Have the bound agent do something auditable, then look at who did it:

```bash
coder ssh ai-demo.ai -- 'coder whoami'
```

In the audit log the actor is the agent identity, and the on-behalf-of
field is the human owner. Filter by the human and their agents' actions
appear alongside their own; filter by the agent and only the agent's do.

### 4. Egress is denied by default

```bash
# The coderd host is always allowed, or the agent could not connect.
coder ssh ai-demo.ai -- 'curl -sS -o /dev/null -w "%{http_code}\n" https://YOUR-CODER-HOST'

# Anything else is denied by coder-sandbox.
coder ssh ai-demo.ai -- 'curl -sS --max-time 5 https://example.org' || echo denied
```

Now widen the allowlist through the `sandbox_allow` parameter, restart, and
show the same request succeeding. The allowlist is an administrator control:
nothing inside the sandbox can change it.

Show the recorded flows:

```bash
coder ssh ai-demo.main -- 'coder-sandbox ls'
# and the sandbox's own request log, path per coder/sandbox docs
```

### 5. The AI cannot reach the owner's other workspaces

Create a second, ordinary workspace for the same user. Then, using a
credential scoped to the AI identity, attempt to reach it. Reads succeed;
SSH, start, stop, and update are denied, because the designation boundary
requires the workspace's `ai_agent_id` to equal the acting identity.

This is the property that makes credential starvation meaningful: without
it, an agent denied credentials in its own workspace could simply SSH into
one that still has them.

### 6. The workspace is still the human's

```bash
# NULL: an ai_bound agent does not designate its workspace.
# The host agent therefore keeps its credentials and its policy.
psql -c "SELECT ai_agent_id FROM workspaces WHERE name = 'ai-demo'"
```

## Ordering guarantee worth pointing out

The startup script logs the platform egress proxy address before booting
the sandbox. That address exists because the agent starts its proxy and
waits for it to be listening **before** running any startup script. There is
no window in which a sandbox is running unconfined, and the guarantee is
covered by a test that asserts the script cannot run while readiness is
withheld.

## What this backend does and does not give you

`coder/sandbox` owns the guest's egress: the guest can open exactly one TCP
path, to the sandbox's own recording proxy, which applies the allowlist and
writes `requests.log`. Consequences worth stating plainly during the demo:

- Enforcement and per-request recording are **coder/sandbox's**, and they
  are strong: the guest cannot bypass them.
- The platform still owns identity, binding, credential starvation, the
  audit attribution, and the attestation.
- **The platform's own egress event stream stays empty for this sandbox.**
  The guest cannot reach `CODER_EGRESS_PROXY`, so the workspace egress
  activity view will show nothing. Use the sandbox's `requests.log` instead.

A Docker-container backend routes through the platform proxy and does
populate that view, at the cost of weaker isolation. The two are a real
tradeoff, not a preference.

## Not yet wired

Be honest about these if they come up:

- **`CODER_AI_SESSION_TOKEN` is not delivered.** The bound agent's *agent*
  token comes from Terraform (`coder_agent.ai.token`), which is why the demo
  works. The scoped AI *session* token, for `coder` CLI calls inside the
  sandbox, requires server-side minting under the AI identity that is not
  implemented. In-sandbox CLI commands that need a session token will fail.
- **`egress_enforcement` is not persisted.** It reaches coderd through the
  provisioner but is not yet stored or surfaced in the API or UI.
- **The platform proxy has no client authentication.** In a container
  backend it must bind an address the sandbox can reach, and anything else
  that can reach it could use the workspace's allowlist. This backend does
  not expose that, because its guest cannot reach the proxy at all.
- **`ai_credential_mode`** is specified in the design but not implemented.
  Every bound agent is effectively `none`.
