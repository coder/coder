---
display_name: AI sandbox (declarative)
description: An AI agent confined to a sibling container, declared in Terraform
icon: ../../../site/static/icon/docker.png
maintainer_github: coder
tags: [docker, ai, security, demo]
---

# AI sandbox, declared in Terraform

A human-owned workspace containing two agents: an ordinary one holding the
owner's credentials, and an AI-bound one confined to a sibling container with
no route to the internet except through a policy proxy. The AI agent is
declared with a single Terraform attribute, `ai_bound = true`.

This demonstrates the work described in `AI_AGENT_IDENTITY_SPEC.md` and
`AI_AGENT_SANDBOX_SPEC.md`.

## What it shows

| Claim | How to see it |
|---|---|
| The AI is a distinct principal | Its API calls audit as an `ai_agent` user, on behalf of the owner |
| The AI cannot reach the owner's credentials | No user secrets, no external auth, no Git SSH key, no owner session token |
| The AI cannot escape to the owner's other workspaces | Every workspace action except read and create requires a designation match |
| The AI's egress is structurally confined | Its network has no route out; the proxy is the only path |
| The workspace is still the human's | The host agent keeps full credentials; `workspaces.ai_agent_id` stays NULL |

## Topology

The sandbox is a **sibling** container, not a nested one. The Docker CLI runs
in the workspace but the daemon is the host's, so both containers are peers:

```text
Docker host
├── coder-you-ai-demo         workspace: owner's credentials, egress proxy
│      │  docker CLI -> /var/run/docker.sock (host daemon)
│      └── attached to sbnet-<id> as "coder-egress-proxy"
└── sb-<id>                   sandbox: the AI-bound agent
       └── on sbnet-<id> only, an --internal network with no route out
```

`--internal` is the enforcement. A process in the sandbox that ignores the
proxy environment variables still cannot route anywhere; the only address it
can reach is the workspace's, on that network, where the proxy listens.

## Recreating this yourself

### 1. Build a Coder server from the `ais` branch

The `ai_bound` handling, identity binding, and script environment injection
are all server side and in the agent binary.

```bash
git clone https://github.com/coder/coder.git
cd coder && git checkout ais
./scripts/develop.sh
```

That serves on `http://localhost:3000` and creates a first user. Leave it
running.

### 2. Build the Terraform provider from its branch

`ai_bound` and `egress_enforcement` are not in any published provider
release, so the provider must be built and overridden locally.

```bash
git clone https://github.com/coder/terraform-provider-coder.git
cd terraform-provider-coder && git checkout ai-agent-identity
mkdir -p ~/.terraform.d/plugins
go build -o ~/.terraform.d/plugins/terraform-provider-coder
```

Add a dev override so Terraform uses it instead of the registry:

```hcl
# ~/.terraformrc
provider_installation {
  dev_overrides {
    "coder/coder" = "/home/YOU/.terraform.d/plugins"
  }
  direct {}
}
```

### 3. Confirm the host can do what the template needs

```bash
docker version                      # a reachable daemon
docker network create --internal x  # internal networks are permitted
docker network rm x
```

The workspace image must contain the Docker CLI.
`codercom/enterprise-base:ubuntu`, the default here, does.

### 4. Push the template and create a workspace

```bash
coder templates push ai-sandbox-declarative \
  -d examples/templates/ai-sandbox-declarative

coder create ai-demo --template ai-sandbox-declarative
```

### 5. Confirm the sandbox came up

```bash
coder ssh ai-demo.main -- docker ps --filter name=sb- --format '{{.Names}}'
coder ssh ai-demo.main -- docker network ls --filter name=sbnet-
```

The startup script's log, in the workspace page or `coder logs`, shows the
proxy address it was given and the network it created.

## Demo walkthrough

### Two agents, one workspace, different privilege

The workspace page lists `main` and `ai`; the `ai` row carries the AI-bound
badge.

```bash
# The host agent has the owner's session token.
coder ssh ai-demo.main -- 'echo "[${CODER_SESSION_TOKEN:0:8}]"'

# The bound agent does not. This prints [].
coder ssh ai-demo.ai -- 'echo "[${CODER_SESSION_TOKEN:-}]"'
```

Worth saying out loud: nothing in the template asked for this. The only
declaration was `ai_bound = true`. Withholding credentials is a server
decision that a template cannot override, and setting `ai_bound = false`
later cannot undo it for a workspace the server has already designated.

### Each credential source is denied separately

Starvation is enforced at every source, not by stripping one variable:

```bash
coder ssh ai-demo.ai   -- 'coder external-auth access-token github' ; echo "rc=$?"
coder ssh ai-demo.main -- 'coder external-auth access-token github' ; echo "rc=$?"

coder ssh ai-demo.ai   -- 'git ls-remote git@github.com:coder/coder.git' ; echo "rc=$?"
coder ssh ai-demo.main -- 'git ls-remote git@github.com:coder/coder.git' ; echo "rc=$?"
```

### Egress is structurally confined

```bash
# The sandbox has no route to the internet at all.
coder ssh ai-demo.main -- \
  docker exec sb-$(coder show ai-demo --output json | jq -r .id) \
  sh -c 'wget -q -T 3 -O - http://1.1.1.1/ || echo "no route: denied"'

# It can reach the workspace's proxy, and only that.
coder ssh ai-demo.main -- \
  docker exec sb-$(coder show ai-demo --output json | jq -r .id) \
  sh -c 'wget -q -T 3 -O - http://coder-egress-proxy:13337/ ; echo'
```

The second command proves the path exists; the first proves it is the only
one. A process that ignores `HTTP_PROXY` gets the first result.

To show policy rather than routing, set a template egress policy allowing one
host, then request an allowed and a denied host through the proxy from inside
the sandbox.

### The AI cannot reach the owner's other workspaces

Create a second, ordinary workspace for the same user. Using a credential
scoped to the AI identity, reads succeed but SSH, start, stop, and update are
denied: the designation boundary requires the workspace's `ai_agent_id` to
equal the acting identity.

This is what makes credential starvation meaningful. Without it, an agent
denied credentials in its own workspace could simply connect to one that
still has them.

### The workspace is still the human's

```sql
-- NULL: an ai_bound agent does not designate its workspace, which is why the
-- host agent keeps its credentials and its policy.
SELECT ai_agent_id FROM workspaces WHERE name = 'ai-demo';

-- The bound agent row, by contrast, carries an identity.
SELECT name, ai_agent_id FROM workspace_agents
WHERE ai_agent_id IS NOT NULL;
```

## Ordering guarantee

The startup script logs the proxy address before creating anything. That
address exists because the agent starts its proxy and waits for it to listen
**before** running any startup script, so there is no window in which the
sandbox runs unconfined. The guarantee is covered by a test that asserts a
script cannot run while proxy readiness is withheld.

## Known gaps

State these plainly if they come up.

- **Egress events are not retained server side in this mode.** The proxy
  enforces policy and logs decisions locally, but the audit endpoints
  attribute a flow through the reporting agent's *child*, and a
  Terraform-declared `ai_bound` agent is a *sibling* with no `parent_id`. The
  workspace egress activity view will therefore be empty. Closing this needs
  a parent relationship between the host agent and the declared agent.
- **`CODER_AI_SESSION_TOKEN` is not delivered.** The bound agent's *agent*
  token comes from Terraform, which is why the demo works. The scoped AI
  *session* token, for `coder` CLI calls inside the sandbox, needs server-side
  minting under the AI identity that is not implemented.
- **`egress_enforcement` is not persisted.** It reaches coderd through the
  provisioner but is not yet stored or surfaced.
- **The host Docker socket is mounted into the workspace**, which is
  effectively host root for anything that can reach it. The AI cannot: the
  socket is in the workspace, and the sandbox has no route there. Rootless
  Podman removes the socket entirely and is the production direction.
- **`ai_credential_mode`** is specified but not implemented; every bound agent
  is effectively `none`.
