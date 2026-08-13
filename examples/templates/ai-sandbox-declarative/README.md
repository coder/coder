---
display_name: AI sandbox (declarative)
description: An AI agent confined to a Terraform-managed sibling container
icon: ../../../site/static/icon/docker.png
maintainer_github: coder
tags: [docker, ai, security, demo]
---

# AI sandbox, declared in Terraform

A human-owned workspace containing two Terraform-managed sibling containers:
an ordinary workspace with the owner's credentials, and a sandbox containing
an AI-bound agent. The sandbox has no route to the internet except through the
policy proxy running in the workspace container.

The security declaration is one attribute on the sandbox's agent:

```hcl
resource "coder_agent" "ai" {
  ai_bound           = true
  egress_enforcement = "forced"
}
```

This demonstrates the work described in `AI_AGENT_IDENTITY_SPEC.md` and
`AI_AGENT_SANDBOX_SPEC.md`.

## What it shows

| Claim | How to see it |
|---|---|
| The AI is a distinct principal | Its API calls audit as an `ai_agent` user, on behalf of the owner |
| The AI cannot reach the owner's credentials | No user secrets, no external auth, no Git SSH key, no owner session token |
| The AI cannot escape to the owner's other workspaces | Every workspace action except read and create requires a designation match |
| The AI's egress is structurally confined | Its Docker network is internal; the workspace proxy is its only reachable egress path |
| The workspace is still the human's | The host agent keeps full credentials; `workspaces.ai_agent_id` stays NULL |

## Topology

Terraform owns all four lifecycle resources: two networks and two containers.
No Docker socket is mounted into the workspace, and no startup script creates
or destroys containers.

```text
Docker host
├── workspace network        ordinary network, external connectivity
├── sandbox network          --internal, no external route
│
├── coder-<owner>-<workspace>
│   ├── coder_agent.main     unbound, owner's credentials
│   ├── workspace network
│   ├── sandbox network      alias: coder-egress-proxy
│   └── policy proxy :13337
│
└── coder-<owner>-<workspace>-ai
    ├── coder_agent.ai       ai_bound, credential-starved
    └── sandbox network only
```

The sandbox container waits for a real TCP listener at
`coder-egress-proxy:13337` before launching its agent. Ignoring `HTTP_PROXY`
does not bypass the boundary because the internal network has no external
route.

## Recreating this yourself

### 1. Build a Coder server from the `ais` branch

```bash
git clone https://github.com/coder/coder.git
cd coder
git checkout ais
./scripts/develop.sh -- --access-url=""
```

Leave it running. It creates `admin@coder.com`; the password is printed in the
startup log.

In a second terminal:

```bash
export CODER_URL=http://localhost:3000
coder login "$CODER_URL"
```

If the development server reports a different URL, use that URL instead.

### 2. Build the Terraform provider from its branch

`ai_bound` and `egress_enforcement` are not in a published provider release.

```bash
git clone https://github.com/coder/terraform-provider-coder.git
cd terraform-provider-coder
git checkout ai-agent-identity
mkdir -p ~/.terraform.d/plugins
CGO_ENABLED=0 go build -o ~/.terraform.d/plugins/terraform-provider-coder
```

Add a development override. This file must be in the home directory of the
user running the Coder provisioner:

```hcl
# ~/.terraformrc
provider_installation {
  dev_overrides {
    "coder/coder" = "/home/YOU/.terraform.d/plugins"
  }
  direct {}
}
```

Restart `./scripts/develop.sh` after writing the override.

### 3. Verify Docker on the provisioner host

```bash
docker version
docker network create --internal ai-sandbox-probe
docker network rm ai-sandbox-probe
```

The Docker provider, not either workspace container, talks to this daemon.

### 4. Push the template and create the workspace

From the Coder repository:

```bash
coder templates push ai-sandbox-declarative \
  -d examples/templates/ai-sandbox-declarative

coder create ai-demo --template ai-sandbox-declarative
```

### 5. Confirm Terraform created both containers and both networks

```bash
WS_ID=$(coder show ai-demo --output json | jq -r .id)

docker ps \
  --filter "label=coder.workspace_id=${WS_ID}" \
  --format 'table {{.Names}}\t{{.Status}}'

docker network ls \
  --filter "name=coder-.*-ai-demo" \
  --format 'table {{.Name}}\t{{.Driver}}'
```

Expected containers resemble:

```text
coder-admin-ai-demo
coder-admin-ai-demo-ai
```

The second is the sandbox.

If the sandbox container is not running, inspect both logs:

```bash
docker logs "coder-admin-ai-demo"
docker logs "coder-admin-ai-demo-ai"
```

The sandbox fails closed after 60 seconds if the host proxy never becomes
ready.

## Demo walkthrough

Set the sandbox container name once for the direct Docker checks:

```bash
WS_ID=$(coder show ai-demo --output json | jq -r .id)
SANDBOX=$(docker ps \
  --filter "label=coder.workspace_id=${WS_ID}" \
  --filter "label=coder.ai_bound=true" \
  --format '{{.Names}}')
test -n "$SANDBOX" && echo "$SANDBOX"
```

### Two agents, one workspace, different privilege

The workspace page lists `main` and `ai`; the `ai` row carries the AI-bound
badge.

```bash
# The host agent has the owner's session token.
coder ssh ai-demo.main -- 'echo "[${CODER_SESSION_TOKEN:0:8}]"'

# The bound agent does not. This prints [].
coder ssh ai-demo.ai -- 'echo "[${CODER_SESSION_TOKEN:-}]"'
```

Nothing in the template asks Coder to strip individual variables. The only
security declaration is `ai_bound = true`; credential denial is server policy.

### Each credential source is denied separately

```bash
coder ssh ai-demo.ai   -- 'coder external-auth access-token github'; echo "rc=$?"
coder ssh ai-demo.main -- 'coder external-auth access-token github'; echo "rc=$?"

coder ssh ai-demo.ai   -- 'git ls-remote git@github.com:coder/coder.git'; echo "rc=$?"
coder ssh ai-demo.main -- 'git ls-remote git@github.com:coder/coder.git'; echo "rc=$?"
```

The positive controls require GitHub external auth and a Git SSH key to be
configured for the user.

### Egress is structurally confined

Prove the proxy is reachable:

```bash
docker exec "$SANDBOX" bash -lc \
  'echo >/dev/tcp/coder-egress-proxy/13337 && echo "proxy reachable"'
```

Prove direct internet routing is absent, explicitly bypassing proxy variables:

```bash
docker exec "$SANDBOX" bash -lc \
  'HTTP_PROXY= HTTPS_PROXY= http_proxy= https_proxy= \
   curl --noproxy "*" --max-time 3 http://1.1.1.1/ || echo "no route: denied"'
```

To show policy rather than routing, set a template egress policy allowing one
host, then request an allowed and a denied host normally from `ai-demo.ai`.
Those requests use the host proxy because the sandbox container carries
`HTTP_PROXY` and `HTTPS_PROXY`.

### The AI cannot reach the owner's other workspaces

Create a second ordinary workspace for the same user. A key scoped to the AI
identity may read workspace metadata, but SSH, application connect, start,
stop, and update require an exact designation match and are denied for the
ordinary workspace.

### The workspace is still the human's

```sql
-- NULL: ai_bound on one agent does not designate the whole workspace.
SELECT ai_agent_id FROM workspaces WHERE name = 'ai-demo';

-- Only the sandbox's agent row carries the AI identity.
SELECT name, ai_agent_id FROM workspace_agents
WHERE ai_agent_id IS NOT NULL;
```

## Ordering guarantee

Terraform's `depends_on` guarantees only that the host container exists. It
does not guarantee the agent inside has fetched policy and opened the proxy.
The sandbox entrypoint therefore waits for the actual listener before starting
`coder_agent.ai`. The wait is bounded to 60 seconds and fails closed.

## Known gaps

- **Egress events are not retained server side in proxy-only mode.** The proxy
  enforces policy, but the audit API attributes a flow through a reporting
  agent's child. The two Terraform-declared agents are siblings with no
  `parent_id`, so the workspace egress activity view remains empty.
- **`CODER_AI_SESSION_TOKEN` is not delivered.** The sandbox agent connects
  with `coder_agent.ai.token`, but in-sandbox `coder` CLI calls that require a
  user session token fail.
- **`egress_enforcement` is not persisted or surfaced.** It reaches coderd in
  the provisioner protocol but is not stored.
- **Per-agent binding is not yet sticky across template versions.** In an
  undesignated workspace, a later template version that removes `ai_bound`
  creates a replacement unbound agent row. The demo does not exercise this,
  but production use needs a durable per-agent binding decision.
- **The proxy has no per-sandbox client authentication.** The internal Docker
  network limits reachability to the two containers in this template, which is
  sufficient for the demo but not a general multi-sandbox design.
- **`ai_credential_mode` is specified but not implemented.** Every bound agent
  is effectively `none`.
